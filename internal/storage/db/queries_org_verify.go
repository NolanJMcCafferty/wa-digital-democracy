package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

type UpsertIRSBMFParams struct {
	EIN               string
	Name              string
	NormalizedName    string
	SortName          string
	Street            string
	City              string
	State             string
	Zip               string
	SubsectionCode    string
	Classification    string
	DeductibilityCode string
	ActivityCodes     string
	FoundationCode    string
	OrganizationCode  string
	StatusCode        string
	RulingDate        string
	NTEECode          string
	IncomeAmount      int64
	RevenueAmount     int64
	AssetAmount       int64
	Raw               map[string]string
}

func (s *Store) UpsertIRSBMFOrganization(ctx context.Context, p UpsertIRSBMFParams) error {
	rawJSON, err := json.Marshal(p.Raw)
	if err != nil {
		return fmt.Errorf("marshal irs bmf raw: %w", err)
	}
	const q = `
INSERT INTO irs_bmf_organization (ein, name, normalized_name, sort_name,
    street, city, state, zip, subsection_code, classification, deductibility_code,
    activity_codes, foundation_code, organization_code, status_code, ruling_date,
    ntee_code, income_amount, revenue_amount, asset_amount, raw)
VALUES ($1,$2,COALESCE(wa_dd_normalize_entity_name($2),''),$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20::jsonb)
ON CONFLICT (ein) DO UPDATE SET
  name              = EXCLUDED.name,
  normalized_name   = EXCLUDED.normalized_name,
  sort_name         = EXCLUDED.sort_name,
  street            = EXCLUDED.street,
  city              = EXCLUDED.city,
  state             = EXCLUDED.state,
  zip               = EXCLUDED.zip,
  subsection_code   = EXCLUDED.subsection_code,
  classification    = EXCLUDED.classification,
  deductibility_code= EXCLUDED.deductibility_code,
  activity_codes    = EXCLUDED.activity_codes,
  foundation_code   = EXCLUDED.foundation_code,
  organization_code = EXCLUDED.organization_code,
  status_code       = EXCLUDED.status_code,
  ruling_date       = EXCLUDED.ruling_date,
  ntee_code         = EXCLUDED.ntee_code,
  income_amount     = EXCLUDED.income_amount,
  revenue_amount    = EXCLUDED.revenue_amount,
  asset_amount      = EXCLUDED.asset_amount,
  raw               = EXCLUDED.raw,
  fetched_at        = NOW();`
	_, err = s.Pool.Exec(ctx, q,
		p.EIN, p.Name, strOrNull(p.SortName),
		strOrNull(p.Street), strOrNull(p.City), strOrNull(p.State), strOrNull(p.Zip),
		strOrNull(p.SubsectionCode), strOrNull(p.Classification), strOrNull(p.DeductibilityCode),
		strOrNull(p.ActivityCodes), strOrNull(p.FoundationCode), strOrNull(p.OrganizationCode),
		strOrNull(p.StatusCode), strOrNull(p.RulingDate), strOrNull(p.NTEECode),
		p.IncomeAmount, p.RevenueAmount, p.AssetAmount,
		string(rawJSON),
	)
	if err != nil {
		return fmt.Errorf("upsert irs_bmf_organization: %w", err)
	}
	return nil
}

type UpsertPDCEmployerParams struct {
	EmployerID         string
	Name               string
	NormalizedName     string
	LastEmploymentYear string
	LastReportNumber   string
	LastEmploymentURL  string
	Raw                map[string]any
}

type UpsertPDCLobbyistAffiliationParams struct {
	ReportNumber     string
	LobbyistID       string
	LobbyistName     string
	EmployerID       string
	EmployerName     string
	EmploymentYear   string
	EmploymentURL    string
	EmploymentPeriod string
	Raw              map[string]any
}

