# Data ingestion

Last updated: 2026-06-03.

This document describes how data flows from the official Washington
state sources (LWS, CSI, TVW/Invintus, PDC, DataWA) into Postgres and out to
the Next.js frontend. It's the operator's reference for what runs when,
where things land, and how to debug a stuck or misbehaving run.

## TL;DR

```
hosted daily: wa-dd daily

ingest-legislators
  → ingest-session
      LWS bill metadata + LWS hearing rows + CSI/TVW hearing discovery
  → ingest-irs-bmf-wa
  → ingest-pdc-employers
      source context available before testimony organization seeding
  → ingest-hearings
      CSI testifiers + Invintus media + diarization + transcript windows
      + hearing-scoped organization population
  → verify-organizations
      confirms existing organizations against fresh IRS/PDC refs
  → generate-vendor-entity-matches
      creates reviewable source/org match candidates

  → Postgres (truth)
  → wa-dd-api at :8080  (HTTP / JSON)
  → Next.js frontend at :3000
```

The public API returns route-specific page objects assembled from Postgres. Postgres plus `wa-dd-api` is the only page-data flow.

## Legislative daily passes

Each pass writes to Postgres directly. Each is idempotent: re-running uses
stable source IDs plus `ON CONFLICT DO UPDATE` / `DO NOTHING` on domain rows.

### 1. `wa-dd ingest-session --biennium 2025-26`

**What it does:** pulls every bill in the biennium from LWS, stores
metadata + sponsors + status timeline + LWS-reported hearing references, then
discovers CSI agenda IDs and TVW event IDs for the hearing rows it created.

**Why:** establishes the bill universe. Most bills don't have
hearings, but every bill should have a metadata page (sponsors, status,
title, biennium). The discovery post-pass needs those `hearing` rows before it
can attach CSI/TVW join IDs.

**Implementation:**

- Calls `LegislationService.GetLegislationByYear(year)` for both years
  of the biennium (~3,300 + ~3,900 bills, deduped by `(biennium, bill_id)`).
- For each bill, calls `Pipeline.RunMetadataOnly` which runs only the
  `IngestBill` step.
- `IngestBill` makes 4 LWS SOAP calls per bill:
  - `GetLegislation` → `bill` row + current status snapshot
  - `GetSponsors` → `bill_sponsor` rows, resolved against the roster
    loaded by `ingest-legislators`; missing sponsor IDs are warned and
    skipped
  - `GetLegislativeStatusChangesByBillNumber` → `bill_status_change` rows
  - `GetHearings` → `hearing` rows (one per LWS-reported hearing)
- Per-bill failure isolation; one bad bill doesn't poison the run.
- After the LWS metadata pass succeeds, runs hearing discovery for every
  undiscovered hearing in the biennium.

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
- A summary at `data/processed/_discovery.json` with per-hearing discovery
  statuses.

**Cost:** ~5,000 bills × 4 LWS calls = 20,000 calls. The default driver
uses concurrent workers while the shared HTTP client enforces the LWS
host rate limit, so runtime depends on upstream latency and the selected
`--workers` / `--rate` values. The hosted `wa-dd daily` wrapper defaults
session metadata to `--session-workers=4` and `--session-rate=50`.

**Resume / smoke-test flags:**

- `--skip-fresh=24h` is the default: bills whose `bill.updated_at` is newer
  than the window are skipped so a killed run can resume without re-fetching
  already-ingested bill metadata. Use `--skip-fresh=0` for a forced full pass.
- `--only-types=HB,SB` narrows the bill-prefix set for targeted checks.
- `--limit=N` stops after N bills for smoke tests.

**Code:** `cmd/wa-dd/cmd_ingest_session.go` (`runIngestSession`),
`cmd/wa-dd/cmd_discovery.go` (`runHearingDiscovery`),
`internal/jobs/jobs.go` (`Pipeline.RunMetadataOnly`),
`internal/jobs/ingest_bill.go` (`IngestBill`), and
`internal/jobs/discover.go` (`Discoverer`).

### 2. Hearing discovery inside `wa-dd ingest-session`

After LWS metadata ingestion finishes successfully, `ingest-session` discovers
every LWS hearing in the DB whose CSI/TVW IDs are still blank. It walks four
lookups to fill them in:

1. **CSI committee** — given LWS `(chamber, committee_name)` like
   `("Senate", "Senate Housing")`, strip the chamber prefix and match
   against `csi.Client.ListCommittees(chamber)` by normalized name.
