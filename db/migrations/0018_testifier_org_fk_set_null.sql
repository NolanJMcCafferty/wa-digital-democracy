-- +goose Up
-- +goose StatementBegin

-- testifier.normalized_org_id originally had no ON DELETE action (see
-- 0001_initial.sql), which forced the prune-junk-organizations command to
-- manually unlink testifier rows before deleting an organization. Switch the
-- constraint to ON DELETE SET NULL so deletes propagate cleanly.

ALTER TABLE testifier
    DROP CONSTRAINT IF EXISTS fk_testifier_organization;

ALTER TABLE testifier
    ADD CONSTRAINT fk_testifier_organization
    FOREIGN KEY (normalized_org_id)
    REFERENCES organization(id)
    ON DELETE SET NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE testifier
    DROP CONSTRAINT IF EXISTS fk_testifier_organization;

ALTER TABLE testifier
    ADD CONSTRAINT fk_testifier_organization
    FOREIGN KEY (normalized_org_id)
    REFERENCES organization(id);

-- +goose StatementEnd
