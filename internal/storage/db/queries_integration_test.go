package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func TestUpsertBill_Idempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	id1, err := store.UpsertBill(ctx, db.UpsertBillParams{
		Biennium: "9999-99", Prefix: "HB", Number: 9990,
		Title: "First version", ChamberOrigin: "House",
		StatusDate: time.Date(2025, 1, 13, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("upsert 1: %v", err)
	}
	id2, err := store.UpsertBill(ctx, db.UpsertBillParams{
		Biennium: "9999-99", Prefix: "HB", Number: 9990,
		Title: "Second version", ChamberOrigin: "House",
	})
	if err != nil {
		t.Fatalf("upsert 2: %v", err)
	}
	if id1 != id2 {
		t.Errorf("ids differ: %d vs %d", id1, id2)
	}
}

func TestUpsertOrganization_MergesAliases(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	orgID, err := store.UpsertOrganization(ctx, db.UpsertOrganizationParams{
		CanonicalName:   "Alias Merge Test Organization",
		Aliases:         []string{"AMTO", "Alias Merge Test Org"},
		MatchConfidence: "possible",
		MatchNotes:      "first",
	})
	if err != nil {
		t.Fatalf("upsert org 1: %v", err)
	}
	if _, err := store.UpsertOrganization(ctx, db.UpsertOrganizationParams{
		CanonicalName:   "Alias Merge Test Organization",
		Aliases:         []string{"amto", "Alias Merge Coalition"},
		MatchConfidence: "possible",
		MatchNotes:      "second",
	}); err != nil {
		t.Fatalf("upsert org 2: %v", err)
	}
	if _, err := store.UpsertOrganization(ctx, db.UpsertOrganizationParams{
		CanonicalName:   "Alias Merge Test Organization",
		Aliases:         []string{},
		MatchConfidence: "possible",
		MatchNotes:      "third",
	}); err != nil {
		t.Fatalf("upsert org 3: %v", err)
	}

	var aliases []string
	if err := store.Pool.QueryRow(ctx, `SELECT aliases FROM organization WHERE id = $1`, orgID).Scan(&aliases); err != nil {
		t.Fatalf("read aliases: %v", err)
	}
	for _, want := range []string{"AMTO", "Alias Merge Test Org", "Alias Merge Coalition"} {
		if !hasString(aliases, want) {
			t.Fatalf("aliases = %#v, want %q", aliases, want)
		}
	}
	if hasString(aliases, "amto") {
		t.Fatalf("aliases = %#v, duplicate case-variant alias was not deduped", aliases)
	}
}

func TestUpsertPDCLobbyistAffiliation_MarksSourceBackedLobbyistConfirmed(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	orgID, err := store.UpsertOrganization(ctx, db.UpsertOrganizationParams{
		CanonicalName:         "Confirmed PDC Employer Association",
		PDCLobbyistEmployerID: "EMP-PDC-CONFIRMED-1",
		MatchConfidence:       "confirmed",
		MatchNotes:            "fixture",
	})
	if err != nil {
		t.Fatalf("upsert organization: %v", err)
	}

	if err := store.UpsertPDCLobbyistAffiliation(ctx, db.UpsertPDCLobbyistAffiliationParams{
		ReportNumber:     "R-PDC-CONFIRMED-1",
		LobbyistID:       "L-PDC-CONFIRMED-1",
		LobbyistName:     "Source, Pat",
		EmployerID:       "EMP-PDC-CONFIRMED-1",
		EmployerName:     "Confirmed PDC Employer Association",
		EmploymentYear:   "2026",
		EmploymentURL:    "https://web.pdc.wa.gov/example/confirmed",
		EmploymentPeriod: "Annual",
		Raw:              map[string]any{"source": "pdc"},
	}); err != nil {
		t.Fatalf("upsert PDC lobbyist affiliation: %v", err)
	}

	var personConfidence string
	if err := store.Pool.QueryRow(ctx, `
SELECT match_confidence::text
  FROM person
 WHERE pdc_lobbyist_id = 'L-PDC-CONFIRMED-1';`).Scan(&personConfidence); err != nil {
		t.Fatalf("read person confidence: %v", err)
	}
	if personConfidence != "confirmed" {
		t.Fatalf("person confidence = %q, want confirmed", personConfidence)
	}

	var affiliationConfidence string
	if err := store.Pool.QueryRow(ctx, `
SELECT confidence::text
  FROM person_organization_affiliation
 WHERE organization_id = $1
   AND source_kind = 'pdc_lobbyist_employment'
   AND relationship_type = 'lobbyist_for';`, orgID).Scan(&affiliationConfidence); err != nil {
		t.Fatalf("read affiliation confidence: %v", err)
	}
	if affiliationConfidence != "confirmed" {
		t.Fatalf("affiliation confidence = %q, want confirmed", affiliationConfidence)
	}
}

func TestAttachPDCEmployerAffiliationsToOrganization_BackfillsPreloadedPDC(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.UpsertPDCLobbyistAffiliation(ctx, db.UpsertPDCLobbyistAffiliationParams{
		ReportNumber:     "R-PDC-PRELOADED-1",
		LobbyistID:       "L-PDC-PRELOADED-1",
		LobbyistName:     "Preloaded, Pat",
		EmployerID:       "EMP-PDC-PRELOADED-1",
		EmployerName:     "Preloaded PDC Employer Association",
		EmploymentYear:   "2026",
		EmploymentURL:    "https://web.pdc.wa.gov/example/preloaded",
		EmploymentPeriod: "Annual",
		Raw:              map[string]any{"source": "pdc"},
	}); err != nil {
		t.Fatalf("upsert PDC lobbyist affiliation: %v", err)
	}

	var nullOrgRows int
	if err := store.Pool.QueryRow(ctx, `
SELECT COUNT(*)
  FROM person_organization_affiliation
 WHERE source_kind = 'pdc_lobbyist_employment'
   AND source_row_id = 'R-PDC-PRELOADED-1|L-PDC-PRELOADED-1|EMP-PDC-PRELOADED-1|2026'
   AND organization_id IS NULL;`).Scan(&nullOrgRows); err != nil {
		t.Fatalf("count null organization affiliations: %v", err)
	}
	if nullOrgRows != 1 {
		t.Fatalf("null organization affiliations = %d, want 1", nullOrgRows)
	}

	orgID, err := store.UpsertOrganization(ctx, db.UpsertOrganizationParams{
		CanonicalName:   "Preloaded PDC Employer Association",
		MatchConfidence: "possible",
		MatchNotes:      "fixture",
	})
	if err != nil {
		t.Fatalf("upsert organization: %v", err)
	}
	if err := store.AttachPDCEmployerAffiliationsToOrganization(ctx, orgID, "EMP-PDC-PRELOADED-1", "Preloaded PDC Employer Association"); err != nil {
		t.Fatalf("attach PDC employer affiliations: %v", err)
	}

	var attachedRows int
	if err := store.Pool.QueryRow(ctx, `
SELECT COUNT(*)
  FROM person_organization_affiliation
 WHERE source_kind = 'pdc_lobbyist_employment'
   AND source_row_id = 'R-PDC-PRELOADED-1|L-PDC-PRELOADED-1|EMP-PDC-PRELOADED-1|2026'
   AND organization_id = $1;`, orgID).Scan(&attachedRows); err != nil {
		t.Fatalf("count attached affiliations: %v", err)
	}
	if attachedRows != 1 {
		t.Fatalf("attached affiliations = %d, want 1", attachedRows)
	}
}

func TestReplaceTestifiersForAgenda_ReusesCSIPersonByNormalizedName(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	orgID, err := store.UpsertOrganization(ctx, db.UpsertOrganizationParams{
		CanonicalName:   "CSI Person Dedupe Organization",
		MatchConfidence: "possible",
		MatchNotes:      "fixture",
	})
	if err != nil {
		t.Fatalf("upsert organization: %v", err)
	}

	hearingID, err := store.UpsertHearing(ctx, db.UpsertHearingParams{
		CommitteeName: "Test Committee", Chamber: "House",
		MeetingDateTime: time.Date(2026, 3, 1, 13, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("hearing: %v", err)
	}
	firstAgendaID, err := store.UpsertAgendaItem(ctx, db.UpsertAgendaItemParams{
		HearingID: hearingID, Label: "HB 9995 Test", CSIAgendaItemID: "csi-test-9995-a",
	})
	if err != nil {
		t.Fatalf("first agenda: %v", err)
	}
	secondAgendaID, err := store.UpsertAgendaItem(ctx, db.UpsertAgendaItemParams{
		HearingID: hearingID, Label: "HB 9996 Test", CSIAgendaItemID: "csi-test-9995-b",
	})
	if err != nil {
		t.Fatalf("second agenda: %v", err)
	}

	testifier := func(agendaID int64, testified bool) []db.InsertTestifierParams {
		return []db.InsertTestifierParams{{
			AgendaItemID:    agendaID,
			RawName:         "McAleenan, Mellani",
			RawOrganization: "CSI Person Dedupe Organization",
			Position:        "Pro",
			Testified:       testified,
		}}
	}
	if err := store.ReplaceTestifiersForAgenda(ctx, firstAgendaID, testifier(firstAgendaID, false)); err != nil {
		t.Fatalf("replace first agenda: %v", err)
	}
	if err := store.ReplaceTestifiersForAgenda(ctx, secondAgendaID, testifier(secondAgendaID, true)); err != nil {
		t.Fatalf("replace second agenda: %v", err)
	}

	var personCount int
	if err := store.Pool.QueryRow(ctx, `
SELECT count(*)
  FROM person
 WHERE normalized_name = 'MCALEENAN, MELLANI';`).Scan(&personCount); err != nil {
		t.Fatalf("count CSI people: %v", err)
	}
	if personCount != 1 {
		t.Fatalf("person count = %d, want 1", personCount)
	}

	affiliations, err := store.GetOrganizationPersonAffiliations(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrganizationPersonAffiliations: %v", err)
	}
	if len(affiliations) != 2 {
		t.Fatalf("affiliation groups = %d, want 2: %#v", len(affiliations), affiliations)
	}
	for _, a := range affiliations {
		if a.PersonName != "McAleenan, Mellani" {
			t.Fatalf("person name = %q, want McAleenan, Mellani", a.PersonName)
		}
		if a.SourceCount != 1 {
			t.Fatalf("%s source count = %d, want 1", a.RelationshipType, a.SourceCount)
		}
	}

	if err := store.BackfillCSITestifierPersonAffiliationsForRawOrganizations(ctx, []string{"CSI Person Dedupe Organization"}); err != nil {
		t.Fatalf("backfill affiliations: %v", err)
	}
	affiliations, err = store.GetOrganizationPersonAffiliations(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrganizationPersonAffiliations after backfill: %v", err)
	}
	for _, a := range affiliations {
		if a.SourceCount != 1 {
			t.Fatalf("%s source count after backfill = %d, want 1", a.RelationshipType, a.SourceCount)
		}
	}
}

func TestReplaceTestifiers_ReplacesOnSecondCall(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Need a hearing + agenda_item to satisfy FKs.
	hearingID, err := store.UpsertHearing(ctx, db.UpsertHearingParams{
		CommitteeName: "Test Committee", Chamber: "House",
		MeetingDateTime: time.Date(2026, 1, 1, 13, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("hearing: %v", err)
	}
	agendaID, err := store.UpsertAgendaItem(ctx, db.UpsertAgendaItemParams{
		HearingID: hearingID, Label: "HB 9991 Test", CSIAgendaItemID: "csi-test-9991",
	})
	if err != nil {
		t.Fatalf("agenda: %v", err)
	}

	first := []db.InsertTestifierParams{
		{AgendaItemID: agendaID, RawName: "Alice", Position: "Pro", Testified: true},
		{AgendaItemID: agendaID, RawName: "Bob", Position: "Con", Testified: true},
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
		{AgendaItemID: agendaID, RawName: "Carol", Position: "Other", Testified: false},
	}
	if err := store.ReplaceTestifiersForAgenda(ctx, agendaID, second); err != nil {
		t.Fatalf("replace 2: %v", err)
	}
	if got := count(); got != 1 {
		t.Errorf("count after replace = %d, want 1 (Alice/Bob removed, Carol present)", got)
	}
}

func TestListDiscoveredHearingsForIngest_UsesHearingPipelineMarker(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	billID, err := store.UpsertBill(ctx, db.UpsertBillParams{
		Biennium: "9999-99", Prefix: "HB", Number: 9993,
		Title: "Hearing Pipeline Marker Test", ChamberOrigin: "House",
	})
	if err != nil {
		t.Fatalf("bill: %v", err)
	}
	makeHearing := func(offset time.Duration, tvwEventID string, agendaIDs ...string) int64 {
		t.Helper()
		hearingID, err := store.UpsertHearing(ctx, db.UpsertHearingParams{
			BillID: pInt64Test(billID), CommitteeName: "Hearing Pipeline Committee", Chamber: "House",
			MeetingDateTime: time.Now().UTC().Add(offset),
			TVWEventID:      tvwEventID,
		})
		if err != nil {
			t.Fatalf("hearing %s: %v", tvwEventID, err)
		}
		for _, agendaID := range agendaIDs {
			if _, err := store.UpsertAgendaItem(ctx, db.UpsertAgendaItemParams{
				HearingID: hearingID, BillID: pInt64Test(billID), Label: "HB 9993 Hearing Pipeline",
				CSIAgendaItemID: agendaID,
			}); err != nil {
				t.Fatalf("agenda %s: %v", agendaID, err)
			}
		}
		return hearingID
	}
	completeID := makeHearing(time.Minute, "tvw-hearing-complete", "csi-hearing-complete")
	pendingID := makeHearing(2*time.Minute, "tvw-hearing-pending", "csi-hearing-pending-a", "csi-hearing-pending-b")

	runID, err := store.StartIngestionRun(ctx, "hearing-pipeline", map[string]any{"hearing_id": completeID})
	if err != nil {
		t.Fatalf("start hearing run: %v", err)
	}
	if err := store.FinishIngestionRun(ctx, runID, "succeeded", 0, 0, nil); err != nil {
		t.Fatalf("finish hearing run: %v", err)
	}

	rows, err := store.ListDiscoveredHearingsForIngest(ctx, "9999-99")
	if err != nil {
		t.Fatalf("list discovered hearings: %v", err)
	}
	if len(rows) != 1 || rows[0].HearingID != pendingID || rows[0].AgendaItemCount != 2 {
		t.Fatalf("rows = %#v, want only pending hearing %d with 2 agenda items", rows, pendingID)
	}

	items, err := store.ListAgendaItemsForHearing(ctx, pendingID)
	if err != nil {
		t.Fatalf("list agenda items for hearing: %v", err)
	}
	if len(items) != 2 || items[0].BillPrefix != "HB" || items[0].TVWEventID != "tvw-hearing-pending" {
		t.Fatalf("items = %#v", items)
	}
}

func pInt64Test(v int64) *int64 { return &v }

func TestUpsertTVWEventAndMediaAssets_Idempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

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
	}); err != nil {
		t.Fatalf("upsert tvw_event: %v", err)
	}
	rows := []db.UpsertTVWMediaAssetParams{
		{TVWEventID: "test-media-event", AssetID: "caption-1", AssetType: "caption", Name: "caption.vtt", FileURL: "https://example.com/caption.vtt"},
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

func TestUpsertDataWAContract_Idempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	params := db.UpsertDataWAContractParams{
		SourceDatasetID: "test-contracts",
		SourceRowID:     "row-1",
		FiscalYear:      2025,
		AgencyName:      "Dept",
		ContractorName:  "Vendor A",
		TotalAmount:     "123.45",
		RawFields:       map[string]any{"source": "first"},
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
	// "Acme Tech" alias normalizes to "ACME TECHNOLOGIES" (TECH → TECHNOLOGIES,
	// per migration 0031), matching the source's normalized form, which
	// triggers the alias-confirmed path in entitymatch.ConfidenceFor.
	if found.CandidateConfidence != "confirmed" {
		t.Fatalf("confidence = %q, want confirmed", found.CandidateConfidence)
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
	var aliases []string
	if err := store.Pool.QueryRow(ctx, `SELECT aliases FROM organization WHERE id = $1`, orgID).Scan(&aliases); err != nil {
		t.Fatalf("read aliases: %v", err)
	}
	if !hasString(aliases, "ACME TECHNOLOGIES LLC") {
		t.Fatalf("aliases = %#v, want confirmed source name alias", aliases)
	}
}

// TestGenerateVendorEntityMatchCandidates_CSITestimonyOrganization verifies
// that distinct testifier.raw_organization strings produce csi_testimony_organization
// match candidates and that confirming one promotes the org to 'confirmed'.
func TestGenerateVendorEntityMatchCandidates_CSITestimonyOrganization(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Seed a canonical organization that matches the raw testifier string.
	orgID, err := store.UpsertOrganization(ctx, db.UpsertOrganizationParams{
		CanonicalName:   "Fixture Civic Coalition",
		Aliases:         []string{},
		MatchConfidence: "possible",
		MatchNotes:      "csi testimony seed",
	})
	if err != nil {
		t.Fatalf("upsert org: %v", err)
	}
	defer store.Pool.Exec(ctx, `DELETE FROM organization WHERE id = $1`, orgID)

	// Seed a hearing + agenda_item + testifier carrying the matching raw_organization.
	hearingID, err := store.UpsertHearing(ctx, db.UpsertHearingParams{
		CommitteeName: "Test Committee", Chamber: "House",
		MeetingDateTime: time.Date(2026, 2, 1, 13, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("hearing: %v", err)
	}
	defer store.Pool.Exec(ctx, `DELETE FROM hearing WHERE id = $1`, hearingID)
	agendaID, err := store.UpsertAgendaItem(ctx, db.UpsertAgendaItemParams{
		HearingID: hearingID, Label: "HB 9994 Test", CSIAgendaItemID: "csi-test-9994",
	})
	if err != nil {
		t.Fatalf("agenda: %v", err)
	}
	rawOrg := "Fixture Civic Coalition"
	if err := store.ReplaceTestifiersForAgenda(ctx, agendaID, []db.InsertTestifierParams{
		{AgendaItemID: agendaID, RawName: "Test Witness", RawOrganization: rawOrg, Position: "Pro", Testified: true},
	}); err != nil {
		t.Fatalf("replace testifiers: %v", err)
	}

	candidates, err := store.GenerateVendorEntityMatchCandidates(ctx, 5000)
	if err != nil {
		t.Fatalf("GenerateVendorEntityMatchCandidates: %v", err)
	}
	var found *db.VendorEntityMatchCandidate
	for i := range candidates {
		if candidates[i].SourceKind == "csi_testimony_organization" &&
			candidates[i].SourceName == rawOrg &&
			candidates[i].OrganizationID == orgID {
			found = &candidates[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected csi_testimony_organization candidate for %q linked to org %d", rawOrg, orgID)
	}
	if found.SourceTable != "testifier" {
		t.Errorf("source_table = %q, want testifier", found.SourceTable)
	}
	if found.SourceDatasetID != "csi" {
		t.Errorf("source_dataset_id = %q, want csi", found.SourceDatasetID)
	}

	// Testimony context should surface the agenda item.
	tcByID, err := store.ListEntityMatchTestimonyContext(ctx, []int64{found.ID})
	if err != nil {
		t.Fatalf("ListEntityMatchTestimonyContext: %v", err)
	}
	tc, ok := tcByID[found.ID]
	if !ok {
		t.Fatalf("no testimony context for candidate %d", found.ID)
	}
	if tc.TestifierCount < 1 {
		t.Errorf("testifier_count = %d, want >= 1", tc.TestifierCount)
	}
	if len(tc.Appearances) == 0 || tc.Appearances[0].HearingID != hearingID {
		t.Errorf("appearances did not include hearing %d: %#v", hearingID, tc.Appearances)
	}

	// Confirm decision must promote organization.match_confidence to 'confirmed'.
	if _, err := store.UpsertVendorEntityMatchDecision(ctx, db.InsertVendorEntityMatchDecisionParams{
		CandidateID:    found.ID,
		OrganizationID: orgID,
		Decision:       "confirmed",
		Confidence:     "confirmed",
		ReviewedBy:     "integration-test",
		ReviewNotes:    "csi-test confirm",
	}); err != nil {
		t.Fatalf("decision: %v", err)
	}
	if err := store.MarkOrganizationConfirmedByReview(ctx, orgID, "csi_testimony_review", "integration-test", "csi-test confirm"); err != nil {
		t.Fatalf("MarkOrganizationConfirmedByReview: %v", err)
	}
	var (
		gotConfidence string
		gotSource     *string
	)
	if err := store.Pool.QueryRow(ctx, `SELECT match_confidence::text, verification_source FROM organization WHERE id = $1`, orgID).Scan(&gotConfidence, &gotSource); err != nil {
		t.Fatalf("read promoted org: %v", err)
	}
	if gotConfidence != "confirmed" {
		t.Errorf("match_confidence = %q, want confirmed", gotConfidence)
	}
	if gotSource == nil || *gotSource != "csi_testimony_review" {
		t.Errorf("verification_source = %v, want csi_testimony_review", gotSource)
	}
}

func hasString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func TestUpsertDataWAWEBSVendor_Idempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

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
