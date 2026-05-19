-- +goose Up
-- +goose StatementBegin

-- Reviewable speaker identity assignments. Diarization gives anonymous clusters;
-- this layer stores evidence and human decisions that link a cluster to a real
-- legislator, testifier, or manually-entered person label.

CREATE TYPE speaker_candidate_kind AS ENUM (
    'legislator',
    'testifier',
    'person',
    'unknown'
);

CREATE TYPE speaker_evidence_type AS ENUM (
    'self_introduction',
    'chair_call',
    'entity_mention',
    'csi_order',
    'manual_note'
);

CREATE TYPE speaker_review_status AS ENUM (
    'pending',
    'accepted',
    'rejected',
    'needs_more_evidence',
    'superseded'
);

CREATE TABLE speaker_identity_evidence (
    id                            BIGSERIAL PRIMARY KEY,
    evidence_key                  TEXT NOT NULL UNIQUE,
    diarization_job_id            BIGINT NOT NULL REFERENCES diarization_job(id) ON DELETE CASCADE,
    speaker_cluster_id            BIGINT NOT NULL REFERENCES speaker_cluster(id) ON DELETE CASCADE,
    diarized_speech_segment_id    BIGINT REFERENCES diarized_speech_segment(id) ON DELETE CASCADE,
    evidence_type                 speaker_evidence_type NOT NULL,
    evidence_text                 TEXT NOT NULL,
    candidate_kind                speaker_candidate_kind NOT NULL,
    candidate_id                  BIGINT,
    candidate_label               TEXT NOT NULL,
    confidence                    NUMERIC,
    start_ms                      INT,
    end_ms                        INT,
    raw                           JSONB NOT NULL DEFAULT '{}',
    created_at                    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (end_ms IS NULL OR start_ms IS NULL OR end_ms >= start_ms)
);
CREATE INDEX idx_speaker_identity_evidence_job_cluster ON speaker_identity_evidence (diarization_job_id, speaker_cluster_id);
CREATE INDEX idx_speaker_identity_evidence_candidate ON speaker_identity_evidence (candidate_kind, candidate_id);

CREATE TABLE speaker_review_task (
    id                          BIGSERIAL PRIMARY KEY,
    diarization_job_id          BIGINT NOT NULL REFERENCES diarization_job(id) ON DELETE CASCADE,
    speaker_cluster_id          BIGINT NOT NULL REFERENCES speaker_cluster(id) ON DELETE CASCADE,
    status                      speaker_review_status NOT NULL DEFAULT 'pending',
    priority                    INT NOT NULL DEFAULT 0,
    proposed_candidate_kind     speaker_candidate_kind NOT NULL,
    proposed_candidate_id       BIGINT,
    proposed_label              TEXT NOT NULL,
    proposed_confidence         NUMERIC,
    evidence_ids                BIGINT[] NOT NULL DEFAULT '{}',
    reviewer                    TEXT,
    review_notes                TEXT,
    reviewed_at                 TIMESTAMPTZ,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX uniq_speaker_review_task_candidate
    ON speaker_review_task (diarization_job_id, speaker_cluster_id, proposed_candidate_kind, COALESCE(proposed_candidate_id, 0), proposed_label);
CREATE INDEX idx_speaker_review_task_status ON speaker_review_task (status, priority DESC, created_at DESC);

CREATE TABLE speaker_assignment (
    id                    BIGSERIAL PRIMARY KEY,
    diarization_job_id    BIGINT NOT NULL REFERENCES diarization_job(id) ON DELETE CASCADE,
    speaker_cluster_id    BIGINT NOT NULL REFERENCES speaker_cluster(id) ON DELETE CASCADE,
    speaker_kind          speaker_candidate_kind NOT NULL,
    speaker_id            BIGINT,
    speaker_label         TEXT NOT NULL,
    confidence            NUMERIC,
    review_task_id        BIGINT REFERENCES speaker_review_task(id),
    review_status         speaker_review_status NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (diarization_job_id, speaker_cluster_id)
);
CREATE INDEX idx_speaker_assignment_label ON speaker_assignment USING gin (speaker_label gin_trgm_ops);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_speaker_assignment_label;
DROP TABLE IF EXISTS speaker_assignment;

DROP INDEX IF EXISTS idx_speaker_review_task_status;
DROP INDEX IF EXISTS uniq_speaker_review_task_candidate;
DROP TABLE IF EXISTS speaker_review_task;

DROP INDEX IF EXISTS idx_speaker_identity_evidence_candidate;
DROP INDEX IF EXISTS idx_speaker_identity_evidence_job_cluster;
DROP TABLE IF EXISTS speaker_identity_evidence;

DROP TYPE IF EXISTS speaker_review_status;
DROP TYPE IF EXISTS speaker_evidence_type;
DROP TYPE IF EXISTS speaker_candidate_kind;

-- +goose StatementEnd