2. **CSI meeting** — given the CSI committee ID and an LWS
   `meeting_datetime`, scan `csi.Client.ListMeetings(chamber, committee_id)`
   for a meeting whose `StartDateTime` is within ±15 minutes. Pick the
   closest.
3. **CSI agenda item** — given the meeting, list its agenda items and
   match by leading bill number in the label
   (regex covers HB/SB/HJR/SJR/HCR/SCR/HJM/SJM with optional engrossed /
   substitute chrome, plus SGA; first capture group is the number).
4. **TVW event** — fetch the TVW WP archive
   (`/wp/v2/invintus_video?after=...&before=...`) for the meeting's
   day. For each post, sanity-check that the title contains the LWS
   committee name (case-insensitive) and the date is within ±2 hours.
   Extract the event ID via `EventIDFromWPPost` (regex on
   `data-eventid="\d+"` in the rendered HTML content).

The CSI side is required — if any of steps 1–3 fail, the hearing is
skipped. The TVW side is best-effort during discovery — a hearing with
CSI agenda metadata but no matched TVW event is still useful, but the full
`ingest-hearings` pipeline only runs hearings that have both CSI agenda
item IDs and a TVW event ID.

**Caching policy:** the discoverer caches CSI directory data and the
TVW WP archive per process. Scope of caching:

| Cache | Key | Typical size per nightly run |
|---|---|---|
| `committees` | chamber | 2 entries (House, Senate) |
| `meetings` | committee_id | ~50 entries (committees with hearings) |
| `agendaItems` | meeting_family_id | hundreds (one per unique CSI meeting) |
| `tvwPostsByDay` | YYYY-MM-DD | ~100 days during session |

The wall-clock cost is dominated by `ListAgendaItems` — typically one HTTP call
per unique meeting after the run-scoped caches warm up.

**Sanity checks:**

- **Committee-name match for TVW**: rejects matches where the WP post
  title doesn't contain the LWS committee name. Without this we'd
  occasionally attach the wrong video to a hearing (e.g. a press
  conference happening in the same chamber on the same day).
- **±15 min CSI meeting window**: tighter than the ±2 hour TVW window
  because CSI publishes meeting times to the minute.
- **Bill-number regex constrained to ingested legislative prefixes**
  (HB/SB/HJR/SJR/HCR/SCR/HJM/SJM with optional E/S/2S/3S chrome, plus
  SGA): rejects unrelated numeric labels while still matching the
  bill-like rows LWS stores in Postgres.
- **SGA discovery skip:** gubernatorial appointments are kept as
  metadata rows, but discovery does not process their hearings
  because CSI does not expose SGA appointments as testimony agenda items
  in the sign-in data this pipeline ingests.

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
  duration, status (`ok` / `no-tvw` / `no-csi-meeting` /
  `no-agenda-item` / `no-committee` / `failed`), and error.
  The `no-*` statuses are expected source mismatches and do not make
  the command exit nonzero; `failed` is reserved for operational errors
  such as unexpected upstream, parser, or database failures.

**Cost:** dominated by `ListAgendaItems` — typically one call per
unique CSI meeting in the biennium. Empirically ~few minutes for a
full biennium run after the first batch warms the caches.

**Code:** `cmd/wa-dd/cmd_discovery.go` (`runHearingDiscovery`),
`internal/jobs/discover.go` (`Discoverer`).

### 3. `wa-dd ingest-hearings --biennium 2025-26`

**What it does:** for every discovered hearing whose rows have CSI agenda-item
IDs and a TVW event ID, runs the full hearing pipeline. The hearing is the unit
of work because TVW media and diarization are event-level, while CSI testifiers
and bill-window segmentation are agenda-item-level.

**Retry selection:** `Store.ListDiscoveredHearingsForIngest` selects discovered
hearings whose hearing-level pipeline has not completed. The terminal marker is
a succeeded `ingestion_run.job = 'hearing-pipeline'` row tagged with the
`hearing_id`. If a hearing fails after partially inserting testifiers, TVW media,
diarization rows, or transcript windows, the next run retries the whole hearing.

**Why:** discovery populated the join keys; this pass uses them to fetch the
actual testimony, media assets, diarized transcript, bill-discussion windows,
and organization links.

**Implementation:**

- Reads `(hearing_id, tvw_event_id)` rows via
  `Store.ListDiscoveredHearingsForIngest`.
- Loads every CSI agenda item attached to the hearing via
  `Store.ListAgendaItemsForHearing`.
