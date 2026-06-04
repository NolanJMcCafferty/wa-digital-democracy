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
  aliases                   = (
    SELECT COALESCE(array_agg(alias ORDER BY first_seen), ARRAY[]::text[])
      FROM (
        SELECT DISTINCT ON (lower(trim(alias))) trim(alias) AS alias, ord AS first_seen
          FROM unnest(organization.aliases || EXCLUDED.aliases) WITH ORDINALITY AS u(alias, ord)
         WHERE NULLIF(trim(alias), '') IS NOT NULL
         ORDER BY lower(trim(alias)), ord
      ) merged
  ),
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

// AddOrganizationAlias appends aliasName to organization.aliases when it is a
// non-empty name distinct from the canonical name and existing aliases.
func (s *Store) AddOrganizationAlias(ctx context.Context, orgID int64, aliasName string) error {
	const q = `
UPDATE organization o
   SET aliases = (
     SELECT COALESCE(array_agg(alias ORDER BY first_seen), ARRAY[]::text[])
       FROM (
         SELECT DISTINCT ON (lower(trim(alias))) trim(alias) AS alias, ord AS first_seen
           FROM unnest(o.aliases || ARRAY[$2]::text[]) WITH ORDINALITY AS u(alias, ord)
          WHERE NULLIF(trim(alias), '') IS NOT NULL
            AND lower(trim(alias)) <> lower(trim(o.canonical_name))
          ORDER BY lower(trim(alias)), ord
       ) merged
   ),
       updated_at = NOW()
 WHERE o.id = $1
   AND NULLIF(trim($2), '') IS NOT NULL
   AND lower(trim($2)) <> lower(trim(o.canonical_name));`
	if _, err := s.Pool.Exec(ctx, q, orgID, aliasName); err != nil {
		return fmt.Errorf("add organization alias: %w", err)
	}
	return nil
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
	MatchConfidence string
	Evidence        []string
}

