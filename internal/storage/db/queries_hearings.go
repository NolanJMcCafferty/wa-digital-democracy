package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type UpsertHearingParams struct {
	BillID                    *int64
	CommitteeName             string
	CommitteeAcronym          string
	Chamber                   string
	MeetingDateTime           time.Time
	Location                  string
	LWSMeetingID              string
	CommitteeScheduleAgendaID string
	CommitteeScheduleVideoID  string
	TVWEventID                string
	OfficialAgendaURL         string
	TVWURL                    string
}

// UpsertHearing inserts a hearing; uses (chamber, meeting_datetime,
// committee_name) as a soft uniqueness key via WHERE-on-update.
func (s *Store) UpsertHearing(ctx context.Context, p UpsertHearingParams) (int64, error) {
	// Find existing first; if none, insert.
	const findQ = `
SELECT id FROM hearing
 WHERE chamber = $1 AND committee_name = $2 AND meeting_datetime = $3
 LIMIT 1;`
	var id int64
	err := s.Pool.QueryRow(ctx, findQ, p.Chamber, p.CommitteeName, p.MeetingDateTime).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		const insQ = `
INSERT INTO hearing (bill_id, committee_name, committee_acronym, chamber, meeting_datetime,
                     location, lws_meeting_id, committee_schedule_agenda_id,
                     committee_schedule_video_id, tvw_event_id, official_agenda_url,
                     tvw_url)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
RETURNING id;`
		err = s.Pool.QueryRow(ctx, insQ,
			p.BillID, p.CommitteeName, strOrNull(p.CommitteeAcronym),
			p.Chamber, p.MeetingDateTime, strOrNull(p.Location),
			strOrNull(p.LWSMeetingID), strOrNull(p.CommitteeScheduleAgendaID),
			strOrNull(p.CommitteeScheduleVideoID), strOrNull(p.TVWEventID),
			strOrNull(p.OfficialAgendaURL), strOrNull(p.TVWURL),
		).Scan(&id)
		if err != nil {
			return 0, fmt.Errorf("insert hearing: %w", err)
		}
		return id, nil
	}
	if err != nil {
		return 0, fmt.Errorf("find hearing: %w", err)
	}
	const updQ = `
UPDATE hearing SET
  bill_id = COALESCE($2, bill_id),
  committee_acronym = COALESCE($3, committee_acronym),
  location = COALESCE($4, location),
  lws_meeting_id = COALESCE($5, lws_meeting_id),
  committee_schedule_agenda_id = COALESCE($6, committee_schedule_agenda_id),
  committee_schedule_video_id  = COALESCE($7, committee_schedule_video_id),
  tvw_event_id = COALESCE($8, tvw_event_id),
  official_agenda_url = COALESCE($9, official_agenda_url),
  tvw_url = COALESCE($10, tvw_url),
  updated_at = NOW()
WHERE id = $1;`
	_, err = s.Pool.Exec(ctx, updQ, id,
		p.BillID, strOrNull(p.CommitteeAcronym), strOrNull(p.Location),
		strOrNull(p.LWSMeetingID), strOrNull(p.CommitteeScheduleAgendaID),
		strOrNull(p.CommitteeScheduleVideoID), strOrNull(p.TVWEventID),
		strOrNull(p.OfficialAgendaURL), strOrNull(p.TVWURL),
	)
	if err != nil {
		return 0, fmt.Errorf("update hearing: %w", err)
	}
	return id, nil
}

type UpsertAgendaItemParams struct {
	HearingID             int64
	BillID                *int64
	Label                 string
	CSIMeetingFamilyID    string
	CSIAgendaItemFamilyID string
	CSIAgendaItemID       string
	OrderIndex            int
}

