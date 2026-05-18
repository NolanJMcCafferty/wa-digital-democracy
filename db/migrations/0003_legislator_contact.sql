-- +goose Up
-- +goose StatementBegin

-- Adds the per-member fields LWS SponsorService.GetHouseSponsors and
-- GetSenateSponsors return that the original schema didn't surface.
-- All nullable so existing rows (populated by IngestBill from per-bill
-- GetSponsors responses, which carry less detail) keep working.

ALTER TABLE legislator
    ADD COLUMN IF NOT EXISTS first_name TEXT,
    ADD COLUMN IF NOT EXISTS last_name  TEXT,
    ADD COLUMN IF NOT EXISTS email      TEXT,
    ADD COLUMN IF NOT EXISTS phone      TEXT,
    ADD COLUMN IF NOT EXISTS acronym    TEXT;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE legislator
    DROP COLUMN IF EXISTS first_name,
    DROP COLUMN IF EXISTS last_name,
    DROP COLUMN IF EXISTS email,
    DROP COLUMN IF EXISTS phone,
    DROP COLUMN IF EXISTS acronym;

-- +goose StatementEnd