func (s *Store) UpsertPDCEmployer(ctx context.Context, p UpsertPDCEmployerParams) error {
	rawJSON, err := json.Marshal(p.Raw)
	if err != nil {
		return fmt.Errorf("marshal pdc employer raw: %w", err)
	}
	const q = `
INSERT INTO pdc_employer (employer_id, name, normalized_name,
    last_employment_year, last_report_number, last_employment_url,
    raw, last_seen_at)
VALUES ($1,$2,COALESCE(wa_dd_normalize_entity_name($2),''),$3,$4,$5,$6::jsonb,NOW())
ON CONFLICT (employer_id) DO UPDATE SET
  name                = EXCLUDED.name,
  normalized_name     = EXCLUDED.normalized_name,
  last_employment_year = COALESCE(EXCLUDED.last_employment_year, pdc_employer.last_employment_year),
  last_report_number   = COALESCE(EXCLUDED.last_report_number, pdc_employer.last_report_number),
  last_employment_url  = COALESCE(EXCLUDED.last_employment_url, pdc_employer.last_employment_url),
  raw                  = EXCLUDED.raw,
  last_seen_at         = NOW();`
	_, err = s.Pool.Exec(ctx, q,
		p.EmployerID, p.Name,
		strOrNull(p.LastEmploymentYear), strOrNull(p.LastReportNumber), strOrNull(p.LastEmploymentURL),
		string(rawJSON),
	)
	if err != nil {
		return fmt.Errorf("upsert pdc_employer: %w", err)
	}
	return nil
}

func (s *Store) UpsertPDCLobbyistAffiliation(ctx context.Context, p UpsertPDCLobbyistAffiliationParams) error {
	lobbyistName := strings.TrimSpace(p.LobbyistName)
	if p.LobbyistID == "" || lobbyistName == "" || p.EmployerID == "" || p.EmployerName == "" {
		return nil
	}
	rawJSON, err := json.Marshal(p.Raw)
	if err != nil {
		return fmt.Errorf("marshal pdc lobbyist affiliation raw: %w", err)
	}
	contextJSON, err := json.Marshal(map[string]any{
		"report_number":     p.ReportNumber,
		"lobbyist_id":       p.LobbyistID,
		"lobbyist_name":     p.LobbyistName,
		"employer_id":       p.EmployerID,
		"employer_name":     p.EmployerName,
		"employment_year":   p.EmploymentYear,
		"employment_url":    p.EmploymentURL,
		"employment_period": p.EmploymentPeriod,
		"raw":               json.RawMessage(rawJSON),
	})
	if err != nil {
		return fmt.Errorf("marshal pdc lobbyist affiliation context: %w", err)
	}
	evidenceJSON, err := json.Marshal([]string{
		fmt.Sprintf("PDC lobbyist employment row: lobbyist_id=%s employer_id=%s year=%s report=%s", p.LobbyistID, p.EmployerID, p.EmploymentYear, p.ReportNumber),
	})
	if err != nil {
		return fmt.Errorf("marshal pdc lobbyist affiliation evidence: %w", err)
	}

	const personQ = `
INSERT INTO person (display_name, normalized_name, pdc_lobbyist_id, match_confidence, match_notes)
VALUES ($1, $2, $3, 'confirmed', 'Auto-seeded from PDC lobbyist_id; PDC source identity is authoritative for lobbyist status.')
ON CONFLICT (pdc_lobbyist_id) WHERE pdc_lobbyist_id IS NOT NULL DO UPDATE SET
  display_name = EXCLUDED.display_name,
  normalized_name = COALESCE(person.normalized_name, EXCLUDED.normalized_name),
  match_confidence = 'confirmed',
  match_notes = EXCLUDED.match_notes,
  updated_at = NOW()
RETURNING id;`
	var personID int64
	if err := s.Pool.QueryRow(ctx, personQ, lobbyistName, normalizePersonName(lobbyistName), p.LobbyistID).Scan(&personID); err != nil {
		return fmt.Errorf("upsert PDC lobbyist person: %w", err)
	}

	var orgID *int64
	const orgQ = `
SELECT id
  FROM organization
 WHERE pdc_lobbyist_employer_id = $1
    OR lower(trim(canonical_name)) = lower(trim($2))
    OR lower(trim($2)) = ANY(
         SELECT lower(trim(alias)) FROM unnest(COALESCE(aliases, ARRAY[]::text[])) AS alias
       )
 ORDER BY CASE WHEN pdc_lobbyist_employer_id = $1 THEN 0 ELSE 1 END, id
 LIMIT 1;`
	var foundOrgID int64
	err = s.Pool.QueryRow(ctx, orgQ, p.EmployerID, p.EmployerName).Scan(&foundOrgID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("lookup PDC employer organization: %w", err)
	}
	if err == nil {
		orgID = &foundOrgID
	}

	recordYear := 0
	if y, err := strconv.Atoi(strings.TrimSpace(p.EmploymentYear)); err == nil {
		recordYear = y
	}
	sourceRowID := strings.Join([]string{p.ReportNumber, p.LobbyistID, p.EmployerID, p.EmploymentYear}, "|")
	const affQ = `
INSERT INTO person_organization_affiliation (
  person_id, organization_id, raw_person_name, raw_organization_name,
  relationship_type, role_title, record_year, source_kind, source_table,
  source_pk, source_row_id, context, confidence, review_status, evidence
)
VALUES ($1, $2, $3, $4,
        'lobbyist_for', 'lobbyist', NULLIF($5,0), 'pdc_lobbyist_employment', 'data.wa.gov:xhn7-64im',
        0, $6, $7, 'confirmed', 'auto', $8)
ON CONFLICT (relationship_type, source_kind, source_table, source_pk, source_row_id, person_id, organization_id, raw_person_name, raw_organization_name) DO UPDATE SET
  organization_id = COALESCE(EXCLUDED.organization_id, person_organization_affiliation.organization_id),
  record_year = COALESCE(EXCLUDED.record_year, person_organization_affiliation.record_year),
  context = EXCLUDED.context,
  confidence = 'confirmed',
  evidence = EXCLUDED.evidence,
  updated_at = NOW();`
	if _, err := s.Pool.Exec(ctx, affQ,
		personID, orgID, lobbyistName, p.EmployerName,
		recordYear, sourceRowID, contextJSON, evidenceJSON,
	); err != nil {
		return fmt.Errorf("upsert PDC lobbyist affiliation: %w", err)
	}
	return nil
}

