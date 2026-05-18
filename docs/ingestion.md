# Data ingestion

Last updated: 2026-05-18.

This document describes how data flows from the official Washington
state sources (LWS, CSI, TVW/Invintus, PDC) into Postgres and out to
the Next.js frontend. It's the operator's reference for what runs when,
where things land, and how to debug a stuck or misbehaving run.

## TL;DR

```
                     nightly cron: make daily
        ┌──────────────────┬──────────────────┬──────────────────┬──────────────────┐
        │                  │                  │                  │                  │
   ingest-session    discover-hearings    ingest-hearings    daily-bundles
   (~70 min)         (~few min)           (variable)         (variable)
        │                  │                  │                  │
   LWS metadata      CSI agenda IDs       full pipeline       curated overrides
   for ~5,000 bills  + TVW event IDs      (CSI testifiers,    (operator-pinned
   in the biennium   on every hearing      TVW captions,       bills in
                     LWS reports           transcript          selected_bills.yml)
                                           segments,
                                           speakers, PDC)
        │                  │                  │                  │
        └──────────────────┴────────┬─────────┴──────────────────┘
                                    ▼
                              Postgres (truth)
                                    ▼
                       wa-dd-api at :8080  (HTTP / JSON)
                                    ▼
                       Next.js frontend at :3000
```

The same Bundle JSON shape is produced by `firstpage.Build` regardless of
which path populated the underlying Postgres rows. The frontend never
distinguishes "auto-discovered" from "operator-curated" data.

## The four passes

Each pass writes to Postgres directly. Each is idempotent — re-running
just bumps `fetched_at` on `source_record` rows where bytes are
unchanged, and does `ON CONFLICT DO UPDATE` (or `DO NOTHING`) on
domain rows.

### 1. `wa-dd ingest-session --biennium 2025-26`

**What it does:** pulls every bill in the biennium from LWS and stores
metadata + sponsors + status timeline + LWS-reported hearing
references.

**Why:** establishes the bill universe. Most bills don't have
hearings, but every bill should have a metadata page (sponsors, status,
title, biennium). And `discover-hearings` needs `hearing` rows to
enrich.

**Implementation:**

- Calls `LegislationService.GetLegislationByYear(year)` for both years
  of the biennium (~3,300 + ~3,900 bills, deduped by `(biennium, bill_id)`).
- For each bill, calls `Pipeline.RunMetadataOnly` which runs only the
  `IngestBill` step.
- `IngestBill` makes 4 LWS SOAP calls per bill:
  - `GetLegislation` → `bill` row + current status snapshot
  - `GetSponsors` → `legislator` + `bill_sponsor` rows
  - `GetLegislativeStatusChangesByBillNumber` → `bill_status_change` rows
  - `GetHearings` → `hearing` rows (one per LWS-reported hearing)
- Per-bill failure isolation; one bad bill doesn't poison the run.

**Why all hearings, not just one:** the original curated path had
`IngestBill` store only the hearing matching the demo's chamber. That
made `ingest-session` (which has no demo chamber) leave the `hearing`
table empty for most bills. As of 2026-05-18, `IngestBill` has two
modes: when `demo.Chamber` is set, it narrows to one hearing (curated
path); when it's empty, it stores every hearing LWS reports
(metadata-only path).

**Output:**

- ~5,000 bills in `bill`
- ~150 legislators in `legislator`, with sponsor relationships
- Hundreds to thousands of `hearing` rows (most bills have 0–2
  hearings; some have more for House+Senate referrals)
- ~5,000 `bill_status_change` rows
- A summary at `data/processed/_session.json` with per-bill durations
  and any failures.

**Cost:** ~5,000 bills × 4 LWS calls = 20,000 calls. At 5 req/sec to
`wslwebservices.leg.wa.gov`, that's ~70 minutes.

**Code:** `cmd/wa-dd/main.go` (`runIngestSession`), `internal/jobs/jobs.go`
(`Pipeline.RunMetadataOnly`), `internal/jobs/ingest_bill.go`
(`IngestBill`).

### 2. `wa-dd discover-hearings --biennium 2025-26`

**What it does:** for every LWS hearing in the DB whose CSI/TVW IDs
are still blank, walks four lookups to fill them in:

