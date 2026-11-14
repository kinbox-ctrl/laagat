package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"buildcost/internal/audit"
	"buildcost/internal/calc"
)

// ---- BOQ ----

type boqInput struct {
	SlNo        string  `json:"sl_no"`
	Description string  `json:"description"`
	Unit        string  `json:"unit"`
	Quantity    float64 `json:"quantity"`
	Rate        float64 `json:"rate"`
	Amount      float64 `json:"amount"`
	WorkItem    *string `json:"work_item"` // nil: classify automatically
}

func (in *boqInput) normalize() (workItem, source string, err error) {
	in.Description = strings.TrimSpace(in.Description)
	if in.Description == "" {
		return "", "", fmt.Errorf("description is required")
	}
	if in.Quantity < 0 || in.Rate < 0 || in.Amount < 0 {
		return "", "", fmt.Errorf("quantity, rate and amount must not be negative")
	}
	if in.Amount == 0 {
		in.Amount = calc.Round(in.Quantity*in.Rate, 2)
	}
	if in.WorkItem == nil {
		if code := calc.Classify(in.Description); code != "" {
			return code, "keyword", nil
		}
		return "", "none", nil
	}
	if *in.WorkItem == "" {
		return "", "none", nil
	}
	if calc.Item(*in.WorkItem) == nil {
		return "", "", fmt.Errorf("unknown work item %q", *in.WorkItem)
	}
	return *in.WorkItem, "analyst", nil
}

func (s *Server) listBOQ(w http.ResponseWriter, r *http.Request, u *User, p *Project) {
	rows, err := s.boq(r.Context(), p.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, rows)
}

func (s *Server) boq(ctx context.Context, projectID int64) ([]BOQItem, error) {
	return query[BOQItem](ctx, s.DB, `SELECT * FROM boq_items WHERE project_id = $1 ORDER BY sort, id`, projectID)
}

func (s *Server) createBOQItem(w http.ResponseWriter, r *http.Request, u *User, p *Project) {
	var in boqInput
	if !decode(w, r, &in) {
		return
	}
	wi, src, err := in.normalize()
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	_, err = s.DB.Exec(r.Context(), `INSERT INTO boq_items (project_id, sl_no, description, unit, quantity, rate, amount, work_item, mapping_source, sort)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9, (SELECT coalesce(max(sort),0)+1 FROM boq_items WHERE project_id=$1))`,
		p.ID, in.SlNo, in.Description, in.Unit, in.Quantity, in.Rate, in.Amount, wi, src)
	s.done(w, err)
}

func (s *Server) updateBOQItem(w http.ResponseWriter, r *http.Request, u *User) {
	var in boqInput
	if !decode(w, r, &in) {
		return
	}
	wi, src, err := in.normalize()
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	_, err = s.DB.Exec(r.Context(), `UPDATE boq_items SET sl_no=$2, description=$3, unit=$4, quantity=$5, rate=$6, amount=$7, work_item=$8, mapping_source=$9, ai_suggestion='' WHERE id=$1`,
		pathID(r), in.SlNo, in.Description, in.Unit, in.Quantity, in.Rate, in.Amount, wi, src)
	s.done(w, err)
}

