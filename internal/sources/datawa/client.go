// Package datawa implements clients for general data.wa.gov datasets beyond
// PDC, especially DES contracts/procurement and statewide open-data overlays.
package datawa

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

type MasterContractSale struct {
	SourceDatasetID      string
	SourceRowID          string
	CustomerType         string
	CustomerName         string
	ContractNumber       string
	ContractTitle        string
	VendorName           string
	ReportYear           int
	Q1SalesReported      string
	Q2SalesReported      string
	Q3SalesReported      string
	Q4SalesReported      string
	TotalSalesReported   string
	OMWBE                string
	VeteranOwned         string
	SmallBusiness        string
	DiverseOptions       string
	NormalizationWarning []string
}

func (c *Client) FetchAgencyContracts(ctx context.Context, fiscalYear int, q socrata.Query) ([]socrata.Row, error) {
	rows, _, err := c.FetchAgencyContractsWithSource(ctx, fiscalYear, q)
	return rows, err
}

func (c *Client) FetchAgencyContractsWithSource(ctx context.Context, fiscalYear int, q socrata.Query) ([]socrata.Row, httpx.RawFetch, error) {
	datasetID := AgencyContractFiscalYears[fiscalYear]
	if datasetID == "" {
		return nil, httpx.RawFetch{}, strconv.ErrSyntax
	}
	return c.FetchPageWithSource(ctx, datasetID, q)
}

func (c *Client) FetchMasterContractSalesWithSource(ctx context.Context, q socrata.Query) ([]socrata.Row, httpx.RawFetch, error) {
	return c.FetchPageWithSource(ctx, DatasetMasterContractSales, q)
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

func NormalizeMasterContractSale(row socrata.Row) MasterContractSale {
	year, yearWarn := parseIntField(first(row, "year", "report_year"), "year")
	q1 := first(row, "q1_sales_reported", "q1_sales")
	q2 := first(row, "q2_sales_reported", "q2_sales")
	q3 := first(row, "q3_sales_reported", "q3_sales")
	q4 := first(row, "q4_sales_reported", "q4_sales")
	return MasterContractSale{
		SourceDatasetID:      DatasetMasterContractSales,
		SourceRowID:          first(row, ":id", "sid", "id"),
		CustomerType:         first(row, "customer_type"),
		CustomerName:         first(row, "customer_name"),
		ContractNumber:       first(row, "contract_number"),
		ContractTitle:        first(row, "contract_title"),
		VendorName:           first(row, "vendor_name", "vendor"),
		ReportYear:           year,
		Q1SalesReported:      q1,
		Q2SalesReported:      q2,
		Q3SalesReported:      q3,
		Q4SalesReported:      q4,
		TotalSalesReported:   sumMoneyStrings(q1, q2, q3, q4),
		OMWBE:                first(row, "omwbe"),
		VeteranOwned:         first(row, "vet_owned", "veteran_owned"),
		SmallBusiness:        first(row, "small_business"),
		DiverseOptions:       first(row, "diverse_options"),
		NormalizationWarning: compact(yearWarn),
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
	default:
		return ""
	}
}

func parseIntField(raw, field string) (int, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, ""
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, "invalid_" + field + ":" + raw
	}
	return n, ""
}

func sumMoneyStrings(xs ...string) string {
	var total float64
	for _, x := range xs {
		x = strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(x), "$", ""), ",", "")
		if x == "" {
			continue
		}
		v, err := strconv.ParseFloat(x, 64)
		if err != nil {
			return ""
		}
		total += v
	}
	return strconv.FormatFloat(total, 'f', 2, 64)
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
