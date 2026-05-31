package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type InsertTranscriptSegmentParams struct {
	TVWEventID        string
	AgendaItemID      *int64
	StartMS           int
	EndMS             int
	Text              string
	SpeakerLabel      string
	SpeakerEntityID   *int64
	SpeakerConfidence string // speaker_confidence enum
	SourceCaptionURL  string
	SourceRecordID    int64
}

// ReplaceTranscriptSegments wipes and reloads all segments for an event —
// captions are immutable per fetch, but we may re-fetch corrected versions.
func (s *Store) ReplaceTranscriptSegments(ctx context.Context, tvwEventID string, rows []InsertTranscriptSegmentParams) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM transcript_segment WHERE tvw_event_id = $1`, tvwEventID); err != nil {
		return fmt.Errorf("delete segments: %w", err)
	}
	const insQ = `
INSERT INTO transcript_segment (tvw_event_id, agenda_item_id, start_ms, end_ms, text,
                                speaker_label, speaker_entity_id, speaker_confidence,
                                source_caption_url, source_record_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8::speaker_confidence,$9,$10);`
	for _, r := range rows {
		if _, err := tx.Exec(ctx, insQ,
			r.TVWEventID, r.AgendaItemID, r.StartMS, r.EndMS, r.Text,
			strOrNull(r.SpeakerLabel), r.SpeakerEntityID,
			defaultStr(r.SpeakerConfidence, "unknown_speaker"),
			r.SourceCaptionURL, r.SourceRecordID,
		); err != nil {
			return fmt.Errorf("insert segment: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// AgendaItemWindow is one detected bill-discussion span on a TVW event.
// SegmentTranscript writes these atomically with the corresponding
// transcript_segment.agenda_item_id assignments; page assemblers
// read them back literally so window boundaries are stable across
// re-renders and aren't re-derived in SQL with a different threshold.
type AgendaItemWindow struct {
	StartMS  int
	EndMS    int
	Mentions int
}

// AssignSegmentsToAgendaItemWindows is the canonical write path for
// transcript segmentation. In one transaction it (a) clears any prior
// agenda_item_id pointing at this agenda item, (b) re-tags transcript
// segments whose start falls inside any supplied window, and (c)
// rewrites the agenda_item_window rows for that agenda item.
//
// Re-segmentation is idempotent: running this with the same input set
// twice converges to the same DB state. Running it after a heuristic
// change correctly evicts stale segments and stale windows.
func (s *Store) AssignSegmentsToAgendaItemWindows(
	ctx context.Context,
	tvwEventID string,
	agendaItemID int64,
	windows []AgendaItemWindow,
) (int64, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	// Clear prior tags so segments that fell out of the new window set
	// stop pointing at this agenda item.
	if _, err := tx.Exec(ctx, `UPDATE transcript_segment SET agenda_item_id = NULL WHERE tvw_event_id = $1 AND agenda_item_id = $2`, tvwEventID, agendaItemID); err != nil {
		return 0, err
	}
	// Clear prior persisted windows for the same reason.
	if _, err := tx.Exec(ctx, `DELETE FROM agenda_item_window WHERE agenda_item_id = $1`, agendaItemID); err != nil {
		return 0, err
	}

	const tagQ = `
UPDATE transcript_segment SET agenda_item_id = $2
 WHERE tvw_event_id = $1 AND start_ms >= $3 AND start_ms <= $4;`
	const insWinQ = `
INSERT INTO agenda_item_window (agenda_item_id, start_ms, end_ms, mentions)
VALUES ($1, $2, $3, $4);`

	var total int64
	for _, w := range windows {
		if w.EndMS <= w.StartMS {
			continue
		}
		tag, err := tx.Exec(ctx, tagQ, tvwEventID, agendaItemID, w.StartMS, w.EndMS)
		if err != nil {
			return 0, err
		}
		total += tag.RowsAffected()
		if _, err := tx.Exec(ctx, insWinQ, agendaItemID, w.StartMS, w.EndMS, w.Mentions); err != nil {
			return 0, err
		}
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

// TranscriptCue is a minimal transcript segment used by the segmentation step.
type TranscriptCue struct {
	StartMS int
	EndMS   int
	Text    string
}

// ListTranscriptCues returns ordered caption cues for a TVW event.
func (s *Store) ListTranscriptCues(ctx context.Context, tvwEventID string) ([]TranscriptCue, error) {
	const q = `
