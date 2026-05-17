package jobs

import (
	"context"
	"fmt"
)

// SegmentTranscript assigns agenda_item_id to transcript segments that
// fall within the bill-discussion window. Per Blueprint Step 5 priority
// order:
//
//   1. (Skipped) agenda timestamps from Committee Schedules — out of scope.
//   2. Bill-number regex over transcript text.
//   3. Operator-supplied [start_ms, end_ms] override from selected_demo.yml.
//
// Heuristic: when bill-number mentions exist, take the span from the first
// to the last mention plus a small window (60s) on each side. The first
// page only needs a single visible "bill discussion segment", and a tight
// window minimizes false-positive speaker matches downstream.
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
		_, err := p.Store.AssignSegmentsToAgendaItem(ctx,
			ids.TVWEventID, ids.AgendaItemID,
			ids.BillSegmentStart, ids.BillSegmentEnd)
		return err
	}

	// Bill-number regex path.
	hits, err := p.Store.FindBillNumberMentions(ctx, ids.TVWEventID, p.Demo.BillPrefix, p.Demo.BillNumber)
	if err != nil {
		return err
	}
	if len(hits) == 0 {
		fmt.Fprintf(stderrSink, "  no transcript mentions of %s %d; bill segment unset (set transcript_override in selected_demo.yml to override)\n",
			p.Demo.BillPrefix, p.Demo.BillNumber)
		return nil
	}
	const windowMS = 60_000
	start := hits[0] - windowMS
	if start < 0 {
		start = 0
	}
	end := hits[len(hits)-1] + windowMS

	ids.BillSegmentStart, ids.BillSegmentEnd = start, end
	rows, err := p.Store.AssignSegmentsToAgendaItem(ctx, ids.TVWEventID, ids.AgendaItemID, start, end)
	if err != nil {
		return err
	}
	fmt.Fprintf(stderrSink, "  bill-segment [%d, %d] from %d mentions; %d segments tagged\n",
		start, end, len(hits), rows)
	return nil
}
