// Package seattle implements clients for Seattle Open Data Socrata datasets.
package seattle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"

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

type OperatingBudget struct {
	SourceDatasetID string
	SourceRowID     string
	FiscalYear      int
	Service         string
	Department      string
	Program         string
	Fund            string
	FundType        string
	ExpenseType     string
	Description     string
	ApprovedAmount  string
	Raw             socrata.Row
}

func (c *Client) FetchBuildingPermits(ctx context.Context, q socrata.Query) ([]socrata.Row, error) {
	return c.FetchPage(ctx, DatasetBuildingPermitMap, q)
}

func (c *Client) FetchOperatingBudgetWithSource(ctx context.Context, q socrata.Query) ([]socrata.Row, httpx.RawFetch, error) {
	return c.FetchPageWithSource(ctx, DatasetOperatingBudget, q)
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

func NormalizeOperatingBudget(row socrata.Row) OperatingBudget {
	fy, _ := strconv.Atoi(first(row, "fiscal_year"))
	return OperatingBudget{
		SourceDatasetID: DatasetOperatingBudget,
		SourceRowID:     first(row, ":id", "sid", "id"),
		FiscalYear:      fy,
		Service:         first(row, "service"),
		Department:      first(row, "department"),
		Program:         first(row, "program"),
		Fund:            first(row, "fund"),
		FundType:        first(row, "fund_type"),
		ExpenseType:     first(row, "expense_type"),
		Description:     first(row, "description"),
		ApprovedAmount:  first(row, "approved_amount"),
		Raw:             row,
	}
}

func StableRowID(row socrata.Row) string {
	body, _ := json.Marshal(row)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func first(row socrata.Row, keys ...string) string {
	for _, k := range keys {
		if v, ok := row[k]; ok && v != nil {
			s := strings.TrimSpace(toString(v))
			if s != "" {
				return s
			}
		}
	}
	return ""
}

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	default:
		return ""
	}
}
