package seattleauditor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

const dashboardFixture = `{
  "dashboardTitle":"Seattle City Auditor Recommendations",
  "publishedAt":1766439336269,
  "open":{"overallStats":{"count":1},"recommendationsTable":[{"recommendationId":"r1","recommendationNumber":"1","recommendationText":"Do the thing","category":"Ops","auditName":"Audit","auditType":"Performance","department":"Police","status":"Pending","update":"Soon","timestamp":"ts","auditURL":"https://example/audit","issueDate":"2025-01-01","findingText":"Finding","findingNumber":"F1","year":"2025"}]},
  "all":{"recommendationsTable":[]},
  "closed":{"recommendationsTable":[{"recommendationId":"r2","recommendationText":"Done","status":"Closed"}]}
}`

func TestParseDashboard(t *testing.T) {
	d, err := ParseDashboard([]byte(dashboardFixture))
	if err != nil {
		t.Fatal(err)
	}
	if d.Title == "" || d.PublishedAt == nil || len(d.Open.RecommendationsTable) != 1 || len(d.Closed.RecommendationsTable) != 1 {
		t.Fatalf("dashboard = %#v", d)
	}
	r := d.Open.RecommendationsTable[0]
	if r.RecommendationID != "r1" || r.Department != "Police" || r.Status != "Pending" || r.FindingText != "Finding" || r.Raw["recommendationId"] != "r1" {
		t.Fatalf("recommendation = %#v", r)
	}
}

func TestParseDashboardErrorAndMissingGroups(t *testing.T) {
	if _, err := ParseDashboard([]byte(`not-json`)); err == nil {
		t.Fatal("expected error")
	}
	d, err := ParseDashboard([]byte(`{"dashboardTitle":"x"}`))
	if err != nil || d.Open.RecommendationsTable != nil || d.PublishedAt != nil {
		t.Fatalf("dashboard=%#v err=%v", d, err)
	}
}

func TestFetchDashboardAndReportsPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/dash/current.json":
			if r.Header.Get("Accept") != "application/json" {
				t.Fatalf("Accept=%q", r.Header.Get("Accept"))
			}
			_, _ = w.Write([]byte(dashboardFixture))
		case "/reports":
			if r.Header.Get("Accept") != "text/html,application/xhtml+xml" {
				t.Fatalf("Accept=%q", r.Header.Get("Accept"))
			}
			_, _ = w.Write([]byte("<html>reports</html>"))
		default:
			t.Fatalf("path=%s", r.URL.Path)
		}
	}))
	defer srv.Close()
	c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}}))
	c.S3BaseURL = srv.URL
	c.DashboardID = "dash"
	c.ReportsURL = srv.URL + "/reports"
	d, err := c.FetchDashboard(context.Background())
	if err != nil || d.Title == "" {
		t.Fatalf("dashboard=%#v err=%v", d, err)
	}
	body, err := c.FetchReportsPage(context.Background())
	if err != nil || string(body) != "<html>reports</html>" {
		t.Fatalf("body=%q err=%v", body, err)
	}
}