// importBOQ queues extraction of BOQ rows from an uploaded document.
func (s *Server) importBOQ(w http.ResponseWriter, r *http.Request, u *User, p *Project) {
	var in struct {
		DocumentID int64 `json:"document_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	tag, err := s.DB.Exec(r.Context(), `UPDATE documents SET status='queued', note='' WHERE id=$1 AND project_id=$2`, in.DocumentID, p.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 404, "document not found in this project")
		return
	}
	payload, _ := json.Marshal(map[string]int64{"document_id": in.DocumentID})
	_, err = s.DB.Exec(r.Context(), `INSERT INTO jobs (kind, payload) VALUES ('import_boq', $1)`, payload)
	s.done(w, err)
}

// boqFromTakeoff builds or refreshes estimate BOQ lines from the measurement
// sheet: one line per measured work item, priced from the reference build-up
// where every price in it is verified. Other lines get no rate (the analysis
// flags them) so unverified prices never reach an estimate unnoticed. Rates
// already set by an analyst are kept.
func (s *Server) boqFromTakeoff(w http.ResponseWriter, r *http.Request, u *User, p *Project) {
	ctx := r.Context()
	takeoff, err := s.takeoff(ctx, p.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	rates, err := s.ratesFor(ctx, p)
	if err != nil {
		serverError(w, err)
		return
	}
	existing, err := s.boq(ctx, p.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	byItem := map[string]BOQItem{}
	for _, b := range existing {
		if b.MappingSource == "takeoff" {
			byItem[b.WorkItem] = b
		}
	}
	n := 0
	for i, wi := range calc.Catalog {
		qty, ok := takeoff[wi.Code]
		if !ok {
			continue
		}
		rate := 0.0
		if prev, found := byItem[wi.Code]; found && prev.Rate > 0 {
			rate = prev.Rate
		} else if bu := calc.BuildRate(wi.Code, rates); bu.Supported && bu.Complete && bu.AllVerified {
			rate = bu.Rate
		}
		qty = calc.Round(qty, 2)
		amount := calc.Round(qty*rate, 2)
		if prev, found := byItem[wi.Code]; found {
			_, err = s.DB.Exec(ctx, `UPDATE boq_items SET quantity=$2, rate=$3, amount=$4 WHERE id=$1`, prev.ID, qty, rate, amount)
		} else {
			_, err = s.DB.Exec(ctx, `INSERT INTO boq_items (project_id, sl_no, description, unit, quantity, rate, amount, work_item, mapping_source, sort)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'takeoff',$9)`, p.ID, fmt.Sprint(n+1), wi.Name, wi.Unit, qty, rate, amount, wi.Code, 1000+i)
		}
		if err != nil {
			serverError(w, err)
			return
		}
		n++
	}
	writeJSON(w, 200, map[string]any{"items": n})
}

func (s *Server) suggestMappings(w http.ResponseWriter, r *http.Request, u *User, p *Project) {
	if s.AI == nil {
		fail(w, 503, "AI assistance is not configured (set ANTHROPIC_API_KEY)")
		return
	}
	items, err := query[BOQItem](r.Context(), s.DB, `SELECT * FROM boq_items WHERE project_id=$1 AND work_item='' ORDER BY sort, id`, p.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	if len(items) == 0 {
		writeJSON(w, 200, map[string]any{"suggested": 0})
		return
	}
	descs := make([]string, len(items))
	for i, it := range items {
		descs[i] = it.Description
	}
	codes, err := s.AI.SuggestWorkItems(r.Context(), descs)
	if err != nil {
		fail(w, 502, "AI request failed: "+err.Error())
		return
	}
	n := 0
	for i, code := range codes {
		if code == "" {
			continue
		}
		// Stored as a suggestion only: an analyst must accept it.
		if _, err := s.DB.Exec(r.Context(), `UPDATE boq_items SET ai_suggestion=$2 WHERE id=$1`, items[i].ID, code); err != nil {
			serverError(w, err)
			return
		}
		n++
	}
	writeJSON(w, 200, map[string]any{"suggested": n})
}

// ---- measurements (quantity takeoff) ----

func (s *Server) listMeasurements(w http.ResponseWriter, r *http.Request, u *User, p *Project) {
	rows, err := s.measurements(r.Context(), p.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, rows)
}

func (s *Server) measurements(ctx context.Context, projectID int64) ([]Measurement, error) {
	return query[Measurement](ctx, s.DB, `SELECT * FROM measurements WHERE project_id=$1 ORDER BY work_item, id`, projectID)
}

func (s *Server) takeoff(ctx context.Context, projectID int64) (map[string]float64, error) {
	rows, err := s.DB.Query(ctx, `SELECT work_item, sum(quantity) FROM measurements WHERE project_id=$1 GROUP BY work_item`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]float64{}
	for rows.Next() {
		var code string
		var q float64
		if err := rows.Scan(&code, &q); err != nil {
			return nil, err
		}
		out[code] = calc.Round(q, 3)
	}
	return out, rows.Err()
}

var openingItems = map[string]bool{"PL-INT": true, "PL-EXT": true, "BW-230": true, "BW-115": true}

func (s *Server) createMeasurement(w http.ResponseWriter, r *http.Request, u *User, p *Project) {
	var in Measurement
	if !decode(w, r, &in) {
		return
	}
	wi := calc.Item(in.WorkItem)
	if wi == nil || wi.Unit == calc.UnitLS {
		fail(w, 400, "choose a measurable work item")
		return
	}
	if in.Method == "" {
		in.Method = calc.MethodLBD
	}
	if in.Method == calc.MethodSteel && in.WorkItem != "STEEL" {
		fail(w, 400, "bar bending lines belong to the reinforcement steel item")
		return
	}
	if in.Method == calc.MethodOpening && !openingItems[in.WorkItem] {
		fail(w, 400, "opening deductions apply to masonry and plaster items")
		return
	}
	if strings.TrimSpace(in.Description) == "" {
		fail(w, 400, "description is required (e.g. location or drawing reference)")
		return
	}
	res, err := calc.ComputeMeasurement(calc.MeasurementInput{
		Method: in.Method, WorkItem: in.WorkItem, Nos: in.Nos, Length: in.Length, Breadth: in.Breadth,
		Depth: in.Depth, DiaMM: in.DiaMM, IsDeduction: in.IsDeduction,
	})
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	if in.Nos == 0 {
		in.Nos = 1
	}
	if in.DocumentID != nil {
		var ok bool
		_ = s.DB.QueryRow(r.Context(), `SELECT true FROM documents WHERE id=$1 AND project_id=$2`, *in.DocumentID, p.ID).Scan(&ok)
		if !ok {
			fail(w, 400, "source document does not belong to this project")
			return
		}
	}
	_, err = s.DB.Exec(r.Context(), `INSERT INTO measurements (project_id, work_item, description, method, nos, length, breadth, depth, dia_mm, is_deduction, quantity, formula, document_id, remarks, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		p.ID, in.WorkItem, strings.TrimSpace(in.Description), in.Method, in.Nos, in.Length, in.Breadth, in.Depth, in.DiaMM,
		in.IsDeduction, res.Quantity, res.Formula, in.DocumentID, in.Remarks, u.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"quantity": res.Quantity, "formula": res.Formula})
}