// UpsertAgendaItem inserts/updates keyed on csi_agenda_item_id.
func (s *Store) UpsertAgendaItem(ctx context.Context, p UpsertAgendaItemParams) (int64, error) {
	const q = `
INSERT INTO agenda_item (hearing_id, bill_id, label, csi_meeting_family_id,
                         csi_agenda_item_family_id, csi_agenda_item_id,
                         order_index)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (csi_agenda_item_id) DO UPDATE SET
  hearing_id = EXCLUDED.hearing_id,
  bill_id    = EXCLUDED.bill_id,
  label      = EXCLUDED.label,
  csi_meeting_family_id     = EXCLUDED.csi_meeting_family_id,
  csi_agenda_item_family_id = EXCLUDED.csi_agenda_item_family_id,
  order_index               = EXCLUDED.order_index
RETURNING id;`
	var id int64
	err := s.Pool.QueryRow(ctx, q,
		p.HearingID, p.BillID, p.Label,
		strOrNull(p.CSIMeetingFamilyID), strOrNull(p.CSIAgendaItemFamilyID),
		p.CSIAgendaItemID, p.OrderIndex,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert agenda_item: %w", err)
	}
	return id, nil
}

type InsertTestifierParams struct {
	AgendaItemID    int64
	RawName         string
	RawOrganization string
	Position        string // testifier_position enum value
	Testified       bool
	TimeSignedIn    time.Time
}

