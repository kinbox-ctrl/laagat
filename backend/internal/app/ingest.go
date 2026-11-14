package app

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/xuri/excelize/v2"

	"buildcost/internal/calc"
)

// ParsedItem is one BOQ row extracted from a document. Extracted rows are
// always drafts: an analyst reviews and corrects them in the BOQ editor.
type ParsedItem struct {
	SlNo, Description, Unit string
	Qty, Rate, Amount       float64
}

var headerAliases = map[string][]string{
	"sl":     {"s no", "sno", "sl no", "slno", "sr no", "srno", "item no", "sl", "sr", "no", "#"},
	"desc":   {"description", "particular", "item of work", "work description", "name of work", "item", "work"},
	"unit":   {"unit", "uom"},
	"qty":    {"qty", "quantity"},
	"rate":   {"rate", "unit price", "unit rate", "price"},
	"amount": {"amount", "total", "value", "cost"},
}

// matchHeader maps a header cell ("Sr. No.", "Particulars", "Rate (Rs)") to
// a column key. Aliases of four or more letters match as prefixes; shorter
// ones must be followed by a non-letter so "no" does not match "nos".
func matchHeader(cell string) string {
	c := strings.ToLower(strings.ReplaceAll(cell, ".", " "))
	c = strings.Join(strings.Fields(c), " ")
	if c == "" {
		return ""
	}
	for _, key := range []string{"qty", "rate", "amount", "sl", "unit", "desc"} {
		for _, a := range headerAliases[key] {
			if !strings.HasPrefix(c, a) {
				continue
			}
			rest := c[len(a):]
			if rest == "" || len(a) >= 4 || rest[0] < 'a' || rest[0] > 'z' {
				return key
			}
		}
	}
	return ""
}

func parseNumber(s string) (float64, bool) {
	s = strings.NewReplacer(",", "", "₹", "", "Rs.", "", "Rs", "", "INR", "", "/-", "", " ", "").Replace(strings.TrimSpace(s))
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

// ParseTable finds the BOQ header row in a grid of cells and extracts the
// item rows beneath it. It also returns the stated grand total, if any.
func ParseTable(rows [][]string) (items []ParsedItem, declaredTotal float64, err error) {
	cols := map[string]int{}
	header := -1
	for i, row := range rows {
		found := map[string]int{}
		for j, cell := range row {
			if k := matchHeader(cell); k != "" {
				if _, dup := found[k]; !dup {
					found[k] = j
				}
			}
		}
		_, hasDesc := found["desc"]
		_, hasQty := found["qty"]
		_, hasAmt := found["amount"]
		_, hasRate := found["rate"]
		if hasDesc && (hasQty || hasAmt || hasRate) {
			cols, header = found, i
			break
		}
	}
	if header < 0 {
		return nil, 0, errors.New("no BOQ table found: expected a header row with a Description column and Quantity, Rate or Amount columns")
	}
	cell := func(row []string, key string) string {
		j, ok := cols[key]
		if !ok || j >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[j])
	}
	grandSeen := false
	for _, row := range rows[header+1:] {
		desc := cell(row, "desc")
		qty, hasQty := parseNumber(cell(row, "qty"))
		rate, _ := parseNumber(cell(row, "rate"))
		amount, hasAmt := parseNumber(cell(row, "amount"))
		lower := strings.ToLower(desc + " " + cell(row, "sl"))
		if strings.Contains(lower, "total") {
			// A "grand total" row wins; otherwise the last total row is used.
			if grand := strings.Contains(lower, "grand"); hasAmt && (grand || !grandSeen) {
				declaredTotal, grandSeen = amount, grand
			}
			continue
		}
		if desc == "" || (!hasQty && !hasAmt) {
			continue // section headings and blank rows
		}
		items = append(items, ParsedItem{SlNo: cell(row, "sl"), Description: desc, Unit: cell(row, "unit"), Qty: qty, Rate: rate, Amount: amount})
	}
	if len(items) == 0 {
		return nil, 0, errors.New("a BOQ header was found but no item rows with quantities or amounts")
	}
	return items, declaredTotal, nil
}

func readXLSX(path string) ([][]string, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open spreadsheet: %w", err)
	}
	defer f.Close()
	var firstErr error
	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetRows(sheet)
		if err != nil {
			continue
		}
		if _, _, perr := ParseTable(rows); perr == nil {
			return rows, nil
		} else if firstErr == nil {
			firstErr = perr
		}
	}
	if firstErr == nil {
		firstErr = errors.New("spreadsheet has no readable sheets")
	}
	return nil, firstErr
}

