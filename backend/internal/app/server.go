package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type Server struct {
	DB      *pgxpool.Pool
	DataDir string // object storage root (local filesystem in V1)
	AI      *AI    // nil when no API credentials are configured
	// PDFWorker is the command that extracts tables from a PDF and prints
	// them as JSON, e.g. ["workers/.venv/bin/python", "workers/extract_tables.py"].
	PDFWorker []string
}

const sessionCookie = "bc_session"

type authed func(w http.ResponseWriter, r *http.Request, u *User)
type scoped func(w http.ResponseWriter, r *http.Request, u *User, p *Project)

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/register", s.register)
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.HandleFunc("GET /api/me", s.auth(func(w http.ResponseWriter, r *http.Request, u *User) {
		writeJSON(w, 200, map[string]any{"user": u, "ai_enabled": s.AI != nil})
	}))

	mux.HandleFunc("GET /api/projects", s.auth(s.listProjects))
	mux.HandleFunc("POST /api/projects", s.auth(s.createProject))
	mux.HandleFunc("GET /api/projects/{id}", s.project(false, s.getProject))
	mux.HandleFunc("POST /api/projects/{id}/documents", s.project(false, s.uploadDocument))
	mux.HandleFunc("GET /api/documents/{id}/download", s.auth(s.downloadDocument))
	mux.HandleFunc("GET /api/projects/{id}/messages", s.project(false, s.listMessages))
	mux.HandleFunc("POST /api/projects/{id}/messages", s.project(false, s.postMessage))
	mux.HandleFunc("POST /api/projects/{id}/accept", s.project(false, s.acceptQuote))
	mux.HandleFunc("GET /api/projects/{id}/reports", s.project(false, s.listReports))
	mux.HandleFunc("GET /api/reports/{id}/{format}", s.auth(s.downloadReport))

	// Internal (staff) workspace.
	mux.HandleFunc("POST /api/projects/{id}/quote", s.project(true, s.sendQuote))
	mux.HandleFunc("POST /api/projects/{id}/paid", s.project(true, s.markPaid))
	mux.HandleFunc("POST /api/projects/{id}/status", s.project(true, s.setStatus))
	mux.HandleFunc("PUT /api/projects/{id}/quoted-total", s.project(true, s.setQuotedTotal))
	mux.HandleFunc("GET /api/projects/{id}/boq", s.project(true, s.listBOQ))
	mux.HandleFunc("POST /api/projects/{id}/boq", s.project(true, s.createBOQItem))
	mux.HandleFunc("POST /api/projects/{id}/boq/import", s.project(true, s.importBOQ))
	mux.HandleFunc("POST /api/projects/{id}/boq/from-takeoff", s.project(true, s.boqFromTakeoff))
	mux.HandleFunc("POST /api/projects/{id}/boq/suggest", s.project(true, s.suggestMappings))
	mux.HandleFunc("PUT /api/boq/{id}", s.staff(s.updateBOQItem))
	mux.HandleFunc("DELETE /api/boq/{id}", s.staff(s.deleteRow("boq_items")))
	mux.HandleFunc("GET /api/projects/{id}/measurements", s.project(true, s.listMeasurements))
	mux.HandleFunc("POST /api/projects/{id}/measurements", s.project(true, s.createMeasurement))
	mux.HandleFunc("DELETE /api/measurements/{id}", s.staff(s.deleteRow("measurements")))
	mux.HandleFunc("GET /api/projects/{id}/assumptions", s.project(true, s.listAssumptions))
	mux.HandleFunc("POST /api/projects/{id}/assumptions", s.project(true, s.createAssumption))
	mux.HandleFunc("DELETE /api/assumptions/{id}", s.staff(s.deleteRow("assumptions")))
	mux.HandleFunc("POST /api/projects/{id}/analyze", s.project(true, s.analyze))
	mux.HandleFunc("GET /api/projects/{id}/analysis", s.project(true, s.getAnalysis))
	mux.HandleFunc("GET /api/projects/{id}/rate-analysis", s.project(true, s.rateAnalysis))
	mux.HandleFunc("PUT /api/findings/{id}", s.staff(s.updateFinding))
	mux.HandleFunc("POST /api/findings/{id}/draft", s.staff(s.draftFindingNote))
	mux.HandleFunc("POST /api/projects/{id}/approve", s.project(true, s.approve))
	mux.HandleFunc("POST /api/projects/{id}/deliver", s.project(true, s.deliver))
	mux.HandleFunc("GET /api/catalog", s.staff(s.catalog))
	mux.HandleFunc("GET /api/rates", s.staff(s.listRates))
	mux.HandleFunc("POST /api/rates", s.staff(s.createRate))
	mux.HandleFunc("DELETE /api/rates/{id}", s.staff(s.deleteRow("rates")))
	return logRequests(mux)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// serverError logs the cause and returns a generic message to the client.
