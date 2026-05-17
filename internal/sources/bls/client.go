// Package bls implements the Bureau of Labor Statistics public API client.
package bls

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

const (
	SystemName     = "bls"
	DefaultBaseURL = "https://api.bls.gov/publicAPI/v2"
)

type Client struct {
	HTTP            *httpx.Client
	BaseURL         string
	RegistrationKey string
}

func New(h *httpx.Client, registrationKey string) *Client {
	return &Client{HTTP: h, BaseURL: DefaultBaseURL, RegistrationKey: registrationKey}
}

type TimeSeriesRequest struct {
	SeriesID        []string `json:"seriesid"`
	StartYear       string   `json:"startyear,omitempty"`
	EndYear         string   `json:"endyear,omitempty"`
	RegistrationKey string   `json:"registrationkey,omitempty"`
}

type TimeSeriesResponse struct {
	Status       string   `json:"status"`
	ResponseTime int      `json:"responseTime"`
	Message      []string `json:"message"`
	Results      struct {
		Series []Series `json:"series"`
	} `json:"Results"`
}

type Series struct {
	SeriesID string        `json:"seriesID"`
	Data     []Observation `json:"data"`
}

type Observation struct {
	Year       string     `json:"year"`
	Period     string     `json:"period"`
	PeriodName string     `json:"periodName"`
	Value      string     `json:"value"`
	Footnotes  []Footnote `json:"footnotes"`
}

type Footnote struct {
	Code string `json:"code"`
	Text string `json:"text"`
}

func (c *Client) TimeSeries(ctx context.Context, req TimeSeriesRequest) (*TimeSeriesResponse, error) {
	if req.RegistrationKey == "" {
		req.RegistrationKey = c.RegistrationKey
	}
	body, _ := json.Marshal(req)
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/json")
	fetch, err := c.HTTP.Do(ctx, httpx.Request{System: SystemName, Endpoint: "timeseries.data", Method: http.MethodPost, URL: c.BaseURL + "/timeseries/data/", Headers: headers, Body: body})
	if err != nil {
		return nil, err
	}
	return ParseTimeSeries(fetch.Body)
}

func ParseTimeSeries(body []byte) (*TimeSeriesResponse, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	var out TimeSeriesResponse
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("bls timeseries: %w", err)
	}
	return &out, nil
}
