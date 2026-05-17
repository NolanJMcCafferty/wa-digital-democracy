# Common dev tasks for WA Digital Democracy.
# Source-of-truth wiki lives at ~/Documents/v1/wiki/politics/.

DSN ?= postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable
COMPOSE := docker compose -f infra/docker-compose.yml

.PHONY: help up down nuke ps psql migrate-up migrate-down migrate-fresh test build vet fmt tidy

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

migrate-up:   ## Apply all migrations (uses goose if installed; otherwise psql)
	@if command -v goose >/dev/null; then \
		goose -dir db/migrations postgres "$(DSN)" up; \
	else \
		echo "goose not installed; applying with psql (Up section only)"; \
		awk '/-- \+goose Up/{flag=1; next} /-- \+goose Down/{flag=0} flag' db/migrations/0001_initial.sql \
		  | grep -vE '^-- \+goose Statement(Begin|End)' \
		  | PGPASSWORD=wadd psql -h localhost -U wadd -d wa_dd -v ON_ERROR_STOP=1 -q; \
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
	go test ./...

build:        ## Build all binaries into ./bin
	mkdir -p bin
	go build -o bin/ ./cmd/...

vet:
	go vet ./...

fmt:
	gofmt -w .

tidy:
	go mod tidy
