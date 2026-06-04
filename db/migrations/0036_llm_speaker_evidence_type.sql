-- +goose Up
-- +goose StatementBegin

ALTER TYPE speaker_evidence_type ADD VALUE IF NOT EXISTS 'llm_candidate';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- PostgreSQL enum values cannot be removed without replacing the type, so the
-- speaker_evidence_type addition is intentionally left in place on down.

-- +goose StatementEnd
