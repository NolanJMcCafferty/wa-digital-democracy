# Seattle Council video ingestion plan

Last updated: 2026-06-02.

This document captures a future ingestion path for City of Seattle Council and
committee meetings. The goal is to combine Seattle's structured Legistar data
with Seattle Channel video/caption pages, then add Digital Democracy-style
item-level transcript windows and searchable civic pages.

## Summary

Seattle has enough public source material to support a useful local-government
video/transcript pipeline:

- **Legistar Web API** provides structured meetings, committees, agenda items,
  legislation/matters, actions, agenda PDFs, minutes PDFs, and meeting-level
  Seattle Channel links.
- **Seattle Channel** provides archived meeting video pages with direct MP4
  sources, downloadable SRT captions, agenda-summary text, and coarse timestamp
  links such as “Public Comment - 3:31”.

The raw sources are strong, but they do not appear to provide a complete
productized item-level transcript graph. In particular, the sample checked had
`EventItemVideo` and `EventItemVideoIndex` as `null` for agenda items even
though the meeting had a Seattle Channel video with visible timestamp links.
The value-add is to join these sources, infer item windows, slice captions by
item, and expose searchable per-item transcript pages.

## Source map

### Legistar Web API

Base API:

```text
https://webapi.legistar.com/v1/seattle
```

API help:

```text
https://webapi.legistar.com/Help
```

Useful endpoints:

```text
GET /v1/seattle/events
GET /v1/seattle/events/{EventId}
GET /v1/seattle/events/{EventId}/EventItems?AgendaNote=1&MinutesNote=1&Attachments=1
GET /v1/seattle/matters
GET /v1/seattle/matters/{MatterId}
GET /v1/seattle/matters/{MatterId}/Histories?AgendaNote=1&MinutesNote=1
GET /v1/seattle/matters/{MatterId}/Attachments
GET /v1/seattle/bodies
```

The API supports OData-style query parameters. Example verified during research:

```text
https://webapi.legistar.com/v1/seattle/events?$filter=EventDate eq datetime'2026-05-29'
```

Sample event fields observed:

- `EventId`
- `EventGuid`
- `EventLastModifiedUtc`
- `EventBodyId`
- `EventBodyName`
- `EventDate`
- `EventTime`
- `EventVideoStatus`
- `EventAgendaStatusName`
- `EventMinutesStatusName`
- `EventLocation`
- `EventAgendaFile`
- `EventMinutesFile`
- `EventComment`
- `EventMedia` — meeting-level Seattle Channel URL when available
- `EventInSiteURL` — Legistar meeting detail page

Sample event-item fields observed:

- `EventItemId`
- `EventItemGuid`
- `EventItemEventId`
- `EventItemAgendaSequence`
- `EventItemMinutesSequence`
- `EventItemAgendaNumber`
- `EventItemTitle`
- `EventItemAgendaNote`
- `EventItemMinutesNote`
- `EventItemActionName`
- `EventItemActionText`
- `EventItemPassedFlagName`
- `EventItemMatterId`
- `EventItemMatterGuid`
- `EventItemMatterFile` — e.g. `CB 121202`, `Appt 03497`, `Inf 2899`
- `EventItemMatterName`
- `EventItemMatterType`
- `EventItemMatterStatus`
- `EventItemMatterAttachments`
- `EventItemVideo`
- `EventItemVideoIndex`

Caveat: in the sample meeting checked, `EventItemVideo` and
`EventItemVideoIndex` were `null`, so the pipeline should not rely on Legistar
for agenda-item video offsets.

### Seattle Channel

Main archive page:

```text
https://www.seattlechannel.org/CityCouncil
```

Seattle Channel organizes videos by Full Council, Council Briefings, special
committees, current committees, and historical committee terms.

Example video page checked:

```text
https://www.seattlechannel.org/videos?videoid=x187514
```

That page represented:

```text
Human Services, Labor, and Economic Development Committee - Special Meeting 5/29/2026
```