- For each agenda item, fetches CSI testifiers via `IngestCSI`.
- Fetches rich Invintus event detail and media assets once for the hearing's
  TVW event via `IngestTVW` (requires `INVINTUS_EMBEDDER_KEY`). The TVW
  WordPress API is used during discovery to resolve the event ID; hearing ingest
  does not re-search WordPress. Invintus `captionPath` is recorded on
  `tvw_event`, but VTT text is not fetched for the transcript path.
- Ensures one succeeded diarization job exists for that TVW event before
  transcript segmentation. The command uses an event-level Postgres advisory
  lock plus `--diarization-concurrency` so parallel hearing workers do not
  submit duplicate provider jobs for the same recording.
- For each agenda item, runs `SegmentTranscript` against the latest succeeded
  diarized transcript and writes `agenda_item_window` rows.
- Runs hearing-scoped organization population for CSI `raw_organization`
  strings. PDC/vendor/federal context flows through separate source-context
  ingestion plus reviewable entity matching.

Bill metadata is not refreshed here. `ingest-session` owns `bill`,
`bill_sponsor`, `bill_status_change`, and the initial LWS `hearing` rows.

**Output:**

- Per hearing: TVW event/media rows, one succeeded diarization job when needed,
  CSI testifier rows per agenda item, `agenda_item_window` rows per bill, and
  organization links where CSI org strings are present.
- A summary at `data/processed/_ingest.json` with per-hearing durations and any
  failures.

**Provider defaults and cost:** CSI and Invintus requests are short; provider
diarization dominates fresh hearings. `ingest-hearings` defaults to pyannoteAI
`precision-2` with bundled transcription. Use `--provider deepgram` for
Deepgram, `--workers` for hearing-level parallelism, and
`--diarization-concurrency` to cap provider jobs; the default cap is 5. Use
`--limit=1` for a single-hearing smoke test, or `make ingest-hearings LIMIT=1`
through the Make wrapper.
PDC/IRS source-context runs are part of daily before hearing ingest. DataWA,
FiscalWA, and Federal source-context commands remain separate bounded backfills
with their own costs.

**Code:** `cmd/wa-dd/cmd_ingest_hearings.go` (`runIngestHearings`),
`internal/jobs/ingest_csi.go`, `internal/jobs/ingest_tvw.go`,
`internal/jobs/segment_transcript.go`,
and `internal/jobs/populate_organizations.go`.

## Hearing diarization and speaker review

See [`docs/hearing-diarization-and-review.md`](hearing-diarization-and-review.md)
for the Deepgram diarization flow, speaker evidence extraction, admin review
workflow, and public-display safety rules for reviewed speaker labels.

## Organization matching and entity review

See [`docs/organization-matching-and-review.md`](organization-matching-and-review.md)
for CSI organization seeding, public-record source-context matching, Deepgram
organization mentions, entity-review decisions, and public-display safety rules
for reviewed organization context.

## Provenance

Public facts keep source-specific identifiers, official URLs, raw-field JSON,
and normalization warnings where those are useful for review. There is no global
fetch ledger; source-specific rows are the provenance boundary.

The shared `httpx` client handles retry/backoff and returns response metadata
plus body bytes to source-specific parsers.

## Daily orchestration

The hosted operator path is the CLI command:

```sh
wa-dd daily --biennium "${BIENNIUM:-2025-26}"
```

Railway runs that command from `infra/railway/config/daily.railway.json` and
`scripts/bootstrap-railway.mjs`. The command acquires a Postgres advisory lock
for the whole chain; if another daily run is active, the new run exits cleanly.

`make daily` remains a local convenience target and chains the same public
operator work through Make:

```makefile
daily: ingest-legislators ingest-session ingest-irs-bmf-wa ingest-pdc-employers ingest-pdc-lobbyist-compensation ingest-hearings verify-organizations generate-vendor-entity-matches
```

The order matters when fresh:

1. `ingest-legislators` refreshes the roster and owns `legislator`
   rows.
2. `ingest-session` creates `bill`, `bill_sponsor`, status, and
   `hearing` rows for every bill in the biennium. In the hosted `daily`
   command this stage uses `--session-workers`, `--session-rate`, and
   optional `--session-limit`.
3. `ingest-session` also enriches those `hearing` rows with CSI/TVW IDs
   and creates `agenda_item` rows.
4. `ingest-irs-bmf-wa` and `ingest-pdc-employers` refresh source-context
   reference tables before testimony organizations are seeded. That lets
   hearing ingestion confirm CSI organizations during population instead of
   leaving them as `possible` until a later verification pass.
