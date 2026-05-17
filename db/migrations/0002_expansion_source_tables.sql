-- +goose Up
-- +goose StatementBegin

-- Expansion-source schema for WA Digital Democracy. These tables are scoped to
-- the real clients under internal/sources/{datawa,seattle,kingcounty,sao,
-- seattleauditor,census,usaspending,fema,bls,hud,epa}. Keep every normalized
-- record source-linked through source_record_id.

ALTER TABLE source_record DROP CONSTRAINT IF EXISTS source_record_source_system_check;
ALTER TABLE source_record ADD CONSTRAINT source_record_source_system_check CHECK (source_system IN (
    'lws', 'committee_schedules', 'csi', 'tvw', 'invintus', 'pdc_socrata',
    'datawa_socrata', 'seattle_socrata', 'kingcounty_socrata',
    'sao_reportsearch', 'seattle_auditor', 'census', 'usaspending',
    'openfema', 'bls', 'hud', 'epa'
));

CREATE TABLE source_dataset (
    id                  BIGSERIAL PRIMARY KEY,
    source_system       TEXT NOT NULL,
    dataset_id          TEXT NOT NULL,
    name                TEXT,
    description         TEXT,
    domain              TEXT,
    web_url             TEXT,
    data_url            TEXT,
    metadata_json       JSONB NOT NULL DEFAULT '{}',
    source_record_id    BIGINT REFERENCES source_record(id),
    first_seen_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_system, dataset_id)
);
CREATE INDEX idx_source_dataset_system ON source_dataset (source_system);
CREATE INDEX idx_source_dataset_name_trgm ON source_dataset USING gin (name gin_trgm_ops);

CREATE TABLE datawa_contract (
    id                      BIGSERIAL PRIMARY KEY,
    source_dataset_id        TEXT NOT NULL,
    source_row_id            TEXT NOT NULL,
    fiscal_year              INT,
    agency_name              TEXT,
    agency_number            TEXT,
    contract_number          TEXT,
    amendment_number         TEXT,
    contractor_name          TEXT,
    statewide_vendor_number  TEXT,
    description              TEXT,
    start_date               DATE,
    end_date                 DATE,
    period_start             DATE,
    period_end               DATE,
    federal_amount           NUMERIC,
    state_amount             NUMERIC,
    other_amount             NUMERIC,
    total_amount             NUMERIC,
    procurement_type         TEXT,
    minority_woman_owned     TEXT,
    small_business           TEXT,
    veteran_owned            TEXT,
    normalization_warnings   JSONB NOT NULL DEFAULT '[]',
    raw_fields               JSONB NOT NULL DEFAULT '{}',
    source_record_id         BIGINT NOT NULL REFERENCES source_record(id),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_dataset_id, source_row_id)
);
CREATE INDEX idx_datawa_contract_agency ON datawa_contract USING gin (agency_name gin_trgm_ops);
CREATE INDEX idx_datawa_contract_contractor ON datawa_contract USING gin (contractor_name gin_trgm_ops);
CREATE INDEX idx_datawa_contract_fy ON datawa_contract (fiscal_year);

CREATE TABLE seattle_permit (
    id                  BIGSERIAL PRIMARY KEY,
    source_dataset_id    TEXT NOT NULL,
    source_row_id        TEXT NOT NULL,
    permit_number        TEXT,
    status               TEXT,
    address              TEXT,
    description          TEXT,
    category             TEXT,
    permit_type          TEXT,
    valuation            NUMERIC,
    latitude             DOUBLE PRECISION,
    longitude            DOUBLE PRECISION,
    raw_fields           JSONB NOT NULL DEFAULT '{}',
    source_record_id     BIGINT NOT NULL REFERENCES source_record(id),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_dataset_id, source_row_id)
);
CREATE INDEX idx_seattle_permit_number ON seattle_permit (permit_number) WHERE permit_number IS NOT NULL;
CREATE INDEX idx_seattle_permit_address_trgm ON seattle_permit USING gin (address gin_trgm_ops);
CREATE INDEX idx_seattle_permit_status ON seattle_permit (status);

CREATE TABLE kingcounty_parcel (
    id                  BIGSERIAL PRIMARY KEY,
    source_dataset_id    TEXT NOT NULL,
    source_row_id        TEXT NOT NULL,
    pin                  TEXT,
    address              TEXT,
    jurisdiction         TEXT,
    latitude             DOUBLE PRECISION,
    longitude            DOUBLE PRECISION,
    raw_fields           JSONB NOT NULL DEFAULT '{}',
    source_record_id     BIGINT NOT NULL REFERENCES source_record(id),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_dataset_id, source_row_id)
);
CREATE INDEX idx_kingcounty_parcel_pin ON kingcounty_parcel (pin) WHERE pin IS NOT NULL;
CREATE INDEX idx_kingcounty_parcel_address_trgm ON kingcounty_parcel USING gin (address gin_trgm_ops);

