package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
	SourceRecordID    int64
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
    ntee_code, income_amount, revenue_amount, asset_amount, raw, source_record_id)
VALUES ($1,$2,COALESCE(wa_dd_normalize_entity_name($2),''),$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20::jsonb,NULLIF($21,0))
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
  source_record_id  = COALESCE(EXCLUDED.source_record_id, irs_bmf_organization.source_record_id),
  fetched_at        = NOW();`
	_, err = s.Pool.Exec(ctx, q,
		p.EIN, p.Name, strOrNull(p.SortName),
		strOrNull(p.Street), strOrNull(p.City), strOrNull(p.State), strOrNull(p.Zip),
		strOrNull(p.SubsectionCode), strOrNull(p.Classification), strOrNull(p.DeductibilityCode),
		strOrNull(p.ActivityCodes), strOrNull(p.FoundationCode), strOrNull(p.OrganizationCode),
		strOrNull(p.StatusCode), strOrNull(p.RulingDate), strOrNull(p.NTEECode),
		p.IncomeAmount, p.RevenueAmount, p.AssetAmount,
		string(rawJSON), p.SourceRecordID,
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
	SourceRecordID     int64
}

func (s *Store) UpsertPDCEmployer(ctx context.Context, p UpsertPDCEmployerParams) error {
	rawJSON, err := json.Marshal(p.Raw)
	if err != nil {
		return fmt.Errorf("marshal pdc employer raw: %w", err)
	}
	const q = `
INSERT INTO pdc_employer (employer_id, name, normalized_name,
    last_employment_year, last_report_number, last_employment_url,
    raw, source_record_id, last_seen_at)
VALUES ($1,$2,COALESCE(wa_dd_normalize_entity_name($2),''),$3,$4,$5,$6::jsonb,NULLIF($7,0),NOW())
ON CONFLICT (employer_id) DO UPDATE SET
  name                = EXCLUDED.name,
  normalized_name     = EXCLUDED.normalized_name,
  last_employment_year = COALESCE(EXCLUDED.last_employment_year, pdc_employer.last_employment_year),
  last_report_number   = COALESCE(EXCLUDED.last_report_number, pdc_employer.last_report_number),
  last_employment_url  = COALESCE(EXCLUDED.last_employment_url, pdc_employer.last_employment_url),
  raw                  = EXCLUDED.raw,
  source_record_id     = COALESCE(EXCLUDED.source_record_id, pdc_employer.source_record_id),
  last_seen_at         = NOW();`
	_, err = s.Pool.Exec(ctx, q,
		p.EmployerID, p.Name,
		strOrNull(p.LastEmploymentYear), strOrNull(p.LastReportNumber), strOrNull(p.LastEmploymentURL),
		string(rawJSON), p.SourceRecordID,
	)
	if err != nil {
		return fmt.Errorf("upsert pdc_employer: %w", err)
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
	default:
		return fmt.Errorf("unknown cross-source source: %q", m.Source)
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