1. **CSI committee** — given LWS `(chamber, committee_name)` like
   `("Senate", "Senate Housing")`, strip the chamber prefix and match
   against `csi.Client.ListCommittees(chamber)` by normalized name.
2. **CSI meeting** — given the CSI committee ID and an LWS
   `meeting_datetime`, scan `csi.Client.ListMeetings(chamber, committee_id)`
   for a meeting whose `StartDateTime` is within ±15 minutes. Pick the
   closest.
3. **CSI agenda item** — given the meeting, list its agenda items and
   match by leading bill number in the label
   (regex `\b(?:E?[23]?S?(?:HB|SB|HJR|SJR|HCR|SCR|HJM|SJM))\s*(\d{3,5})\b`,
   first capture group).
4. **TVW event** — fetch the TVW WP archive
   (`/wp/v2/invintus_video?after=...&before=...`) for the meeting's
   day. For each post, sanity-check that the title contains the LWS
   committee name (case-insensitive) and the date is within ±2 hours.
   Extract the event ID via `EventIDFromWPPost` (regex on
   `data-eventid="\d+"` in the rendered HTML content).

The CSI side is required — if any of steps 1–3 fail, the hearing is
skipped. The TVW side is best-effort — a hearing with CSI testifiers
but no TVW captions is still useful.

**Caching policy:** the discoverer caches CSI directory data and the
TVW WP archive per process. Scope of caching:

| Cache | Key | Typical size per nightly run |
|---|---|---|
| `committees` | chamber | 2 entries (House, Senate) |
| `meetings` | committee_id | ~50 entries (committees with hearings) |
| `agendaItems` | meeting_family_id | hundreds (one per unique CSI meeting) |
| `tvwPostsByDay` | YYYY-MM-DD | ~100 days during session |

The wall-clock cost is dominated by `ListAgendaItems` — one HTTP call
per unique meeting. The `--limit 47` smoke run hit ~5–6 calls per
hearing on average and finished in ~17 seconds.

**Sanity checks:**

- **Committee-name match for TVW**: rejects matches where the WP post
  title doesn't contain the LWS committee name. Without this we'd
  occasionally attach the wrong video to a hearing (e.g. a press
  conference happening in the same chamber on the same day).
- **±15 min CSI meeting window**: tighter than the ±2 hour TVW window
  because CSI publishes meeting times to the minute.
- **Bill-number regex constrained to real bill prefixes** (HB/SB/HJR/
  SJR/HCR/SCR/HJM/SJM with optional E/S/2S/3S chrome): rejects
  non-bill agenda items like `SGA 9280` (gubernatorial appointment)
  even when the digit pattern matches.

**Failure modes observed in smoke:**

LWS's `GetHearings` is *optimistic* — it reports hearings for bills
that the committee discussed *or could have discussed*. About half of
LWS-reported hearings on the smoke set turned out not to actually have
the bill on the agenda. Discovery correctly rejects those with `no
agenda item for bill number X in meeting Y`.

A smaller fraction failed `no CSI meeting within 15min of <date>`.
Those are typically older hearings that have rolled off CSI's
default-recent-meetings window. The ±15min window is correct; the
right fix if this becomes a problem is to extend the CSI client's
list-meetings range, not loosen the time window.

**Output:**

