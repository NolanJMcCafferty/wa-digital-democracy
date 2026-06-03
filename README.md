# wa-digital-democracy

A source-linked public graph of Washington State legislative activity — bills,
hearings, testimony, video, transcripts, reviewed speakers, organizations, and
public-record context — modeled on CalMatters Digital Democracy.

## Stack

Current stack:

- **Backend / ingestion:** Go (`net/http` + `chi`, `pgx`, `goose`).
- **Frontend:** Next.js + React + TypeScript + Tailwind + shadcn-style components.
- **Database:** Postgres with JSONB, `pg_trgm`, and project-pinned Goose migrations.

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

`INVINTUS_EMBEDDER_KEY` is required for Invintus event/media ingestion.
`PYANNOTEAI_API_KEY` is required for the default hearing diarization path.

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

The daily chain is idempotent and safe to re-run:

1. **`make ingest-legislators`** — refreshes the LWS House+Senate roster used
   by bill sponsor joins and speaker/entity context.

2. **`make ingest-session`** — pulls **every bill in the biennium** from
   LWS `GetLegislationByYear` and stores metadata + sponsors + status
   timeline + hearing references, then discovers CSI agenda IDs and TVW event
   IDs for those hearing rows. Testimony/video/media are **not** ingested here.
   ~5,000 bills at the default rate/concurrency, runtime depends on upstream
   latency. Summaries: `data/processed/_session.json` and
   `data/processed/_discovery.json`.

3. **`make ingest-irs-bmf-wa`** and **`make ingest-pdc-employers`** — refresh
   organization verification context before hearing testimony is loaded. IRS
   BMF gives nonprofit verification; PDC gives lobbyist-employer registrations
   and lobbyist/person affiliations.

4. **`make ingest-hearings`** — runs the full pipeline for every discovered
   hearing: CSI testifiers per agenda item, Invintus event/media metadata once
   per TVW event, diarization before transcript segmentation, bill-window
   segmentation per agenda item, and organization seeding. Because IRS/PDC
   context is already loaded, source-backed CSI organizations can be confirmed
   as they are created. This is what produces the rich bill-hearing pages.
   Summary: `data/processed/_ingest.json`.

5. **`make verify-organizations`** — re-checks any existing unconfirmed
   organizations against the fresh IRS/PDC reference tables so older rows do
   not have to wait for a hearing re-ingest.

6. **`make generate-vendor-entity-matches`** — generates reviewable source/org
   match candidates after verification has established the best available
   canonical organization state.

For nightly cron, one line is enough:

```cron
30 3 * * * cd ~/workspace/wa-digital-democracy && INVINTUS_EMBEDDER_KEY=… PYANNOTEAI_API_KEY=… make daily >> /tmp/wa-dd-daily.log 2>&1
```

Re-running is cheap in DB writes because ingestion uses stable source IDs and
idempotent upserts, but every run still re-hits upstream APIs at the configured
rate.

## Layout

```
cmd/
  wa-dd/                  # operator CLI: ingest, daily, backfill jobs
  wa-dd-api/              # HTTP API for the Next.js frontend
apps/
  web/                    # Next.js frontend, admin review UI, API proxy routes
internal/
  candidate/              # candidate hearing finder
  diarization/            # speaker diarization/evidence helpers
  common/                 # shared civic value objects
  entitymatch/            # organization/entity matching logic
  jobs/                   # ingestion and enrichment pipeline steps
  sources/                # external source connectors
  sources/httpx/          # shared HTTP retry/rate-limit client
  storage/db/             # pgx store and query helpers
db/
  fixtures/               # deterministic test/e2e fixture data
  migrations/             # goose-style SQL migrations
infra/
  docker-compose.yml      # local services
  railway/                # Railway bootstrap config and deployment docs
scripts/
  seed-test-fixtures.sh
  cleanup-test-fixtures.sh
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
```
