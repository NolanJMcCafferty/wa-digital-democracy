package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// timeOrNull returns t for non-zero times, or pgtype Null otherwise. We
// use pgtype.Timestamptz because nullable TIMESTAMPTZ fields don't accept
// Go's zero time.Time gracefully.
func timeOrNull(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// dateOrNull returns d as a pg date or null when zero.
func dateOrNull(t time.Time) pgtype.Date {
	if t.IsZero() {
		return pgtype.Date{Valid: false}
	}
	return pgtype.Date{Time: t, Valid: true}
}

// strOrNull returns the string when non-empty or NULL otherwise.
func strOrNull(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ---------------------------------------------------------------------------
// bill
// ---------------------------------------------------------------------------

type UpsertBillParams struct {
	Biennium       string
	Prefix         string
	Number         int
	Title          string
	Description    string
	ChamberOrigin  string
	CurrentStatus  string
	StatusDate     time.Time
	OfficialURL    string
	SourceRecordID int64
}

// UpsertBill inserts/updates a bill keyed on (biennium, prefix, number).
func (s *Store) UpsertBill(ctx context.Context, p UpsertBillParams) (int64, error) {
	const q = `
INSERT INTO bill (biennium, prefix, number, title, description, chamber_origin,
                  current_status, status_date, official_url, source_record_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (biennium, prefix, number) DO UPDATE SET
  title          = EXCLUDED.title,
  description    = EXCLUDED.description,
  chamber_origin = EXCLUDED.chamber_origin,
  current_status = EXCLUDED.current_status,
  status_date    = EXCLUDED.status_date,
  official_url   = EXCLUDED.official_url,
  source_record_id = EXCLUDED.source_record_id,
  updated_at     = NOW()
RETURNING id;`
	var id int64
	err := s.Pool.QueryRow(ctx, q,
		p.Biennium, p.Prefix, p.Number,
		strOrNull(p.Title), strOrNull(p.Description), strOrNull(p.ChamberOrigin),
		strOrNull(p.CurrentStatus), dateOrNull(p.StatusDate), strOrNull(p.OfficialURL),
		p.SourceRecordID,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert bill: %w", err)
	}
	return id, nil
}

// ---------------------------------------------------------------------------
// legislator + bill_sponsor
// ---------------------------------------------------------------------------

type UpsertLegislatorParams struct {
	LWSSponsorID string
	Name         string
	Chamber      string
	District     string
	Party        string
	OfficialURL  string
}

func (s *Store) UpsertLegislator(ctx context.Context, p UpsertLegislatorParams) (int64, error) {
	const q = `
INSERT INTO legislator (lws_sponsor_id, name, chamber, district, party, official_url)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (lws_sponsor_id) DO UPDATE SET
  name = EXCLUDED.name,
  chamber = EXCLUDED.chamber,
  district = COALESCE(EXCLUDED.district, legislator.district),
  party = COALESCE(EXCLUDED.party, legislator.party),
  official_url = COALESCE(EXCLUDED.official_url, legislator.official_url),
  updated_at = NOW()
RETURNING id;`
	var id int64
	err := s.Pool.QueryRow(ctx, q,
		strOrNull(p.LWSSponsorID), p.Name, strOrNull(p.Chamber),
		strOrNull(p.District), strOrNull(p.Party), strOrNull(p.OfficialURL),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert legislator: %w", err)
	}
	return id, nil
}

func (s *Store) UpsertBillSponsor(ctx context.Context, billID, legislatorID int64, sponsorType string) error {
	const q = `
INSERT INTO bill_sponsor (bill_id, legislator_id, sponsor_type)
VALUES ($1, $2, $3)
ON CONFLICT (bill_id, legislator_id, sponsor_type) DO NOTHING;`
	_, err := s.Pool.Exec(ctx, q, billID, legislatorID, sponsorType)
	return err
}

// ---------------------------------------------------------------------------
// hearing + agenda_item + testifier
// ---------------------------------------------------------------------------

type UpsertHearingParams struct {
	BillID                       *int64
	CommitteeName                string
	CommitteeAcronym             string
	Chamber                      string
	MeetingDateTime              time.Time
	Location                     string
	LWSMeetingID                 string
	CommitteeScheduleAgendaID    string
	CommitteeScheduleVideoID     string
	TVWEventID                   string
	OfficialAgendaURL            string
	TVWURL                       string
	SourceRecordID               int64
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
                     tvw_url, source_record_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
RETURNING id;`
		err = s.Pool.QueryRow(ctx, insQ,
			p.BillID, p.CommitteeName, strOrNull(p.CommitteeAcronym),
			p.Chamber, p.MeetingDateTime, strOrNull(p.Location),
			strOrNull(p.LWSMeetingID), strOrNull(p.CommitteeScheduleAgendaID),
			strOrNull(p.CommitteeScheduleVideoID), strOrNull(p.TVWEventID),
			strOrNull(p.OfficialAgendaURL), strOrNull(p.TVWURL),
			p.SourceRecordID,
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
	HearingID              int64
	BillID                 *int64
	Label                  string
	CSIMeetingFamilyID     string
	CSIAgendaItemFamilyID  string
	CSIAgendaItemID        string
	OrderIndex             int
	SourceRecordID         int64
}

// UpsertAgendaItem inserts/updates keyed on csi_agenda_item_id.
func (s *Store) UpsertAgendaItem(ctx context.Context, p UpsertAgendaItemParams) (int64, error) {
	const q = `
INSERT INTO agenda_item (hearing_id, bill_id, label, csi_meeting_family_id,
                         csi_agenda_item_family_id, csi_agenda_item_id,
                         order_index, source_record_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (csi_agenda_item_id) DO UPDATE SET
  hearing_id = EXCLUDED.hearing_id,
  bill_id    = EXCLUDED.bill_id,
  label      = EXCLUDED.label,
  csi_meeting_family_id     = EXCLUDED.csi_meeting_family_id,
  csi_agenda_item_family_id = EXCLUDED.csi_agenda_item_family_id,
  order_index               = EXCLUDED.order_index,
  source_record_id          = EXCLUDED.source_record_id
RETURNING id;`
	var id int64
	err := s.Pool.QueryRow(ctx, q,
		p.HearingID, p.BillID, p.Label,
		strOrNull(p.CSIMeetingFamilyID), strOrNull(p.CSIAgendaItemFamilyID),
		p.CSIAgendaItemID, p.OrderIndex, p.SourceRecordID,
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
	SourceRecordID  int64
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

	if _, err := tx.Exec(ctx, `DELETE FROM testifier WHERE agenda_item_id = $1`, agendaItemID); err != nil {
		return fmt.Errorf("delete testifiers: %w", err)
	}
	const insQ = `
INSERT INTO testifier (agenda_item_id, raw_name, raw_organization, position,
                       testified, time_signed_in, source_record_id)
VALUES ($1, $2, $3, $4::testifier_position, $5, $6, $7);`
	for _, r := range rows {
		if _, err := tx.Exec(ctx, insQ,
			r.AgendaItemID, r.RawName, strOrNull(r.RawOrganization),
			r.Position, r.Testified, timeOrNull(r.TimeSignedIn), r.SourceRecordID,
		); err != nil {
			return fmt.Errorf("insert testifier: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// ---------------------------------------------------------------------------
// tvw_event + transcript_segment
// ---------------------------------------------------------------------------

type UpsertTVWEventParams struct {
	TVWEventID     string
	WPPostID       *int64
	Title          string
	Description    string
	StartDateTime  time.Time
	CaptionURL     string
	ThumbnailURL   string
	RawCategories  []string
	SourceRecordID int64
}

func (s *Store) UpsertTVWEvent(ctx context.Context, p UpsertTVWEventParams) (int64, error) {
	const q = `
INSERT INTO tvw_event (tvw_event_id, wp_post_id, title, description, start_datetime,
                       caption_url, thumbnail_url, raw_categories, source_record_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (tvw_event_id) DO UPDATE SET
  wp_post_id     = COALESCE(EXCLUDED.wp_post_id, tvw_event.wp_post_id),
  title          = EXCLUDED.title,
  description    = EXCLUDED.description,
  start_datetime = EXCLUDED.start_datetime,
  caption_url    = EXCLUDED.caption_url,
  thumbnail_url  = EXCLUDED.thumbnail_url,
  raw_categories = EXCLUDED.raw_categories,
  source_record_id = EXCLUDED.source_record_id,
  updated_at     = NOW()
RETURNING id;`
	var id int64
	err := s.Pool.QueryRow(ctx, q,
		p.TVWEventID, p.WPPostID, strOrNull(p.Title), strOrNull(p.Description),
		timeOrNull(p.StartDateTime), strOrNull(p.CaptionURL), strOrNull(p.ThumbnailURL),
		p.RawCategories, p.SourceRecordID,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert tvw_event: %w", err)
	}
	return id, nil
}

type InsertTranscriptSegmentParams struct {
	TVWEventID        string
	AgendaItemID      *int64
	StartMS           int
	EndMS             int
	Text              string
	SpeakerLabel      string
	SpeakerEntityID   *int64
	SpeakerConfidence string // speaker_confidence enum
	SourceCaptionURL  string
	SourceRecordID    int64
}

// ReplaceTranscriptSegments wipes and reloads all segments for an event —
// captions are immutable per fetch, but we may re-fetch corrected versions.
func (s *Store) ReplaceTranscriptSegments(ctx context.Context, tvwEventID string, rows []InsertTranscriptSegmentParams) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM transcript_segment WHERE tvw_event_id = $1`, tvwEventID); err != nil {
		return fmt.Errorf("delete segments: %w", err)
	}
	const insQ = `
INSERT INTO transcript_segment (tvw_event_id, agenda_item_id, start_ms, end_ms, text,
                                speaker_label, speaker_entity_id, speaker_confidence,
                                source_caption_url, source_record_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8::speaker_confidence,$9,$10);`
	for _, r := range rows {
		if _, err := tx.Exec(ctx, insQ,
			r.TVWEventID, r.AgendaItemID, r.StartMS, r.EndMS, r.Text,
			strOrNull(r.SpeakerLabel), r.SpeakerEntityID,
			defaultStr(r.SpeakerConfidence, "unknown_speaker"),
			r.SourceCaptionURL, r.SourceRecordID,
		); err != nil {
			return fmt.Errorf("insert segment: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// AssignSegmentsToAgendaItem sets agenda_item_id on segments whose start
// falls within [start_ms, end_ms]. Used by Step 5 (transcript segmentation).
func (s *Store) AssignSegmentsToAgendaItem(ctx context.Context, tvwEventID string, agendaItemID int64, startMS, endMS int) (int64, error) {
	const q = `
UPDATE transcript_segment SET agenda_item_id = $2
 WHERE tvw_event_id = $1 AND start_ms >= $3 AND start_ms <= $4;`
	tag, err := s.Pool.Exec(ctx, q, tvwEventID, agendaItemID, startMS, endMS)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// FindBillNumberMentions returns segment start_ms values where the
// transcript text mentions the given bill (e.g. "HB 1234" or "House Bill 1234").
func (s *Store) FindBillNumberMentions(ctx context.Context, tvwEventID string, prefix string, number int) ([]int, error) {
	pattern := billMentionPattern(prefix, number)
	const q = `
SELECT start_ms FROM transcript_segment
 WHERE tvw_event_id = $1 AND text ~* $2
 ORDER BY start_ms ASC;`
	rows, err := s.Pool.Query(ctx, q, tvwEventID, pattern)
	if err != nil {
		return nil, fmt.Errorf("find mentions: %w", err)
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var ms int
		if err := rows.Scan(&ms); err != nil {
			return nil, err
		}
		out = append(out, ms)
	}
	return out, rows.Err()
}

// billMentionPattern returns a single Postgres regex matching common spoken
// forms. Handles "HB 1234", "HB1234", "House Bill 1234", "House Bill1234".
// The \m / \M boundaries are POSIX word boundaries Postgres understands.
func billMentionPattern(prefix string, number int) string {
	chamberWord := map[string]string{
		"HB": "house bill", "SB": "senate bill",
		"HJR": "house joint resolution", "SJR": "senate joint resolution",
	}
	cw, ok := chamberWord[prefix]
	num := fmt.Sprintf("%d", number)
	if !ok {
		return fmt.Sprintf(`\m%s\s*%s\M`, prefix, num)
	}
	return fmt.Sprintf(`\m(%s|%s)\s*%s\M`, prefix, cw, num)
}

// ---------------------------------------------------------------------------
// organization + org_context_record
// ---------------------------------------------------------------------------

type UpsertOrganizationParams struct {
	CanonicalName            string
	Aliases                  []string
	PDCLobbyistEmployerID    string
	PDCCommitteeOrFilerID    string
	MatchConfidence          string // org_match_confidence enum
	MatchNotes               string
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

type InsertOrgContextParams struct {
	OrganizationID  int64
	ContextType     string // see schema CHECK
	SourceDatasetID string
	SourceRowID     string
	SummaryFields   map[string]any
	SourceURL       string
	MatchConfidence string
	SourceRecordID  int64
}

func (s *Store) ReplaceOrgContextForOrganization(ctx context.Context, orgID int64, rows []InsertOrgContextParams) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM org_context_record WHERE organization_id = $1`, orgID); err != nil {
		return fmt.Errorf("delete org_context: %w", err)
	}
	const insQ = `
INSERT INTO org_context_record (organization_id, context_type, source_dataset_id,
                                source_row_id, summary_fields, source_url,
                                match_confidence, source_record_id)
VALUES ($1,$2,$3,$4,$5,$6,$7::org_match_confidence,$8);`
	for _, r := range rows {
		if _, err := tx.Exec(ctx, insQ,
			r.OrganizationID, r.ContextType, r.SourceDatasetID,
			strOrNull(r.SourceRowID), r.SummaryFields, r.SourceURL,
			defaultStr(r.MatchConfidence, "possible"), r.SourceRecordID,
		); err != nil {
			return fmt.Errorf("insert org_context: %w", err)
		}
	}
	return tx.Commit(ctx)
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
	return tag.RowsAffected(), nil
}

// ---------------------------------------------------------------------------
// bill_status_change
// ---------------------------------------------------------------------------

type InsertStatusChangeParams struct {
	BillID         int64
	ActionDate     time.Time
	HistoryLine    string
	Actor          string
	SourceRecordID int64
}

// ReplaceStatusChangesForBill clears and reloads the status timeline. LWS
// returns the canonical sequence each time we ask, so a snapshot replace
// keeps the table consistent without diffing.
func (s *Store) ReplaceStatusChangesForBill(ctx context.Context, billID int64, rows []InsertStatusChangeParams) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM bill_status_change WHERE bill_id = $1`, billID); err != nil {
		return fmt.Errorf("delete status: %w", err)
	}
	const insQ = `
INSERT INTO bill_status_change (bill_id, action_date, history_line, actor, source_record_id)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT DO NOTHING;`
	for _, r := range rows {
		if _, err := tx.Exec(ctx, insQ,
			r.BillID, dateOrNull(r.ActionDate), r.HistoryLine, strOrNull(r.Actor), r.SourceRecordID,
		); err != nil {
			return fmt.Errorf("insert status: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// ---------------------------------------------------------------------------
// ingestion_run — bookkeeping
// ---------------------------------------------------------------------------

func (s *Store) StartIngestionRun(ctx context.Context, job string, args map[string]any) (int64, error) {
	const q = `INSERT INTO ingestion_run (job, args) VALUES ($1, $2) RETURNING id;`
	var id int64
	err := s.Pool.QueryRow(ctx, q, job, args).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("start ingestion_run: %w", err)
	}
	return id, nil
}

func (s *Store) FinishIngestionRun(ctx context.Context, id int64, status string, fetched, upserted int, runErr error) error {
	const q = `UPDATE ingestion_run SET finished_at = NOW(), status = $2,
                                rows_fetched = $3, rows_upserted = $4, error = $5
              WHERE id = $1;`
	var errStr *string
	if runErr != nil {
		s := runErr.Error()
		errStr = &s
	}
	_, err := s.Pool.Exec(ctx, q, id, status, fetched, upserted, errStr)
	return err
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func defaultStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func nullStringArray(a []string) any {
	if a == nil {
		return []string{}
	}
	return a
}

func lowerTrim(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		out = append(out, r)
	}
	// Trim leading/trailing spaces.
	start, end := 0, len(out)
	for start < end && (out[start] == ' ' || out[start] == '\t') {
		start++
	}
	for end > start && (out[end-1] == ' ' || out[end-1] == '\t') {
		end--
	}
	return string(out[start:end])
}
