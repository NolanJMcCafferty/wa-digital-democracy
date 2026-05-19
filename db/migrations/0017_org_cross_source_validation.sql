-- +goose Up
-- +goose StatementBegin

-- Allow source_record rows tagged with the new irs_bmf system.
ALTER TABLE source_record DROP CONSTRAINT IF EXISTS source_record_source_system_check;
ALTER TABLE source_record ADD CONSTRAINT source_record_source_system_check CHECK (source_system IN (
    'lws', 'committee_schedules', 'csi', 'tvw', 'invintus', 'pdc_socrata',
    'datawa_socrata', 'seattle_socrata', 'kingcounty_socrata',
    'sao_reportsearch', 'seattle_auditor', 'census', 'usaspending',
    'openfema', 'bls', 'hud', 'epa', 'fiscal_wa', 'irs_bmf'
));

-- Cross-source validation tables for organization seeding.
-- Adds two authoritative source tables (IRS BMF for 501(c) orgs in WA, PDC
-- lobbyist employers from xhn7-64im) and lets organization rows reference them
-- so we can promote a CSI-only seed to "verified" only when it matches an
-- external registry. Mirrors the design discussed: every distinct CSI string is
-- not an organization on its own.

CREATE TABLE irs_bmf_organization (
    ein                 TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    normalized_name     TEXT NOT NULL,
    sort_name           TEXT,
    street              TEXT,
    city                TEXT,
    state               TEXT,
    zip                 TEXT,
    subsection_code     TEXT,        -- 501(c)(N) subsection
    classification      TEXT,
    deductibility_code  TEXT,
    activity_codes      TEXT,
    foundation_code     TEXT,
    organization_code   TEXT,
    status_code         TEXT,        -- e.g. 01 active, 26 revoked
    ruling_date         TEXT,        -- raw YYYYMM
    ntee_code           TEXT,
    income_amount       BIGINT,
    revenue_amount      BIGINT,
    asset_amount        BIGINT,
    raw                 JSONB NOT NULL,
    source_record_id    BIGINT REFERENCES source_record(id),
    fetched_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_irs_bmf_normalized_name ON irs_bmf_organization (normalized_name);
CREATE INDEX idx_irs_bmf_name_trgm ON irs_bmf_organization USING gin (name gin_trgm_ops);

CREATE TABLE pdc_employer (
    employer_id         TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    normalized_name     TEXT NOT NULL,
    last_employment_year TEXT,
    last_report_number   TEXT,
    last_employment_url  TEXT,
    raw                 JSONB NOT NULL,
    source_record_id    BIGINT REFERENCES source_record(id),
    first_seen_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_pdc_employer_normalized_name ON pdc_employer (normalized_name);
CREATE INDEX idx_pdc_employer_name_trgm ON pdc_employer USING gin (name gin_trgm_ops);

-- Add verification metadata to organization. organization.pdc_lobbyist_employer_id
-- already exists from 0001_initial.sql; we add the IRS EIN and a "verified"
-- timestamp so the UI can hide unverified seeds without losing the data.
ALTER TABLE organization
    ADD COLUMN irs_bmf_ein TEXT REFERENCES irs_bmf_organization(ein) ON DELETE SET NULL,
    ADD COLUMN verified_at TIMESTAMPTZ,
    ADD COLUMN verification_source TEXT CHECK (verification_source IN ('irs_bmf','pdc_employer','manual'));

-- Backfill the FK from any existing pdc_lobbyist_employer_id matches will be
-- handled by the populate command, not in-migration (the source table is empty
-- right now).

CREATE INDEX idx_organization_irs_bmf_ein ON organization (irs_bmf_ein) WHERE irs_bmf_ein IS NOT NULL;
CREATE INDEX idx_organization_verified ON organization (verified_at) WHERE verified_at IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_organization_verified;
DROP INDEX IF EXISTS idx_organization_irs_bmf_ein;

ALTER TABLE organization
    DROP COLUMN IF EXISTS verification_source,
    DROP COLUMN IF EXISTS verified_at,
    DROP COLUMN IF EXISTS irs_bmf_ein;

DROP INDEX IF EXISTS idx_pdc_employer_name_trgm;
DROP INDEX IF EXISTS idx_pdc_employer_normalized_name;
DROP TABLE IF EXISTS pdc_employer;

DROP INDEX IF EXISTS idx_irs_bmf_name_trgm;
DROP INDEX IF EXISTS idx_irs_bmf_normalized_name;
DROP TABLE IF EXISTS irs_bmf_organization;

ALTER TABLE source_record DROP CONSTRAINT IF EXISTS source_record_source_system_check;
ALTER TABLE source_record ADD CONSTRAINT source_record_source_system_check CHECK (source_system IN (
    'lws', 'committee_schedules', 'csi', 'tvw', 'invintus', 'pdc_socrata',
    'datawa_socrata', 'seattle_socrata', 'kingcounty_socrata',
    'sao_reportsearch', 'seattle_auditor', 'census', 'usaspending',
    'openfema', 'bls', 'hud', 'epa', 'fiscal_wa'
));

-- +goose StatementEnd