// CrossSourceMatch represents a match for a CSI organization name against
// an external authoritative registry (IRS BMF or PDC employer roster).
type CrossSourceMatch struct {
	Source     string // "irs_bmf" or "pdc_employer"
	EIN        string // populated when Source == "irs_bmf"
	EmployerID string // populated when Source == "pdc_employer"
	Name       string
}

// LookupOrgCrossSource returns the first cross-source match for normalizedName,
// preferring IRS BMF (501(c) status) over PDC employer registration. Returns
// (match, true, nil) on hit, (_, false, nil) on miss.
func (s *Store) LookupOrgCrossSource(ctx context.Context, normalizedName string) (CrossSourceMatch, bool, error) {
	if strings.TrimSpace(normalizedName) == "" {
		return CrossSourceMatch{}, false, nil
	}
	{
		const q = `SELECT ein, name FROM irs_bmf_organization WHERE normalized_name = $1 ORDER BY ein LIMIT 2`
		rows, err := s.Pool.Query(ctx, q, normalizedName)
		if err != nil {
			return CrossSourceMatch{}, false, fmt.Errorf("lookup irs bmf: %w", err)
		}
		var matches []CrossSourceMatch
		for rows.Next() {
			var ein, name string
			if err := rows.Scan(&ein, &name); err != nil {
				rows.Close()
				return CrossSourceMatch{}, false, fmt.Errorf("scan irs bmf match: %w", err)
			}
			matches = append(matches, CrossSourceMatch{Source: "irs_bmf", EIN: ein, Name: name})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return CrossSourceMatch{}, false, fmt.Errorf("lookup irs bmf: %w", err)
		}
		if len(matches) == 1 {
			return matches[0], true, nil
		}
		if len(matches) > 1 {
			return CrossSourceMatch{}, false, nil
		}
	}
	{
		const q = `SELECT employer_id, name FROM pdc_employer WHERE normalized_name = $1 ORDER BY employer_id LIMIT 2`
		rows, err := s.Pool.Query(ctx, q, normalizedName)
		if err != nil {
			return CrossSourceMatch{}, false, fmt.Errorf("lookup pdc employer: %w", err)
		}
		var matches []CrossSourceMatch
		for rows.Next() {
			var id, name string
			if err := rows.Scan(&id, &name); err != nil {
				rows.Close()
				return CrossSourceMatch{}, false, fmt.Errorf("scan pdc employer match: %w", err)
			}
			matches = append(matches, CrossSourceMatch{Source: "pdc_employer", EmployerID: id, Name: name})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return CrossSourceMatch{}, false, fmt.Errorf("lookup pdc employer: %w", err)
		}
		if len(matches) == 1 {
			return matches[0], true, nil
		}
		if len(matches) > 1 {
			return CrossSourceMatch{}, false, nil
		}
	}
	return CrossSourceMatch{}, false, nil
}