Useful fields visible in page text/source:

- title
- date
- duration
- agenda summary
- Seattle Channel internal media number, e.g. `2852618`
- “Advance to a specific part” timestamp links
- SRT caption link
- JWPlayer source config with direct MP4 URL

Example direct assets observed in page source:

```text
//video.seattle.gov/media/council/human_052926_2852618.mp4
documents/seattlechannel/closedcaption/2026/human_052926_2852618.srt
```

The timestamp index appears in the page as links such as:

```text
Public Comment - 3:31
Appt 03497: Appointment of Sandra J. Valenciano as Director of Public Health - 8:55
Overview of Resolution Updating Policies for Establishing and Managing Parking and Business Improvement Areas (BIAs) - 49:27
King County Crisis Care Center Levy Implementation Update - 1:08:41
```

This index is useful but coarse: it gives item starts, not explicit end times,
caption slices, speaker labels, or structured item-to-Legistar mappings.

### Live video

Seattle Channel live page:

```text
https://www.seattlechannel.org/watch-live
```

The page points viewers to the Seattle Channel YouTube channel for council
meetings and select live events. Treat YouTube/live video as a fallback or
future near-real-time path; the archived Seattle Channel pages are the better
canonical source for repeatable ingestion.

## Proposed connector architecture

### 1. `legistar` connector

Responsibilities:

- List events with filters, ordering, and paging.
- Fetch a single event by `EventId`.
- Fetch event items with agenda/minutes notes and attachments.
- Fetch matters/legislation.
- Fetch matter histories and attachments.
- Preserve raw JSON responses for provenance.

Suggested methods:

```go
ListEvents(ctx, params) ([]Event, error)
GetEvent(ctx, eventID int64) (*Event, error)
ListEventItems(ctx, eventID int64, opts EventItemOptions) ([]EventItem, error)
ListMatters(ctx, params) ([]Matter, error)
GetMatter(ctx, matterID int64) (*Matter, error)
ListMatterHistories(ctx, matterID int64, opts HistoryOptions) ([]MatterHistory, error)
ListMatterAttachments(ctx, matterID int64) ([]MatterAttachment, error)
```

Source-of-truth role:

- meeting identity and date/time/location;
- committee/body identity;
- agenda and minutes status;
- agenda/minutes PDF URLs;
- agenda item sequence and titles;
- legislation/matter IDs, files, types, statuses;
- actions and results.

### 2. `seattlechannel` connector

Responsibilities:

- Fetch video page by URL or `videoid`.
- Parse video page metadata.
- Extract direct MP4 URL from JWPlayer config.
- Extract SRT caption URL from tracks or description link.
- Extract agenda timestamp index from `.seekItem` links / page text.
- Fetch SRT captions.
- Preserve raw HTML and SRT responses for provenance.

Suggested methods:

```go
ParseVideoID(rawURL string) (string, error)
FetchVideoPage(ctx, videoID string) (*VideoPage, error)
FetchCaptions(ctx, captionURL string) ([]CaptionCue, error)
```

Suggested parsed model:

```go
type VideoPage struct {
    VideoID       string
    URL           string
    Title         string
    Date          time.Time
    Duration      time.Duration
    MediaNumber   string
    Description   string
    MP4URL        string
    SRTURL        string
    IndexItems    []VideoIndexItem
}

type VideoIndexItem struct {
    Label     string
    Start     time.Duration
    RawHref   string
}
```

### 3. Join/window builder

Responsibilities:

- Join Legistar events to Seattle Channel pages using `EventMedia`.
- Join Legistar event items to Seattle Channel timestamp labels.
- Infer item windows.
- Slice captions by item window.
- Persist reviewable confidence/evidence for the join.

Recommended matching strategy:

1. **Direct matter-file match**: match labels containing `EventItemMatterFile`
   such as `CB 121202`, `Res 32200`, `Appt 03497`, or `Inf 2899`.
