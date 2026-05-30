// Package epa implements lightweight EPA ECHO/EJScreen fetch helpers.
package epa

import (
	"context"
	"net/http"
	"net/url"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/connector"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

const (
	SystemName         = "epa"
	DefaultECHOBaseURL = "https://echodata.epa.gov/echo"
)

var descriptor = connector.Descriptor{
	System:      SystemName,
	BaseURL:     DefaultECHOBaseURL,
	Description: "EPA ECHO facility search",
}

func init() { connector.Register(descriptor) }

type Client struct {
	HTTP        *httpx.Client
	ECHOBaseURL string
}

func New(h *httpx.Client) *Client {
	return &Client{HTTP: h, ECHOBaseURL: DefaultECHOBaseURL}
}

// Descriptor implements connector.Source.
func (c *Client) Descriptor() connector.Descriptor { return descriptor }

type FacilityRecord map[string]any

func (c *Client) FacilitySearch(ctx context.Context, params url.Values) ([]byte, error) {
	if params == nil {
		params = url.Values{}
	}
	params.Set("output", "JSON")
	if params.Get("p_st") == "" {
		params.Set("p_st", "WA")
	}
	u := c.ECHOBaseURL + "/eff_rest_services.get_facilities?" + params.Encode()
	fetch, err := c.HTTP.Do(ctx, httpx.Request{
		System:   SystemName,
		Endpoint: "echo.eff_rest_services.get_facilities",
		URL:      u,
		Headers:  jsonAccept(),
	})
	if err != nil {
		return nil, err
	}
	return fetch.Body, nil
}

func jsonAccept() http.Header {
	h := http.Header{}
	h.Set("Accept", "application/json")
	return h
}
