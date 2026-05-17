# wa-digital-democracy

A source-linked public graph of Washington State government — bills, hearings,
testimony, video, money, and lobbying — modeled on CalMatters Digital Democracy.

This repo is the implementation of the MVP defined in
`~/Documents/v1/wiki/politics/Washington Digital Democracy - First Page Implementation Blueprint.md`.

## Status

**Phase 1 (repo scaffold + shared infra):** in progress.

Earlier phases:

- **Phase 0 (feasibility spike):** complete. Findings in `data/spike/REPORT.md`. Headline: TVW captions cover 100% of legislative-committee events sampled, CSI retains the full 2025–26 biennium, and Committee Schedules exposes a direct TVW event-ID mapping for ~75% of returned meetings.

## Stack

Per `~/Documents/v1/wiki/politics/Washington Digital Democracy - Recommended Tech Stack.md`:

- **Backend / ingestion:** Go (`net/http` + `chi`, `pgx`, `sqlc`, `goose`).
- **Frontend:** Next.js + React + TypeScript + Tailwind + shadcn-style components.
- **Database:** Postgres (+ PostGIS later) with JSONB and `pg_trgm`.
- **Raw storage:** local filesystem under `data/raw/` for prototype; S3/R2 in production.

The first-page architecture is "Go produces a JSON bundle; Next.js renders the bundle."

## Local dev

```sh
make up            # start Postgres in Docker
make migrate-up    # apply schema (uses goose if installed; falls back to psql)
make test          # run Go tests
make build         # build the wa-dd CLI and wa-dd-api server
make psql          # open a shell against the local DB
```

## Layout

```
cmd/
  wa-dd/                  # operator CLI (find-candidates, ingest-*, build-bundle)
  wa-dd-api/              # read-only HTTP API for the Next.js frontend
  wa-dd-spike/            # Phase 0 throwaway programs
internal/
  sources/{lws,csi,committeeschedules,tvw,pdc}/
                          # connectors (Phase 2): Fetch / StoreRaw / Parse / Normalize
  sources/httpx/          # shared retry + rate-limit + raw-bytes hook
  storage/{db,objectstore}/
                          # pgx wrapper, source_record helpers, filesystem object store
db/
  migrations/             # goose-style SQL migrations
  queries/                # sqlc query files
config/
  issue_keywords.yml
  selected_demo.yml       # operator-edited; pins the bill/hearing rendered
  reviewed_matches.yml    # human-approved org matches
infra/
  docker-compose.yml
data/
  raw/                    # immutable raw API responses (gitignored)
  processed/              # JSON bundles for rendering (gitignored)
  spike/                  # Phase 0 outputs
```

## Architectural ground rules

From the wiki Recommended Tech Stack §"Key architectural decisions":

1. Postgres owns truth; search and AI summaries are rebuildable.
2. Raw source records are immutable.
3. Every public fact needs provenance (`source_url` + `fetched_at`).
4. Confidence is a first-class field.
5. Manual review is a feature, not a failure.
6. Start static, grow dynamic.

The schema in `db/migrations/0001_initial.sql` enforces #2 and #3 by requiring
every normalized row to point at an immutable `source_record`.

## License

TBD.
