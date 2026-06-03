//go:build integration
// +build integration

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestListHearingsHandler(t *testing.T) {
	store := openTestStore(t)
	r := chi.NewRouter()
	r.Get("/api/v1/hearings", listHearingsHandler(store))

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
}

func TestGetHearingHandler_NotFound(t *testing.T) {
	store := openTestStore(t)
	r := chi.NewRouter()
	r.Get("/api/v1/hearings/{hearingId}", getHearingHandler(store))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/hearings/999999999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
}

func TestGetHearingHandler_OK(t *testing.T) {
	store := openTestStore(t)
	r := chi.NewRouter()
	r.Get("/api/v1/hearings/{hearingId}", getHearingHandler(store))

	var hearingID int64
	if err := store.Pool.QueryRow(context.Background(), `SELECT id FROM hearing WHERE tvw_event_id = 'fixture-tvw-event-9001'`).Scan(&hearingID); err != nil {
		t.Fatalf("lookup fixture hearing: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hearings/"+strconv.FormatInt(hearingID, 10), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var body struct {
		HearingID   int64 `json:"hearing_id"`
		AgendaItems []struct {
			BillID string `json:"bill_id"`
		} `json:"agenda_items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.HearingID != hearingID {
		t.Errorf("hearing_id = %d, want %d", body.HearingID, hearingID)
	}
	found := false
	for _, item := range body.AgendaItems {
		if item.BillID == "HB 9001" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("agenda_items did not include HB 9001: %+v", body.AgendaItems)
	}
}
