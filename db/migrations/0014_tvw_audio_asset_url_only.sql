-- +goose Up
-- +goose StatementBegin

-- Diarization normally sends the TVW/Invintus downloadable audio URL directly
-- to the provider. Keep tvw_audio_asset as the job's media-source record even
-- when no local download/normalization has been performed.

ALTER TABLE tvw_audio_asset
    ALTER COLUMN original_path DROP NOT NULL,
    ALTER COLUMN normalized_path DROP NOT NULL,
    ALTER COLUMN content_hash DROP NOT NULL;

ALTER TABLE tvw_audio_asset
    DROP CONSTRAINT IF EXISTS tvw_audio_asset_tvw_event_id_content_hash_key;

CREATE UNIQUE INDEX IF NOT EXISTS uniq_tvw_audio_asset_event_source_url
    ON tvw_audio_asset (tvw_event_id, source_url);

CREATE INDEX IF NOT EXISTS idx_tvw_audio_asset_event_content_hash
    ON tvw_audio_asset (tvw_event_id, content_hash)
    WHERE content_hash IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_tvw_audio_asset_event_content_hash;
DROP INDEX IF EXISTS uniq_tvw_audio_asset_event_source_url;

DELETE FROM tvw_audio_asset
 WHERE original_path IS NULL
    OR normalized_path IS NULL
    OR content_hash IS NULL;

ALTER TABLE tvw_audio_asset
    ALTER COLUMN original_path SET NOT NULL,
    ALTER COLUMN normalized_path SET NOT NULL,
    ALTER COLUMN content_hash SET NOT NULL;

ALTER TABLE tvw_audio_asset
    ADD CONSTRAINT tvw_audio_asset_tvw_event_id_content_hash_key
    UNIQUE (tvw_event_id, content_hash);

-- +goose StatementEnd
