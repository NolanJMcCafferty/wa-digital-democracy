# AGENTS.md

Guidance for AI coding agents working in this repository.

## What this is

A source-linked public graph of Washington State legislative activity — bills, hearings, testimony, video, transcripts, reviewed speakers, organizations, and public-record context — modeled on CalMatters Digital Democracy. Long-form ingestion walkthrough lives at **`docs/ingestion.md`** — read that before making changes to anything in `internal/jobs/` or `internal/sources/`.

## Architecture in one minute

```
nightly cron: make daily
  ├─ wa-dd ingest-legislators  LWS roster → person, legislator, memberships
  ├─ wa-dd ingest-session     LWS metadata for every bill in biennium (~70 min)
  ├─ wa-dd discover-hearings  fills CSI agenda IDs + TVW event IDs on hearing rows
  ├─ wa-dd ingest-hearings    full hearing pipeline (CSI testifiers +
  │                           Invintus event/media + diarization +
  │                           transcript segmentation + organization links)
  └─ wa-dd ingest-pdc-employers
                              PDC lobbyist-employer registrations

  → Postgres (single source of truth)
  → wa-dd-api on :8080 (chi router; reads only; Clerk-gated /admin routes)
  → Next.js on :3000 (Server Components fetch the API; ISR revalidate: 60;
                      /admin UI for speaker + org review)
```

The `bill` / `legislator` / `bill_sponsor` / `bill_status_change` / `hearing` / `agenda_item` / `tvw_event` / `organization` tables are upsertable on stable keys — re-running passes is safe. `testifier` and `agenda_item_window` are replaced per agenda item; diarization rows are appended per provider job, but `ingest-hearings` checks for an existing succeeded diarization job and uses an event-level advisory lock to avoid duplicate provider submissions.

## Layout

```
cmd/
  wa-dd/        operator CLI. One file per subcommand (cmd_*.go). Covers
                ingestion (ingest-session, discover-hearings, ingest-hearings,
                ingest-legislators, ingest-contracts, ingest-pdc-*,
                ingest-irs-bmf-wa, ingest-usaspending-wa-awards, …),
                diarization (audio-cache, diarize-event, diarize-pending,
                extract-speaker-evidence,
                backfill-speaker-evidence), and entity/org workflows
                (populate-organizations,
                generate-vendor-entity-matches, decide-entity-match,
                verify-organizations, prune-junk-organizations).
  wa-dd-api/    read-only HTTP API the Next.js frontend reads from (:8080).
                Routes split per resource (bill/hearing/org/...); registry in
                routes.go.
internal/
  sources/{lws,csi,committeeschedules,tvw,pdc,socrata,datawa,fiscalwa,
           usaspending,irsbmf,bls,census,epa,fema,hud,sao,seattle,
           seattleauditor,kingcounty,webs,connector,...}/
                connectors. Each follows Fetch / StoreRaw / Parse / Normalize.
                Phase status table lives in internal/sources/README.md.
  sources/httpx/ shared retry + per-host rate-limit client
  jobs/         pipeline step primitives (ingest-bill, ingest-csi, ingest-tvw,
                segment-transcript, populate-organizations, discover)
  diarization/  provider-neutral diarization (Provider interface, Deepgram +
                pyannoteAI implementations, MergeConsecutiveSegments,
                self-introduction evidence extractor).
  entitymatch/  organization/vendor entity-match candidate generation +
                decision recording.
  common/       shared civic value objects (BillKey, BillAgendaTarget)
  storage/db/   pgx wrapper, hand-written Pool.Query methods on *Store,
                BillAgendaTarget lookups and query helpers
db/migrations/  goose-style SQL; project-pinned via tools/goose
db/queries/     EMPTY. sqlc.yaml exists but the project uses hand-written
                Pool.Query methods on *Store, not codegen. Don't add to this
                directory unless explicitly migrating to sqlc.
apps/web/       Next.js 16 + React 19 + TS + Tailwind 4. Server Components
                fetch the Go API by absolute URL (process.env.WADD_API_URL)
                because they don't go through next.config.ts rewrites.
                Client components use the rewrite (/api/v1/* → :8080).
                /admin uses Clerk in production; bypassed in non-prod via
                NODE_ENV gate (see src/lib/adminAuth.ts and src/proxy.ts).
data/processed/ run-summary JSONs, diarization output, and derived
                artifacts (gitignored)
docs/           ingestion.md is the canonical implementation doc.
                hearing-diarization-and-review.md covers the diarization +
                speaker-review pipeline. organization-matching-and-review.md
                covers the org/vendor entity-match flow.
```

