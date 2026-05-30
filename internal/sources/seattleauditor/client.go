// Package seattleauditor implements clients for Seattle City Auditor reports
// and the public Missionmark recommendation dashboard JSON.
package seattleauditor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/connector"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

const (
	SystemName         = "seattle_auditor"
	DefaultDashboardID = "fb051ed2-f68f-4efb-9285-5339de0d7884"
	DefaultS3BaseURL   = "https://s3-recommendation-dashboard-production.s3-us-west-2.amazonaws.com"
	DefaultReportsURL  = "https://www.seattle.gov/cityauditor/reports"
)

var descriptor = connector.Descriptor{
	System:      SystemName,
	BaseURL:     DefaultReportsURL,
	Description: "Seattle City Auditor reports + Missionmark dashboard",
}

func init() { connector.Register(descriptor) }

type Client struct {
	HTTP        *httpx.Client
	DashboardID string
	S3BaseURL   string
	ReportsURL  string
}

func New(h *httpx.Client) *Client {
	return &Client{HTTP: h, DashboardID: DefaultDashboardID, S3BaseURL: DefaultS3BaseURL, ReportsURL: DefaultReportsURL}
}

// Descriptor implements connector.Source.
func (c *Client) Descriptor() connector.Descriptor { return descriptor }

type Dashboard struct {
	Title       string
	PublishedAt *time.Time
	All         RecommendationGroup
	Open        RecommendationGroup
	Closed      RecommendationGroup
	Raw         map[string]any
}

type RecommendationGroup struct {
	OverallStats         map[string]any
	RecommendationsTable []Recommendation
}

type Recommendation struct {
	RecommendationID     string
	RecommendationNumber string
	RecommendationText   string
	Category             string
	AuditName            string
	AuditType            string
	Department           string
	Status               string
	Update               string
	Timestamp            string
	AuditURL             string
	IssueDate            string
	FindingText          string
	FindingNumber        string
	Year                 string
	Raw                  map[string]any
}

func (c *Client) FetchDashboard(ctx context.Context) (*Dashboard, error) {
	u := c.S3BaseURL + "/" + c.DashboardID + "/current.json"
	fetch, err := c.HTTP.Do(ctx, httpx.Request{System: SystemName, Endpoint: "missionmark.current", URL: u, Headers: jsonAccept()})
	if err != nil {
		return nil, err
	}
	return ParseDashboard(fetch.Body)
}

func (c *Client) FetchReportsPage(ctx context.Context) ([]byte, error) {
	fetch, err := c.HTTP.Do(ctx, httpx.Request{System: SystemName, Endpoint: "cityauditor.reports", URL: c.ReportsURL, Headers: htmlAccept()})
	if err != nil {
		return nil, err
	}
	return fetch.Body, nil
}

func ParseDashboard(body []byte) (*Dashboard, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("seattle auditor dashboard: %w", err)
	}
	return &Dashboard{
		Title:       str(raw["dashboardTitle"]),
		PublishedAt: millisTime(raw["publishedAt"]),
		All:         parseGroup(raw["all"]),
		Open:        parseGroup(raw["open"]),
		Closed:      parseGroup(raw["closed"]),
		Raw:         raw,
	}, nil
}

func parseGroup(v any) RecommendationGroup {
	m, _ := v.(map[string]any)
	if m == nil {
		return RecommendationGroup{}
	}
	var recs []Recommendation
	if xs, ok := m["recommendationsTable"].([]any); ok {
		for _, x := range xs {
			if row, ok := x.(map[string]any); ok {
				recs = append(recs, Recommendation{
					RecommendationID:     str(row["recommendationId"]),
					RecommendationNumber: str(row["recommendationNumber"]),
					RecommendationText:   str(row["recommendationText"]),
					Category:             str(row["category"]),
					AuditName:            str(row["auditName"]),
					AuditType:            str(row["auditType"]),
					Department:           str(row["department"]),
					Status:               str(row["status"]),
					Update:               str(row["update"]),
					Timestamp:            str(row["timestamp"]),
					AuditURL:             str(row["auditURL"]),
					IssueDate:            str(row["issueDate"]),
					FindingText:          str(row["findingText"]),
					FindingNumber:        str(row["findingNumber"]),
					Year:                 str(row["year"]),
					Raw:                  row,
				})
			}
		}
	}
	stats, _ := m["overallStats"].(map[string]any)
	return RecommendationGroup{OverallStats: stats, RecommendationsTable: recs}
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func millisTime(v any) *time.Time {
	var ms int64
	switch x := v.(type) {
	case float64:
		ms = int64(x)
	case int64:
		ms = x
	default:
		return nil
	}
	t := time.UnixMilli(ms).UTC()
	return &t
}

func jsonAccept() http.Header {
	h := http.Header{}
	h.Set("Accept", "application/json")
	return h
}

func htmlAccept() http.Header {
	h := http.Header{}
	h.Set("Accept", "text/html,application/xhtml+xml")
	return h
}
