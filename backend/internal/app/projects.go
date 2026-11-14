package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"buildcost/internal/calc"
)

const GSTPct = 18.0

var documentKinds = map[string]bool{
	"drawing_architectural": true, "drawing_structural": true, "boq": true,
	"quotation": true, "specification": true, "other": true,
}

var allowedExt = map[string]bool{
	".pdf": true, ".dwg": true, ".dxf": true, ".jpg": true, ".jpeg": true, ".png": true,
	".xlsx": true, ".csv": true, ".docx": true,
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request, u *User) {
	var rows []Project
	var err error
	if u.Staff() {
		rows, err = query[Project](r.Context(), s.DB, projectSelect+` ORDER BY p.created_at DESC`)
	} else {
		rows, err = query[Project](r.Context(), s.DB, projectSelect+` WHERE p.customer_id = $1 ORDER BY p.created_at DESC`, u.ID)
	}
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, rows)
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request, u *User) {
	var in struct {
		Title        string  `json:"title"`
		City         string  `json:"city"`
		State        string  `json:"state"`
		BuildingType string  `json:"building_type"`
		AreaSqft     float64 `json:"built_up_area_sqft"`
		Floors       string  `json:"floors"`
		Service      string  `json:"service"`
		Notes        string  `json:"notes"`
		QuotedTotal  float64 `json:"quoted_total"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Title, in.City = strings.TrimSpace(in.Title), strings.TrimSpace(in.City)
	if in.Title == "" || in.City == "" || (in.Service != "audit" && in.Service != "estimate") {
		fail(w, 400, "title, city and a service (audit or estimate) are required")
		return
	}
	if in.AreaSqft < 0 || in.QuotedTotal < 0 {
		fail(w, 400, "area and quoted amount must not be negative")
		return
	}
	if in.BuildingType == "" {
		in.BuildingType = "Independent house"
	}
	if in.Floors == "" {
		in.Floors = "G+1"
	}
	var id int64
	err := s.DB.QueryRow(r.Context(), `INSERT INTO projects (customer_id, title, city, state, building_type, built_up_area_sqft, floors, service, notes, quoted_total)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
		u.ID, in.Title, in.City, strings.TrimSpace(in.State), in.BuildingType, in.AreaSqft, in.Floors, in.Service, in.Notes, in.QuotedTotal).Scan(&id)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"id": id})
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request, u *User, p *Project) {
	docs, err := query[Document](r.Context(), s.DB, `SELECT * FROM documents WHERE project_id = $1 ORDER BY created_at`, p.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"project": p, "documents": docs})
}

func (s *Server) uploadDocument(w http.ResponseWriter, r *http.Request, u *User, p *Project) {
	r.Body = http.MaxBytesReader(w, r.Body, 50<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		fail(w, 400, "upload too large (limit 50 MB) or malformed")
		return
	}
	kind := r.FormValue("kind")
	if !documentKinds[kind] {
		fail(w, 400, "unknown document kind")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		fail(w, 400, "file is required")
		return
	}
	defer file.Close()
	name := filepath.Base(header.Filename)
	ext := strings.ToLower(filepath.Ext(name))
	if !allowedExt[ext] {
		fail(w, 400, "unsupported file type; allowed: PDF, DWG, DXF, JPG, PNG, XLSX, CSV, DOCX")
		return
	}
	rnd := make([]byte, 12)
	_, _ = rand.Read(rnd)
	key := fmt.Sprintf("p%d/%s%s", p.ID, hex.EncodeToString(rnd), ext)
	dst := filepath.Join(s.DataDir, key)
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		serverError(w, err)
		return
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		serverError(w, err)
		return
	}
	size, err := io.Copy(out, file)
	out.Close()
	if err != nil {
		os.Remove(dst)
		serverError(w, err)
		return
	}
	var id int64
	err = s.DB.QueryRow(r.Context(), `INSERT INTO documents (project_id, kind, filename, storage_key, size, uploaded_by, revision)
		VALUES ($1,$2,$3,$4,$5,$6, 1 + (SELECT count(*) FROM documents WHERE project_id = $1 AND kind = $2 AND filename = $3)) RETURNING id`,
		p.ID, kind, name, key, size, u.ID).Scan(&id)
	if err != nil {
		os.Remove(dst)
		serverError(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"id": id})
}

func (s *Server) downloadDocument(w http.ResponseWriter, r *http.Request, u *User) {
	d, err := queryOne[Document](r.Context(), s.DB, `SELECT * FROM documents WHERE id = $1`, pathID(r))
	if err != nil {
		fail(w, 404, "not found")
		return
	}
	p, err := s.loadProject(r.Context(), d.ProjectID)
	if err != nil || (!u.Staff() && p.CustomerID != u.ID) {
		fail(w, 404, "not found")
		return
	}
	s.serveFile(w, r, d.StorageKey, d.Filename)
}

