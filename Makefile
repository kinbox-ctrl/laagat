# BuildCost V1 - local development
PG_PORT ?= 5544
PGDATA  := .dev/pgdata

.PHONY: db-up db-down worker api web test

db-up: ## start the project-local Postgres (created on first run)
	@test -d $(PGDATA) || (mkdir -p .dev && initdb -D $(PGDATA) -A trust -U buildcost >/dev/null)
	@pg_ctl -D $(PGDATA) status >/dev/null 2>&1 || pg_ctl -D $(PGDATA) -o "-p $(PG_PORT) -k /tmp" -l .dev/pg.log start
	@psql -h localhost -p $(PG_PORT) -U buildcost -d postgres -Atc "SELECT 1 FROM pg_database WHERE datname='buildcost'" | grep -q 1 \
		|| createdb -h localhost -p $(PG_PORT) -U buildcost buildcost

db-down:
	pg_ctl -D $(PGDATA) stop

worker: ## install the Python PDF table-extraction worker
	python3 -m venv workers/.venv && workers/.venv/bin/pip install -q -r workers/requirements.txt

api: ## run the Go API and job workers on :8080
	cd backend && go run ./cmd/server

web: ## run the Next.js portal and dashboard on :3000
	cd frontend && npm install && npm run dev

test:
	cd backend && go vet ./... && go test ./...