// ReplaceTestifiersForAgenda is the idempotent upsert pattern: delete the
// agenda's current testifier rows and re-insert the fresh batch. CSI is the
// authoritative source for the *current* state; preserving stale rows would
// drift.
func (s *Store) ReplaceTestifiersForAgenda(ctx context.Context, agendaItemID int64, rows []InsertTestifierParams) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// ReplaceTestifiersForAgenda deletes and re-inserts testifier rows. Clean up
	// the derived person mention/affiliation rows tied to the old testifier IDs
	// first so repeated CSI ingest does not accumulate stale person facts.
	if _, err := tx.Exec(ctx, `
DELETE FROM person_organization_affiliation
 WHERE source_kind = 'csi_testifier'
   AND source_table = 'testifier'
   AND source_pk IN (SELECT id FROM testifier WHERE agenda_item_id = $1);`, agendaItemID); err != nil {
		return fmt.Errorf("delete CSI person affiliations: %w", err)
	}
	if _, err := tx.Exec(ctx, `
DELETE FROM person_source_mention
 WHERE source_kind = 'csi_testifier'
   AND source_table = 'testifier'
   AND source_pk IN (SELECT id FROM testifier WHERE agenda_item_id = $1);`, agendaItemID); err != nil {
		return fmt.Errorf("delete CSI person mentions: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM testifier WHERE agenda_item_id = $1`, agendaItemID); err != nil {
		return fmt.Errorf("delete testifiers: %w", err)
	}
	const insQ = `
INSERT INTO testifier (agenda_item_id, raw_name, raw_organization, position,
                       testified, time_signed_in)
VALUES ($1, $2, $3, $4::testifier_position, $5, $6)
RETURNING id;`
	for _, r := range rows {
		var testifierID int64
		if err := tx.QueryRow(ctx, insQ,
			r.AgendaItemID, r.RawName, strOrNull(r.RawOrganization),
			r.Position, r.Testified, timeOrNull(r.TimeSignedIn),
		).Scan(&testifierID); err != nil {
			return fmt.Errorf("insert testifier: %w", err)
		}
		if err := insertCSIPersonMentionAndAffiliation(ctx, tx, testifierID, r); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func insertCSIPersonMentionAndAffiliation(ctx context.Context, tx pgx.Tx, testifierID int64, r InsertTestifierParams) error {
	return upsertCSITestifierPersonAffiliation(ctx, tx, csiPersonAffiliationInput{
		TestifierID:     testifierID,
		AgendaItemID:    r.AgendaItemID,
		RawName:         r.RawName,
		RawOrganization: r.RawOrganization,
		Position:        r.Position,
		Testified:       r.Testified,
		TimeSignedIn:    r.TimeSignedIn,
	})
}

type csiPersonAffiliationInput struct {
	TestifierID     int64
	AgendaItemID    int64
	RawName         string
	RawOrganization string
	Position        string
	Testified       bool
	TimeSignedIn    time.Time
}

func upsertCSITestifierPersonAffiliation(ctx context.Context, tx pgx.Tx, in csiPersonAffiliationInput) error {
	rawName := strings.TrimSpace(in.RawName)
	if rawName == "" {
		return nil
	}
	normalizedName := strings.TrimSpace(normalizePersonName(rawName))
	if normalizedName == "" {
		normalizedName = rawName
	}

	contextJSON, err := json.Marshal(map[string]any{
		"agenda_item_id": in.AgendaItemID,
		"testifier_id":   in.TestifierID,
		"position":       in.Position,
		"testified":      in.Testified,
		"time_signed_in": zeroTimeToNil(in.TimeSignedIn),
	})
	if err != nil {
		return fmt.Errorf("marshal person mention context: %w", err)
	}
	sourceRowID := fmt.Sprintf("%d", in.TestifierID)

	const personQ = `
WITH existing AS (
  SELECT id
    FROM person
   WHERE normalized_name = $2
   ORDER BY id
   LIMIT 1
), inserted AS (
  INSERT INTO person (display_name, normalized_name, match_confidence, match_notes)
  SELECT $1, $2, 'possible', 'Auto-seeded from CSI testifier sign-in; source mentions share this normalized-name identity.'
   WHERE NOT EXISTS (SELECT 1 FROM existing)
  RETURNING id
)
SELECT id FROM inserted
UNION ALL
SELECT id FROM existing
LIMIT 1;`
	var personID int64
	if err := tx.QueryRow(ctx, personQ, rawName, normalizedName).Scan(&personID); err != nil {
		return fmt.Errorf("upsert CSI person: %w", err)
	}

	const mentionQ = `
INSERT INTO person_source_mention (
  person_id, source_kind, source_table, source_pk, source_row_id, source_name, normalized_name,
  source_role, context, confidence, review_status
)
VALUES ($1, 'csi_testifier', 'testifier', $2, $3, $4, $5,
        'testifier', $6, 'possible', 'auto')
ON CONFLICT (source_kind, source_table, source_pk, source_row_id, source_name) DO UPDATE SET
  person_id = EXCLUDED.person_id,
  normalized_name = COALESCE(person_source_mention.normalized_name, EXCLUDED.normalized_name),
  context = EXCLUDED.context,
  last_seen_at = NOW()
RETURNING id;`
	var mentionID int64
	if err := tx.QueryRow(ctx, mentionQ, personID, in.TestifierID, sourceRowID, rawName, normalizedName, contextJSON).Scan(&mentionID); err != nil {
		return fmt.Errorf("upsert CSI person mention: %w", err)
	}

	rawOrg := strings.TrimSpace(in.RawOrganization)
	var orgID *int64
	if rawOrg != "" {
		const orgQ = `
SELECT id
  FROM organization
 WHERE lower(trim(canonical_name)) = lower(trim($1))
    OR lower(trim($1)) = ANY(
         SELECT lower(trim(alias)) FROM unnest(COALESCE(aliases, ARRAY[]::text[])) AS alias
       )
 ORDER BY CASE WHEN lower(trim(canonical_name)) = lower(trim($1)) THEN 0 ELSE 1 END, id
 LIMIT 1;`
		var id int64
		err := tx.QueryRow(ctx, orgQ, rawOrg).Scan(&id)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("lookup CSI organization for affiliation: %w", err)
		}
		if err == nil {
			orgID = &id
		}
	}
	if rawOrg == "" && orgID == nil {
		return nil
	}

	relationshipType := "signed_in_for"
	if in.Testified {
		relationshipType = "testified_for"
	}
	rawOrgForAffiliation := rawOrg
	if rawOrgForAffiliation == "" {
		rawOrgForAffiliation = "<no organization>"
	}
	const affQ = `
INSERT INTO person_organization_affiliation (
  person_id, person_mention_id, organization_id, raw_person_name, raw_organization_name,
  relationship_type, role_title, source_kind, source_table, source_pk, source_row_id,
  context, confidence, review_status, evidence
)
VALUES ($1, $2, $3, $4, $5,
        $6::person_org_affiliation_type, 'testifier', 'csi_testifier', 'testifier', $7, $8,
        $9, 'possible', 'auto', $10)
ON CONFLICT (relationship_type, source_kind, source_table, source_pk, source_row_id, person_id, organization_id, raw_person_name, raw_organization_name) DO UPDATE SET
  person_mention_id = COALESCE(person_organization_affiliation.person_mention_id, EXCLUDED.person_mention_id),
  organization_id = COALESCE(person_organization_affiliation.organization_id, EXCLUDED.organization_id),
  context = EXCLUDED.context,
  evidence = EXCLUDED.evidence,
  updated_at = NOW();`
	evidenceJSON, err := json.Marshal([]string{
		fmt.Sprintf("CSI testifier %q signed in as %s for agenda_item_id=%d", rawName, defaultStr(rawOrg, "<no organization>"), in.AgendaItemID),
	})
	if err != nil {
		return fmt.Errorf("marshal affiliation evidence: %w", err)
	}
	if _, err := tx.Exec(ctx, affQ,
		personID, mentionID, orgID, rawName, rawOrgForAffiliation,
		relationshipType, in.TestifierID, sourceRowID,
		contextJSON, evidenceJSON,
	); err != nil {
		return fmt.Errorf("upsert CSI person affiliation: %w", err)
	}
	return nil
}

// BackfillCSITestifierPersonAffiliationsForRawOrganizations refreshes person
// mentions and affiliations for existing testifier rows whose organization link
// may have been set after the testifier was first ingested by PopulateOrganizations.
func (s *Store) BackfillCSITestifierPersonAffiliationsForRawOrganizations(ctx context.Context, rawOrgNames []string) error {
	if len(rawOrgNames) == 0 {
		return nil
	}
	lowers := make([]string, len(rawOrgNames))
	for i, n := range rawOrgNames {
		lowers[i] = lowerTrim(n)
	}
	const q = `
SELECT id, agenda_item_id, raw_name, COALESCE(raw_organization, ''), position::text,
       testified, time_signed_in
  FROM testifier
 WHERE lower(trim(raw_organization)) = ANY($1);`
	rows, err := s.Pool.Query(ctx, q, lowers)
	if err != nil {
		return fmt.Errorf("list CSI testifiers for person affiliation backfill: %w", err)
	}
	defer rows.Close()

	inputs := []csiPersonAffiliationInput{}
	for rows.Next() {
		var in csiPersonAffiliationInput
		var signedAt *time.Time
		if err := rows.Scan(&in.TestifierID, &in.AgendaItemID, &in.RawName, &in.RawOrganization, &in.Position, &in.Testified, &signedAt); err != nil {
			return fmt.Errorf("scan CSI testifier for person affiliation backfill: %w", err)
		}
		if signedAt != nil {
			in.TimeSignedIn = *signedAt
		}
		inputs = append(inputs, in)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(inputs) == 0 {
		return nil
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, in := range inputs {
		if err := upsertCSITestifierPersonAffiliation(ctx, tx, in); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func normalizePersonName(s string) string {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(s)))
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, " ")
}

func zeroTimeToNil(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC()
}

// ---------------------------------------------------------------------------
// tvw_event + diarized_speech_segment
// ---------------------------------------------------------------------------

// HearingAgendaItemAggregate is one agenda item/bill associated with a hearing.
type HearingAgendaItemAggregate struct {
	CSIAgendaItemID string
	AgendaItemLabel string
	Biennium        string
	BillID          string
	BillPrefix      string
	BillNumber      int
	TestifierCount  int
	TestifiedCount  int
}

// HearingAggregate is the row shape ListHearings returns: one row per
// hearing/committee meeting, with agenda items nested under it.
type HearingAggregate struct {
	HearingID       int64
	CommitteeName   string
	Chamber         string
	MeetingDateTime time.Time
	Location        string
	TVWURL          string
	TVWEventID      string
	AgendaItems     []HearingAgendaItemAggregate
	HasTVW          bool
}

// ListHearings returns every hearing with a TVW event mapping, ordered by
// meeting datetime descending. Agenda items are nested under each hearing.
func (s *Store) ListHearings(ctx context.Context) ([]HearingAggregate, error) {
	hits, _, err := s.SearchHearings(ctx, HearingSearchParams{Limit: 100000, Offset: 0})
	return hits, err
}

// HearingSearchParams are the filter knobs SearchHearings accepts. Empty
// strings/slices are no-ops. The handler is responsible for clamping
// limit and offset to safe ranges.
type HearingSearchParams struct {
	Committee     string   // ILIKE match on hearing.committee_name
	Bill          string   // ILIKE match on bill.bill_number or bill.title
	Speaker       string   // ILIKE match on testifier.raw_name
	Chambers      []string // OR-set of hearing.chamber values
	TopicKeywords []string // OR-set of ILIKE keywords matched against bill title/desc, agenda label, committee name
	Biennium      string   // exact match on bill.biennium
	Limit         int
	Offset        int
}

// HearingSearchFacets carries the distinct values we render in the
// hearings sidebar so the UI doesn't hard-code lists.
type HearingSearchFacets struct {
	Chambers   []string
	Committees []string
	Biennia    []string
}

// SearchHearings is the paginated, filtered query backing /api/v1/hearings.
// Returns (hits, total, err). The returned hits are hearing-level rows;
// filters that refer to bills, speakers, or topic keywords match if any
// agenda item under the hearing matches.
func (s *Store) SearchHearings(ctx context.Context, p HearingSearchParams) ([]HearingAggregate, int, error) {
	if p.Limit <= 0 {
		p.Limit = 50
	}
	if p.Offset < 0 {
		p.Offset = 0
	}

	args := []any{}
	where := []string{"h.tvw_event_id IS NOT NULL"}
	push := func(v any) int {
		args = append(args, v)
		return len(args)
	}
	if c := strings.TrimSpace(p.Committee); c != "" {
		idx := push(c)
		where = append(where, fmt.Sprintf("h.committee_name ILIKE '%%' || $%d || '%%'", idx))
	}
	if bq := strings.TrimSpace(p.Bill); bq != "" {
		idx := push(bq)
		where = append(where, fmt.Sprintf(`EXISTS (
			SELECT 1 FROM agenda_item a
			JOIN bill b ON b.id = a.bill_id
			WHERE a.hearing_id = h.id
			  AND (b.bill_number ILIKE '%%' || $%d || '%%' OR b.title ILIKE '%%' || $%d || '%%')
		)`, idx, idx))
	}
	if sp := strings.TrimSpace(p.Speaker); sp != "" {
		idx := push(sp)
		where = append(where, fmt.Sprintf(`EXISTS (
			SELECT 1 FROM agenda_item a
			JOIN testifier t ON t.agenda_item_id = a.id
			WHERE a.hearing_id = h.id
			  AND t.raw_name ILIKE '%%' || $%d || '%%'
		)`, idx))
	}
	if chambers := nonEmptyStrings(p.Chambers); len(chambers) > 0 {
		placeholders := make([]string, 0, len(chambers))
		for _, c := range chambers {
			placeholders = append(placeholders, fmt.Sprintf("$%d", push(c)))
		}
		where = append(where, "h.chamber IN ("+strings.Join(placeholders, ",")+")")
	}
	if keywords := nonEmptyStrings(p.TopicKeywords); len(keywords) > 0 {
		ors := make([]string, 0, len(keywords))
		for _, kw := range keywords {
			idx := push(kw)
			ors = append(ors, fmt.Sprintf(`(
				h.committee_name ILIKE '%%' || $%d || '%%'
				OR EXISTS (
					SELECT 1 FROM agenda_item a
					JOIN bill b ON b.id = a.bill_id
					WHERE a.hearing_id = h.id
					  AND (b.title ILIKE '%%' || $%d || '%%' OR COALESCE(b.description,'') ILIKE '%%' || $%d || '%%' OR a.label ILIKE '%%' || $%d || '%%')
				)
			)`, idx, idx, idx, idx))
		}
		where = append(where, "("+strings.Join(ors, " OR ")+")")
	}
	if p.Biennium != "" {
		idx := push(p.Biennium)
		where = append(where, fmt.Sprintf(`EXISTS (
			SELECT 1 FROM agenda_item a
			JOIN bill b ON b.id = a.bill_id
			WHERE a.hearing_id = h.id AND b.biennium = $%d
		)`, idx))
	}
	whereSQL := strings.Join(where, " AND ")

	countQ := "SELECT COUNT(*) FROM hearing h WHERE " + whereSQL
	var total int
	if err := s.Pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count hearings: %w", err)
	}

	args = append(args, p.Limit, p.Offset)
	q := `
SELECT h.id, h.committee_name, h.chamber, h.meeting_datetime,
       COALESCE(h.location, ''), COALESCE(h.tvw_url, ''), COALESCE(h.tvw_event_id, ''),
       (h.tvw_event_id IS NOT NULL) AS has_tvw
  FROM hearing h
 WHERE ` + whereSQL + `
 ORDER BY h.meeting_datetime DESC
 LIMIT $` + fmt.Sprintf("%d", len(args)-1) + ` OFFSET $` + fmt.Sprintf("%d", len(args))
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("search hearings: %w", err)
	}
	defer rows.Close()
	out := []HearingAggregate{}
	ids := make([]int64, 0)
	for rows.Next() {
		var hh HearingAggregate
		if err := rows.Scan(&hh.HearingID, &hh.CommitteeName, &hh.Chamber, &hh.MeetingDateTime,
			&hh.Location, &hh.TVWURL, &hh.TVWEventID, &hh.HasTVW); err != nil {
			return nil, 0, fmt.Errorf("scan hearing: %w", err)
		}
		ids = append(ids, hh.HearingID)
		out = append(out, hh)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if len(ids) > 0 {
		itemsByHearing, err := s.listAgendaItemsForHearings(ctx, ids)
		if err != nil {
			return nil, 0, err
		}
		for i := range out {
			out[i].AgendaItems = itemsByHearing[out[i].HearingID]
		}
	}
	return out, total, nil
}

func (s *Store) listAgendaItemsForHearings(ctx context.Context, hearingIDs []int64) (map[int64][]HearingAgendaItemAggregate, error) {
	rows, err := s.Pool.Query(ctx, `
SELECT a.hearing_id, COALESCE(a.csi_agenda_item_id, ''), a.label,
       COALESCE(b.biennium, ''), COALESCE(b.bill_number, ''), COALESCE(b.prefix, ''), COALESCE(b.number, 0),
       COUNT(t.id) AS testifier_count,
       COUNT(t.id) FILTER (WHERE t.testified) AS testified_count
  FROM agenda_item a
  LEFT JOIN bill b ON b.id = a.bill_id
  LEFT JOIN testifier t ON t.agenda_item_id = a.id
 WHERE a.hearing_id = ANY($1)
 GROUP BY a.hearing_id, a.id, b.biennium, b.bill_number, b.prefix, b.number
 ORDER BY a.hearing_id, a.order_index NULLS LAST, a.id`, hearingIDs)
	if err != nil {
		return nil, fmt.Errorf("list hearing agenda items: %w", err)
	}
	defer rows.Close()
	out := make(map[int64][]HearingAgendaItemAggregate, len(hearingIDs))
	for rows.Next() {
		var hearingID int64
		var item HearingAgendaItemAggregate
		if err := rows.Scan(&hearingID, &item.CSIAgendaItemID, &item.AgendaItemLabel,
			&item.Biennium, &item.BillID, &item.BillPrefix, &item.BillNumber,
			&item.TestifierCount, &item.TestifiedCount); err != nil {
			return nil, fmt.Errorf("scan hearing agenda item: %w", err)
		}
		out[hearingID] = append(out[hearingID], item)
	}
	return out, rows.Err()
}

// GetHearing returns one hearing/committee meeting by internal hearing ID,
// with all agenda items nested under it.
func (s *Store) GetHearing(ctx context.Context, hearingID int64) (*HearingAggregate, error) {
	const q = `
SELECT h.id, h.committee_name, h.chamber, h.meeting_datetime,
       COALESCE(h.location, ''), COALESCE(h.tvw_url, ''), COALESCE(h.tvw_event_id, ''),
       (h.tvw_event_id IS NOT NULL) AS has_tvw
  FROM hearing h
 WHERE h.id = $1
   AND h.tvw_event_id IS NOT NULL
 LIMIT 1;`
	var h HearingAggregate
	if err := s.Pool.QueryRow(ctx, q, hearingID).Scan(&h.HearingID, &h.CommitteeName, &h.Chamber,
		&h.MeetingDateTime, &h.Location, &h.TVWURL, &h.TVWEventID, &h.HasTVW); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}
		return nil, fmt.Errorf("get hearing: %w", err)
	}
	itemsByHearing, err := s.listAgendaItemsForHearings(ctx, []int64{hearingID})
	if err != nil {
		return nil, err
	}
	h.AgendaItems = itemsByHearing[hearingID]
	return &h, nil
}