// MarkOrganizationVerified records that orgID has been confirmed against an
// authoritative source. Sets verified_at, verification_source, and the
// matching FK column. Match confidence is bumped to 'confirmed'.
func (s *Store) MarkOrganizationVerified(ctx context.Context, orgID int64, m CrossSourceMatch) error {
	switch m.Source {
	case "irs_bmf":
		const q = `
UPDATE organization
   SET irs_bmf_ein         = $1,
       verified_at         = NOW(),
       verification_source = 'irs_bmf',
       match_confidence    = 'confirmed',
       match_notes         = COALESCE(match_notes, '') || CASE WHEN match_notes IS NULL OR match_notes = '' THEN '' ELSE E'\n' END || 'Cross-source match: IRS BMF EIN ' || $1,
       updated_at          = NOW()
 WHERE id = $2;`
		if _, err := s.Pool.Exec(ctx, q, m.EIN, orgID); err != nil {
			return fmt.Errorf("mark org verified (irs_bmf): %w", err)
		}
	case "pdc_employer":
		const q = `
UPDATE organization
   SET pdc_lobbyist_employer_id = $1,
       verified_at              = NOW(),
       verification_source      = 'pdc_employer',
       match_confidence         = 'confirmed',
       match_notes              = COALESCE(match_notes, '') || CASE WHEN match_notes IS NULL OR match_notes = '' THEN '' ELSE E'\n' END || 'Cross-source match: PDC employer_id ' || $1,
       updated_at               = NOW()
 WHERE id = $2;`
		if _, err := s.Pool.Exec(ctx, q, m.EmployerID, orgID); err != nil {
			return fmt.Errorf("mark org verified (pdc_employer): %w", err)
		}
		if err := s.AttachPDCEmployerAffiliationsToOrganization(ctx, orgID, m.EmployerID, m.Name); err != nil {
			return fmt.Errorf("attach PDC employer affiliations: %w", err)
		}
	default:
		return fmt.Errorf("unknown cross-source source: %q", m.Source)
	}
	return nil
}

