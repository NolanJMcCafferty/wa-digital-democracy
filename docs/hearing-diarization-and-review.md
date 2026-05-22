# Hearing diarization and speaker review

Last updated: 2026-05-20.

This document explains the WA Digital Democracy hearing diarization flow and the
manual human review that must happen before anonymous speaker clusters become
public speaker labels.

The short version:

1. `ingest-hearings` gets each hearing's CSI agenda/testifier data plus TVW /
   Invintus video, audio, and caption metadata.
2. `diarize-event` or `diarize-pending` sends the TVW/Invintus audio/video URL
   to Deepgram with diarization, punctuation, utterances, and entity detection
   enabled.
3. The Deepgram output is normalized into anonymous speaker clusters and merged
   speech segments.
4. `extract-speaker-evidence` scans the diarized text for conservative identity
   clues, currently mostly self-introductions.
5. Human reviewers use `/admin/review/speakers` to accept, reject, request more
   evidence, or manually assign labels.
6. Public hearing transcripts render accepted `speaker_assignment` labels only;
   unresolved clusters remain anonymous/unreviewed.

## Why this exists

Deepgram can answer **"which spans sound like the same speaker?"** It cannot be
trusted to answer **"who is this person?"** for public-facing civic records.

The system therefore separates:

- diarization: anonymous time spans and cluster labels such as `SPEAKER_00`;
- evidence extraction: text/source clues that suggest a candidate identity;
- human review: a reviewer decides whether evidence is strong enough;
- public display: only accepted assignments become names on public transcript
  pages.

This separation is intentional. It keeps the public product useful without
turning uncertain model output into authoritative identity claims.

## Inputs and prerequisites

A TVW event must already exist in Postgres. In the normal path this means:

```sh
make ingest-legislators
make ingest-session BIENNIUM=2025-26
make ingest-hearings BIENNIUM=2025-26
```

`ingest-hearings` discovers CSI agenda IDs and TVW event IDs, fetches CSI
testifiers, fetches TVW/Invintus event details and captions, and stores the
video/audio metadata needed by diarization.

Diarization requires:

- a TVW/Invintus event ID (`tvw_event.tvw_event_id`);
- a usable audio/video source URL from `tvw_event` or `tvw_media_asset`;
- `DEEPGRAM_API_KEY` in the environment, or `--api-key` passed to the CLI;
- Postgres reachable through `WADD_DSN` or `--dsn`.

## Data model

The relevant tables are introduced mainly by:

- `db/migrations/0012_diarization_audio.sql`
- `db/migrations/0013_entity_mentions.sql`
- `db/migrations/0014_speaker_review.sql`

### Audio and diarization tables

- `tvw_audio_asset`
  - Caches the selected source URL and optional normalized local audio metadata.
  - Tracks `source_url`, `source_kind`, local paths, content hash, duration,
    sample rate, channels, and codec.

- `diarization_job`
  - One provider run for one TVW event.
  - Tracks provider, model, status (`pending`, `running`, `succeeded`, `failed`),
    raw result path, and errors.
  - The public/API path uses the latest succeeded job for a TVW event.

- `speaker_cluster`
  - One anonymous speaker cluster from one diarization job.
  - Example labels: `SPEAKER_00`, `SPEAKER_01`.
  - Stores total speech time and turn count for prioritization/review.

- `diarized_speech_segment`
  - Merged speech spans for the public transcript path.
  - Each row has `start_ms`, `end_ms`, text, confidence, and `cluster_label`.
  - Adjacent same-speaker Deepgram utterances are merged so the UI shows
    readable turns instead of sentence-level fragments.

- `entity_mention`
  - Provider-derived named entities from Deepgram output.
  - These are evidence only; they are not confirmed organization/person facts.

### Review tables

