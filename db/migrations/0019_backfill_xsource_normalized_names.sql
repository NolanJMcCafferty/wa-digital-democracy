-- +goose Up
-- +goose StatementBegin

-- The IRS BMF and PDC employer ingestion commands originally wrote
-- normalized_name using a Go-side normalizer (lowercase, kept corporate
-- suffixes), while organization-name lookups normalize via the SQL function
-- wa_dd_normalize_entity_name (uppercase, strips suffixes). The two
-- conventions never matched, so verify-organizations always reported zero
-- hits. Backfill both tables using the SQL normalizer; new inserts now use
-- the same function inline.

UPDATE irs_bmf_organization
   SET normalized_name = COALESCE(wa_dd_normalize_entity_name(name), '');

UPDATE pdc_employer
   SET normalized_name = COALESCE(wa_dd_normalize_entity_name(name), '');

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- No-op: the previous Go-side normalization was lossy and not worth restoring.
SELECT 1;
-- +goose StatementEnd
