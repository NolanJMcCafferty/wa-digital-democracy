-- +goose Up
-- +goose StatementBegin

-- Retire organization_source_mention. Only the CSI testifier path ever wrote
-- to it, and that information is fully recoverable from testifier
-- (raw_organization, normalized_org_id, source_record_id). Cross-source org
-- provenance lives in reviewed_vendor_entity_match plus the organization.pdc_*
-- ID columns; nothing reads from organization_source_mention.

DROP INDEX IF EXISTS idx_organization_source_mention_kind;
DROP INDEX IF EXISTS idx_organization_source_mention_normalized;
DROP INDEX IF EXISTS idx_organization_source_mention_org;
DROP TABLE IF EXISTS organization_source_mention;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

CREATE TABLE organization_source_mention (
    id                 BIGSERIAL PRIMARY KEY,
    source_kind        TEXT NOT NULL CHECK (source_kind IN (
        'csi_testifier', 'pdc_lobbying_employer', 'deepgram_entity',
        'datawa_vendor', 'federal_recipient', 'manual_import'
    )),
    source_table       TEXT NOT NULL,
    source_pk          BIGINT,
    source_name        TEXT NOT NULL,
    normalized_name    TEXT NOT NULL,
    organization_id    BIGINT REFERENCES organization(id) ON DELETE SET NULL,
    occurrence_count   INT NOT NULL DEFAULT 1,
    confidence         org_match_confidence NOT NULL DEFAULT 'possible',
    source_record_id   BIGINT REFERENCES source_record(id),
    first_seen_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_kind, source_table, source_pk, source_name)
);
CREATE INDEX idx_organization_source_mention_org ON organization_source_mention (organization_id);
CREATE INDEX idx_organization_source_mention_normalized ON organization_source_mention (normalized_name);
CREATE INDEX idx_organization_source_mention_kind ON organization_source_mention (source_kind);

-- +goose StatementEnd
