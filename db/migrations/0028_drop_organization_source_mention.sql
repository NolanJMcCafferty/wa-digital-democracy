-- +goose Up
-- +goose StatementBegin

-- Retire organization_source_mention. Only the CSI testifier path ever wrote
-- to it, and that information is fully recoverable from testifier
-- (raw_organization, normalized_org_id). Cross-source org context lives in
-- reviewed_vendor_entity_match plus organization source-specific ID columns;
-- nothing reads from organization_source_mention.

DROP INDEX IF EXISTS idx_organization_source_mention_kind;
DROP INDEX IF EXISTS idx_organization_source_mention_normalized;
DROP INDEX IF EXISTS idx_organization_source_mention_org;
DROP TABLE IF EXISTS organization_source_mention;

-- Remove fetch-ledger provenance plumbing. Domain rows keep official URLs,
-- stable source IDs, raw_fields JSON, and refreshed timestamps where those are
-- product-relevant; the global source_record ledger and raw-object pointers are
-- not used by the application.
ALTER TABLE IF EXISTS bill DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS bill_status_change DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS hearing DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS agenda_item DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS testifier DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS tvw_event DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS tvw_media_asset DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS person_source_mention DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS person_organization_affiliation DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS legislator_roster_membership DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS datawa_contract DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS datawa_master_contract_sale DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS datawa_it_contract DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS datawa_webs_vendor DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS fiscalwa_vendor_payment DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS federal_award DROP COLUMN IF EXISTS source_record_id;
ALTER TABLE IF EXISTS seattle_operating_budget DROP COLUMN IF EXISTS source_record_id;
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

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

CREATE TABLE source_record (
    id              BIGSERIAL PRIMARY KEY,
    source_system   TEXT NOT NULL,
    source_endpoint TEXT NOT NULL,
    source_url      TEXT NOT NULL,
    source_id       TEXT,
    fetched_at      TIMESTAMPTZ NOT NULL,
    content_hash    TEXT NOT NULL,
    raw_path        TEXT NOT NULL,
    content_type    TEXT,
    transform_version TEXT NOT NULL DEFAULT 'v0'
);
CREATE INDEX idx_source_record_system_endpoint ON source_record (source_system, source_endpoint);
CREATE INDEX idx_source_record_source_id      ON source_record (source_system, source_id);
CREATE INDEX idx_source_record_fetched_at     ON source_record (fetched_at DESC);
CREATE UNIQUE INDEX uniq_source_record_request_hash
    ON source_record (source_system, source_endpoint, source_url, content_hash, transform_version);

CREATE TABLE organization_source_mention (
    id                 BIGSERIAL PRIMARY KEY,
    source_kind        TEXT NOT NULL CHECK (source_kind IN (
        'csi_testifier', 'pdc_lobbying_employer', 'deepgram_entity',
        'datawa_vendor', 'federal_recipient', 'manual_import'
    )),
    source_table       TEXT NOT NULL,
    source_pk          BIGINT,
    source_name        TEXT NOT NULL,
    normalized_name    TEXT NOT NULL,
    organization_id    BIGINT REFERENCES organization(id) ON DELETE SET NULL,
    occurrence_count   INT NOT NULL DEFAULT 1,
    confidence         org_match_confidence NOT NULL DEFAULT 'possible',
    source_record_id   BIGINT REFERENCES source_record(id),
    first_seen_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_kind, source_table, source_pk, source_name)
);
CREATE INDEX idx_organization_source_mention_org ON organization_source_mention (organization_id);
CREATE INDEX idx_organization_source_mention_normalized ON organization_source_mention (normalized_name);
CREATE INDEX idx_organization_source_mention_kind ON organization_source_mention (source_kind);

-- Down migration intentionally does not re-add source_record_id columns to every
-- historical table: the removed columns were provenance plumbing, not domain data,
-- and restoring them without backfilled source_record rows would be lossy anyway.

-- +goose StatementEnd