5. `ingest-pdc-lobbyist-compensation` ingests PDC compensation data and creates
   lobbying firm↔client affiliation edges. It runs after `ingest-pdc-employers`
   so that filer and employer organizations already exist.
6. `ingest-hearings` runs the full pipeline against discovered hearings:
   CSI testifiers per agenda item, Invintus event/media metadata once per event,
   diarization once per event, and transcript segmentation per agenda item.
   `--hearing-limit` applies to hearing ingest in `wa-dd daily` for smoke tests.
7. `verify-organizations` re-checks existing unconfirmed organizations against
   the fresh IRS/PDC reference tables, catching older rows that were not touched
   by the current hearing ingest.
8. `generate-vendor-entity-matches` generates reviewable source/org match
   candidates from the verified organization set and the latest source-context
   rows. Unique high-confidence matches can be auto-confirmed.

For an old-school local cron, one line still works:

```cron
30 3 * * * cd ~/workspace/wa-digital-democracy && INVINTUS_EMBEDDER_KEY=… PYANNOTEAI_API_KEY=… make daily >> /tmp/wa-dd-daily.log 2>&1
```

If any pass exits non-zero, cron mail / Railway logs will surface it. Each
step's per-bill or per-hearing isolation means most failures are partial, not
blocking.

## Rate limits and politeness

The httpx client (`internal/sources/httpx/client.go`) enforces per-host
token-bucket rate limits. Default is 10 req/sec across all upstream
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

## Re-running

All daily stages are safe to re-run. What changes:

- `bill`, `legislator`, `bill_sponsor`, `bill_status_change`,
  `hearing`, `agenda_item`, `tvw_event`, `organization` — UPSERT, so
  re-running just refreshes timestamps and any changed fields.
- `testifier` and `agenda_item_window` — replaced per agenda item, so
  hearing retries refresh those rows. `diarized_speech_segment` is appended
  per diarization job, but hearing ingestion first checks for an existing
  succeeded job and uses an event-level advisory lock to avoid duplicate
  provider submissions. The work-list query treats succeeded
  `hearing-pipeline` rows keyed by `hearing_id` as the current terminal marker.
- source rows — identified by stable source-specific IDs and updated through idempotent upserts. Identical responses bump `fetched_at` on the
  same row. Different responses (e.g. status timeline got a new
  entry) create a new row.
- `ingestion_run` — append-only. Each run creates a new row per step.
  Useful for performance tracking over time.

## Where to look when something breaks

- **Per-step error?** `data/processed/_session.json`,
  `_discovery.json`, `_ingest.json`. `ingest-session` writes both the session
  and discovery summaries; each entry
  has a `status` and `error` field per bill or hearing.
- **Per-fetch error?** `ingestion_run` table — includes the step name,
  start/finish timestamps, and the error message. Filter by
  `status = 'failed'`.
- **Wrong data on a page?** Inspect the relevant normalized rows and their official URL/source-ID fields, then rerun the source-specific ingest command for a fresh pull.
- **Stuck rate limit?** `_session.json`'s per-bill durations. If they
  shoot up by 10x for a stretch, an upstream is throttling.
- **Bill page doesn't render?** Hit
  `http://localhost:8080/api/v1/bills/<biennium>/<slug>/page`
  directly. The frontend turns 404 into Next.js 404, but other errors
  are visible in the API response body.

## Out of scope today

- Fine-grained parallelism inside a single hearing. `ingest-hearings` already
  runs hearings concurrently with `--workers`; inside each hearing, agenda-item
  CSI and segmentation steps stay ordered so they can share the event-level TVW
  and diarization result.
- Per-step selective re-fetching. Today every run re-hits every
  upstream API. Idempotent upserts make this
  cheap on storage, but expensive on bandwidth. Conditional GETs
  (`If-Modified-Since` / `ETag`) aren't supported by the upstreams
  we've checked.
- Real-time / sub-day refresh. TVW recordings and downloadable media can lag
  the hearing, and CSI sign-ins for tomorrow's hearings don't exist yet. Daily
  is the right cadence given the data sources.
- Broad accountability-graph expansion. The current public-beta product is
  complete around legislative/testimony pages; DataWA/FiscalWA/Federal
  context commands are bounded source-context tools, not a Phase 4 roadmap.

## Source-context: PDC and IRS organization verification

