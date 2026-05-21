-- +goose Up
-- +goose StatementBegin

CREATE EXTENSION IF NOT EXISTS postgis;

CREATE TABLE IF NOT EXISTS job_lock (
    name        TEXT PRIMARY KEY,
    owner       TEXT NOT NULL,
    acquired_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at  TIMESTAMPTZ
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS job_lock;
DROP EXTENSION IF EXISTS postgis;

-- +goose StatementEnd
