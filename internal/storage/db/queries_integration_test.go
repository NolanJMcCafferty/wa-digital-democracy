package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

// dbCleanup is a no-op shim retained so existing tests compile unchanged. The
// testcontainers harness in testmain_test.go now restores the database from a
// post-migration snapshot in t.Cleanup, so per-FK row deletion is unnecessary.
type dbCleanup struct{}

func newDBCleanup(_ *testing.T, _ *db.Store) *dbCleanup { return &dbCleanup{} }

// insertProvenance creates a fake source_record we can FK against.
func insertProvenance(t *testing.T, store *db.Store, _ *dbCleanup, system, hash string) int64 {
	t.Helper()
	id, err := store.InsertSourceRecord(context.Background(), db.SourceRecordParams{
		System:      system,
		Endpoint:    "test." + system,
		URL:         "http://example/" + hash,
		FetchedAt:   time.Now().UTC(),
		ContentHash: hash,
		RawPath:     system + "/" + hash + ".bin",
		ContentType: "application/json",
	})
	if err != nil {
		t.Fatalf("source_record: %v", err)
	}
	return id
}

func TestUpsertBill_Idempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	cleanup := newDBCleanup(t, store)
	srID := insertProvenance(t, store, cleanup, "lws", "bill-test-1")

	id1, err := store.UpsertBill(ctx, db.UpsertBillParams{
		Biennium: "9999-99", Prefix: "HB", Number: 9990,
		Title: "First version", ChamberOrigin: "House",
		StatusDate:     time.Date(2025, 1, 13, 0, 0, 0, 0, time.UTC),
		SourceRecordID: srID,
	})
	if err != nil {
		t.Fatalf("upsert 1: %v", err)
	}
	id2, err := store.UpsertBill(ctx, db.UpsertBillParams{
		Biennium: "9999-99", Prefix: "HB", Number: 9990,
		Title: "Second version", ChamberOrigin: "House",
		SourceRecordID: srID,
	})
	if err != nil {
		t.Fatalf("upsert 2: %v", err)
	}
	if id1 != id2 {
		t.Errorf("ids differ: %d vs %d", id1, id2)
	}
}

func TestReplaceTestifiers_ReplacesOnSecondCall(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	cleanup := newDBCleanup(t, store)
	srID := insertProvenance(t, store, cleanup, "csi", "test-csi-1")

	// Need a hearing + agenda_item to satisfy FKs.
	hearingID, err := store.UpsertHearing(ctx, db.UpsertHearingParams{
		CommitteeName: "Test Committee", Chamber: "House",
		MeetingDateTime: time.Date(2026, 1, 1, 13, 0, 0, 0, time.UTC),
		SourceRecordID:  srID,
	})
	if err != nil {
		t.Fatalf("hearing: %v", err)
	}
	agendaID, err := store.UpsertAgendaItem(ctx, db.UpsertAgendaItemParams{
		HearingID: hearingID, Label: "HB 9991 Test", CSIAgendaItemID: "csi-test-9991",
		SourceRecordID: srID,
	})
	if err != nil {
		t.Fatalf("agenda: %v", err)
	}

	first := []db.InsertTestifierParams{
		{AgendaItemID: agendaID, RawName: "Alice", Position: "Pro", Testified: true, SourceRecordID: srID},
		{AgendaItemID: agendaID, RawName: "Bob", Position: "Con", Testified: true, SourceRecordID: srID},
	}
	if err := store.ReplaceTestifiersForAgenda(ctx, agendaID, first); err != nil {
		t.Fatalf("replace 1: %v", err)
	}
	count := func() int {
		var n int
		store.Pool.QueryRow(ctx, `SELECT count(*) FROM testifier WHERE agenda_item_id = $1`, agendaID).Scan(&n)
		return n
	}
	if got := count(); got != 2 {
		t.Errorf("count after first = %d, want 2", got)
	}
	second := []db.InsertTestifierParams{
		{AgendaItemID: agendaID, RawName: "Carol", Position: "Other", Testified: false, SourceRecordID: srID},
	}
	if err := store.ReplaceTestifiersForAgenda(ctx, agendaID, second); err != nil {
		t.Fatalf("replace 2: %v", err)
	}
	if got := count(); got != 1 {
		t.Errorf("count after replace = %d, want 1 (Alice/Bob removed, Carol present)", got)
	}
}

