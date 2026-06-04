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

func TestListOrganizationsHandler_JSONArray(t *testing.T) {
	store := openTestStore(t)
	r := chi.NewRouter()
	r.Get("/api/v1/organizations", listOrganizationsHandler(store))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var arr []any
	if err := json.Unmarshal(w.Body.Bytes(), &arr); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, w.Body.String())
	}
	// Empty arrays must be `[]`, not `null` — the frontend treats
	// these as arrays unconditionally.
	if w.Body.Len() == 0 || w.Body.Bytes()[0] != '[' {
		t.Errorf("body must serialize as JSON array, got %q", w.Body.String())
	}
}

// TestListOrganizationsHandler_TopicKeywordFilter verifies that the
// organization aggregator scopes to testifiers whose agenda item / bill
// matches at least one topic_keyword. With the housing fixture in place,
// "?topic_keyword=housing" must include the Fixture Housing Coalition and
// "?topic_keyword=zzz_no_match" must exclude it.
func TestListOrganizationsHandler_TopicKeywordFilter(t *testing.T) {
	store := openTestStore(t)
	r := chi.NewRouter()
	r.Get("/api/v1/organizations", listOrganizationsHandler(store))

	type item struct {
		CanonicalName  string `json:"canonical_name"`
		TestifierCount int    `json:"testifier_count"`
	}

	get := func(t *testing.T, path string) []item {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status = %d; body=%s", path, w.Code, w.Body.String())
		}
		var got []item
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("%s: unmarshal: %v; body=%s", path, err, w.Body.String())
		}
		return got
	}

	contains := func(items []item, name string) *item {
		for i := range items {
			if items[i].CanonicalName == name {
				return &items[i]
			}
		}
		return nil
	}

	matched := get(t, "/api/v1/organizations?topic_keyword=Housing")
	if contains(matched, "Fixture Housing Coalition") == nil {
		t.Fatalf("topic_keyword=Housing must include Fixture Housing Coalition; got %d entries", len(matched))
	}

	// Empty keyword (single empty string param) must behave like the
	// unfiltered call so a frontend bug never silently empties the list.
	emptyKW := get(t, "/api/v1/organizations?topic_keyword=")
	unfiltered := get(t, "/api/v1/organizations")
	if len(emptyKW) != len(unfiltered) {
		t.Errorf("topic_keyword= (empty) returned %d, unfiltered returned %d; should match", len(emptyKW), len(unfiltered))
	}

	// A keyword that matches nothing should drop orgs with zero qualifying
	// testifiers (HAVING COUNT > 0 in the SQL).
	noMatch := get(t, "/api/v1/organizations?topic_keyword=zzz_definitely_no_match_xyz")
	if contains(noMatch, "Fixture Housing Coalition") != nil {
		t.Errorf("Fixture Housing Coalition should be excluded for non-matching keyword")
	}
}

// TestGetOrganizationHandler_TranscriptQuotes verifies that the organization
// detail handler includes transcript quotes with proper structure and only
// includes reviewed speaker assignments.
func TestGetOrganizationHandler_TranscriptQuotes(t *testing.T) {
	store := openTestStore(t)
	r := chi.NewRouter()
	r.Get("/api/v1/organizations/{slug}", getOrganizationHandler(store))

	type transcriptQuote struct {
		ID             int64  `json:"id"`
		Text           string `json:"text"`
		SpeakerLabel   string `json:"speaker_label"`
		SpeakerKind    string `json:"speaker_kind"`
		ReviewStatus   string `json:"review_status"`
		StartMS        int    `json:"start_ms"`
		EndMS          int    `json:"end_ms"`
		TVWEventID     string `json:"tvw_event_id,omitempty"`
		TestifierName  string `json:"testifier_name,omitempty"`
	}

	type orgResponse struct {
		CanonicalName    string            `json:"canonical_name"`
		TranscriptQuotes []transcriptQuote `json:"transcript_quotes"`
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/fixture-housing-coalition", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var org orgResponse
	if err := json.Unmarshal(w.Body.Bytes(), &org); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, w.Body.String())
	}

	// Verify transcript_quotes field exists and is an array (not null)
	if org.TranscriptQuotes == nil {
		t.Error("transcript_quotes field must be initialized array, not null")
	}

	// If there are quotes in the test fixtures, verify their structure
	for i, quote := range org.TranscriptQuotes {
		if quote.Text == "" {
			t.Errorf("quote[%d].text must not be empty", i)
		}
		if quote.SpeakerLabel == "" {
			t.Errorf("quote[%d].speaker_label must not be empty", i)
		}
		if quote.ReviewStatus != "accepted" {
			t.Errorf("quote[%d].review_status must be 'accepted', got %q", i, quote.ReviewStatus)
		}
		if quote.SpeakerKind != "testifier" {
			t.Errorf("quote[%d].speaker_kind must be 'testifier', got %q", i, quote.SpeakerKind)
		}
		if quote.EndMS <= quote.StartMS {
			t.Errorf("quote[%d].end_ms (%d) must be > start_ms (%d)", i, quote.EndMS, quote.StartMS)
		}
	}
}