// AttachPDCEmployerAffiliationsToOrganization links PDC lobbyist-employment
// affiliation rows that were ingested before the canonical organization existed.
func (s *Store) AttachPDCEmployerAffiliationsToOrganization(ctx context.Context, orgID int64, employerID, employerName string) error {
	employerID = strings.TrimSpace(employerID)
	employerName = strings.TrimSpace(employerName)
	if orgID == 0 || (employerID == "" && employerName == "") {
		return nil
	}

	const deleteDuplicates = `
WITH candidates AS (
    SELECT id, relationship_type, source_kind, source_table, source_pk, source_row_id,
           person_id, raw_person_name, raw_organization_name
      FROM person_organization_affiliation
     WHERE source_kind = 'pdc_lobbyist_employment'
       AND relationship_type = 'lobbyist_for'
       AND organization_id IS NULL
       AND (
             ($2 <> '' AND context->>'employer_id' = $2)
          OR ($3 <> '' AND lower(trim(raw_organization_name)) = lower(trim($3)))
          OR ($3 <> '' AND lower(trim(context->>'employer_name')) = lower(trim($3)))
       )
),
duplicates AS (
    SELECT c.id
      FROM candidates c
      JOIN person_organization_affiliation existing
        ON existing.organization_id = $1
       AND existing.relationship_type = c.relationship_type
       AND existing.source_kind = c.source_kind
       AND existing.source_table = c.source_table
       AND existing.source_pk IS NOT DISTINCT FROM c.source_pk
       AND existing.source_row_id IS NOT DISTINCT FROM c.source_row_id
       AND existing.person_id IS NOT DISTINCT FROM c.person_id
       AND existing.raw_person_name IS NOT DISTINCT FROM c.raw_person_name
       AND existing.raw_organization_name IS NOT DISTINCT FROM c.raw_organization_name
)
DELETE FROM person_organization_affiliation poa
 USING duplicates
 WHERE poa.id = duplicates.id;`
	if _, err := s.Pool.Exec(ctx, deleteDuplicates, orgID, employerID, employerName); err != nil {
		return fmt.Errorf("delete duplicate PDC employer affiliations: %w", err)
	}

	const deleteCandidateDuplicates = `
WITH candidates AS (
    SELECT id, relationship_type, source_kind, source_table, source_pk, source_row_id,
           person_id, raw_person_name, raw_organization_name,
           ROW_NUMBER() OVER (
               PARTITION BY relationship_type, source_kind, source_table, source_pk,
                            source_row_id, person_id, raw_person_name,
                            raw_organization_name
               ORDER BY id
           ) AS rn
      FROM person_organization_affiliation
     WHERE source_kind = 'pdc_lobbyist_employment'
       AND relationship_type = 'lobbyist_for'
       AND organization_id IS NULL
       AND (
             ($1 <> '' AND context->>'employer_id' = $1)
          OR ($2 <> '' AND lower(trim(raw_organization_name)) = lower(trim($2)))
          OR ($2 <> '' AND lower(trim(context->>'employer_name')) = lower(trim($2)))
       )
)
DELETE FROM person_organization_affiliation poa
 USING candidates
 WHERE poa.id = candidates.id
   AND candidates.rn > 1;`
	if _, err := s.Pool.Exec(ctx, deleteCandidateDuplicates, employerID, employerName); err != nil {
		return fmt.Errorf("delete duplicate unattached PDC employer affiliations: %w", err)
	}

	const attach = `
UPDATE person_organization_affiliation
   SET organization_id = $1,
       updated_at = NOW()
 WHERE source_kind = 'pdc_lobbyist_employment'
   AND relationship_type = 'lobbyist_for'
   AND organization_id IS NULL
   AND (
         ($2 <> '' AND context->>'employer_id' = $2)
      OR ($3 <> '' AND lower(trim(raw_organization_name)) = lower(trim($3)))
      OR ($3 <> '' AND lower(trim(context->>'employer_name')) = lower(trim($3)))
   );`
	if _, err := s.Pool.Exec(ctx, attach, orgID, employerID, employerName); err != nil {
		return fmt.Errorf("attach PDC employer affiliations: %w", err)
	}
	return nil
}

// MarkOrganizationConfirmedByReview promotes an organization to
// match_confidence='confirmed' as a result of a human review decision. Unlike
// MarkOrganizationVerified, it does not require an authoritative cross-source
// hit: the verification provenance is the reviewer themselves.
func (s *Store) MarkOrganizationConfirmedByReview(ctx context.Context, orgID int64, source, reviewer, note string) error {
	if source == "" {
		source = "manual"
	}
	const q = `
UPDATE organization
   SET match_confidence    = 'confirmed',
       verified_at         = COALESCE(verified_at, NOW()),
       verification_source = COALESCE(verification_source, $2),
       match_notes         = COALESCE(match_notes, '')
                             || CASE WHEN match_notes IS NULL OR match_notes = '' THEN '' ELSE E'\n' END
                             || $3,
       updated_at          = NOW()
 WHERE id = $1;`
	noteLine := fmt.Sprintf("Confirmed via review (%s)", source)
	if reviewer != "" {
		noteLine += " by " + reviewer
	}
	if note != "" {
		noteLine += ": " + note
	}
	if _, err := s.Pool.Exec(ctx, q, orgID, source, noteLine); err != nil {
		return fmt.Errorf("mark org confirmed by review: %w", err)
	}
	return nil
}

