package jobs

import (
	"context"
	"fmt"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

// SegmentTranscript assigns agenda_item_id to transcript segments that
// fall within detected bill-discussion windows. Manual overrides still
// win, but the automatic path supports multiple windows for the same
// bill in one TVW event instead of a single first-mention-to-last-
// mention span. Windows are persisted to agenda_item_window so the
// page assemblers read them back literally.
func (p *Pipeline) SegmentTranscript(ctx context.Context, ids *IDs) error {
	if ids.AgendaItemID == 0 {
		return fmt.Errorf("segment-transcript: AgendaItemID not set; ingest-csi must run first")
	}
	if ids.TVWEventID == "" {
		return fmt.Errorf("segment-transcript: TVWEventID not set; ingest-tvw must run first")
	}

	// Manual override wins outright (Blueprint line 596 endorses this).
	if p.Demo.TranscriptOverride.EndMS > p.Demo.TranscriptOverride.StartMS {
		ids.BillSegmentStart = p.Demo.TranscriptOverride.StartMS
		ids.BillSegmentEnd = p.Demo.TranscriptOverride.EndMS
		fmt.Fprintf(stderrSink, "  using transcript_override [%d, %d]\n",
			ids.BillSegmentStart, ids.BillSegmentEnd)
		_, err := p.Store.AssignSegmentsToAgendaItemWindows(ctx,
			ids.TVWEventID, ids.AgendaItemID,
			[]db.AgendaItemWindow{{
				StartMS:  ids.BillSegmentStart,
				EndMS:    ids.BillSegmentEnd,
				Mentions: 0, // operator-supplied; mention count not meaningful here
			}})
		return err
	}

	cues, err := p.Store.ListTranscriptCues(ctx, ids.TVWEventID)
	if err != nil {
		return err
	}
	jobCues := make([]segmentCue, 0, len(cues))
	for _, c := range cues {
		jobCues = append(jobCues, segmentCue{StartMS: c.StartMS, EndMS: c.EndMS, Text: c.Text})
	}
	windows := DetectBillDiscussionWindows(jobCues, p.Demo.BillPrefix, p.Demo.BillNumber)
	if len(windows) == 0 {
		fmt.Fprintf(stderrSink, "  no transcript mentions of %s %d; bill segment unset (set TranscriptOverride on the selected demo to override)\n",
			p.Demo.BillPrefix, p.Demo.BillNumber)
		// Empty input still clears any stale assignments + windows
		// from a prior run that found mentions and now doesn't.
		_, err := p.Store.AssignSegmentsToAgendaItemWindows(ctx, ids.TVWEventID, ids.AgendaItemID, nil)
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
	rows, err := p.Store.AssignSegmentsToAgendaItemWindows(ctx, ids.TVWEventID, ids.AgendaItemID, dbWindows)
	if err != nil {
		return err
	}
	fmt.Fprintf(stderrSink, "  bill-segment windows %s from %d mentions; %d segments tagged\n",
		describeWindows(windows), mentions, rows)
	return nil
}
