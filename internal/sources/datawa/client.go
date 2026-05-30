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

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/connector"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/socrata"
)

const (
	SystemName     = "datawa_socrata"
	DefaultBaseURL = "https://data.wa.gov"
)

var descriptor = connector.Descriptor{
	System:      SystemName,
	BaseURL:     DefaultBaseURL,
	Description: "data.wa.gov general datasets (DES contracts, statewide overlays)",
}

func init() { connector.Register(descriptor) }

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
	DatasetWEBSVendors           = "3kwi-7zsj"
)

var AgencyContractFiscalYears = map[int]string{
	2025: DatasetAgencyContractsFY2025,
	2024: DatasetAgencyContractsFY2024,
	2023: DatasetAgencyContractsFY2023,
	2022: DatasetAgencyContractsFY2022,
}

var ITContractFiscalYears = map[int]string{
	2025: DatasetITContractsFY2025,
	2024: DatasetITContractsFY2024,
	2023: DatasetITContractsFY2023,
	2022: DatasetITContractsFY2022,
}

type Client struct{ *socrata.Client }

func New(h *httpx.Client, appToken string) *Client {
	return &Client{Client: socrata.New(h, SystemName, DefaultBaseURL, appToken)}
}

// Descriptor implements connector.Source.
func (c *Client) Descriptor() connector.Descriptor { return descriptor }

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

type ITContract struct {
	SourceDatasetID           string
	SourceRowID               string
	ReportFiscalYear          int
	AgencyNumberAgencyName    string
	AgencyNumber              string
	AgencyName                string
	ContractNumber            string
	ContractorName            string
	ContractorDBA             string
	CooperativePurchase       *bool
	CooperativeName           string
	StatewideContractPurchase *bool
	ContractStartDate         *time.Time
	ContractEndDate           *time.Time
	FiscalYearStart           string
	FiscalYearEnd             string
	ITTowerApplication        string
	ITTowerCompute            string
	ITTowerDataCenter         string
	ITTowerDelivery           string
	ITTowerEndUser            string
	ITTowerITManagement       string
	ITTowerNetwork            string
	ITTowerOutput             string
	ITTowerPlatform           string
	ITTowerSecurity           string
	ITTowerStorage            string
	OtherNonIT                string
	TotalPercentage           string
	ContractAmountFY20        string
	ContractAmountFY21        string
	ContractAmountFY22        string
	ContractAmountFY23        string
	ContractAmountFY24        string
	ContractAmountFY25        string
	ContractAmountFY26        string
	ContractAmountFY27        string
	ContractAmountFY28        string
	ContractAmountFY29        string
	ContractAmountFY30        string
	TotalContractAmount       string
	ContractAmountExplanation string
	NormalizationWarning      []string
}

