-- +goose Up
-- +goose StatementBegin

-- Add 'paid_by' to person_org_affiliation_type so PDC lobbyist compensation
-- ingestion (9nnw-c693) can record firm <- client edges (filer is paid_by employer).
ALTER TYPE person_org_affiliation_type
    ADD VALUE IF NOT EXISTS 'paid_by'
    AFTER 'paid_lobbying_for';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Postgres cannot remove enum values; leaving 'paid_by' in place on rollback.

-- +goose StatementEnd
