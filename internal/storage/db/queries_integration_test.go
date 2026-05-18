//go:build integration
// +build integration

package db_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func openTestStore(t *testing.T) *db.Store {
	t.Helper()
	dsn := os.Getenv("WADD_TEST_DSN")
	if dsn == "" {
		dsn = "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"
	}
	store, err := db.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(store.Close)
	return store
}

type dbCleanup struct {
	t     *testing.T
	store *db.Store
	srIDs []int64
}

func newDBCleanup(t *testing.T, store *db.Store) *dbCleanup {
	t.Helper()
	c := &dbCleanup{t: t, store: store}
	t.Cleanup(c.run)
	return c
}

func (c *dbCleanup) addSourceRecord(id int64) {
	c.srIDs = append(c.srIDs, id)
}

func (c *dbCleanup) run() {
	ctx := context.Background()
	for _, q := range []string{
		`DELETE FROM datawa_master_contract_sale WHERE source_record_id = ANY($1)`,
		`DELETE FROM datawa_contract WHERE source_record_id = ANY($1)`,
		`DELETE FROM bill_status_change WHERE source_record_id = ANY($1)`,
		`DELETE FROM transcript_segment WHERE source_record_id = ANY($1)`,
		`DELETE FROM org_context_record WHERE source_record_id = ANY($1)`,
		`DELETE FROM testifier WHERE source_record_id = ANY($1)`,
		`DELETE FROM agenda_item WHERE source_record_id = ANY($1)`,
		`DELETE FROM hearing WHERE source_record_id = ANY($1)`,
		`DELETE FROM bill WHERE source_record_id = ANY($1)`,
		`DELETE FROM tvw_event WHERE source_record_id = ANY($1)`,
		`DELETE FROM source_record WHERE id = ANY($1)`,
	} {
		if _, err := c.store.Pool.Exec(ctx, q, c.srIDs); err != nil {
			c.t.Errorf("cleanup %q: %v", q, err)
		}
	}
}

// insertProvenance creates a fake source_record we can FK against.
func insertProvenance(t *testing.T, store *db.Store, cleanup *dbCleanup, system, hash string) int64 {
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
	cleanup.addSourceRecord(id)
	return id
}

func TestUpsertBill_Idempotent(t *testing.T) {
	store := openTestStore(t)
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
	store := openTestStore(t)
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

func TestFindBillNumberMentions(t *testing.T) {
	store := openTestStore(t)
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
	store := openTestStore(t)
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
	store := openTestStore(t)
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
