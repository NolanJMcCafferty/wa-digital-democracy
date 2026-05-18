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

ifneq (,$(wildcard $(ENV_FILE)))
include $(ENV_FILE)
export
endif

.PHONY: help up down nuke ps psql migrate-up migrate-down migrate-fresh db-docs db-docs-open test build build-demo vet fmt tidy api ingest-session discover-hearings ingest-hearings daily

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

build:        ## Build all binaries into ./bin
	mkdir -p bin
	$(GO) build -o bin/ ./cmd/...

build-demo: build ## Build the selected first-page demo bundle using $(ENV_FILE)
	@test -n "$$INVINTUS_EMBEDDER_KEY" || { echo "INVINTUS_EMBEDDER_KEY is required; copy .env.example to .env.local and fill it in."; exit 1; }
	./bin/wa-dd build-bundle --config config/selected_demo.yml

vet:
	$(GO) vet ./...

fmt:
	gofmt -w .

tidy:
	$(GO) mod tidy

api:          ## Run the read-only HTTP API on :8080 (read by the Next.js frontend)
	$(GO) run ./cmd/wa-dd-api

ingest-session: ## Pull LWS metadata for every bill in BIENNIUM (default 2025-26). Used by cron.
	$(GO) run ./cmd/wa-dd ingest-session --biennium $${BIENNIUM:-2025-26}

discover-hearings: ## Auto-fill CSI/TVW IDs on every LWS hearing in BIENNIUM
	$(GO) run ./cmd/wa-dd discover-hearings --biennium $${BIENNIUM:-2025-26}

ingest-hearings: ## Run the full pipeline for every discovered agenda item in BIENNIUM
	@if [ -z "$$INVINTUS_EMBEDDER_KEY" ]; then \
		echo "INVINTUS_EMBEDDER_KEY is required (export it or put it in your env)"; exit 1; \
	fi
	$(GO) run ./cmd/wa-dd ingest-hearings --biennium $${BIENNIUM:-2025-26}

daily: ingest-session discover-hearings ingest-hearings ## One-call nightly: metadata + hearing discovery + auto-ingest
