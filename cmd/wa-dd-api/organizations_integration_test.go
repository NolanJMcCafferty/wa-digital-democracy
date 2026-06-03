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
