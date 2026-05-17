package jobs

import (
	"context"
	"fmt"
)

// MatchSpeakers heuristically assigns speaker labels to transcript segments
// that fall in the bill segment. The wiki's Blueprint §"Speaker attribution
// for MVP" (lines 274–284) endorses confidence labels rather than perfect
// diarization.
//
// v1 strategy: for each segment with no speaker, scan the segment text for
// any testifier name (last-name match preferred, falling back to full name).
// If found, label as `likely_testifier`; if not, leave as `unknown_speaker`.
// Legislator detection is deferred — sponsor/committee chair detection in
// v1 produces too many false positives without a member roster.
//
// This pass mutates only segments inside [BillSegmentStart, BillSegmentEnd].
func (p *Pipeline) MatchSpeakers(ctx context.Context, ids *IDs) error {
	if ids.AgendaItemID == 0 || ids.TVWEventID == "" {
		return fmt.Errorf("match-speakers: missing IDs (run ingest-csi/ingest-tvw first)")
	}
	if ids.BillSegmentEnd <= ids.BillSegmentStart {
		fmt.Fprintln(stderrSink, "  no bill segment range; skipping speaker matching")
		return nil
	}

	const q = `
WITH testifiers AS (
  SELECT raw_name FROM testifier WHERE agenda_item_id = $1
),
candidates AS (
  SELECT id, text FROM transcript_segment
   WHERE tvw_event_id = $2 AND start_ms BETWEEN $3 AND $4
)
UPDATE transcript_segment ts
   SET speaker_label = sub.raw_name,
       speaker_confidence = 'likely_testifier'::speaker_confidence
  FROM (
    SELECT c.id, t.raw_name FROM candidates c
      JOIN testifiers t
        ON c.text ILIKE '%' || split_part(t.raw_name, ',', 1) || '%'
    LIMIT 5000
  ) sub
 WHERE ts.id = sub.id
   AND ts.speaker_confidence = 'unknown_speaker';`
	tag, err := p.Store.Pool.Exec(ctx, q,
		ids.AgendaItemID, ids.TVWEventID,
		ids.BillSegmentStart, ids.BillSegmentEnd)
	if err != nil {
		return fmt.Errorf("match-speakers: %w", err)
	}
	fmt.Fprintf(stderrSink, "  labeled %d segments as likely_testifier\n", tag.RowsAffected())
	return nil
}
