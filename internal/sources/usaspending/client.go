// Package usaspending implements selected USAspending API v2 clients.
package usaspending

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

const (
	SystemName     = "usaspending"
	DefaultBaseURL = "https://api.usaspending.gov/api/v2"
)

type Client struct {
	HTTP    *httpx.Client
	BaseURL string
}

func New(h *httpx.Client) *Client { return &Client{HTTP: h, BaseURL: DefaultBaseURL} }

type AwardSearchRequest struct {
	Filters map[string]any `json:"filters"`
	Fields  []string       `json:"fields"`
	Page    int            `json:"page"`
	Limit   int            `json:"limit"`
	Sort    string         `json:"sort,omitempty"`
	Order   string         `json:"order,omitempty"`
}

type AwardSearchResponse struct {
	Limit        int              `json:"limit"`
	Page         int              `json:"page"`
	Results      []map[string]any `json:"results"`
	PageMetadata struct {
		Page        int  `json:"page"`
		HasNextPage bool `json:"hasNextPage"`
	} `json:"page_metadata"`
}

var DefaultAwardFields = []string{"Award ID", "Recipient Name", "Start Date", "End Date", "Award Amount", "Awarding Agency", "Funding Agency", "Award Type", "Place of Performance State Code", "Place of Performance County", "Recipient UEI"}

func WashingtonAwardSearchRequest(startDate, endDate string) AwardSearchRequest {
	return AwardSearchRequest{
		Filters: map[string]any{
			"time_period":                    []map[string]string{{"start_date": startDate, "end_date": endDate}},
			"place_of_performance_locations": []map[string]string{{"country": "USA", "state": "WA"}},
		},
		Fields: DefaultAwardFields,
		Page:   1,
		Limit:  100,
		Sort:   "Award Amount",
		Order:  "desc",
	}
}

func (c *Client) SearchAwards(ctx context.Context, req AwardSearchRequest) (*AwardSearchResponse, error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Limit <= 0 {
		req.Limit = 100
	}
	if len(req.Fields) == 0 {
		req.Fields = DefaultAwardFields
	}
	body, _ := json.Marshal(req)
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/json")
	fetch, err := c.HTTP.Do(ctx, httpx.Request{System: SystemName, Endpoint: "search.spending_by_award", Method: http.MethodPost, URL: c.BaseURL + "/search/spending_by_award/", Headers: headers, Body: body})
	if err != nil {
		return nil, err
	}
	return ParseAwardSearch(fetch.Body)
}

func (c *Client) TopTierAgencies(ctx context.Context) ([]map[string]any, error) {
	fetch, err := c.HTTP.Do(ctx, httpx.Request{System: SystemName, Endpoint: "references.toptier_agencies", URL: c.BaseURL + "/references/toptier_agencies/", Headers: jsonAccept()})
	if err != nil {
		return nil, err
	}
	var raw struct {
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal(fetch.Body, &raw); err != nil {
		return nil, fmt.Errorf("usaspending agencies: %w", err)
	}
	return raw.Results, nil
}

func ParseAwardSearch(body []byte) (*AwardSearchResponse, error) {
	var out AwardSearchResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("usaspending award search: %w", err)
	}
	return &out, nil
}

func jsonAccept() http.Header { h := http.Header{}; h.Set("Accept", "application/json"); return h }