SELECT start_ms, end_ms, text FROM transcript_segment
 WHERE tvw_event_id = $1
 ORDER BY start_ms ASC;`
	rows, err := s.Pool.Query(ctx, q, tvwEventID)
	if err != nil {
		return nil, fmt.Errorf("list transcript cues: %w", err)
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

// FindBillNumberMentions returns segment start_ms values where the
// transcript text mentions the given bill (e.g. "HB 1234" or "House Bill 1234").
func (s *Store) FindBillNumberMentions(ctx context.Context, tvwEventID string, prefix string, number int) ([]int, error) {
	pattern := billMentionPattern(prefix, number)
	const q = `
SELECT start_ms FROM transcript_segment
 WHERE tvw_event_id = $1 AND text ~* $2
 ORDER BY start_ms ASC;`
	rows, err := s.Pool.Query(ctx, q, tvwEventID, pattern)
	if err != nil {
		return nil, fmt.Errorf("find mentions: %w", err)
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var ms int
		if err := rows.Scan(&ms); err != nil {
			return nil, err
		}
		out = append(out, ms)
	}
	return out, rows.Err()
}

// billMentionPattern returns a single Postgres regex matching common spoken
// forms. Handles "HB 1234", "HB1234", "House Bill 1234", "House Bill1234".
// The \m / \M boundaries are POSIX word boundaries Postgres understands.
func billMentionPattern(prefix string, number int) string {
	chamberWord := map[string]string{
		"HB": "house bill", "SB": "senate bill",
		"HJR": "house joint resolution", "SJR": "senate joint resolution",
	}
	cw, ok := chamberWord[prefix]
	num := fmt.Sprintf("%d", number)
	if !ok {
		return fmt.Sprintf(`\m%s\s*%s\M`, prefix, num)
	}
	return fmt.Sprintf(`\m(%s|%s)\s*%s\M`, prefix, cw, num)
}

// ---------------------------------------------------------------------------
// organization
// ---------------------------------------------------------------------------

// TranscriptSearchHit is one row returned by SearchTranscripts. The bill
// fields can all be empty when the agenda_item / bill joins miss
// (transcript_segment.agenda_item_id is nullable until SegmentTranscript
// has run on the row).
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
// the transcript_segment table and joins the result back to bill +
// hearing + agenda_item for context.
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

	const sql = `
WITH q AS (SELECT websearch_to_tsquery('english', $1) AS tsq)
SELECT
  ts.id,
  COALESCE(b.bill_number, '')              AS bill_id,
  COALESCE(b.biennium, '')                 AS biennium,
  COALESCE(b.prefix, '')                   AS bill_prefix,
  COALESCE(b.number, 0)                    AS bill_number,
  COALESCE(a.label, '')                    AS agenda_item_label,
  COALESCE(h.committee_name, '')           AS committee_name,
  h.meeting_datetime,
  ts.start_ms,
  ts.end_ms,
  ts.text,
  COALESCE(ts.speaker_label, '')           AS speaker_label,
  COALESCE(h.tvw_event_id, ts.tvw_event_id, '') AS tvw_event_id,
  COUNT(*) OVER ()                         AS total_count
FROM transcript_segment ts
LEFT JOIN agenda_item a ON a.id = ts.agenda_item_id
LEFT JOIN hearing     h ON h.id = a.hearing_id
LEFT JOIN bill        b ON b.id = a.bill_id
WHERE to_tsvector('english', ts.text) @@ (SELECT tsq FROM q)
ORDER BY ts_rank_cd(to_tsvector('english', ts.text), (SELECT tsq FROM q)) DESC,
         ts.start_ms ASC
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