func TestListDiscoveredAgendaItems_UsesCurrentAndLegacyCompletionMarkers(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	cleanup := newDBCleanup(t, store)
	srID := insertProvenance(t, store, cleanup, "lws", "discovered-agenda-items-test-1")

	billID, err := store.UpsertBill(ctx, db.UpsertBillParams{
		Biennium: "9999-99", Prefix: "HB", Number: 9992,
		Title: "Retry Marker Test", ChamberOrigin: "House",
		SourceRecordID: srID,
	})
	if err != nil {
		t.Fatalf("bill: %v", err)
	}
	makeAgenda := func(tvwEventID, csiAgendaID string) {
		t.Helper()
		hearingID, err := store.UpsertHearing(ctx, db.UpsertHearingParams{
			BillID: pInt64Test(billID), CommitteeName: "Retry Marker Committee", Chamber: "House",
			MeetingDateTime: time.Now().UTC().Add(time.Duration(len(csiAgendaID)) * time.Minute),
			TVWEventID:      tvwEventID,
			SourceRecordID:  srID,
		})
		if err != nil {
			t.Fatalf("hearing %s: %v", csiAgendaID, err)
		}
		if _, err := store.UpsertAgendaItem(ctx, db.UpsertAgendaItemParams{
			HearingID: hearingID, BillID: pInt64Test(billID), Label: "HB 9992 Retry Marker",
			CSIAgendaItemID: csiAgendaID, SourceRecordID: srID,
		}); err != nil {
			t.Fatalf("agenda %s: %v", csiAgendaID, err)
		}
	}
	makeAgenda("tvw-current", "csi-current-complete")
	makeAgenda("tvw-legacy", "csi-legacy-complete")
	makeAgenda("tvw-pending", "csi-pending")

	currentRun, err := store.StartIngestionRun(ctx, "populate-organizations", map[string]any{"agenda_item_id": "csi-current-complete"})
	if err != nil {
		t.Fatalf("start current run: %v", err)
	}
	if err := store.FinishIngestionRun(ctx, currentRun, "succeeded", 0, 0, nil); err != nil {
		t.Fatalf("finish current run: %v", err)
	}
	legacyRun, err := store.StartIngestionRun(ctx, "pdc-context", map[string]any{"agenda_item_id": "csi-legacy-complete"})
	if err != nil {
		t.Fatalf("start legacy run: %v", err)
	}
	if err := store.FinishIngestionRun(ctx, legacyRun, "succeeded", 0, 0, nil); err != nil {
		t.Fatalf("finish legacy run: %v", err)
	}

	rows, err := store.ListDiscoveredAgendaItems(ctx, "9999-99")
	if err != nil {
		t.Fatalf("list discovered agenda items: %v", err)
	}
	if len(rows) != 1 || rows[0].CSIAgendaItemID != "csi-pending" {
		t.Fatalf("rows = %#v, want only csi-pending", rows)
	}
}

func pInt64Test(v int64) *int64 { return &v }

