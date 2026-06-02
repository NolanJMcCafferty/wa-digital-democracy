-- +goose Up
-- +goose StatementBegin

-- Canonical person/entity layer. A person row is an identity assertion; raw
-- names from source systems should first be preserved in person_source_mention
-- and only linked to person when the match is source-backed or reviewed.
CREATE TYPE person_match_confidence AS ENUM (
    'confirmed',
    'probable',
    'possible',
    'unmatched'
);

CREATE TYPE person_review_status AS ENUM (
    'confirmed',
    'rejected',
    'needs_review',
    'auto'
);

CREATE TYPE person_source_kind AS ENUM (
    'csi_testifier',
    'pdc_lobbyist_employment',
    'pdc_lobbyist_compensation',
    'pdc_contribution',
    'webs_vendor_contact',
    'deepgram_speaker',
    'legislator_roster',
    'manual_import'
);

CREATE TYPE person_org_affiliation_type AS ENUM (
    'signed_in_for',
    'testified_for',
    'lobbyist_for',
    'lobbying_firm_for',
    'paid_lobbying_for',
    'employed_by',
    'vendor_contact_for',
    'campaign_contributor_affiliation',
    'spoke_for_org',
    'reviewed_manual'
);

CREATE TABLE person (
    id                  BIGSERIAL PRIMARY KEY,
    display_name        TEXT NOT NULL,
    first_name          TEXT,
    last_name           TEXT,
    aliases             TEXT[] NOT NULL DEFAULT '{}',
    normalized_name     TEXT,
    match_confidence    person_match_confidence NOT NULL DEFAULT 'possible',
    match_notes         TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (display_name)
);
CREATE INDEX idx_person_normalized_name ON person (normalized_name) WHERE normalized_name IS NOT NULL;
CREATE INDEX idx_person_display_name_trgm ON person USING gin (display_name gin_trgm_ops);

-- Raw source-backed occurrences of person-like names. Mentions preserve
-- provenance and are not, by themselves, proof that two names are the same
-- person. source_pk may be NULL for non-integer source rows; source_row_id
-- preserves external/string IDs.
CREATE TABLE person_source_mention (
    id                  BIGSERIAL PRIMARY KEY,
    person_id           BIGINT REFERENCES person(id) ON DELETE SET NULL,
    source_kind         person_source_kind NOT NULL,
    source_table        TEXT NOT NULL,
    source_pk           BIGINT,
    source_row_id       TEXT,
    source_name         TEXT NOT NULL,
    normalized_name     TEXT,
    source_role         TEXT,
    context             JSONB NOT NULL DEFAULT '{}',
    confidence          person_match_confidence NOT NULL DEFAULT 'possible',
    review_status       person_review_status NOT NULL DEFAULT 'auto',
    source_record_id    BIGINT REFERENCES source_record(id),
    first_seen_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_kind, source_table, source_pk, source_row_id, source_name)
);
CREATE INDEX idx_person_source_mention_person ON person_source_mention (person_id);
CREATE INDEX idx_person_source_mention_normalized ON person_source_mention (normalized_name) WHERE normalized_name IS NOT NULL;
CREATE INDEX idx_person_source_mention_kind ON person_source_mention (source_kind, source_table);
CREATE INDEX idx_person_source_mention_source_record ON person_source_mention (source_record_id) WHERE source_record_id IS NOT NULL;

-- Source-backed relationships between people and organizations. This table is
-- deliberately relationship-typed rather than a generic "members" list: a
-- person can testify for, lobby for, be paid by, or be listed as a public
-- contact for an organization, and those claims have different meanings.
CREATE TABLE person_organization_affiliation (
    id                  BIGSERIAL PRIMARY KEY,
    person_id           BIGINT REFERENCES person(id) ON DELETE SET NULL,
    person_mention_id   BIGINT REFERENCES person_source_mention(id) ON DELETE SET NULL,
    organization_id     BIGINT REFERENCES organization(id) ON DELETE SET NULL,
    raw_person_name     TEXT,
    raw_organization_name TEXT,
    relationship_type   person_org_affiliation_type NOT NULL,
    role_title          TEXT,
    start_date          DATE,
    end_date            DATE,
    record_year         INT,
    source_kind         person_source_kind NOT NULL,
    source_table        TEXT NOT NULL,
    source_pk           BIGINT,
    source_row_id       TEXT,
    source_record_id    BIGINT REFERENCES source_record(id),
    context             JSONB NOT NULL DEFAULT '{}',
    confidence          person_match_confidence NOT NULL DEFAULT 'possible',
    review_status       person_review_status NOT NULL DEFAULT 'auto',
    evidence            JSONB NOT NULL DEFAULT '[]',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (person_id IS NOT NULL OR person_mention_id IS NOT NULL OR raw_person_name IS NOT NULL),
    CHECK (organization_id IS NOT NULL OR raw_organization_name IS NOT NULL),
    UNIQUE (relationship_type, source_kind, source_table, source_pk, source_row_id, person_id, organization_id, raw_person_name, raw_organization_name)
);
CREATE INDEX idx_person_org_affiliation_person ON person_organization_affiliation (person_id, relationship_type);
CREATE INDEX idx_person_org_affiliation_org ON person_organization_affiliation (organization_id, relationship_type);
CREATE INDEX idx_person_org_affiliation_mention ON person_organization_affiliation (person_mention_id) WHERE person_mention_id IS NOT NULL;
CREATE INDEX idx_person_org_affiliation_source ON person_organization_affiliation (source_kind, source_table, source_pk);
CREATE INDEX idx_person_org_affiliation_record_year ON person_organization_affiliation (record_year) WHERE record_year IS NOT NULL;
CREATE INDEX idx_person_org_affiliation_source_record ON person_organization_affiliation (source_record_id) WHERE source_record_id IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_person_org_affiliation_source_record;
DROP INDEX IF EXISTS idx_person_org_affiliation_record_year;
DROP INDEX IF EXISTS idx_person_org_affiliation_source;
DROP INDEX IF EXISTS idx_person_org_affiliation_mention;
DROP INDEX IF EXISTS idx_person_org_affiliation_org;
DROP INDEX IF EXISTS idx_person_org_affiliation_person;
DROP TABLE IF EXISTS person_organization_affiliation;

DROP INDEX IF EXISTS idx_person_source_mention_source_record;
DROP INDEX IF EXISTS idx_person_source_mention_kind;
DROP INDEX IF EXISTS idx_person_source_mention_normalized;
DROP INDEX IF EXISTS idx_person_source_mention_person;
DROP TABLE IF EXISTS person_source_mention;

DROP INDEX IF EXISTS idx_person_display_name_trgm;
DROP INDEX IF EXISTS idx_person_normalized_name;
DROP TABLE IF EXISTS person;

DROP TYPE IF EXISTS person_org_affiliation_type;
DROP TYPE IF EXISTS person_source_kind;
DROP TYPE IF EXISTS person_review_status;
DROP TYPE IF EXISTS person_match_confidence;

-- +goose StatementEnd
