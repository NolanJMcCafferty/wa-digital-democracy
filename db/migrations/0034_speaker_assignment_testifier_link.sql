-- +goose Up
-- +goose StatementBegin

-- Add testifier_id foreign key to speaker_assignment to create a direct
-- link from reviewed speaker assignments to CSI testifiers, strengthening
-- the speaker -> testifier -> organization identity bridge.
ALTER TABLE speaker_assignment ADD COLUMN testifier_id BIGINT REFERENCES testifier(id);

-- Index for querying speaker assignments by testifier
CREATE INDEX idx_speaker_assignment_testifier ON speaker_assignment (testifier_id) WHERE testifier_id IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_speaker_assignment_testifier;
ALTER TABLE speaker_assignment DROP COLUMN IF EXISTS testifier_id;

-- +goose StatementEnd