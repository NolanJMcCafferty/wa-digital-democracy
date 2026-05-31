package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type UpsertLegislatorParams struct {
	LWSSponsorID string
	Name         string
	Chamber      string
	District     string
	Party        string
	OfficialURL  string

	// Optional roster fields populated by ingest-legislators (LWS
	// SponsorService). Bill ingestion does not call this upsert; sponsor
	// joins resolve against roster-owned legislator rows instead.
	FirstName string
	LastName  string
	Email     string
	Phone     string
	Acronym   string
}

func (s *Store) UpsertLegislator(ctx context.Context, p UpsertLegislatorParams) (int64, error) {
	const q = `
INSERT INTO legislator (lws_sponsor_id, name, chamber, district, party, official_url,
                        first_name, last_name, email, phone, acronym)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT (lws_sponsor_id) DO UPDATE SET
  name = EXCLUDED.name,
  chamber = EXCLUDED.chamber,
  district = COALESCE(EXCLUDED.district, legislator.district),
  party = COALESCE(EXCLUDED.party, legislator.party),
  official_url = COALESCE(EXCLUDED.official_url, legislator.official_url),
  first_name = COALESCE(EXCLUDED.first_name, legislator.first_name),
  last_name = COALESCE(EXCLUDED.last_name, legislator.last_name),
  email = COALESCE(EXCLUDED.email, legislator.email),
  phone = COALESCE(EXCLUDED.phone, legislator.phone),
  acronym = COALESCE(EXCLUDED.acronym, legislator.acronym),
  updated_at = NOW()
RETURNING id;`
	var id int64
	err := s.Pool.QueryRow(ctx, q,
		strOrNull(p.LWSSponsorID), p.Name, strOrNull(p.Chamber),
		strOrNull(p.District), strOrNull(p.Party), strOrNull(p.OfficialURL),
		strOrNull(p.FirstName), strOrNull(p.LastName), strOrNull(p.Email),
		strOrNull(p.Phone), strOrNull(p.Acronym),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert legislator: %w", err)
	}
	return id, nil
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

// LegislatorAggregate is the row shape ListLegislators returns. The
// roster fields (FirstName, LastName, District, Party) are populated by
// `wa-dd ingest-legislators` from LWS SponsorService; older rows
// created by IngestBill alone may have them empty.
type LegislatorAggregate struct {
	ID           int64
	LWSSponsorID string
	Name         string // "Senator Alvarado" — the LongName form, kept for back-compat
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

// ListLegislators returns currently-seated members of the WA legislature
// for the active biennium with their sponsored-bill counts.
//
// LWS's SponsorService returns every member who held a seat during the
// biennium, so districts with mid-biennium turnover (resignation,
// appointment, election to higher office) come back with multiple rows.
// We keep the latest N members per (chamber, district) — N=2 for House
// (two seats per district) and N=1 for Senate (one seat per district)
// — using lws_sponsor_id DESC as a proxy for "most recently seated"
// since LWS IDs increment monotonically per appointment date.
//
// Legacy rows missing first_name (orphan IDs from per-bill GetSponsors
// calls before the roster ingest existed) are filtered out — they have
// no renderable display name.
//
// Order: alphabetical by last name.
func (s *Store) ListLegislators(ctx context.Context) ([]LegislatorAggregate, error) {
	const q = `
WITH ranked AS (
  SELECT l.*,
         ROW_NUMBER() OVER (
           PARTITION BY l.chamber, l.district
           ORDER BY l.lws_sponsor_id::int DESC
         ) AS rn
    FROM legislator l
   WHERE l.first_name IS NOT NULL
     AND l.chamber IN ('House', 'Senate')
     AND l.district IS NOT NULL
)
SELECT r.id, COALESCE(r.lws_sponsor_id, ''), r.name,
       COALESCE(r.first_name, ''), COALESCE(r.last_name, ''),
       COALESCE(r.chamber, ''),
       COALESCE(r.district, ''), COALESCE(r.party, ''),
       COALESCE(r.email, ''), COALESCE(r.phone, ''), COALESCE(r.official_url, ''),
       COUNT(DISTINCT bs.bill_id)
  FROM ranked r
  LEFT JOIN bill_sponsor bs ON bs.legislator_id = r.id
 WHERE (r.chamber = 'House'  AND r.rn <= 2)
    OR (r.chamber = 'Senate' AND r.rn <= 1)
 GROUP BY r.id, r.lws_sponsor_id, r.name, r.first_name, r.last_name, r.chamber,
          r.district, r.party, r.email, r.phone, r.official_url
 ORDER BY r.last_name, r.first_name;`
	rows, err := s.Pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list legislators: %w", err)
	}
	defer rows.Close()
	out := []LegislatorAggregate{}
	for rows.Next() {
		var l LegislatorAggregate
		if err := rows.Scan(&l.ID, &l.LWSSponsorID, &l.Name, &l.FirstName, &l.LastName,
			&l.Chamber, &l.District, &l.Party, &l.Email, &l.Phone,
			&l.OfficialURL, &l.BillCount); err != nil {
			return nil, fmt.Errorf("scan legislator: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ListLegislatorsByDistrict returns the active roster rows known locally for
// one Washington legislative district. Districts are stored as text from LWS,
// so the query compares their numeric form.
func (s *Store) ListLegislatorsByDistrict(ctx context.Context, district string) ([]LegislatorAggregate, error) {
	const q = `
SELECT l.id, COALESCE(l.lws_sponsor_id, ''), l.name,
       COALESCE(l.first_name, ''), COALESCE(l.last_name, ''),
       COALESCE(l.chamber, ''),
       COALESCE(l.district, ''), COALESCE(l.party, ''),
       COALESCE(l.email, ''), COALESCE(l.phone, ''), COALESCE(l.official_url, ''),
       COUNT(DISTINCT bs.bill_id)
  FROM legislator l
  LEFT JOIN bill_sponsor bs ON bs.legislator_id = l.id
 WHERE regexp_replace(COALESCE(l.district, ''), '\D', '', 'g') = $1
   AND l.first_name IS NOT NULL
 GROUP BY l.id
 ORDER BY CASE l.chamber WHEN 'Senate' THEN 0 WHEN 'House' THEN 1 ELSE 2 END,
          COUNT(DISTINCT bs.bill_id) DESC,
          l.last_name, l.first_name;`
	rows, err := s.Pool.Query(ctx, q, district)
	if err != nil {
		return nil, fmt.Errorf("list legislators by district: %w", err)
	}
	defer rows.Close()
	out := []LegislatorAggregate{}
	for rows.Next() {
		var l LegislatorAggregate
		if err := rows.Scan(&l.ID, &l.LWSSponsorID, &l.Name, &l.FirstName, &l.LastName,
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

// GetLegislatorBills returns every bill the legislator has sponsored,
// joined to bill metadata for display.
func (s *Store) GetLegislatorBills(ctx context.Context, legislatorID int64) ([]LegislatorAppearance, error) {
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
    SELECT l.name, COALESCE(l.first_name, '') AS first_name,
           COALESCE(l.last_name, '') AS last_name,
           COALESCE(l.party, '') AS party
      FROM bill_sponsor primary_bs
      JOIN legislator l ON l.id = primary_bs.legislator_id
     WHERE primary_bs.bill_id = b.id AND primary_bs.sponsor_type = 'Primary'
     ORDER BY l.id
     LIMIT 1
  ) primary_sponsor ON TRUE
 WHERE bs.legislator_id = $1
 ORDER BY b.biennium DESC, b.prefix, b.number;`
	rows, err := s.Pool.Query(ctx, q, legislatorID)
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
