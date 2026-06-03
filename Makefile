# Common dev tasks for WA Digital Democracy.
# Source-of-truth wiki lives at ~/Documents/v1/wiki/politics/.

DSN ?= postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable
COMPOSE := docker compose -f infra/docker-compose.yml
COMPOSE_NETWORK ?= infra_default
ENV_FILE ?= .env.local
GO ?= go
PSQL ?= psql
GOOSE ?= $(GO) run -modfile=tools/goose/go.mod github.com/pressly/goose/v3/cmd/goose
SCHEMASPY_IMAGE ?= schemaspy/schemaspy:latest
SCHEMASPY_OUT ?= docs/db/schemaspy
COVERAGE_THRESHOLD ?= 50.0
COVERAGE_PKGS ?= ./internal/sources/... ./internal/common

ifneq (,$(wildcard $(ENV_FILE)))
include $(ENV_FILE)
export
endif

.PHONY: help up up-db down nuke ps logs logs-api logs-web logs-postgres logs-migrate analytics metabase metabase-open psql migrate-up migrate-down migrate-fresh integration-db seed-test-fixtures seed-e2e-fixtures db-docs test integration e2e e2e-install coverage build docker-build-api docker-build-cli docker-build-migrate docker-build-railway docker-build-web railway-bootstrap railway-deploy vet fmt tidy api ingest-legislators ingest-session discover-hearings ingest-hearings ingest-pdc-employers daily

help:
	@awk 'BEGIN{FS=":.*##"} /^[a-zA-Z_-]+:.*?##/ {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

up:           ## Start the full local stack (Postgres + migrations + API + web)
	$(COMPOSE) up -d --build postgres migrate api web

up-db:        ## Start local Postgres only
	$(COMPOSE) up -d postgres

down:         ## Stop containers
	$(COMPOSE) down

nuke:         ## Stop containers AND wipe volumes
	$(COMPOSE) down -v

ps:           ## Show service status
	$(COMPOSE) ps

logs:         ## Tail logs for all services
	$(COMPOSE) logs -f

logs-api:     ## Tail API logs
	$(COMPOSE) logs -f api

logs-web:     ## Tail web logs
	$(COMPOSE) logs -f web

logs-postgres: ## Tail Postgres logs
	$(COMPOSE) logs -f postgres

logs-migrate: ## Show migration container logs
	$(COMPOSE) logs migrate

analytics:    ## Start optional local Metabase analytics UI on :3001
	$(COMPOSE) --profile analytics up -d metabase

metabase: analytics ## Alias for analytics

metabase-open: analytics ## Start Metabase and open it in the default browser
	@xdg-open http://localhost:3001 >/dev/null 2>&1 || echo "Open http://localhost:3001"

psql:         ## Open psql shell against local Postgres
	PGPASSWORD=wadd psql -h localhost -U wadd -d wa_dd

migrate-up:   ## Apply all migrations with project-pinned goose
	$(GOOSE) -dir db/migrations postgres "$(DSN)" up

migrate-down: ## Roll back the last migration with project-pinned goose
	$(GOOSE) -dir db/migrations postgres "$(DSN)" down

migrate-fresh: nuke up-db ## Wipe DB and re-migrate
	@sleep 2
	@$(MAKE) migrate-up

integration-db: up-db migrate-up ## Start and migrate the local real DB used by integration tests

seed-test-fixtures: ## Seed deterministic fixture data into the local integration DB
	WADD_TEST_DSN="$(DSN)" scripts/seed-test-fixtures.sh --dsn "$(DSN)"

seed-e2e-fixtures: ## Seed deterministic fixture data into an existing e2e/staging DB (requires WADD_E2E_DSN)
	@test -n "$$WADD_E2E_DSN" || { echo "WADD_E2E_DSN is required"; exit 1; }
	scripts/seed-test-fixtures.sh --dsn "$$WADD_E2E_DSN"

db-docs: up-db ## Generate SchemaSpy HTML docs and open them in the default browser
	@mkdir -p $(SCHEMASPY_OUT)
	docker run --rm \
		--network $(COMPOSE_NETWORK) \
		-v "$(CURDIR)/$(SCHEMASPY_OUT):/output" \
		-v "$(CURDIR)/docs/db/schemaspy.properties:/schemaspy.properties:ro" \
		$(SCHEMASPY_IMAGE)
	@xdg-open "$(CURDIR)/$(SCHEMASPY_OUT)/index.html" >/dev/null 2>&1 || echo "Open $(CURDIR)/$(SCHEMASPY_OUT)/index.html"