## Common commands

```sh
# Local dev environment
make up                     # start Postgres in Docker
make migrate-up             # apply migrations via project-pinned goose
make api                    # run wa-dd-api on :8080
cd apps/web && pnpm dev     # Next.js on :3000

# Tests + gates — see "Running tests" below for full setup per category
go vet ./...
go test ./...                                       # Go unit tests, no infra
make integration                                    # Go integration tests (boots Postgres + seeds)
cd apps/web && pnpm typecheck                       # Frontend typecheck
make e2e                                            # Playwright e2e (requires Postgres + fixtures + chromium)

# Ingestion (operator-driven; usually triggered via make daily)
INVINTUS_EMBEDDER_KEY=… PYANNOTEAI_API_KEY=… make daily  # full nightly chain
go run ./cmd/wa-dd ingest-session --biennium 2025-26 --limit 25  # smoke
go run ./cmd/wa-dd discover-hearings --biennium 2025-26 --limit 47
go run ./cmd/wa-dd ingest-hearings  --biennium 2025-26 --limit 5

```

`make help` lists every Makefile target with a one-line description.

## Running tests

Four categories. **Always use these exact commands** — don't improvise (e.g. `pnpm exec playwright` directly will fail without env vars).

### 1. Go unit tests (no infra)

```sh
go test ./...
go test ./internal/sources/lws/... -run TestParse   # single package + test
go vet ./...
```

### 2. Go integration tests (need Postgres)

```sh
make integration              # boots integration DB, seeds fixtures, runs go test -tags=integration ./...
# under the hood:
#   make integration-db       # docker compose up + migrate
#   make seed-test-fixtures   # deterministic fixture rows
#   WADD_TEST_DSN=… go test -tags=integration ./...
```

For a single integration package: `WADD_TEST_DSN="postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable" go test -tags=integration ./cmd/wa-dd-api/...` (DB must already be up + seeded).

### 3. Frontend typecheck / lint

```sh
cd apps/web && pnpm install      # first time only
cd apps/web && pnpm typecheck
cd apps/web && pnpm lint
```

### 4. End-to-end (Playwright + axe a11y)

E2E spins up `wa-dd-api` and `next start` against a real Postgres seeded with e2e fixtures, then drives Chromium. **First-time setup is required** or every test will fail with "browser not installed" / "module not found".

```sh
# One-time setup
make up                                 # Postgres in Docker
make migrate-up
cd apps/web && pnpm install             # installs @playwright/test, @axe-core/playwright, etc.
make e2e-install                        # downloads Chromium for Playwright

# Before each run
WADD_E2E_DSN="postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable" \
  make seed-e2e-fixtures                # idempotent; reseeds deterministic fixtures

# Run
make e2e                                # builds wa-dd-api + Next.js, then runs full Playwright suite

# Single test (DB already seeded, binary already built)
cd apps/web && WADD_API_BIN="$(pwd)/../../bin/wa-dd-api" \
  WADD_E2E_DSN="postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable" \
  pnpm exec playwright test -g "accessibility landmarks" --reporter=list
```

E2E tests live in `apps/web/e2e/`. Accessibility checks use `@axe-core/playwright` against WCAG 2.0/2.1 A+AA — a real violation fails the build, so fix the markup/styles rather than suppressing rules.

