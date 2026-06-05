-- +goose Up
ALTER TYPE speaker_evidence_type ADD VALUE IF NOT EXISTS 'llm_candidate';

-- +goose Down
-- Note: Postgres does not support removing enum values without recreating the type.
-- Since this is additive and the system is designed to be forwards-compatible,
-- we leave the enum value in place on rollback to avoid data loss.