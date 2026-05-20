-- +goose Up
-- +goose StatementBegin

-- org_context_record was an early PDC-context table populated by a removed
-- reviewed-match path. Current organization verification uses pdc_employer and
-- irs_bmf_organization directly, so this table is no longer part of the model.
DROP TABLE IF EXISTS org_context_record;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

CREATE TABLE org_context_record (
    id                  BIGSERIAL PRIMARY KEY,
    organization_id     BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    context_type        TEXT NOT NULL CHECK (context_type IN (
        'lobbying_registration', 'lobbying_spend',
        'campaign_contribution', 'independent_expenditure'
    )),
    source_dataset_id   TEXT NOT NULL,
    source_row_id       TEXT,
    summary_fields      JSONB NOT NULL,
    source_url          TEXT NOT NULL,
    match_confidence    org_match_confidence NOT NULL DEFAULT 'possible',
    source_record_id    BIGINT REFERENCES source_record(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_org_context_org_type ON org_context_record (organization_id, context_type);

-- +goose StatementEnd
