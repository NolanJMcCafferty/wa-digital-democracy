-- +goose Up
-- +goose StatementBegin

-- Extracted entity mentions from provider/model outputs. These are evidence,
-- not canonical facts: later entity-resolution/review links them to bills,
-- legislators, organizations, agencies, vendors, etc.
CREATE TABLE entity_mention (
    id                    BIGSERIAL PRIMARY KEY,
    tvw_event_id           TEXT REFERENCES tvw_event(tvw_event_id) ON DELETE CASCADE,
    diarization_job_id     BIGINT REFERENCES diarization_job(id) ON DELETE CASCADE,
    source_kind            TEXT NOT NULL CHECK (source_kind IN (
        'diarization_job', 'transcript_segment', 'transcript_turn', 'testimony', 'document'
    )),
    source_id              BIGINT,
    extractor              TEXT NOT NULL,
    model                  TEXT,
    entity_type            TEXT NOT NULL,
    text                   TEXT NOT NULL,
    normalized_text        TEXT,
    start_ms               INT,
    end_ms                 INT,
    start_word             INT,
    end_word               INT,
    confidence             NUMERIC,
    raw                    JSONB NOT NULL DEFAULT '{}',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (end_ms IS NULL OR start_ms IS NULL OR end_ms >= start_ms)
);
CREATE INDEX idx_entity_mention_event_type ON entity_mention (tvw_event_id, entity_type);
CREATE INDEX idx_entity_mention_job ON entity_mention (diarization_job_id);
CREATE INDEX idx_entity_mention_text_trgm ON entity_mention USING gin (text gin_trgm_ops);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_entity_mention_text_trgm;
DROP INDEX IF EXISTS idx_entity_mention_job;
DROP INDEX IF EXISTS idx_entity_mention_event_type;
DROP TABLE IF EXISTS entity_mention;

-- +goose StatementEnd
