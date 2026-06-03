package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/entitymatch"
)

// UpsertDataWAContractParams is the normalized row shape for datawa_contract.
type UpsertDataWAContractParams struct {
	SourceDatasetID          string
	SourceRowID              string
	FiscalYear               int
	AgencyName               string
	AgencyNumber             string
	ContractNumber           string
	AmendmentNumber          string
	ContractorName           string
	NormalizedContractorName string
	StatewideVendorNumber    string
	Description              string
	StartDate                *time.Time
	EndDate                  *time.Time
	PeriodStart              *time.Time
	PeriodEnd                *time.Time
	FederalAmount            string
	StateAmount              string
	OtherAmount              string
	TotalAmount              string
	ProcurementType          string
	MinorityWomanOwned       string
	SmallBusiness            string
	VeteranOwned             string
	Warnings                 []string
	RawFields                map[string]any
}

// UpsertDataWAContract inserts or updates one normalized data.wa.gov contract row.
func (s *Store) UpsertDataWAContract(ctx context.Context, p UpsertDataWAContractParams) error {
	warnings, err := json.Marshal(p.Warnings)
	if err != nil {
		return fmt.Errorf("marshal warnings: %w", err)
	}
	raw, err := json.Marshal(p.RawFields)
	if err != nil {
		return fmt.Errorf("marshal raw fields: %w", err)
	}
	const q = `
INSERT INTO datawa_contract (
  source_dataset_id, source_row_id, fiscal_year, agency_name, agency_number,
  contract_number, amendment_number, contractor_name, normalized_contractor_name,
  statewide_vendor_number, description, start_date, end_date, period_start, period_end,
  federal_amount, state_amount, other_amount, total_amount, procurement_type,
  minority_woman_owned, small_business, veteran_owned, normalization_warnings,
  raw_fields
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,
  NULLIF($16,'')::numeric, NULLIF($17,'')::numeric, NULLIF($18,'')::numeric, NULLIF($19,'')::numeric,
  $20,$21,$22,$23,$24::jsonb,$25::jsonb
)
ON CONFLICT (source_dataset_id, source_row_id) DO UPDATE SET
  fiscal_year = EXCLUDED.fiscal_year,
  agency_name = EXCLUDED.agency_name,
  agency_number = EXCLUDED.agency_number,
  contract_number = EXCLUDED.contract_number,
  amendment_number = EXCLUDED.amendment_number,
  contractor_name = EXCLUDED.contractor_name,
  normalized_contractor_name = EXCLUDED.normalized_contractor_name,
  statewide_vendor_number = EXCLUDED.statewide_vendor_number,
  description = EXCLUDED.description,
  start_date = EXCLUDED.start_date,
  end_date = EXCLUDED.end_date,
  period_start = EXCLUDED.period_start,
  period_end = EXCLUDED.period_end,
  federal_amount = EXCLUDED.federal_amount,
  state_amount = EXCLUDED.state_amount,
  other_amount = EXCLUDED.other_amount,
  total_amount = EXCLUDED.total_amount,
  procurement_type = EXCLUDED.procurement_type,
  minority_woman_owned = EXCLUDED.minority_woman_owned,
  small_business = EXCLUDED.small_business,
  veteran_owned = EXCLUDED.veteran_owned,
  normalization_warnings = EXCLUDED.normalization_warnings,
  raw_fields = EXCLUDED.raw_fields,
  updated_at = NOW();`
	_, err = s.Pool.Exec(ctx, q,
		p.SourceDatasetID, p.SourceRowID, p.FiscalYear, strOrNull(p.AgencyName), strOrNull(p.AgencyNumber),
		strOrNull(p.ContractNumber), strOrNull(p.AmendmentNumber), strOrNull(p.ContractorName), strOrNull(defaultStr(p.NormalizedContractorName, entitymatch.NormalizedName(p.ContractorName))),
		strOrNull(p.StatewideVendorNumber), strOrNull(p.Description), datePtrOrNull(p.StartDate), datePtrOrNull(p.EndDate), datePtrOrNull(p.PeriodStart), datePtrOrNull(p.PeriodEnd),
		p.FederalAmount, p.StateAmount, p.OtherAmount, p.TotalAmount, strOrNull(p.ProcurementType),
		strOrNull(p.MinorityWomanOwned), strOrNull(p.SmallBusiness), strOrNull(p.VeteranOwned), string(warnings), string(raw),
	)
	if err != nil {
		return fmt.Errorf("upsert datawa_contract: %w", err)
	}
	return nil
}

