-- +goose Up
-- +goose StatementBegin

ALTER TYPE entity_match_source_kind ADD VALUE IF NOT EXISTS 'csi_testimony_organization';

ALTER TABLE organization DROP CONSTRAINT IF EXISTS organization_verification_source_check;
ALTER TABLE organization
    ADD CONSTRAINT organization_verification_source_check
    CHECK (verification_source IS NULL OR verification_source IN
        ('irs_bmf', 'pdc_employer', 'manual', 'csi_testimony_review'));

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE organization DROP CONSTRAINT IF EXISTS organization_verification_source_check;
ALTER TABLE organization
    ADD CONSTRAINT organization_verification_source_check
    CHECK (verification_source IS NULL OR verification_source IN
        ('irs_bmf', 'pdc_employer', 'manual'));

-- PostgreSQL enum values cannot be removed without replacing the type, so the
-- csi_testimony_organization addition is intentionally left in place on down.

-- +goose StatementEnd