func serverError(w http.ResponseWriter, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "not found")
		return
	}
	log.Printf("ERROR: %v", err)
	fail(w, 500, "internal error")
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		fail(w, 400, "invalid JSON body")
		return false
	}
	return true
}

func pathID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id
}

// ---- auth ----

func (s *Server) currentUser(r *http.Request) *User {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil
	}
	u, err := queryOne[User](r.Context(), s.DB,
		`SELECT u.* FROM users u JOIN sessions s ON s.user_id = u.id WHERE s.token = $1 AND s.expires_at > now()`, c.Value)
	if err != nil {
		return nil
	}
	return u
}

func (s *Server) auth(h authed) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := s.currentUser(r)
		if u == nil {
			fail(w, 401, "sign in required")
			return
		}
		h(w, r, u)
	}
}

func (s *Server) staff(h authed) http.HandlerFunc {
	return s.auth(func(w http.ResponseWriter, r *http.Request, u *User) {
		if !u.Staff() {
			fail(w, 403, "staff only")
			return
		}
		h(w, r, u)
	})
}

// project loads the project in the path and checks the caller may access it:
// staff may access any project, customers only their own.
func (s *Server) project(staffOnly bool, h scoped) http.HandlerFunc {
	return s.auth(func(w http.ResponseWriter, r *http.Request, u *User) {
		if staffOnly && !u.Staff() {
			fail(w, 403, "staff only")
			return
		}
		p, err := s.loadProject(r.Context(), pathID(r))
		if err != nil || (!u.Staff() && p.CustomerID != u.ID) {
			fail(w, 404, "project not found")
			return
		}
		h(w, r, u, p)
	})
}

func (s *Server) loadProject(ctx context.Context, id int64) (*Project, error) {
	return queryOne[Project](ctx, s.DB, projectSelect+` WHERE p.id = $1`, id)
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, userID int64) error {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	token := hex.EncodeToString(b)
	exp := time.Now().Add(14 * 24 * time.Hour)
	if _, err := s.DB.Exec(r.Context(), `INSERT INTO sessions (token, user_id, expires_at) VALUES ($1,$2,$3)`, token, userID, exp); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", Expires: exp, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"})
	return nil
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Name, Password string }
	if !decode(w, r, &in) {
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.Name = strings.TrimSpace(in.Name)
	if !strings.Contains(in.Email, "@") || in.Name == "" || len(in.Password) < 8 {
		fail(w, 400, "name, a valid email and a password of at least 8 characters are required")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		serverError(w, err)
		return
	}
	var id int64
	err = s.DB.QueryRow(r.Context(), `INSERT INTO users (email, name, password_hash) VALUES ($1,$2,$3) ON CONFLICT (email) DO NOTHING RETURNING id`, in.Email, in.Name, string(hash)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 409, "an account with this email already exists")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	if err := s.startSession(w, r, id); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"ok": true})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Password string }
	if !decode(w, r, &in) {
		return
	}
	u, err := queryOne[User](r.Context(), s.DB, `SELECT * FROM users WHERE email = $1`, strings.ToLower(strings.TrimSpace(in.Email)))
	if err != nil || bcrypt.CompareHashAndPassword([]byte(u.Hash), []byte(in.Password)) != nil {
		fail(w, 401, "incorrect email or password")
		return
	}
	if err := s.startSession(w, r, u.ID); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"user": u})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_, _ = s.DB.Exec(r.Context(), `DELETE FROM sessions WHERE token = $1`, c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, 200, map[string]any{"ok": true})
}

// deleteRow deletes a staff-managed row by id from a fixed table name.
func (s *Server) deleteRow(table string) authed {
	return func(w http.ResponseWriter, r *http.Request, u *User) {
		if _, err := s.DB.Exec(r.Context(), `DELETE FROM `+table+` WHERE id = $1`, pathID(r)); err != nil {
			serverError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	}
}
