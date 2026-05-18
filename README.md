# wa-digital-democracy

A source-linked public graph of Washington State government — bills, hearings,
testimony, video, money, and lobbying — modeled on CalMatters Digital Democracy.

This repo is the implementation of the MVP defined in
`~/Documents/v1/wiki/politics/Washington Digital Democracy - First Page Implementation Blueprint.md`.

## Status

**Phase 1 (repo scaffold + shared infra):** in progress.

Earlier phases:

- **Phase 0 (feasibility spike):** complete. Findings in `docs/phase0-spike-report.md`. Headline: TVW captions cover 100% of legislative-committee events sampled, CSI retains the full 2025–26 biennium, and Committee Schedules exposes a direct TVW event-ID mapping for ~75% of returned meetings. The throwaway spike commands have been removed; durable learnings now live in docs and source connector tests.

## Stack

Per `~/Documents/v1/wiki/politics/Washington Digital Democracy - Recommended Tech Stack.md`:

- **Backend / ingestion:** Go (`net/http` + `chi`, `pgx`, `sqlc`, `goose`).
- **Frontend:** Next.js + React + TypeScript + Tailwind + shadcn-style components.
- **Database:** Postgres (+ PostGIS later) with JSONB and `pg_trgm`.
- **Raw storage:** local filesystem under `data/raw/` for prototype; S3/R2 in production.

The first-page architecture is "Go produces a JSON bundle; Next.js renders the bundle."

## Local dev

```sh
cp .env.example .env.local  # fill in local-only secrets; .env.local is gitignored
make up                     # start Postgres in Docker
make migrate-up             # apply schema (uses goose, host psql, or container psql)
make build-demo             # build the selected first-page JSON bundle
make test                   # run Go tests
make build                  # build the wa-dd CLI and wa-dd-api server
make psql                   # open a shell against the local DB
make api                    # run the HTTP API the Next.js frontend reads from (:8080)
```

`make build-demo` loads `.env.local` by default. Set `ENV_FILE=/path/to/file`
to use a different local environment file.

`INVINTUS_EMBEDDER_KEY` is required for TVW/Invintus caption ingestion.
`SOCRATA_APP_TOKEN` is optional for data.wa.gov/PDC reads; leave it blank
unless/until broader PDC ingestion starts hitting Socrata/Tyler throttling.

The frontend reads from the API, so a full local loop is:

```sh
make up                                     # Postgres
make api &                                  # Go API on :8080
cd apps/web && pnpm dev                     # Next.js on :3000
```

Override the API URL the frontend hits with `WADD_API_URL` (default
`http://localhost:8080`) — useful when running the API on a non-default
port or against a remote dev DB.

## Daily batch

`make daily-bundles` rebuilds bundles for every entry in
`config/selected_bills.yml`. The frontend reads bills via the Go API
(`make api`), which queries the same Postgres rows the daily run
populates — so a successful batch refresh is what makes the page fresh.
The on-disk bundles in `data/processed/bundles/` are kept as snapshots
for offline inspection and as a debugging fallback; the frontend does
not read them.

To populate the list:

1. `go run ./cmd/wa-dd find-candidates --issue housing` (or another keyword
   set in `config/issue_keywords.yml`). This writes
   `data/processed/candidates.json` and prints a copy-paste-ready table.
2. Pick 5–10 candidates with TVW captions and paste each one's IDs into
   `config/selected_bills.yml` under `bills:` (the file's header comment
   has the schema). Bills without a `tvw.event_id` are rejected at load
   time — TVW is required for transcript ingestion.
3. `INVINTUS_EMBEDDER_KEY=… make daily-bundles`.

Per-bill failures are isolated: one bad entry won't stop the rest. After
the run, `data/processed/bundles/_run.json` summarizes which bills
succeeded, which failed (with the error), and how long each took. The
process exits non-zero if any bill failed so cron flags it.

For a daily refresh, add a crontab entry on the operator's machine, e.g.:

```cron
30 3 * * * cd ~/workspace/wa-digital-democracy && INVINTUS_EMBEDDER_KEY=… make daily-bundles >> /tmp/wa-dd-daily.log 2>&1
```

Re-running is cheap in DB writes — `source_record` dedups on
`(system, endpoint, url, content_hash, transform_version)` and just bumps
`fetched_at` for unchanged content — but every run still re-hits every API
at 2 req/sec, so 5–10 bills is the right scope here.

## Layout

```
cmd/
  wa-dd/                  # operator CLI (find-candidates, ingest-*, build-bundle)
  wa-dd-api/              # read-only HTTP API for the Next.js frontend
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
docs/
  phase0-spike-report.md          # preserved feasibility findings
  written-testimony-source-note.md # pending written-testimony access note
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
