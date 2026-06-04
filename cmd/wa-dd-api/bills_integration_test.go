//go:build integration
// +build integration

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

// TestBillPageHandler_NotFound expects the test DB to NOT contain a
// bill numbered 9999 — none of our seeded fixtures do.
func TestBillPageHandler_NotFound(t *testing.T) {
	store := openTestStore(t)
	r := chi.NewRouter()
	r.Get("/api/v1/bills/{biennium}/{billNumber}/page", billPageHandler(store))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bills/2025-26/HB9999/page", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
}

func TestBillPageHandler_OK(t *testing.T) {
	store := openTestStore(t)
	r := chi.NewRouter()
	r.Get("/api/v1/bills/{biennium}/{billNumber}/page", billPageHandler(store))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bills/2099-00/HB9001/page", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Bill struct {
			BillID string `json:"bill_id"`
			Title  string `json:"title"`
		} `json:"bill"`
		Hearings []struct {
			Hearing struct {
				CSIAgendaItemID string `json:"csi_agenda_item_id"`
			} `json:"hearing"`
			Transcript *struct {
				Segments []struct {
					Text string `json:"text"`
				} `json:"segments"`
			} `json:"transcript"`
		} `json:"hearings"`
		Sources []any `json:"sources"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Bill.BillID != "HB 9001" {
		t.Errorf("bill_id = %q, want %q", body.Bill.BillID, "HB 9001")
	}
	if body.Bill.Title != "Fixture Housing Stability Act" {
		t.Errorf("title = %q, want fixture title", body.Bill.Title)
	}
	if len(body.Hearings) != 1 {
		t.Fatalf("hearings len = %d, want 1", len(body.Hearings))
	}
	if body.Hearings[0].Hearing.CSIAgendaItemID != "fixture-agenda-item-9001" {
		t.Errorf("csi_agenda_item_id = %q", body.Hearings[0].Hearing.CSIAgendaItemID)
	}
	if body.Hearings[0].Transcript == nil || len(body.Hearings[0].Transcript.Segments) == 0 {
		t.Fatalf("fixture bill should include transcript segments")
	}
	if body.Sources == nil {
		t.Errorf("sources should serialize as [], not null")
	}
}

func TestListBillsHandler(t *testing.T) {
	store := openTestStore(t)
	r := chi.NewRouter()
	r.Get("/api/v1/bills", listBillsHandler(store))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bills", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Bills []map[string]any `json:"bills"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Bills == nil {
		t.Fatalf("bills field must serialize as JSON array, got %q", w.Body.String())
	}
	// The integration DB may be freshly migrated and unseeded. If bills exist,
	// total should be internally consistent with the returned page.
	if body.Total < len(body.Bills) {
		t.Errorf("total = %d, returned bills = %d", body.Total, len(body.Bills))
	}
}

func TestListBillsHandler_SponsorFilterOrdersPrimaryMatchesFirst(t *testing.T) {
	store := openTestStore(t)
	seedSponsorOrderingFixture(t, store)

	r := chi.NewRouter()
	r.Get("/api/v1/bills", listBillsHandler(store))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bills?sponsor=representative-fixture-sponsor&q=Sponsor%20Ordering&limit=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Bills []struct {
			BillID   string `json:"bill_id"`
			LeadSlug string `json:"lead_slug"`
		} `json:"bills"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Bills) != 2 {
		t.Fatalf("returned bills = %d, want 2; body=%s", len(body.Bills), w.Body.String())
	}
	if body.Bills[0].BillID != "HB 9100" || body.Bills[0].LeadSlug != "representative-fixture-sponsor" {
		t.Fatalf("first bill = %#v, want primary-sponsored HB 9100 first", body.Bills[0])
	}
	if body.Bills[1].BillID != "HB 9000" {
		t.Fatalf("second bill = %#v, want secondary-sponsored HB 9000", body.Bills[1])
	}
}

func seedSponsorOrderingFixture(t *testing.T, store *db.Store) {
	t.Helper()
	ctx := context.Background()
	cleanup := func() {
		_, _ = store.Pool.Exec(ctx, `DELETE FROM bill WHERE biennium = '2098-00' AND prefix = 'HB' AND number IN (9000, 9100)`)
		_, _ = store.Pool.Exec(ctx, `DELETE FROM legislator_roster_membership WHERE biennium = '2025-26' AND lws_sponsor_id = '990002'`)
		_, _ = store.Pool.Exec(ctx, `DELETE FROM legislator WHERE lws_sponsor_id = '990002'`)
	}
	cleanup()
	t.Cleanup(cleanup)

	_, err := store.UpsertLegislatorRosterMembership(ctx, db.UpsertLegislatorParams{
		Biennium:     "2025-26",
		LWSSponsorID: "990002",
		Name:         "Representative Lead Fixture",
		Chamber:      "House",
		District:     "98",
		Party:        "D",
		OfficialURL:  "https://example.test/legislators/lead-fixture",
		FirstName:    "Lead",
		LastName:     "Fixture",
		Email:        "lead.fixture@example.test",
		Phone:        "360-555-0101",
		Acronym:      "LFX",
	})
	if err != nil {
		t.Fatalf("upsert lead legislator membership: %v", err)
	}

	statusDate := time.Date(2098, 1, 10, 0, 0, 0, 0, time.UTC)
	secondaryBillID, err := store.UpsertBill(ctx, db.UpsertBillParams{
		Biennium:      "2098-00",
		Prefix:        "HB",
		Number:        9000,
		Title:         "Sponsor Ordering Secondary Bill",
		ChamberOrigin: "House",
		CurrentStatus: "Public hearing scheduled",
		StatusDate:    statusDate,
	})
	if err != nil {
		t.Fatalf("upsert secondary bill: %v", err)
	}
	primaryBillID, err := store.UpsertBill(ctx, db.UpsertBillParams{
		Biennium:      "2098-00",
		Prefix:        "HB",
		Number:        9100,
		Title:         "Sponsor Ordering Primary Bill",
		ChamberOrigin: "House",
		CurrentStatus: "Public hearing scheduled",
		StatusDate:    statusDate,
	})
	if err != nil {
		t.Fatalf("upsert primary bill: %v", err)
	}
	leadMembershipID, leadPersonID, ok, err := store.FindLegislatorRosterMembershipByLWSSponsorID(ctx, "2025-26", "990002")
	if err != nil || !ok {
		t.Fatalf("find lead membership: ok=%v err=%v", ok, err)
	}
	fixtureMembershipID, fixturePersonID, ok, err := store.FindLegislatorRosterMembershipByLWSSponsorID(ctx, "2025-26", "990001")
	if err != nil || !ok {
		t.Fatalf("find fixture membership: ok=%v err=%v", ok, err)
	}
	if err := store.UpsertBillSponsorMembership(ctx, secondaryBillID, leadMembershipID, leadPersonID, "Primary"); err != nil {
		t.Fatalf("upsert secondary bill primary sponsor: %v", err)
	}
	if err := store.UpsertBillSponsorMembership(ctx, secondaryBillID, fixtureMembershipID, fixturePersonID, "Secondary"); err != nil {
		t.Fatalf("upsert secondary bill secondary sponsor: %v", err)
	}
	if err := store.UpsertBillSponsorMembership(ctx, primaryBillID, fixtureMembershipID, fixturePersonID, "Primary"); err != nil {
		t.Fatalf("upsert primary bill primary sponsor: %v", err)
	}
}
