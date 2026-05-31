package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// UpsertFederalAwardParams is the normalized row shape for federal_award.
type UpsertFederalAwardParams struct {
	AwardID        string
	RecipientName  string
	RecipientUEI   string
	AwardingAgency string
	FundingAgency  string
	AwardType      string
	AwardAmount    string
	StartDate      *time.Time
	EndDate        *time.Time
	PlaceStateCode string
	PlaceCounty    string
	RawFields      map[string]any
	SourceRecordID int64
}

// UpsertFederalAward inserts or updates one USAspending award row.
func (s *Store) UpsertFederalAward(ctx context.Context, p UpsertFederalAwardParams) error {
	raw, err := json.Marshal(p.RawFields)
	if err != nil {
		return fmt.Errorf("marshal raw fields: %w", err)
	}
	const q = `
INSERT INTO federal_award (
  award_id, recipient_name, recipient_uei, awarding_agency, funding_agency,
  award_type, award_amount, start_date, end_date, place_state_code,
  normalized_recipient_name,
  place_county, raw_fields, source_record_id
) VALUES (
  $1,$2,$3,$4,$5,$6,NULLIF($7,'')::numeric,$8,$9,$10,wa_dd_normalize_entity_name($2),$11,$12::jsonb,$13
)
ON CONFLICT (award_id) DO UPDATE SET
  recipient_name = EXCLUDED.recipient_name,
  recipient_uei = EXCLUDED.recipient_uei,
  awarding_agency = EXCLUDED.awarding_agency,
  funding_agency = EXCLUDED.funding_agency,
  award_type = EXCLUDED.award_type,
  award_amount = EXCLUDED.award_amount,
  start_date = EXCLUDED.start_date,
  end_date = EXCLUDED.end_date,
  place_state_code = EXCLUDED.place_state_code,
  normalized_recipient_name = EXCLUDED.normalized_recipient_name,
  place_county = EXCLUDED.place_county,
  raw_fields = EXCLUDED.raw_fields,
  source_record_id = EXCLUDED.source_record_id,
  updated_at = NOW();`
	_, err = s.Pool.Exec(ctx, q,
		p.AwardID, strOrNull(p.RecipientName), strOrNull(p.RecipientUEI), strOrNull(p.AwardingAgency), strOrNull(p.FundingAgency),
		strOrNull(p.AwardType), p.AwardAmount, datePtrOrNull(p.StartDate), datePtrOrNull(p.EndDate), strOrNull(p.PlaceStateCode),
		strOrNull(p.PlaceCounty), string(raw), p.SourceRecordID,
	)
	if err != nil {
		return fmt.Errorf("upsert federal_award: %w", err)
	}
	return nil
}

// UpsertSeattleOperatingBudgetParams is the normalized row shape for
// seattle_operating_budget.
type UpsertSeattleOperatingBudgetParams struct {
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
	RawFields       map[string]any
	SourceRecordID  int64
}

// UpsertSeattleOperatingBudget inserts or updates one Seattle operating budget row.
func (s *Store) UpsertSeattleOperatingBudget(ctx context.Context, p UpsertSeattleOperatingBudgetParams) error {
	raw, err := json.Marshal(p.RawFields)
	if err != nil {
		return fmt.Errorf("marshal raw fields: %w", err)
	}
	const q = `
INSERT INTO seattle_operating_budget (
  source_dataset_id, source_row_id, fiscal_year, service, department, program,
  fund, fund_type, expense_type, description, approved_amount, raw_fields,
  source_record_id
) VALUES (
  $1,$2,NULLIF($3,0),$4,$5,$6,$7,$8,$9,$10,NULLIF($11,'')::numeric,$12::jsonb,$13
)
ON CONFLICT (source_dataset_id, source_row_id) DO UPDATE SET
  fiscal_year = EXCLUDED.fiscal_year,
  service = EXCLUDED.service,
  department = EXCLUDED.department,
  program = EXCLUDED.program,
  fund = EXCLUDED.fund,
  fund_type = EXCLUDED.fund_type,
  expense_type = EXCLUDED.expense_type,
  description = EXCLUDED.description,
  approved_amount = EXCLUDED.approved_amount,
  raw_fields = EXCLUDED.raw_fields,
  source_record_id = EXCLUDED.source_record_id,
  updated_at = NOW();`
	_, err = s.Pool.Exec(ctx, q,
		p.SourceDatasetID, p.SourceRowID, p.FiscalYear, strOrNull(p.Service), strOrNull(p.Department), strOrNull(p.Program),
		strOrNull(p.Fund), strOrNull(p.FundType), strOrNull(p.ExpenseType), strOrNull(p.Description), p.ApprovedAmount,
		string(raw), p.SourceRecordID,
	)
	if err != nil {
		return fmt.Errorf("upsert seattle_operating_budget: %w", err)
	}
	return nil
}