test:         ## Run unit tests (db package boots a Postgres testcontainer; set WADD_SKIP_DB_TESTS=1 to skip)
	$(GO) test ./...

integration: integration-db seed-test-fixtures ## Run backend integration tests against a real Postgres DB
	WADD_TEST_DSN="$(DSN)" $(GO) test -tags=integration ./...

e2e-install: ## Install Playwright browser dependencies for end-to-end tests
	cd apps/web && pnpm exec playwright install --with-deps chromium

e2e: build ## Run frontend -> backend end-to-end tests against configured env/services
	cd apps/web && SKIP_BUILD_STATIC_PARAMS=1 WADD_INTERNAL_API_TOKEN=$${WADD_INTERNAL_API_TOKEN:-e2e-internal-token} WADD_API_URL=$${WADD_API_URL:-http://127.0.0.1:$${WADD_E2E_API_PORT:-18080}} pnpm build
	cd apps/web && WADD_API_BIN="$(CURDIR)/bin/wa-dd-api" WADD_E2E_DSN="$${WADD_E2E_DSN:-$(DSN)}" pnpm exec playwright test

coverage:     ## Enforce unit test coverage threshold for core packages
	@tmp=$$(mktemp); \
	$(GO) test $(COVERAGE_PKGS) -coverprofile=$$tmp -covermode=atomic; \
	coverage=$$($(GO) tool cover -func=$$tmp | awk '/^total:/ { sub(/%/, "", $$3); print $$3 }'); \
	rm -f $$tmp; \
	echo "Total unit test coverage: $${coverage}% (required: $(COVERAGE_THRESHOLD)%)"; \
	awk -v coverage="$$coverage" -v threshold="$(COVERAGE_THRESHOLD)" 'BEGIN { if (coverage + 0 < threshold + 0) { printf "Coverage %.1f%% is below required %.1f%%\n", coverage, threshold; exit 1 } }'

build:        ## Build all binaries into ./bin
	mkdir -p bin
	$(GO) build -o bin/ ./cmd/...

docker-build-api: ## Build the production Go API container target
	docker build --target api -t wa-dd-api:local .

docker-build-cli: ## Build the production Go CLI/cron container target
	docker build --target cli -t wa-dd-cli:local .

docker-build-migrate: ## Build the production migration container target
	docker build --target migrate -t wa-dd-migrate:local .

docker-build-railway: ## Build the default Railway root container
	docker build -t wa-dd-railway:local .

docker-build-web: ## Build the production Next.js web container
	docker build -t wa-dd-web:local apps/web


railway-bootstrap: ## Reconcile Railway project/environments/services/variables without deploying
	node scripts/bootstrap-railway.mjs apply --skip-deploy

railway-deploy: ## Reconcile Railway and trigger deployments
	node scripts/bootstrap-railway.mjs apply --deploy


vet:
	$(GO) vet ./...

fmt:
	gofmt -w .

tidy:
	$(GO) mod tidy

api:          ## Run the read-only HTTP API on :8080 (read by the Next.js frontend)
	$(GO) run ./cmd/wa-dd-api

ingest-legislators: ## Pull the full House+Senate roster for BIENNIUM (default 2025-26)
	$(GO) run ./cmd/wa-dd ingest-legislators --biennium $${BIENNIUM:-2025-26}

ingest-session: ## Pull LWS metadata for every bill in BIENNIUM (default 2025-26). Used by cron.
	$(GO) run ./cmd/wa-dd ingest-session --biennium $${BIENNIUM:-2025-26}

discover-hearings: ## Auto-fill CSI/TVW IDs on every LWS hearing in BIENNIUM
	$(GO) run ./cmd/wa-dd discover-hearings --biennium $${BIENNIUM:-2025-26}

ingest-hearings: ## Discover hearing IDs, then run the full pipeline for every discovered agenda item
	@if [ -z "$$INVINTUS_EMBEDDER_KEY" ]; then \
		echo "INVINTUS_EMBEDDER_KEY is required (export it or put it in your env)"; exit 1; \
	fi
	$(GO) run ./cmd/wa-dd discover-hearings --biennium $${BIENNIUM:-2025-26}
	$(GO) run ./cmd/wa-dd ingest-hearings --biennium $${BIENNIUM:-2025-26}

ingest-pdc-employers: ## Pull PDC lobbyist-employer registrations into person/org context
	$(GO) run ./cmd/wa-dd ingest-pdc-employers

daily: ingest-legislators ingest-session ingest-hearings ingest-pdc-employers ## One-call nightly: roster + metadata + hearing discovery + PDC context
