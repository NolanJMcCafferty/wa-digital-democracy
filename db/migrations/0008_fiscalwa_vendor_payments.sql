-- +goose Up
-- +goose StatementBegin

ALTER TABLE source_record DROP CONSTRAINT IF EXISTS source_record_source_system_check;
ALTER TABLE source_record ADD CONSTRAINT source_record_source_system_check CHECK (source_system IN (
    'lws', 'committee_schedules', 'csi', 'tvw', 'invintus', 'pdc_socrata',
    'datawa_socrata', 'seattle_socrata', 'kingcounty_socrata',
    'sao_reportsearch', 'seattle_auditor', 'census', 'usaspending',
    'openfema', 'bls', 'hud', 'epa', 'fiscal_wa'
));

CREATE TABLE fiscalwa_vendor_payment (
    id                  BIGSERIAL PRIMARY KEY,
    source_dataset_id    TEXT NOT NULL,
    source_row_id        TEXT NOT NULL,
    biennium             TEXT,
    fiscal_year          INT,
    fiscal_month         TEXT,
    agency_number        TEXT,
    agency_name          TEXT,
    object_code          TEXT,
    object_category      TEXT,
    subobject_code       TEXT,
    subobject_name       TEXT,
    vendor_name          TEXT,
    amount               NUMERIC,
    raw_fields           JSONB NOT NULL DEFAULT '{}',
    source_record_id     BIGINT NOT NULL REFERENCES source_record(id),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_dataset_id, source_row_id)
);

CREATE INDEX idx_fiscalwa_vendor_payment_agency ON fiscalwa_vendor_payment USING gin (agency_name gin_trgm_ops);
CREATE INDEX idx_fiscalwa_vendor_payment_vendor ON fiscalwa_vendor_payment USING gin (vendor_name gin_trgm_ops);
CREATE INDEX idx_fiscalwa_vendor_payment_bien_fy ON fiscalwa_vendor_payment (biennium, fiscal_year, fiscal_month);
CREATE INDEX idx_fiscalwa_vendor_payment_object ON fiscalwa_vendor_payment (object_code, subobject_code);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS fiscalwa_vendor_payment;

ALTER TABLE source_record DROP CONSTRAINT IF EXISTS source_record_source_system_check;
ALTER TABLE source_record ADD CONSTRAINT source_record_source_system_check CHECK (source_system IN (
    'lws', 'committee_schedules', 'csi', 'tvw', 'invintus', 'pdc_socrata',
    'datawa_socrata', 'seattle_socrata', 'kingcounty_socrata',
    'sao_reportsearch', 'seattle_auditor', 'census', 'usaspending',
    'openfema', 'bls', 'hud', 'epa'
));

-- +goose StatementEnd