// UpsertFiscalWAVendorPaymentParams is the normalized row shape for
// fiscalwa_vendor_payment.
type UpsertFiscalWAVendorPaymentParams struct {
	SourceDatasetID string
	SourceRowID     string
	Biennium        string
	FiscalYear      int
	FiscalMonth     string
	AgencyNumber    string
	AgencyName      string
	ObjectCode      string
	ObjectCategory  string
	SubobjectCode   string
	SubobjectName   string
	VendorName      string
	Amount          string
	RawFields       map[string]any
	SourceRecordID  int64
}

// UpsertFiscalWAVendorPayment inserts or updates one fiscal.wa.gov vendor
// payment row from the Open Checkbook workbook.
func (s *Store) UpsertFiscalWAVendorPayment(ctx context.Context, p UpsertFiscalWAVendorPaymentParams) error {
	raw, err := json.Marshal(p.RawFields)
	if err != nil {
		return fmt.Errorf("marshal raw fields: %w", err)
	}
	const q = `
INSERT INTO fiscalwa_vendor_payment (
  source_dataset_id, source_row_id, biennium, fiscal_year, fiscal_month,
  agency_number, agency_name, object_code, object_category, subobject_code,
  subobject_name, vendor_name, normalized_vendor_name, amount, raw_fields, source_record_id
) VALUES (
  $1,$2,$3,NULLIF($4,0),$5,$6,$7,$8,$9,$10,$11,$12,wa_dd_normalize_entity_name($12),NULLIF($13,'')::numeric,$14::jsonb,$15
)
ON CONFLICT (source_dataset_id, source_row_id) DO UPDATE SET
  biennium = EXCLUDED.biennium,
  fiscal_year = EXCLUDED.fiscal_year,
  fiscal_month = EXCLUDED.fiscal_month,
  agency_number = EXCLUDED.agency_number,
  agency_name = EXCLUDED.agency_name,
  object_code = EXCLUDED.object_code,
  object_category = EXCLUDED.object_category,
  subobject_code = EXCLUDED.subobject_code,
  subobject_name = EXCLUDED.subobject_name,
  vendor_name = EXCLUDED.vendor_name,
  normalized_vendor_name = EXCLUDED.normalized_vendor_name,
  amount = EXCLUDED.amount,
  raw_fields = EXCLUDED.raw_fields,
  source_record_id = EXCLUDED.source_record_id,
  updated_at = NOW();`
	_, err = s.Pool.Exec(ctx, q,
		p.SourceDatasetID, p.SourceRowID, strOrNull(p.Biennium), p.FiscalYear, strOrNull(p.FiscalMonth),
		strOrNull(p.AgencyNumber), strOrNull(p.AgencyName), strOrNull(p.ObjectCode), strOrNull(p.ObjectCategory),
		strOrNull(p.SubobjectCode), strOrNull(p.SubobjectName), strOrNull(p.VendorName), p.Amount, string(raw), p.SourceRecordID,
	)
	if err != nil {
		return fmt.Errorf("upsert fiscalwa_vendor_payment: %w", err)
	}
	return nil
}
