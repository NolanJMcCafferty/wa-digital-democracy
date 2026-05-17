package fema

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

func TestURL(t *testing.T) {
	c := New(nil)
	u, err := c.URL(Query{Entity: DatasetDisasterDeclarations, Filter: "state eq 'WA'", Select: []string{"disasterNumber", "state"}, OrderBy: "declarationDate desc", Top: 10, Skip: 20, Count: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"DisasterDeclarationsSummaries", "%24filter=state+eq+%27WA%27", "%24select=disasterNumber%2Cstate", "%24orderby=declarationDate+desc", "%24top=10", "%24skip=20", "%24count=true", "%24format=json"} {
		if !strings.Contains(u, want) {
			t.Fatalf("url=%s missing %s", u, want)
		}
	}
}

func TestURLError(t *testing.T) {
	if _, err := New(nil).URL(Query{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseEntity(t *testing.T) {
	rows, err := ParseEntity([]byte(`{"DisasterDeclarationsSummaries":[{"state":"WA"}]}`), DatasetDisasterDeclarations)
	if err != nil || len(rows) != 1 || rows[0]["state"] != "WA" {
		t.Fatalf("rows=%#v err=%v", rows, err)
	}
	if _, err := ParseEntity([]byte(`not-json`), DatasetDisasterDeclarations); err == nil {
		t.Fatal("expected error")
	}
}

func TestQueryAndWashingtonDisasters(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/DisasterDeclarationsSummaries" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if r.URL.Query().Get("$filter") != "state eq 'WA'" || r.Header.Get("Accept") != "application/json" {
			t.Fatalf("query/header=%s %q", r.URL.RawQuery, r.Header.Get("Accept"))
		}
		_, _ = w.Write([]byte(`{"DisasterDeclarationsSummaries":[{"state":"WA"}]}`))
	}))
	defer srv.Close()
	c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}}))
	c.BaseURL = srv.URL
	rows, err := c.WashingtonDisasters(context.Background())
	if err != nil || len(rows) != 1 || calls != 1 {
		t.Fatalf("rows=%#v calls=%d err=%v", rows, calls, err)
	}
}
