package seattle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/socrata"
)

func TestFetchOperatingBudgetWithSourceReturnsProvenance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/resource/8u2j-imqx.json" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{":id":"row1"}]`))
	}))
	defer srv.Close()

	c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}}), "")
	c.BaseURL = srv.URL
	rows, fetch, err := c.FetchOperatingBudgetWithSource(context.Background(), socrata.Query{Limit: 1})
	if err != nil || len(rows) != 1 || fetch.System != SystemName || fetch.Endpoint != "resource.8u2j-imqx" {
		t.Fatalf("rows=%#v fetch=%#v err=%v", rows, fetch, err)
	}
}

func TestNormalizeOperatingBudget(t *testing.T) {
	budget := NormalizeOperatingBudget(socrata.Row{
		":id":             "row1",
		"fiscal_year":     "2026",
		"service":         "Administration",
		"department":      "Office of the City Auditor",
		"program":         "Office of the City Auditor",
		"fund":            "00100 - General Fund",
		"fund_type":       "General Fund",
		"expense_type":    "Expenditures",
		"description":     "Labor",
		"approved_amount": "1632174",
	})
	if budget.SourceDatasetID != DatasetOperatingBudget || budget.SourceRowID != "row1" || budget.FiscalYear != 2026 {
		t.Fatalf("unexpected budget identity: %#v", budget)
	}
	if budget.Department != "Office of the City Auditor" || budget.Program != "Office of the City Auditor" || budget.ApprovedAmount != "1632174" {
		t.Fatalf("unexpected budget row: %#v", budget)
	}
}

func TestStableRowID(t *testing.T) {
	id1 := StableRowID(map[string]any{"b": "two", "a": "one"})
	id2 := StableRowID(map[string]any{"a": "one", "b": "two"})
	if id1 == "" || id1 != id2 {
		t.Fatalf("stable ids = %q %q", id1, id2)
	}
}
