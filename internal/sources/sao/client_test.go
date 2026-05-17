package sao

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

func TestParseDotNetDate(t *testing.T) {
	got := parseDotNetDate("/Date(1758783600000)/")
	if got == nil || got.UTC().Format("2006-01-02") != "2025-09-25" {
		t.Fatalf("date = %v", got)
	}
	if parseDotNetDate("bad") != nil || parseDotNetDate("/Date(nope)/") != nil {
		t.Fatal("bad dates should parse nil")
	}
}

func TestReportPDFURL(t *testing.T) {
	c := New(nil)
	got := c.ReportPDFURL("1039000")
	want := "https://portal.sao.wa.gov/ReportSearch/Home/ViewReportFile?arn=1039000&isFinding=false&sp=false"
	if got != want {
		t.Fatalf("url = %s, want %s", got, want)
	}
}

func TestNormalizeReport(t *testing.T) {
	r := normalizeReport(map[string]any{
		"AuditNumber":       "A1",
		"AuditReportNumber": "1039000",
		"ReportTitle":       "City of Seattle Accountability Audit",
		"AuditTypeName":     "Accountability",
		"GovTypeDesc":       "City/Town",
		"DateReleased":      "/Date(1758783600000)/",
		"BeginAuditPeriod":  "/Date(1704067200000)/",
		"EndAuditPeriod":    "/Date(1735689600000)/",
		"Findings":          "true",
		"AuditReportLink":   "https://example/report.pdf",
		"FindingsLink":      "https://example/findings.pdf",
	})
	if r.AuditReportNumber != "1039000" || !r.Findings || r.ReportTitle == "" || r.GovTypeDesc != "City/Town" || r.AuditReportLink == "" {
		t.Fatalf("report = %#v", r)
	}
	if r.DateReleased == nil || !r.DateReleased.Equal(time.UnixMilli(1758783600000).UTC()) {
		t.Fatalf("DateReleased = %v", r.DateReleased)
	}
}

func TestGovTypesAuditTypesEntitiesAndSearch(t *testing.T) {
	var sawSearch bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/json" {
			t.Fatalf("Accept=%q", r.Header.Get("Accept"))
		}
		switch r.URL.Path {
		case "/api/Reports/GovTypes":
			_, _ = w.Write([]byte(`[{"GovTypeCode":"C","GovTypeDesc":"City/Town"}]`))
		case "/api/Reports/AuditTypes":
			_, _ = w.Write([]byte(`[{"AuditTypeID":"1","AuditTypeName":"Accountability"}]`))
		case "/Home/GetEntities":
			if r.URL.Query().Get("NameStartsWith") != "Seattle" {
				t.Fatalf("query=%s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`[{"MCAG":"0433","Name":"City of Seattle","GovTypeDesc":"City/Town"}]`))
		case "/Home/SearchReports":
			sawSearch = true
			q := r.URL.Query()
			if q.Get("pageSize") != "50" || q.Get("pageNumber") != "2" || q.Get("StartDate") != "1/1/2023" || q.Get("EndDate") != "12/31/2026" || q.Get("HasFindings") != "true" || !strings.Contains(q.Get("Keyword"), "Seattle") {
				t.Fatalf("query=%s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"total":1,"data":[{"AuditReportNumber":"1039000","ReportTitle":"Report","Findings":true}]}`))
		default:
			t.Fatalf("path=%s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}}))
	c.BaseURL = srv.URL
	govs, err := c.GovTypes(context.Background())
	if err != nil || len(govs) != 1 || govs[0].Code != "C" || govs[0].Name != "City/Town" {
		t.Fatalf("govs=%#v err=%v", govs, err)
	}
	audits, err := c.AuditTypes(context.Background())
	if err != nil || len(audits) != 1 || audits[0].ID != "1" {
		t.Fatalf("audits=%#v err=%v", audits, err)
	}
	ents, err := c.GetEntities(context.Background(), "Seattle")
	if err != nil || len(ents) != 1 || ents[0].MCAG != "0433" {
		t.Fatalf("ents=%#v err=%v", ents, err)
	}
	hasFindings := true
	res, err := c.SearchReports(context.Background(), SearchParams{PageSize: 50, PageNumber: 2, StartDate: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC), Keyword: "Seattle", HasFindings: &hasFindings, SortField: "DateReleased", SortDir: "desc"})
	if err != nil || res.Total != 1 || len(res.Reports) != 1 || !sawSearch {
		t.Fatalf("res=%#v err=%v", res, err)
	}
}

func TestParseErrors(t *testing.T) {
	for _, fn := range []func(*Client) error{
		func(c *Client) error { _, err := c.GovTypes(context.Background()); return err },
		func(c *Client) error { _, err := c.AuditTypes(context.Background()); return err },
		func(c *Client) error { _, err := c.GetEntities(context.Background(), "x"); return err },
		func(c *Client) error { _, err := c.SearchReports(context.Background(), SearchParams{}); return err },
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`not-json`)) }))
		c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}}))
		c.BaseURL = srv.URL
		if err := fn(c); err == nil {
			t.Fatal("expected error")
		}
		srv.Close()
	}
}
