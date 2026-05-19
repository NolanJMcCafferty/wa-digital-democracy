-- +goose Up
-- +goose StatementBegin

-- Reviewable procurement/entity-resolution layer. These tables store match
-- candidates and explicit review decisions without overwriting raw DataWA,
-- WEBS, PDC, or organization names.

CREATE OR REPLACE FUNCTION wa_dd_normalize_entity_name(raw TEXT)
RETURNS TEXT
LANGUAGE sql
IMMUTABLE
AS $$
    SELECT NULLIF(
        array_to_string(
            ARRAY(
                SELECT token
                  FROM unnest(regexp_split_to_array(upper(regexp_replace(coalesce(raw, ''), '&', ' AND ', 'g')), '[^A-Z0-9]+')) token
                 WHERE token <> ''
                   AND token NOT IN (
                       'THE','A','AN','INC','INCORPORATED','LLC','L','LTD','LIMITED',
                       'CORP','CORPORATION','CO','COMPANY','PLC','PC','PLLC','LP','LLP',
                       'ASSN','ASSOCIATION'
                   )
            ),
            ' '
        ),
        ''
    );
$$;

ALTER TABLE datawa_contract
    ADD COLUMN IF NOT EXISTS normalized_contractor_name TEXT;
UPDATE datawa_contract
   SET normalized_contractor_name = wa_dd_normalize_entity_name(contractor_name)
 WHERE normalized_contractor_name IS NULL AND contractor_name IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_datawa_contract_normalized_contractor
    ON datawa_contract (normalized_contractor_name)
 WHERE normalized_contractor_name IS NOT NULL;

ALTER TABLE datawa_master_contract_sale
    ADD COLUMN IF NOT EXISTS normalized_vendor_name TEXT,
    ADD COLUMN IF NOT EXISTS normalized_customer_name TEXT;
UPDATE datawa_master_contract_sale
   SET normalized_vendor_name = wa_dd_normalize_entity_name(vendor_name)
 WHERE normalized_vendor_name IS NULL AND vendor_name IS NOT NULL;
UPDATE datawa_master_contract_sale
   SET normalized_customer_name = wa_dd_normalize_entity_name(customer_name)
 WHERE normalized_customer_name IS NULL AND customer_name IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_datawa_master_sale_normalized_vendor
    ON datawa_master_contract_sale (normalized_vendor_name)
 WHERE normalized_vendor_name IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_datawa_master_sale_normalized_customer
    ON datawa_master_contract_sale (normalized_customer_name)
 WHERE normalized_customer_name IS NOT NULL;

ALTER TABLE datawa_it_contract
    ADD COLUMN IF NOT EXISTS normalized_contractor_name TEXT,
    ADD COLUMN IF NOT EXISTS normalized_contractor_dba TEXT;
UPDATE datawa_it_contract
   SET normalized_contractor_name = wa_dd_normalize_entity_name(contractor_name)
 WHERE normalized_contractor_name IS NULL AND contractor_name IS NOT NULL;
UPDATE datawa_it_contract
   SET normalized_contractor_dba = wa_dd_normalize_entity_name(contractor_dba)
 WHERE normalized_contractor_dba IS NULL AND contractor_dba IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_datawa_it_contract_normalized_contractor
    ON datawa_it_contract (normalized_contractor_name)
 WHERE normalized_contractor_name IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_datawa_it_contract_normalized_dba
    ON datawa_it_contract (normalized_contractor_dba)
 WHERE normalized_contractor_dba IS NOT NULL;

CREATE TYPE entity_match_source_kind AS ENUM (
    'datawa_contract_contractor',
    'datawa_master_contract_vendor',
    'datawa_master_contract_customer',
    'datawa_it_contract_contractor',
    'datawa_it_contract_dba',
    'datawa_webs_vendor',
    'pdc_lobbying_organization',
    'organization_alias'
);

CREATE TYPE entity_match_decision AS ENUM (
    'confirmed',
    'rejected',
    'needs_review'
);

CREATE TABLE vendor_entity_match_candidate (
    id                      BIGSERIAL PRIMARY KEY,
    source_kind              entity_match_source_kind NOT NULL,
    source_table             TEXT NOT NULL,
    source_pk                BIGINT,
    source_dataset_id        TEXT,
    source_row_id            TEXT,
    source_name              TEXT NOT NULL,
    normalized_name          TEXT NOT NULL,
    organization_id          BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    candidate_confidence     org_match_confidence NOT NULL DEFAULT 'possible',
    evidence                 JSONB NOT NULL DEFAULT '[]',
    source_record_id         BIGINT REFERENCES source_record(id),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_kind, source_dataset_id, source_row_id, source_name, organization_id)
);
CREATE INDEX idx_vendor_entity_match_candidate_org
    ON vendor_entity_match_candidate (organization_id, candidate_confidence);
CREATE INDEX idx_vendor_entity_match_candidate_normalized
    ON vendor_entity_match_candidate (normalized_name);
CREATE INDEX idx_vendor_entity_match_candidate_source
    ON vendor_entity_match_candidate (source_kind, source_table, source_pk);

CREATE TABLE vendor_entity_match_decision (
    id                      BIGSERIAL PRIMARY KEY,
    candidate_id             BIGINT NOT NULL REFERENCES vendor_entity_match_candidate(id) ON DELETE CASCADE,
    organization_id          BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    decision                 entity_match_decision NOT NULL DEFAULT 'needs_review',
    reviewed_confidence      org_match_confidence NOT NULL DEFAULT 'possible',
    reviewed_by              TEXT,
    review_notes             TEXT,
    reviewed_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (candidate_id)
);
CREATE INDEX idx_vendor_entity_match_decision_org
    ON vendor_entity_match_decision (organization_id, decision, reviewed_confidence);

-- Public/API consumers should read this view rather than treating all generated
-- candidates as authoritative. Only explicit confirmed decisions are surfaced
-- here as reviewed links.
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
       c.evidence,
       c.source_record_id
  FROM vendor_entity_match_candidate c
  JOIN vendor_entity_match_decision d ON d.candidate_id = c.id
 WHERE d.decision = 'confirmed';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP VIEW IF EXISTS reviewed_vendor_entity_match;
DROP TABLE IF EXISTS vendor_entity_match_decision;
DROP TABLE IF EXISTS vendor_entity_match_candidate;
DROP TYPE IF EXISTS entity_match_decision;
DROP TYPE IF EXISTS entity_match_source_kind;

DROP INDEX IF EXISTS idx_datawa_it_contract_normalized_dba;
DROP INDEX IF EXISTS idx_datawa_it_contract_normalized_contractor;
ALTER TABLE datawa_it_contract
    DROP COLUMN IF EXISTS normalized_contractor_dba,
    DROP COLUMN IF EXISTS normalized_contractor_name;

DROP INDEX IF EXISTS idx_datawa_master_sale_normalized_customer;
DROP INDEX IF EXISTS idx_datawa_master_sale_normalized_vendor;
ALTER TABLE datawa_master_contract_sale
    DROP COLUMN IF EXISTS normalized_customer_name,
    DROP COLUMN IF EXISTS normalized_vendor_name;

DROP INDEX IF EXISTS idx_datawa_contract_normalized_contractor;
ALTER TABLE datawa_contract
    DROP COLUMN IF EXISTS normalized_contractor_name;

DROP FUNCTION IF EXISTS wa_dd_normalize_entity_name(TEXT);

-- +goose StatementEnd