2. **Normalized-title similarity**: compare Seattle Channel timestamp label to
   `EventItemTitle`, ignoring punctuation, stop words, and boilerplate.
3. **Sequence fallback**: when labels omit matter IDs, align the ordered
   Seattle Channel index with meaningful Legistar agenda items after excluding
   boilerplate rows such as “Call To Order”, “Approval of the Agenda”, and
   “Items of Business”.
4. **Manual/review queue**: flag low-confidence joins for review rather than
   silently attaching the wrong video window to a matter.

Window inference:

- `start_ms` = matched Seattle Channel timestamp start.
- `end_ms` = next matched item start, or video duration for the final item.
- Store source/evidence: matched label, match type, confidence, and raw source
  URLs.

Caption slicing:

- Parse SRT into caption cues with start/end times.
- Assign each cue to the agenda item window whose interval overlaps the cue.
- Store item-scoped transcript segments separately from meeting-level captions.
- Preserve meeting-level captions for search/debugging.

## Suggested database shape

Names are illustrative; adapt to the current schema conventions when
implementing.

```sql
CREATE TABLE seattle_legistar_event (
  id BIGSERIAL PRIMARY KEY,
  event_id BIGINT UNIQUE NOT NULL,
  event_guid TEXT NOT NULL,
  body_id BIGINT,
  body_name TEXT,
  event_date DATE,
  event_time TEXT,
  location TEXT,
  agenda_status TEXT,
  minutes_status TEXT,
  agenda_file_url TEXT,
  minutes_file_url TEXT,
  event_media_url TEXT,
  insite_url TEXT,
  source_record_id BIGINT REFERENCES source_record(id),
  fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE seattle_legistar_event_item (
  id BIGSERIAL PRIMARY KEY,
  event_id BIGINT NOT NULL REFERENCES seattle_legistar_event(event_id),
  event_item_id BIGINT UNIQUE NOT NULL,
  agenda_sequence INT,
  minutes_sequence INT,
  agenda_number TEXT,
  title TEXT,
  agenda_note TEXT,
  minutes_note TEXT,
  action_name TEXT,
  action_text TEXT,
  passed_flag_name TEXT,
  matter_id BIGINT,
  matter_guid TEXT,
  matter_file TEXT,
  matter_type TEXT,
  matter_status TEXT,
  source_record_id BIGINT REFERENCES source_record(id)
);

CREATE TABLE seattle_channel_video (
  id BIGSERIAL PRIMARY KEY,
  video_id TEXT UNIQUE NOT NULL,
  url TEXT NOT NULL,
  title TEXT,
  video_date DATE,
  duration_ms INT,
  media_number TEXT,
  mp4_url TEXT,
  srt_url TEXT,
  description TEXT,
  source_record_id BIGINT REFERENCES source_record(id),
  fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE seattle_channel_video_index_item (
  id BIGSERIAL PRIMARY KEY,
  video_id TEXT NOT NULL REFERENCES seattle_channel_video(video_id),
  label TEXT NOT NULL,
  start_ms INT NOT NULL,
  ordinal INT NOT NULL
);

CREATE TABLE seattle_event_item_video_window (
  id BIGSERIAL PRIMARY KEY,
  event_item_id BIGINT NOT NULL REFERENCES seattle_legistar_event_item(event_item_id),
  video_id TEXT NOT NULL REFERENCES seattle_channel_video(video_id),
  start_ms INT NOT NULL,
  end_ms INT,
  match_type TEXT NOT NULL,
  match_confidence NUMERIC,
  matched_label TEXT,
  review_status TEXT NOT NULL DEFAULT 'auto',
  UNIQUE(event_item_id, video_id)
);

CREATE TABLE seattle_event_item_caption_segment (
  id BIGSERIAL PRIMARY KEY,
  event_item_id BIGINT NOT NULL REFERENCES seattle_legistar_event_item(event_item_id),
  video_id TEXT NOT NULL REFERENCES seattle_channel_video(video_id),
  start_ms INT NOT NULL,
  end_ms INT NOT NULL,
  text TEXT NOT NULL,
  cue_index INT
);
```