func (s *Server) serveFile(w http.ResponseWriter, r *http.Request, key, downloadName string) {
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", downloadName))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, filepath.Join(s.DataDir, filepath.FromSlash(key)))
}

func (s *Server) listMessages(w http.ResponseWriter, r *http.Request, u *User, p *Project) {
	rows, err := query[Message](r.Context(), s.DB, `SELECT m.id, m.user_id, u.name AS author, u.role, m.body, m.created_at
		FROM messages m JOIN users u ON u.id = m.user_id WHERE m.project_id = $1 ORDER BY m.created_at`, p.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, rows)
}

func (s *Server) postMessage(w http.ResponseWriter, r *http.Request, u *User, p *Project) {
	var in struct{ Body string }
	if !decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Body) == "" {
		fail(w, 400, "message is empty")
		return
	}
	if _, err := s.DB.Exec(r.Context(), `INSERT INTO messages (project_id, user_id, body) VALUES ($1,$2,$3)`, p.ID, u.ID, strings.TrimSpace(in.Body)); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"ok": true})
}

// ---- order: service quotation, acceptance, payment ----

func (s *Server) sendQuote(w http.ResponseWriter, r *http.Request, u *User, p *Project) {
	var in struct {
		Fee          float64 `json:"fee_amount"`
		DeliveryDays int     `json:"delivery_days"`
		Note         string  `json:"quote_note"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Fee <= 0 || in.DeliveryDays <= 0 {
		fail(w, 400, "fee and delivery days must be positive")
		return
	}
	if p.PaidAt != nil {
		fail(w, 409, "this order is already paid; the quotation cannot be changed")
		return
	}
	gst := calc.Round(in.Fee*GSTPct/100, 2)
	_, err := s.DB.Exec(r.Context(), `UPDATE projects SET fee_amount=$2, gst_amount=$3, delivery_days=$4, quote_note=$5,
		status = CASE WHEN status IN ('submitted','quoted','accepted') THEN 'quoted' ELSE status END WHERE id=$1`,
		p.ID, in.Fee, gst, in.DeliveryDays, in.Note)
	s.done(w, err)
}

func (s *Server) acceptQuote(w http.ResponseWriter, r *http.Request, u *User, p *Project) {
	if p.Status != "quoted" {
		fail(w, 409, "there is no open quotation to accept")
		return
	}
	_, err := s.DB.Exec(r.Context(), `UPDATE projects SET status='accepted' WHERE id=$1`, p.ID)
	s.done(w, err)
}

// markPaid records a payment received through the external payment provider.
func (s *Server) markPaid(w http.ResponseWriter, r *http.Request, u *User, p *Project) {
	var in struct {
		Ref string `json:"payment_ref"`
	}
	if !decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Ref) == "" || p.FeeAmount <= 0 {
		fail(w, 400, "a quotation and a payment reference are required")
		return
	}
	if p.PaidAt != nil {
		fail(w, 409, "payment is already recorded")
		return
	}
	invoice := fmt.Sprintf("BC/%s/%05d", time.Now().Format("2006"), p.ID)
	_, err := s.DB.Exec(r.Context(), `UPDATE projects SET payment_ref=$2, invoice_no=$3, paid_at=now(),
		status = CASE WHEN status IN ('submitted','quoted','accepted') THEN 'paid' ELSE status END WHERE id=$1`, p.ID, strings.TrimSpace(in.Ref), invoice)
	s.done(w, err)
}

func (s *Server) setStatus(w http.ResponseWriter, r *http.Request, u *User, p *Project) {
	var in struct{ Status string }
	if !decode(w, r, &in) {
		return
	}
	if in.Status != "in_analysis" && in.Status != "in_review" {
		fail(w, 400, "status must be in_analysis or in_review")
		return
	}
	if p.Status == "delivered" {
		fail(w, 409, "project is already delivered")
		return
	}
	_, err := s.DB.Exec(r.Context(), `UPDATE projects SET status=$2 WHERE id=$1`, p.ID, in.Status)
	s.done(w, err)
}

func (s *Server) setQuotedTotal(w http.ResponseWriter, r *http.Request, u *User, p *Project) {
	var in struct {
		QuotedTotal float64 `json:"quoted_total"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.QuotedTotal < 0 {
		fail(w, 400, "amount must not be negative")
		return
	}
	_, err := s.DB.Exec(r.Context(), `UPDATE projects SET quoted_total=$2 WHERE id=$1`, p.ID, in.QuotedTotal)
	s.done(w, err)
}

func (s *Server) done(w http.ResponseWriter, err error) {
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