func TestUpsertTVWEventAndMediaAssets_Idempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	cleanup := newDBCleanup(t, store)
	srID := insertProvenance(t, store, cleanup, "invintus", "tvw-media-test-1")

	wpID := int64(77334)
	if _, err := store.UpsertTVWEvent(ctx, db.UpsertTVWEventParams{
		TVWEventID:          "test-media-event",
		WPPostID:            &wpID,
		WPSlug:              "senate-housing-test-media-event",
		WPLink:              "https://tvw.org/video/senate-housing-test-media-event/",
		Title:               "Senate Housing",
		Description:         "Public hearing",
		StartDateTime:       time.Date(2026, 2, 4, 18, 30, 0, 0, time.UTC),
		CaptionURL:          "https://example.com/caption.vtt",
		ThumbnailURL:        "https://example.com/thumb.jpg",
		CustomID:            "33828",
		LocationName:        "Senate Hearing Rm 4 and Virtual",
		TotalRuntime:        "01:30:01",
		TotalRuntimeSeconds: 5401,
		PublishedAudioURL:   "https://example.com/audio.mp3",
		VideoDownloadURL:    "https://example.com/video.mp4",
		StreamingURIs:       map[string]any{"main": "https://example.com/media.m3u8"},
		RawCategories:       []string{"Legislative", "Senate Housing"},
		RawKeywords:         []string{"1501"},
		RawWPTags:           []int{7507},
		RawWPCategories:     []int{6090},
		SourceRecordID:      srID,
	}); err != nil {
		t.Fatalf("upsert tvw_event: %v", err)
	}
	rows := []db.UpsertTVWMediaAssetParams{
		{TVWEventID: "test-media-event", AssetID: "caption-1", AssetType: "caption", Name: "caption.vtt", FileURL: "https://example.com/caption.vtt", SourceRecordID: srID},
		{
			TVWEventID:          "test-media-event",
			AssetID:             "video-1",
			AssetType:           "video",
			Name:                "Edit",
			FileURL:             "https://example.com/video.mp4",
			FileSizeBytes:       2213283547,
			TotalRuntime:        "01:30:01",
			TotalRuntimeSeconds: 5401,
			AdvancedDetails:     map[string]any{"audio": []any{map[string]any{"channels": "2.0ch"}}},
			SourceRecordID:      srID,
		},
	}
	if err := store.ReplaceTVWMediaAssets(ctx, "test-media-event", rows); err != nil {
		t.Fatalf("replace media assets: %v", err)
	}
	rows = rows[:1]
	rows[0].Name = "caption-updated.vtt"
	if err := store.ReplaceTVWMediaAssets(ctx, "test-media-event", rows); err != nil {
		t.Fatalf("replace media assets 2: %v", err)
	}

	var slug, stream, tags string
	if err := store.Pool.QueryRow(ctx, `
SELECT wp_slug, streaming_uris->>'main', raw_wp_tags::text
  FROM tvw_event WHERE tvw_event_id = 'test-media-event';`).Scan(&slug, &stream, &tags); err != nil {
		t.Fatalf("query tvw_event: %v", err)
	}
	if slug != "senate-housing-test-media-event" || stream == "" || tags != "[7507]" {
		t.Fatalf("slug=%q stream=%q tags=%q", slug, stream, tags)
	}
	var count int
	var captionName string
	if err := store.Pool.QueryRow(ctx, `
SELECT count(*), max(name) FROM tvw_media_asset WHERE tvw_event_id = 'test-media-event';`).Scan(&count, &captionName); err != nil {
		t.Fatalf("query assets: %v", err)
	}
	if count != 1 || captionName != "caption-updated.vtt" {
		t.Fatalf("count=%d captionName=%q", count, captionName)
	}
}

func TestFindBillNumberMentions(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	cleanup := newDBCleanup(t, store)
	srID := insertProvenance(t, store, cleanup, "tvw", "test-tvw-1")

	tvwEventID := "test-99999999"
	if _, err := store.UpsertTVWEvent(ctx, db.UpsertTVWEventParams{
		TVWEventID: tvwEventID, Title: "Test", SourceRecordID: srID,
	}); err != nil {
		t.Fatalf("upsert tvw_event: %v", err)
	}

	rows := []db.InsertTranscriptSegmentParams{
		{TVWEventID: tvwEventID, StartMS: 0, EndMS: 1000, Text: "Welcome to the meeting.", SourceCaptionURL: "u", SourceRecordID: srID},
		{TVWEventID: tvwEventID, StartMS: 1000, EndMS: 2000, Text: "We will now consider HB 1234.", SourceCaptionURL: "u", SourceRecordID: srID},
		{TVWEventID: tvwEventID, StartMS: 2000, EndMS: 3000, Text: "Moving on to House Bill 5678 next.", SourceCaptionURL: "u", SourceRecordID: srID},
		{TVWEventID: tvwEventID, StartMS: 3000, EndMS: 4000, Text: "Questions on HB 1234?", SourceCaptionURL: "u", SourceRecordID: srID},
	}
	if err := store.ReplaceTranscriptSegments(ctx, tvwEventID, rows); err != nil {
		t.Fatalf("replace segments: %v", err)
	}

	got, err := store.FindBillNumberMentions(ctx, tvwEventID, "HB", 1234)
	if err != nil {
		t.Fatalf("FindBillNumberMentions: %v", err)
	}
	if len(got) != 2 || got[0] != 1000 || got[1] != 3000 {
		t.Fatalf("hits = %v, want [1000 3000]", got)
	}

	got, _ = store.FindBillNumberMentions(ctx, tvwEventID, "HB", 5678)
	if len(got) != 1 || got[0] != 2000 {
		t.Fatalf("HB 5678 hits = %v, want [2000]", got)
	}
}

