//go:build integration
// +build integration

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

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
		t.Fatalf("fixture transcript search returned no hits")
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
