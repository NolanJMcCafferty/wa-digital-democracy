// Package pdc is the Washington Public Disclosure Commission connector,
// reading via the data.wa.gov Socrata API.
//
// Per data-sources/05:
//   - Endpoint pattern: GET https://data.wa.gov/resource/<dataset_id>.json
//   - Optional X-App-Token / $$app_token query param.
//   - Pagination: $limit + $offset; iterate until len(page) < $limit.
//
// First-page MVP cares about three datasets (Blueprint lines 334–347):
//   - xhn7-64im  Lobbyist Employment Registrations
//   - 9nnw-c693  Lobbyist Compensation and Expenses by Source
//   - 2jwd-akfb  Contributions to Candidates
//
// Schema-drift caveat (spec 05 line 165): PDC lobbying categories changed
// around 2026-05-15-relative-to-the-doc's "Oct 30, 2024" — preserve raw
// fields verbatim; never collapse `use_categories` or undocumented columns.
// We therefore deserialize each row into map[string]any and pull the few
// fields the page needs by name.
package pdc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

const (
	SystemName     = "pdc_socrata"
	DefaultBaseURL = "https://data.wa.gov"
	defaultPage    = 50000
)

// Dataset IDs for the first-page pipeline.
const (
	DatasetLobbyistEmployment   = "xhn7-64im"
	DatasetLobbyistCompensation = "9nnw-c693"
	DatasetContributions        = "2jwd-akfb"
)

// Row is a single Socrata row as returned by the JSON endpoint. We use a
// generic map so dataset additions don't break the connector.
type Row map[string]any

// Client wraps an httpx.Client with Socrata configuration.
type Client struct {
	HTTP     *httpx.Client
	BaseURL  string
	AppToken string // optional; sent as $$app_token query param
}

// New returns a Client.
func New(h *httpx.Client, appToken string) *Client {
	return &Client{HTTP: h, BaseURL: DefaultBaseURL, AppToken: appToken}
}

// Query is a single SoQL query against a dataset.
type Query struct {
	Select string // $select
	Where  string // $where
	Order  string // $order; defaults to ":id" for stable iteration
	Limit  int    // $limit; defaults to 50000
	Offset int    // $offset
	Extra  url.Values
}

// FetchPage fetches one page of rows.
func (c *Client) FetchPage(ctx context.Context, datasetID string, q Query) ([]Row, error) {
	u, err := c.url(datasetID, q)
	if err != nil {
		return nil, err
	}
	headers := http.Header{}
	headers.Set("Accept", "application/json")

	fetch, err := c.HTTP.Do(ctx, httpx.Request{
		System:   SystemName,
		Endpoint: "resource." + datasetID,
		URL:      u,
		Headers:  headers,
	})
	if err != nil {
		return nil, err
	}
	return ParseRows(fetch.Body)
}

// PageAll iterates the dataset in $limit-sized chunks. Caller terminates
// early by returning false from yield.
func (c *Client) PageAll(ctx context.Context, datasetID string, q Query, yield func(Row) bool) error {
	if q.Limit <= 0 {
		q.Limit = defaultPage
	}
	if q.Order == "" {
		q.Order = ":id"
	}
	for {
		page, err := c.FetchPage(ctx, datasetID, q)
		if err != nil {
			return err
		}
		for _, row := range page {
			if !yield(row) {
				return nil
			}
		}
		if len(page) < q.Limit {
			return nil
		}
		q.Offset += q.Limit
	}
}

// Count runs a $select=count(*) query.
func (c *Client) Count(ctx context.Context, datasetID, where string) (int64, error) {
	rows, err := c.FetchPage(ctx, datasetID, Query{
		Select: "count(*)",
		Where:  where,
		Limit:  1,
	})
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	switch v := rows[0]["count"].(type) {
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n, nil
	case float64:
		return int64(v), nil
	default:
		return 0, fmt.Errorf("pdc: unexpected count type %T", v)
	}
}

// Metadata fetches /api/views/<dataset_id>.
func (c *Client) Metadata(ctx context.Context, datasetID string) ([]byte, error) {
	u := c.BaseURL + "/api/views/" + url.PathEscape(datasetID)
	if c.AppToken != "" {
		u += "?$$app_token=" + url.QueryEscape(c.AppToken)
	}
	headers := http.Header{}
	headers.Set("Accept", "application/json")

	fetch, err := c.HTTP.Do(ctx, httpx.Request{
		System:   SystemName,
		Endpoint: "api.views." + datasetID,
		URL:      u,
		Headers:  headers,
	})
	if err != nil {
		return nil, err
	}
	return fetch.Body, nil
}

func (c *Client) url(datasetID string, q Query) (string, error) {
	if datasetID == "" {
		return "", fmt.Errorf("pdc: datasetID required")
	}
	v := url.Values{}
	if q.Select != "" {
		v.Set("$select", q.Select)
	}
	if q.Where != "" {
		v.Set("$where", q.Where)
	}
	if q.Order != "" {
		v.Set("$order", q.Order)
	}
	if q.Limit > 0 {
		v.Set("$limit", strconv.Itoa(q.Limit))
	}
	if q.Offset > 0 {
		v.Set("$offset", strconv.Itoa(q.Offset))
	}
	for k, vs := range q.Extra {
		for _, val := range vs {
			v.Add(k, val)
		}
	}
	if c.AppToken != "" {
		v.Set("$$app_token", c.AppToken)
	}
	return c.BaseURL + "/resource/" + url.PathEscape(datasetID) + ".json?" + v.Encode(), nil
}

// ParseRows decodes a Socrata array response.
func ParseRows(body []byte) ([]Row, error) {
	if len(body) == 0 {
		return nil, nil
	}
	var rows []Row
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("pdc decode rows: %w", err)
	}
	return rows, nil
}
