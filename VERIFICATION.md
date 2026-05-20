# Verification

This repository is verified through the live ingestion/API/frontend flow, not
through generated page snapshots.

## Routine gates

```sh
go test ./...
go run -modfile=tools/goose/go.mod github.com/pressly/goose/v3/cmd/goose -dir db/migrations validate
cd apps/web && pnpm typecheck
cd apps/web && pnpm build
```

## Local end-to-end smoke

```sh
make up
make migrate-up
make ingest-legislators
make ingest-session BIENNIUM=2025-26
make discover-hearings BIENNIUM=2025-26
INVINTUS_EMBEDDER_KEY=... make ingest-hearings BIENNIUM=2025-26
make api
cd apps/web && pnpm dev
```

The expected data path is:

1. `wa-dd` ingestion commands write normalized, source-linked records to Postgres.
2. `wa-dd-api` assembles route-specific JSON responses from Postgres.
3. The Next.js app renders those API responses.

Generated first-page bundle snapshots are no longer part of the supported flow.
