# AGENTS.md

Guidance for AI coding agents working in this repository.

## What this is

A source-linked public graph of Washington State government — bills, hearings, testimony, video, money, lobbying — modeled on CalMatters Digital Democracy. The implementation tracks the MVP defined in `~/Documents/v1/wiki/politics/Washington Digital Democracy - First Page Implementation Blueprint.md`. Long-form ingestion walkthrough lives at **`docs/ingestion.md`** — read that before making changes to anything in `internal/jobs/` or `internal/sources/`.

## Architecture in one minute

```
nightly cron: make daily
  ├─ wa-dd ingest-session    LWS metadata for every bill in biennium (~70 min)
  ├─ wa-dd discover-hearings  fills CSI agenda IDs + TVW event IDs on hearing rows
  └─ wa-dd ingest-hearings    full pipeline (CSI testifiers + TVW captions +
                              transcript + speaker matching + PDC) per hearing

  → Postgres (single source of truth)
  → wa-dd-api on :8080 (chi router; reads only)
  → Next.js on :3000 (Server Components fetch the API; ISR revalidate: 60)
```

Every public fact traces back to a `source_record` row (raw bytes on disk under `data/raw/<system>/`, deduped by `(system, endpoint, url, content_hash, transform_version)`). The `httpx.RawSink` is wired into the HTTP client so every connector fetch records provenance with no per-step boilerplate.

The `bill` / `legislator` / `bill_sponsor` / `bill_status_change` / `hearing` / `agenda_item` / `tvw_event` / `organization` tables are upsertable on stable keys — re-running passes is safe. **Exception:** `testifier` and `transcript_segment` are insert-only with no dedupe; `ingest-hearings` filters to agenda items without testifier rows so the routine path doesn't hit this.

## Layout

```
cmd/
  wa-dd/        operator CLI (find-candidates, ingest-session,
                discover-hearings, ingest-hearings, entity/backfill jobs)
  wa-dd-api/    read-only HTTP API the Next.js frontend reads from (:8080)
internal/
  sources/{lws,csi,committeeschedules,tvw,pdc,socrata,...}/
                connectors. Each follows Fetch / StoreRaw / Parse / Normalize.
                Phase status table lives in internal/sources/README.md.
  sources/httpx/ shared retry + per-host rate-limit + RawSink hook
  jobs/         pipeline steps (ingest-bill, ingest-csi, ingest-tvw,
                segment-transcript, match-speakers, pdc-context, discover)
  candidate/    find-candidates implementation (CSI committee scan)
  storage/db/   pgx wrapper, hand-written Pool.Query methods on *Store,
                source_record helpers, RawSink
  storage/objectstore/  filesystem object store for raw API responses
  render/firstpage/  Page-object assemblers + DB→SelectedDemo lookups
  config/       shared ingestion configuration structs
db/migrations/  goose-style SQL; project-pinned via tools/goose
db/queries/     EMPTY. sqlc.yaml exists but the project uses hand-written
                Pool.Query methods on *Store, not codegen. Don't add to this
                directory unless explicitly migrating to sqlc.
apps/web/       Next.js 16 + React 19 + TS + Tailwind 4. Server Components
                fetch the Go API by absolute URL (process.env.WADD_API_URL)
                because they don't go through next.config.ts rewrites.
                Client components use the rewrite (/api/v1/* → :8080).
config/         operator-edited YAML (issue_keywords.yml)
data/raw/       immutable raw API responses (gitignored)
data/processed/ run-summary JSONs and derived artifacts (gitignored)
docs/           ingestion.md is the canonical implementation doc.
                phase0-spike-report.md is the frozen Phase 0 findings.
```

## Common commands

```sh
# Local dev environment
make up                     # start Postgres in Docker
make migrate-up             # apply migrations via project-pinned goose
make api                    # run wa-dd-api on :8080
cd apps/web && pnpm dev     # Next.js on :3000

# Tests + gates
go vet ./...
go test ./...
go test -tags=integration ./cmd/wa-dd-api/...   # needs WADD_DSN + dev Postgres
go test ./internal/sources/lws/... -run TestParse  # single package, single test
cd apps/web && pnpm typecheck

# Ingestion (operator-driven; usually triggered via make daily)
INVINTUS_EMBEDDER_KEY=… make daily             # full nightly chain
go run ./cmd/wa-dd ingest-session --biennium 2025-26 --limit 25  # smoke
go run ./cmd/wa-dd discover-hearings --biennium 2025-26 --limit 47
go run ./cmd/wa-dd ingest-hearings  --biennium 2025-26 --limit 5

```

`make help` lists every Makefile target with a one-line description.

## Conventions worth knowing

- **Time zones.** WA legislative timestamps (LWS, TVW WP archive) are Pacific wall-clock without an explicit zone. Always parse with `time.ParseInLocation(..., "America/Los_Angeles")`. There's a fixed bug history here — see `internal/sources/lws/normalize.go:parseLWSDate` and `internal/jobs/discover.go:tvwPostsForDay`.
- **Bill prefixes.** LWS reports `BillID` in the *current* substituted/engrossed form (`SSB 6054`, `2SHB 1859`). `internal/sources/lws/normalize.go:baseBillPrefix` strips `E`/`N`/`S` chrome down to the bare prefix (`HB`/`SB`/`HJR`/etc.) so a single bill doesn't fork into multiple rows as it moves through the legislature.
- **`IngestBill` two-mode behavior.** When `Demo.Chamber` is set (curated path), it stores one chamber-matched hearing. When empty (`ingest-session` path), it stores every hearing LWS reports so `discover-hearings` has rows to enrich.
- **Rate limit default 10 req/sec** per upstream host. The User-Agent identifies the project so state-agency operators can contact us. Retries on 429/5xx with exponential backoff.
- **Pipeline orchestration** lives in `internal/jobs/jobs.go`. `Pipeline.Run` runs all 6 hearing-ingestion steps; `Pipeline.RunMetadataOnly` runs only `IngestBill`. The CLI commands wire steps into the discovery/ingest-hearings drivers.
- **Page response shapes.** Public API routes should return explicit page/list objects (`BillPage`, `HearingPage`, `OrganizationPage`, etc.). Keep collection fields initialized to `[]` rather than `nil` so frontend code can treat them as arrays.
- **No generated page snapshots.** The old generated snapshot path has been removed. Public/frontend page data should come from route-specific API objects assembled from Postgres.
- **API handler pattern.** `func handler(store *db.Store) http.HandlerFunc` returning a closure. Use the `writeJSON` envelope and `{"error": "..."}` for errors. Soft-parse query params (bad `limit=abc` falls back to default rather than 400) — see `billPageHandler` and `searchTranscriptsHandler` for examples.
- **Frontend fetch path.** Server Components fetch by absolute URL (`process.env.WADD_API_URL ?? "http://localhost:8080"`) because they don't traverse `next.config.ts` rewrites. Client components use the rewrite path `/api/v1/...` so requests stay same-origin.

## What lives where in the wiki

- `~/Documents/v1/wiki/politics/Washington Digital Democracy - Comprehensive Plan.md` — phase markers, current "Done so far" / "Still to do" / "Immediate Next Steps". Update this when phases progress.
- `~/Documents/v1/wiki/politics/Washington Digital Democracy - First Page Implementation Blueprint.md` — frontend route inventory, API endpoint list, three-pass ingestion summary. Updated alongside Plan.
- `~/Documents/v1/wiki/politics/data-sources/` — per-source connector specs.
- The wiki is the project-level intent; `docs/ingestion.md` is the implementation walkthrough.
