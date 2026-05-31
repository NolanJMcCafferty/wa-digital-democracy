// Package pdc is the Washington Public Disclosure Commission connector,
// reading via the data.wa.gov Socrata API.
//
// Per data-sources/05:
//   - Endpoint pattern: GET https://data.wa.gov/resource/<dataset_id>.json
//   - Optional X-App-Token / $$app_token query param. Leave blank until
//     broader ingestion hits throttling; public reads work without it for the
//     first-page MVP demo.
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
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/connector"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/socrata"
)

const (
	SystemName     = "pdc_socrata"
	DefaultBaseURL = "https://data.wa.gov"
)

var descriptor = connector.Descriptor{
	System:      SystemName,
	BaseURL:     DefaultBaseURL,
	Description: "Washington Public Disclosure Commission via data.wa.gov Socrata",
}

func init() { connector.Register(descriptor) }

// Dataset IDs for the first-page pipeline.
const (
	DatasetLobbyistEmployment   = "xhn7-64im"
	DatasetLobbyistCompensation = "9nnw-c693"
	DatasetContributions        = "2jwd-akfb"
)

// Row and Query are re-exports of the shared Socrata types.
type (
	Row   = socrata.Row
	Query = socrata.Query
)

// ParseRows is re-exported for tests and callers that previously imported it
// from this package.
var ParseRows = socrata.ParseRows

// Client wraps the shared socrata.Client with PDC-specific defaults.
type Client struct{ *socrata.Client }

// New returns a Client.
func New(h *httpx.Client, appToken string) *Client {
	return &Client{Client: socrata.New(h, SystemName, DefaultBaseURL, appToken)}
}

// Descriptor implements connector.Source.
func (c *Client) Descriptor() connector.Descriptor { return descriptor }
