-- +goose Up
-- +goose StatementBegin

-- Retire the VTT-derived transcript_segment table. Diarized speech segments
-- (diarized_speech_segment) plus reviewed speaker_assignment rows are now
-- the canonical transcript surface for both the bill-detail page and
-- transcript search. agenda_item_window remains the per-bill time mapping.

ALTER TABLE entity_mention DROP CONSTRAINT IF EXISTS entity_mention_source_kind_check;
ALTER TABLE entity_mention ADD CONSTRAINT entity_mention_source_kind_check
    CHECK (source_kind IN (
        'diarization_job', 'transcript_turn', 'testimony', 'document'
    ));

DROP TABLE IF EXISTS transcript_segment;

DROP TYPE IF EXISTS speaker_confidence;

-- Diarized text is now the search corpus. The previous FTS index lived on
-- transcript_segment; mirror it on diarized_speech_segment.
CREATE INDEX IF NOT EXISTS idx_diarized_segment_text_fts
    ON diarized_speech_segment USING gin (to_tsvector('english', text));

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_diarized_segment_text_fts;

CREATE TYPE speaker_confidence AS ENUM (
    'confirmed_legislator',
    'likely_legislator',
    'likely_testifier',
    'unknown_speaker',
    'ai_inferred_pending_review'
);

CREATE TABLE transcript_segment (
    id                  BIGSERIAL PRIMARY KEY,
    tvw_event_id        TEXT NOT NULL REFERENCES tvw_event(tvw_event_id) ON DELETE CASCADE,
    agenda_item_id      BIGINT REFERENCES agenda_item(id),
    start_ms            INT NOT NULL,
    end_ms              INT NOT NULL,
    text                TEXT NOT NULL,
    speaker_label       TEXT,
    speaker_entity_id   BIGINT,
    speaker_confidence  speaker_confidence NOT NULL DEFAULT 'unknown_speaker',
    source_caption_url  TEXT NOT NULL,
    source_record_id    BIGINT NOT NULL REFERENCES source_record(id)
);
CREATE INDEX idx_transcript_segment_event_time ON transcript_segment (tvw_event_id, start_ms);
CREATE INDEX idx_transcript_segment_agenda     ON transcript_segment (agenda_item_id) WHERE agenda_item_id IS NOT NULL;
CREATE INDEX idx_transcript_segment_text_fts   ON transcript_segment USING gin (to_tsvector('english', text));

ALTER TABLE entity_mention DROP CONSTRAINT IF EXISTS entity_mention_source_kind_check;
ALTER TABLE entity_mention ADD CONSTRAINT entity_mention_source_kind_check
    CHECK (source_kind IN (
        'diarization_job', 'transcript_segment', 'transcript_turn', 'testimony', 'document'
    ));

-- +goose StatementEnd
