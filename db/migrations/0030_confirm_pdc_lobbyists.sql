-- +goose Up
-- +goose StatementBegin

UPDATE person
   SET match_confidence = 'confirmed',
       match_notes = 'Auto-seeded from PDC lobbyist_id; PDC source identity is authoritative for lobbyist status.',
       updated_at = NOW()
 WHERE pdc_lobbyist_id IS NOT NULL
   AND match_confidence <> 'confirmed';

UPDATE person_organization_affiliation
   SET confidence = 'confirmed',
       updated_at = NOW()
 WHERE source_kind = 'pdc_lobbyist_employment'
   AND relationship_type = 'lobbyist_for'
   AND confidence <> 'confirmed';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

UPDATE person
   SET match_confidence = 'probable',
       match_notes = 'Auto-seeded from PDC lobbyist_id; PDC source identity is stable within PDC.',
       updated_at = NOW()
 WHERE pdc_lobbyist_id IS NOT NULL
   AND match_confidence = 'confirmed'
   AND match_notes = 'Auto-seeded from PDC lobbyist_id; PDC source identity is authoritative for lobbyist status.';

UPDATE person_organization_affiliation
   SET confidence = 'probable',
       updated_at = NOW()
 WHERE source_kind = 'pdc_lobbyist_employment'
   AND relationship_type = 'lobbyist_for'
   AND confidence = 'confirmed';

-- +goose StatementEnd