type WEBSVendor struct {
	SourceDatasetID       string
	SourceRowID           string
	CompanyName           string
	NormalizedCompanyName string
	DBAName               string
	PhoneNumber           string
	ContactEmail          string
	City                  string
	State                 string
	WebAddress            string
	CommodityCode         string
	DescriptionOfWork     string
	SmallBusiness         string
	VeteranOwned          string
	OtherCert             string
	OtherCert2            string
	NormalizationWarning  []string
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

func (c *Client) FetchITContractsWithSource(ctx context.Context, fiscalYear int, q socrata.Query) ([]socrata.Row, httpx.RawFetch, error) {
	datasetID := ITContractFiscalYears[fiscalYear]
	if datasetID == "" {
		return nil, httpx.RawFetch{}, strconv.ErrSyntax
	}
	return c.FetchPageWithSource(ctx, datasetID, q)
}

func (c *Client) FetchWEBSVendorsWithSource(ctx context.Context, q socrata.Query) ([]socrata.Row, httpx.RawFetch, error) {
	return c.FetchPageWithSource(ctx, DatasetWEBSVendors, q)
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

func NormalizeITContract(datasetID string, fiscalYear int, row socrata.Row) ITContract {
	start, startWarn := parseContractDate(first(row, "contract_start_date", "start_date"))
	end, endWarn := parseContractDate(first(row, "contract_end_date", "end_date"))
	agencyNumber, agencyName := splitAgencyNumberName(first(row, "agency_number_agency_name"))
	coopPurchase, coopPurchaseWarn := parseBoolField(first(row, "cooperative_purchase_yes", "cooperative_purchase"), "cooperative_purchase")
	statewidePurchase, statewidePurchaseWarn := parseBoolField(first(row, "was_this_purchased_through", "statewide_contract_purchase"), "statewide_contract_purchase")
	return ITContract{
		SourceDatasetID:           datasetID,
		SourceRowID:               first(row, ":id", "sid", "id"),
		ReportFiscalYear:          fiscalYear,
		AgencyNumberAgencyName:    first(row, "agency_number_agency_name"),
		AgencyNumber:              agencyNumber,
		AgencyName:                agencyName,
		ContractNumber:            first(row, "contract_no", "contract_number"),
		ContractorName:            first(row, "contractor_name"),
		ContractorDBA:             first(row, "contractor_name_d_b_a_optional", "contractor_dba"),
		CooperativePurchase:       coopPurchase,
		CooperativeName:           first(row, "cooperative_name_if_applicable", "cooperative_name"),
		StatewideContractPurchase: statewidePurchase,
		ContractStartDate:         start,
		ContractEndDate:           end,
		FiscalYearStart:           first(row, "fiscal_year_start"),
		FiscalYearEnd:             first(row, "fiscal_year_end"),
		ITTowerApplication:        first(row, "it_tower_application"),
		ITTowerCompute:            first(row, "it_tower_compute"),
		ITTowerDataCenter:         first(row, "it_tower_data_center"),
		ITTowerDelivery:           first(row, "it_tower_delivery"),
		ITTowerEndUser:            first(row, "it_tower_end_user"),
		ITTowerITManagement:       first(row, "it_tower_it_management"),
		ITTowerNetwork:            first(row, "it_tower_network"),
		ITTowerOutput:             first(row, "it_tower_output"),
		ITTowerPlatform:           first(row, "it_tower_platform"),
		ITTowerSecurity:           first(row, "it_tower_security"),
		ITTowerStorage:            first(row, "it_tower_storage"),
		OtherNonIT:                first(row, "other_non_it"),
		TotalPercentage:           first(row, "total_percentage_auto", "total_percentage"),
		ContractAmountFY20:        first(row, "contract_amount_fy20"),
		ContractAmountFY21:        first(row, "contract_amount_fy21"),
		ContractAmountFY22:        first(row, "contract_amount_fy22"),
		ContractAmountFY23:        first(row, "contract_amount_fy23"),
		ContractAmountFY24:        first(row, "contract_amount_fy24"),
		ContractAmountFY25:        first(row, "contract_amount_fy25"),
		ContractAmountFY26:        first(row, "contract_amount_fy26"),
		ContractAmountFY27:        first(row, "contract_amount_fy27"),
		ContractAmountFY28:        first(row, "contract_amount_fy28"),
		ContractAmountFY29:        first(row, "contract_amount_fy29"),
		ContractAmountFY30:        first(row, "contract_amount_fy30"),
		TotalContractAmount:       first(row, "total_contract_amount"),
		ContractAmountExplanation: first(row, "explanation_of_contract_amount", "contract_amount_explanation"),
		NormalizationWarning:      compact(startWarn, endWarn, coopPurchaseWarn, statewidePurchaseWarn),
	}
}

func NormalizeWEBSVendor(row socrata.Row) WEBSVendor {
	companyName := first(row, "company_name", "vendor_name", "business_name")
	return WEBSVendor{
		SourceDatasetID:       DatasetWEBSVendors,
		SourceRowID:           first(row, ":id", "sid", "id"),
		CompanyName:           companyName,
		NormalizedCompanyName: NormalizeEntityName(companyName),
		DBAName:               first(row, "dba_name", "doing_business_as"),
		PhoneNumber:           first(row, "phone_number", "phone"),
		ContactEmail:          first(row, "contact_email", "email"),
		City:                  first(row, "city"),
		State:                 first(row, "state"),
		WebAddress:            first(row, "web_address", "website", "url"),
		CommodityCode:         first(row, "code", "commodity_code"),
		DescriptionOfWork:     first(row, "description_of_work", "commodity_description"),
		SmallBusiness:         first(row, "small_business"),
		VeteranOwned:          first(row, "veteran_owned", "vet_owned"),
		OtherCert:             first(row, "other_cert"),
		OtherCert2:            first(row, "other_cert_2"),
	}
}

func NormalizeEntityName(name string) string {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(name)))
	return strings.Join(fields, " ")
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

func splitAgencyNumberName(raw string) (string, string) {
	parts := strings.SplitN(strings.TrimSpace(raw), " - ", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return "", strings.TrimSpace(raw)
}

func parseBoolField(raw, field string) (*bool, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ""
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, "invalid_" + field + ":" + raw
	}
	return &b, ""
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