- `speaker_identity_evidence`
  - Evidence row suggesting a candidate label for a speaker cluster.
  - Evidence types include `self_introduction`, `chair_call`, `entity_mention`,
    `csi_order`, and `manual_note`.
  - Current automatic extraction is intentionally conservative and mostly emits
    self-introduction evidence.

- `speaker_review_task`
  - A proposed identity assignment that needs human action.
  - Status values: `pending`, `accepted`, `rejected`, `needs_more_evidence`,
    `superseded`.

- `speaker_assignment`
  - The accepted public label for a cluster.
  - Unique per `(diarization_job_id, speaker_cluster_id)`.
  - Public hearing transcripts only show names through accepted assignments.

## Provider normalization

The provider boundary lives in `internal/diarization`.

Deepgram is currently the supported provider. `diarize-event` calls Deepgram's
prerecorded endpoint with these query options:

- `diarize=true`
- `punctuate=true`
- `utterances=true`
- `detect_entities=true`
- `model=<model>`, default `nova-3`

The normalized provider-neutral result contains:

- `Segments`: start/end milliseconds, anonymous speaker cluster, confidence,
  text;
- `Words`: optional word-level timing and speaker info;
- `Entities`: provider-detected named entities;
- `Raw`: raw provider JSON, written to `data/processed/diarization/...`.

`internal/diarization.MergeConsecutiveSegments` collapses adjacent segments from
the same speaker cluster into one turn-like segment. This is the canonical
public transcript unit today.

## Running diarization

### One event

For a single TVW/Invintus event ID:

```sh
DEEPGRAM_API_KEY=... \
  go run ./cmd/wa-dd diarize-event \
    --event-id <tvw_event_id>
```

Useful flags:

```txt
--provider deepgram              # currently the only supported provider
--model nova-3                   # Deepgram model; default nova-3
--out-dir data/processed/diarization
--use-source-url=true            # default: send provider the TVW/Invintus URL
--api-key ...                    # defaults to DEEPGRAM_API_KEY
--dsn ...                        # defaults to WADD_DSN or local dev DSN
```

By default the command sends the original TVW/Invintus URL to Deepgram. If a
provider cannot fetch the source URL, run `audio-cache` first and then call
`diarize-event --use-source-url=false`.

### Audio cache fallback

```sh
go run ./cmd/wa-dd audio-cache --event-id <tvw_event_id>
DEEPGRAM_API_KEY=... \
  go run ./cmd/wa-dd diarize-event \
    --event-id <tvw_event_id> \
    --use-source-url=false
```

`audio-cache` downloads and normalizes a TVW/Invintus audio/video source and
stores metadata in `tvw_audio_asset`. The local normalized path is then used for
upload mode.

### Pending events

To process events that have audio/video source URLs and no successful
diarization job yet:

```sh
DEEPGRAM_API_KEY=... \
  go run ./cmd/wa-dd diarize-pending --limit 25 --concurrency 4
```

Useful flags:

```txt
--dry-run             # print pending event IDs without calling provider
--limit N             # cap event count
--concurrency N       # default 10; lower this if provider/API limits bite
--max-attempts N      # default 5 per event
```

`diarize-pending` retries transient failures. Each event gets its own
`diarization_job`; failed jobs preserve the error for debugging.

### Re-merge stored raw results

If merge behavior changes, reprocess stored Deepgram JSON without recalling the
provider:

```sh
go run ./cmd/wa-dd merge-segments --event-id <tvw_event_id>
```

Use `--dry-run` to inspect how many merged segments would be written.

## Extracting speaker evidence

Diarization creates anonymous clusters. The next step is to generate reviewable
identity candidates:

```sh
go run ./cmd/wa-dd extract-speaker-evidence \
  --event-id <tvw_event_id> \
  --job-id <diarization_job_id>
```

Optional:

```txt
--limit N     # scan at most N diarized segments
```

Current automatic extraction lives in `internal/diarization/speaker_evidence.go`.
It intentionally emits few candidates because false positives are worse than
misses. Current high-precision patterns include:

