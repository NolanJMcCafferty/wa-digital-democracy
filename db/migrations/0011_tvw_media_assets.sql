-- +goose Up
-- +goose StatementBegin

-- Rich TVW/Invintus media metadata. This preserves the public video/audio,
-- caption, HLS, and document assets needed for transcript QA and future
-- diarization while keeping raw source responses in source_record.

ALTER TABLE tvw_event
    ADD COLUMN IF NOT EXISTS wp_slug TEXT,
    ADD COLUMN IF NOT EXISTS wp_link TEXT,
    ADD COLUMN IF NOT EXISTS custom_id TEXT,
    ADD COLUMN IF NOT EXISTS location_name TEXT,
    ADD COLUMN IF NOT EXISTS total_runtime TEXT,
    ADD COLUMN IF NOT EXISTS total_runtime_seconds INT,
    ADD COLUMN IF NOT EXISTS published_audio_url TEXT,
    ADD COLUMN IF NOT EXISTS audio_download_url TEXT,
    ADD COLUMN IF NOT EXISTS video_download_url TEXT,
    ADD COLUMN IF NOT EXISTS streaming_uris JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS raw_keywords JSONB NOT NULL DEFAULT '[]',
    ADD COLUMN IF NOT EXISTS raw_wp_tags JSONB NOT NULL DEFAULT '[]',
    ADD COLUMN IF NOT EXISTS raw_wp_categories JSONB NOT NULL DEFAULT '[]';

CREATE TABLE tvw_media_asset (
    id                  BIGSERIAL PRIMARY KEY,
    tvw_event_id        TEXT NOT NULL REFERENCES tvw_event(tvw_event_id) ON DELETE CASCADE,
    asset_id            TEXT NOT NULL,
    asset_type          TEXT NOT NULL,
    name                TEXT,
    file_url            TEXT,
    thumbnail_url       TEXT,
    sprite_url          TEXT,
    preview_url         TEXT,
    file_size_bytes     BIGINT,
    total_runtime       TEXT,
    total_runtime_seconds INT,
    current_status      TEXT,
    date_created        TIMESTAMPTZ,
    advanced_details    JSONB NOT NULL DEFAULT '{}',
    source_record_id    BIGINT NOT NULL REFERENCES source_record(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tvw_event_id, asset_id)
);
CREATE INDEX idx_tvw_media_asset_event_type ON tvw_media_asset (tvw_event_id, asset_type);
CREATE INDEX idx_tvw_media_asset_type ON tvw_media_asset (asset_type);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS tvw_media_asset;

ALTER TABLE tvw_event
    DROP COLUMN IF EXISTS raw_wp_categories,
    DROP COLUMN IF EXISTS raw_wp_tags,
    DROP COLUMN IF EXISTS raw_keywords,
    DROP COLUMN IF EXISTS streaming_uris,
    DROP COLUMN IF EXISTS video_download_url,
    DROP COLUMN IF EXISTS audio_download_url,
    DROP COLUMN IF EXISTS published_audio_url,
    DROP COLUMN IF EXISTS total_runtime_seconds,
    DROP COLUMN IF EXISTS total_runtime,
    DROP COLUMN IF EXISTS location_name,
    DROP COLUMN IF EXISTS custom_id,
    DROP COLUMN IF EXISTS wp_link,
    DROP COLUMN IF EXISTS wp_slug;

-- +goose StatementEnd
