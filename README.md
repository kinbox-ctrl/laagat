# BuildCost Intelligence — V1

Independent construction cost assessment: a service business run on internal software.
V1 covers the two launch services from the product definition:

1. **Contractor Estimate & BOQ Audit** — check a contractor's quotation.
2. **Drawing-to-BOQ & Cost Estimate** — human-measured takeoff, BOQ and estimate.

## Run it

Needs Go 1.25+, Node 20+, PostgreSQL binaries and Python 3 on the PATH.

```sh
make db-up      # project-local Postgres on :5544 (data in .dev/)
make worker     # optional: Python PDF table extraction
make api        # Go API + job workers on :8080
make web        # Next.js on http://localhost:3000   (second terminal)
make test       # go vet + unit tests
```

Staff sign-in on first run: `analyst@buildcost.local` / `buildcost-dev`
(set `ADMIN_EMAIL` / `ADMIN_PASSWORD` before the first start anywhere but your laptop).
Customers register themselves. `samples/contractor-quotation.csv` is a test quotation.

## How a project flows

| Step | Who | Where |
|---|---|---|
| Submit project, upload documents | Customer | Portal |
| Send fee quotation, record payment | Staff | Overview tab |
| Extract BOQ rows from XLSX / CSV / PDF, correct and map them | Staff | Overview → BOQ tab |
| Enter the quantity takeoff (measurement sheet) | Staff | Quantity takeoff tab |
| Maintain dated, sourced reference prices | Staff | Rates & rules |
| Run analysis, confirm or dismiss every finding | Staff | Analysis & report tab |
| Approve (freezes a snapshot, renders PDF + Excel), deliver | Staff | Analysis & report tab |
| Download report, ask questions | Customer | Portal |

## Architecture

```
frontend/   Next.js  customer portal + analyst dashboard (proxies /api to the Go API)
backend/    Go modular monolith
  internal/calc    deterministic engine: units, measurement rules (IS 1200 openings,
                   steel d²/162), consumption coefficients, rate build-ups, materials
  internal/audit   validation engine: findings + assessed estimate (pure function)
  internal/app     HTTP API, auth, Postgres, job queue, ingestion, PDF/XLSX reports, AI
workers/    Python   PDF table extraction (pdfplumber), called by the Go job worker
```

Rules the code enforces:

- **Calculations are deterministic.** `calc` and `audit` have no database, network or AI access and are unit-tested.
- **AI only suggests.** It proposes work-item mappings and drafts notes; an analyst must accept each one. It never writes quantities, rates or report text.
- **Unverified prices never change an assessed amount.** They produce indicative notes only.
- **Nothing ships unreviewed.** Approval re-runs the analysis and refuses while any finding is open; reports render from the frozen snapshot.
- **Every quantity is traceable** to a measurement line with its formula; every reference rate to a dated, sourced price.

## Before this is used on a paying customer

- `backend/internal/calc/catalog.go` holds the measurement rules and consumption
  coefficients (rule set `2026.11-draft1`). They are textbook values and the labour
  constants in particular are rough. **A qualified quantity surveyor must review them.**
- The seeded prices are placeholders marked unverified. Enter real dated quotations per city.
- Payment is recorded manually from your gateway's reference; there is no gateway integration.
- Files are stored on local disk (`DATA_DIR`); move to object storage before deploying.

## Deliberately not in V1

Automatic drawing/CAD parsing (drawings are stored and measured by a person), DWG/image
extraction, cost monitoring (Service 4), a public rate-intelligence product (Service 3),
mobile app, marketplace, ERP, BIM.
