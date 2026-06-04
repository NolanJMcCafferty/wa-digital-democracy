-- +goose Up
-- +goose StatementBegin

-- Preserve CSI roster metadata that is useful for speaker identity review, and
-- make testifier rows stable across refreshes. CSI Count is the roster order;
-- CssClass groups panel testimony rows.
ALTER TABLE testifier
    ADD COLUMN IF NOT EXISTS csi_order INT,
    ADD COLUMN IF NOT EXISTS csi_panel_class TEXT,
    ADD COLUMN IF NOT EXISTS source_key TEXT,
    ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

UPDATE testifier
   SET source_key = concat_ws('|', agenda_item_id::text, COALESCE(csi_order::text, ''), lower(trim(raw_name)), lower(trim(COALESCE(raw_organization, ''))), position::text, testified::text)
 WHERE source_key IS NULL OR source_key = '';

CREATE UNIQUE INDEX IF NOT EXISTS uniq_testifier_agenda_source_key
    ON testifier (agenda_item_id, source_key)
    WHERE source_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_testifier_agenda_active_order
    ON testifier (agenda_item_id, active, csi_order NULLS LAST, id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_testifier_agenda_active_order;
DROP INDEX IF EXISTS uniq_testifier_agenda_source_key;
ALTER TABLE testifier
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS last_seen_at,
    DROP COLUMN IF EXISTS active,
    DROP COLUMN IF EXISTS source_key,
    DROP COLUMN IF EXISTS csi_panel_class,
    DROP COLUMN IF EXISTS csi_order;

-- +goose StatementEnd
