-- +goose Up
-- +goose StatementBegin

CREATE TABLE datawa_it_contract (
    id                           BIGSERIAL PRIMARY KEY,
    source_dataset_id             TEXT NOT NULL,
    source_row_id                 TEXT NOT NULL,
    report_fiscal_year            INT,
    agency_number_agency_name     TEXT,
    agency_number                 TEXT,
    agency_name                   TEXT,
    contract_number               TEXT,
    contractor_name               TEXT,
    contractor_dba                TEXT,
    cooperative_purchase          BOOLEAN,
    cooperative_name              TEXT,
    statewide_contract_purchase   BOOLEAN,
    contract_start_date           DATE,
    contract_end_date             DATE,
    fiscal_year_start             TEXT,
    fiscal_year_end               TEXT,
    it_tower_application          NUMERIC,
    it_tower_compute              NUMERIC,
    it_tower_data_center          NUMERIC,
    it_tower_delivery             NUMERIC,
    it_tower_end_user             NUMERIC,
    it_tower_it_management        NUMERIC,
    it_tower_network              NUMERIC,
    it_tower_output               NUMERIC,
    it_tower_platform             NUMERIC,
    it_tower_security             NUMERIC,
    it_tower_storage              NUMERIC,
    other_non_it                  NUMERIC,
    total_percentage              NUMERIC,
    contract_amount_fy20          NUMERIC,
    contract_amount_fy21          NUMERIC,
    contract_amount_fy22          NUMERIC,
    contract_amount_fy23          NUMERIC,
    contract_amount_fy24          NUMERIC,
    contract_amount_fy25          NUMERIC,
    contract_amount_fy26          NUMERIC,
    contract_amount_fy27          NUMERIC,
    contract_amount_fy28          NUMERIC,
    contract_amount_fy29          NUMERIC,
    contract_amount_fy30          NUMERIC,
    total_contract_amount         NUMERIC,
    contract_amount_explanation   TEXT,
    normalization_warnings        JSONB NOT NULL DEFAULT '[]',
    raw_fields                    JSONB NOT NULL DEFAULT '{}',
    source_record_id              BIGINT NOT NULL REFERENCES source_record(id),
    created_at                    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_dataset_id, source_row_id)
);

CREATE INDEX idx_datawa_it_contract_agency ON datawa_it_contract USING gin (agency_name gin_trgm_ops);
CREATE INDEX idx_datawa_it_contract_contractor ON datawa_it_contract USING gin (contractor_name gin_trgm_ops);
CREATE INDEX idx_datawa_it_contract_report_fy ON datawa_it_contract (report_fiscal_year);
CREATE INDEX idx_datawa_it_contract_contract ON datawa_it_contract (contract_number) WHERE contract_number IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS datawa_it_contract;

-- +goose StatementEnd
