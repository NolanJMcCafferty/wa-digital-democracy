// Package seattle implements clients for Seattle Open Data Socrata datasets.
package seattle

import (
	"context"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/socrata"
)

const (
	SystemName     = "seattle_socrata"
	DefaultBaseURL = "https://data.seattle.gov"
)

const (
	DatasetBuildingPermitMap           = "5rc4-5s78"
	DatasetPermitReviewTime            = "crg2-ssqd"
	DatasetPlanComments                = "e285-aq8h"
	DatasetPlanReview                  = "tqk8-y2z5"
	DatasetCertificatesOfOccupancy     = "axkr-2p68"
	DatasetBuiltUnitsSince2010         = "p9qy-t26h"
	DatasetResidentialPermitsSince1990 = "rs98-eyib"
	DatasetOperatingBudget             = "8u2j-imqx"
	DatasetCapitalBudget               = "m6va-m4qe"
	DatasetCustomerServiceRequests     = "5ngg-rpne"
)

type Client struct{ *socrata.Client }

func New(h *httpx.Client, appToken string) *Client {
	return &Client{Client: socrata.New(h, SystemName, DefaultBaseURL, appToken)}
}

type Permit struct {
	SourceDatasetID string
	SourceRowID     string
	PermitNumber    string
	Status          string
	Address         string
	Description     string
	Category        string
	PermitType      string
	Value           string
	Latitude        string
	Longitude       string
	Raw             socrata.Row
}

func (c *Client) FetchBuildingPermits(ctx context.Context, q socrata.Query) ([]socrata.Row, error) {
	return c.FetchPage(ctx, DatasetBuildingPermitMap, q)
}

func NormalizePermit(datasetID string, row socrata.Row) Permit {
	return Permit{
		SourceDatasetID: datasetID,
		SourceRowID:     first(row, ":id", "sid", "id"),
		PermitNumber:    first(row, "permitnum", "permit_number", "permit_no"),
		Status:          first(row, "status", "permit_status"),
		Address:         first(row, "address", "original_address"),
		Description:     first(row, "description", "project_description"),
		Category:        first(row, "category", "permit_category"),
		PermitType:      first(row, "permit_type", "type"),
		Value:           first(row, "value", "estimated_value", "valuation"),
		Latitude:        first(row, "latitude"),
		Longitude:       first(row, "longitude"),
		Raw:             row,
	}
}

func first(row socrata.Row, keys ...string) string {
	for _, k := range keys {
		if v, ok := row[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}
