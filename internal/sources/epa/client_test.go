package epa

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

func TestFacilitySearchDefaultsWashingtonAndJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/eff_rest_services.get_facilities" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("p_st") != "WA" || r.URL.Query().Get("output") != "JSON" {
			t.Fatalf("query = %s", r.URL.RawQuery)
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Fatalf("Accept = %q", r.Header.Get("Accept"))
		}
		_, _ = w.Write([]byte(`{"Results":{"Facilities":[]}}`))
	}))
	defer srv.Close()

	c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}}))
	c.ECHOBaseURL = srv.URL
	body, err := c.FacilitySearch(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) == 0 {
		t.Fatal("empty body")
	}
}

func TestFacilitySearchPreservesExplicitState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("p_st") != "OR" {
			t.Fatalf("query = %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}}))
	c.ECHOBaseURL = srv.URL
	params := url.Values{"p_st": {"OR"}}
	if _, err := c.FacilitySearch(context.Background(), params); err != nil {
		t.Fatal(err)
	}
}
