-- +goose Up
-- +goose StatementBegin

CREATE TABLE datawa_master_contract_sale (
    id                      BIGSERIAL PRIMARY KEY,
    source_dataset_id        TEXT NOT NULL,
    source_row_id            TEXT NOT NULL,
    customer_type            TEXT,
    customer_name            TEXT,
    contract_number          TEXT,
    contract_title           TEXT,
    vendor_name              TEXT,
    report_year              INT,
    q1_sales_reported        NUMERIC,
    q2_sales_reported        NUMERIC,
    q3_sales_reported        NUMERIC,
    q4_sales_reported        NUMERIC,
    total_sales_reported     NUMERIC,
    omwbe                    TEXT,
    veteran_owned            TEXT,
    small_business           TEXT,
    diverse_options          TEXT,
    normalization_warnings   JSONB NOT NULL DEFAULT '[]',
    raw_fields               JSONB NOT NULL DEFAULT '{}',
    source_record_id         BIGINT NOT NULL REFERENCES source_record(id),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_dataset_id, source_row_id)
);

CREATE INDEX idx_datawa_master_sale_customer ON datawa_master_contract_sale USING gin (customer_name gin_trgm_ops);
CREATE INDEX idx_datawa_master_sale_vendor ON datawa_master_contract_sale USING gin (vendor_name gin_trgm_ops);
CREATE INDEX idx_datawa_master_sale_contract ON datawa_master_contract_sale (contract_number) WHERE contract_number IS NOT NULL;
CREATE INDEX idx_datawa_master_sale_year ON datawa_master_contract_sale (report_year);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS datawa_master_contract_sale;

-- +goose StatementEnd
