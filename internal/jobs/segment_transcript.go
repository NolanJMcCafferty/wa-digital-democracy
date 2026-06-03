package jobs

import (
	"context"
	"fmt"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

// SegmentTranscript detects bill-discussion windows in the diarized
// transcript and persists them to agenda_item_window. Manual overrides
// still win. Multiple windows per agenda item are supported for bills
// revisited later in the same TVW event.
func (p *Pipeline) SegmentTranscript(ctx context.Context, ids *IDs) error {
	if ids.AgendaItemID == 0 {
		return fmt.Errorf("segment-transcript: AgendaItemID not set; ingest-csi must run first")
	}
	if ids.TVWEventID == "" {
		return fmt.Errorf("segment-transcript: TVWEventID not set; ingest-tvw must run first")
	}

	// Manual override wins outright (Blueprint line 596 endorses this).
	if p.BillAgendaTarget.TranscriptOverride.EndMS > p.BillAgendaTarget.TranscriptOverride.StartMS {
		ids.BillSegmentStart = p.BillAgendaTarget.TranscriptOverride.StartMS
		ids.BillSegmentEnd = p.BillAgendaTarget.TranscriptOverride.EndMS
		fmt.Fprintf(stderrSink, "  using transcript_override [%d, %d]\n",
			ids.BillSegmentStart, ids.BillSegmentEnd)
		_, err := p.Store.ReplaceAgendaItemWindows(ctx, ids.AgendaItemID,
			[]db.AgendaItemWindow{{
				StartMS:  ids.BillSegmentStart,
				EndMS:    ids.BillSegmentEnd,
				Mentions: 0, // operator-supplied; mention count not meaningful here
			}})
		return err
	}

	cues, err := p.Store.ListDiarizedCues(ctx, ids.TVWEventID)
	if err != nil {
		return err
	}
	if len(cues) == 0 {
		return fmt.Errorf("segment-transcript: no diarized turns for tvw_event_id=%s; run hearing ingestion or `wa-dd diarize-pending` first", ids.TVWEventID)
	}
	jobCues := make([]segmentCue, 0, len(cues))
	for _, c := range cues {
		jobCues = append(jobCues, segmentCue{StartMS: c.StartMS, EndMS: c.EndMS, Text: c.Text})
	}
	windows := DetectBillDiscussionWindows(jobCues, p.BillAgendaTarget.Bill.Prefix, p.BillAgendaTarget.Bill.Number)
	if len(windows) == 0 {
		fmt.Fprintf(stderrSink, "  no diarized mentions of %s %d; bill segment unset (set TranscriptOverride on the selected bill agenda target to override)\n",
			p.BillAgendaTarget.Bill.Prefix, p.BillAgendaTarget.Bill.Number)
		// Empty input still clears any stale assignments + windows
		// from a prior run that found mentions and now doesn't.
		_, err := p.Store.ReplaceAgendaItemWindows(ctx, ids.AgendaItemID, nil)
		return err
	}

	dbWindows := make([]db.AgendaItemWindow, 0, len(windows))
	ids.BillSegmentStart, ids.BillSegmentEnd = windows[0].StartMS, windows[0].EndMS
	mentions := 0
	for _, w := range windows {
		dbWindows = append(dbWindows, db.AgendaItemWindow{
			StartMS:  w.StartMS,
			EndMS:    w.EndMS,
			Mentions: w.Mentions,
		})
		if w.StartMS < ids.BillSegmentStart {
			ids.BillSegmentStart = w.StartMS
		}
		if w.EndMS > ids.BillSegmentEnd {
			ids.BillSegmentEnd = w.EndMS
		}
		mentions += w.Mentions
	}
	if _, err := p.Store.ReplaceAgendaItemWindows(ctx, ids.AgendaItemID, dbWindows); err != nil {
		return err
	}
	fmt.Fprintf(stderrSink, "  bill-segment windows %s from %d mentions\n",
		describeWindows(windows), mentions)
	return nil
}
