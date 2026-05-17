// Package census implements a Census ACS API client.
package census

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

const (
	SystemName     = "census"
	DefaultBaseURL = "https://api.census.gov/data"
)

type Client struct {
	HTTP    *httpx.Client
	BaseURL string
	APIKey  string
}

func New(h *httpx.Client, apiKey string) *Client {
	return &Client{HTTP: h, BaseURL: DefaultBaseURL, APIKey: apiKey}
}

type Query struct {
	Year      string
	Dataset   string // e.g. acs/acs5/profile
	Get       []string
	For       string
	In        []string
	Variables bool
}

func (c *Client) Fetch(ctx context.Context, q Query) ([]map[string]string, error) {
	u, err := c.URL(q)
	if err != nil {
		return nil, err
	}
	fetch, err := c.HTTP.Do(ctx, httpx.Request{System: SystemName, Endpoint: q.Year + "/" + q.Dataset, URL: u, Headers: jsonAccept()})
	if err != nil {
		return nil, err
	}
	return ParseTable(fetch.Body)
}

func (c *Client) Variables(ctx context.Context, year, dataset string) (map[string]Variable, error) {
	u := c.BaseURL + "/" + year + "/" + strings.Trim(dataset, "/") + "/variables.json"
	fetch, err := c.HTTP.Do(ctx, httpx.Request{System: SystemName, Endpoint: year + "/" + dataset + ".variables", URL: u, Headers: jsonAccept()})
	if err != nil {
		return nil, err
	}
	return ParseVariables(fetch.Body)
}

func (c *Client) URL(q Query) (string, error) {
	if q.Year == "" || q.Dataset == "" || q.For == "" || len(q.Get) == 0 {
		return "", fmt.Errorf("census: year, dataset, get, and for are required")
	}
	v := url.Values{}
	v.Set("get", strings.Join(q.Get, ","))
	v.Set("for", q.For)
	for _, in := range q.In {
		v.Add("in", in)
	}
	if c.APIKey != "" {
		v.Set("key", c.APIKey)
	}
	return c.BaseURL + "/" + q.Year + "/" + strings.Trim(q.Dataset, "/") + "?" + v.Encode(), nil
}

type Variable struct {
	Name    string
	Label   string `json:"label"`
	Concept string `json:"concept"`
	Group   string `json:"group"`
}

func ParseTable(body []byte) ([]map[string]string, error) {
	var table [][]string
	if err := json.Unmarshal(body, &table); err != nil {
		return nil, fmt.Errorf("census table: %w", err)
	}
	if len(table) == 0 {
		return nil, nil
	}
	headers := table[0]
	out := make([]map[string]string, 0, len(table)-1)
	for _, row := range table[1:] {
		m := map[string]string{}
		for i, h := range headers {
			if i < len(row) {
				m[h] = row[i]
			}
		}
		out = append(out, m)
	}
	return out, nil
}

func ParseVariables(body []byte) (map[string]Variable, error) {
	var raw struct {
		Variables map[string]Variable `json:"variables"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("census variables: %w", err)
	}
	for k, v := range raw.Variables {
		v.Name = k
		raw.Variables[k] = v
	}
	return raw.Variables, nil
}

func jsonAccept() http.Header { h := http.Header{}; h.Set("Accept", "application/json"); return h }