// UpsertDataWAMasterContractSaleParams is the normalized row shape for
// datawa_master_contract_sale.
type UpsertDataWAMasterContractSaleParams struct {
	SourceDatasetID        string
	SourceRowID            string
	CustomerType           string
	CustomerName           string
	NormalizedCustomerName string
	ContractNumber         string
	ContractTitle          string
	VendorName             string
	NormalizedVendorName   string
	ReportYear             int
	Q1SalesReported        string
	Q2SalesReported        string
	Q3SalesReported        string
	Q4SalesReported        string
	TotalSalesReported     string
	OMWBE                  string
	VeteranOwned           string
	SmallBusiness          string
	DiverseOptions         string
	Warnings               []string
	RawFields              map[string]any
}

// UpsertDataWAMasterContractSale inserts or updates one normalized DataWA
// statewide/master-contract sales row.
func (s *Store) UpsertDataWAMasterContractSale(ctx context.Context, p UpsertDataWAMasterContractSaleParams) error {
	warnings, err := json.Marshal(p.Warnings)
	if err != nil {
		return fmt.Errorf("marshal warnings: %w", err)
	}
	raw, err := json.Marshal(p.RawFields)
	if err != nil {
		return fmt.Errorf("marshal raw fields: %w", err)
	}
	const q = `
INSERT INTO datawa_master_contract_sale (
  source_dataset_id, source_row_id, customer_type, customer_name,
  normalized_customer_name, contract_number, contract_title, vendor_name,
  normalized_vendor_name, report_year,
  q1_sales_reported, q2_sales_reported, q3_sales_reported, q4_sales_reported,
  total_sales_reported, omwbe, veteran_owned, small_business, diverse_options,
  normalization_warnings, raw_fields
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10, 0),
  NULLIF($11,'')::numeric, NULLIF($12,'')::numeric, NULLIF($13,'')::numeric, NULLIF($14,'')::numeric,
  NULLIF($15,'')::numeric, $16,$17,$18,$19,$20::jsonb,$21::jsonb
)
ON CONFLICT (source_dataset_id, source_row_id) DO UPDATE SET
  customer_type = EXCLUDED.customer_type,
  customer_name = EXCLUDED.customer_name,
  normalized_customer_name = EXCLUDED.normalized_customer_name,
  contract_number = EXCLUDED.contract_number,
  contract_title = EXCLUDED.contract_title,
  vendor_name = EXCLUDED.vendor_name,
  normalized_vendor_name = EXCLUDED.normalized_vendor_name,
  report_year = EXCLUDED.report_year,
  q1_sales_reported = EXCLUDED.q1_sales_reported,
  q2_sales_reported = EXCLUDED.q2_sales_reported,
  q3_sales_reported = EXCLUDED.q3_sales_reported,
  q4_sales_reported = EXCLUDED.q4_sales_reported,
  total_sales_reported = EXCLUDED.total_sales_reported,
  omwbe = EXCLUDED.omwbe,
  veteran_owned = EXCLUDED.veteran_owned,
  small_business = EXCLUDED.small_business,
  diverse_options = EXCLUDED.diverse_options,
  normalization_warnings = EXCLUDED.normalization_warnings,
  raw_fields = EXCLUDED.raw_fields,
  updated_at = NOW();`
	_, err = s.Pool.Exec(ctx, q,
		p.SourceDatasetID, p.SourceRowID, strOrNull(p.CustomerType), strOrNull(p.CustomerName),
		strOrNull(defaultStr(p.NormalizedCustomerName, entitymatch.NormalizedName(p.CustomerName))), strOrNull(p.ContractNumber), strOrNull(p.ContractTitle), strOrNull(p.VendorName),
		strOrNull(defaultStr(p.NormalizedVendorName, entitymatch.NormalizedName(p.VendorName))), p.ReportYear,
		p.Q1SalesReported, p.Q2SalesReported, p.Q3SalesReported, p.Q4SalesReported, p.TotalSalesReported,
		strOrNull(p.OMWBE), strOrNull(p.VeteranOwned), strOrNull(p.SmallBusiness), strOrNull(p.DiverseOptions),
		string(warnings), string(raw),
	)
	if err != nil {
		return fmt.Errorf("upsert datawa_master_contract_sale: %w", err)
	}
	return nil
}

