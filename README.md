# wa-digital-democracy

A source-linked public graph of Washington State legislative activity — bills,
hearings, testimony, video, transcripts, reviewed speakers, organizations, and
public-record context — modeled on CalMatters Digital Democracy.

## Stack

Current stack:

- **Backend / ingestion:** Go (`net/http` + `chi`, `pgx`, `goose`).
- **Frontend:** Next.js + React + TypeScript + Tailwind + shadcn-style components.
- **Database:** Postgres with JSONB, `pg_trgm`, source records, and project-pinned Goose migrations.
- **Raw storage:** local filesystem under `data/raw/` for immutable source responses.

## Local dev

```sh
cp .env.example .env.local  # fill in local-only secrets; .env.local is gitignored
make up                     # start Postgres in Docker
make migrate-up             # apply schema with project-pinned Goose
make test                   # run Go tests
make build                  # build the wa-dd CLI and wa-dd-api server
make psql                   # open a shell against the local DB
make db-docs                # generate SchemaSpy HTML docs and open them in a browser
make analytics              # start optional Metabase analytics UI on :3001
make api                    # run the HTTP API the Next.js frontend reads from (:8080)
```

`INVINTUS_EMBEDDER_KEY` is required for TVW/Invintus caption ingestion.

The frontend reads from the API, so a full local loop is:

```sh
make up                                     # Postgres
export WADD_INTERNAL_API_TOKEN=dev-change-me
make api &                                  # Go API on :8080
cd apps/web && WADD_INTERNAL_API_TOKEN=dev-change-me pnpm dev  # Next.js on :3000
```

### Database docs

Generate browsable SchemaSpy documentation for the local Postgres schema and open it in the default browser:

```sh
make db-docs
```

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
   every discovered agenda item (CSI testifiers + TVW captions,
   transcript segmentation, organization seeding, and source/context
   enrichment). Skips agenda items already ingested (no testifier rows
   means "not yet ingested"). This is what produces the rich
   bill-hearing pages.
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
  wa-dd/                  # operator CLI: ingest, discover, daily, backfill jobs
  wa-dd-api/              # HTTP API for the Next.js frontend
apps/
  web/                    # Next.js frontend, admin review UI, API proxy routes
internal/
  candidate/              # candidate hearing finder
  diarization/            # speaker diarization/evidence helpers
  domain/                 # shared civic-domain value objects
  entitymatch/            # organization/entity matching logic
  jobs/                   # ingestion and enrichment pipeline steps
  pageassembly/           # API response assemblers
  sources/                # external source connectors
  sources/httpx/          # shared HTTP retry/rate-limit/raw sink hook
  storage/db/             # pgx store, source records, query helpers
  storage/objectstore/    # local/S3 raw artifact storage
db/
  fixtures/               # deterministic test/e2e fixture data
  migrations/             # goose-style SQL migrations
config/
  issue_keywords.yml
infra/
  docker-compose.yml      # local services
  railway/                # Railway Terraform/config/docs
scripts/
  seed-test-fixtures.sh
tools/
  goose/                  # project-pinned goose module
data/
  raw/                    # immutable raw API responses (gitignored)
  processed/              # run summaries and derived artifacts (gitignored)
docs/
  db/                     # SchemaSpy config and database docs notes
  ingestion.md            # canonical implementation/operator walkthrough
  testing.md              # test strategy and commands
  *.md                    # feature/source/operator notes
DEPLOYMENT.md             # hosted deployment overview
VERIFICATION.md           # verification checklist/status
```
