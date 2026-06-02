package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type UpsertOrganizationParams struct {
	CanonicalName         string
	Aliases               []string
	PDCLobbyistEmployerID string
	PDCCommitteeOrFilerID string
	MatchConfidence       string // org_match_confidence enum
	MatchNotes            string
}

func (s *Store) UpsertOrganization(ctx context.Context, p UpsertOrganizationParams) (int64, error) {
	const q = `
INSERT INTO organization (canonical_name, aliases, pdc_lobbyist_employer_id,
                          pdc_committee_or_filer_id, match_confidence, match_notes)
VALUES ($1,$2,$3,$4,$5::org_match_confidence,$6)
ON CONFLICT (canonical_name) DO UPDATE SET
  aliases                   = EXCLUDED.aliases,
  pdc_lobbyist_employer_id  = COALESCE(EXCLUDED.pdc_lobbyist_employer_id, organization.pdc_lobbyist_employer_id),
  pdc_committee_or_filer_id = COALESCE(EXCLUDED.pdc_committee_or_filer_id, organization.pdc_committee_or_filer_id),
  match_confidence          = EXCLUDED.match_confidence,
  match_notes               = EXCLUDED.match_notes,
  updated_at                = NOW()
RETURNING id;`
	var id int64
	err := s.Pool.QueryRow(ctx, q,
		p.CanonicalName,
		nullStringArray(p.Aliases),
		strOrNull(p.PDCLobbyistEmployerID),
		strOrNull(p.PDCCommitteeOrFilerID),
		defaultStr(p.MatchConfidence, "unmatched"),
		strOrNull(p.MatchNotes),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert organization: %w", err)
	}
	return id, nil
}

