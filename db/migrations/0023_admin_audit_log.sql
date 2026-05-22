-- +goose Up
-- +goose StatementBegin

CREATE TABLE admin_audit_log (
    id              BIGSERIAL PRIMARY KEY,
    actor_user_id   TEXT NOT NULL,
    actor_email     TEXT NOT NULL,
    actor_name      TEXT,
    actor_role      TEXT NOT NULL,
    route           TEXT NOT NULL,
    action          TEXT NOT NULL,
    target_type     TEXT NOT NULL,
    target_id       TEXT NOT NULL,
    previous_state  JSONB,
    new_state       JSONB,
    reviewer_notes  TEXT,
    request_id      TEXT,
    ip_address      INET,
    user_agent      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_admin_audit_log_created_at ON admin_audit_log (created_at DESC);
CREATE INDEX idx_admin_audit_log_target ON admin_audit_log (target_type, target_id, created_at DESC);
CREATE INDEX idx_admin_audit_log_actor ON admin_audit_log (actor_email, created_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_admin_audit_log_actor;
DROP INDEX IF EXISTS idx_admin_audit_log_target;
DROP INDEX IF EXISTS idx_admin_audit_log_created_at;
DROP TABLE IF EXISTS admin_audit_log;

-- +goose StatementEnd
