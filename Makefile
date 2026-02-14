# pro-rag Makefile — CLI-first, Makefile-first (DEVELOPMENT_RULES.md §0)
.PHONY: help db-up db-down db-migrate db-reset db-seed db-psql \
        api-build api-run api-test \
        ingest-run ingest-test ingest-corpus ingest-corpus-all generate-corpus generate-corpus-expanded \
        test eval eval-retrieval eval-full e2e-smoke redteam \
        update-rules validate-rules

# Default: show help
help:
	@echo "pro-rag Make targets:"
	@echo ""
	@echo "  Database:"
	@echo "    db-up          Start Postgres (pgvector)"
	@echo "    db-down        Stop all services + remove containers"
	@echo "    db-migrate     Run SQL migrations"
	@echo "    db-reset       Destroy DB volume + recreate + migrate"
	@echo "    db-seed        Seed test tenant"
	@echo "    db-psql        Open psql shell"
	@echo ""
	@echo "  Query API (Go):"
	@echo "    api-build      Build core-api-go"
	@echo "    api-run        Start core-api-go via Docker Compose"
	@echo "    api-test       Run Go tests"
	@echo ""
	@echo "  Ingestion (Python):"
	@echo "    ingest-run     Run ingestion pipeline"
	@echo "    ingest-test    Run Python tests"
	@echo "    ingest-corpus      Ingest 5 original test corpus docs"
	@echo "    ingest-corpus-all  Ingest all 15 expanded corpus docs"
	@echo "    generate-corpus          Generate 5 original DOCX files"
	@echo "    generate-corpus-expanded Generate 10 additional docs (DOCX/HTML/PDF)"
	@echo ""
	@echo "  All:"
	@echo "    test           Run all tests (api-test + ingest-test)"
	@echo "    eval           Run retrieval-only evaluation (default)"
	@echo "    eval-retrieval Run retrieval-only evaluation (DB direct)"
	@echo "    eval-full      Run full pipeline evaluation (calls API)"
	@echo "    e2e-smoke      End-to-end smoke test"
	@echo "    redteam        Run red team probes (injection/exfil/stale)"
	@echo ""
	@echo "  Meta:"
	@echo "    update-rules   Capture a learning (MSG=...)"
	@echo "    validate-rules Check project consistency"

# ── Database ──────────────────────────────────────────────

db-up:
	docker compose up -d postgres
	@echo "==> Postgres is starting. Use 'make db-migrate' to apply migrations."

db-down:
	docker compose down

db-migrate:
	docker compose run --rm migrate

db-reset:
	docker compose down -v
	docker compose up -d postgres
	@echo "==> Waiting for Postgres to be healthy..."
	@sleep 3
	docker compose run --rm migrate
	@echo "==> DB reset complete."

db-seed:
	docker compose exec postgres psql -U $${POSTGRES_USER:-prorag} -d $${POSTGRES_DB:-prorag} -f /dev/stdin < migrations/seed.sql
	@echo "==> Seed data applied."

db-psql:
	docker compose exec postgres psql -U $${POSTGRES_USER:-prorag} -d $${POSTGRES_DB:-prorag}

# ── Query API (Go) ───────────────────────────────────────

api-build:
	cd core-api-go && go build -o bin/server ./cmd/server

api-run:
	docker compose up core-api-go

api-test:
	cd core-api-go && go test ./...

# ── Ingestion (Python) ──────────────────────────────────

ingest-run:
	docker compose run --rm ingest

ingest-test:
	cd ingest && .venv/bin/python -m pytest tests/ -v

TENANT_ID ?= 00000000-0000-0000-0000-000000000001
CORPUS_DIR ?= data/test-corpus

generate-corpus:
	cd ingest && .venv/bin/python ../scripts/generate_test_corpus.py
	@echo "==> Test corpus generated in $(CORPUS_DIR)/"

generate-corpus-expanded:
	cd ingest && .venv/bin/python ../scripts/generate_expanded_corpus.py
	@echo "==> Expanded corpus generated in $(CORPUS_DIR)/"

ingest-corpus:
	@echo "==> Ingesting 5 original test corpus docs from $(CORPUS_DIR)/ for tenant $(TENANT_ID)..."
	cd ingest && .venv/bin/python -m ingest.cli ingest ../$(CORPUS_DIR)/it_security_policy.docx \
		--tenant-id $(TENANT_ID) --title "Acme Corp IT Security Policy"
	cd ingest && .venv/bin/python -m ingest.cli ingest ../$(CORPUS_DIR)/employee_onboarding_guide.docx \
		--tenant-id $(TENANT_ID) --title "Acme Corp Employee Onboarding Guide"
	cd ingest && .venv/bin/python -m ingest.cli ingest ../$(CORPUS_DIR)/expense_reimbursement_policy.docx \
		--tenant-id $(TENANT_ID) --title "Acme Corp Expense Reimbursement Policy"
	cd ingest && .venv/bin/python -m ingest.cli ingest ../$(CORPUS_DIR)/remote_work_policy.docx \
		--tenant-id $(TENANT_ID) --title "Acme Corp Remote Work Policy"
	cd ingest && .venv/bin/python -m ingest.cli ingest ../$(CORPUS_DIR)/incident_response_procedure.docx \
		--tenant-id $(TENANT_ID) --title "Acme Corp Incident Response Procedure"
	@echo "==> All 5 original documents ingested."

