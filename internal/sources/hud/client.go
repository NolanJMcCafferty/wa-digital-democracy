// Package hud implements lightweight HUD dataset/file fetch helpers.
package hud

import (
	"context"
	"net/http"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/connector"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

const (
	SystemName         = "hud"
	DefaultDatasetsURL = "https://data.hud.gov/datasets"
)

var descriptor = connector.Descriptor{
	System:      SystemName,
	BaseURL:     DefaultDatasetsURL,
	Description: "HUD datasets index and file fetch",
}

func init() { connector.Register(descriptor) }

type Client struct {
	HTTP        *httpx.Client
	DatasetsURL string
}

func New(h *httpx.Client) *Client {
	return &Client{HTTP: h, DatasetsURL: DefaultDatasetsURL}
}

// Descriptor implements connector.Source.
func (c *Client) Descriptor() connector.Descriptor { return descriptor }

type DatasetRef struct {
	Name      string
	URL       string
	Kind      string
	Program   string
	Geography string
	Year      int
	Notes     string
}

func (c *Client) FetchDatasetsPage(ctx context.Context) ([]byte, error) {
	fetch, err := c.HTTP.Do(ctx, httpx.Request{
		System:   SystemName,
		Endpoint: "datasets.page",
		URL:      c.DatasetsURL,
		Headers:  htmlAccept(),
	})
	if err != nil {
		return nil, err
	}
	return fetch.Body, nil
}

func (c *Client) FetchKnownFile(ctx context.Context, u string) ([]byte, error) {
	fetch, err := c.HTTP.Do(ctx, httpx.Request{
		System:   SystemName,
		Endpoint: "known_file",
		URL:      u,
	})
	if err != nil {
		return nil, err
	}
	return fetch.Body, nil
}

func htmlAccept() http.Header {
	h := http.Header{}
	h.Set("Accept", "text/html,application/xhtml+xml")
	return h
}