## Conventions worth knowing

- **Time zones.** WA legislative timestamps (LWS, TVW WP archive) are Pacific wall-clock without an explicit zone. Always parse with `time.ParseInLocation(..., "America/Los_Angeles")`. There's a fixed bug history here — see `internal/sources/lws/normalize.go:parseLWSDate` and `internal/jobs/discover.go:tvwPostsForDay`.
- **Bill prefixes.** LWS reports `BillID` in the *current* substituted/engrossed form (`SSB 6054`, `2SHB 1859`). `internal/sources/lws/normalize.go:baseBillPrefix` strips `E`/`N`/`S` chrome down to the bare prefix (`HB`/`SB`/`HJR`/etc.) so a single bill doesn't fork into multiple rows as it moves through the legislature.
- **`IngestBill` two-mode behavior.** When `Demo.Chamber` is set (curated path), it stores one chamber-matched hearing. When empty (`ingest-session` path), it stores every hearing LWS reports so `discover-hearings` has rows to enrich.
- **Rate limit default 10 req/sec** per upstream host. The User-Agent identifies the project so state-agency operators can contact us. Retries on 429/5xx with exponential backoff.
- **Pipeline orchestration** lives in the CLI drivers. `Pipeline.RunMetadataOnly` runs only `IngestBill` for `ingest-session`; `ingest-hearings` iterates by `hearing_id`, runs CSI per agenda item, Invintus + diarization once per TVW event, transcript segmentation per agenda item, then hearing-scoped organization population.
- **API response shapes.** Public API routes should return explicit response/list objects (`BillDetailResponse`, `HearingPage`, `OrganizationPage`, etc.). Keep collection fields initialized to `[]` rather than `nil` so frontend code can treat them as arrays.
- **No generated page snapshots.** Public/frontend page data comes from route-specific API objects assembled from Postgres.
- **API handler pattern.** `func handler(store *db.Store) http.HandlerFunc` returning a closure. Use the `writeJSON` envelope and `{"error": "..."}` for errors. Soft-parse query params (bad `limit=abc` falls back to default rather than 400) — see `billPageHandler` and `searchTranscriptsHandler` for examples.
- **Frontend fetch path.** Server Components fetch by absolute URL (`process.env.WADD_API_URL ?? "http://localhost:8080"`) because they don't traverse `next.config.ts` rewrites. Client components use the rewrite path `/api/v1/...` so requests stay same-origin.
- **Diarization providers.** `internal/diarization` is provider-neutral. pyannoteAI (`precision-2`) is the default for hearing ingestion and can return bundled transcription; Deepgram (`nova-3`) remains available as an alternate provider. See `docs/hearing-diarization-and-review.md` for tradeoffs and run commands.
- **Local admin auth.** `/admin` routes bypass Clerk when `NODE_ENV !== "production"` — see `apps/web/src/lib/adminAuth.ts`, `apps/web/src/proxy.ts`, and `cmd/wa-dd-api/auth.go`'s `localDevAdminBypass`. The web Dockerfile has a separate `web-dev` stage (`NODE_ENV=development`, `next dev`) used by `infra/docker-compose.yml`; the final `web` stage stays production-default for Railway.
- **Backwards compatibility** Typically, you do not need to make changes backwards compatible. Only include backwards compatibility if the user explicitly says so.

## What lives where in the wiki

- `~/Documents/v1/wiki/politics/Washington Digital Democracy - Comprehensive Plan.md` — current product/architecture record. The MVP phases through public beta are marked complete; do not reintroduce stale "next steps" or Phase 4 roadmap language unless Nolan asks.
- `~/Documents/v1/wiki/politics/Washington Digital Democracy - Canonical Data Sources.md` — source-of-truth list of upstream data sources.
- `~/Documents/v1/wiki/politics/data-sources/` — per-source connector specs.
- The wiki is the project-level intent; `docs/ingestion.md` is the implementation walkthrough.
