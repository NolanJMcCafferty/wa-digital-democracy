-- +goose Up
-- +goose StatementBegin

-- Initial schema for WA Digital Democracy first-page MVP.
-- Mirrors `Washington Digital Democracy - First Page Implementation Blueprint.md`
-- §"Minimal data model for first page" (lines 395–525).
--
-- Key architectural decisions enforced here:
--   - Postgres owns truth (Recommended Tech Stack §Key Decisions #1)
--   - Raw source records are immutable (#2): source_record is append-only;
--     normalized rows reference it via source_record_id.
--   - Every public fact needs provenance (#3): every normalized table carries
--     a NOT NULL source_record_id (except join tables and derived rows whose
--     parent already carries one).
--   - Confidence is a first-class field (#4): explicit enums.

CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS unaccent;

-- ---------------------------------------------------------------------------
-- Confidence enums
-- ---------------------------------------------------------------------------

CREATE TYPE speaker_confidence AS ENUM (
    'confirmed_legislator',
    'likely_legislator',
    'likely_testifier',
    'unknown_speaker',
    'ai_inferred_pending_review'
);

CREATE TYPE org_match_confidence AS ENUM (
    'confirmed',
    'probable',
    'possible',
    'unmatched'
);

CREATE TYPE testifier_position AS ENUM (
    'Pro',
    'Con',
    'Other',
    'Unknown'
);

-- ---------------------------------------------------------------------------
-- source_record — immutable provenance for every external fetch
-- ---------------------------------------------------------------------------

CREATE TABLE source_record (
    id              BIGSERIAL PRIMARY KEY,
    source_system   TEXT NOT NULL CHECK (source_system IN (
        'lws', 'committee_schedules', 'csi', 'tvw', 'invintus', 'pdc_socrata'
    )),
    source_endpoint TEXT NOT NULL,                 -- e.g. "LegislationService.GetLegislation"
    source_url      TEXT NOT NULL,                 -- canonical URL of the request
    source_id       TEXT,                          -- source-specific primary key, when known
    fetched_at      TIMESTAMPTZ NOT NULL,
    content_hash    TEXT NOT NULL,                 -- sha256 of raw response bytes
    raw_path        TEXT NOT NULL,                 -- relative path under data/raw/
    content_type    TEXT,
    transform_version TEXT NOT NULL DEFAULT 'v0'   -- bump when a parser changes meaning
);

CREATE INDEX idx_source_record_system_endpoint ON source_record (source_system, source_endpoint);
CREATE INDEX idx_source_record_source_id      ON source_record (source_system, source_id);
CREATE INDEX idx_source_record_fetched_at     ON source_record (fetched_at DESC);
-- Preserve distinct provenance rows for distinct logical requests even when
-- two endpoints/URLs return identical bytes. Raw object storage can dedupe by
-- content hash; source_record must remain source-link accurate.
CREATE UNIQUE INDEX uniq_source_record_request_hash
    ON source_record (source_system, source_endpoint, source_url, content_hash, transform_version);

-- ---------------------------------------------------------------------------
-- bill, legislator, bill_sponsor
-- ---------------------------------------------------------------------------

CREATE TABLE bill (
    id              BIGSERIAL PRIMARY KEY,
    biennium        TEXT NOT NULL,                 -- e.g. "2025-26"
    prefix          TEXT NOT NULL,                 -- "HB", "SB", "HJR", etc.
    number          INT  NOT NULL,
    bill_number     TEXT GENERATED ALWAYS AS (prefix || ' ' || number) STORED,
    title           TEXT,
    description     TEXT,
    chamber_origin  TEXT,
    current_status  TEXT,
    status_date     DATE,
    official_url    TEXT,
    source_record_id BIGINT NOT NULL REFERENCES source_record(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (biennium, prefix, number)
);
CREATE INDEX idx_bill_biennium ON bill (biennium);

CREATE TABLE legislator (
    id              BIGSERIAL PRIMARY KEY,
    lws_sponsor_id  TEXT,                          -- LWS member identifier when known
    name            TEXT NOT NULL,
    chamber         TEXT,                          -- "House" | "Senate"
    district        TEXT,
    party           TEXT,
    official_url    TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (lws_sponsor_id)
);
CREATE INDEX idx_legislator_name_trgm ON legislator USING gin (name gin_trgm_ops);

CREATE TABLE bill_sponsor (
    bill_id         BIGINT NOT NULL REFERENCES bill(id) ON DELETE CASCADE,
    legislator_id   BIGINT NOT NULL REFERENCES legislator(id),
    sponsor_type    TEXT NOT NULL,                 -- "Prime", "Cosponsor", "Requester"
    PRIMARY KEY (bill_id, legislator_id, sponsor_type)
);

-- ---------------------------------------------------------------------------
-- hearing, agenda_item, testifier
-- ---------------------------------------------------------------------------

CREATE TABLE hearing (
    id                            BIGSERIAL PRIMARY KEY,
    bill_id                       BIGINT REFERENCES bill(id),
    committee_name                TEXT NOT NULL,
    committee_acronym             TEXT,
    chamber                       TEXT NOT NULL,
    meeting_datetime              TIMESTAMPTZ NOT NULL,
    location                      TEXT,
    -- Source identifiers (Blueprint lines 446–454)
    lws_meeting_id                TEXT,
    committee_schedule_agenda_id  TEXT,
    committee_schedule_video_id   TEXT,
    tvw_event_id                  TEXT,
    official_agenda_url           TEXT,
    tvw_url                       TEXT,
    source_record_id              BIGINT NOT NULL REFERENCES source_record(id),
    created_at                    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_hearing_meeting_datetime ON hearing (meeting_datetime);
CREATE INDEX idx_hearing_tvw_event_id     ON hearing (tvw_event_id) WHERE tvw_event_id IS NOT NULL;
CREATE INDEX idx_hearing_lws_meeting_id   ON hearing (lws_meeting_id) WHERE lws_meeting_id IS NOT NULL;

CREATE TABLE agenda_item (
    id                          BIGSERIAL PRIMARY KEY,
    hearing_id                  BIGINT NOT NULL REFERENCES hearing(id) ON DELETE CASCADE,
    bill_id                     BIGINT REFERENCES bill(id),
    label                       TEXT NOT NULL,    -- e.g. "HB 2747 Budget sustainability"
    csi_meeting_family_id       TEXT,
    csi_agenda_item_family_id   TEXT,
    csi_agenda_item_id          TEXT,             -- primary key for CSI testifier lookup
    order_index                 INT,
    source_record_id            BIGINT NOT NULL REFERENCES source_record(id),
    UNIQUE (csi_agenda_item_id)
);
CREATE INDEX idx_agenda_item_hearing_bill ON agenda_item (hearing_id, bill_id);

-- testifier.normalized_org_id FK is added later, after the organization table
-- is created.
CREATE TABLE testifier (
    id                  BIGSERIAL PRIMARY KEY,
    agenda_item_id      BIGINT NOT NULL REFERENCES agenda_item(id) ON DELETE CASCADE,
    raw_name            TEXT NOT NULL,
    raw_organization    TEXT,
    normalized_person_id BIGINT,
    normalized_org_id   BIGINT,
    position            testifier_position NOT NULL DEFAULT 'Unknown',
    testified           BOOLEAN NOT NULL,         -- true => testifyingDataTable; false => notTestifying
    time_signed_in      TIMESTAMPTZ,
    source_record_id    BIGINT NOT NULL REFERENCES source_record(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_testifier_agenda_item   ON testifier (agenda_item_id);
CREATE INDEX idx_testifier_org_trgm      ON testifier USING gin (raw_organization gin_trgm_ops);

-- ---------------------------------------------------------------------------
-- TVW / Invintus
-- ---------------------------------------------------------------------------

CREATE TABLE tvw_event (
    id                  BIGSERIAL PRIMARY KEY,
    tvw_event_id        TEXT NOT NULL UNIQUE,
    wp_post_id          BIGINT,
    title               TEXT,
    description         TEXT,
    start_datetime      TIMESTAMPTZ,
    caption_url         TEXT,
    thumbnail_url       TEXT,
    raw_categories      JSONB,
    source_record_id    BIGINT NOT NULL REFERENCES source_record(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE transcript_segment (
    id                  BIGSERIAL PRIMARY KEY,
    tvw_event_id        TEXT NOT NULL REFERENCES tvw_event(tvw_event_id) ON DELETE CASCADE,
    agenda_item_id      BIGINT REFERENCES agenda_item(id),
    start_ms            INT NOT NULL,
    end_ms              INT NOT NULL,
    text                TEXT NOT NULL,
    speaker_label       TEXT,
    speaker_entity_id   BIGINT,                    -- references legislator OR testifier; not enforced
    speaker_confidence  speaker_confidence NOT NULL DEFAULT 'unknown_speaker',
    source_caption_url  TEXT NOT NULL,
    source_record_id    BIGINT NOT NULL REFERENCES source_record(id)
);
CREATE INDEX idx_transcript_segment_event_time ON transcript_segment (tvw_event_id, start_ms);
CREATE INDEX idx_transcript_segment_agenda     ON transcript_segment (agenda_item_id) WHERE agenda_item_id IS NOT NULL;
CREATE INDEX idx_transcript_segment_text_fts   ON transcript_segment USING gin (to_tsvector('english', text));

-- ---------------------------------------------------------------------------
-- Organization + PDC context
-- ---------------------------------------------------------------------------

CREATE TABLE organization (
    id                          BIGSERIAL PRIMARY KEY,
    canonical_name              TEXT NOT NULL,
    aliases                     TEXT[] NOT NULL DEFAULT '{}',
    pdc_lobbyist_employer_id    TEXT,
    pdc_committee_or_filer_id   TEXT,
    match_confidence            org_match_confidence NOT NULL DEFAULT 'unmatched',
    match_notes                 TEXT,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (canonical_name)
);
CREATE INDEX idx_organization_name_trgm ON organization USING gin (canonical_name gin_trgm_ops);

-- Re-add the testifier FK now that organization actually exists.
ALTER TABLE testifier
    ADD CONSTRAINT fk_testifier_organization
    FOREIGN KEY (normalized_org_id) REFERENCES organization(id);

CREATE TABLE org_context_record (
    id                  BIGSERIAL PRIMARY KEY,
    organization_id     BIGINT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
    context_type        TEXT NOT NULL CHECK (context_type IN (
        'lobbying_registration', 'lobbying_spend',
        'campaign_contribution', 'independent_expenditure'
    )),
    source_dataset_id   TEXT NOT NULL,             -- e.g. "9nnw-c693"
    source_row_id       TEXT,
    summary_fields      JSONB NOT NULL,            -- subset of dataset fields we display
    source_url          TEXT NOT NULL,
    match_confidence    org_match_confidence NOT NULL DEFAULT 'possible',
    source_record_id    BIGINT NOT NULL REFERENCES source_record(id)
);
CREATE INDEX idx_org_context_org_type ON org_context_record (organization_id, context_type);

-- ---------------------------------------------------------------------------
-- ingestion_runs — Go Backend Evaluation §"Workflow orchestration"
-- ---------------------------------------------------------------------------

CREATE TABLE ingestion_run (
    id              BIGSERIAL PRIMARY KEY,
    job             TEXT NOT NULL,                 -- "ingest-bill", "ingest-csi", etc.
    args            JSONB NOT NULL DEFAULT '{}',
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at     TIMESTAMPTZ,
    status          TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running','succeeded','failed')),
    rows_fetched    INT NOT NULL DEFAULT 0,
    rows_upserted   INT NOT NULL DEFAULT 0,
    error           TEXT
);
CREATE INDEX idx_ingestion_run_job_started ON ingestion_run (job, started_at DESC);

-- ---------------------------------------------------------------------------
-- Status timeline (denormalized for the page; rebuildable from source_record)
-- ---------------------------------------------------------------------------

CREATE TABLE bill_status_change (
    id              BIGSERIAL PRIMARY KEY,
    bill_id         BIGINT NOT NULL REFERENCES bill(id) ON DELETE CASCADE,
    action_date     DATE NOT NULL,
    history_line    TEXT NOT NULL,
    actor           TEXT,
    source_record_id BIGINT NOT NULL REFERENCES source_record(id),
    UNIQUE (bill_id, action_date, history_line)
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS bill_status_change;
DROP TABLE IF EXISTS ingestion_run;
DROP TABLE IF EXISTS org_context_record;
ALTER TABLE IF EXISTS testifier DROP CONSTRAINT IF EXISTS fk_testifier_organization;
DROP TABLE IF EXISTS organization;
DROP TABLE IF EXISTS transcript_segment;
DROP TABLE IF EXISTS tvw_event;
DROP TABLE IF EXISTS testifier;
DROP TABLE IF EXISTS agenda_item;
DROP TABLE IF EXISTS hearing;
DROP TABLE IF EXISTS bill_sponsor;
DROP TABLE IF EXISTS legislator;
DROP TABLE IF EXISTS bill;
DROP TABLE IF EXISTS source_record;
DROP TYPE IF EXISTS testifier_position;
DROP TYPE IF EXISTS org_match_confidence;
DROP TYPE IF EXISTS speaker_confidence;
-- +goose StatementEnd
