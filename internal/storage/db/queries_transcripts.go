package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// AgendaItemWindow is one detected bill-discussion span on a TVW event.
// SegmentTranscript writes these atomically; page assemblers and search
// read them back literally so window boundaries are stable across
// re-renders and aren't re-derived in SQL with a different threshold.
type AgendaItemWindow struct {
	StartMS  int
	EndMS    int
	Mentions int
}

// ReplaceAgendaItemWindows is the canonical write path for transcript
// segmentation. It clears prior windows for the agenda item and inserts
// the supplied set in one transaction. Returns the number of windows
// written. Re-segmentation is idempotent.
func (s *Store) ReplaceAgendaItemWindows(
	ctx context.Context,
	agendaItemID int64,
	windows []AgendaItemWindow,
) (int64, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM agenda_item_window WHERE agenda_item_id = $1`, agendaItemID); err != nil {
		return 0, err
	}

	const insWinQ = `
INSERT INTO agenda_item_window (agenda_item_id, start_ms, end_ms, mentions)
VALUES ($1, $2, $3, $4);`

	var total int64
	for _, w := range windows {
		if w.EndMS <= w.StartMS {
			continue
		}
		if _, err := tx.Exec(ctx, insWinQ, agendaItemID, w.StartMS, w.EndMS, w.Mentions); err != nil {
			return 0, err
		}
		total++
	}
	return total, tx.Commit(ctx)
}

// ListAgendaItemWindowsByAgendaItem returns the persisted windows for a
// CSI agenda-item ID, ordered by start_ms. Used by page assemblers.
func (s *Store) ListAgendaItemWindowsByAgendaItem(ctx context.Context, csiAgendaItemID string) ([]AgendaItemWindow, error) {
	const q = `
SELECT w.start_ms, w.end_ms, w.mentions
  FROM agenda_item_window w
  JOIN agenda_item a ON a.id = w.agenda_item_id
 WHERE a.csi_agenda_item_id = $1
 ORDER BY w.start_ms ASC;`
	rows, err := s.Pool.Query(ctx, q, csiAgendaItemID)
	if err != nil {
		return nil, fmt.Errorf("list agenda_item_window: %w", err)
	}
	defer rows.Close()
	var out []AgendaItemWindow
	for rows.Next() {
		var w AgendaItemWindow
		if err := rows.Scan(&w.StartMS, &w.EndMS, &w.Mentions); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// TranscriptCue is a minimal transcript turn used by the segmentation step.
type TranscriptCue struct {
	StartMS int
	EndMS   int
	Text    string
}

// ListDiarizedCues returns ordered text turns from the latest succeeded
// diarization job for a TVW event. SegmentTranscript uses these as the
// input to bill-discussion window detection. Returns an empty slice (not
// an error) when no succeeded job exists yet — the caller decides
// whether to skip or fail.
func (s *Store) ListDiarizedCues(ctx context.Context, tvwEventID string) ([]TranscriptCue, error) {
	const q = `
SELECT d.start_ms, d.end_ms, COALESCE(d.text, '')
  FROM diarized_speech_segment d
 WHERE d.tvw_event_id = $1
   AND d.text IS NOT NULL
   AND d.diarization_job_id = (
     SELECT id FROM diarization_job
      WHERE tvw_event_id = $1 AND status = 'succeeded'
      ORDER BY finished_at DESC NULLS LAST, id DESC LIMIT 1
   )
 ORDER BY d.start_ms ASC;`
	rows, err := s.Pool.Query(ctx, q, tvwEventID)
	if err != nil {
		return nil, fmt.Errorf("list diarized cues: %w", err)
	}
	defer rows.Close()
	var out []TranscriptCue
	for rows.Next() {
		var c TranscriptCue
		if err := rows.Scan(&c.StartMS, &c.EndMS, &c.Text); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// organization
// ---------------------------------------------------------------------------

// TranscriptSearchHit is one row returned by SearchTranscripts. The bill
// fields can all be empty when no agenda_item_window overlaps the hit
// (i.e. the diarized turn falls outside any detected bill discussion).
type TranscriptSearchHit struct {
	ID              int64
	BillID          string // "HB 1501" — empty when bill join misses
	Biennium        string
	BillPrefix      string
	BillNumber      int
	AgendaItemLabel string
	CommitteeName   string
	MeetingDateTime time.Time // zero when hearing join misses
	StartMS         int
	EndMS           int
	Text            string
	SpeakerLabel    string
	TVWEventID      string
}

// SearchTranscripts runs a websearch_to_tsquery full-text search over
// the diarized_speech_segment table (latest succeeded job per event) and
// joins the result back to the agenda_item_window with the largest time
// overlap to recover bill + hearing + agenda_item context.
//
// Returns ([]hits, total, err). total comes from a COUNT(*) OVER ()
// window function so the caller can render "showing N of TOTAL" without
// a second query.
//
// Empty / whitespace-only query returns ([]hits=nil, total=0, err=nil)
// without touching Postgres.
//
// limit and offset are caller-provided; the API handler is responsible
// for clamping them.
func (s *Store) SearchTranscripts(
	ctx context.Context, query string, limit, offset int,
) ([]TranscriptSearchHit, int, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, 0, nil
	}
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	// For each diarized hit, pick the agenda_item_window with the
	// largest time overlap (LATERAL + ORDER BY overlap DESC LIMIT 1).
	// A turn that spans two windows is attributed to the larger one
	// rather than emitting duplicate hits.
	const sql = `
WITH q AS (SELECT websearch_to_tsquery('english', $1) AS tsq),
hits AS (
  SELECT d.id, d.tvw_event_id, d.start_ms, d.end_ms, COALESCE(d.text,'') AS text,
         d.diarization_job_id, d.speaker_cluster_id
    FROM diarized_speech_segment d
    JOIN diarization_job dj ON dj.id = d.diarization_job_id
   WHERE dj.status = 'succeeded'
     AND dj.id = (
       SELECT id FROM diarization_job
        WHERE tvw_event_id = d.tvw_event_id AND status = 'succeeded'
        ORDER BY finished_at DESC NULLS LAST, id DESC LIMIT 1
     )
     AND d.text IS NOT NULL
     AND to_tsvector('english', d.text) @@ (SELECT tsq FROM q)
)
SELECT
  hits.id,
  COALESCE(b.bill_number, '')              AS bill_id,
  COALESCE(b.biennium, '')                 AS biennium,
  COALESCE(b.prefix, '')                   AS bill_prefix,
  COALESCE(b.number, 0)                    AS bill_number,
  COALESCE(a.label, '')                    AS agenda_item_label,
  COALESCE(h.committee_name, '')           AS committee_name,
  h.meeting_datetime,
  hits.start_ms,
  hits.end_ms,
  hits.text,
  CASE WHEN sa.id IS NOT NULL THEN COALESCE(sa.speaker_label, '') ELSE '' END AS speaker_label,
  hits.tvw_event_id,
  COUNT(*) OVER ()                         AS total_count
FROM hits
LEFT JOIN hearing h ON h.tvw_event_id = hits.tvw_event_id
LEFT JOIN LATERAL (
  SELECT w.agenda_item_id
    FROM agenda_item_window w
    JOIN agenda_item ai ON ai.id = w.agenda_item_id
   WHERE ai.hearing_id = h.id
     AND hits.start_ms < w.end_ms
     AND hits.end_ms   > w.start_ms
   ORDER BY LEAST(hits.end_ms, w.end_ms) - GREATEST(hits.start_ms, w.start_ms) DESC
   LIMIT 1
) win ON TRUE
LEFT JOIN agenda_item a ON a.id = win.agenda_item_id
LEFT JOIN bill        b ON b.id = a.bill_id
LEFT JOIN speaker_assignment sa
  ON sa.diarization_job_id = hits.diarization_job_id
 AND sa.speaker_cluster_id = hits.speaker_cluster_id
 AND sa.review_status = 'accepted'
ORDER BY ts_rank_cd(to_tsvector('english', hits.text), (SELECT tsq FROM q)) DESC,
         hits.start_ms ASC
LIMIT $2 OFFSET $3;`

	rows, err := s.Pool.Query(ctx, sql, q, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("search transcripts: %w", err)
	}
	defer rows.Close()

	out := []TranscriptSearchHit{}
	total := 0
	for rows.Next() {
		var h TranscriptSearchHit
		var meetingTS pgtype.Timestamptz
		if err := rows.Scan(
			&h.ID, &h.BillID, &h.Biennium, &h.BillPrefix, &h.BillNumber,
			&h.AgendaItemLabel, &h.CommitteeName, &meetingTS,
			&h.StartMS, &h.EndMS, &h.Text, &h.SpeakerLabel,
			&h.TVWEventID, &total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan transcript hit: %w", err)
		}
		if meetingTS.Valid {
			h.MeetingDateTime = meetingTS.Time
		}
		out = append(out, h)
	}
	return out, total, rows.Err()
}

// ---------------------------------------------------------------------------
// audio cache + diarization
// ---------------------------------------------------------------------------
