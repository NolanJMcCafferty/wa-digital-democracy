// Package kingcounty implements clients for King County Socrata open data.
package kingcounty

import (
	"context"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/socrata"
)

const (
	SystemName     = "kingcounty_socrata"
	DefaultBaseURL = "https://data.kingcounty.gov"
)

const (
	DatasetParcelViewer                 = "2kfd-2c3u"
	DatasetERealPropertySearch          = "4zym-vfd2"
	DatasetPropertyLegalDescriptions    = "4854-i48r"
	DatasetRealPropertyTaxReceivables   = "dkna-i698"
	DatasetVoterTurnoutByPrecinct       = "gj9e-v6eq"
	DatasetVoterRegistrationCensusTract = "4uz2-aqdz"
	DatasetFoodInspectionData           = "f29f-zza5"
	DatasetOffenseReports2020Present    = "4kmt-kfqf"
)

type Client struct{ *socrata.Client }

func New(h *httpx.Client, appToken string) *Client {
	return &Client{Client: socrata.New(h, SystemName, DefaultBaseURL, appToken)}
}

type Parcel struct {
	SourceDatasetID string
	SourceRowID     string
	PIN             string
	Address         string
	Jurisdiction    string
	Latitude        string
	Longitude       string
	Raw             socrata.Row
}

func (c *Client) FetchParcels(ctx context.Context, q socrata.Query) ([]socrata.Row, error) {
	return c.FetchPage(ctx, DatasetParcelViewer, q)
}

func NormalizeParcel(datasetID string, row socrata.Row) Parcel {
	return Parcel{
		SourceDatasetID: datasetID,
		SourceRowID:     first(row, ":id", "sid", "id"),
		PIN:             first(row, "pin", "parcel", "parcel_number", "major_minor"),
		Address:         first(row, "address", "site_address"),
		Jurisdiction:    first(row, "jurisdiction", "city"),
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
