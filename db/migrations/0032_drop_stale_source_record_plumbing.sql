-- +goose Up
-- +goose StatementBegin

-- Migration 0028 now removes fetch-ledger provenance plumbing, but databases
-- that applied an older 0028 need a forward-only cleanup migration. Keep this
-- idempotent so fresh databases that already got the 0028 cleanup are no-ops.

DROP INDEX IF EXISTS idx_organization_source_mention_kind;
DROP INDEX IF EXISTS idx_organization_source_mention_normalized;
DROP INDEX IF EXISTS idx_organization_source_mention_org;
DROP TABLE IF EXISTS organization_source_mention;

DROP VIEW IF EXISTS reviewed_vendor_entity_match;

ALTER TABLE IF EXISTS agenda_item DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS bill DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS bill_status_change DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS hearing DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS testifier DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS tvw_event DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS tvw_media_asset DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS legislator_roster_membership DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS person_source_mention DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS person_organization_affiliation DROP COLUMN IF EXISTS source_record_id;

ALTER TABLE IF EXISTS datawa_contract DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS datawa_master_contract_sale DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS datawa_it_contract DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS datawa_webs_vendor DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS fiscalwa_vendor_payment DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS federal_award DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS seattle_operating_budget DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS vendor_entity_match_candidate DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS entity_match_candidate DROP COLUMN IF EXISTS source_record_id;

ALTER TABLE IF EXISTS irs_bmf_organization DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS pdc_employer DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS source_dataset DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS org_context_record DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS bls_observation DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS census_observation DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS epa_ejscreen_observation DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS epa_facility DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS fema_disaster_declaration DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS hud_dataset_ref DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS hud_housing_observation DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS kingcounty_parcel DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS sao_audit_type DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS sao_government_entity DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS sao_government_type DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS sao_report DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS seattle_auditor_recommendation DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS seattle_permit DROP COLUMN IF EXISTS source_record_id;

DROP TABLE IF EXISTS source_record CASCADE;

CREATE VIEW reviewed_vendor_entity_match AS
SELECT c.id AS candidate_id,
       d.id AS decision_id,
       c.source_kind,
       c.source_table,
       c.source_pk,
       c.source_dataset_id,
       c.source_row_id,
       c.source_name,
       c.normalized_name,
       d.organization_id,
       d.reviewed_confidence AS match_confidence,
       d.review_notes,
       d.reviewed_at,
       c.evidence
  FROM vendor_entity_match_candidate c
  JOIN vendor_entity_match_decision d ON d.candidate_id = c.id
 WHERE d.decision = 'confirmed';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Intentionally irreversible. Reconstructing the old fetch ledger would require
-- backfilling rows and foreign keys that the application no longer maintains.

-- +goose StatementEnd
