-- +goose Up
-- +goose StatementBegin

-- Audio caching + provider-neutral diarization output for TVW/Invintus events.
-- TVW provides caption VTT plus downloadable audio/video URLs; this layer keeps
-- local normalized audio metadata and anonymous speaker-cluster time spans for
-- later VTT alignment, transcript_turn creation, and human review.

CREATE TYPE diarization_job_status AS ENUM (
    'pending',
    'running',
    'succeeded',
    'failed'
);

CREATE TABLE tvw_audio_asset (
    id                  BIGSERIAL PRIMARY KEY,
    tvw_event_id        TEXT NOT NULL REFERENCES tvw_event(tvw_event_id) ON DELETE CASCADE,
    source_url          TEXT NOT NULL,
    source_kind         TEXT NOT NULL CHECK (source_kind IN (
        'audio_download_url', 'published_audio_url', 'media_asset_audio',
        'video_download_url', 'media_asset_video'
    )),
    original_path       TEXT NOT NULL,
    normalized_path     TEXT NOT NULL,
    content_hash        TEXT NOT NULL,
    duration_ms         INT,
    sample_rate         INT,
    channels            INT,
    codec               TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tvw_event_id, content_hash)
);
CREATE INDEX idx_tvw_audio_asset_event ON tvw_audio_asset (tvw_event_id);

CREATE TABLE diarization_job (
    id                  BIGSERIAL PRIMARY KEY,
    tvw_event_id        TEXT NOT NULL REFERENCES tvw_event(tvw_event_id) ON DELETE CASCADE,
    audio_asset_id      BIGINT REFERENCES tvw_audio_asset(id),
    provider            TEXT NOT NULL,
    provider_job_id     TEXT,
    model               TEXT,
    status              diarization_job_status NOT NULL DEFAULT 'pending',
    submitted_at        TIMESTAMPTZ,
    finished_at         TIMESTAMPTZ,
    error               TEXT,
    raw_result_path     TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_diarization_job_event ON diarization_job (tvw_event_id, created_at DESC);
CREATE INDEX idx_diarization_job_status ON diarization_job (status);

CREATE TABLE speaker_cluster (
    id                  BIGSERIAL PRIMARY KEY,
    diarization_job_id  BIGINT NOT NULL REFERENCES diarization_job(id) ON DELETE CASCADE,
    tvw_event_id        TEXT NOT NULL,
    cluster_label       TEXT NOT NULL,
    total_speech_ms     INT,
    turn_count          INT,
    metadata            JSONB NOT NULL DEFAULT '{}',
    UNIQUE (diarization_job_id, cluster_label)
);
CREATE INDEX idx_speaker_cluster_event ON speaker_cluster (tvw_event_id);

CREATE TABLE diarized_speech_segment (
    id                  BIGSERIAL PRIMARY KEY,
    diarization_job_id  BIGINT NOT NULL REFERENCES diarization_job(id) ON DELETE CASCADE,
    speaker_cluster_id  BIGINT REFERENCES speaker_cluster(id) ON DELETE CASCADE,
    tvw_event_id        TEXT NOT NULL,
    cluster_label       TEXT NOT NULL,
    start_ms            INT NOT NULL,
    end_ms              INT NOT NULL,
    confidence          NUMERIC,
    text                TEXT,
    raw                 JSONB NOT NULL DEFAULT '{}',
    CHECK (end_ms > start_ms)
);
CREATE INDEX idx_diarized_segment_event_time ON diarized_speech_segment (tvw_event_id, start_ms);
CREATE INDEX idx_diarized_segment_job_cluster ON diarized_speech_segment (diarization_job_id, cluster_label);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_diarized_segment_job_cluster;
DROP INDEX IF EXISTS idx_diarized_segment_event_time;
DROP TABLE IF EXISTS diarized_speech_segment;

DROP INDEX IF EXISTS idx_speaker_cluster_event;
DROP TABLE IF EXISTS speaker_cluster;

DROP INDEX IF EXISTS idx_diarization_job_status;
DROP INDEX IF EXISTS idx_diarization_job_event;
DROP TABLE IF EXISTS diarization_job;

DROP INDEX IF EXISTS idx_tvw_audio_asset_event;
DROP TABLE IF EXISTS tvw_audio_asset;

DROP TYPE IF EXISTS diarization_job_status;

-- +goose StatementEnd
