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

func TestFetchMasterContractSalesWithSourceReturnsProvenance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/resource/n8q6-4twj.json" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{":id":"row1"}]`))
	}))
	defer srv.Close()

	c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}}), "")
	c.BaseURL = srv.URL
	rows, fetch, err := c.FetchMasterContractSalesWithSource(context.Background(), socrata.Query{Limit: 1})
	if err != nil || len(rows) != 1 || fetch.System != SystemName || fetch.Endpoint != "resource.n8q6-4twj" {
		t.Fatalf("rows=%#v fetch=%#v err=%v", rows, fetch, err)
	}
}

func TestFetchAgencyContractsWithSourceReturnsProvenance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{":id":"row1"}]`))
	}))
	defer srv.Close()

	c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}}), "")
	c.BaseURL = srv.URL
	rows, fetch, err := c.FetchAgencyContractsWithSource(context.Background(), 2025, socrata.Query{Limit: 1})
	if err != nil || len(rows) != 1 || fetch.System != SystemName || fetch.Endpoint != "resource.6fx9-ncas" {
		t.Fatalf("rows=%#v fetch=%#v err=%v", rows, fetch, err)
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

func TestNormalizeMasterContractSale(t *testing.T) {
	sale := NormalizeMasterContractSale(socrata.Row{
		":id":               "row1",
		"customer_type":     "State Agency",
		"customer_name":     "TRANSPORTATION DEPT OF",
		"contract_number":   "00111",
		"contract_title":    "Fertilizers",
		"vendor_name":       "WILBUR-ELLIS COMPANY LLC",
		"year":              "2015",
		"q1_sales_reported": "1.25",
		"q2_sales_reported": "2.75",
		"q3_sales_reported": "0",
		"q4_sales_reported": "1239",
		"omwbe":             "N",
		"vet_owned":         "N",
		"small_business":    "N",
		"diverse_options":   "N",
	})
	if sale.SourceDatasetID != DatasetMasterContractSales || sale.SourceRowID != "row1" || sale.ReportYear != 2015 {
		t.Fatalf("unexpected normalized sale: %#v", sale)
	}
	if sale.TotalSalesReported != "1243.00" {
		t.Fatalf("total sales = %q", sale.TotalSalesReported)
	}
}

func TestNormalizeMasterContractSaleInvalidYear(t *testing.T) {
	sale := NormalizeMasterContractSale(socrata.Row{"year": "FY15"})
	if len(sale.NormalizationWarning) != 1 || sale.NormalizationWarning[0] != "invalid_year:FY15" {
		t.Fatalf("warnings = %#v", sale.NormalizationWarning)
	}
}

func TestStableRowID(t *testing.T) {
	id1 := StableRowID(map[string]any{"b": "two", "a": "one"})
	id2 := StableRowID(map[string]any{"a": "one", "b": "two"})
	if id1 == "" || id1 != id2 {
		t.Fatalf("stable ids = %q %q", id1, id2)
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
