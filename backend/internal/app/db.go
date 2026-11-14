package app

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const schema = `
CREATE TABLE IF NOT EXISTS users (
  id bigserial PRIMARY KEY,
  email text NOT NULL UNIQUE,
  name text NOT NULL,
  password_hash text NOT NULL,
  role text NOT NULL DEFAULT 'customer' CHECK (role IN ('customer','analyst','admin')),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS sessions (
  token text PRIMARY KEY,
  user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at timestamptz NOT NULL
);
CREATE TABLE IF NOT EXISTS projects (
  id bigserial PRIMARY KEY,
  customer_id bigint NOT NULL REFERENCES users(id),
  title text NOT NULL,
  city text NOT NULL,
  state text NOT NULL DEFAULT '',
  building_type text NOT NULL DEFAULT 'Independent house',
  built_up_area_sqft double precision NOT NULL DEFAULT 0,
  floors text NOT NULL DEFAULT 'G+1',
  service text NOT NULL CHECK (service IN ('audit','estimate')),
  status text NOT NULL DEFAULT 'submitted',
  notes text NOT NULL DEFAULT '',
  quoted_total double precision NOT NULL DEFAULT 0,
  fee_amount double precision NOT NULL DEFAULT 0,
  gst_amount double precision NOT NULL DEFAULT 0,
  delivery_days int NOT NULL DEFAULT 0,
  quote_note text NOT NULL DEFAULT '',
  payment_ref text NOT NULL DEFAULT '',
  invoice_no text NOT NULL DEFAULT '',
  paid_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS documents (
  id bigserial PRIMARY KEY,
  project_id bigint NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  kind text NOT NULL,
  filename text NOT NULL,
  storage_key text NOT NULL,
  size bigint NOT NULL,
  revision int NOT NULL DEFAULT 1,
  uploaded_by bigint NOT NULL REFERENCES users(id),
  status text NOT NULL DEFAULT 'stored',
  note text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS boq_items (
  id bigserial PRIMARY KEY,
  project_id bigint NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  document_id bigint REFERENCES documents(id) ON DELETE SET NULL,
  sl_no text NOT NULL DEFAULT '',
  description text NOT NULL,
  unit text NOT NULL DEFAULT '',
  quantity double precision NOT NULL DEFAULT 0,
  rate double precision NOT NULL DEFAULT 0,
  amount double precision NOT NULL DEFAULT 0,
  work_item text NOT NULL DEFAULT '',
  mapping_source text NOT NULL DEFAULT 'none',
  ai_suggestion text NOT NULL DEFAULT '',
  sort int NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS measurements (
  id bigserial PRIMARY KEY,
  project_id bigint NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  work_item text NOT NULL,
  description text NOT NULL,
  method text NOT NULL DEFAULT 'lbd',
  nos double precision NOT NULL DEFAULT 1,
  length double precision NOT NULL DEFAULT 0,
  breadth double precision NOT NULL DEFAULT 0,
  depth double precision NOT NULL DEFAULT 0,
  dia_mm double precision NOT NULL DEFAULT 0,
  is_deduction boolean NOT NULL DEFAULT false,
  quantity double precision NOT NULL,
  formula text NOT NULL,
  document_id bigint REFERENCES documents(id) ON DELETE SET NULL,
  remarks text NOT NULL DEFAULT '',
  created_by bigint NOT NULL REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS assumptions (
  id bigserial PRIMARY KEY,
  project_id bigint NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  kind text NOT NULL CHECK (kind IN ('assumption','exclusion')),
  body text NOT NULL
);
CREATE TABLE IF NOT EXISTS analysis_runs (
  id bigserial PRIMARY KEY,
  project_id bigint NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  result jsonb NOT NULL,
  created_by bigint NOT NULL REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS findings (
  id bigserial PRIMARY KEY,
  project_id bigint NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  fingerprint text NOT NULL,
  code text NOT NULL,
  severity text NOT NULL,
  title text NOT NULL,
  explanation text NOT NULL,
  required_action text NOT NULL,
  evidence jsonb NOT NULL DEFAULT '{}',
  amount_impact double precision NOT NULL DEFAULT 0,
  boq_item_id bigint,
  status text NOT NULL DEFAULT 'open' CHECK (status IN ('open','confirmed','dismissed')),
  analyst_note text NOT NULL DEFAULT '',
  ai_draft text NOT NULL DEFAULT '',
  reviewed_by bigint REFERENCES users(id),
  UNIQUE (project_id, fingerprint)
);
CREATE TABLE IF NOT EXISTS reports (
  id bigserial PRIMARY KEY,
  project_id bigint NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  version int NOT NULL,
  approved_by bigint NOT NULL REFERENCES users(id),
  pdf_key text NOT NULL,
  xlsx_key text NOT NULL,
  snapshot jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS messages (
  id bigserial PRIMARY KEY,
  project_id bigint NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  user_id bigint NOT NULL REFERENCES users(id),
  body text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS rates (
  id bigserial PRIMARY KEY,
  resource text NOT NULL,
  location text NOT NULL,
  rate double precision NOT NULL,
  effective_date date NOT NULL,
  source text NOT NULL,
  spec text NOT NULL DEFAULT '',
  incl_tax boolean NOT NULL DEFAULT true,
  verified boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS jobs (
  id bigserial PRIMARY KEY,
  kind text NOT NULL,
  payload jsonb NOT NULL,
  status text NOT NULL DEFAULT 'queued',
  attempts int NOT NULL DEFAULT 0,
  error text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS boq_items_project ON boq_items(project_id);
CREATE INDEX IF NOT EXISTS measurements_project ON measurements(project_id);
CREATE INDEX IF NOT EXISTS rates_lookup ON rates(resource, location, effective_date DESC);
`

