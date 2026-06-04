-- +goose Up
-- +goose StatementBegin

-- Table for PDC lobbyist compensation data (9nnw-c693).
-- This dataset exposes firm↔client relationships: it shows how much
-- lobbying firms are paid by their clients per filing period.
CREATE TABLE IF NOT EXISTS pdc_lobbyist_compensation (
    filer_id        TEXT NOT NULL,
    employer_id     TEXT NOT NULL,
    filing_period   TEXT NOT NULL,
    filer_name      TEXT NOT NULL,
    funding_source_id TEXT,
    funding_source  TEXT,
    employer_name   TEXT NOT NULL,
    compensation    DECIMAL(15,2),
    total_expenses  DECIMAL(15,2),
    net_total       DECIMAL(15,2),
    url             TEXT,
    raw             JSONB NOT NULL,
    fetched_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Natural key constraint
    CONSTRAINT pk_pdc_lobbyist_compensation 
        PRIMARY KEY (filer_id, employer_id, filing_period)
);

-- Indexes for efficient lookups
CREATE INDEX IF NOT EXISTS idx_pdc_lobbyist_compensation_filer_id 
    ON pdc_lobbyist_compensation (filer_id);
CREATE INDEX IF NOT EXISTS idx_pdc_lobbyist_compensation_employer_id 
    ON pdc_lobbyist_compensation (employer_id);
CREATE INDEX IF NOT EXISTS idx_pdc_lobbyist_compensation_filing_period 
    ON pdc_lobbyist_compensation (filing_period);

-- Add pdc_lobbyist_compensation to the person_source_kind enum
ALTER TYPE person_source_kind
    ADD VALUE IF NOT EXISTS 'pdc_lobbyist_compensation'
    AFTER 'pdc_lobbyist_employment';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Drop indexes
DROP INDEX IF EXISTS idx_pdc_lobbyist_compensation_filing_period;
DROP INDEX IF EXISTS idx_pdc_lobbyist_compensation_employer_id;
DROP INDEX IF EXISTS idx_pdc_lobbyist_compensation_filer_id;

-- Drop table
DROP TABLE IF EXISTS pdc_lobbyist_compensation;

-- Note: Cannot remove enum values in PostgreSQL, so we leave 
-- 'pdc_lobbyist_compensation' in person_source_kind

-- +goose StatementEnd