type VerifyOrganizationsStats struct {
	Scanned    int
	Matched    int
	IRSMatches int
	PDCMatches int
	Deleted    int // unmatched rows removed when DeleteUnmatched is set
}

// VerifyOrganizationsOptions controls VerifyOrganizationsAgainstSources.
type VerifyOrganizationsOptions struct {
	DryRun          bool
	DeleteUnmatched bool // delete unverified, possible-confidence rows that don't match any source
}

// VerifyOrganizationsAgainstSources iterates organization rows that are not
// yet verified, looks each one up against irs_bmf_organization and pdc_employer
// by normalized name, and on hit calls MarkOrganizationVerified. When
// opts.DeleteUnmatched is set, rows with match_confidence='possible' that
// remain unmatched are deleted (testifier.normalized_org_id is ON DELETE SET
// NULL since 0018, so deletes are safe).
func (s *Store) VerifyOrganizationsAgainstSources(ctx context.Context, opts VerifyOrganizationsOptions, progress func(scanned, matched int, name string)) (VerifyOrganizationsStats, error) {
	const q = `
SELECT id, canonical_name, aliases, match_confidence::text
  FROM organization
 WHERE verified_at IS NULL
 ORDER BY id;`
	rows, err := s.Pool.Query(ctx, q)
	if err != nil {
		return VerifyOrganizationsStats{}, fmt.Errorf("list unverified organizations: %w", err)
	}
	type cand struct {
		id         int64
		name       string
		aliases    []string
		confidence string
	}
	var cands []cand
	for rows.Next() {
		var c cand
		if err := rows.Scan(&c.id, &c.name, &c.aliases, &c.confidence); err != nil {
			rows.Close()
			return VerifyOrganizationsStats{}, fmt.Errorf("scan org: %w", err)
		}
		cands = append(cands, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return VerifyOrganizationsStats{}, err
	}

	var stats VerifyOrganizationsStats
	for _, c := range cands {
		stats.Scanned++
		var match CrossSourceMatch
		var ok bool
		names := append([]string{c.name}, c.aliases...)
		for _, n := range names {
			normRow := s.Pool.QueryRow(ctx, `SELECT wa_dd_normalize_entity_name($1)`, n)
			var norm *string
			if err := normRow.Scan(&norm); err != nil {
				return stats, fmt.Errorf("normalize %q: %w", n, err)
			}
			if norm == nil || *norm == "" {
				continue
			}
			m, found, err := s.LookupOrgCrossSource(ctx, *norm)
			if err != nil {
				return stats, err
			}
			if found {
				match = m
				ok = true
				break
			}
		}
		if !ok {
			if opts.DeleteUnmatched && c.confidence == "possible" {
				stats.Deleted++
				if !opts.DryRun {
					if _, err := s.Pool.Exec(ctx, `DELETE FROM organization WHERE id = $1`, c.id); err != nil {
						return stats, fmt.Errorf("delete unmatched organization %d (%q): %w", c.id, c.name, err)
					}
				}
			}
			if progress != nil {
				progress(stats.Scanned, stats.Matched, c.name)
			}
			continue
		}
		stats.Matched++
		switch match.Source {
		case "irs_bmf":
			stats.IRSMatches++
		case "pdc_employer":
			stats.PDCMatches++
		}
		if !opts.DryRun {
			if err := s.MarkOrganizationVerified(ctx, c.id, match); err != nil {
				return stats, err
			}
		}
		if progress != nil {
			progress(stats.Scanned, stats.Matched, c.name)
		}
	}
	return stats, nil
}