CREATE TABLE sao_government_type (
    code                TEXT PRIMARY KEY,
    name                TEXT,
    raw_fields          JSONB NOT NULL DEFAULT '{}',
    source_record_id    BIGINT REFERENCES source_record(id)
);

CREATE TABLE sao_audit_type (
    audit_type_id       TEXT PRIMARY KEY,
    name                TEXT,
    raw_fields          JSONB NOT NULL DEFAULT '{}',
    source_record_id    BIGINT REFERENCES source_record(id)
);

CREATE TABLE sao_government_entity (
    id                  BIGSERIAL PRIMARY KEY,
    mcag                TEXT NOT NULL UNIQUE,
    name                TEXT NOT NULL,
    gov_type            TEXT,
    raw_fields          JSONB NOT NULL DEFAULT '{}',
    source_record_id    BIGINT REFERENCES source_record(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_sao_entity_name_trgm ON sao_government_entity USING gin (name gin_trgm_ops);

CREATE TABLE sao_report (
    id                  BIGSERIAL PRIMARY KEY,
    audit_report_number TEXT NOT NULL UNIQUE,
    audit_number        TEXT,
    report_title        TEXT,
    audit_type_name     TEXT,
    gov_type_desc       TEXT,
    date_released       DATE,
    begin_audit_period  DATE,
    end_audit_period    DATE,
    findings            BOOLEAN NOT NULL DEFAULT false,
    audit_report_url    TEXT,
    findings_url        TEXT,
    raw_fields          JSONB NOT NULL DEFAULT '{}',
    source_record_id    BIGINT NOT NULL REFERENCES source_record(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_sao_report_released ON sao_report (date_released DESC);
CREATE INDEX idx_sao_report_title_trgm ON sao_report USING gin (report_title gin_trgm_ops);
CREATE INDEX idx_sao_report_findings ON sao_report (findings);

CREATE TABLE seattle_auditor_recommendation (
    id                      BIGSERIAL PRIMARY KEY,
    dashboard_id             TEXT NOT NULL,
    recommendation_id        TEXT NOT NULL,
    recommendation_number    TEXT,
    recommendation_text      TEXT NOT NULL,
    category                 TEXT,
    audit_name               TEXT,
    audit_type               TEXT,
    department               TEXT,
    status                   TEXT,
    latest_update            TEXT,
    source_timestamp         TEXT,
    audit_url                TEXT,
    issue_date               DATE,
    finding_text             TEXT,
    finding_number           TEXT,
    year                     INT,
    raw_fields               JSONB NOT NULL DEFAULT '{}',
    source_record_id         BIGINT NOT NULL REFERENCES source_record(id),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (dashboard_id, recommendation_id)
);
CREATE INDEX idx_seattle_auditor_rec_status ON seattle_auditor_recommendation (status);
CREATE INDEX idx_seattle_auditor_rec_department ON seattle_auditor_recommendation (department);
CREATE INDEX idx_seattle_auditor_rec_text_trgm ON seattle_auditor_recommendation USING gin (recommendation_text gin_trgm_ops);

CREATE TABLE census_observation (
    id                  BIGSERIAL PRIMARY KEY,
    dataset             TEXT NOT NULL,
    vintage_year        INT NOT NULL,
    geography_id        TEXT NOT NULL,
    geography_name      TEXT,
    variable            TEXT NOT NULL,
    value_text          TEXT,
    value_numeric       NUMERIC,
    raw_fields          JSONB NOT NULL DEFAULT '{}',
    source_record_id    BIGINT NOT NULL REFERENCES source_record(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (dataset, vintage_year, geography_id, variable)
);
CREATE INDEX idx_census_obs_geo ON census_observation (geography_id);
CREATE INDEX idx_census_obs_variable ON census_observation (variable);

CREATE TABLE federal_award (
    id                          BIGSERIAL PRIMARY KEY,
    award_id                    TEXT NOT NULL UNIQUE,
    recipient_name              TEXT,
    recipient_uei               TEXT,
    awarding_agency             TEXT,
    funding_agency              TEXT,
    award_type                  TEXT,
    award_amount                NUMERIC,
    start_date                  DATE,
    end_date                    DATE,
    place_state_code            TEXT,
    place_county                TEXT,
    raw_fields                  JSONB NOT NULL DEFAULT '{}',
    source_record_id            BIGINT NOT NULL REFERENCES source_record(id),
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_federal_award_recipient_trgm ON federal_award USING gin (recipient_name gin_trgm_ops);
CREATE INDEX idx_federal_award_place ON federal_award (place_state_code, place_county);

CREATE TABLE fema_disaster_declaration (
    id                      BIGSERIAL PRIMARY KEY,
    disaster_number          TEXT NOT NULL,
    declaration_type         TEXT,
    state_code               TEXT,
    county_name              TEXT,
    incident_type            TEXT,
    incident_title           TEXT,
    declaration_date         DATE,
    incident_begin_date      DATE,
    incident_end_date        DATE,
    raw_fields               JSONB NOT NULL DEFAULT '{}',
    source_record_id         BIGINT NOT NULL REFERENCES source_record(id),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (disaster_number, state_code, county_name)
);
CREATE INDEX idx_fema_disaster_state_county ON fema_disaster_declaration (state_code, county_name);
CREATE INDEX idx_fema_disaster_type ON fema_disaster_declaration (incident_type);

CREATE TABLE bls_observation (
    id                  BIGSERIAL PRIMARY KEY,
    series_id           TEXT NOT NULL,
    year                INT NOT NULL,
    period              TEXT NOT NULL,
    period_name         TEXT,
    value_numeric       NUMERIC,
    value_text          TEXT,
    footnotes           JSONB NOT NULL DEFAULT '[]',
    source_record_id    BIGINT NOT NULL REFERENCES source_record(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (series_id, year, period)
);
CREATE INDEX idx_bls_obs_series_year ON bls_observation (series_id, year);

CREATE TABLE hud_dataset_ref (
    id                  BIGSERIAL PRIMARY KEY,
    source_key          TEXT NOT NULL UNIQUE,
    name                TEXT NOT NULL,
    url                 TEXT NOT NULL,
    kind                TEXT,
    program             TEXT,
    geography           TEXT,
    dataset_year        INT,
    notes               TEXT,
    source_record_id    BIGINT REFERENCES source_record(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_hud_dataset_name_trgm ON hud_dataset_ref USING gin (name gin_trgm_ops);

CREATE TABLE hud_housing_observation (
    id                  BIGSERIAL PRIMARY KEY,
    dataset_key         TEXT NOT NULL,
    geography_id        TEXT NOT NULL,
    geography_name      TEXT,
    indicator           TEXT NOT NULL,
    observation_year    INT,
    value_numeric       NUMERIC,
    value_text          TEXT,
    unit                TEXT,
    raw_fields          JSONB NOT NULL DEFAULT '{}',
    source_record_id    BIGINT NOT NULL REFERENCES source_record(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (dataset_key, geography_id, indicator, observation_year)
);
CREATE INDEX idx_hud_obs_geo ON hud_housing_observation (geography_id);
CREATE INDEX idx_hud_obs_indicator ON hud_housing_observation (indicator);

CREATE TABLE epa_facility (
    id                  BIGSERIAL PRIMARY KEY,
    registry_id         TEXT NOT NULL UNIQUE,
    name                TEXT,
    address             TEXT,
    city                TEXT,
    state_code          TEXT,
    zip                 TEXT,
    latitude            DOUBLE PRECISION,
    longitude           DOUBLE PRECISION,
    programs            TEXT[] NOT NULL DEFAULT '{}',
    raw_fields          JSONB NOT NULL DEFAULT '{}',
    source_record_id    BIGINT NOT NULL REFERENCES source_record(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_epa_facility_name_trgm ON epa_facility USING gin (name gin_trgm_ops);
CREATE INDEX idx_epa_facility_state_city ON epa_facility (state_code, city);

CREATE TABLE epa_ejscreen_observation (
    id                  BIGSERIAL PRIMARY KEY,
    geography_id        TEXT NOT NULL,
    indicator           TEXT NOT NULL,
    percentile          NUMERIC,
    value_numeric       NUMERIC,
    raw_fields          JSONB NOT NULL DEFAULT '{}',
    source_record_id    BIGINT NOT NULL REFERENCES source_record(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (geography_id, indicator)
);
CREATE INDEX idx_epa_ej_geo ON epa_ejscreen_observation (geography_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS epa_ejscreen_observation;
DROP TABLE IF EXISTS epa_facility;
DROP TABLE IF EXISTS hud_housing_observation;
DROP TABLE IF EXISTS hud_dataset_ref;
DROP TABLE IF EXISTS bls_observation;
DROP TABLE IF EXISTS fema_disaster_declaration;
DROP TABLE IF EXISTS federal_award;
DROP TABLE IF EXISTS census_observation;
DROP TABLE IF EXISTS seattle_auditor_recommendation;
DROP TABLE IF EXISTS sao_report;
DROP TABLE IF EXISTS sao_government_entity;
DROP TABLE IF EXISTS sao_audit_type;
DROP TABLE IF EXISTS sao_government_type;
DROP TABLE IF EXISTS kingcounty_parcel;
DROP TABLE IF EXISTS seattle_permit;
DROP TABLE IF EXISTS datawa_contract;
DROP TABLE IF EXISTS source_dataset;
ALTER TABLE source_record DROP CONSTRAINT IF EXISTS source_record_source_system_check;
ALTER TABLE source_record ADD CONSTRAINT source_record_source_system_check CHECK (source_system IN (
    'lws', 'committee_schedules', 'csi', 'tvw', 'invintus', 'pdc_socrata'
));

-- +goose StatementEnd