// ListHearingSearchFacets returns the distinct chamber/committee/biennium
// values present in hearings with a TVW event mapping, for the sidebar.
func (s *Store) ListHearingSearchFacets(ctx context.Context) (HearingSearchFacets, error) {
	var f HearingSearchFacets

	rows, err := s.Pool.Query(ctx, `
SELECT DISTINCT h.chamber
  FROM hearing h
 WHERE h.tvw_event_id IS NOT NULL
   AND h.chamber IS NOT NULL AND h.chamber <> ''
 ORDER BY h.chamber`)
	if err != nil {
		return f, fmt.Errorf("facets hearing chamber: %w", err)
	}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return f, err
		}
		f.Chambers = append(f.Chambers, v)
	}
	rows.Close()

	rows, err = s.Pool.Query(ctx, `
SELECT DISTINCT h.committee_name
  FROM hearing h
 WHERE h.tvw_event_id IS NOT NULL
   AND h.committee_name IS NOT NULL AND h.committee_name <> ''
 ORDER BY h.committee_name`)
	if err != nil {
		return f, fmt.Errorf("facets hearing committee: %w", err)
	}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return f, err
		}
		f.Committees = append(f.Committees, v)
	}
	rows.Close()

	rows, err = s.Pool.Query(ctx, `
SELECT DISTINCT b.biennium
  FROM agenda_item a
  JOIN hearing h ON h.id = a.hearing_id
  JOIN bill    b ON b.id = a.bill_id
 WHERE h.tvw_event_id IS NOT NULL
   AND b.biennium <> ''
 ORDER BY b.biennium DESC`)
	if err != nil {
		return f, fmt.Errorf("facets hearing biennium: %w", err)
	}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return f, err
		}
		f.Biennia = append(f.Biennia, v)
	}
	rows.Close()

	return f, nil
}

