// Package socrata provides a small reusable client for Socrata/Tyler open-data
// portals. It is intentionally source-agnostic: callers decide the source
// system name used for provenance and the dataset IDs to query.
package socrata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

const defaultPage = 50000

// Row is a single Socrata row as returned by the JSON endpoint. Dataset
// schemas drift, so callers should normalize from generic maps.
type Row map[string]any

// Metadata is a lightly-typed Socrata dataset metadata object. The full raw
// metadata remains available through Raw.
type Metadata struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Domain      string         `json:"domain"`
	WebURI      string         `json:"webUri"`
	DataURI     string         `json:"dataUri"`
	Raw         map[string]any `json:"-"`
}

// Client wraps an httpx.Client for one Socrata domain.
type Client struct {
	HTTP       *httpx.Client
	BaseURL    string
	SystemName string
	AppToken   string // optional; sent as $$app_token query param when needed
}

func New(h *httpx.Client, systemName, baseURL, appToken string) *Client {
	return &Client{HTTP: h, SystemName: systemName, BaseURL: baseURL, AppToken: appToken}
}

// Query is a single SoQL query against a dataset.
type Query struct {
	Select string // $select
	Where  string // $where
	Order  string // $order
	Limit  int    // $limit; defaults are caller-controlled except PageAll
	Offset int    // $offset
	Extra  url.Values
}

func (c *Client) FetchPage(ctx context.Context, datasetID string, q Query) ([]Row, error) {
	rows, _, err := c.FetchPageWithSource(ctx, datasetID, q)
	return rows, err
}

func (c *Client) FetchPageWithSource(ctx context.Context, datasetID string, q Query) ([]Row, httpx.RawFetch, error) {
	u, err := c.ResourceURL(datasetID, q)
	if err != nil {
		return nil, httpx.RawFetch{}, err
	}
	headers := http.Header{}
	headers.Set("Accept", "application/json")
	fetch, err := c.HTTP.Do(ctx, httpx.Request{
		System:   c.SystemName,
		Endpoint: "resource." + datasetID,
		URL:      u,
		Headers:  headers,
	})
	if err != nil {
		return nil, fetch, err
	}
	rows, err := ParseRows(fetch.Body)
	if err != nil {
		return nil, fetch, err
	}
	return rows, fetch, nil
}

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

func (c *Client) Count(ctx context.Context, datasetID, where string) (int64, error) {
	rows, err := c.FetchPage(ctx, datasetID, Query{Select: "count(*)", Where: where, Limit: 1})
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
		return 0, fmt.Errorf("socrata: unexpected count type %T", v)
	}
}

func (c *Client) Metadata(ctx context.Context, datasetID string) (*Metadata, error) {
	u := c.BaseURL + "/api/views/" + url.PathEscape(datasetID)
	if c.AppToken != "" {
		u += "?$$app_token=" + url.QueryEscape(c.AppToken)
	}
	fetch, err := c.HTTP.Do(ctx, httpx.Request{
		System:   c.SystemName,
		Endpoint: "api.views." + datasetID,
		URL:      u,
		Headers:  jsonAccept(),
	})
	if err != nil {
		return nil, err
	}
	return ParseMetadata(fetch.Body)
}

func (c *Client) Catalog(ctx context.Context) ([]Metadata, error) {
	u := c.BaseURL + "/api/views/metadata/v1"
	if c.AppToken != "" {
		u += "?$$app_token=" + url.QueryEscape(c.AppToken)
	}
	fetch, err := c.HTTP.Do(ctx, httpx.Request{
		System:   c.SystemName,
		Endpoint: "api.views.metadata.v1",
		URL:      u,
		Headers:  jsonAccept(),
	})
	if err != nil {
		return nil, err
	}
	return ParseCatalog(fetch.Body)
}

func (c *Client) ResourceURL(datasetID string, q Query) (string, error) {
	if datasetID == "" {
		return "", fmt.Errorf("socrata: datasetID required")
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

func ParseRows(body []byte) ([]Row, error) {
	if len(body) == 0 {
		return nil, nil
	}
	var rows []Row
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("socrata decode rows: %w", err)
	}
	return rows, nil
}

func ParseMetadata(body []byte) (*Metadata, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("socrata decode metadata: %w", err)
	}
	m := metadataFromRaw(raw)
	return &m, nil
}

func ParseCatalog(body []byte) ([]Metadata, error) {
	var raws []map[string]any
	if err := json.Unmarshal(body, &raws); err != nil {
		return nil, fmt.Errorf("socrata decode catalog: %w", err)
	}
	out := make([]Metadata, 0, len(raws))
	for _, raw := range raws {
		out = append(out, metadataFromRaw(raw))
	}
	return out, nil
}

func metadataFromRaw(raw map[string]any) Metadata {
	return Metadata{
		ID:          str(raw["id"]),
		Name:        str(raw["name"]),
		Description: str(raw["description"]),
		Domain:      str(raw["domain"]),
		WebURI:      str(raw["webUri"]),
		DataURI:     str(raw["dataUri"]),
		Raw:         raw,
	}
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func jsonAccept() http.Header {
	h := http.Header{}
	h.Set("Accept", "application/json")
	return h
}