type OrganizationPersonAffiliation struct {
	PersonID            int64
	PersonName          string
	RelationshipType    string
	RoleTitle           string
	SourceKind          string
	SourceLabel         string
	RawOrganizationName string
	RecordYears         string
	SourceCount         int
	Confidence          string
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

func (s *Store) GetOrganizationPersonAffiliations(ctx context.Context, organizationID int64) ([]OrganizationPersonAffiliation, error) {
	const q = `
SELECT COALESCE(p.id, 0) AS person_id,
       COALESCE(NULLIF(p.display_name, ''), NULLIF(poa.raw_person_name, ''), 'Unknown person') AS person_name,
       poa.relationship_type::text,
       COALESCE(NULLIF(poa.role_title, ''), '') AS role_title,
       poa.source_kind::text,
       CASE poa.source_kind::text
         WHEN 'csi_testifier' THEN 'Committee Sign In'
         WHEN 'pdc_lobbyist_employment' THEN 'PDC lobbyist employment'
         WHEN 'pdc_lobbyist_compensation' THEN 'PDC lobbyist compensation'
         WHEN 'pdc_contribution' THEN 'PDC contribution'
         WHEN 'webs_vendor_contact' THEN 'WEBS vendor contact'
         WHEN 'deepgram_speaker' THEN 'Transcript speaker'
         WHEN 'legislator_roster' THEN 'Legislator roster'
         ELSE poa.source_kind::text
       END AS source_label,
       COALESCE(NULLIF(poa.raw_organization_name, ''), '') AS raw_organization_name,
       COALESCE(
         array_to_string(
           array_agg(DISTINCT poa.record_year ORDER BY poa.record_year DESC)
             FILTER (WHERE poa.record_year IS NOT NULL),
           ', '
         ),
         ''
       ) AS record_years,
       COUNT(*)::int AS source_count,
       CASE
         WHEN bool_or(poa.confidence = 'confirmed') THEN 'confirmed'
         WHEN bool_or(poa.confidence = 'probable') THEN 'probable'
         WHEN bool_or(poa.confidence = 'possible') THEN 'possible'
         ELSE 'unmatched'
       END AS confidence
  FROM person_organization_affiliation poa
  LEFT JOIN person p ON p.id = poa.person_id
 WHERE poa.organization_id = $1
 GROUP BY p.id, p.display_name, poa.raw_person_name, poa.relationship_type,
          poa.role_title, poa.source_kind, poa.raw_organization_name
 ORDER BY CASE poa.relationship_type::text
            WHEN 'lobbyist_for' THEN 0
            WHEN 'testified_for' THEN 1
            WHEN 'signed_in_for' THEN 2
            ELSE 3
          END,
          person_name;`
	rows, err := s.Pool.Query(ctx, q, organizationID)
	if err != nil {
		return nil, fmt.Errorf("organization person affiliations: %w", err)
	}
	defer rows.Close()
	out := []OrganizationPersonAffiliation{}
	for rows.Next() {
		var a OrganizationPersonAffiliation
		if err := rows.Scan(&a.PersonID, &a.PersonName, &a.RelationshipType, &a.RoleTitle,
			&a.SourceKind, &a.SourceLabel, &a.RawOrganizationName, &a.RecordYears,
			&a.SourceCount, &a.Confidence); err != nil {
			return nil, fmt.Errorf("scan organization person affiliation: %w", err)
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
       ORDER BY pe2.last_employment_year DESC NULLS LAST, pe2.id DESC
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
         0,
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
         0,
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
         0,
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
         0,
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
         0,
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
         0,
         m.match_confidence::text,
         m.evidence
    FROM reviewed_vendor_entity_match m
    JOIN federal_award a ON a.id = m.source_pk
   WHERE m.organization_id = $1
     AND m.source_kind = 'federal_award_recipient'
)
SELECT DISTINCT ON (source_kind, source_name, detail)
       context_type, source_kind, source_label, COALESCE(source_name, ''),
       COALESCE(detail, ''), amount, COALESCE(record_year, 0), record_date,
       url, match_confidence, evidence
  FROM ctx
 ORDER BY source_kind, source_name, detail,
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
			&c.MatchConfidence, &evidence); err != nil {
			return nil, fmt.Errorf("scan organization public context: %w", err)
		}
		if len(evidence) > 0 {
			_ = json.Unmarshal(evidence, &c.Evidence)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// OrganizationTranscriptQuote represents a reviewed transcript quote attributed to an organization.
type OrganizationTranscriptQuote struct {
	ID               int64     `json:"id"`
	Biennium         string    `json:"biennium,omitempty"`
	BillID           string    `json:"bill_id,omitempty"`
	BillPrefix       string    `json:"bill_prefix,omitempty"`
	BillNumber       int       `json:"bill_number,omitempty"`
	AgendaItemLabel  string    `json:"agenda_item_label,omitempty"`
	CommitteeName    string    `json:"committee_name,omitempty"`
	MeetingDateTime  time.Time `json:"meeting_datetime,omitempty"`
	StartMS          int       `json:"start_ms"`
	EndMS            int       `json:"end_ms"`
	Text             string    `json:"text"`
	SpeakerLabel     string    `json:"speaker_label"`
	SpeakerKind      string    `json:"speaker_kind"`
	TVWEventID       string    `json:"tvw_event_id,omitempty"`
	TestifierName    string    `json:"testifier_name,omitempty"`
	TestifierID      int64     `json:"testifier_id,omitempty"`
	ReviewStatus     string    `json:"review_status"`
}

// GetOrganizationTranscriptQuotes returns transcript quotes linked to an organization
// through reviewed speaker assignments. Only includes quotes from accepted speaker
// assignments where the speaker was identified as a testifier for that organization.
func (s *Store) GetOrganizationTranscriptQuotes(ctx context.Context, organizationID int64) ([]OrganizationTranscriptQuote, error) {
	const q = `
WITH org_testifiers AS (
  -- Get all testifiers linked to this organization
  SELECT t.id AS testifier_id, t.raw_name, t.agenda_item_id
    FROM testifier t
   WHERE t.normalized_org_id = $1
),
quote_segments AS (
  -- Get diarized segments with accepted speaker assignments matching org testifiers
  SELECT d.id, d.start_ms, d.end_ms, d.text, d.tvw_event_id,
         sa.speaker_label, sa.speaker_kind, sa.review_status,
         ot.testifier_id, ot.raw_name AS testifier_name,
         ot.agenda_item_id
    FROM diarized_speech_segment d
    JOIN speaker_assignment sa 
      ON sa.diarization_job_id = d.diarization_job_id 
     AND sa.speaker_cluster_id = d.speaker_cluster_id
     AND sa.review_status = 'accepted'
     AND sa.speaker_kind = 'testifier'
    JOIN org_testifiers ot
      ON sa.speaker_id = ot.testifier_id
   WHERE d.text IS NOT NULL
)
SELECT 
  qs.id,
  COALESCE(b.biennium, '') AS biennium,
  COALESCE(b.bill_number, '') AS bill_id,
  COALESCE(b.prefix, '') AS bill_prefix,
  COALESCE(b.number, 0) AS bill_number,
  COALESCE(a.label, '') AS agenda_item_label,
  COALESCE(h.committee_name, '') AS committee_name,
  h.meeting_datetime,
  qs.start_ms,
  qs.end_ms,
  qs.text,
  qs.speaker_label,
  qs.speaker_kind,
  qs.tvw_event_id,
  qs.testifier_name,
  qs.testifier_id,
  qs.review_status
FROM quote_segments qs
LEFT JOIN hearing h ON h.tvw_event_id = qs.tvw_event_id
LEFT JOIN agenda_item a ON a.id = qs.agenda_item_id
LEFT JOIN bill b ON b.id = a.bill_id
ORDER BY h.meeting_datetime DESC NULLS LAST, qs.start_ms ASC;`

	rows, err := s.Pool.Query(ctx, q, organizationID)
	if err != nil {
		return nil, fmt.Errorf("organization transcript quotes: %w", err)
	}
	defer rows.Close()
	
	out := []OrganizationTranscriptQuote{}
	for rows.Next() {
		var q OrganizationTranscriptQuote
		var meetingTS *time.Time
		if err := rows.Scan(&q.ID, &q.Biennium, &q.BillID, &q.BillPrefix, &q.BillNumber,
			&q.AgendaItemLabel, &q.CommitteeName, &meetingTS,
			&q.StartMS, &q.EndMS, &q.Text, &q.SpeakerLabel, &q.SpeakerKind,
			&q.TVWEventID, &q.TestifierName, &q.TestifierID, &q.ReviewStatus); err != nil {
			return nil, fmt.Errorf("scan organization transcript quote: %w", err)
		}
		if meetingTS != nil {
			q.MeetingDateTime = *meetingTS
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

type PopulateOrganizationsStats struct {
	CandidatesProcessed   int
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

// PopulateOrganizationsFromCSIWithProgress seeds organization rows from
// distinct testifier.raw_organization values and links all matching testifier
// rows to the resulting organization. It is intentionally source-local: no
// cross-source merge is asserted here.
func (s *Store) PopulateOrganizationsFromCSIWithProgress(ctx context.Context, progress func(PopulateOrganizationsProgress)) (PopulateOrganizationsStats, error) {
	return s.populateOrganizationsFromCSI(ctx, 0, progress)
}

func (s *Store) PopulateOrganizationsFromCSIForHearing(ctx context.Context, hearingID int64) (PopulateOrganizationsStats, error) {
	if hearingID == 0 {
		return PopulateOrganizationsStats{}, errors.New("populate organizations for hearing: hearing id required")
	}
	return s.populateOrganizationsFromCSI(ctx, hearingID, nil)
}

func (s *Store) populateOrganizationsFromCSI(ctx context.Context, hearingID int64, progress func(PopulateOrganizationsProgress)) (PopulateOrganizationsStats, error) {
	const q = `
WITH orgs AS (
    SELECT trim(t.raw_organization) AS source_name,
           wa_dd_normalize_entity_name(trim(t.raw_organization)) AS normalized_name,
           COUNT(*) AS occurrence_count
      FROM testifier t
      JOIN agenda_item a ON a.id = t.agenda_item_id
     WHERE NULLIF(trim(t.raw_organization), '') IS NOT NULL
       AND ($1::bigint = 0 OR a.hearing_id = $1)
     GROUP BY trim(t.raw_organization), wa_dd_normalize_entity_name(trim(t.raw_organization))
)
SELECT source_name, normalized_name
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
	rows, err := s.Pool.Query(ctx, q, hearingID)
	if err != nil {
		return PopulateOrganizationsStats{}, fmt.Errorf("list CSI organizations: %w", err)
	}
	defer rows.Close()

	var stats PopulateOrganizationsStats
	emitProgress("processing", stats, "")
	lastProgress := time.Now()
	for rows.Next() {
		var sourceName string
		var normalized *string
		if err := rows.Scan(&sourceName, &normalized); err != nil {
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
		stats.OrganizationsUpserted++
		if match, ok, err := s.LookupOrgCrossSource(ctx, *normalized); err != nil {
			return stats, err
		} else if ok {
			if err := s.MarkOrganizationVerified(ctx, orgID, match); err != nil {
				return stats, err
			}
			stats.Verified++
		}
		if err := s.AttachPDCEmployerAffiliationsToOrganization(ctx, orgID, "", sourceName); err != nil {
			return stats, err
		}
		var linked int64
		if hearingID == 0 {
			linked, err = s.LinkTestifiersToOrg(ctx, orgID, []string{sourceName})
		} else {
			linked, err = s.linkTestifiersToOrgForHearing(ctx, hearingID, orgID, []string{sourceName})
		}
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

func (s *Store) linkTestifiersToOrgForHearing(ctx context.Context, hearingID, orgID int64, rawOrgNames []string) (int64, error) {
	if len(rawOrgNames) == 0 {
		return 0, nil
	}
	lowers := make([]string, len(rawOrgNames))
	for i, n := range rawOrgNames {
		lowers[i] = lowerTrim(n)
	}
	const q = `
UPDATE testifier t
   SET normalized_org_id = $1
  FROM agenda_item a
 WHERE a.id = t.agenda_item_id
   AND a.hearing_id = $2
   AND lower(trim(t.raw_organization)) = ANY($3);`
	tag, err := s.Pool.Exec(ctx, q, orgID, hearingID, lowers)
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
	// SET NULL (see migrations 0001/0010/0018), so individual DELETEs suffice.
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
