package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/xuri/excelize/v2"

	"buildcost/internal/audit"
	"buildcost/internal/calc"
)

// ReportData is the reviewed, structured snapshot a report is rendered from.
// Reports are never rendered from AI output.
type ReportData struct {
	Project      Project            `json:"project"`
	Version      int                `json:"version"`
	GeneratedAt  time.Time          `json:"generated_at"`
	Approver     string             `json:"approver"`
	Result       audit.Result       `json:"result"`
	Findings     []FindingRow       `json:"findings"` // confirmed by a reviewer
	Dismissed    int                `json:"dismissed"`
	Assumptions  []Assumption       `json:"assumptions"`
	Measurements []Measurement      `json:"measurements"`
	BuildUps     []calc.RateBuildUp `json:"build_ups"`
}

func (d *ReportData) title() string {
	if d.Project.Service == "estimate" {
		return "Construction Quantity Takeoff & Cost Estimate"
	}
	return "Independent Construction Estimate & BOQ Audit Report"
}

// approve re-runs the analysis on current data, requires every finding to be
// reviewed, then freezes a snapshot and renders the deliverables.
func (s *Server) approve(w http.ResponseWriter, r *http.Request, u *User, p *Project) {
	ctx := r.Context()
	res, err := s.runAnalysis(ctx, p, u.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	if len(res.Lines) == 0 {
		fail(w, 409, "the BOQ is empty; there is nothing to report")
		return
	}
	all, err := s.findings(ctx, p.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	data := ReportData{Project: *p, GeneratedAt: time.Now(), Approver: u.Name, Result: *res, Findings: []FindingRow{}}
	open := 0
	for _, f := range all {
		switch f.Status {
		case "open":
			open++
		case "confirmed":
			data.Findings = append(data.Findings, f)
		default:
			data.Dismissed++
		}
	}
	if open > 0 {
		fail(w, 409, fmt.Sprintf("%d finding(s) are still open. Every finding must be confirmed or dismissed by a reviewer before approval (data changes re-open affected findings).", open))
		return
	}
	if data.Assumptions, err = query[Assumption](ctx, s.DB, `SELECT * FROM assumptions WHERE project_id=$1 ORDER BY kind, id`, p.ID); err != nil {
		serverError(w, err)
		return
	}
	if data.Measurements, err = s.measurements(ctx, p.ID); err != nil {
		serverError(w, err)
		return
	}
	rates, err := s.ratesFor(ctx, p)
	if err != nil {
		serverError(w, err)
		return
	}
	data.BuildUps = s.buildUps(rates)
	if err := s.DB.QueryRow(ctx, `SELECT coalesce(max(version),0)+1 FROM reports WHERE project_id=$1`, p.ID).Scan(&data.Version); err != nil {
		serverError(w, err)
		return
	}

	dir := fmt.Sprintf("p%d/reports", p.ID)
	if err := os.MkdirAll(filepath.Join(s.DataDir, dir), 0o750); err != nil {
		serverError(w, err)
		return
	}
	pdfKey := fmt.Sprintf("%s/v%d.pdf", dir, data.Version)
	xlsxKey := fmt.Sprintf("%s/v%d.xlsx", dir, data.Version)
	if err := writePDF(&data, filepath.Join(s.DataDir, pdfKey)); err != nil {
		serverError(w, fmt.Errorf("render pdf: %w", err))
		return
	}
	if err := writeXLSX(&data, filepath.Join(s.DataDir, xlsxKey)); err != nil {
		serverError(w, fmt.Errorf("render xlsx: %w", err))
		return
	}
	snap, _ := json.Marshal(data)
	if _, err := s.DB.Exec(ctx, `INSERT INTO reports (project_id, version, approved_by, pdf_key, xlsx_key, snapshot) VALUES ($1,$2,$3,$4,$5,$6)`,
		p.ID, data.Version, u.ID, pdfKey, xlsxKey, snap); err != nil {
		serverError(w, err)
		return
	}
	_, err = s.DB.Exec(ctx, `UPDATE projects SET status='approved' WHERE id=$1 AND status <> 'delivered'`, p.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"version": data.Version})
}

func (s *Server) deliver(w http.ResponseWriter, r *http.Request, u *User, p *Project) {
	var n int
	_ = s.DB.QueryRow(r.Context(), `SELECT count(*) FROM reports WHERE project_id=$1`, p.ID).Scan(&n)
	if n == 0 {
		fail(w, 409, "approve a report before delivering")
		return
	}
	if p.PaidAt == nil {
		fail(w, 409, "payment has not been recorded for this order")
		return
	}
	_, err := s.DB.Exec(r.Context(), `UPDATE projects SET status='delivered' WHERE id=$1`, p.ID)
	s.done(w, err)
}

func (s *Server) listReports(w http.ResponseWriter, r *http.Request, u *User, p *Project) {
	if !u.Staff() && p.Status != "delivered" {
		writeJSON(w, 200, []Report{})
		return
	}
	rows, err := query[Report](r.Context(), s.DB, `SELECT r.id, r.project_id, r.version, u.name AS approver, r.pdf_key, r.xlsx_key, r.created_at
		FROM reports r JOIN users u ON u.id = r.approved_by WHERE r.project_id=$1 ORDER BY r.version DESC`, p.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	if !u.Staff() && len(rows) > 1 {
		rows = rows[:1] // customers receive the latest approved version
	}
	writeJSON(w, 200, rows)
}

func (s *Server) downloadReport(w http.ResponseWriter, r *http.Request, u *User) {
	rep, err := queryOne[Report](r.Context(), s.DB, `SELECT r.id, r.project_id, r.version, '' AS approver, r.pdf_key, r.xlsx_key, r.created_at FROM reports r WHERE r.id=$1`, pathID(r))
	if err != nil {
		fail(w, 404, "not found")
		return
	}
	p, err := s.loadProject(r.Context(), rep.ProjectID)
	if err != nil || (!u.Staff() && (p.CustomerID != u.ID || p.Status != "delivered")) {
		fail(w, 404, "not found")
		return
	}
	base := fmt.Sprintf("BuildCost-P%d-v%d", p.ID, rep.Version)
	switch r.PathValue("format") {
	case "pdf":
		s.serveFile(w, r, rep.PDFKey, base+".pdf")
	case "xlsx":
		s.serveFile(w, r, rep.XLSXKey, base+".xlsx")
	default:
		fail(w, 404, "not found")
	}
}

// ---- PDF ----

type pdfDoc struct {
	*fpdf.Fpdf
	tr func(string) string
}

var (
	ink   = [3]int{24, 32, 44}
	muted = [3]int{98, 108, 122}
	brand = [3]int{15, 76, 92}
	rule  = [3]int{215, 220, 226}
)

func (p *pdfDoc) color(c [3]int) { p.SetTextColor(c[0], c[1], c[2]) }

func (p *pdfDoc) text(style string, size float64, c [3]int, s string) {
	p.SetFont("Helvetica", style, size)
	p.color(c)
	p.MultiCell(0, size*0.48, p.tr(s), "", "L", false)
}

func (p *pdfDoc) heading(n, s string) {
	if p.GetY() > 245 {
		p.AddPage()
	}
	p.Ln(5)
	p.SetFont("Helvetica", "B", 8)
	p.color(brand)
	p.CellFormat(0, 5, p.tr(strings.ToUpper(n)), "", 1, "L", false, 0, "")
	p.SetFont("Helvetica", "B", 14)
	p.color(ink)
	p.CellFormat(0, 8, p.tr(s), "", 1, "L", false, 0, "")
	p.SetDrawColor(rule[0], rule[1], rule[2])
	p.Line(15, p.GetY()+1, 195, p.GetY()+1)
	p.Ln(4)
}

// row draws one table row with wrapped cells and a bottom rule.
func (p *pdfDoc) row(widths []float64, cells []string, aligns string, style string, size float64, c [3]int) {
	p.SetFont("Helvetica", style, size)
	p.color(c)
	lh := size * 0.45
	lines := make([][]string, len(cells))
	maxLines := 1
	for i, cell := range cells {
		lines[i] = p.SplitText(p.tr(cell), widths[i]-2)
		if len(lines[i]) > maxLines {
			maxLines = len(lines[i])
		}
	}
	h := float64(maxLines)*lh + 2.4
	if p.GetY()+h > 280 {
		p.AddPage()
	}
	x, y := 15.0, p.GetY()
	for i, ls := range lines {
		for j, l := range ls {
			p.SetXY(x+1, y+1.2+float64(j)*lh)
			p.CellFormat(widths[i]-2, lh, l, "", 0, string(aligns[i]), false, 0, "")
		}
		x += widths[i]
	}
	p.SetDrawColor(rule[0], rule[1], rule[2])
	p.Line(15, y+h, 195, y+h)
	p.SetXY(15, y+h)
}

func (p *pdfDoc) stat(x, w float64, label, value string) {
	y := p.GetY()
	p.SetXY(x, y)
	p.SetFont("Helvetica", "", 8)
	p.color(muted)
	p.CellFormat(w, 5, p.tr(label), "", 2, "L", false, 0, "")
	p.SetFont("Helvetica", "B", 16)
	p.color(ink)
	p.CellFormat(w, 9, p.tr(value), "", 0, "L", false, 0, "")
	p.SetY(y)
}

func rs(v float64) string { return "Rs " + audit.FormatINR(v) }
func num(v float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
}

func writePDF(d *ReportData, path string) error {
	f := fpdf.New("P", "mm", "A4", "")
	p := &pdfDoc{Fpdf: f, tr: f.UnicodeTranslatorFromDescriptor("")}
	p.SetMargins(15, 15, 15)
	p.SetAutoPageBreak(true, 16)
	ref := fmt.Sprintf("BC-P%d-v%d", d.Project.ID, d.Version)
	p.SetFooterFunc(func() {
		p.SetY(-11)
		p.SetFont("Helvetica", "", 7)
		p.color(muted)
		p.CellFormat(0, 4, p.tr(fmt.Sprintf("BuildCost Intelligence  |  %s  |  Page %d", ref, p.PageNo())), "", 0, "C", false, 0, "")
	})
	pr, sum := d.Project, d.Result.Summary
	isAudit := pr.Service != "estimate"

	// Cover
	p.AddPage()
	p.SetFont("Helvetica", "B", 11)
	p.color(brand)
	p.CellFormat(0, 6, "BUILDCOST", "", 1, "L", false, 0, "")
	p.text("", 8, muted, "CONSTRUCTION COST INTELLIGENCE")
	p.Ln(22)
	p.text("B", 22, ink, d.title())
	p.Ln(3)
	p.text("", 12, muted, fmt.Sprintf("%s  -  %s, %s", pr.Title, pr.City, pr.State))
	p.Ln(10)
	w2 := []float64{50, 130}
	for _, kv := range [][2]string{
		{"Report reference", ref}, {"Date", d.GeneratedAt.Format("2 January 2006")},
		{"Prepared for", pr.CustomerName}, {"Building", fmt.Sprintf("%s, %s, %s sq ft built-up", pr.BuildingType, pr.Floors, num(pr.AreaSqft))},
		{"Reviewed and approved by", d.Approver}, {"Calculation rules", "Version " + sum.RuleVersion},
	} {
		p.row(w2, []string{kv[0], kv[1]}, "LL", "", 9.5, ink)
	}
	p.Ln(8)
	p.text("", 8, muted, "This report is an independent desk assessment based only on the documents supplied by the client. It distinguishes quantities that were independently measured from figures carried as quoted; see section \"Basis and limitations\". It is not a structural design check, a site inspection or a guarantee of final construction cost.")

	// 01 Executive assessment
	p.heading("01", "Executive assessment")
	if isAudit {
		p.stat(15, 60, "Contractor quoted (sum of items)", rs(sum.QuotedTotal))
		p.stat(75, 60, "Independently assessed", rs(sum.AssessedTotal))
		p.stat(135, 60, "Difference requiring reconciliation", rs(sum.Difference))
		p.Ln(19)
		p.text("", 9, ink, fmt.Sprintf("%.1f%% of the quoted value was checked against an independent quantity takeoff, and %.1f%% against a reference rate build-up. Items that could not be independently assessed are carried at the contractor's quoted figure, so the assessed amount is not a complete independent estimate.", sum.MeasuredPct, sum.RateCheckedPct))
		p.Ln(1.5)
		p.text("", 9, ink, "The difference is not automatically an achievable saving. It identifies value that should be reconciled with the contractor against the findings below before a contract is signed.")
		if sum.DeclaredTotal > 0 && sum.DeclaredTotal != sum.QuotedTotal {
			p.Ln(1.5)
			p.text("", 9, ink, fmt.Sprintf("The quotation states a total of %s; the listed items add up to %s.", rs(sum.DeclaredTotal), rs(sum.QuotedTotal)))
		}
	} else {
		p.stat(15, 90, "Estimated construction cost (items measured)", rs(sum.AssessedTotal))
		p.Ln(19)
		p.text("", 9, ink, "This estimate covers only the work items listed in the BOQ below, at the quantities measured from the supplied drawings and the rates shown. Work not listed is excluded.")
	}
	if sum.UnverifiedRates {
		p.Ln(1.5)
		p.text("I", 9, muted, "Some reference prices available for this location are unverified placeholders. They were used for indicative comparison only and were not applied to any assessed amount.")
	}

	// 02 Cost breakdown
	p.heading("02", "Cost breakdown by work category")
	wc := []float64{80, 50, 50}
	if isAudit {
		p.row(wc, []string{"Work category", "Quoted", "Assessed"}, "LRR", "B", 8.5, muted)
		for _, c := range sum.ByCategory {
			p.row(wc, []string{c.Category, rs(c.Quoted), rs(c.Assessed)}, "LRR", "", 9, ink)
		}
		p.row(wc, []string{"Total", rs(sum.QuotedTotal), rs(sum.AssessedTotal)}, "LRR", "B", 9, ink)
	} else {
		p.row(wc, []string{"Work category", "", "Estimated"}, "LRR", "B", 8.5, muted)
		for _, c := range sum.ByCategory {
			p.row(wc, []string{c.Category, "", rs(c.Assessed)}, "LRR", "", 9, ink)
		}
		p.row(wc, []string{"Total", "", rs(sum.AssessedTotal)}, "LRR", "B", 9, ink)
	}

	// 03 Findings
	p.heading("03", "Findings and observations")
	if len(d.Findings) == 0 {
		p.text("", 9, ink, "No findings were confirmed by the reviewer.")
	}
	label := map[string]string{"major": "MAJOR", "review": "REVIEW", "info": "NOTE"}
	for i, f := range d.Findings {
		if p.GetY() > 250 {
			p.AddPage()
		}
		p.SetFont("Helvetica", "B", 7.5)
		p.color(brand)
		tag := fmt.Sprintf("%s  -  F%02d  -  %s", label[f.Severity], i+1, f.Code)
		if f.Impact != 0 {
			tag += "  -  value affected " + rs(f.Impact)
		}
		p.CellFormat(0, 4.5, p.tr(tag), "", 1, "L", false, 0, "")
		p.text("B", 10, ink, f.Title)
		p.text("", 9, ink, f.Explanation)
		p.text("", 9, ink, "Action: "+f.Action)
		if f.AnalystNote != "" {
			p.text("I", 9, muted, "Reviewer's note: "+f.AnalystNote)
		}
		p.Ln(3.5)
	}

	// 04 BOQ
	p.heading("04", "Bill of quantities assessment")
	wb := []float64{10, 52, 12, 17, 17, 23, 17, 23, 9}
	head := []string{"Sl", "Description", "Unit", "Quoted qty", "Quoted rate", "Quoted amount", "Assessed qty", "Assessed amount", "Basis"}
	if !isAudit {
		head = []string{"Sl", "Description", "Unit", "", "", "", "Quantity", "Amount", "Basis"}
	}
	p.row(wb, head, "LLLRRRRRL", "B", 7, muted)
	for _, l := range d.Result.Lines {
		basis := map[string]string{"measured": "M", "quoted": "Q"}[l.QtyBasis] + "/" + map[string]string{"reference": "R", "quoted": "Q"}[l.RateBasis]
		cells := []string{l.SlNo, l.Description, l.Unit, num(l.QuotedQty), num(l.QuotedRate), audit.FormatINR(l.QuotedAmount), num(l.AssessedQty), audit.FormatINR(l.AssessedAmount), basis}
		if !isAudit {
			cells[3], cells[4], cells[5] = "", "", ""
			cells[8] = map[string]string{"measured": "M", "quoted": "E"}[l.QtyBasis]
		}
		p.row(wb, cells, "LLLRRRRRL", "", 7.5, ink)
	}
	p.Ln(2)
	if isAudit {
		p.text("", 7.5, muted, "Basis (quantity/rate): M = independently measured from drawings; R = reference rate build-up from verified prices; Q = carried as quoted by the contractor, not independently assessed. Amounts in rupees.")
	} else {
		p.text("", 7.5, muted, "Basis: M = measured from drawings (see measurement sheets in the Excel workbook); E = entered by the estimator. Amounts in rupees.")
	}

	// 05 Materials
	if len(sum.Materials) > 0 {
		p.heading("05", "Indicative material and labour requirement")
		wm := []float64{90, 40, 50}
		p.row(wm, []string{"Resource", "Unit", "Quantity"}, "LLR", "B", 8.5, muted)
		for _, m := range sum.Materials {
			p.row(wm, []string{m.Name, m.Unit, num(m.Qty)}, "LLR", "", 9, ink)
		}
		p.Ln(2)
		p.text("", 7.5, muted, sum.MaterialsBasis+" Covers only work items with a supported consumption build-up.")
	}

	// 06 Assumptions
	p.heading("06", "Assumptions and exclusions")
	if len(d.Assumptions) == 0 {
		p.text("", 9, ink, "None recorded.")
	}
	for _, a := range d.Assumptions {
		p.text("", 9, ink, fmt.Sprintf("%s: %s", strings.ToUpper(a.Kind[:1])+a.Kind[1:], a.Body))
		p.Ln(1)
	}

	// 07 Basis
	p.heading("07", "Basis and limitations")
	for _, s := range []string{
		"Quantities marked as measured were taken off from the supplied drawings by an estimator using the measurement rules referenced in the workbook (IS 1200 conventions). Each measured quantity is traceable to its dimensions in the measurement sheet.",
		"Reference rates are built up from material and labour consumption coefficients, dated reference prices for the project location, and " + num(calc.OverheadProfitPct) + "% overhead and profit. Only prices recorded as verified against a dated source are applied to assessed amounts.",
		"The measurement rules and consumption coefficients in rule set " + sum.RuleVersion + " are standard published values in draft status; they should be read as an estimator's reference, not as contract rates.",
		"Findings are generated by deterministic checks and were individually confirmed by the reviewer named on the cover. " + fmt.Sprintf("%d generated finding(s) were dismissed on review.", d.Dismissed),
		"This assessment relies on the completeness and accuracy of the documents supplied. It does not verify structural adequacy, site conditions, workmanship, statutory compliance or the contractor's commercial terms.",
	} {
		p.text("", 8.5, ink, s)
		p.Ln(1.5)
	}
	return p.OutputFileAndClose(path)
}

// ---- Excel workbook ----

func writeXLSX(d *ReportData, path string) error {
	f := excelize.NewFile()
	defer f.Close()
	bold, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	sheet := func(name string, widths []float64, header []any) func(row ...any) {
		if idx, _ := f.GetSheetIndex(name); idx < 0 {
			f.NewSheet(name)
		}
		for i, w := range widths {
			col, _ := excelize.ColumnNumberToName(i + 1)
			f.SetColWidth(name, col, col, w)
		}
		f.SetSheetRow(name, "A1", &header)
		f.SetRowStyle(name, 1, 1, bold)
		n := 1
		return func(row ...any) {
			n++
			f.SetSheetRow(name, fmt.Sprintf("A%d", n), &row)
		}
	}
	f.SetSheetName("Sheet1", "BOQ")
	sum := d.Result.Summary

	add := sheet("BOQ", []float64{6, 48, 8, 11, 11, 14, 10, 12, 12, 12, 12, 14, 10, 10},
		[]any{"Sl", "Description", "Unit", "Quoted qty", "Quoted rate", "Quoted amount", "Work item", "Measured qty", "Reference rate", "Assessed qty", "Assessed rate", "Assessed amount", "Qty basis", "Rate basis"})
	for _, l := range d.Result.Lines {
		var mq, rr any
		if l.MeasuredQty != nil {
			mq = *l.MeasuredQty
		}
		if l.ReferenceRate != nil {
			rr = *l.ReferenceRate
		}
		add(l.SlNo, l.Description, l.Unit, l.QuotedQty, l.QuotedRate, l.QuotedAmount, l.WorkItem, mq, rr, l.AssessedQty, l.AssessedRate, l.AssessedAmount, l.QtyBasis, l.RateBasis)
	}
	add()
	add("", "TOTAL", "", "", "", sum.QuotedTotal, "", "", "", "", "", sum.AssessedTotal)

	add = sheet("Measurement Sheet", []float64{12, 34, 40, 8, 8, 9, 9, 9, 9, 12, 60, 24},
		[]any{"Work item", "Item name", "Description / location", "Method", "Nos", "Length", "Breadth", "Depth/Height", "Dia (mm)", "Quantity", "Calculation", "Remarks"})
	last, subtotal := "", 0.0
	flush := func() {
		if last != "" {
			add("", "", "Net quantity - "+last, "", "", "", "", "", "", calc.Round(subtotal, 3), calc.Item(last).Unit)
			add()
		}
	}
	for _, m := range d.Measurements {
		if m.WorkItem != last {
			flush()
			last, subtotal = m.WorkItem, 0
		}
		subtotal += m.Quantity
		add(m.WorkItem, calc.Item(m.WorkItem).Name, m.Description, m.Method, m.Nos, m.Length, m.Breadth, m.Depth, m.DiaMM, m.Quantity, m.Formula, m.Remarks)
	}
	flush()

	add = sheet("Rate Analysis", []float64{12, 36, 8, 10, 10, 12, 44, 40, 12, 14, 9},
		[]any{"Work item", "Resource", "Unit", "Qty per unit", "Rate", "Amount", "Basis of coefficient", "Price source", "Price date", "Price location", "Verified"})
	for _, b := range d.BuildUps {
		w := calc.Item(b.WorkItem)
		add(b.WorkItem, w.Name+" - per "+w.Unit)
		for _, l := range b.Lines {
			if l.Missing {
				add("", l.Name, l.Unit, l.Qty, "NO PRICE", "", l.Basis)
				continue
			}
			add("", l.Name, l.Unit, l.Qty, l.Rate, l.Amount, l.Basis, l.Source, l.Date, l.Location, l.Verified)
		}
		if b.Complete {
			add("", "Materials", "", "", "", b.Material)
			add("", "Labour", "", "", "", b.Labour)
			add("", fmt.Sprintf("Overhead and profit %s%%", num(b.OverheadPct)), "", "", "", b.Overhead)
			add("", "RATE per "+w.Unit, "", "", "", b.Rate, "", "", "", "", b.AllVerified)
		} else {
			add("", "Incomplete: one or more resources have no reference price")
		}
		add()
	}

	add = sheet("Materials", []float64{40, 12, 10, 14}, []any{"Resource", "Kind", "Unit", "Quantity"})
	for _, m := range sum.Materials {
		add(m.Name, m.Kind, m.Unit, m.Qty)
	}
	add()
	add(sum.MaterialsBasis)

	add = sheet("Findings", []float64{6, 10, 16, 50, 80, 60, 16, 50},
		[]any{"No", "Severity", "Check", "Finding", "Explanation", "Required action", "Value affected", "Reviewer's note"})
	for i, fd := range d.Findings {
		add(i+1, fd.Severity, fd.Code, fd.Title, fd.Explanation, fd.Action, fd.Impact, fd.AnalystNote)
	}

	add = sheet("Assumptions", []float64{14, 110}, []any{"Kind", "Statement"})
	for _, a := range d.Assumptions {
		add(a.Kind, a.Body)
	}
	return f.SaveAs(path)
}
