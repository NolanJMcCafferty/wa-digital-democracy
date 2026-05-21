# wa-digital-democracy

A source-linked public graph of Washington State government — bills, hearings,
testimony, video, money, and lobbying — modeled on CalMatters Digital Democracy.

This repo is the implementation of the MVP defined in
`~/Documents/v1/wiki/politics/Washington Digital Democracy - First Page Implementation Blueprint.md`.

## Stack

Per `~/Documents/v1/wiki/politics/Washington Digital Democracy - Recommended Tech Stack.md`:

- **Backend / ingestion:** Go (`net/http` + `chi`, `pgx`, `goose`).
- **Frontend:** Next.js + React + TypeScript + Tailwind + shadcn-style components.
- **Database:** Postgres (+ PostGIS later) with JSONB and `pg_trgm`.
- **Raw storage:** local filesystem under `data/raw/` for prototype; S3/R2 in production.

The page architecture is "Go assembles route-specific JSON page objects from Postgres; Next.js renders those page objects." Postgres is the source of truth; the frontend no longer reads generated snapshot files.

## Local dev

```sh
cp .env.example .env.local  # fill in local-only secrets; .env.local is gitignored
make up                     # start Postgres in Docker
make migrate-up             # apply schema with project-pinned Goose
make test                   # run Go tests
make build                  # build the wa-dd CLI and wa-dd-api server
make psql                   # open a shell against the local DB
make db-docs                # generate SchemaSpy HTML docs for the local schema
make analytics              # start optional Metabase analytics UI on :3001
make api                    # run the HTTP API the Next.js frontend reads from (:8080)
```

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

## Railway deployment

Railway is the first hosted demo target. The deploy contract is:

- `apps/web/Dockerfile` for the Next.js web service.
- root `Dockerfile` default Railway image for `wa-dd-api`, `wa-dd`, and
  `goose`.
- cron jobs use the same root image, with start command
  `wa-dd daily --biennium 2025-26`.
- migrations use the same root image, with `goose -dir /app/db/migrations ...`.
- Cloudflare R2 via `OBJECT_STORE=s3` and `S3_*` variables for raw source
  artifacts.

See `infra/railway/README.md` and `infra/railway/variables.example.env` for
the service layout and required variables. Hosted code accepts `DATABASE_URL`
as the production alias for local `WADD_DSN`; the API also honors Railway's
`PORT`.

### Database docs

Generate browsable SchemaSpy documentation for the local Postgres schema:

```sh
make db-docs
```

Open `docs/db/schemaspy/index.html` after generation. The generated HTML is
ignored by git; the committed config lives in `docs/db/schemaspy.properties`.
Use `make db-docs-open` to generate and open the docs in the default browser.

### Local analytics

Start the optional Metabase analytics UI:

```sh
make analytics
```

Then open `http://localhost:3001`. See `docs/metabase.md` for first-run setup
and starter dashboard ideas.

## Daily batch

Three cooperating ingestions, chained by `make daily`. Each stage is
idempotent and safe to re-run:

1. **`make ingest-session`** — pulls **every bill in the biennium** from
   LWS `GetLegislationByYear` and stores metadata + sponsors + status
   timeline + hearing references. Hearings/testimony/video are **not**
   touched here — just the LWS-side claims about each bill. ~5,000
   bills at the default rate/concurrency, runtime depends on upstream latency. Summary:
   `data/processed/_session.json`.

2. **`make ingest-hearings`** — first discovers CSI agenda IDs + TVW
   event IDs for LWS-reported hearings, then runs the full pipeline for
   every discovered agenda item (CSI testifiers + TVW captions +
   transcript segmentation + speaker matching + PDC context). Skips
   agenda items already ingested (no testifier rows means "not yet
   ingested"). This is what produces the rich bill-hearing pages.
   Summaries: `data/processed/_discovery.json` and
   `data/processed/_ingest.json`.

`make discover-hearings` remains available as a lower-level debugging
and backfill target when you only want to refresh CSI/TVW join IDs.

For nightly cron, one line is enough:

```cron
30 3 * * * cd ~/workspace/wa-digital-democracy && INVINTUS_EMBEDDER_KEY=… make daily >> /tmp/wa-dd-daily.log 2>&1
```

Re-running is cheap in DB writes — `source_record` dedups on
`(system, endpoint, url, content_hash, transform_version)` and just
bumps `fetched_at` for unchanged content — but every run still re-hits
every upstream API at the configured rate.

## Layout

```
cmd/
  wa-dd/                  # operator CLI (find-candidates, ingest-*, daily pipeline)
  wa-dd-api/              # read-only HTTP API for the Next.js frontend
internal/
  sources/{lws,csi,committeeschedules,tvw,pdc}/
                          # connectors (Phase 2): Fetch / StoreRaw / Parse / Normalize
  sources/httpx/          # shared retry + rate-limit + raw-bytes hook
  storage/{db,objectstore}/
                          # pgx wrapper, source_record helpers, filesystem object store
db/
  migrations/             # goose-style SQL migrations
config/
  issue_keywords.yml
infra/
  docker-compose.yml
data/
  raw/                    # immutable raw API responses (gitignored)
  processed/              # run summaries and derived artifacts (gitignored)
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