// ---- assumptions and exclusions ----

func (s *Server) listAssumptions(w http.ResponseWriter, r *http.Request, u *User, p *Project) {
	rows, err := query[Assumption](r.Context(), s.DB, `SELECT * FROM assumptions WHERE project_id=$1 ORDER BY kind, id`, p.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, rows)
}

func (s *Server) createAssumption(w http.ResponseWriter, r *http.Request, u *User, p *Project) {
	var in Assumption
	if !decode(w, r, &in) {
		return
	}
	if (in.Kind != "assumption" && in.Kind != "exclusion") || strings.TrimSpace(in.Body) == "" {
		fail(w, 400, "kind (assumption or exclusion) and text are required")
		return
	}
	_, err := s.DB.Exec(r.Context(), `INSERT INTO assumptions (project_id, kind, body) VALUES ($1,$2,$3)`, p.ID, in.Kind, strings.TrimSpace(in.Body))
	s.done(w, err)
}

// ---- rates ----

// ratesFor resolves the reference price of every resource for a project: the
// most recent rate for the project's city, else its state, else DEFAULT.
func (s *Server) ratesFor(ctx context.Context, p *Project) (map[string]calc.ResourceRate, error) {
	rows, err := query[Rate](ctx, s.DB, rateSelect+` WHERE lower(location) IN (lower($1), lower($2), lower($3))
		ORDER BY CASE WHEN lower(location)=lower($1) THEN 0 WHEN lower(location)=lower($2) THEN 1 ELSE 2 END, effective_date DESC, id DESC`,
		p.City, p.State, DefaultLocation)
	if err != nil {
		return nil, err
	}
	out := map[string]calc.ResourceRate{}
	for _, r := range rows {
		if _, seen := out[r.Resource]; !seen {
			out[r.Resource] = calc.ResourceRate{Resource: r.Resource, Rate: r.Rate, Location: r.Location, EffectiveDate: r.EffectiveDate, Source: r.Source, Verified: r.Verified}
		}
	}
	return out, nil
}

