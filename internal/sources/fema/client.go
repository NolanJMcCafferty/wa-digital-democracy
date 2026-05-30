// Package fema implements an OpenFEMA API v2 client.
package fema

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/connector"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

const (
	SystemName                  = "openfema"
	DefaultBaseURL              = "https://www.fema.gov/api/open/v2"
	DatasetDisasterDeclarations = "DisasterDeclarationsSummaries"
)

var descriptor = connector.Descriptor{
	System:      SystemName,
	BaseURL:     DefaultBaseURL,
	Description: "OpenFEMA API v2",
}

func init() { connector.Register(descriptor) }

type Client struct {
	HTTP    *httpx.Client
	BaseURL string
}

func New(h *httpx.Client) *Client { return &Client{HTTP: h, BaseURL: DefaultBaseURL} }

// Descriptor implements connector.Source.
func (c *Client) Descriptor() connector.Descriptor { return descriptor }

type Query struct {
	Entity  string
	Filter  string
	Select  []string
	OrderBy string
	Top     int
	Skip    int
	Count   bool
}

func (c *Client) Query(ctx context.Context, q Query) ([]map[string]any, error) {
	u, err := c.URL(q)
	if err != nil {
		return nil, err
	}
	fetch, err := c.HTTP.Do(ctx, httpx.Request{System: SystemName, Endpoint: "entity." + q.Entity, URL: u, Headers: jsonAccept()})
	if err != nil {
		return nil, err
	}
	return ParseEntity(fetch.Body, q.Entity)
}

func (c *Client) WashingtonDisasters(ctx context.Context) ([]map[string]any, error) {
	return c.Query(ctx, Query{Entity: DatasetDisasterDeclarations, Filter: "state eq 'WA'", Top: 1000})
}

func (c *Client) URL(q Query) (string, error) {
	if q.Entity == "" {
		return "", fmt.Errorf("fema: entity required")
	}
	v := url.Values{}
	if q.Filter != "" {
		v.Set("$filter", q.Filter)
	}
	if len(q.Select) > 0 {
		v.Set("$select", strings.Join(q.Select, ","))
	}
	if q.OrderBy != "" {
		v.Set("$orderby", q.OrderBy)
	}
	if q.Top > 0 {
		v.Set("$top", strconv.Itoa(q.Top))
	}
	if q.Skip > 0 {
		v.Set("$skip", strconv.Itoa(q.Skip))
	}
	if q.Count {
		v.Set("$count", "true")
	}
	v.Set("$format", "json")
	return c.BaseURL + "/" + url.PathEscape(q.Entity) + "?" + v.Encode(), nil
}

func ParseEntity(body []byte, entity string) ([]map[string]any, error) {
	var raw map[string][]map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("fema entity: %w", err)
	}
	return raw[entity], nil
}

func jsonAccept() http.Header {
	h := http.Header{}
	h.Set("Accept", "application/json")
	return h
}