- `hearing` rows updated with `committee_schedule_agenda_id`
  (= CSI's meeting_family_id) and `tvw_event_id` (when found).
- `agenda_item` rows inserted with the three CSI IDs.
- A summary at `data/processed/_discovery.json` with per-hearing
  duration, status (`ok` / `no-tvw` / `failed`), and error.

**Cost:** dominated by `ListAgendaItems` — typically one call per
unique CSI meeting in the biennium. Empirically ~few minutes for a
full biennium run after the first batch warms the caches.

**Code:** `cmd/wa-dd/main.go` (`runDiscoverHearings`),
`internal/jobs/discover.go` (`Discoverer`).

### 3. `wa-dd ingest-hearings --biennium 2025-26`

**What it does:** for every `agenda_item` row whose hearing has a TVW
event ID but no testifiers ingested yet, runs the full 6-step
pipeline.

**Why:** discovery populated the join keys; this pass uses them to
fetch the actual testimony, video captions, and downstream derivatives.

**Implementation:**

- Reads `(csi_agenda_item_id, biennium, bill_prefix, bill_number)` rows
  via `Store.ListDiscoveredAgendaItems`.
- For each row, calls `firstpage.LookupSelectedDemoByAgendaItem` to
  rebuild a `*config.SelectedDemo` from the DB (no YAML parsing).
- Calls `buildOne` (the same function the curated `build-bundles` uses)
  which runs `Pipeline.Run`'s 6 steps and writes the bundle JSON
  snapshot.

The 6 pipeline steps (`internal/jobs/jobs.go`):

1. **`IngestBill`** — re-runs the LWS pull. Idempotent: the bill row
   gets touched, but no new data unless the bill metadata or status
   changed.
2. **`IngestCSI`** — fetches the CSI testifier list for the agenda
   item via `csi.Client.GetTestifiers`. Inserts `testifier` rows,
   updates `agenda_item` and `hearing` if needed.
3. **`IngestTVW`** — fetches the Invintus event detail (requires
   `INVINTUS_EMBEDDER_KEY` env var) and the WebVTT caption file.
   Inserts `tvw_event` and `transcript_segment` rows.
4. **`SegmentTranscript`** — finds the bill-discussion window in the
   transcript via bill-mention regex; tags the matching
   `transcript_segment` rows with the agenda_item_id.
5. **`MatchSpeakers`** — heuristic speaker attribution. Today this
   pass largely produces `unknown_speaker` labels; it's a known
   stretch-criterion gap (see [[Comprehensive Plan §Phase 3]]).
6. **`PDCContext`** — for any organization in
   `config/reviewed_matches.yml`, fetches lobbying registrations and
   campaign-finance records from PDC Socrata datasets.

**Output:**

- Per agenda item: 1 hearing row updated, ~1–500 testifier rows,
  ~10–1000 transcript segments, optional org context records.
- Bundle snapshot at `data/processed/bundles/wa_<biennium>_<prefix><n>.json`.
- A summary at `data/processed/_ingest.json` with per-bill durations
  and any failures.

**Cost:** ~6 HTTP calls per bill (3 LWS for the re-ingest, 1 CSI for
testifiers, 2 TVW/Invintus for event detail + captions, plus PDC if
matched orgs). At 5 req/sec, ~1.2–1.6 seconds per bill. Hundreds of
hearings → tens of minutes.

**Code:** `cmd/wa-dd/main.go` (`runIngestHearings`),
`internal/jobs/ingest_csi.go`, `internal/jobs/ingest_tvw.go`,
`internal/jobs/segment_transcript.go`,
`internal/jobs/match_speakers.go`, `internal/jobs/pdc_context.go`.

### 4. `wa-dd build-bundles` (operator overrides)

**What it does:** runs the same 6-step pipeline as `ingest-hearings`,
but driven by `config/selected_bills.yml` instead of the DB. This is
the manual fallback for cases discovery missed (~25% of meetings,
plus anything LWS doesn't report a hearing for).

**Why keep it:** operators can pin a specific bill+hearing+TVW event
combination by hand, e.g. when:

- A hearing's TVW title doesn't contain the committee name
  (sanity-check rejection).
- LWS doesn't report a hearing that actually happened (rare, but
  possible — committees sometimes add bills informally).
- The operator wants to force a re-ingest of a specific bill (e.g.
  after fixing a regex bug).

**Workflow:**

1. `wa-dd find-candidates --issue housing` — scans CSI committees and
   writes `data/processed/candidates.json` with copy-paste-ready IDs
   per candidate.
2. The operator picks 1–N candidates and pastes their IDs into
   `config/selected_bills.yml` under `bills:` (along with a
   `tvw.event_id` looked up from the TVW website).
3. `INVINTUS_EMBEDDER_KEY=… make daily-bundles`.

**Code:** `cmd/wa-dd/main.go` (`runBuildBundles`, `runFindCandidates`).

## Postgres tables

The schema is defined in `db/migrations/0001_initial.sql`. The four
passes interact with these tables:

| Table | Populated by | Notes |
|---|---|---|
| `bill` | `IngestBill` | UPSERT on `(biennium, prefix, number)`. Bill prefix is normalized to bare form (`HB`/`SB`/`HJR`/etc.) — engrossment and substitution chrome (`E`/`2S`/`SS`) is stripped at ingest time so a bill doesn't fork into multiple rows as it moves through the legislature. |
| `legislator` | `IngestBill` | UPSERT on `lws_sponsor_id`. |
| `bill_sponsor` | `IngestBill` | UPSERT on `(bill_id, legislator_id, sponsor_type)` DO NOTHING. |
| `bill_status_change` | `IngestBill` | UPSERT on `(bill_id, action_date, history_line)` DO NOTHING. |
| `hearing` | `IngestBill` (creates), `Discoverer.Commit` (enriches), `IngestCSI` (touches), `IngestTVW` (sets tvw fields) | Soft-key on `(chamber, committee_name, meeting_datetime)`. UPDATE uses COALESCE so partial enrichment is safe. |
| `agenda_item` | `Discoverer.Commit` (creates), `IngestCSI` (touches) | UPSERT on `csi_agenda_item_id`. |
| `testifier` | `IngestCSI` | One row per CSI sign-in. `testified` boolean distinguishes "did testify" from "registered position only". |
| `tvw_event` | `IngestTVW` | One row per Invintus event. UPSERT on `tvw_event_id`. |
| `transcript_segment` | `IngestTVW` (creates), `SegmentTranscript` (tags), `MatchSpeakers` (labels) | One row per WebVTT cue. `agenda_item_id` is set when the cue falls in the bill window. |
| `organization` | `IngestCSI` (rough), `PDCContext` (matched) | UPSERT on `canonical_name`. |
| `org_context_record` | `PDCContext` | One row per PDC dataset row matched to an organization. |
| `source_record` | every HTTP fetch via `httpx.RawSink` | Append-only. UPSERT on `(system, endpoint, url, content_hash, transform_version)` DO UPDATE SET fetched_at — so identical responses get one row that ages forward. |
| `ingestion_run` | `Pipeline.Run`, `RunMetadataOnly` | One row per pipeline step, with `started_at`, `finished_at`, `status`, `error`. Useful for grep-style debugging across runs. |

## Provenance

Every public fact on the page traces to a `source_record` row, and
every `source_record` row stores the canonical URL, fetched-at
timestamp, content hash, and a relative path to the raw bytes on disk
(under `data/raw/<system>/`). The "Sources & confidence" panel at the
bottom of every bill page is built from these rows.

The `httpx.RawSink` (`internal/storage/db/store.go`) is wired into the
HTTP client so every connector fetch automatically writes a
source_record row with no per-step boilerplate.

## Daily orchestration

`make daily` chains the four passes:

```makefile
daily: ingest-session discover-hearings ingest-hearings daily-bundles
```

For nightly cron:

```cron
30 3 * * * cd ~/workspace/wa-digital-democracy && INVINTUS_EMBEDDER_KEY=… make daily >> /tmp/wa-dd-daily.log 2>&1
```

The order matters when fresh:

1. `ingest-session` creates `bill` and `hearing` rows for every bill in
   the biennium.
2. `discover-hearings` enriches those `hearing` rows with CSI/TVW IDs
   and creates `agenda_item` rows.
3. `ingest-hearings` runs the full pipeline against discovered agenda
   items.
4. `daily-bundles` runs the full pipeline against operator-curated
   entries.

If any pass exits non-zero, cron mail will surface it. The chain
doesn't stop on failure — make's default behavior would be to halt,
but each step's per-bill isolation means most failures are partial,
not blocking.

## Rate limits and politeness

The httpx client (`internal/sources/httpx/client.go`) enforces per-host
token-bucket rate limits. Default is 5 req/sec across all upstream
hosts (`wslwebservices.leg.wa.gov`, `app.leg.wa.gov`, `tvw.org`,
`api.v3.invintus.com`, `data.wa.gov`). Override per CLI invocation
with `--rate=N`.

The User-Agent is
`wa-dd/0.0.1 (https://github.com/nolan-mccafferty/wa-digital-democracy)`
so state-agency operators can identify and contact us if our traffic
is causing problems.

Retries are limited to 2 additional attempts with exponential backoff
on 429/500/502/503/504. No retry on other 4xx — those are usually our
fault (bad params), not the upstream's.

## Time zones

Washington legislative timestamps are Pacific wall-clock without an
explicit zone. Both LWS and TVW publish times this way. The codebase
parses them as `America/Los_Angeles` (handles DST correctly) — see
`internal/sources/lws/normalize.go:parseLWSDate` and
`internal/jobs/discover.go:tvwPostsForDay`. Storing as Pacific-local
time means UI rendering in either Pacific or UTC is unambiguous.

A previous bug stored Pacific wall-clock as UTC, causing meetings to
render as "2:30 AM PST" instead of "10:30 AM PST". That's fixed at the
parse layer and tested.

## Auto-discovery vs operator curation

The two paths produce identical Bundle JSON, but reach the data via
different routes:

| | Auto-discovery (`discover-hearings` + `ingest-hearings`) | Operator curation (`build-bundles` + `selected_bills.yml`) |
|---|---|---|
| Trigger | Every LWS-reported hearing in the biennium | Operator pastes IDs into YAML |
| CSI agenda IDs | Matched against CSI lists by date + bill number | Pasted from `find-candidates` output |
| TVW event ID | Matched against TVW WP archive by date + committee name | Looked up by hand from the TVW website |
| Coverage | ~75% of LWS-reported hearings (Phase 0) | Whatever the operator picks |
| Failure mode | Skip + log; no false positives (sanity checks) | Won't run if YAML invalid; loud at parse time |
| Best for | Bulk coverage of the session | Hand-fixing discovery misses, forcing re-ingest |

## Re-running

All four passes are safe to re-run. What changes:

- `bill`, `legislator`, `bill_sponsor`, `bill_status_change`,
  `hearing`, `agenda_item`, `tvw_event`, `organization` — UPSERT, so
  re-running just refreshes timestamps and any changed fields.
- `testifier`, `transcript_segment`, `org_context_record` — these are
  insert-only with no dedupe. Re-running creates duplicates today.
  In practice the curated path runs against fresh agenda_item rows or
  re-ingests the demo set; neither hits this. Worth fixing eventually,
  but not blocking.
- `source_record` — UPSERT on `(system, endpoint, url, content_hash,
  transform_version)`. Identical responses bump `fetched_at` on the
  same row. Different responses (e.g. status timeline got a new
  entry) create a new row.
- `ingestion_run` — append-only. Each run creates a new row per step.
  Useful for performance tracking over time.

## Where to look when something breaks

- **Per-step error?** `data/processed/_session.json`,
  `_discovery.json`, `_ingest.json`, `bundles/_run.json`. Each entry
  has a `status` and `error` field per bill or hearing.
- **Per-fetch error?** `ingestion_run` table — includes the step name,
  start/finish timestamps, and the error message. Filter by
  `status = 'failed'`.
- **Wrong data on a page?** `source_record` rows for that bill: 
  `SELECT * FROM source_record WHERE source_url LIKE '%HB1501%' ORDER BY fetched_at DESC;`.
  The raw response bytes are at `data/raw/<system>/<hash>` for inspection.
- **Stuck rate limit?** `_session.json`'s per-bill durations. If they
  shoot up by 10x for a stretch, an upstream is throttling.
- **Bundle doesn't render?** Hit
  `http://localhost:8080/api/v1/bills/<biennium>/<slug>/first-page`
  directly. The frontend turns 404 into Next.js 404, but other errors
  are visible in the API response body.

## Out of scope today

- Parallelism across bills. All four passes are serial. At 5 req/sec
  the bottleneck is upstream rate limits, not local CPU; concurrency
  would primarily help if we raise the rate.
- Per-step selective re-fetching. Today every run re-hits every
  upstream API. The `source_record` content_hash dedupe makes this
  cheap on storage, but expensive on bandwidth. Conditional GETs
  (`If-Modified-Since` / `ETag`) aren't supported by the upstreams
  we've checked.
- Real-time / sub-day refresh. TVW captions don't appear until hours
  after a hearing, and CSI sign-ins for tomorrow's hearings don't
  exist yet. Daily is the right cadence given the data sources.
- Speaker attribution beyond the deterministic heuristic — the
  `MatchSpeakers` step currently labels most segments
  `unknown_speaker`. Improving this is a known gap; see the
  Comprehensive Plan's Phase 3 notes.
