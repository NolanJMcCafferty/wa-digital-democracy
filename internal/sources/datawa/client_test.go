package datawa

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/socrata"
)

func TestNewConfiguresSocrataClient(t *testing.T) {
	c := New(nil, "tok")
	if c.SystemName != SystemName || c.BaseURL != DefaultBaseURL || c.AppToken != "tok" {
		t.Fatalf("client = %#v", c.Client)
	}
}

func TestFetchAgencyContractsUsesFiscalYearDataset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/resource/6fx9-ncas.json" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{":id":"row1"}]`))
	}))
	defer srv.Close()

	c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}}), "")
	c.BaseURL = srv.URL
	rows, err := c.FetchAgencyContracts(context.Background(), 2025, socrata.Query{Limit: 1})
	if err != nil || len(rows) != 1 || rows[0][":id"] != "row1" {
		t.Fatalf("rows=%#v err=%v", rows, err)
	}
}

func TestFetchAgencyContractsRejectsUnknownYear(t *testing.T) {
	c := New(nil, "")
	if _, err := c.FetchAgencyContracts(context.Background(), 1999, socrata.Query{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestNormalizeContractAllFieldsAndDateFormats(t *testing.T) {
	c := NormalizeContract(DatasetAgencyContractsFY2025, 2025, map[string]any{
		":id":                     "row1",
		"agency_name":             "Dept",
		"agency_number":           "100",
		"contract_number":         "C123",
		"amendment_number":        "A1",
		"contractor_name":         "Vendor",
		"statewide_vendor_number": "SWV",
		"description":             "Work",
		"start_date":              "2025-01-02",
		"end_date":                "01/03/2025",
		"period_start":            "2025-01-04T00:00:00Z",
		"period_end":              "2025-01-05T00:00:00",
		"total_amount":            123.45,
		"procurement_type":        "Competitive",
		"minority_woman_owned":    "Y",
		"small_business":          "N",
		"veteran_owned":           "Y",
	})
	if c.SourceRowID != "row1" || c.AgencyNumber != "100" || c.ContractNumber != "C123" || c.TotalAmount != "123.45" {
		t.Fatalf("contract = %#v", c)
	}
	if c.StartDate == nil || c.EndDate == nil || c.PeriodStart == nil || c.PeriodEnd == nil || len(c.NormalizationWarning) != 0 {
		t.Fatalf("dates/warnings = %#v", c)
	}
}

func TestNormalizeContractDateWarnings(t *testing.T) {
	c := NormalizeContract(DatasetAgencyContractsFY2025, 2025, map[string]any{
		":id":        "row1",
		"start_date": "9999-05-21",
		"end_date":   "bogus",
	})
	if c.StartDate != nil || c.EndDate != nil || len(c.NormalizationWarning) != 2 {
		t.Fatalf("date/warnings = %v %v %#v", c.StartDate, c.EndDate, c.NormalizationWarning)
	}
}
