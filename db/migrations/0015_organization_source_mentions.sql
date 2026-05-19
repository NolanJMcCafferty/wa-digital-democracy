-- +goose Up
-- +goose StatementBegin

-- Source-backed organization-name mentions used to seed canonical organization
-- rows and keep provenance around where a name came from. Mentions are not
-- cross-source identity assertions; reviewed/candidate match layers handle that.
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

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_organization_source_mention_kind;
DROP INDEX IF EXISTS idx_organization_source_mention_normalized;
DROP INDEX IF EXISTS idx_organization_source_mention_org;
DROP TABLE IF EXISTS organization_source_mention;

-- +goose StatementEnd