func TestUpsertDataWAContract_Idempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	cleanup := newDBCleanup(t, store)
	srID := insertProvenance(t, store, cleanup, "datawa_socrata", "datawa-contract-test-1")

	params := db.UpsertDataWAContractParams{
		SourceDatasetID: "test-contracts",
		SourceRowID:     "row-1",
		FiscalYear:      2025,
		AgencyName:      "Dept",
		ContractorName:  "Vendor A",
		TotalAmount:     "123.45",
		RawFields:       map[string]any{"source": "first"},
		SourceRecordID:  srID,
	}
	if err := store.UpsertDataWAContract(ctx, params); err != nil {
		t.Fatalf("upsert 1: %v", err)
	}
	params.ContractorName = "Vendor B"
	params.TotalAmount = "456.78"
	params.RawFields = map[string]any{"source": "second"}
	if err := store.UpsertDataWAContract(ctx, params); err != nil {
		t.Fatalf("upsert 2: %v", err)
	}

	var count int
	var contractor string
	var amount string
	if err := store.Pool.QueryRow(ctx, `
SELECT count(*), max(contractor_name), max(total_amount)::text
  FROM datawa_contract
 WHERE source_dataset_id = 'test-contracts' AND source_row_id = 'row-1';`).Scan(&count, &contractor, &amount); err != nil {
		t.Fatalf("query contract: %v", err)
	}
	if count != 1 || contractor != "Vendor B" || amount != "456.78" {
		t.Fatalf("count=%d contractor=%q amount=%q", count, contractor, amount)
	}
}

func TestUpsertDataWAMasterContractSale_Idempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	cleanup := newDBCleanup(t, store)
	srID := insertProvenance(t, store, cleanup, "datawa_socrata", "datawa-master-sale-test-1")

	params := db.UpsertDataWAMasterContractSaleParams{
		SourceDatasetID:    "n8q6-4twj",
		SourceRowID:        "row-1",
		CustomerType:       "State Agency",
		CustomerName:       "TRANSPORTATION DEPT OF",
		ContractNumber:     "00111",
		ContractTitle:      "Fertilizers",
		VendorName:         "Vendor A",
		ReportYear:         2015,
		Q1SalesReported:    "1.00",
		Q2SalesReported:    "2.00",
		Q3SalesReported:    "3.00",
		Q4SalesReported:    "4.00",
		TotalSalesReported: "10.00",
		RawFields:          map[string]any{"source": "first"},
		SourceRecordID:     srID,
	}
	if err := store.UpsertDataWAMasterContractSale(ctx, params); err != nil {
		t.Fatalf("upsert 1: %v", err)
	}
	params.VendorName = "Vendor B"
	params.TotalSalesReported = "20.00"
	params.RawFields = map[string]any{"source": "second"}
	if err := store.UpsertDataWAMasterContractSale(ctx, params); err != nil {
		t.Fatalf("upsert 2: %v", err)
	}

	var count int
	var vendor string
	var amount string
	if err := store.Pool.QueryRow(ctx, `
SELECT count(*), max(vendor_name), max(total_sales_reported)::text
  FROM datawa_master_contract_sale
 WHERE source_dataset_id = 'n8q6-4twj' AND source_row_id = 'row-1';`).Scan(&count, &vendor, &amount); err != nil {
		t.Fatalf("query sale: %v", err)
	}
	if count != 1 || vendor != "Vendor B" || amount != "20.00" {
		t.Fatalf("count=%d vendor=%q amount=%q", count, vendor, amount)
	}
}