func readCSV(path string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	return r.ReadAll()
}

// readPDF runs the Python table-extraction worker. The worker prints a JSON
// array of rows (arrays of cell strings).
func (s *Server) readPDF(ctx context.Context, path string) ([][]string, error) {
	if len(s.PDFWorker) == 0 {
		return nil, errors.New("PDF table extraction worker is not installed; enter the BOQ manually or upload it as XLSX/CSV")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.PDFWorker[0], append(s.PDFWorker[1:], path)...)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("PDF extraction failed: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("PDF extraction failed: %w", err)
	}
	var rows [][]string
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, fmt.Errorf("PDF worker returned invalid output: %w", err)
	}
	return rows, nil
}

func (s *Server) importDocument(ctx context.Context, docID int64) (int, error) {
	d, err := queryOne[Document](ctx, s.DB, `SELECT * FROM documents WHERE id=$1`, docID)
	if err != nil {
		return 0, err
	}
	path := filepath.Join(s.DataDir, filepath.FromSlash(d.StorageKey))
	var rows [][]string
	switch strings.ToLower(filepath.Ext(d.Filename)) {
	case ".xlsx":
		rows, err = readXLSX(path)
	case ".csv":
		rows, err = readCSV(path)
	case ".pdf":
		rows, err = s.readPDF(ctx, path)
	default:
		err = errors.New("BOQ rows can be extracted from XLSX, CSV and text-based PDF files only; enter this document manually")
	}
	if err != nil {
		return 0, err
	}
	items, declared, err := ParseTable(rows)
	if err != nil {
		return 0, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	// Re-importing a document replaces the rows previously taken from it.
	if _, err := tx.Exec(ctx, `DELETE FROM boq_items WHERE document_id=$1`, d.ID); err != nil {
		return 0, err
	}
	for i, it := range items {
		code := calc.Classify(it.Description)
		src := "keyword"
		if code == "" {
			src = "none"
		}
		if it.Amount == 0 {
			it.Amount = calc.Round(it.Qty*it.Rate, 2)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO boq_items (project_id, document_id, sl_no, description, unit, quantity, rate, amount, work_item, mapping_source, sort)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, d.ProjectID, d.ID, it.SlNo, it.Description, it.Unit, it.Qty, it.Rate, it.Amount, code, src, i+1); err != nil {
			return 0, err
		}
	}
	if declared > 0 {
		if _, err := tx.Exec(ctx, `UPDATE projects SET quoted_total=$2 WHERE id=$1 AND quoted_total=0`, d.ProjectID, declared); err != nil {
			return 0, err
		}
	}
	return len(items), tx.Commit(ctx)
}

// RunWorker processes queued jobs until ctx is cancelled. Several workers may
// run concurrently: jobs are claimed with FOR UPDATE SKIP LOCKED.
func (s *Server) RunWorker(ctx context.Context) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		for s.runOneJob(ctx) {
		}
	}
}

func (s *Server) runOneJob(ctx context.Context) bool {
	var id int64
	var kind string
	var payload []byte
	err := s.DB.QueryRow(ctx, `UPDATE jobs SET status='running', attempts=attempts+1
		WHERE id = (SELECT id FROM jobs WHERE status='queued' ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1)
		RETURNING id, kind, payload`).Scan(&id, &kind, &payload)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) && ctx.Err() == nil {
			log.Printf("worker: claim job: %v", err)
		}
		return false
	}
	var jobErr error
	switch kind {
	case "import_boq":
		var p struct {
			DocumentID int64 `json:"document_id"`
		}
		_ = json.Unmarshal(payload, &p)
		n, err := s.importDocument(ctx, p.DocumentID)
		if err != nil {
			jobErr = err
			_, _ = s.DB.Exec(ctx, `UPDATE documents SET status='failed', note=$2 WHERE id=$1`, p.DocumentID, err.Error())
		} else {
			_, _ = s.DB.Exec(ctx, `UPDATE documents SET status='processed', note=$2 WHERE id=$1`, p.DocumentID,
				fmt.Sprintf("%d BOQ rows extracted as drafts; review them in the BOQ editor", n))
		}
	default:
		jobErr = fmt.Errorf("unknown job kind %q", kind)
	}
	status, msg := "done", ""
	if jobErr != nil {
		status, msg = "failed", jobErr.Error()
		log.Printf("worker: job %d (%s) failed: %v", id, kind, jobErr)
	}
	_, _ = s.DB.Exec(ctx, `UPDATE jobs SET status=$2, error=$3 WHERE id=$1`, id, status, msg)
	return true
}
