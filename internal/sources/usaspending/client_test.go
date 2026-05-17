package usaspending

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

func TestWashingtonAwardSearchRequest(t *testing.T) {
	r := WashingtonAwardSearchRequest("2025-10-01", "2026-09-30")
	if r.Page != 1 || r.Limit != 100 || r.Sort != "Award Amount" || r.Order != "desc" || len(r.Fields) == 0 || r.Filters["time_period"] == nil || r.Filters["place_of_performance_locations"] == nil {
		t.Fatalf("request=%#v", r)
	}
}

func TestParseAwardSearch(t *testing.T) {
	got, err := ParseAwardSearch([]byte(`{"limit":1,"page":1,"results":[{"Award ID":"A"}],"page_metadata":{"page":1,"hasNextPage":false}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 1 || got.Results[0]["Award ID"] != "A" || got.PageMetadata.Page != 1 {
		t.Fatalf("got=%#v", got)
	}
	if _, err := ParseAwardSearch([]byte(`not-json`)); err == nil {
		t.Fatal("expected error")
	}
}

func TestSearchAwardsPostsDefaults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/search/spending_by_award/" {
			t.Fatalf("method/path=%s %s", r.Method, r.URL.Path)
		}
		var req AwardSearchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Page != 1 || req.Limit != 100 || len(req.Fields) == 0 {
			t.Fatalf("request=%#v", req)
		}
		_, _ = w.Write([]byte(`{"limit":100,"page":1,"results":[]}`))
	}))
	defer srv.Close()
	c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}}))
	c.BaseURL = srv.URL
	got, err := c.SearchAwards(context.Background(), AwardSearchRequest{Filters: map[string]any{}})
	if err != nil || got.Page != 1 {
		t.Fatalf("got=%#v err=%v", got, err)
	}
}

func TestTopTierAgencies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/references/toptier_agencies/" || r.Header.Get("Accept") != "application/json" {
			t.Fatalf("path/header=%s %q", r.URL.Path, r.Header.Get("Accept"))
		}
		_, _ = w.Write([]byte(`{"results":[{"name":"Agency"}]}`))
	}))
	defer srv.Close()
	c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}}))
	c.BaseURL = srv.URL
	got, err := c.TopTierAgencies(context.Background())
	if err != nil || len(got) != 1 || got[0]["name"] != "Agency" {
		t.Fatalf("got=%#v err=%v", got, err)
	}
}
