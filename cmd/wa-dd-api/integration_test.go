//go:build integration
// +build integration

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"

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

// TestFirstPageHandler_NotFound expects the test DB to NOT contain a
// bill numbered 9999 — none of our seeded fixtures do.
func TestFirstPageHandler_NotFound(t *testing.T) {
	store := openTestStore(t)
	r := chi.NewRouter()
	r.Get("/api/v1/bills/{biennium}/{billNumber}/first-page", firstPageHandler(store))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bills/2025-26/HB9999/first-page", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
}

// TestFirstPageHandler_OK assumes HB 1501 has been ingested in the local
// dev DB (the daily-batch seed). If it hasn't, skip — a clean check
// for "is the success path wired" without forcing a fixture upload.
func TestFirstPageHandler_OK(t *testing.T) {
	store := openTestStore(t)
	r := chi.NewRouter()
	r.Get("/api/v1/bills/{biennium}/{billNumber}/first-page", firstPageHandler(store))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bills/2025-26/HB1501/first-page", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Skip("HB 1501 not ingested in the test DB; run `make daily` first")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Bill struct {
			BillID string `json:"bill_id"`
		} `json:"bill"`
		Testifiers    []any `json:"testifiers"`
		Organizations []any `json:"organizations"`
		Sources       []any `json:"sources"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Bill.BillID != "HB 1501" {
		t.Errorf("bill_id = %q, want %q", body.Bill.BillID, "HB 1501")
	}
	if body.Testifiers == nil || body.Organizations == nil || body.Sources == nil {
		t.Errorf("collection fields should serialize as [], not null: %+v", body)
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
	var bills []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &bills); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// At minimum the daily-batch seed contains HB 1501.
	found := false
	for _, b := range bills {
		if b["bill_id"] == "HB 1501" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("HB 1501 not in /api/v1/bills response (got %d bills)", len(bills))
	}
}

func TestAggregationHandlers(t *testing.T) {
	store := openTestStore(t)
	r := chi.NewRouter()
	r.Get("/api/v1/legislators", listLegislatorsHandler(store))
	r.Get("/api/v1/organizations", listOrganizationsHandler(store))
	r.Get("/api/v1/hearings", listHearingsHandler(store))
	r.Get("/api/v1/sources", listSourcesHandler(store))

	endpoints := []string{
		"/api/v1/legislators",
		"/api/v1/organizations",
		"/api/v1/sources",
	}
	for _, ep := range endpoints {
		t.Run(ep, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, ep, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("%s: status = %d, want 200; body=%s", ep, w.Code, w.Body.String())
			}
			var arr []any
			if err := json.Unmarshal(w.Body.Bytes(), &arr); err != nil {
				t.Fatalf("%s: unmarshal: %v; body=%s", ep, err, w.Body.String())
			}
			// Empty arrays must be `[]`, not `null` — the frontend treats
			// these as arrays unconditionally. Body must start with '['.
			if w.Body.Len() == 0 || w.Body.Bytes()[0] != '[' {
				t.Errorf("%s: body must serialize as JSON array, got %q", ep, w.Body.String())
			}
		})
	}

	t.Run("/api/v1/hearings", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/hearings", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Hearings []any `json:"hearings"`
			Total    int   `json:"total"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal: %v; body=%s", err, w.Body.String())
		}
		if body.Hearings == nil {
			t.Errorf("hearings field must serialize as JSON array, got %q", w.Body.String())
		}
	})
}

func TestGetHearingHandler_NotFound(t *testing.T) {
	store := openTestStore(t)
	r := chi.NewRouter()
	r.Get("/api/v1/hearings/{csiAgendaItemId}", getHearingHandler(store))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/hearings/00000", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
}

func TestGetHearingHandler_OK(t *testing.T) {
	store := openTestStore(t)
	r := chi.NewRouter()
	r.Get("/api/v1/hearings/{csiAgendaItemId}", getHearingHandler(store))

	// Demo seed: HB 1501 lives at csi_agenda_item_id=27885.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hearings/27885", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code == http.StatusNotFound {
		t.Skip("agenda item 27885 (HB 1501) not ingested in the test DB; run `make daily` first")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Bill struct {
			BillID string `json:"bill_id"`
		} `json:"bill"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Bill.BillID != "HB 1501" {
		t.Errorf("bill_id = %q, want %q", body.Bill.BillID, "HB 1501")
	}
}

func TestGetLegislatorHandler_NotFound(t *testing.T) {
	store := openTestStore(t)
	r := chi.NewRouter()
	r.Get("/api/v1/legislators/{slug}", getLegislatorHandler(store))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/legislators/no-such-name", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
}


func TestSearchTranscriptsHandler_Empty(t *testing.T) {
	store := openTestStore(t)
	r := chi.NewRouter()
	r.Get("/api/v1/search/transcripts", searchTranscriptsHandler(store))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search/transcripts?q=", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Total int   `json:"total"`
		Hits  []any `json:"hits"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Total != 0 || len(body.Hits) != 0 {
		t.Errorf("empty q should return 0 hits, got total=%d hits=%d", body.Total, len(body.Hits))
	}
}

func TestSearchTranscriptsHandler_LimitClamp(t *testing.T) {
	store := openTestStore(t)
	r := chi.NewRouter()
	r.Get("/api/v1/search/transcripts", searchTranscriptsHandler(store))

	// limit=999 should clamp to searchMaxLimit (50). Use a query that
	// matches plenty of segments; "the" has stop-word semantics in the
	// 'english' config so it returns nothing — pick a real word.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search/transcripts?q=housing&limit=999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Limit int `json:"limit"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Limit != searchMaxLimit {
		t.Errorf("limit = %d, want %d (clamped)", body.Limit, searchMaxLimit)
	}
}

func TestSearchTranscriptsHandler_BadOffset(t *testing.T) {
	store := openTestStore(t)
	r := chi.NewRouter()
	r.Get("/api/v1/search/transcripts", searchTranscriptsHandler(store))

	// Malformed offset — should fall back to 0, not 400.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search/transcripts?q=housing&offset=abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Offset int `json:"offset"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Offset != 0 {
		t.Errorf("offset = %d, want 0 (default)", body.Offset)
	}
}

func TestSearchTranscriptsHandler_OK(t *testing.T) {
	store := openTestStore(t)
	r := chi.NewRouter()
	r.Get("/api/v1/search/transcripts", searchTranscriptsHandler(store))

	// Assumes ingested transcript content — uses "housing" since the demo
	// set is housing-themed. If no segments exist (fresh DB), skip.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search/transcripts?q=housing&limit=5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Query string `json:"query"`
		Total int    `json:"total"`
		Limit int    `json:"limit"`
		Hits  []struct {
			ID      int64  `json:"id"`
			BillID  string `json:"bill_id"`
			Text    string `json:"text"`
			StartMS int    `json:"start_ms"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Query != "housing" {
		t.Errorf("query = %q, want %q", body.Query, "housing")
	}
	if body.Limit != 5 {
		t.Errorf("limit = %d, want 5", body.Limit)
	}
	if body.Total == 0 {
		t.Skip("no transcript segments matching 'housing' in test DB; skipping (run ingest-hearings first)")
	}
	if len(body.Hits) == 0 {
		t.Errorf("expected hits when total=%d, got 0", body.Total)
	}
	for _, h := range body.Hits {
		if h.Text == "" {
			t.Errorf("hit id=%d has empty text", h.ID)
		}
	}
}