`wa-dd ingest-pdc-employers` ingests PDC lobbyist-employer registrations from
DataWA/Socrata (`xhn7-64im`) into `pdc_employer`, `person`, and
`person_organization_affiliation`. It runs before `ingest-hearings` in
`wa-dd daily` so CSI organization population can verify against fresh PDC
employer rows. PDC affiliation rows ingested before the corresponding canonical
organization exists are attached when the organization is later seeded or
verified.

`wa-dd ingest-pdc-lobbyist-compensation` ingests PDC lobbyist compensation data from
DataWA/Socrata (`9nnw-c693`) into `pdc_lobbyist_compensation` and creates
`person_organization_affiliation` edges that expose firm↔client relationships.
This dataset reveals how much lobbying firms are paid by their clients per
filing period, bridging the gap between lobbyist employment (who works for whom)
and lobbyist compensation (who pays whom). It runs after `ingest-pdc-employers`
in the daily chain so that employer/filer organizations already exist before
creating compensation-based affiliations.

The compensation dataset creates "paid_by" affiliation edges linking filer
organizations (lobbying firms) to employer organizations (clients). This enables
queries like "which organizations pay Cascadia Public Affairs for lobbying" and
"how much does Washington State Hospital Association spend on lobbying per quarter."

`wa-dd ingest-irs-bmf-wa` ingests the IRS Business Master File Washington
501(c) extract into `irs_bmf_organization`:

```sh
wa-dd ingest-irs-bmf-wa
```

`wa-dd verify-organizations` cross-matches seeded CSI organizations against
IRS BMF and PDC employer rows. Treat these as source-backed evidence for
review/verification, not automatic proof that two real-world entities are the
same in every context.

The command verifies existing `organization` rows; PDC/IRS ingests do not create
canonical organizations by themselves. IRS BMF is checked first and wins when
there is exactly one normalized-name match. PDC employers are checked only when
IRS has no unique match. Ambiguous or missing matches leave the organization at
its existing confidence, usually `possible`, and the public organization list
continues to hide it until it is confirmed by verification or review.

Use `wa-dd sources` to list registered source connectors and their base URLs.
The registry includes both shipped legislative/testimony connectors and broader
source-context clients (Socrata/DataWA, PDC, IRS-adjacent verification data,
USAspending, FEMA, BLS, HUD, EPA, Census, SAO, Seattle, King County, etc.).
Not every registered connector has a first-class ingest command yet; commands
listed in this document are the operator-supported ingestion surfaces.

## Optional source-context: DataWA contract and vendor ingestion

`wa-dd ingest-contracts` ingests DataWA agency-contract fiscal-year datasets into
`datawa_contract` with stable source dataset/row IDs.

```sh
wa-dd ingest-contracts --fiscal-year 2025 --limit 1000
```

Current fiscal-year dataset mapping lives in `internal/sources/datawa`:

- 2025 → `6fx9-ncas` — Agency Contracts Fiscal Year 2025
- 2024 → `s8d5-pj78` — Agency Contracts Fiscal Year 2024
- 2023 → `mz6y-pfem` — Agency Contracts Fiscal Year 2023
- 2022 → `pwse-3zea` — Agency Contracts Fiscal Year 2022

Rows are normalized into `datawa_contract` and keep:

- source dataset/row IDs;
- fiscal year;
- agency and contractor names;
- contract/amendment/vendor identifiers;
- dates and money fields;
- raw fields as JSONB;
- normalization warnings for sentinel/invalid dates;
- stable source dataset and row IDs for matching/upserts.

`wa-dd ingest-master-contract-sales` ingests DataWA statewide/master-contract
sales into `datawa_master_contract_sale`:

```sh
wa-dd ingest-master-contract-sales --limit 1000
```

The source dataset is:

- `n8q6-4twj` — Statewide Contract / Master Contract Sales Data by Customer, Contract, Vendor

Rows preserve customer type/name, contract number/title, vendor name, report
year, quarterly and total sales, OMWBE/veteran/small/diverse-business flags,
raw fields and normalization warnings.

`wa-dd ingest-it-contracts` ingests annual DataWA IT Contracts Report datasets
into `datawa_it_contract`:

```sh
wa-dd ingest-it-contracts --fiscal-year 2025 --limit 1000
```

Current fiscal-year dataset mapping lives in `internal/sources/datawa`:

- 2025 → `3txe-z9i9` — IT Contracts Report 2025
- 2024 → `ktim-amuz` — IT Contracts Report 2024
- 2023 → `hycx-v82h` — IT Contracts Report 2023
- 2022 → `dzvi-rs2c` — IT Contracts Report 2022