- `for the record, Senator Jane Smith ...`
- `Representative Jane Smith from the ... district ...`
- `for the record, my name is Jane Smith ...`
- `my name is Jane Smith ...`

For each candidate, the command:

1. resolves the candidate against known legislator/testifier context when
   possible;
2. writes or updates `speaker_identity_evidence`;
3. writes or updates a `speaker_review_task` for the cluster/candidate;
4. assigns higher priority to legislator candidates and candidates with a
   resolved source ID.

Evidence extraction does **not** publish a name. It only creates tasks.

## Manual human review

Review is required before names appear publicly.

### Review entry points

- `/admin/review/speakers`
  - event-level overview of diarized events needing review;
  - shows cluster counts, accepted assignments, pending task counts, unresolved
    clusters, and total speech time.

- `/admin/review/speakers/events/<tvwEventId>`
  - lists clusters for one TVW event;
  - links into the cluster review page.

- `/admin/review/speakers/clusters/<clusterId>`
  - main review screen for a speaker cluster;
  - shows candidate tasks, current accepted assignment, evidence rows, and
    sample segments;
  - supports accepting/rejecting candidate tasks, marking “more evidence,” and
    manual assignment.

- `/admin/review/speakers/<taskId>`
  - older task-centered detail route.

### What reviewers should check

For each cluster, review:

1. **Evidence text**
   - Does the transcript text actually identify the speaker?
   - Is the name complete enough for a public label?
   - Could the text be quoting someone else rather than self-identifying?

2. **Timing and surrounding context**
   - Open the TVW timestamp if needed.
   - Check whether the same cluster speaks consistently before/after the
     evidence segment.
   - Watch for chair/staff transitions where diarization may accidentally merge
     speakers.

3. **Candidate kind**
   - `legislator`: should match a known LWS sponsor/legislator when possible.
   - `testifier`: should be consistent with CSI testifier list/order and the
     bill/agenda context.
   - `person`: acceptable for manual/freeform labels when no structured ID is
     available.
   - `unknown`: use when no public label should be assigned.

4. **Cluster quality**
   - A cluster may contain more than one real person if diarization merged
     voices incorrectly.
   - A real person may appear in multiple clusters if diarization split them.
   - If uncertain, reject or mark “more evidence” rather than accepting.

5. **Public-risk posture**
   - Names on public pages should be reviewed, source-supported, and not
     speculative.
   - If the evidence is weak, leave the cluster anonymous.

### Review actions

- **Accept**
  - Marks the review task `accepted`.
  - Upserts `speaker_assignment` for the cluster.
  - The accepted label can then appear in public hearing transcripts.

- **Reject**
  - Marks the candidate task `rejected`.
  - Does not create or change a public assignment.

- **More evidence**
  - Marks the task `needs_more_evidence`.
  - Use when the candidate might be right but the current evidence is
    insufficient.

- **Manual assignment**
  - Directly assigns a label to a cluster when the reviewer has enough evidence
    not captured by automatic extraction.
  - Requires choosing a kind (`person`, `legislator`, `testifier`, `unknown`),
    entering a label, reviewer, and optional notes.

## Public display rules

Public hearing pages call `ListDiarizedSegmentsByTVWEvent` through the hearing
API. That query joins the latest succeeded `diarization_job` for the TVW event
against accepted `speaker_assignment` rows.

Rules:

- reviewed/accepted assignment → show `speaker_label`;
- no accepted assignment → do **not** publish a name;
- unresolved cluster with `cluster_label` → show an unreviewed/anonymous speaker
  affordance in the UI;
- rejected or needs-more-evidence task → not public as a label;
- provider entity mentions → evidence only, not public identity facts.

The frontend helper `PublicSpeakerLabel()` intentionally returns an empty string
unless the segment is reviewed. This is the safety gate that keeps unresolved
clusters from becoming authoritative facts.