// LinkTestifiersToOrg sets normalized_org_id on testifier rows whose
// raw_organization matches any of the supplied raw names (case-insensitive).
// Used by Step 7 (org context) to bind testifiers to their canonical org.
func (s *Store) LinkTestifiersToOrg(ctx context.Context, orgID int64, rawOrgNames []string) (int64, error) {
	if len(rawOrgNames) == 0 {
		return 0, nil
	}
	lowers := make([]string, len(rawOrgNames))
	for i, n := range rawOrgNames {
		lowers[i] = lowerTrim(n)
	}
	const q = `
UPDATE testifier SET normalized_org_id = $1
 WHERE lower(trim(raw_organization)) = ANY($2);`
	tag, err := s.Pool.Exec(ctx, q, orgID, lowers)
	if err != nil {
		return 0, err
	}
	if tag.RowsAffected() == 0 {
		return 0, nil
	}
	if err := s.BackfillCSITestifierPersonAffiliationsForRawOrganizations(ctx, rawOrgNames); err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ---------------------------------------------------------------------------
// bill_status_change
// ---------------------------------------------------------------------------

// OrganizationAggregate is the row shape ListOrganizations returns.
type OrganizationAggregate struct {
	ID              int64
	CanonicalName   string
	Aliases         []string
	MatchConfidence string
	MatchNotes      string
	TestifierCount  int
	ProCount        int
	ConCount        int
	OtherCount      int
	UnknownCount    int
}

// ListOrganizations returns every organization with aggregated testifier
// position counts. When topicKeywords is non-empty, aggregation is scoped to
// testifiers whose agenda item is on a bill matching any keyword (matched
// against bill number/title/description, agenda item label, or committee
// name) and organizations with no qualifying testifiers are dropped.
func (s *Store) ListOrganizations(ctx context.Context, topicKeywords []string) ([]OrganizationAggregate, error) {
	keywords := nonEmptyStrings(topicKeywords)
	args := []any{}
	push := func(v any) int { args = append(args, v); return len(args) }

	testifierWhere := ""
	if len(keywords) > 0 {
		ors := make([]string, 0, len(keywords))
		for _, kw := range keywords {
			idx := push(kw)
			ors = append(ors, fmt.Sprintf(`(
		b.bill_number ILIKE '%%' || $%d || '%%'
		OR b.title ILIKE '%%' || $%d || '%%'
		OR COALESCE(b.description, '') ILIKE '%%' || $%d || '%%'
		OR a.label ILIKE '%%' || $%d || '%%'
		OR h.committee_name ILIKE '%%' || $%d || '%%'
)`, idx, idx, idx, idx, idx))
		}
		testifierWhere = `
       AND EXISTS (
         SELECT 1
           FROM agenda_item a
           JOIN hearing  h ON h.id = a.hearing_id
           LEFT JOIN bill b ON b.id = a.bill_id
          WHERE a.id = t.agenda_item_id
            AND (` + strings.Join(ors, " OR ") + `)
       )`
	}

	q := `
SELECT o.id, o.canonical_name, o.aliases,
       o.match_confidence::text, COALESCE(o.match_notes, ''),
       COUNT(DISTINCT t.id) AS testifier_count,
       COUNT(DISTINCT t.id) FILTER (WHERE t.position = 'Pro')   AS pro_count,
       COUNT(DISTINCT t.id) FILTER (WHERE t.position = 'Con')   AS con_count,
       COUNT(DISTINCT t.id) FILTER (WHERE t.position = 'Other') AS other_count,
       COUNT(DISTINCT t.id) FILTER (WHERE t.position = 'Unknown') AS unknown_count
  FROM organization o
  LEFT JOIN testifier t ON t.normalized_org_id = o.id` + testifierWhere + `
 GROUP BY o.id`
	if len(keywords) > 0 {
		q += "\nHAVING COUNT(DISTINCT t.id) > 0"
	}
	q += "\n ORDER BY o.canonical_name;"

	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list organizations: %w", err)
	}
	defer rows.Close()
	out := []OrganizationAggregate{}
	for rows.Next() {
		var o OrganizationAggregate
		if err := rows.Scan(&o.ID, &o.CanonicalName, &o.Aliases,
			&o.MatchConfidence, &o.MatchNotes,
			&o.TestifierCount, &o.ProCount, &o.ConCount,
			&o.OtherCount, &o.UnknownCount); err != nil {
			return nil, fmt.Errorf("scan organization: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// OrganizationAppearance is one (org, agenda_item) row.
type OrganizationAppearance struct {
	Biennium        string
	BillID          string
	BillPrefix      string
	BillNumber      int
	CSIAgendaItemID string
	HearingID       int64
	HearingTitle    string
	CommitteeName   string
	Chamber         string
	MeetingDateTime time.Time
	Position        string
	TestifierCount  int
}

type OrganizationPublicContext struct {
	ContextType     string
	SourceKind      string
	SourceLabel     string
	SourceName      string
	Detail          string
	Amount          string
	RecordYear      int
	RecordDate      string
	URL             string
	SourceRecordID  int64
	MatchConfidence string
	Evidence        []string
}

// GetOrganizationAppearances returns every (agenda_item, org) appearance
// where at least one testifier from that org signed in.
func (s *Store) GetOrganizationAppearances(ctx context.Context, organizationID int64) ([]OrganizationAppearance, error) {
	const q = `
SELECT b.biennium, b.bill_number, b.prefix, b.number,
       COALESCE(a.csi_agenda_item_id, ''),
       h.id,
       COALESCE(a.label, ''),
       h.committee_name,
       COALESCE(h.chamber, ''),
       h.meeting_datetime,
       MAX(t.position::text) AS position,
       COUNT(t.id) AS testifier_count
  FROM testifier t
  JOIN agenda_item a ON a.id = t.agenda_item_id
  JOIN hearing     h ON h.id = a.hearing_id
  LEFT JOIN bill   b ON b.id = a.bill_id
 WHERE t.normalized_org_id = $1
 GROUP BY b.biennium, b.bill_number, b.prefix, b.number,
          a.csi_agenda_item_id, h.id, a.label, h.committee_name, h.chamber, h.meeting_datetime
 ORDER BY h.meeting_datetime DESC;`
	rows, err := s.Pool.Query(ctx, q, organizationID)
	if err != nil {
		return nil, fmt.Errorf("organization appearances: %w", err)
	}
	defer rows.Close()
	out := []OrganizationAppearance{}
	for rows.Next() {
		var a OrganizationAppearance
		var biennium, billID, prefix *string
		var billNumber *int
		if err := rows.Scan(&biennium, &billID, &prefix, &billNumber,
			&a.CSIAgendaItemID, &a.HearingID, &a.HearingTitle,
			&a.CommitteeName, &a.Chamber, &a.MeetingDateTime,
			&a.Position, &a.TestifierCount); err != nil {
			return nil, fmt.Errorf("scan appearance: %w", err)
		}
		if biennium != nil {
			a.Biennium = *biennium
		}
		if billID != nil {
			a.BillID = *billID
		}
		if prefix != nil {
			a.BillPrefix = *prefix
		}
		if billNumber != nil {
			a.BillNumber = *billNumber
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) GetOrganizationPublicContexts(ctx context.Context, organizationID int64) ([]OrganizationPublicContext, error) {
	const q = `
WITH ctx AS (
  SELECT 'lobbying_registration'::text AS context_type,
         m.source_kind::text AS source_kind,
         'PDC lobbying employer'::text AS source_label,
         pe.name AS source_name,
         NULLIF('Employer ID ' || pe.employer_id, '') AS detail,
         ''::text AS amount,
         CASE WHEN pe.last_employment_year ~ '^[0-9]+$' THEN pe.last_employment_year::int ELSE 0 END AS record_year,
         ''::text AS record_date,
         COALESCE(pe.last_employment_url, '') AS url,
         COALESCE(pe.source_record_id,0) AS source_record_id,
         m.match_confidence::text AS match_confidence,
         m.evidence AS evidence
    FROM (
      SELECT DISTINCT ON (source_row_id) *
        FROM reviewed_vendor_entity_match
       WHERE organization_id = $1
         AND source_kind = 'pdc_lobbying_organization'
       ORDER BY source_row_id, reviewed_at DESC
    ) m
    JOIN LATERAL (
      SELECT *
        FROM pdc_employer pe2
       WHERE pe2.employer_id = m.source_row_id
       ORDER BY pe2.last_employment_year DESC NULLS LAST, pe2.source_record_id DESC
       LIMIT 1
    ) pe ON TRUE
  UNION ALL
  SELECT 'state_contract',
         m.source_kind::text,
         'DataWA agency contract',
         c.contractor_name,
         COALESCE(NULLIF(c.contract_number, ''), NULLIF(c.description, ''), 'Agency contract'),
         COALESCE(c.total_amount::text, ''),
         COALESCE(c.fiscal_year, 0),
         '',
         '',
         COALESCE(c.source_record_id,0),
         m.match_confidence::text,
         m.evidence
    FROM reviewed_vendor_entity_match m
    JOIN datawa_contract c ON c.id = m.source_pk
   WHERE m.organization_id = $1
     AND m.source_kind = 'datawa_contract_contractor'
  UNION ALL
  SELECT 'state_contract',
         m.source_kind::text,
         'DataWA master contract sale',
         s.vendor_name,
         COALESCE(NULLIF(s.contract_number, ''), NULLIF(s.contract_title, ''), 'Master contract sale'),
         COALESCE(s.total_sales_reported::text, ''),
         COALESCE(s.report_year, 0),
         '',
         '',
         COALESCE(s.source_record_id,0),
         m.match_confidence::text,
         m.evidence
    FROM reviewed_vendor_entity_match m
    JOIN datawa_master_contract_sale s ON s.id = m.source_pk
   WHERE m.organization_id = $1
     AND m.source_kind = 'datawa_master_contract_vendor'
  UNION ALL
  SELECT 'state_contract',
         m.source_kind::text,
         'DataWA IT contract',
         c.contractor_name,
         COALESCE(NULLIF(c.contract_number, ''), 'IT contract'),
         COALESCE(c.total_contract_amount::text, ''),
         COALESCE(c.report_fiscal_year, 0),
         '',
         '',
         COALESCE(c.source_record_id,0),
         m.match_confidence::text,
         m.evidence
    FROM reviewed_vendor_entity_match m
    JOIN datawa_it_contract c ON c.id = m.source_pk
   WHERE m.organization_id = $1
     AND m.source_kind IN ('datawa_it_contract_contractor', 'datawa_it_contract_dba')
  UNION ALL
  SELECT 'state_vendor',
         m.source_kind::text,
         'WEBS vendor',
         v.company_name,
         COALESCE(NULLIF(v.description_of_work, ''), NULLIF(v.commodity_code, ''), 'WEBS vendor registration'),
         '',
         0,
         '',
         COALESCE(v.web_address, ''),
         COALESCE(v.source_record_id,0),
         m.match_confidence::text,
         m.evidence
    FROM reviewed_vendor_entity_match m
    JOIN datawa_webs_vendor v ON v.id = m.source_pk
   WHERE m.organization_id = $1
     AND m.source_kind = 'datawa_webs_vendor'
  UNION ALL
  SELECT 'state_vendor_payment',
         m.source_kind::text,
         'FiscalWA vendor payment',
         p.vendor_name,
         COALESCE(NULLIF(p.agency_name, ''), NULLIF(p.subobject_name, ''), 'Vendor payment'),
         COALESCE(p.amount::text, ''),
         COALESCE(p.fiscal_year, 0),
         '',
         '',
         COALESCE(p.source_record_id,0),
         m.match_confidence::text,
         m.evidence
    FROM reviewed_vendor_entity_match m
    JOIN fiscalwa_vendor_payment p ON p.id = m.source_pk
   WHERE m.organization_id = $1
     AND m.source_kind = 'fiscalwa_vendor_payment'
  UNION ALL
  SELECT 'federal_award',
         m.source_kind::text,
         'USAspending award',
         a.recipient_name,
         COALESCE(NULLIF(a.awarding_agency, ''), NULLIF(a.funding_agency, ''), a.award_id),
         COALESCE(a.award_amount::text, ''),
         COALESCE(EXTRACT(YEAR FROM a.start_date)::int, 0),
         COALESCE(a.start_date::text, ''),
         '',
         COALESCE(a.source_record_id,0),
         m.match_confidence::text,
         m.evidence
    FROM reviewed_vendor_entity_match m
    JOIN federal_award a ON a.id = m.source_pk
   WHERE m.organization_id = $1
     AND m.source_kind = 'federal_award_recipient'
)
SELECT DISTINCT ON (source_kind, source_record_id, source_name, detail)
       context_type, source_kind, source_label, COALESCE(source_name, ''),
       COALESCE(detail, ''), amount, COALESCE(record_year, 0), record_date,
       url, source_record_id, match_confidence, evidence
  FROM ctx
 ORDER BY source_kind, source_record_id, source_name, detail,
          context_type, record_year DESC NULLS LAST
 LIMIT 100;`
	rows, err := s.Pool.Query(ctx, q, organizationID)
	if err != nil {
		return nil, fmt.Errorf("organization public contexts: %w", err)
	}
	defer rows.Close()
	out := []OrganizationPublicContext{}
	for rows.Next() {
		var c OrganizationPublicContext
		var evidence []byte
		if err := rows.Scan(&c.ContextType, &c.SourceKind, &c.SourceLabel, &c.SourceName,
			&c.Detail, &c.Amount, &c.RecordYear, &c.RecordDate, &c.URL,
			&c.SourceRecordID, &c.MatchConfidence, &evidence); err != nil {
			return nil, fmt.Errorf("scan organization public context: %w", err)
		}
		if len(evidence) > 0 {
			_ = json.Unmarshal(evidence, &c.Evidence)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type PopulateOrganizationsStats struct {
	CandidatesProcessed   int
	MentionsUpserted      int
	OrganizationsUpserted int
	TestifiersLinked      int64
	Skipped               int
	Verified              int // matched against IRS BMF or PDC employer
}

type PopulateOrganizationsProgress struct {
	Phase       string
	Stats       PopulateOrganizationsStats
	SourceName  string
	ElapsedTime time.Duration
}

// PopulateOrganizationsFromCSI seeds organization rows from distinct
// testifier.raw_organization values, records source mentions, and links all
// matching testifier rows to the resulting organization. It is intentionally
// source-local: no cross-source merge is asserted here.
func (s *Store) PopulateOrganizationsFromCSI(ctx context.Context) (PopulateOrganizationsStats, error) {
	return s.PopulateOrganizationsFromCSIWithProgress(ctx, nil)
}

func (s *Store) PopulateOrganizationsFromCSIWithProgress(ctx context.Context, progress func(PopulateOrganizationsProgress)) (PopulateOrganizationsStats, error) {
	const q = `
WITH orgs AS (
    SELECT MIN(t.id) AS source_pk,
           trim(t.raw_organization) AS source_name,
           wa_dd_normalize_entity_name(trim(t.raw_organization)) AS normalized_name,
           COUNT(*) AS occurrence_count,
           MIN(t.source_record_id) AS source_record_id
      FROM testifier t
     WHERE NULLIF(trim(t.raw_organization), '') IS NOT NULL
     GROUP BY trim(t.raw_organization), wa_dd_normalize_entity_name(trim(t.raw_organization))
)
SELECT source_pk, source_name, normalized_name, occurrence_count, source_record_id
  FROM orgs
 WHERE normalized_name IS NOT NULL
 ORDER BY occurrence_count DESC, source_name;`
	started := time.Now()
	emitProgress := func(phase string, stats PopulateOrganizationsStats, sourceName string) {
		if progress == nil {
			return
		}
		progress(PopulateOrganizationsProgress{
			Phase:       phase,
			Stats:       stats,
			SourceName:  sourceName,
			ElapsedTime: time.Since(started).Round(time.Second),
		})
	}

	emitProgress("listing", PopulateOrganizationsStats{}, "")
	rows, err := s.Pool.Query(ctx, q)
	if err != nil {
		return PopulateOrganizationsStats{}, fmt.Errorf("list CSI organizations: %w", err)
	}
	defer rows.Close()

	var stats PopulateOrganizationsStats
	emitProgress("processing", stats, "")
	lastProgress := time.Now()
	for rows.Next() {
		var sourcePK, count, sourceRecordID int64
		var sourceName string
		var normalized *string
		if err := rows.Scan(&sourcePK, &sourceName, &normalized, &count, &sourceRecordID); err != nil {
			return stats, fmt.Errorf("scan CSI organization: %w", err)
		}
		stats.CandidatesProcessed++
		if normalized == nil || junkOrganizationName(sourceName, *normalized) {
			stats.Skipped++
			if stats.CandidatesProcessed == 1 || stats.CandidatesProcessed%100 == 0 || time.Since(lastProgress) >= 5*time.Second {
				emitProgress("processing", stats, sourceName)
				lastProgress = time.Now()
			}
			continue
		}
		orgID, err := s.organizationIDForAliasOrCanonical(ctx, sourceName)
		if err != nil {
			return stats, err
		}
		if orgID == 0 {
			orgID, err = s.UpsertOrganization(ctx, UpsertOrganizationParams{
				CanonicalName:   sourceName,
				Aliases:         []string{},
				MatchConfidence: "possible",
				MatchNotes:      "Seeded from CSI testimony organization string; source-local identity only.",
			})
			if err != nil {
				return stats, err
			}
		}
		if err != nil {
			return stats, err
		}
		stats.OrganizationsUpserted++
		if match, ok, err := s.LookupOrgCrossSource(ctx, *normalized); err != nil {
			return stats, err
		} else if ok {
			if err := s.MarkOrganizationVerified(ctx, orgID, match); err != nil {
				return stats, err
			}
			stats.Verified++
		}
		if err := s.upsertOrganizationSourceMention(ctx, "csi_testifier", "testifier", sourcePK, sourceName, *normalized, orgID, int(count), sourceRecordID, "possible"); err != nil {
			return stats, err
		}
		stats.MentionsUpserted++
		linked, err := s.LinkTestifiersToOrg(ctx, orgID, []string{sourceName})
		if err != nil {
			return stats, err
		}
		stats.TestifiersLinked += linked
		if stats.CandidatesProcessed == 1 || stats.CandidatesProcessed%100 == 0 || time.Since(lastProgress) >= 5*time.Second {
			emitProgress("processing", stats, sourceName)
			lastProgress = time.Now()
		}
	}
	if err := rows.Err(); err != nil {
		return stats, err
	}
	emitProgress("complete", stats, "")
	return stats, nil
}

type PruneJunkOrganizationsStats struct {
	Scanned       int
	JunkDetected  int
	Deleted       int
	SkippedKeptID int // confidence != possible, left in place
}

// PruneJunkOrganizations re-applies the junkOrganizationName heuristic to
// existing organization rows seeded with match_confidence = 'possible' and
// deletes those whose canonical name is junk and whose aliases are all junk.
// Rows with reviewer-edited confidence (anything other than 'possible') are
// preserved regardless. Returns counts for logging.
func (s *Store) PruneJunkOrganizations(ctx context.Context, dryRun bool) (PruneJunkOrganizationsStats, error) {
	const q = `
SELECT id, canonical_name, aliases, match_confidence::text
  FROM organization;`
	rows, err := s.Pool.Query(ctx, q)
	if err != nil {
		return PruneJunkOrganizationsStats{}, fmt.Errorf("list organizations for prune: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		id            int64
		canonicalName string
	}
	var stats PruneJunkOrganizationsStats
	var toDelete []candidate
	for rows.Next() {
		var id int64
		var canonical, confidence string
		var aliases []string
		if err := rows.Scan(&id, &canonical, &aliases, &confidence); err != nil {
			return stats, fmt.Errorf("scan organization: %w", err)
		}
		stats.Scanned++
		if confidence != "possible" {
			stats.SkippedKeptID++
			continue
		}
		// All names (canonical + aliases) must look like junk for the row to be
		// pruned. Normalization isn't critical here — junkOrganizationName
		// re-applies the rune-length and exact-match checks regardless.
		names := append([]string{canonical}, aliases...)
		allJunk := true
		for _, name := range names {
			if !junkOrganizationName(name, strings.TrimSpace(name)) {
				allJunk = false
				break
			}
		}
		if !allJunk {
			continue
		}
		stats.JunkDetected++
		toDelete = append(toDelete, candidate{id: id, canonicalName: canonical})
	}
	if err := rows.Err(); err != nil {
		return stats, err
	}
	if dryRun {
		stats.Deleted = len(toDelete)
		return stats, nil
	}
	if len(toDelete) == 0 {
		return stats, nil
	}
	// Batch the deletes; FK relationships are mostly ON DELETE CASCADE or
	// SET NULL (see migrations 0001/0010/0015/0018), so individual DELETEs suffice.
	for _, c := range toDelete {
		if _, err := s.Pool.Exec(ctx, `DELETE FROM organization WHERE id = $1`, c.id); err != nil {
			return stats, fmt.Errorf("delete organization %d (%q): %w", c.id, c.canonicalName, err)
		}
		stats.Deleted++
	}
	return stats, nil
}

// JunkOrganizationName exposes the heuristic so CLI commands can preview which
// names will be pruned without touching the DB.
func JunkOrganizationName(raw, normalized string) bool {
	return junkOrganizationName(raw, normalized)
}

func (s *Store) upsertOrganizationSourceMention(ctx context.Context, sourceKind, sourceTable string, sourcePK int64, sourceName, normalized string, orgID int64, count int, sourceRecordID int64, confidence string) error {
	const q = `
INSERT INTO organization_source_mention (source_kind, source_table, source_pk, source_name,
                                         normalized_name, organization_id, occurrence_count,
                                         confidence, source_record_id)
VALUES ($1,$2,NULLIF($3,0),$4,$5,NULLIF($6,0),$7,$8::org_match_confidence,NULLIF($9,0))
ON CONFLICT (source_kind, source_table, source_pk, source_name) DO UPDATE SET
  normalized_name = EXCLUDED.normalized_name,
  organization_id = EXCLUDED.organization_id,
  occurrence_count = EXCLUDED.occurrence_count,
  confidence = EXCLUDED.confidence,
  source_record_id = EXCLUDED.source_record_id,
  last_seen_at = NOW();`
	_, err := s.Pool.Exec(ctx, q, sourceKind, sourceTable, sourcePK, sourceName, normalized, orgID, count, defaultStr(confidence, "possible"), sourceRecordID)
	if err != nil {
		return fmt.Errorf("upsert organization_source_mention: %w", err)
	}
	return nil
}

type organizationNameMatch struct {
	ID            int64
	CanonicalName string
	Aliases       []string
}

func (s *Store) organizationMatchesForNormalizedName(ctx context.Context, normalized string) ([]organizationNameMatch, error) {
	const q = `
SELECT id, canonical_name, aliases
  FROM organization
 WHERE wa_dd_normalize_entity_name(canonical_name) = $1
    OR $1 = ANY(
       SELECT wa_dd_normalize_entity_name(alias)
         FROM unnest(aliases) alias
    )
 ORDER BY CASE WHEN match_confidence = 'confirmed' THEN 0 ELSE 1 END, canonical_name;`
	rows, err := s.Pool.Query(ctx, q, normalized)
	if err != nil {
		return nil, fmt.Errorf("organization normalized-name lookup: %w", err)
	}
	defer rows.Close()
	var out []organizationNameMatch
	for rows.Next() {
		var m organizationNameMatch
		if err := rows.Scan(&m.ID, &m.CanonicalName, &m.Aliases); err != nil {
			return nil, fmt.Errorf("scan organization normalized-name match: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) organizationIDForAliasOrCanonical(ctx context.Context, name string) (int64, error) {
	const q = `
SELECT id
  FROM organization
 WHERE lower(canonical_name) = lower($1)
    OR lower($1) = ANY(SELECT lower(alias) FROM unnest(aliases) alias)
 ORDER BY CASE WHEN match_confidence = 'confirmed' THEN 0 ELSE 1 END, id
 LIMIT 1;`
	var id int64
	err := s.Pool.QueryRow(ctx, q, name).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("organization alias lookup: %w", err)
	}
	return id, nil
}

// junkOrganizationName returns true when a CSI raw_organization string is
// almost certainly not a real organization — UI placeholder leakage,
// self-descriptors entered into the org column, single-character noise,
// hashtags, and similar. Source-local: returning true means we will skip
// seeding an organization row, but the underlying testifier row is preserved.
func junkOrganizationName(raw, normalized string) bool {
	r := strings.TrimSpace(raw)
	n := strings.TrimSpace(normalized)
	if r == "" || n == "" {
		return true
	}
	if len([]rune(n)) < 3 {
		return true
	}
	low := strings.ToLower(r)

	// Exact matches: short self-descriptors and form sentinels.
	if junkExactOrgNames[low] {
		return true
	}

	// Wrapped in parens or brackets ((Retired), [Self], "Home", etc.) — almost
	// always self-descriptors entered into the org column rather than a name.
	if junkWrappedOrgRE.MatchString(r) {
		return true
	}

	// Starts with '#': hashtag/handle noise (e.g. "#NotABot", "#169").
	if strings.HasPrefix(r, "#") {
		return true
	}

	// CSI form placeholder text leaking through verbatim.
	if junkPlaceholderRE.MatchString(low) {
		return true
	}

	// At least 50% of the runes must be letters; protects against pure-symbol
	// or pure-digit strings that slip past the length check.
	if !hasMinLetterRatio(r, 0.5) {
		return true
	}

	return false
}

var (
	// junkWrappedOrgRE matches strings entirely wrapped in (), [], or quotes.
	junkWrappedOrgRE = regexp.MustCompile(`^\s*[\(\[\"'][^\)\]\"']*[\)\]\"']\s*$`)
	// junkPlaceholderRE matches CSI form placeholder phrases.
	junkPlaceholderRE = regexp.MustCompile(`(please\s+select|make\s+a\s+selection|select\s+a\s+title|click\s+here|enter\s+(your|name|organization)|n\/?a$)`)
)

var junkExactOrgNames = map[string]bool{
	// Negations / explicit "no org".
	"none": true, "n/a": true, "na": true, "no": true,
	"not applicable": true, "no organization": true, "no affiliation": true,
	"none.": true, "n.a.": true, "n.a": true, "nada": true,
	// Self-as-org.
	"self": true, "myself": true, "me": true, "individual": true,
	"private citizen": true, "citizen": true, "concerned citizen": true,
	"private individual": true, "constituent": true, "voter": true,
	"resident": true, "homeowner": true, "home owner": true, "renter": true,
	"taxpayer": true, "retired": true, "student": true, "parent": true,
	"home": true, "work": true,
}

// ---------------------------------------------------------------------------
// IRS BMF + PDC employer ingestion (cross-source organization validation)
// ---------------------------------------------------------------------------