func TestUpsertDataWAITContract_Idempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	cleanup := newDBCleanup(t, store)
	srID := insertProvenance(t, store, cleanup, "datawa_socrata", "datawa-it-contract-test-1")
	coop := true

	params := db.UpsertDataWAITContractParams{
		SourceDatasetID:           "3txe-z9i9",
		SourceRowID:               "row-1",
		ReportFiscalYear:          2025,
		AgencyNumberAgencyName:    "086 - Governor's Office of Indian Affairs (INA)",
		AgencyNumber:              "086",
		AgencyName:                "Governor's Office of Indian Affairs (INA)",
		ContractNumber:            "04718",
		ContractorName:            "Vendor A",
		CooperativePurchase:       &coop,
		StatewideContractPurchase: &coop,
		ITTowerApplication:        "0.2",
		ITTowerNetwork:            "0.4",
		ContractAmountFY25:        "4864.66",
		TotalContractAmount:       "71081.27",
		RawFields:                 map[string]any{"source": "first"},
		SourceRecordID:            srID,
	}
	if err := store.UpsertDataWAITContract(ctx, params); err != nil {
		t.Fatalf("upsert 1: %v", err)
	}
	params.ContractorName = "Vendor B"
	params.TotalContractAmount = "80000.00"
	params.RawFields = map[string]any{"source": "second"}
	if err := store.UpsertDataWAITContract(ctx, params); err != nil {
		t.Fatalf("upsert 2: %v", err)
	}

	var count int
	var contractor string
	var total string
	if err := store.Pool.QueryRow(ctx, `
SELECT count(*), max(contractor_name), max(total_contract_amount)::text
  FROM datawa_it_contract
 WHERE source_dataset_id = '3txe-z9i9' AND source_row_id = 'row-1';`).Scan(&count, &contractor, &total); err != nil {
		t.Fatalf("query it contract: %v", err)
	}
	if count != 1 || contractor != "Vendor B" || total != "80000.00" {
		t.Fatalf("count=%d contractor=%q total=%q", count, contractor, total)
	}
}