Rows preserve agency, contract number, contractor/DBA, cooperative purchase
fields, contract dates, IT tower percentages, fiscal-year amount columns,
total contract amount, raw fields, normalization warnings, and source
provenance. Monthly WaTech spend datasets are a separate grain and should be
handled by a follow-up issue rather than forced into the contracts table.

`wa-dd ingest-webs-vendors` ingests WEBS vendor/procurement entity rows into
`datawa_webs_vendor`:

```sh
wa-dd ingest-webs-vendors --limit 1000
```

The source dataset is:

- `3kwi-7zsj` — WEBS Vendors by commodity code and MWBE/V/Small status

Rows preserve company/DBA names, normalized company names for candidate joins,
phone/email/city/state/web fields, commodity code and description, small-
business/veteran/other certification flags, raw fields, normalization warnings,
and source provenance.

Matching strategy: use `normalized_company_name` only to generate reviewable
candidate joins against contract contractor/vendor names and PDC/lobbying
organization names. Do not automatically merge or display a match as confirmed
without a confidence/evidence layer or human-reviewed decision.

This remains intentionally bounded. Broader budget/spending work should stay
out of the core legislative/testimony beta unless Nolan explicitly reopens that
scope.

## Optional source-context: USAspending ingestion

`wa-dd ingest-usaspending-wa-awards` ingests a scoped USAspending award-search
page into `federal_award`:

```sh
wa-dd ingest-usaspending-wa-awards --start-date 2025-10-01 --end-date 2026-09-30 --limit 100
```

The current bounded scope is awards whose place of performance is Washington
(`WA`) for the requested date range, sorted by award amount through
USAspending's `/api/v2/search/spending_by_award/` endpoint.

Rows preserve award ID, recipient name/UEI, awarding and funding agencies, award
type, amount, start/end dates, place-of-performance state/county, raw award JSON,
and stable source row identifiers.

Matching strategy: recipient names and UEIs generate reviewable candidate joins
against organization/context records. Do not present uncertain joins as
confirmed without a confidence/evidence layer or human-reviewed decision.

Scope caveat: this command ingests one bounded award-search page. Pagination,
recipient-specific backfills, agency/account-level data, subawards, and richer
entity-resolution workflows are outside the current completed beta scope.

## Optional source-context: Seattle Open Budget ingestion

`wa-dd ingest-seattle-operating-budget` ingests the City of Seattle Operating
Budget Socrata dataset into `seattle_operating_budget`:

```sh
wa-dd ingest-seattle-operating-budget --limit 1000
```

The current bounded source is:

- `8u2j-imqx` — City of Seattle Operating Budget (`data.seattle.gov`), public-domain licensed and attributed to the City of Seattle in Socrata metadata.

Rows preserve fiscal year, service, department, program, fund, fund type,
expense type, description, approved amount, and raw fields
provenance linking back to the fetched Socrata page. This gives optional
Seattle context work a department/program/fiscal-period budget table without
making Seattle accountability expansion part of the current plan.

Scope caveat: this is the operating-budget surface only. Seattle capital budget
(`m6va-m4qe`), actual expenditures, project-level spending, and Open Budget site
visualization metadata are outside the current completed beta scope because
they have different grains and columns.

## Optional source-context: fiscal.wa.gov spending ingestion

`wa-dd ingest-fiscal-vendor-payments` ingests the current fiscal.wa.gov Open
Checkbook vendor-payment workbook into `fiscalwa_vendor_payment`:

```sh
wa-dd ingest-fiscal-vendor-payments --limit 1000
```

The current bounded source is:

- `https://fiscal.wa.gov/Spending/VendorPayments2527.xlsx` — Open Checkbook vendor payments for the 2025-27 biennium

Rows preserve biennium, fiscal year/month, agency number/name, object and
subobject budget categories, vendor name, amount, raw fields, and
stable source row IDs derived from the fetched workbook.

Scope caveat: this is a spending/checkbook slice, not the full state budget.
It supports agency/vendor/category spending context. Proposal-level operating,
capital, transportation, LEAP document, revenue, allotment, and OFM budget book
ingestion is outside the current completed beta scope because those surfaces
have different grains and source formats.

Source/terms caveat: fiscal.wa.gov describes itself as a transparency site for
state fiscal data, reports, charts, and maps. Preserve official source links and
fetch timestamps; show the project as unofficial and source-linked.