// UpsertDataWAITContractParams is the normalized row shape for
// datawa_it_contract.
type UpsertDataWAITContractParams struct {
	SourceDatasetID           string
	SourceRowID               string
	ReportFiscalYear          int
	AgencyNumberAgencyName    string
	AgencyNumber              string
	AgencyName                string
	ContractNumber            string
	ContractorName            string
	NormalizedContractorName  string
	ContractorDBA             string
	NormalizedContractorDBA   string
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
	Warnings                  []string
	RawFields                 map[string]any
}

// UpsertDataWAITContract inserts or updates one normalized DataWA IT contract
// report row.
func (s *Store) UpsertDataWAITContract(ctx context.Context, p UpsertDataWAITContractParams) error {
	warnings, err := json.Marshal(p.Warnings)
	if err != nil {
		return fmt.Errorf("marshal warnings: %w", err)
	}
	raw, err := json.Marshal(p.RawFields)
	if err != nil {
		return fmt.Errorf("marshal raw fields: %w", err)
	}
	const q = `
INSERT INTO datawa_it_contract (
  source_dataset_id, source_row_id, report_fiscal_year, agency_number_agency_name,
  agency_number, agency_name, contract_number, contractor_name,
  normalized_contractor_name, contractor_dba, normalized_contractor_dba,
  cooperative_purchase, cooperative_name, statewide_contract_purchase,
  contract_start_date, contract_end_date, fiscal_year_start, fiscal_year_end,
  it_tower_application, it_tower_compute, it_tower_data_center, it_tower_delivery,
  it_tower_end_user, it_tower_it_management, it_tower_network, it_tower_output,
  it_tower_platform, it_tower_security, it_tower_storage, other_non_it,
  total_percentage, contract_amount_fy20, contract_amount_fy21, contract_amount_fy22,
  contract_amount_fy23, contract_amount_fy24, contract_amount_fy25, contract_amount_fy26,
  contract_amount_fy27, contract_amount_fy28, contract_amount_fy29, contract_amount_fy30,
  total_contract_amount, contract_amount_explanation, normalization_warnings, raw_fields
) VALUES (
  $1,$2,NULLIF($3,0),$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,
  NULLIF($19,'')::numeric, NULLIF($20,'')::numeric, NULLIF($21,'')::numeric, NULLIF($22,'')::numeric,
  NULLIF($23,'')::numeric, NULLIF($24,'')::numeric, NULLIF($25,'')::numeric, NULLIF($26,'')::numeric,
  NULLIF($27,'')::numeric, NULLIF($28,'')::numeric, NULLIF($29,'')::numeric, NULLIF($30,'')::numeric,
  NULLIF($31,'')::numeric, NULLIF($32,'')::numeric, NULLIF($33,'')::numeric, NULLIF($34,'')::numeric,
  NULLIF($35,'')::numeric, NULLIF($36,'')::numeric, NULLIF($37,'')::numeric, NULLIF($38,'')::numeric,
  NULLIF($39,'')::numeric, NULLIF($40,'')::numeric, NULLIF($41,'')::numeric, NULLIF($42,'')::numeric,
  NULLIF($43,'')::numeric, $44, $45::jsonb, $46::jsonb
)
ON CONFLICT (source_dataset_id, source_row_id) DO UPDATE SET
  report_fiscal_year = EXCLUDED.report_fiscal_year,
  agency_number_agency_name = EXCLUDED.agency_number_agency_name,
  agency_number = EXCLUDED.agency_number,
  agency_name = EXCLUDED.agency_name,
  contract_number = EXCLUDED.contract_number,
  contractor_name = EXCLUDED.contractor_name,
  normalized_contractor_name = EXCLUDED.normalized_contractor_name,
  contractor_dba = EXCLUDED.contractor_dba,
  normalized_contractor_dba = EXCLUDED.normalized_contractor_dba,
  cooperative_purchase = EXCLUDED.cooperative_purchase,
  cooperative_name = EXCLUDED.cooperative_name,
  statewide_contract_purchase = EXCLUDED.statewide_contract_purchase,
  contract_start_date = EXCLUDED.contract_start_date,
  contract_end_date = EXCLUDED.contract_end_date,
  fiscal_year_start = EXCLUDED.fiscal_year_start,
  fiscal_year_end = EXCLUDED.fiscal_year_end,
  it_tower_application = EXCLUDED.it_tower_application,
  it_tower_compute = EXCLUDED.it_tower_compute,
  it_tower_data_center = EXCLUDED.it_tower_data_center,
  it_tower_delivery = EXCLUDED.it_tower_delivery,
  it_tower_end_user = EXCLUDED.it_tower_end_user,
  it_tower_it_management = EXCLUDED.it_tower_it_management,
  it_tower_network = EXCLUDED.it_tower_network,
  it_tower_output = EXCLUDED.it_tower_output,
  it_tower_platform = EXCLUDED.it_tower_platform,
  it_tower_security = EXCLUDED.it_tower_security,
  it_tower_storage = EXCLUDED.it_tower_storage,
  other_non_it = EXCLUDED.other_non_it,
  total_percentage = EXCLUDED.total_percentage,
  contract_amount_fy20 = EXCLUDED.contract_amount_fy20,
  contract_amount_fy21 = EXCLUDED.contract_amount_fy21,
  contract_amount_fy22 = EXCLUDED.contract_amount_fy22,
  contract_amount_fy23 = EXCLUDED.contract_amount_fy23,
  contract_amount_fy24 = EXCLUDED.contract_amount_fy24,
  contract_amount_fy25 = EXCLUDED.contract_amount_fy25,
  contract_amount_fy26 = EXCLUDED.contract_amount_fy26,
  contract_amount_fy27 = EXCLUDED.contract_amount_fy27,
  contract_amount_fy28 = EXCLUDED.contract_amount_fy28,
  contract_amount_fy29 = EXCLUDED.contract_amount_fy29,
  contract_amount_fy30 = EXCLUDED.contract_amount_fy30,
  total_contract_amount = EXCLUDED.total_contract_amount,
  contract_amount_explanation = EXCLUDED.contract_amount_explanation,
  normalization_warnings = EXCLUDED.normalization_warnings,
  raw_fields = EXCLUDED.raw_fields,
  updated_at = NOW();`
	_, err = s.Pool.Exec(ctx, q,
		p.SourceDatasetID, p.SourceRowID, p.ReportFiscalYear, strOrNull(p.AgencyNumberAgencyName),
		strOrNull(p.AgencyNumber), strOrNull(p.AgencyName), strOrNull(p.ContractNumber), strOrNull(p.ContractorName),
		strOrNull(defaultStr(p.NormalizedContractorName, entitymatch.NormalizedName(p.ContractorName))), strOrNull(p.ContractorDBA),
		strOrNull(defaultStr(p.NormalizedContractorDBA, entitymatch.NormalizedName(p.ContractorDBA))), boolPtrOrNull(p.CooperativePurchase),
		strOrNull(p.CooperativeName), boolPtrOrNull(p.StatewideContractPurchase),
		datePtrOrNull(p.ContractStartDate), datePtrOrNull(p.ContractEndDate), strOrNull(p.FiscalYearStart), strOrNull(p.FiscalYearEnd),
		p.ITTowerApplication, p.ITTowerCompute, p.ITTowerDataCenter, p.ITTowerDelivery, p.ITTowerEndUser, p.ITTowerITManagement,
		p.ITTowerNetwork, p.ITTowerOutput, p.ITTowerPlatform, p.ITTowerSecurity, p.ITTowerStorage, p.OtherNonIT, p.TotalPercentage,
		p.ContractAmountFY20, p.ContractAmountFY21, p.ContractAmountFY22, p.ContractAmountFY23, p.ContractAmountFY24, p.ContractAmountFY25,
		p.ContractAmountFY26, p.ContractAmountFY27, p.ContractAmountFY28, p.ContractAmountFY29, p.ContractAmountFY30, p.TotalContractAmount,
		strOrNull(p.ContractAmountExplanation), string(warnings), string(raw),
	)
	if err != nil {
		return fmt.Errorf("upsert datawa_it_contract: %w", err)
	}
	return nil
}