Potential future tables:

- `seattle_legistar_matter`
- `seattle_legistar_matter_history`
- `seattle_legistar_attachment`
- `seattle_speaker_cluster`
- `seattle_speaker_assignment`
- `seattle_public_comment_segment`

## Ingestion flow

### Phase 1: meeting/event ingest

1. Query Legistar events for a date range, body, or recent changes.
2. Upsert `seattle_legistar_event`.
3. For each event, fetch event items with notes/attachments.
4. Upsert `seattle_legistar_event_item`.
5. If `EventMedia` is present, extract `videoid` and enqueue Seattle Channel
   fetch.

### Phase 2: video/caption ingest

1. Fetch Seattle Channel video page.
2. Parse page metadata, MP4 URL, SRT URL, duration, media number, and timestamp
   index.
3. Upsert `seattle_channel_video` and `seattle_channel_video_index_item`.
4. Fetch SRT caption file.
5. Store raw SRT and parsed meeting-level cues.

### Phase 3: item-window matching

1. Load Legistar event items and Seattle Channel index items for the event.
2. Match index labels to event items using the strategy above.
3. Infer start/end windows.
4. Upsert `seattle_event_item_video_window`.
5. Flag ambiguous or unmatched items for review.

### Phase 4: caption slicing and search

1. For each item window, assign overlapping caption cues.
2. Upsert `seattle_event_item_caption_segment`.
3. Build item-level transcript search and page assembly.
4. Link each item page back to:
   - Legistar event;
   - Legistar item/matter;
   - agenda/minutes PDFs;
   - Seattle Channel video page;
   - direct caption/video asset URLs when appropriate.

### Phase 5: enrichment

Optional follow-ons:

- speaker diarization and reviewed speaker labels;
- public-comment speaker segmentation;
- matter history timelines;
- councilmember vote/action summaries;
- links to department/budget/contract/source-context records.

## CLI sketch

Potential commands:

```sh
wa-dd ingest-seattle-legistar-events --start-date 2026-01-01 --end-date 2026-06-30
wa-dd ingest-seattle-channel-video --video-id x187514
wa-dd ingest-seattle-council-videos --start-date 2026-01-01 --end-date 2026-06-30
wa-dd match-seattle-video-windows --event-id 6717
wa-dd slice-seattle-captions --event-id 6717
```

A combined operator command could run the full flow:

```sh
wa-dd ingest-seattle-council --start-date 2026-01-01 --end-date 2026-06-30
```

## Risks and caveats

- **Seattle Channel has no confirmed public JSON API for video metadata.** The
  HTML is straightforward, but page structure can change. Keep parsers small,
  tested, and provenance-backed.
- **Legistar item video fields may be empty.** Do not assume `EventItemVideo` or
  `EventItemVideoIndex` is populated.
- **Timestamp labels are not perfect boundaries.** They are starts, not explicit
  end times. Infer end times from the next item or video duration, then allow
  review/refinement.
- **Public comment privacy.** Public commenters are public meeting participants,
  but speaker labeling and surfacing should still be careful and source-linked.
- **Captions may contain errors.** Treat captions as source text with possible
  inaccuracies. Preserve source links and timestamps.
- **Terms/rate limits.** Use polite request rates. Prefer Legistar API for
  structured data and avoid bulk video downloads unless required.

## Implementation recommendation

Start with a small vertical slice:

1. Ingest one recent committee event from Legistar, e.g. EventId `6717`.
2. Fetch its Seattle Channel page from `EventMedia`.
3. Parse MP4/SRT/index metadata.
4. Match three business items to timestamp labels.
5. Slice captions into item windows.
6. Render a local/debug page showing event → item → video window → transcript
   with source links.

If this works across 10-20 varied meetings, promote it into a general Seattle
Council ingestion pipeline.
