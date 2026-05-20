-- +goose Up
-- +goose StatementBegin

ALTER TYPE entity_match_source_kind ADD VALUE IF NOT EXISTS 'fiscalwa_vendor_payment';
ALTER TYPE entity_match_source_kind ADD VALUE IF NOT EXISTS 'federal_award_recipient';
ALTER TYPE entity_match_source_kind ADD VALUE IF NOT EXISTS 'deepgram_organization_mention';

ALTER TABLE fiscalwa_vendor_payment
    ADD COLUMN IF NOT EXISTS normalized_vendor_name TEXT;
UPDATE fiscalwa_vendor_payment
   SET normalized_vendor_name = wa_dd_normalize_entity_name(vendor_name)
 WHERE normalized_vendor_name IS NULL
   AND vendor_name IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_fiscalwa_vendor_payment_normalized_vendor
    ON fiscalwa_vendor_payment (normalized_vendor_name)
 WHERE normalized_vendor_name IS NOT NULL;

ALTER TABLE federal_award
    ADD COLUMN IF NOT EXISTS normalized_recipient_name TEXT;
UPDATE federal_award
   SET normalized_recipient_name = wa_dd_normalize_entity_name(recipient_name)
 WHERE normalized_recipient_name IS NULL
   AND recipient_name IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_federal_award_normalized_recipient
    ON federal_award (normalized_recipient_name)
 WHERE normalized_recipient_name IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_federal_award_normalized_recipient;
ALTER TABLE federal_award
    DROP COLUMN IF EXISTS normalized_recipient_name;

DROP INDEX IF EXISTS idx_fiscalwa_vendor_payment_normalized_vendor;
ALTER TABLE fiscalwa_vendor_payment
    DROP COLUMN IF EXISTS normalized_vendor_name;

-- PostgreSQL enum values cannot be removed without replacing the type, so the
-- entity_match_source_kind additions are intentionally left in place on down.

-- +goose StatementEnd