func (s *Server) listRates(w http.ResponseWriter, r *http.Request, u *User) {
	rows, err := query[Rate](r.Context(), s.DB, rateSelect+` ORDER BY resource, location, effective_date DESC`)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, rows)
}

func (s *Server) createRate(w http.ResponseWriter, r *http.Request, u *User) {
	var in Rate
	if !decode(w, r, &in) {
		return
	}
	known := false
	for _, res := range calc.Resources {
		known = known || res.Code == in.Resource
	}
	in.Location, in.Source = strings.TrimSpace(in.Location), strings.TrimSpace(in.Source)
	if _, err := time.Parse("2006-01-02", in.EffectiveDate); err != nil || !known || in.Rate <= 0 || in.Location == "" || in.Source == "" {
		fail(w, 400, "resource, location, a positive rate, an effective date (YYYY-MM-DD) and a source are required")
		return
	}
	_, err := s.DB.Exec(r.Context(), `INSERT INTO rates (resource, location, rate, effective_date, source, spec, incl_tax, verified) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		in.Resource, in.Location, in.Rate, in.EffectiveDate, in.Source, in.Spec, in.InclTax, in.Verified)
	s.done(w, err)
}

func (s *Server) buildUps(rates map[string]calc.ResourceRate) []calc.RateBuildUp {
	var out []calc.RateBuildUp
	for _, wi := range calc.Catalog {
		if len(wi.Coefficients) > 0 {
			out = append(out, calc.BuildRate(wi.Code, rates))
		}
	}
	return out
}

func (s *Server) rateAnalysis(w http.ResponseWriter, r *http.Request, u *User, p *Project) {
	rates, err := s.ratesFor(r.Context(), p)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"build_ups": s.buildUps(rates), "rates": rates})
}

func (s *Server) catalog(w http.ResponseWriter, r *http.Request, u *User) {
	writeJSON(w, 200, map[string]any{
		"rule_version": calc.RuleVersion, "overhead_profit_pct": calc.OverheadProfitPct,
		"items": calc.Catalog, "resources": calc.Resources,
	})
}

// ---- analysis ----

// runAnalysis executes the deterministic audit on the project's current data,
// stores the run and reconciles the findings table. Analyst decisions on a
// finding survive a re-run unless the finding's content changed.
func (s *Server) runAnalysis(ctx context.Context, p *Project, userID int64) (*audit.Result, error) {
	items, err := s.boq(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	takeoff, err := s.takeoff(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	rates, err := s.ratesFor(ctx, p)
	if err != nil {
		return nil, err
	}
	in := audit.Input{Service: p.Service, DeclaredTotal: p.QuotedTotal, Takeoff: takeoff, Rates: rates, AsOf: time.Now()}
	for _, b := range items {
		in.Lines = append(in.Lines, audit.BOQLine{ID: b.ID, SlNo: b.SlNo, Description: b.Description, Unit: b.Unit, Qty: b.Quantity, Rate: b.Rate, Amount: b.Amount, WorkItem: b.WorkItem})
	}
	res := audit.Run(in)

	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	blob, _ := json.Marshal(res)
	if _, err := tx.Exec(ctx, `INSERT INTO analysis_runs (project_id, result, created_by) VALUES ($1,$2,$3)`, p.ID, blob, userID); err != nil {
		return nil, err
	}
	prints := []string{}
	for _, f := range res.Findings {
		ev, _ := json.Marshal(f.Evidence)
		prints = append(prints, f.Fingerprint)
		_, err := tx.Exec(ctx, `INSERT INTO findings (project_id, fingerprint, code, severity, title, explanation, required_action, evidence, amount_impact, boq_item_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			ON CONFLICT (project_id, fingerprint) DO UPDATE SET code=EXCLUDED.code, severity=EXCLUDED.severity, title=EXCLUDED.title,
			  explanation=EXCLUDED.explanation, required_action=EXCLUDED.required_action, evidence=EXCLUDED.evidence,
			  amount_impact=EXCLUDED.amount_impact, boq_item_id=EXCLUDED.boq_item_id,
			  status = CASE WHEN findings.title=EXCLUDED.title AND findings.explanation=EXCLUDED.explanation THEN findings.status ELSE 'open' END`,
			p.ID, f.Fingerprint, f.Code, f.Severity, f.Title, f.Explanation, f.Action, ev, f.Impact, f.BOQItemID)
		if err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM findings WHERE project_id=$1 AND NOT (fingerprint = ANY($2))`, p.ID, prints); err != nil {
		return nil, err
	}
	return &res, tx.Commit(ctx)
}

func (s *Server) findings(ctx context.Context, projectID int64) ([]FindingRow, error) {
	return query[FindingRow](ctx, s.DB, `SELECT * FROM findings WHERE project_id=$1
		ORDER BY CASE severity WHEN 'major' THEN 0 WHEN 'review' THEN 1 ELSE 2 END, id`, projectID)
}

func (s *Server) analyze(w http.ResponseWriter, r *http.Request, u *User, p *Project) {
	if _, err := s.runAnalysis(r.Context(), p, u.ID); err != nil {
		serverError(w, err)
		return
	}
	s.getAnalysis(w, r, u, p)
}

func (s *Server) getAnalysis(w http.ResponseWriter, r *http.Request, u *User, p *Project) {
	var blob json.RawMessage
	var at time.Time
	err := s.DB.QueryRow(r.Context(), `SELECT result, created_at FROM analysis_runs WHERE project_id=$1 ORDER BY id DESC LIMIT 1`, p.ID).Scan(&blob, &at)
	if err == pgx.ErrNoRows {
		writeJSON(w, 200, map[string]any{"run": nil, "findings": []FindingRow{}})
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	rows, err := s.findings(r.Context(), p.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"run": map[string]any{"created_at": at, "result": blob}, "findings": rows})
}

func (s *Server) updateFinding(w http.ResponseWriter, r *http.Request, u *User) {
	var in struct {
		Status string `json:"status"`
		Note   string `json:"analyst_note"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Status != "open" && in.Status != "confirmed" && in.Status != "dismissed" {
		fail(w, 400, "status must be open, confirmed or dismissed")
		return
	}
	if in.Status == "dismissed" && strings.TrimSpace(in.Note) == "" {
		fail(w, 400, "record the reason for dismissing a finding in the note")
		return
	}
	_, err := s.DB.Exec(r.Context(), `UPDATE findings SET status=$2, analyst_note=$3, reviewed_by=$4 WHERE id=$1`, pathID(r), in.Status, strings.TrimSpace(in.Note), u.ID)
	s.done(w, err)
}

// draftFindingNote asks the AI for a plain-language draft of a finding. The
// draft is stored separately and only reaches a report if an analyst copies
// it into the reviewer's note.
func (s *Server) draftFindingNote(w http.ResponseWriter, r *http.Request, u *User) {
	if s.AI == nil {
		fail(w, 503, "AI assistance is not configured (set ANTHROPIC_API_KEY)")
		return
	}
	f, err := queryOne[FindingRow](r.Context(), s.DB, `SELECT * FROM findings WHERE id=$1`, pathID(r))
	if err != nil {
		serverError(w, err)
		return
	}
	draft, err := s.AI.DraftFindingNote(r.Context(), f)
	if err != nil {
		fail(w, 502, "AI request failed: "+err.Error())
		return
	}
	if _, err := s.DB.Exec(r.Context(), `UPDATE findings SET ai_draft=$2 WHERE id=$1`, f.ID, draft); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ai_draft": draft})
}
