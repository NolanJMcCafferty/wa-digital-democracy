package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

type UpsertLegislatorParams struct {
	Biennium string

	LWSSponsorID string
	Name         string
	Chamber      string
	District     string
	Party        string
	OfficialURL  string

	// Optional roster fields populated by ingest-legislators (LWS
	// SponsorService). Bill ingestion does not call this upsert; sponsor
	// joins resolve against roster-owned membership rows instead.
	FirstName string
	LastName  string
	Email     string
	Phone     string
	Acronym   string

	SourceRecordID int64
}

func (s *Store) UpsertLegislator(ctx context.Context, p UpsertLegislatorParams) (int64, error) {
	return s.UpsertLegislatorRosterMembership(ctx, p)
}

// UpsertLegislatorRosterMembership stores a legislator as a canonical person
// plus a biennium-specific roster membership. The legacy legislator table is
// still mirrored for compatibility with old fixtures/tools, but new query paths
// use person + legislator_roster_membership.
func (s *Store) UpsertLegislatorRosterMembership(ctx context.Context, p UpsertLegislatorParams) (int64, error) {
	biennium := defaultStr(strings.TrimSpace(p.Biennium), "2025-26")
	legacyName := p.Name
	display := strings.TrimSpace(p.Name)
	if display == "" {
		display = strings.TrimSpace(p.FirstName + " " + p.LastName)
	}
	if display == "" {
		return 0, errors.New("upsert legislator roster membership: name required")
	}
	if legacyName == "" {
		legacyName = display
	}
	normalized := strings.TrimSpace(p.FirstName + " " + p.LastName)
	if normalized == "" {
		normalized = display
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	// Keep legacy table populated while downstream code/tests finish migrating.
	const legacyQ = `
INSERT INTO legislator (lws_sponsor_id, name, chamber, district, party, official_url,
                        first_name, last_name, email, phone, acronym)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT (lws_sponsor_id) DO UPDATE SET
  name = EXCLUDED.name,
  chamber = EXCLUDED.chamber,
  district = EXCLUDED.district,
  party = EXCLUDED.party,
  official_url = EXCLUDED.official_url,
  first_name = EXCLUDED.first_name,
  last_name = EXCLUDED.last_name,
  email = EXCLUDED.email,
  phone = EXCLUDED.phone,
  acronym = EXCLUDED.acronym,
  updated_at = NOW()
RETURNING id;`
	var legacyID int64
	if err := tx.QueryRow(ctx, legacyQ,
		strOrNull(p.LWSSponsorID), legacyName, strOrNull(p.Chamber),
		strOrNull(p.District), strOrNull(p.Party), strOrNull(p.OfficialURL),
		strOrNull(p.FirstName), strOrNull(p.LastName), strOrNull(p.Email),
		strOrNull(p.Phone), strOrNull(p.Acronym),
	).Scan(&legacyID); err != nil {
		return 0, fmt.Errorf("upsert legacy legislator mirror: %w", err)
	}

	const personQ = `
WITH existing_from_membership AS (
  SELECT person_id
    FROM legislator_roster_membership
   WHERE lws_sponsor_id = $1
   ORDER BY biennium DESC
   LIMIT 1
), existing_from_name AS (
  SELECT id AS person_id
    FROM person
   WHERE lower(trim(display_name)) = lower(trim($2))
     AND COALESCE(lower(trim(first_name)), '') = COALESCE(lower(trim($3)), '')
     AND COALESCE(lower(trim(last_name)), '') = COALESCE(lower(trim($4)), '')
   ORDER BY id
   LIMIT 1
), chosen AS (
  SELECT person_id FROM existing_from_membership
  UNION ALL
  SELECT person_id FROM existing_from_name
  LIMIT 1
), inserted AS (
  INSERT INTO person (display_name, first_name, last_name, normalized_name, match_confidence, match_notes)
  SELECT $2, $3, $4, wa_dd_normalize_entity_name($5), 'confirmed'::person_match_confidence,
         'Seeded from LWS legislator roster.'
   WHERE NOT EXISTS (SELECT 1 FROM chosen)
  RETURNING id
)
SELECT id FROM inserted
UNION ALL
SELECT person_id FROM chosen
LIMIT 1;`
	var personID int64
	if err := tx.QueryRow(ctx, personQ,
		p.LWSSponsorID, display, strOrNull(p.FirstName), strOrNull(p.LastName), normalized,
	).Scan(&personID); err != nil {
		return 0, fmt.Errorf("upsert legislator person: %w", err)
	}

	const updatePersonQ = `
UPDATE person
   SET first_name = COALESCE($2, first_name),
       last_name = COALESCE($3, last_name),
       normalized_name = COALESCE(wa_dd_normalize_entity_name($4), normalized_name),
       match_confidence = CASE
         WHEN match_confidence IN ('possible', 'unmatched') THEN 'confirmed'::person_match_confidence
         ELSE match_confidence
       END,
       updated_at = NOW()
 WHERE id = $1;`
	if _, err := tx.Exec(ctx, updatePersonQ, personID, strOrNull(p.FirstName), strOrNull(p.LastName), normalized); err != nil {
		return 0, fmt.Errorf("update legislator person: %w", err)
	}

	contextJSON := fmt.Sprintf(`{"biennium":%q,"chamber":%q,"district":%q,"party":%q,"lws_sponsor_id":%q}`,
		biennium, p.Chamber, p.District, p.Party, p.LWSSponsorID)
	const mentionQ = `
INSERT INTO person_source_mention (
  person_id, source_kind, source_table, source_row_id, source_name, normalized_name,
  source_role, context, confidence, review_status, source_record_id
)
VALUES ($1, 'legislator_roster', 'legislator_roster_membership', $2, $3,
        wa_dd_normalize_entity_name($4), $5, $6::jsonb, 'confirmed', 'auto', $7)
ON CONFLICT (source_kind, source_table, source_pk, source_row_id, source_name) DO UPDATE SET
  person_id = COALESCE(person_source_mention.person_id, EXCLUDED.person_id),
  normalized_name = COALESCE(person_source_mention.normalized_name, EXCLUDED.normalized_name),
  source_role = EXCLUDED.source_role,
  context = EXCLUDED.context,
  confidence = EXCLUDED.confidence,
  source_record_id = COALESCE(EXCLUDED.source_record_id, person_source_mention.source_record_id),
  last_seen_at = NOW();`
	if _, err := tx.Exec(ctx, mentionQ,
		personID, biennium+":"+p.LWSSponsorID, display, normalized,
		legislatorRoleLabel(p.Chamber), contextJSON, int64OrNull(p.SourceRecordID),
	); err != nil {
		return 0, fmt.Errorf("upsert legislator person mention: %w", err)
	}

	const membershipQ = `
INSERT INTO legislator_roster_membership (
  person_id, biennium, chamber, district, party, lws_sponsor_id,
  roster_name, first_name, last_name, email, phone, acronym, source_record_id
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
ON CONFLICT (biennium, lws_sponsor_id) DO UPDATE SET
  person_id = EXCLUDED.person_id,
  chamber = EXCLUDED.chamber,
  district = COALESCE(EXCLUDED.district, legislator_roster_membership.district),
  party = COALESCE(EXCLUDED.party, legislator_roster_membership.party),
  roster_name = EXCLUDED.roster_name,
  first_name = COALESCE(EXCLUDED.first_name, legislator_roster_membership.first_name),
  last_name = COALESCE(EXCLUDED.last_name, legislator_roster_membership.last_name),
  email = COALESCE(EXCLUDED.email, legislator_roster_membership.email),
  phone = COALESCE(EXCLUDED.phone, legislator_roster_membership.phone),
  acronym = COALESCE(EXCLUDED.acronym, legislator_roster_membership.acronym),
  source_record_id = COALESCE(EXCLUDED.source_record_id, legislator_roster_membership.source_record_id),
  updated_at = NOW()
RETURNING id;`
	var membershipID int64
	if err := tx.QueryRow(ctx, membershipQ,
		personID, biennium, p.Chamber, strOrNull(p.District), strOrNull(p.Party),
		p.LWSSponsorID, display, strOrNull(p.FirstName), strOrNull(p.LastName),
		strOrNull(p.Email), strOrNull(p.Phone), strOrNull(p.Acronym), int64OrNull(p.SourceRecordID),
	).Scan(&membershipID); err != nil {
		return 0, fmt.Errorf("upsert legislator roster membership: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return membershipID, nil
}

func (s *Store) FindLegislatorIDByLWSSponsorID(ctx context.Context, lwsSponsorID string) (int64, bool, error) {
	const q = `SELECT id FROM legislator WHERE lws_sponsor_id = $1;`
	var id int64
	err := s.Pool.QueryRow(ctx, q, lwsSponsorID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("find legislator by lws sponsor id: %w", err)
	}
	return id, true, nil
}

func (s *Store) FindLegislatorRosterMembershipByLWSSponsorID(ctx context.Context, biennium, lwsSponsorID string) (int64, int64, bool, error) {
	const q = `
SELECT id, person_id
  FROM legislator_roster_membership
 WHERE biennium = $1 AND lws_sponsor_id = $2;`
	var membershipID, personID int64
	err := s.Pool.QueryRow(ctx, q, biennium, lwsSponsorID).Scan(&membershipID, &personID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, fmt.Errorf("find legislator roster membership by LWS sponsor id: %w", err)
	}
	return membershipID, personID, true, nil
}

// LegislatorAggregate is the row shape ListLegislators returns. It represents
// a person plus the currently selected roster membership.
type LegislatorAggregate struct {
	ID           int64 // person_id
	MembershipID int64
	LWSSponsorID string
	Name         string // roster name, e.g. "Senator Alvarado"
	FirstName    string
	LastName     string
	Chamber      string
	District     string
	Party        string
	Email        string
	Phone        string
	OfficialURL  string
	BillCount    int
}

// ListLegislators returns currently-seated members of the WA legislature for
// the latest ingested biennium with their sponsored-bill counts.
func (s *Store) ListLegislators(ctx context.Context) ([]LegislatorAggregate, error) {
	const q = `
WITH active_biennium AS (
  SELECT COALESCE(MAX(biennium), '2025-26') AS biennium FROM legislator_roster_membership
), ranked AS (
  SELECT lrm.*,
         ROW_NUMBER() OVER (
           PARTITION BY lrm.chamber, lrm.district
           ORDER BY lrm.lws_sponsor_id::int DESC
         ) AS rn
    FROM legislator_roster_membership lrm
    JOIN active_biennium ab ON ab.biennium = lrm.biennium
   WHERE lrm.first_name IS NOT NULL
     AND lrm.chamber IN ('House', 'Senate')
     AND lrm.district IS NOT NULL
)
SELECT p.id, r.id, COALESCE(r.lws_sponsor_id, ''), r.roster_name,
       COALESCE(r.first_name, p.first_name, ''), COALESCE(r.last_name, p.last_name, ''),
       COALESCE(r.chamber, ''), COALESCE(r.district, ''), COALESCE(r.party, ''),
       COALESCE(r.email, ''), COALESCE(r.phone, ''), '',
       COUNT(DISTINCT bs.bill_id)
  FROM ranked r
  JOIN person p ON p.id = r.person_id
  LEFT JOIN bill_sponsor bs ON bs.legislator_roster_membership_id = r.id
 WHERE (r.chamber = 'House'  AND r.rn <= 2)
    OR (r.chamber = 'Senate' AND r.rn <= 1)
 GROUP BY p.id, r.id, r.lws_sponsor_id, r.roster_name, r.first_name, r.last_name,
          p.first_name, p.last_name, r.chamber, r.district, r.party, r.email, r.phone
 ORDER BY COALESCE(r.last_name, p.last_name), COALESCE(r.first_name, p.first_name);`
	rows, err := s.Pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list legislators: %w", err)
	}
	defer rows.Close()
	out := []LegislatorAggregate{}
	for rows.Next() {
		var l LegislatorAggregate
		if err := rows.Scan(&l.ID, &l.MembershipID, &l.LWSSponsorID, &l.Name, &l.FirstName, &l.LastName,
			&l.Chamber, &l.District, &l.Party, &l.Email, &l.Phone,
			&l.OfficialURL, &l.BillCount); err != nil {
			return nil, fmt.Errorf("scan legislator: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ListLegislatorsByDistrict returns active roster rows known locally for one
// Washington legislative district.
func (s *Store) ListLegislatorsByDistrict(ctx context.Context, district string) ([]LegislatorAggregate, error) {
	const q = `
WITH active_biennium AS (
  SELECT COALESCE(MAX(biennium), '2025-26') AS biennium FROM legislator_roster_membership
), ranked AS (
  SELECT lrm.*,
         ROW_NUMBER() OVER (
           PARTITION BY lrm.chamber, lrm.district
           ORDER BY lrm.lws_sponsor_id::int DESC
         ) AS rn
    FROM legislator_roster_membership lrm
    JOIN active_biennium ab ON ab.biennium = lrm.biennium
   WHERE regexp_replace(COALESCE(lrm.district, ''), '\D', '', 'g') = $1
     AND lrm.first_name IS NOT NULL
)
SELECT p.id, r.id, COALESCE(r.lws_sponsor_id, ''), r.roster_name,
       COALESCE(r.first_name, p.first_name, ''), COALESCE(r.last_name, p.last_name, ''),
       COALESCE(r.chamber, ''), COALESCE(r.district, ''), COALESCE(r.party, ''),
       COALESCE(r.email, ''), COALESCE(r.phone, ''), '',
       COUNT(DISTINCT bs.bill_id)
  FROM ranked r
  JOIN person p ON p.id = r.person_id
  LEFT JOIN bill_sponsor bs ON bs.legislator_roster_membership_id = r.id
 WHERE (r.chamber = 'House'  AND r.rn <= 2)
    OR (r.chamber = 'Senate' AND r.rn <= 1)
 GROUP BY p.id, r.id, r.lws_sponsor_id, r.roster_name, r.first_name, r.last_name,
          p.first_name, p.last_name, r.chamber, r.district, r.party, r.email, r.phone
 ORDER BY CASE r.chamber WHEN 'Senate' THEN 0 WHEN 'House' THEN 1 ELSE 2 END,
          COUNT(DISTINCT bs.bill_id) DESC,
          COALESCE(r.last_name, p.last_name), COALESCE(r.first_name, p.first_name);`
	rows, err := s.Pool.Query(ctx, q, district)
	if err != nil {
		return nil, fmt.Errorf("list legislators by district: %w", err)
	}
	defer rows.Close()
	out := []LegislatorAggregate{}
	for rows.Next() {
		var l LegislatorAggregate
		if err := rows.Scan(&l.ID, &l.MembershipID, &l.LWSSponsorID, &l.Name, &l.FirstName, &l.LastName,
			&l.Chamber, &l.District, &l.Party, &l.Email, &l.Phone,
			&l.OfficialURL, &l.BillCount); err != nil {
			return nil, fmt.Errorf("scan legislator: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// LegislatorAppearance is one sponsored-bill row joined onto bill metadata.
type LegislatorAppearance struct {
	Biennium      string
	BillID        string
	BillPrefix    string
	BillNumber    int
	BillTitle     string
	SponsorType   string
	ChamberOrigin string
	CurrentStatus string
	LeadSponsor   string
	LeadFirstName string
	LeadLastName  string
	LeadParty     string
}

// GetLegislatorBills returns every bill the person has sponsored, joined to
// bill metadata for display.
func (s *Store) GetLegislatorBills(ctx context.Context, personID int64) ([]LegislatorAppearance, error) {
	const q = `
SELECT b.biennium, b.bill_number, b.prefix, b.number,
       COALESCE(b.title, ''), bs.sponsor_type,
       COALESCE(b.chamber_origin, ''),
       COALESCE(b.current_status, ''),
       COALESCE(primary_sponsor.name, ''),
       COALESCE(primary_sponsor.first_name, ''),
       COALESCE(primary_sponsor.last_name, ''),
       COALESCE(primary_sponsor.party, '')
  FROM bill_sponsor bs
  JOIN bill b ON b.id = bs.bill_id
  LEFT JOIN LATERAL (
    SELECT lrm.roster_name AS name, COALESCE(lrm.first_name, p.first_name, '') AS first_name,
           COALESCE(lrm.last_name, p.last_name, '') AS last_name,
           COALESCE(lrm.party, '') AS party
      FROM bill_sponsor primary_bs
      JOIN legislator_roster_membership lrm ON lrm.id = primary_bs.legislator_roster_membership_id
      JOIN person p ON p.id = lrm.person_id
     WHERE primary_bs.bill_id = b.id AND primary_bs.sponsor_type = 'Primary'
     ORDER BY lrm.id
     LIMIT 1
  ) primary_sponsor ON TRUE
 WHERE bs.person_id = $1
 ORDER BY b.biennium DESC, b.prefix, b.number;`
	rows, err := s.Pool.Query(ctx, q, personID)
	if err != nil {
		return nil, fmt.Errorf("legislator bills: %w", err)
	}
	defer rows.Close()
	out := []LegislatorAppearance{}
	for rows.Next() {
		var a LegislatorAppearance
		if err := rows.Scan(&a.Biennium, &a.BillID, &a.BillPrefix, &a.BillNumber,
			&a.BillTitle, &a.SponsorType, &a.ChamberOrigin, &a.CurrentStatus,
			&a.LeadSponsor, &a.LeadFirstName, &a.LeadLastName, &a.LeadParty); err != nil {
			return nil, fmt.Errorf("scan appearance: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func int64OrNull(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func legislatorRoleLabel(chamber string) string {
	switch chamber {
	case "Senate":
		return "State Senator"
	case "House":
		return "State Representative"
	default:
		return "Legislator"
	}
}