## Quality-control checklist

Before treating a hearing as reviewed enough for public use:

- [ ] The event has a succeeded `diarization_job`.
- [ ] Segments are merged into readable same-speaker turns.
- [ ] `extract-speaker-evidence` has been run for the job.
- [ ] High-priority pending speaker tasks have been accepted, rejected, or
      marked needs-more-evidence.
- [ ] Accepted labels are supported by transcript/source/video context.
- [ ] Ambiguous clusters remain anonymous.
- [ ] Public page rendering shows reviewed labels only.
- [ ] Source links/timestamps let a reader verify the underlying moment.

## Common failure modes

### No audio source found

`diarize-event` depends on `BestTVWAudioSource` / `EnsureTVWAudioAsset`. If no
usable URL exists, re-run TVW ingest for the hearing and inspect `tvw_event` and
`tvw_media_asset` rows.

### Provider cannot fetch URL

Run `audio-cache --event-id ...` and retry `diarize-event --use-source-url=false`.
This uploads local normalized WAV bytes instead of asking Deepgram to fetch the
remote URL.

### Too many tiny transcript blocks

Run `merge-segments` for the event/job. The current parser already merges
same-speaker runs, but `merge-segments` is useful after parser changes or when
old raw results were stored before a merge fix.

### Bad speaker candidate

Reject the task. If the text pattern itself is too permissive, adjust
`internal/diarization/speaker_evidence.go` and add a unit test.

### Missing obvious speaker candidate

Use manual assignment if the evidence is clear. Then consider adding a new
high-precision extractor only if it is likely to generalize without creating
false positives.

### Wrong accepted assignment

Use the cluster review page to assign the correct label manually. The
`speaker_assignment` table is unique per cluster/job, so a new accepted
assignment supersedes the old public label.

## Useful SQL snippets

Latest successful job for an event:

```sql
SELECT id, tvw_event_id, provider, model, status, finished_at, raw_result_path
  FROM diarization_job
 WHERE tvw_event_id = '<event_id>' AND status = 'succeeded'
 ORDER BY finished_at DESC NULLS LAST, id DESC
 LIMIT 1;
```

Clusters needing review:

```sql
SELECT sc.id, sc.cluster_label, sc.total_speech_ms, sc.turn_count,
       COUNT(t.id) FILTER (WHERE t.status = 'pending') AS pending_tasks,
       COALESCE(sa.speaker_label, '') AS accepted_label
  FROM speaker_cluster sc
  JOIN diarization_job j ON j.id = sc.diarization_job_id
  LEFT JOIN speaker_review_task t ON t.speaker_cluster_id = sc.id
  LEFT JOIN speaker_assignment sa
    ON sa.speaker_cluster_id = sc.id
   AND sa.review_status = 'accepted'
 WHERE sc.tvw_event_id = '<event_id>'
   AND j.status = 'succeeded'
 GROUP BY sc.id, sc.cluster_label, sc.total_speech_ms, sc.turn_count, sa.speaker_label
 ORDER BY pending_tasks DESC, sc.total_speech_ms DESC;
```

Accepted assignments for an event:

```sql
SELECT sc.cluster_label, sa.speaker_kind, sa.speaker_label, sa.review_status,
       sa.updated_at
  FROM speaker_assignment sa
  JOIN speaker_cluster sc ON sc.id = sa.speaker_cluster_id
 WHERE sc.tvw_event_id = '<event_id>'
   AND sa.review_status = 'accepted'
 ORDER BY sc.cluster_label;
```

## Safety principles

- Diarization clusters are anonymous until reviewed.
- Model-detected entities are evidence, not confirmed facts.
- Prefer false negatives over false positives for public speaker labels.
- Keep raw provider JSON and source records so decisions can be audited.
- Humans, not models, decide public identity labels.
- When in doubt, leave the speaker anonymous and preserve the timestamped
  evidence trail.
