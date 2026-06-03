-- +goose Up
-- +goose StatementBegin

-- Legislators are people with source-backed roster memberships, not a separate
-- identity namespace. Keep the historical `legislator` table for compatibility
-- during the transition, but move new ingest/query paths to person + membership.
CREATE TABLE legislator_roster_membership (
    id                  BIGSERIAL PRIMARY KEY,
    person_id           BIGINT NOT NULL REFERENCES person(id) ON DELETE CASCADE,
    biennium            TEXT NOT NULL,
    chamber             TEXT NOT NULL,
    district            TEXT,
    party               TEXT,
    lws_sponsor_id      TEXT NOT NULL,
    roster_name         TEXT NOT NULL,
    first_name          TEXT,
    last_name           TEXT,
    email               TEXT,
    phone               TEXT,
    acronym             TEXT,
    source_record_id    BIGINT REFERENCES source_record(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (biennium, lws_sponsor_id)
);

CREATE INDEX idx_legislator_roster_lws_sponsor_id
    ON legislator_roster_membership (lws_sponsor_id);
CREATE INDEX idx_legislator_roster_person
    ON legislator_roster_membership (person_id);
CREATE INDEX idx_legislator_roster_biennium_chamber_district
    ON legislator_roster_membership (biennium, chamber, district);

ALTER TABLE bill_sponsor
    DROP CONSTRAINT IF EXISTS bill_sponsor_pkey;

ALTER TABLE bill_sponsor
    ALTER COLUMN legislator_id DROP NOT NULL,
    ADD COLUMN person_id BIGINT REFERENCES person(id),
    ADD COLUMN legislator_roster_membership_id BIGINT REFERENCES legislator_roster_membership(id);

CREATE UNIQUE INDEX uniq_bill_sponsor_membership
    ON bill_sponsor (bill_id, legislator_roster_membership_id, sponsor_type)
 WHERE legislator_roster_membership_id IS NOT NULL;
CREATE UNIQUE INDEX uniq_bill_sponsor_legacy_legislator
    ON bill_sponsor (bill_id, legislator_id, sponsor_type)
 WHERE legislator_id IS NOT NULL;
CREATE UNIQUE INDEX uniq_bill_sponsor_legacy_conflict
    ON bill_sponsor (bill_id, legislator_id, sponsor_type);

CREATE INDEX idx_bill_sponsor_person
    ON bill_sponsor (person_id)
 WHERE person_id IS NOT NULL;
CREATE INDEX idx_bill_sponsor_roster_membership
    ON bill_sponsor (legislator_roster_membership_id)
 WHERE legislator_roster_membership_id IS NOT NULL;

ALTER TABLE person_source_mention
    DROP CONSTRAINT IF EXISTS person_source_mention_source_record_id_fkey,
    ADD CONSTRAINT person_source_mention_source_record_id_fkey
        FOREIGN KEY (source_record_id) REFERENCES source_record(id) ON DELETE SET NULL;

ALTER TABLE person_organization_affiliation
    DROP CONSTRAINT IF EXISTS person_organization_affiliation_source_record_id_fkey,
    ADD CONSTRAINT person_organization_affiliation_source_record_id_fkey
        FOREIGN KEY (source_record_id) REFERENCES source_record(id) ON DELETE SET NULL;

-- Preserve current local data for environments that already have the old
-- legislator table populated. This is migration-time convenience only; normal
-- operation is handled by ingest-legislators.
WITH legacy_people AS (
    INSERT INTO person (display_name, first_name, last_name, normalized_name,
                        match_confidence, match_notes)
    SELECT l.name,
           l.first_name,
           l.last_name,
           wa_dd_normalize_entity_name(COALESCE(NULLIF(trim(l.first_name || ' ' || l.last_name), ''), l.name)),
           'possible'::person_match_confidence,
           'Backfilled from legacy legislator row during roster-membership migration.'
      FROM legislator l
     WHERE l.first_name IS NOT NULL
       AND NOT EXISTS (
           SELECT 1 FROM person p
            WHERE p.display_name = l.name
              AND COALESCE(p.first_name, '') = COALESCE(l.first_name, '')
              AND COALESCE(p.last_name, '') = COALESCE(l.last_name, '')
       )
    RETURNING id, display_name, first_name, last_name
), all_legacy_people AS (
    SELECT p.id, p.display_name, p.first_name, p.last_name
      FROM person p
), memberships AS (
    INSERT INTO legislator_roster_membership (
        person_id, biennium, chamber, district, party, lws_sponsor_id,
        roster_name, first_name, last_name, email, phone, acronym
    )
    SELECT DISTINCT ON (l.lws_sponsor_id)
           p.id,
           '2025-26',
           COALESCE(l.chamber, ''),
           l.district,
           l.party,
           l.lws_sponsor_id,
           l.name,
           l.first_name,
           l.last_name,
           l.email,
           l.phone,
           l.acronym
      FROM legislator l
      JOIN all_legacy_people p
        ON p.display_name = l.name
       AND COALESCE(p.first_name, '') = COALESCE(l.first_name, '')
       AND COALESCE(p.last_name, '') = COALESCE(l.last_name, '')
     WHERE l.lws_sponsor_id IS NOT NULL
       AND l.first_name IS NOT NULL
    ON CONFLICT (biennium, lws_sponsor_id) DO NOTHING
    RETURNING id, person_id, lws_sponsor_id
)
UPDATE bill_sponsor bs
   SET person_id = lrm.person_id,
       legislator_roster_membership_id = lrm.id
  FROM legislator old_l
  JOIN legislator_roster_membership lrm ON lrm.lws_sponsor_id = old_l.lws_sponsor_id
 WHERE bs.legislator_id = old_l.id
   AND bs.person_id IS NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE person_organization_affiliation
    DROP CONSTRAINT IF EXISTS person_organization_affiliation_source_record_id_fkey,
    ADD CONSTRAINT person_organization_affiliation_source_record_id_fkey
        FOREIGN KEY (source_record_id) REFERENCES source_record(id);

ALTER TABLE person_source_mention
    DROP CONSTRAINT IF EXISTS person_source_mention_source_record_id_fkey,
    ADD CONSTRAINT person_source_mention_source_record_id_fkey
        FOREIGN KEY (source_record_id) REFERENCES source_record(id);

DROP INDEX IF EXISTS uniq_bill_sponsor_legacy_conflict;
DROP INDEX IF EXISTS uniq_bill_sponsor_legacy_legislator;
DROP INDEX IF EXISTS uniq_bill_sponsor_membership;

ALTER TABLE bill_sponsor
    DROP COLUMN IF EXISTS legislator_roster_membership_id,
    DROP COLUMN IF EXISTS person_id;

ALTER TABLE bill_sponsor
    ALTER COLUMN legislator_id SET NOT NULL,
    ADD CONSTRAINT bill_sponsor_pkey PRIMARY KEY (bill_id, legislator_id, sponsor_type);

DROP TABLE IF EXISTS legislator_roster_membership;

-- +goose StatementEnd