ingest-corpus-all: ingest-corpus
	@echo "==> Ingesting 10 expanded corpus docs from $(CORPUS_DIR)/ for tenant $(TENANT_ID)..."
	cd ingest && .venv/bin/python -m ingest.cli ingest ../$(CORPUS_DIR)/leave_benefits_summary.docx \
		--tenant-id $(TENANT_ID) --title "Acme Corp Annual Leave & Benefits Summary"
	cd ingest && .venv/bin/python -m ingest.cli ingest ../$(CORPUS_DIR)/software_development_lifecycle.docx \
		--tenant-id $(TENANT_ID) --title "Acme Corp Software Development Lifecycle"
	cd ingest && .venv/bin/python -m ingest.cli ingest ../$(CORPUS_DIR)/data_retention_policy.docx \
		--tenant-id $(TENANT_ID) --title "Acme Corp Data Retention Policy"
	cd ingest && .venv/bin/python -m ingest.cli ingest ../$(CORPUS_DIR)/vendor_management_policy.html \
		--tenant-id $(TENANT_ID) --title "Acme Corp Vendor Management Policy"
	cd ingest && .venv/bin/python -m ingest.cli ingest ../$(CORPUS_DIR)/acceptable_use_policy.html \
		--tenant-id $(TENANT_ID) --title "Acme Corp Acceptable Use Policy"
	cd ingest && .venv/bin/python -m ingest.cli ingest ../$(CORPUS_DIR)/business_continuity_plan.html \
		--tenant-id $(TENANT_ID) --title "Acme Corp Business Continuity Plan"
	cd ingest && .venv/bin/python -m ingest.cli ingest ../$(CORPUS_DIR)/employee_compensation_bands.pdf \
		--tenant-id $(TENANT_ID) --title "Acme Corp Employee Compensation Bands"
	cd ingest && .venv/bin/python -m ingest.cli ingest ../$(CORPUS_DIR)/it_asset_inventory_standards.pdf \
		--tenant-id $(TENANT_ID) --title "Acme Corp IT Asset Inventory Standards"
	cd ingest && .venv/bin/python -m ingest.cli ingest ../$(CORPUS_DIR)/code_of_conduct.docx \
		--tenant-id $(TENANT_ID) --title "Acme Corp Code of Conduct"
	cd ingest && .venv/bin/python -m ingest.cli ingest ../$(CORPUS_DIR)/travel_safety_guidelines.docx \
		--tenant-id $(TENANT_ID) --title "Acme Corp Travel Safety Guidelines"
	@echo "==> All 15 documents ingested."

# ── All Tests ────────────────────────────────────────────

test: api-test ingest-test

# ── Eval ─────────────────────────────────────────────────

eval: eval-retrieval

eval-retrieval:
	cd ingest && .venv/bin/python ../eval/run_eval.py --mode retrieval --questions ../eval/questions.jsonl --output ../eval/results_retrieval.csv $(EVAL_ARGS)

eval-full:
	cd ingest && .venv/bin/python ../eval/run_eval.py --mode full --questions ../eval/questions.jsonl --output ../eval/results_full.csv --api-url http://localhost:$${API_PORT:-8000} $(EVAL_ARGS)

e2e-smoke:
	@echo "==> Running E2E smoke test..."
	@bash scripts/e2e_smoke.sh http://localhost:$${API_PORT:-8000}

redteam:
	@echo "==> Running red team probes..."
	cd ingest && .venv/bin/python ../eval/run_redteam.py --api-url http://localhost:$${API_PORT:-8000} --output ../eval/redteam_results.json

# ── Meta ─────────────────────────────────────────────────

update-rules:
ifndef MSG
	$(error MSG is required. Usage: make update-rules MSG="what you learned")
endif
	@echo "$$(date -u +%Y-%m-%dT%H:%M:%SZ) — $(MSG)" >> docs/lessons-learned/log.md
	@echo "==> Learning captured in docs/lessons-learned/log.md"

validate-rules:
	@echo "==> Validating project rules..."
	@echo "  Checking required files..."
	@test -f DEVELOPMENT_RULES.md || (echo "MISSING: DEVELOPMENT_RULES.md" && exit 1)
	@test -f docker-compose.yml || (echo "MISSING: docker-compose.yml" && exit 1)
	@test -f Makefile || (echo "MISSING: Makefile" && exit 1)
	@test -f .env.example || (echo "MISSING: .env.example" && exit 1)
	@test -d migrations || (echo "MISSING: migrations/" && exit 1)
	@test -f migrate/run.sh || (echo "MISSING: migrate/run.sh" && exit 1)
	@test -d core-api-go || (echo "MISSING: core-api-go/" && exit 1)
	@test -d ingest || (echo "MISSING: ingest/" && exit 1)
	@test -f docs/ARCHITECTURE.md || (echo "MISSING: docs/ARCHITECTURE.md" && exit 1)
	@test -f docs/DECISIONS.md || (echo "MISSING: docs/DECISIONS.md" && exit 1)
	@echo "  All required files present."
	@echo "==> Validation passed."