// HearingForDiscovery is one row to feed into the Discoverer. We hand
// out only the fields we need to match against CSI/TVW and the
// hearing.id we'll write back into.
type HearingForDiscovery struct {
	HearingID        int64
	BillID           int64
	BillPrefix       string
	BillNumber       int
	CommitteeName    string
	CommitteeAcronym string
	Chamber          string
	MeetingDateTime  time.Time
}

// ListHearingsForDiscovery returns hearings whose CSI/TVW IDs are still
// blank for bills in the given biennium. These are the candidates for
// auto-discovery. Hearings already enriched (have either a TVW event ID
// or a Committee Schedules agenda ID) are skipped. Gubernatorial
// appointments (SGA) are also skipped: LWS stores them as bill-like rows,
// but CSI does not expose them as testimony agenda items in the data this
// pipeline ingests.
func (s *Store) ListHearingsForDiscovery(ctx context.Context, biennium string) ([]HearingForDiscovery, error) {
	const q = `
SELECT h.id, b.id, b.prefix, b.number,
       h.committee_name, COALESCE(h.committee_acronym, ''), h.chamber,
       h.meeting_datetime
  FROM hearing h
  JOIN bill b ON b.id = h.bill_id
 WHERE b.biennium = $1
   AND h.tvw_event_id IS NULL
   AND h.committee_schedule_agenda_id IS NULL
   AND b.prefix <> 'SGA'
 ORDER BY h.meeting_datetime DESC;`
	rows, err := s.Pool.Query(ctx, q, biennium)
	if err != nil {
		return nil, fmt.Errorf("list hearings for discovery: %w", err)
	}
	defer rows.Close()
	out := []HearingForDiscovery{}
	for rows.Next() {
		var h HearingForDiscovery
		if err := rows.Scan(
			&h.HearingID, &h.BillID, &h.BillPrefix, &h.BillNumber,
			&h.CommitteeName, &h.CommitteeAcronym, &h.Chamber,
			&h.MeetingDateTime,
		); err != nil {
			return nil, fmt.Errorf("scan hearing: %w", err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// DiscoveredHearingRow is one hearing ready for hearing-level ingestion.
// Hearing ingestion is grouped at this level because TVW media and
// diarization are event-level while CSI testifiers and segmentation remain
// agenda-item-level.
type DiscoveredHearingRow struct {
	HearingID       int64
	TVWEventID      string
	MeetingDateTime time.Time
	AgendaItemCount int
}

// HearingAgendaItemRow is one agenda item attached to a discovered hearing,
// with enough bill and CSI metadata to run CSI ingestion and transcript
// segmentation without re-fetching bill metadata from LWS.
type HearingAgendaItemRow struct {
	AgendaItemID          int64
	BillID                int64
	Biennium              string
	BillPrefix            string
	BillNumber            int
	CommitteeName         string
	CommitteeAcronym      string
	Chamber               string
	TVWEventID            string
	CSIMeetingFamilyID    string
	CSIAgendaItemFamilyID string
	CSIAgendaItemID       string
	Label                 string
}

// ListDiscoveredHearingsForIngest returns one row per hearing that discovery
// has mapped to a TVW event and at least one CSI agenda item, and that has not
// completed the hearing-level ingest pipeline yet.
func (s *Store) ListDiscoveredHearingsForIngest(ctx context.Context, biennium string) ([]DiscoveredHearingRow, error) {
	const q = `
SELECT h.id, h.tvw_event_id, h.meeting_datetime, COUNT(a.id)::int AS agenda_item_count
  FROM hearing h
  JOIN agenda_item a ON a.hearing_id = h.id
  JOIN bill b ON b.id = a.bill_id
 WHERE b.biennium = $1
   AND h.tvw_event_id IS NOT NULL
   AND NULLIF(a.csi_agenda_item_id, '') IS NOT NULL
   AND NOT EXISTS (
     SELECT 1 FROM ingestion_run r
      WHERE r.job = 'hearing-pipeline'
        AND r.status = 'succeeded'
        AND r.args ->> 'hearing_id' = h.id::text
   )
 GROUP BY h.id, h.tvw_event_id, h.meeting_datetime
 ORDER BY h.meeting_datetime DESC;`
	rows, err := s.Pool.Query(ctx, q, biennium)
	if err != nil {
		return nil, fmt.Errorf("list discovered hearings for ingest: %w", err)
	}
	defer rows.Close()
	out := []DiscoveredHearingRow{}
	for rows.Next() {
		var r DiscoveredHearingRow
		if err := rows.Scan(&r.HearingID, &r.TVWEventID, &r.MeetingDateTime, &r.AgendaItemCount); err != nil {
			return nil, fmt.Errorf("scan discovered hearing: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListAgendaItemsForHearing returns every agenda item for a hearing with the
// bill identity needed for per-item CSI ingest and transcript segmentation.
func (s *Store) ListAgendaItemsForHearing(ctx context.Context, hearingID int64) ([]HearingAgendaItemRow, error) {
	const q = `
SELECT a.id, b.id, b.biennium, b.prefix, b.number,
       h.committee_name, COALESCE(h.committee_acronym, ''), h.chamber,
       COALESCE(h.tvw_event_id, ''),
       COALESCE(a.csi_meeting_family_id, ''),
       COALESCE(a.csi_agenda_item_family_id, ''),
       COALESCE(a.csi_agenda_item_id, ''),
       a.label
  FROM agenda_item a
  JOIN hearing h ON h.id = a.hearing_id
  JOIN bill b ON b.id = a.bill_id
 WHERE h.id = $1
   AND NULLIF(a.csi_agenda_item_id, '') IS NOT NULL
 ORDER BY a.order_index NULLS LAST, a.id;`
	rows, err := s.Pool.Query(ctx, q, hearingID)
	if err != nil {
		return nil, fmt.Errorf("list agenda items for hearing: %w", err)
	}
	defer rows.Close()
	out := []HearingAgendaItemRow{}
	for rows.Next() {
		var r HearingAgendaItemRow
		if err := rows.Scan(
			&r.AgendaItemID, &r.BillID, &r.Biennium, &r.BillPrefix, &r.BillNumber,
			&r.CommitteeName, &r.CommitteeAcronym, &r.Chamber, &r.TVWEventID,
			&r.CSIMeetingFamilyID, &r.CSIAgendaItemFamilyID, &r.CSIAgendaItemID,
			&r.Label,
		); err != nil {
			return nil, fmt.Errorf("scan hearing agenda item: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Transcript full-text search — backs `/api/v1/search/transcripts`.
// ---------------------------------------------------------------------------
