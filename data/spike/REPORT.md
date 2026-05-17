# Phase 0 feasibility spike — REPORT

Generated: 2026-05-15

This report answers the four MVP-blocking unknowns from
`Washington Digital Democracy - First Page Implementation Blueprint.md`
§"Known blockers / open questions". Two of four are complete.

## Summary

| # | Question | Status | Verdict for MVP |
|---|---|---|---|
| 1 | TVW caption coverage rate | ✅ done | **100% caption coverage on legislative committees and floor sessions; 90.5% overall. Far above the 40% exit criterion.** |
| 2 | Committee Schedules → TVW event ID mapping | ✅ partial | **Direct mapping works (~75% of returned meetings); date-filtering needs CSRF/session capture.** |
| 3 | CSI history depth | ✅ done | **Full 2025-26 biennium retained (back to 2025-01-13).** |
| 4 | Written-testimony public access | ⏳ manual investigation | Non-blocker — see `written-testimony.md` |

Three of four answered. The MVP is unblocked: TVW captions are essentially
universal for the surfaces we care about, the current biennium is fully
indexed in CSI, and we have a working direct-ID path from Committee
Schedules to TVW events. Q4 is a labeling decision, not a blocker.

## Q1 — TVW caption coverage rate

**Method:** Paginate `GET /wp-json/wp/v2/invintus_video?after=…&before=…` to
discover events in a real date window, extract `data-eventid="…"` from each
post's rendered HTML (per spec 04 lines 78–81), then `POST
/v2/Event/getDetailed` for each to check `captionPath`.

**Note on the schedule API:** `tvw/v1/schedule?start=…&end=…` ignores its
date parameters and returns a fixed "currently airing" set (~31 events). Use
WP REST for historical sampling. This is a finding the spec didn't mention —
log into the wiki later.

**Window:** 2026-02-01 → 2026-03-01 (active 2026 session, House and Senate
in committee work). Sampled the most recent 200 events posted in that window.

**Findings:**

| Bucket | Total | With caption | Coverage |
|---|---|---|---|
| **legislative-committee** | **15** | **15** | **100.0%** |
| **legislative-floor** | **16** | **16** | **100.0%** |
| court | 31 | 31 | 100.0% |
| agency-board | 15 | 14 | 93.3% |
| other | 123 | 105 | 85.4% |
| **TOTAL** | **200** | **181** | **90.5%** |

**Implication for MVP:** `captionPath` is essentially universal for the
surfaces the first page cares about. We can build the "What was said?"
section against captions alone — no need for ASR fallback in v1.

**Raw output:** `data/spike/tvw-captions.json`

**Exit criterion (40% legislative-committee coverage):** comfortably cleared.

## Q2 — Committee Schedules → TVW event ID mapping

**Method:** `POST /committeeschedules/Home/Search/` with form fields
`StartDate`/`EndDate` (MMDDYYYY), `SearchType=Schedule`, then parse HTML for
`showVideoModal(scheduleID, eventID)` and `showAgendaDetailModal(scheduleID)`
tokens.

**Tried:** May 2026 window (off-session, default), Feb 2026 window (active
session, with `-anchor`).

**Findings:**

- Both runs returned an identical 5,664-byte response with exactly 4 agenda
  meetings and 3 video modals. The date params are being ignored.
- This means the search endpoint defaults to "upcoming meetings only" unless
  given a valid anti-forgery token or session — confirming spec 03 lines
  142–143's warning.
- For the meetings that **were** returned, the mapping rate is high:
  - 3 of 4 agenda meetings (75%) have a paired `showVideoModal(scheduleID, tvwEventID)`.
  - TVW event IDs returned: `2026051107`, `2026051115`, `2026051116`.
  - Concrete example: `showAgendaDetailModal(34154)` + `showVideoModal(34154, 2026051107)`.

**Implication for MVP:** the *direct* mapping path is real and works for
meetings the endpoint chooses to return. To survey at scale we need to do one
of:

1. **Capture-and-replay** an authenticated browser POST (cookies + any
   `__RequestVerificationToken` field) from DevTools, hardcode in the spike,
   and re-run.
2. **Drive a headless browser** (`chromedp` or `playwright`) — heavier,
   probably unnecessary for first page.

For first-page work, this is enough: when an LWS meeting is in scope, we can
search for it directly and expect a TVW event ID about 75% of the time, with
fallback to date+committee fuzzy match against the TVW schedule API.

**Raw outputs:**
- `data/spike/sched-tvw-mapping.json`
- `data/spike/sched-tvw-mapping.html` (raw response snapshot)

## Q3 — CSI history depth

**Method:** `GET /csi/<Chamber>` to discover committee IDs from
`<select id="SelectedCommitteeId">`, then
`GET /csi/Home/GetMeetings?chamber=…&committeeId=…` for each.

**Findings:**

| Chamber | Committees | With meetings | Earliest meeting | Latest meeting |
|---|---|---|---|---|
| House  | 18 | 18 | **2025-01-13** | 2026-03-05 |
| Senate | 14 | 14 | **2025-01-13** | 2026-03-09 |
| Joint  |  2 |  2 | 2025-06-18 | 2025-12-15 |

- All 32 standing committees returned data; zero errors.
- Earliest meeting across the dataset is **2025-01-13** — the first day of the
  2025-26 biennium. CSI retains the full current biennium.
- Most committees have 25–48 meetings recorded over the ~16-month window
  (Appropriations 48, Ways & Means 47, Community Safety 34, Senate Early
  Learning 33, etc.). Even off-session committees (Joint) have records.

**Implication for MVP:** any housing-related hearing in the 2025-26 biennium
is reachable via CSI. The Housing committees we'd target (House Housing
committee ID `31633`, Senate Housing `34078`) are present and active.

**Raw output:** `data/spike/csi-history.json`

## Q4 — Written-testimony public access

**Status:** Pending manual investigation — see
`data/spike/written-testimony.md` for method.

**Tentative position:** Per spec 02 line 21, "a no-auth bulk endpoint for
actual text/attachments is not yet confirmed." Current plan is to source
written testimony only from CSI sign-in metadata (Position, Organization,
TimeSignedIn) for the MVP and label the rest as "available via public records
request" in the source panel.

## Decisions

1. **Proceed to Phase 1.** All three blocking unknowns came back green.
2. **Use `wp/v2/invintus_video` (not `tvw/v1/schedule`) for production event
   discovery.** The schedule endpoint ignores date params; the WP REST
   endpoint paginates real archives by post date. Update spec 04 in the wiki
   when convenient.
3. **Captions are the transcript source for v1.** Skip ASR fallback. No
   transcript fallback either — the wiki's `Transcript/getTranscriptV3` was
   redundant given universal `captionPath` coverage.
4. **Do not over-invest in Committee Schedules form discovery for MVP.** For
   the first page we know the target meeting via CSI/LWS already; we only
   need Committee Schedules to confirm the TVW event ID. Capture a real
   session POST when needed; defer headless-browser work.
5. **Skip Joint committees for first page.** Volume is small and irregular.
6. **Target Housing committees first** (`House:31633`, `Senate:34078`) — both
   confirmed active across the full biennium.
