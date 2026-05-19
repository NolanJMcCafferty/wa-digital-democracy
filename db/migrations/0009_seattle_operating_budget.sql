-- +goose Up
-- +goose StatementBegin

CREATE TABLE seattle_operating_budget (
    id                  BIGSERIAL PRIMARY KEY,
    source_dataset_id    TEXT NOT NULL,
    source_row_id        TEXT NOT NULL,
    fiscal_year          INT,
    service              TEXT,
    department           TEXT,
    program              TEXT,
    fund                 TEXT,
    fund_type            TEXT,
    expense_type         TEXT,
    description          TEXT,
    approved_amount      NUMERIC,
    raw_fields           JSONB NOT NULL DEFAULT '{}',
    source_record_id     BIGINT NOT NULL REFERENCES source_record(id),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_dataset_id, source_row_id)
);

CREATE INDEX idx_seattle_operating_budget_department ON seattle_operating_budget USING gin (department gin_trgm_ops);
CREATE INDEX idx_seattle_operating_budget_program ON seattle_operating_budget USING gin (program gin_trgm_ops);
CREATE INDEX idx_seattle_operating_budget_fy ON seattle_operating_budget (fiscal_year);
CREATE INDEX idx_seattle_operating_budget_fund_type ON seattle_operating_budget (fund_type) WHERE fund_type IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS seattle_operating_budget;

-- +goose StatementEnd
