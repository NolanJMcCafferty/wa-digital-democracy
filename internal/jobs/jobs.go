// Package jobs implements the eight build-pipeline steps from the Blueprint
// (lines 551–712). Each step is idempotent and uses idempotent upserts so
// re-running build-bundle for the same demo converges to the same DB state.
//
// Steps:
//
//   1. IngestBill            LWS bundle → bill, legislator, bill_sponsor, status timeline
//   2. EnrichSchedules       (skipped at runtime when operator provides TVW event ID)
//   3. IngestCSI              CSI agenda + testifiers → hearing, agenda_item, testifier
//   4. IngestTVW              Invintus event detail + VTT → tvw_event, transcript_segment
//   5. SegmentTranscript      bill-mention regex → assign agenda_item_id to segments
//   6. MatchSpeakers          CSI testifier order around bill segment → speaker labels
//   7. PDCContext             reviewed_matches.yml → org_context_record + testifier links
//   8. BuildBundle            (in render/firstpage)
package jobs

import (
	"context"
	"fmt"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/config"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/csi"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/lws"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/pdc"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/tvw"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

// Pipeline holds the dependencies the steps need.
type Pipeline struct {
	Store *db.Store
	LWS   *lws.Client
	CSI   *csi.Client
	TVW   *tvw.Client
	PDC   *pdc.Client
	Demo  *config.SelectedDemo

	// SourceRecordIDFor returns the source_record id for the connector's
	// most recent fetch of a given URL. Populated during a run by reading
	// the source_record row that the RawSink wrote. We maintain it here
	// because connectors don't return source_record IDs through their
	// public API — only through the httpx.RawFetch returned from Do.
	//
	// Phase 4 jobs work around that by re-fetching with httpx.RawFetch
	// captured directly. See ingest_bill.go for the pattern.
}

// IDs is the running set of database IDs the pipeline accumulates as it
// progresses. Each step both reads and writes to this struct, so Step 8 can
// reach all rows it needs to render the bundle.
type IDs struct {
	BillID         int64
	HearingID      int64
	AgendaItemID   int64
	TVWEventID     string // string identifier, not a row id
	OrgIDs         map[string]int64 // canonical_name → organization.id
	BillSegmentStart int             // ms; populated by SegmentTranscript
	BillSegmentEnd   int             // ms; populated by SegmentTranscript
}

// NewIDs returns a zero-value IDs ready for use.
func NewIDs() *IDs {
	return &IDs{OrgIDs: map[string]int64{}}
}

// Run executes the eight steps in order, stopping on the first error.
// Per-step instrumentation (ingestion_run rows) is the caller's job.
func (p *Pipeline) Run(ctx context.Context, log func(string), ids *IDs) error {
	steps := []struct {
		name string
		fn   func(context.Context, *IDs) error
	}{
		{"ingest-bill", p.IngestBill},
		{"ingest-csi", p.IngestCSI},
		{"ingest-tvw", p.IngestTVW},
		{"segment-transcript", p.SegmentTranscript},
		{"match-speakers", p.MatchSpeakers},
		{"pdc-context", p.PDCContext},
	}
	for _, s := range steps {
		log(fmt.Sprintf("==> %s", s.name))
		runID, err := p.Store.StartIngestionRun(ctx, s.name, map[string]any{
			"biennium": p.Demo.Biennium, "bill": p.Demo.BillID(),
			"agenda_item_id": p.Demo.Agenda.CSIAgendaItemID,
		})
		if err != nil {
			return fmt.Errorf("start run %s: %w", s.name, err)
		}
		stepErr := s.fn(ctx, ids)
		status := "succeeded"
		if stepErr != nil {
			status = "failed"
		}
		if err := p.Store.FinishIngestionRun(ctx, runID, status, 0, 0, stepErr); err != nil {
			log(fmt.Sprintf("warning: finish run %s: %v", s.name, err))
		}
		if stepErr != nil {
			return fmt.Errorf("%s: %w", s.name, stepErr)
		}
	}
	return nil
}

// RunMetadataOnly runs only the LWS-bill ingestion step. Used by the
// biennium-wide `wa-dd ingest-session` driver where we ingest metadata
// for every bill in the session but skip CSI/TVW/segment/match/PDC —
// those steps require operator-curated agenda + TVW IDs that aren't in
// scope for the bulk-metadata pass. Same `ingestion_run` instrumentation
// as Run; same per-step error semantics.
func (p *Pipeline) RunMetadataOnly(ctx context.Context, log func(string), ids *IDs) error {
	const step = "ingest-bill"
	log(fmt.Sprintf("==> %s", step))
	runID, err := p.Store.StartIngestionRun(ctx, step, map[string]any{
		"biennium": p.Demo.Biennium, "bill": p.Demo.BillID(),
		"mode": "metadata-only",
	})
	if err != nil {
		return fmt.Errorf("start run %s: %w", step, err)
	}
	stepErr := p.IngestBill(ctx, ids)
	status := "succeeded"
	if stepErr != nil {
		status = "failed"
	}
	if err := p.Store.FinishIngestionRun(ctx, runID, status, 0, 0, stepErr); err != nil {
		log(fmt.Sprintf("warning: finish run %s: %v", step, err))
	}
	if stepErr != nil {
		return fmt.Errorf("%s: %w", step, stepErr)
	}
	return nil
}

// pInt64 wraps a non-zero id in *int64, or returns nil for 0.
func pInt64(id int64) *int64 {
	if id == 0 {
		return nil
	}
	return &id
}