// UpsertDataWAWEBSVendorParams is the normalized row shape for datawa_webs_vendor.
type UpsertDataWAWEBSVendorParams struct {
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
	Warnings              []string
	RawFields             map[string]any
}

// UpsertDataWAWEBSVendor inserts or updates one normalized WEBS vendor row.
func (s *Store) UpsertDataWAWEBSVendor(ctx context.Context, p UpsertDataWAWEBSVendorParams) error {
	warnings, err := json.Marshal(p.Warnings)
	if err != nil {
		return fmt.Errorf("marshal warnings: %w", err)
	}
	raw, err := json.Marshal(p.RawFields)
	if err != nil {
		return fmt.Errorf("marshal raw fields: %w", err)
	}
	const q = `
INSERT INTO datawa_webs_vendor (
  source_dataset_id, source_row_id, company_name, normalized_company_name,
  dba_name, phone_number, contact_email, city, state, web_address,
  commodity_code, description_of_work, small_business, veteran_owned,
  other_cert, other_cert_2, normalization_warnings, raw_fields
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17::jsonb,$18::jsonb
)
ON CONFLICT (source_dataset_id, source_row_id) DO UPDATE SET
  company_name = EXCLUDED.company_name,
  normalized_company_name = EXCLUDED.normalized_company_name,
  dba_name = EXCLUDED.dba_name,
  phone_number = EXCLUDED.phone_number,
  contact_email = EXCLUDED.contact_email,
  city = EXCLUDED.city,
  state = EXCLUDED.state,
  web_address = EXCLUDED.web_address,
  commodity_code = EXCLUDED.commodity_code,
  description_of_work = EXCLUDED.description_of_work,
  small_business = EXCLUDED.small_business,
  veteran_owned = EXCLUDED.veteran_owned,
  other_cert = EXCLUDED.other_cert,
  other_cert_2 = EXCLUDED.other_cert_2,
  normalization_warnings = EXCLUDED.normalization_warnings,
  raw_fields = EXCLUDED.raw_fields,
  updated_at = NOW();`
	_, err = s.Pool.Exec(ctx, q,
		p.SourceDatasetID, p.SourceRowID, strOrNull(p.CompanyName), strOrNull(p.NormalizedCompanyName),
		strOrNull(p.DBAName), strOrNull(p.PhoneNumber), strOrNull(p.ContactEmail), strOrNull(p.City), strOrNull(p.State), strOrNull(p.WebAddress),
		strOrNull(p.CommodityCode), strOrNull(p.DescriptionOfWork), strOrNull(p.SmallBusiness), strOrNull(p.VeteranOwned),
		strOrNull(p.OtherCert), strOrNull(p.OtherCert2), string(warnings), string(raw),
	)
	if err != nil {
		return fmt.Errorf("upsert datawa_webs_vendor: %w", err)
	}
	return nil
}

// Vendor/entity match candidates and decisions.
