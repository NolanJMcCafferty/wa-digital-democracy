-- +goose Up
-- +goose StatementBegin

-- PDC lobbyist_id is a stable source identifier for lobbyists / lobbying firms.
-- Keep it first-class on person so PDC ingestion can deduplicate by source ID
-- instead of name-only matching.
--
-- The original person table had a UNIQUE(display_name) constraint because the
-- first producer was CSI sign-ins. That is too strong for real person identity:
-- two people can share a name, and a PDC lobbyist_id should not collide with a
-- same-named CSI-only person. Replace it with a non-unique display-name index.
ALTER TABLE person
    DROP CONSTRAINT IF EXISTS person_display_name_key;

CREATE INDEX IF NOT EXISTS idx_person_display_name
    ON person (display_name);

ALTER TABLE person
    ADD COLUMN pdc_lobbyist_id TEXT;

CREATE UNIQUE INDEX idx_person_pdc_lobbyist_id
    ON person (pdc_lobbyist_id)
 WHERE pdc_lobbyist_id IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_person_pdc_lobbyist_id;
ALTER TABLE person
    DROP COLUMN IF EXISTS pdc_lobbyist_id;

DROP INDEX IF EXISTS idx_person_display_name;
ALTER TABLE person
    ADD CONSTRAINT person_display_name_key UNIQUE (display_name);

-- +goose StatementEnd
