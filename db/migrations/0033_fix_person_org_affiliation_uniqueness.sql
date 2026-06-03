-- +goose Up
-- +goose StatementBegin

-- The original affiliation uniqueness constraint included nullable columns.
-- PostgreSQL's default unique constraints treat NULL values as distinct, which
-- allowed duplicate source-backed rows while organization_id was still NULL.
-- Those rows later collided when organization_id was attached to a real org.

DELETE FROM person_organization_affiliation poa
USING (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY relationship_type, source_kind, source_table,
                            source_pk, source_row_id, person_id,
                            organization_id, raw_person_name,
                            raw_organization_name
               ORDER BY id
           ) AS rn
      FROM person_organization_affiliation
) ranked
WHERE poa.id = ranked.id
  AND ranked.rn > 1;

ALTER TABLE person_organization_affiliation
    DROP CONSTRAINT IF EXISTS person_organization_affiliati_relationship_type_source_kind_key;

CREATE UNIQUE INDEX IF NOT EXISTS uniq_person_org_affiliation_source_identity
    ON person_organization_affiliation (
        relationship_type, source_kind, source_table, source_pk, source_row_id,
        person_id, organization_id, raw_person_name, raw_organization_name
    )
    NULLS NOT DISTINCT;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS uniq_person_org_affiliation_source_identity;

ALTER TABLE person_organization_affiliation
    ADD CONSTRAINT person_organization_affiliati_relationship_type_source_kind_key
    UNIQUE (relationship_type, source_kind, source_table, source_pk, source_row_id, person_id, organization_id, raw_person_name, raw_organization_name);

-- +goose StatementEnd
