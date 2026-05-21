# Common dev tasks for WA Digital Democracy.
# Source-of-truth wiki lives at ~/Documents/v1/wiki/politics/.

DSN ?= postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable
COMPOSE := docker compose -f infra/docker-compose.yml
COMPOSE_NETWORK ?= infra_default
ENV_FILE ?= .env.local
GO ?= go
GOOSE ?= $(GO) run -modfile=tools/goose/go.mod github.com/pressly/goose/v3/cmd/goose
SCHEMASPY_IMAGE ?= schemaspy/schemaspy:latest
SCHEMASPY_OUT ?= docs/db/schemaspy
COVERAGE_THRESHOLD ?= 50.0
COVERAGE_PKGS ?= ./internal/sources/... ./internal/candidate ./internal/config

ifneq (,$(wildcard $(ENV_FILE)))
include $(ENV_FILE)
export
endif

.PHONY: help up down nuke ps analytics metabase metabase-open psql migrate-up migrate-down migrate-fresh db-docs db-docs-open test coverage build docker-build-api docker-build-cli docker-build-web vet fmt tidy api ingest-legislators ingest-session discover-hearings ingest-hearings daily

help:
	@awk 'BEGIN{FS=":.*##"} /^[a-zA-Z_-]+:.*?##/ {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

up:           ## Start local Postgres (and friends)
	$(COMPOSE) up -d postgres

down:         ## Stop containers
	$(COMPOSE) down

nuke:         ## Stop containers AND wipe volumes
	$(COMPOSE) down -v

ps:           ## Show service status
	$(COMPOSE) ps

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

migrate-fresh: nuke up    ## Wipe DB and re-migrate
	@sleep 2
	@$(MAKE) migrate-up

db-docs: up   ## Generate SchemaSpy HTML docs for the local Postgres schema
	@mkdir -p $(SCHEMASPY_OUT)
	docker run --rm \
		--network $(COMPOSE_NETWORK) \
		-v "$(CURDIR)/$(SCHEMASPY_OUT):/output" \
		-v "$(CURDIR)/docs/db/schemaspy.properties:/schemaspy.properties:ro" \
		$(SCHEMASPY_IMAGE)

db-docs-open: db-docs ## Generate and open SchemaSpy docs in the default browser
	@xdg-open "$(CURDIR)/$(SCHEMASPY_OUT)/index.html" >/dev/null 2>&1 || echo "Open $(CURDIR)/$(SCHEMASPY_OUT)/index.html"

test:         ## Run unit tests
	$(GO) test ./...

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

docker-build-web: ## Build the production Next.js web container
	docker build -t wa-dd-web:local apps/web


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

daily: ingest-legislators ingest-session ingest-hearings ## One-call nightly: roster + metadata + hearing discovery + auto-ingest