func TestGenerateVendorEntityMatchCandidates(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	cleanup := newDBCleanup(t, store)
	srID := insertProvenance(t, store, cleanup, "datawa_socrata", "vendor-match-test-1")

	orgID, err := store.UpsertOrganization(ctx, db.UpsertOrganizationParams{
		CanonicalName:   "Acme Technologies Inc.",
		Aliases:         []string{"Acme Tech"},
		MatchConfidence: "confirmed",
		MatchNotes:      "integration test",
	})
	if err != nil {
		t.Fatalf("upsert org: %v", err)
	}
	defer store.Pool.Exec(ctx, `DELETE FROM organization WHERE id = $1`, orgID)

	if err := store.UpsertDataWAContract(ctx, db.UpsertDataWAContractParams{
		SourceDatasetID: "test-contracts",
		SourceRowID:     "vendor-match-row-1",
		FiscalYear:      2025,
		AgencyName:      "Dept",
		ContractorName:  "ACME TECHNOLOGIES LLC",
		TotalAmount:     "100.00",
		RawFields:       map[string]any{"source": "contract"},
		SourceRecordID:  srID,
	}); err != nil {
		t.Fatalf("upsert contract: %v", err)
	}

	candidates, err := store.GenerateVendorEntityMatchCandidates(ctx, 100)
	if err != nil {
		t.Fatalf("GenerateVendorEntityMatchCandidates: %v", err)
	}
	var found *db.VendorEntityMatchCandidate
	for i := range candidates {
		if candidates[i].SourceRowID == "vendor-match-row-1" && candidates[i].OrganizationID == orgID {
			found = &candidates[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected candidate for ACME contract; got %#v", candidates)
	}
	if found.CandidateConfidence != "probable" {
		t.Fatalf("confidence = %q, want probable", found.CandidateConfidence)
	}

	if _, err := store.UpsertVendorEntityMatchDecision(ctx, db.InsertVendorEntityMatchDecisionParams{
		CandidateID:    found.ID,
		OrganizationID: orgID,
		Decision:       "confirmed",
		Confidence:     "confirmed",
		ReviewedBy:     "integration-test",
		ReviewNotes:    "obvious normalized-name match",
	}); err != nil {
		t.Fatalf("decision: %v", err)
	}
	var reviewed int
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM reviewed_vendor_entity_match WHERE candidate_id = $1`, found.ID).Scan(&reviewed); err != nil {
		t.Fatalf("reviewed query: %v", err)
	}
	if reviewed != 1 {
		t.Fatalf("reviewed count = %d, want 1", reviewed)
	}
}

func TestUpsertDataWAWEBSVendor_Idempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	cleanup := newDBCleanup(t, store)
	srID := insertProvenance(t, store, cleanup, "datawa_socrata", "datawa-webs-vendor-test-1")

	params := db.UpsertDataWAWEBSVendorParams{
		SourceDatasetID:       "3kwi-7zsj",
		SourceRowID:           "row-1",
		CompanyName:           "Sunrise Technologies, Inc.",
		NormalizedCompanyName: "SUNRISE TECHNOLOGIES, INC.",
		DBAName:               "Sunrise Integrated Solutions",
		ContactEmail:          "kdavis@sunrisetechnologies.com",
		City:                  "Folsom",
		State:                 "CA",
		CommodityCode:         "918-71",
		DescriptionOfWork:     "IT Consulting",
		SmallBusiness:         "Y",
		VeteranOwned:          "N",
		RawFields:             map[string]any{"source": "first"},
		SourceRecordID:        srID,
	}
	if err := store.UpsertDataWAWEBSVendor(ctx, params); err != nil {
		t.Fatalf("upsert 1: %v", err)
	}
	params.CompanyName = "Sunrise Technologies Updated"
	params.NormalizedCompanyName = "SUNRISE TECHNOLOGIES UPDATED"
	params.RawFields = map[string]any{"source": "second"}
	if err := store.UpsertDataWAWEBSVendor(ctx, params); err != nil {
		t.Fatalf("upsert 2: %v", err)
	}

	var count int
	var company string
	var normalized string
	if err := store.Pool.QueryRow(ctx, `
SELECT count(*), max(company_name), max(normalized_company_name)
  FROM datawa_webs_vendor
 WHERE source_dataset_id = '3kwi-7zsj' AND source_row_id = 'row-1';`).Scan(&count, &company, &normalized); err != nil {
		t.Fatalf("query vendor: %v", err)
	}
	if count != 1 || company != "Sunrise Technologies Updated" || normalized != "SUNRISE TECHNOLOGIES UPDATED" {
		t.Fatalf("count=%d company=%q normalized=%q", count, company, normalized)
	}
}

func TestUpsertFederalAward_Idempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	cleanup := newDBCleanup(t, store)
	srID := insertProvenance(t, store, cleanup, "usaspending", "usaspending-award-test-1")
	start := time.Date(2025, 10, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)

	params := db.UpsertFederalAwardParams{
		AwardID:        "ASST_NON_123",
		RecipientName:  "CITY OF SEATTLE",
		RecipientUEI:   "ABC123",
		AwardingAgency: "Department of Transportation",
		FundingAgency:  "Federal Highway Administration",
		AwardType:      "Grant",
		AwardAmount:    "12345.67",
		StartDate:      &start,
		EndDate:        &end,
		PlaceStateCode: "WA",
		PlaceCounty:    "King",
		RawFields:      map[string]any{"source": "first"},
		SourceRecordID: srID,
	}
	if err := store.UpsertFederalAward(ctx, params); err != nil {
		t.Fatalf("upsert 1: %v", err)
	}
	params.AwardAmount = "20000"
	params.RawFields = map[string]any{"source": "second"}
	if err := store.UpsertFederalAward(ctx, params); err != nil {
		t.Fatalf("upsert 2: %v", err)
	}

	var count int
	var amount string
	if err := store.Pool.QueryRow(ctx, `
SELECT count(*), max(award_amount)::text
  FROM federal_award
 WHERE award_id = 'ASST_NON_123';`).Scan(&count, &amount); err != nil {
		t.Fatalf("query award: %v", err)
	}
	if count != 1 || amount != "20000" {
		t.Fatalf("count=%d amount=%q", count, amount)
	}
}

func TestUpsertSeattleOperatingBudget_Idempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	cleanup := newDBCleanup(t, store)
	srID := insertProvenance(t, store, cleanup, "seattle_socrata", "seattle-operating-budget-test-1")

	params := db.UpsertSeattleOperatingBudgetParams{
		SourceDatasetID: "8u2j-imqx",
		SourceRowID:     "row-1",
		FiscalYear:      2026,
		Service:         "Administration",
		Department:      "Office of the City Auditor",
		Program:         "Office of the City Auditor",
		Fund:            "00100 - General Fund",
		FundType:        "General Fund",
		ExpenseType:     "Expenditures",
		Description:     "Labor",
		ApprovedAmount:  "1632174",
		RawFields:       map[string]any{"source": "first"},
		SourceRecordID:  srID,
	}
	if err := store.UpsertSeattleOperatingBudget(ctx, params); err != nil {
		t.Fatalf("upsert 1: %v", err)
	}
	params.ApprovedAmount = "1700000"
	params.RawFields = map[string]any{"source": "second"}
	if err := store.UpsertSeattleOperatingBudget(ctx, params); err != nil {
		t.Fatalf("upsert 2: %v", err)
	}

	var count int
	var amount string
	if err := store.Pool.QueryRow(ctx, `
SELECT count(*), max(approved_amount)::text
  FROM seattle_operating_budget
 WHERE source_dataset_id = '8u2j-imqx' AND source_row_id = 'row-1';`).Scan(&count, &amount); err != nil {
		t.Fatalf("query budget: %v", err)
	}
	if count != 1 || amount != "1700000" {
		t.Fatalf("count=%d amount=%q", count, amount)
	}
}

func TestUpsertFiscalWAVendorPayment_Idempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	cleanup := newDBCleanup(t, store)
	srID := insertProvenance(t, store, cleanup, "fiscal_wa", "fiscalwa-vendor-payment-test-1")

	params := db.UpsertFiscalWAVendorPaymentParams{
		SourceDatasetID: "vendor-payments-2025-27",
		SourceRowID:     "row-1",
		Biennium:        "2025-27",
		FiscalYear:      2026,
		FiscalMonth:     "01",
		AgencyNumber:    "300",
		AgencyName:      "Social and Health Services",
		ObjectCode:      "E",
		ObjectCategory:  "Goods and Services",
		SubobjectCode:   "ER",
		SubobjectName:   "Other Contractual Services",
		VendorName:      "HOME CARE MASTERS LLC",
		Amount:          "1402.27",
		RawFields:       map[string]any{"source": "first"},
		SourceRecordID:  srID,
	}
	if err := store.UpsertFiscalWAVendorPayment(ctx, params); err != nil {
		t.Fatalf("upsert 1: %v", err)
	}
	params.Amount = "1500.00"
	params.RawFields = map[string]any{"source": "second"}
	if err := store.UpsertFiscalWAVendorPayment(ctx, params); err != nil {
		t.Fatalf("upsert 2: %v", err)
	}

	var count int
	var amount string
	if err := store.Pool.QueryRow(ctx, `
SELECT count(*), max(amount)::text
  FROM fiscalwa_vendor_payment
 WHERE source_dataset_id = 'vendor-payments-2025-27' AND source_row_id = 'row-1';`).Scan(&count, &amount); err != nil {
		t.Fatalf("query payment: %v", err)
	}
	if count != 1 || amount != "1500.00" {
		t.Fatalf("count=%d amount=%q", count, amount)
	}
}
