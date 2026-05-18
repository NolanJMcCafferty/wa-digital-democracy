-- +goose Up
-- +goose StatementBegin

CREATE TABLE datawa_webs_vendor (
    id                       BIGSERIAL PRIMARY KEY,
    source_dataset_id         TEXT NOT NULL,
    source_row_id             TEXT NOT NULL,
    company_name              TEXT,
    normalized_company_name   TEXT,
    dba_name                  TEXT,
    phone_number              TEXT,
    contact_email             TEXT,
    city                      TEXT,
    state                     TEXT,
    web_address               TEXT,
    commodity_code            TEXT,
    description_of_work       TEXT,
    small_business            TEXT,
    veteran_owned             TEXT,
    other_cert                TEXT,
    other_cert_2              TEXT,
    normalization_warnings    JSONB NOT NULL DEFAULT '[]',
    raw_fields                JSONB NOT NULL DEFAULT '{}',
    source_record_id          BIGINT NOT NULL REFERENCES source_record(id),
    created_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_dataset_id, source_row_id)
);

CREATE INDEX idx_datawa_webs_vendor_company ON datawa_webs_vendor USING gin (company_name gin_trgm_ops);
CREATE INDEX idx_datawa_webs_vendor_normalized_company ON datawa_webs_vendor (normalized_company_name);
CREATE INDEX idx_datawa_webs_vendor_commodity_code ON datawa_webs_vendor (commodity_code) WHERE commodity_code IS NOT NULL;
CREATE INDEX idx_datawa_webs_vendor_state ON datawa_webs_vendor (state) WHERE state IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS datawa_webs_vendor;

-- +goose StatementEnd