// DefaultLocation is the fallback location for reference rates.
const DefaultLocation = "DEFAULT"

const placeholderSource = "ILLUSTRATIVE PLACEHOLDER - not a market quotation; replace with a dated local quote"

// Placeholder prices so that rate build-ups can be exercised before real
// quotations are collected. They are stored unverified and are never applied
// to an assessed estimate.
var placeholderRates = map[string]float64{
	"CEMENT": 400, "SAND": 2000, "AGG20": 1600, "AGG40": 1500, "BRICK": 9, "STEEL": 62,
	"BWIRE": 85, "TILE-VT": 650, "MASON": 900, "HELPER": 600, "BARBENDER": 900,
}

func OpenDB(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if _, err := pool.Exec(ctx, schema); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return pool, nil
}

// Seed inserts placeholder rates and the first staff account when missing.
func Seed(ctx context.Context, db *pgxpool.Pool, adminEmail, adminPassword string) error {
	var n int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM rates`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		today := time.Now().Format("2006-01-02")
		for res, rate := range placeholderRates {
			if _, err := db.Exec(ctx, `INSERT INTO rates (resource, location, rate, effective_date, source, verified) VALUES ($1,$2,$3,$4,$5,false)`,
				res, DefaultLocation, rate, today, placeholderSource); err != nil {
				return err
			}
		}
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM users WHERE role <> 'customer'`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		hash, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		_, err = db.Exec(ctx, `INSERT INTO users (email, name, password_hash, role) VALUES ($1,'BuildCost Analyst',$2,'admin') ON CONFLICT (email) DO NOTHING`, adminEmail, string(hash))
		return err
	}
	return nil
}

// query runs sql and scans every row into T by column name (db tags).
func query[T any](ctx context.Context, db *pgxpool.Pool, sql string, args ...any) ([]T, error) {
	rows, err := db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[T])
	if out == nil {
		out = []T{}
	}
	return out, err
}

func queryOne[T any](ctx context.Context, db *pgxpool.Pool, sql string, args ...any) (*T, error) {
	rows, err := query[T](ctx, db, sql, args...)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, pgx.ErrNoRows
	}
	return &rows[0], nil
}
