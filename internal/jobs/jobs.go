// Package jobs implements the ingestion pipeline steps. Each step is designed
// around idempotent upserts where the underlying source data has stable keys.
//
// Step primitives:
//
//  1. IngestBill            LWS metadata → bill, bill_sponsor, status timeline
//  2. IngestCSI             CSI agenda + testifiers → hearing, agenda_item, testifier
//  3. IngestTVW             Invintus event detail → tvw_event, tvw_media_asset
//  4. SegmentTranscript     bill-window detection → agenda_item_window
//  5. PopulateOrganizations CSI organization strings → organization + testifier links
//
// `wa-dd ingest-session` uses IngestBill through RunMetadataOnly. `wa-dd
// ingest-hearings` orchestrates the hearing steps directly at hearing scope:
// CSI per agenda item, TVW and diarization once per TVW event, then
// segmentation per agenda item.
package jobs

import (
	"context"
	"fmt"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/common"
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
	Demo  *common.BillAgendaTarget

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
// progresses. Each step both reads and writes to this struct so later pipeline steps can
// use database IDs found or created by earlier steps.
type IDs struct {
	BillID           int64
	HearingID        int64
	AgendaItemID     int64
	TVWEventID       string           // string identifier, not a row id
	OrgIDs           map[string]int64 // canonical_name → organization.id
	BillSegmentStart int              // ms; populated by SegmentTranscript
	BillSegmentEnd   int              // ms; populated by SegmentTranscript
}

// NewIDs returns a zero-value IDs ready for use.
func NewIDs() *IDs {
	return &IDs{OrgIDs: map[string]int64{}}
}

// RunMetadataOnly runs only the LWS-bill ingestion step. Used by the
// biennium-wide `wa-dd ingest-session` driver where we ingest metadata
// for every bill in the session but skip CSI/TVW/segment/org work.
func (p *Pipeline) RunMetadataOnly(ctx context.Context, log func(string), ids *IDs) error {
	const step = "ingest-bill"
	log(fmt.Sprintf("==> %s", step))
	runID, err := p.Store.StartIngestionRun(ctx, step, map[string]any{
		"biennium": p.Demo.Bill.Biennium, "bill": p.Demo.Bill.ID(),
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
