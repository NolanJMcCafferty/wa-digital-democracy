// Package datawa implements clients for general data.wa.gov datasets beyond
// PDC, especially DES contracts/procurement and statewide open-data overlays.
package datawa

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/socrata"
)

const (
	SystemName     = "datawa_socrata"
	DefaultBaseURL = "https://data.wa.gov"
)

const (
	DatasetAgencyContractsFY2025 = "6fx9-ncas"
	DatasetAgencyContractsFY2024 = "s8d5-pj78"
	DatasetAgencyContractsFY2023 = "mz6y-pfem"
	DatasetAgencyContractsFY2022 = "pwse-3zea"
	DatasetMasterContractSales   = "n8q6-4twj"
	DatasetITContractsFY2025     = "3txe-z9i9"
	DatasetITContractsFY2024     = "ktim-amuz"
	DatasetITContractsFY2023     = "hycx-v82h"
	DatasetITContractsFY2022     = "dzvi-rs2c"
)

var AgencyContractFiscalYears = map[int]string{
	2025: DatasetAgencyContractsFY2025,
	2024: DatasetAgencyContractsFY2024,
	2023: DatasetAgencyContractsFY2023,
	2022: DatasetAgencyContractsFY2022,
}

type Client struct{ *socrata.Client }

func New(h *httpx.Client, appToken string) *Client {
	return &Client{Client: socrata.New(h, SystemName, DefaultBaseURL, appToken)}
}

type Contract struct {
	SourceDatasetID      string
	SourceRowID          string
	FiscalYear           int
	AgencyName           string
	AgencyNumber         string
	ContractNumber       string
	AmendmentNumber      string
	ContractorName       string
	StatewideVendorNum   string
	Description          string
	StartDate            *time.Time
	EndDate              *time.Time
	PeriodStart          *time.Time
	PeriodEnd            *time.Time
	FederalAmount        string
	StateAmount          string
	OtherAmount          string
	TotalAmount          string
	ProcurementType      string
	MinorityWomanOwned   string
	SmallBusiness        string
	VeteranOwned         string
	NormalizationWarning []string
}

func (c *Client) FetchAgencyContracts(ctx context.Context, fiscalYear int, q socrata.Query) ([]socrata.Row, error) {
	datasetID := AgencyContractFiscalYears[fiscalYear]
	if datasetID == "" {
		return nil, strconv.ErrSyntax
	}
	return c.FetchPage(ctx, datasetID, q)
}

func NormalizeContract(datasetID string, fiscalYear int, row socrata.Row) Contract {
	start, startWarn := parseContractDate(first(row, "start_date", "contract_start_date", "begin_date"))
	end, endWarn := parseContractDate(first(row, "end_date", "contract_end_date"))
	periodStart, periodStartWarn := parseContractDate(first(row, "period_start", "period_start_date"))
	periodEnd, periodEndWarn := parseContractDate(first(row, "period_end", "period_end_date"))
	warnings := compact(startWarn, endWarn, periodStartWarn, periodEndWarn)
	return Contract{
		SourceDatasetID:      datasetID,
		SourceRowID:          first(row, ":id", "sid", "id"),
		FiscalYear:           fiscalYear,
		AgencyName:           first(row, "agency_name", "agency", "agency_title"),
		AgencyNumber:         first(row, "agency_number", "agency_num"),
		ContractNumber:       first(row, "contract_number", "contract_no", "contract"),
		AmendmentNumber:      first(row, "amendment_number", "amendment_no"),
		ContractorName:       first(row, "contractor_name", "vendor_name", "vendor", "contractor"),
		StatewideVendorNum:   first(row, "statewide_vendor_number", "vendor_number", "swv_number"),
		Description:          first(row, "description", "contract_description", "purpose"),
		StartDate:            start,
		EndDate:              end,
		PeriodStart:          periodStart,
		PeriodEnd:            periodEnd,
		FederalAmount:        first(row, "federal_amount", "federal_funds"),
		StateAmount:          first(row, "state_amount", "state_funds"),
		OtherAmount:          first(row, "other_amount", "other_funds"),
		TotalAmount:          first(row, "total_amount", "contract_amount", "amount"),
		ProcurementType:      first(row, "procurement_type", "procurement_method"),
		MinorityWomanOwned:   first(row, "minority_woman_owned", "m_w_owned"),
		SmallBusiness:        first(row, "small_business"),
		VeteranOwned:         first(row, "veteran_owned"),
		NormalizationWarning: warnings,
	}
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
	default:
		return ""
	}
}

func parseContractDate(raw string) (*time.Time, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ""
	}
	if strings.HasPrefix(raw, "9999") {
		return nil, "sentinel_date:" + raw
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02", "01/02/2006"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return &t, ""
		}
	}
	return nil, "invalid_date:" + raw
}

func compact(xs ...string) []string {
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}
