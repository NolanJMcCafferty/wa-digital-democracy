# Common dev tasks for WA Digital Democracy.
# Source-of-truth wiki lives at ~/Documents/v1/wiki/politics/.

DSN ?= postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable
COMPOSE := docker compose -f infra/docker-compose.yml
ENV_FILE ?= .env.local
GO ?= go

ifneq (,$(wildcard $(ENV_FILE)))
include $(ENV_FILE)
export
endif

.PHONY: help up down nuke ps psql migrate-up migrate-down migrate-fresh test build build-demo vet fmt tidy

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

migrate-up:   ## Apply all migrations (uses goose, host psql, or container psql)
	@if command -v goose >/dev/null; then \
		goose -dir db/migrations postgres "$(DSN)" up; \
	else \
		echo "goose not installed; applying with psql (Up sections only)"; \
		if command -v psql >/dev/null; then \
			for f in db/migrations/*.sql; do \
				awk '/-- \+goose Up/{flag=1; next} /-- \+goose Down/{flag=0} flag' $$f \
				  | grep -vE '^-- \+goose Statement(Begin|End)' \
				  | PGPASSWORD=wadd psql -h localhost -U wadd -d wa_dd -v ON_ERROR_STOP=1 -q; \
			done; \
		elif docker ps --format '{{.Names}}' | grep -qx wa-dd-postgres; then \
			for f in db/migrations/*.sql; do \
				awk '/-- \+goose Up/{flag=1; next} /-- \+goose Down/{flag=0} flag' $$f \
				  | grep -vE '^-- \+goose Statement(Begin|End)' \
				  | docker exec -i wa-dd-postgres psql -U wadd -d wa_dd -v ON_ERROR_STOP=1 -q; \
			done; \
		else \
			echo "psql not installed and wa-dd-postgres is not running. Run 'make up' first or install psql/goose."; \
			exit 1; \
		fi; \
	fi

migrate-down: ## Roll back the last migration
	@if command -v goose >/dev/null; then \
		goose -dir db/migrations postgres "$(DSN)" down; \
	else \
		echo "goose not installed; cannot run -down. Install with:"; \
		echo "  go install github.com/pressly/goose/v3/cmd/goose@latest"; \
		exit 1; \
	fi

migrate-fresh: nuke up    ## Wipe DB and re-migrate
	@sleep 2
	@$(MAKE) migrate-up

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
