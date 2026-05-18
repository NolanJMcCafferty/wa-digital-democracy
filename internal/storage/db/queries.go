package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	SourceRecordID            int64
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
	HearingID             int64
	BillID                *int64
	Label                 string
	CSIMeetingFamilyID    string
	CSIAgendaItemFamilyID string
	CSIAgendaItemID       string
	OrderIndex            int
	SourceRecordID        int64
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
	return s.AssignSegmentsToAgendaItemWindows(ctx, tvwEventID, agendaItemID, [][2]int{{startMS, endMS}})
}

// AssignSegmentsToAgendaItemWindows sets agenda_item_id on segments whose start
// falls inside any supplied [start_ms, end_ms] window. It clears prior tags for
// the agenda item in the same TVW event first so re-segmentation is idempotent.
func (s *Store) AssignSegmentsToAgendaItemWindows(ctx context.Context, tvwEventID string, agendaItemID int64, windows [][2]int) (int64, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `UPDATE transcript_segment SET agenda_item_id = NULL WHERE tvw_event_id = $1 AND agenda_item_id = $2`, tvwEventID, agendaItemID); err != nil {
		return 0, err
	}

	var total int64
	const q = `
UPDATE transcript_segment SET agenda_item_id = $2
 WHERE tvw_event_id = $1 AND start_ms >= $3 AND start_ms <= $4;`
	for _, w := range windows {
		if w[1] <= w[0] {
			continue
		}
		tag, err := tx.Exec(ctx, q, tvwEventID, agendaItemID, w[0], w[1])
		if err != nil {
			return 0, err
		}
		total += tag.RowsAffected()
	}
	return total, tx.Commit(ctx)
}

// TranscriptCue is a minimal transcript segment used by the segmentation step.
type TranscriptCue struct {
	StartMS int
	EndMS   int
	Text    string
}

// ListTranscriptCues returns ordered caption cues for a TVW event.
func (s *Store) ListTranscriptCues(ctx context.Context, tvwEventID string) ([]TranscriptCue, error) {
	const q = `
SELECT start_ms, end_ms, text FROM transcript_segment
 WHERE tvw_event_id = $1
 ORDER BY start_ms ASC;`
	rows, err := s.Pool.Query(ctx, q, tvwEventID)
	if err != nil {
		return nil, fmt.Errorf("list transcript cues: %w", err)
	}
	defer rows.Close()
	var out []TranscriptCue
	for rows.Next() {
		var c TranscriptCue
		if err := rows.Scan(&c.StartMS, &c.EndMS, &c.Text); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
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

// ListedBill is the row shape returned by ListIngestedBills — the fields
// the bills-index API endpoint surfaces. BillID is the generated column
// (e.g. "HB 1501") so callers don't have to recompose it.
type ListedBill struct {
	Biennium string
	Prefix   string
	Number   int
	BillID   string
	Title    string
}

// ListIngestedBills returns every bill row in stable display order
// (newest biennium first, then prefix, then number). Filters out the
// occasional placeholder row with number=0 from broken upserts.
func (s *Store) ListIngestedBills(ctx context.Context) ([]ListedBill, error) {
	const q = `
SELECT biennium, prefix, number, bill_number, COALESCE(title, '')
  FROM bill
 WHERE number > 0
 ORDER BY biennium DESC, prefix, number;`
	rows, err := s.Pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list bills: %w", err)
	}
	defer rows.Close()
	out := []ListedBill{}
	for rows.Next() {
		var b ListedBill
		if err := rows.Scan(&b.Biennium, &b.Prefix, &b.Number, &b.BillID, &b.Title); err != nil {
			return nil, fmt.Errorf("scan listed bill: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
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

// ---------------------------------------------------------------------------
// Aggregation queries — back the /api/v1/{legislators,organizations,hearings,sources}
// endpoints. All read-only; safe to run any time.
// ---------------------------------------------------------------------------

// LegislatorAggregate is the row shape ListLegislators returns.
type LegislatorAggregate struct {
	ID        int64
	Name      string
	Chamber   string
	BillCount int
}

// ListLegislators returns every legislator in the DB with their sponsored
// bill count. Order: alphabetical by name.
func (s *Store) ListLegislators(ctx context.Context) ([]LegislatorAggregate, error) {
	const q = `
SELECT l.id, l.name, COALESCE(l.chamber, ''), COUNT(DISTINCT bs.bill_id)
  FROM legislator l
  LEFT JOIN bill_sponsor bs ON bs.legislator_id = l.id
 GROUP BY l.id
 ORDER BY l.name;`
	rows, err := s.Pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list legislators: %w", err)
	}
	defer rows.Close()
	out := []LegislatorAggregate{}
	for rows.Next() {
		var l LegislatorAggregate
		if err := rows.Scan(&l.ID, &l.Name, &l.Chamber, &l.BillCount); err != nil {
			return nil, fmt.Errorf("scan legislator: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// LegislatorAppearance is one sponsored-bill row joined onto bill metadata.
type LegislatorAppearance struct {
	Biennium    string
	BillID      string
	BillPrefix  string
	BillNumber  int
	BillTitle   string
	SponsorType string
}

// GetLegislatorBills returns every bill the legislator has sponsored,
// joined to bill metadata for display.
func (s *Store) GetLegislatorBills(ctx context.Context, legislatorID int64) ([]LegislatorAppearance, error) {
	const q = `
SELECT b.biennium, b.bill_number, b.prefix, b.number,
       COALESCE(b.title, ''), bs.sponsor_type
  FROM bill_sponsor bs
  JOIN bill b ON b.id = bs.bill_id
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
			&a.BillTitle, &a.SponsorType); err != nil {
			return nil, fmt.Errorf("scan appearance: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

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
	ContextCount    int
}

// ListOrganizations returns every organization with aggregated testifier
// position counts and PDC context-record count.
func (s *Store) ListOrganizations(ctx context.Context) ([]OrganizationAggregate, error) {
	const q = `
SELECT o.id, o.canonical_name, o.aliases,
       o.match_confidence::text, COALESCE(o.match_notes, ''),
       COUNT(DISTINCT t.id) AS testifier_count,
       COUNT(DISTINCT t.id) FILTER (WHERE t.position = 'Pro')   AS pro_count,
       COUNT(DISTINCT t.id) FILTER (WHERE t.position = 'Con')   AS con_count,
       COUNT(DISTINCT t.id) FILTER (WHERE t.position = 'Other') AS other_count,
       COUNT(DISTINCT t.id) FILTER (WHERE t.position = 'Unknown') AS unknown_count,
       COUNT(DISTINCT ocr.id) AS context_count
  FROM organization o
  LEFT JOIN testifier          t   ON t.normalized_org_id = o.id
  LEFT JOIN org_context_record ocr ON ocr.organization_id = o.id
 GROUP BY o.id
 ORDER BY o.canonical_name;`
	rows, err := s.Pool.Query(ctx, q)
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
			&o.OtherCount, &o.UnknownCount, &o.ContextCount); err != nil {
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
	HearingTitle    string
	CommitteeName   string
	MeetingDateTime time.Time
	Position        string
	TestifierCount  int
}

// GetOrganizationAppearances returns every (agenda_item, org) appearance
// where at least one testifier from that org signed in.
func (s *Store) GetOrganizationAppearances(ctx context.Context, organizationID int64) ([]OrganizationAppearance, error) {
	const q = `
SELECT b.biennium, b.bill_number, b.prefix, b.number,
       COALESCE(a.csi_agenda_item_id, ''),
       COALESCE(a.label, ''),
       h.committee_name,
       h.meeting_datetime,
       MAX(t.position::text) AS position,
       COUNT(t.id) AS testifier_count
  FROM testifier t
  JOIN agenda_item a ON a.id = t.agenda_item_id
  JOIN hearing     h ON h.id = a.hearing_id
  LEFT JOIN bill   b ON b.id = a.bill_id
 WHERE t.normalized_org_id = $1
 GROUP BY b.biennium, b.bill_number, b.prefix, b.number,
          a.csi_agenda_item_id, a.label, h.committee_name, h.meeting_datetime
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
			&a.CSIAgendaItemID, &a.HearingTitle,
			&a.CommitteeName, &a.MeetingDateTime,
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

// HearingAggregate is the row shape ListHearings returns.
type HearingAggregate struct {
	CSIAgendaItemID string
	AgendaItemLabel string
	CommitteeName   string
	Chamber         string
	MeetingDateTime time.Time
	Biennium        string
	BillID          string
	BillPrefix      string
	BillNumber      int
	HasTVW          bool
}

// ListHearings returns every agenda_item joined to its hearing + bill,
// ordered by meeting datetime descending. Only includes rows where the
// hearing has a TVW event mapping (the curated subset).
func (s *Store) ListHearings(ctx context.Context) ([]HearingAggregate, error) {
	const q = `
SELECT a.csi_agenda_item_id, a.label,
       h.committee_name, h.chamber, h.meeting_datetime,
       b.biennium, b.bill_number, b.prefix, b.number,
       (h.tvw_event_id IS NOT NULL) AS has_tvw
  FROM agenda_item a
  JOIN hearing h ON h.id = a.hearing_id
  JOIN bill    b ON b.id = a.bill_id
 WHERE h.tvw_event_id IS NOT NULL
 ORDER BY h.meeting_datetime DESC;`
	rows, err := s.Pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list hearings: %w", err)
	}
	defer rows.Close()
	out := []HearingAggregate{}
	for rows.Next() {
		var hh HearingAggregate
		if err := rows.Scan(&hh.CSIAgendaItemID, &hh.AgendaItemLabel,
			&hh.CommitteeName, &hh.Chamber, &hh.MeetingDateTime,
			&hh.Biennium, &hh.BillID, &hh.BillPrefix, &hh.BillNumber,
			&hh.HasTVW); err != nil {
			return nil, fmt.Errorf("scan hearing: %w", err)
		}
		out = append(out, hh)
	}
	return out, rows.Err()
}

// SourceSummaryRow is the row shape ListSourceSummaries returns.
type SourceSummaryRow struct {
	System          string
	Calls           int
	LatestFetchedAt time.Time
	Endpoints       []string
}

// ListSourceSummaries groups source_record rows by source_system and
// returns counts + most-recent fetch + distinct endpoints.
func (s *Store) ListSourceSummaries(ctx context.Context) ([]SourceSummaryRow, error) {
	const q = `
SELECT source_system, COUNT(*), MAX(fetched_at), ARRAY_AGG(DISTINCT source_endpoint)
  FROM source_record
 GROUP BY source_system
 ORDER BY source_system;`
	rows, err := s.Pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list source summaries: %w", err)
	}
	defer rows.Close()
	out := []SourceSummaryRow{}
	for rows.Next() {
		var r SourceSummaryRow
		if err := rows.Scan(&r.System, &r.Calls, &r.LatestFetchedAt, &r.Endpoints); err != nil {
			return nil, fmt.Errorf("scan source summary: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Auto-discovery queries — back the `wa-dd discover-hearings` and
// `wa-dd ingest-hearings` commands.
// ---------------------------------------------------------------------------

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
	SourceRecordID   int64 // reused for the discovery-driven UpsertHearing call
}

// ListHearingsForDiscovery returns hearings whose CSI/TVW IDs are still
// blank for bills in the given biennium. These are the candidates for
// auto-discovery. Hearings already enriched (have either a TVW event ID
// or a Committee Schedules agenda ID) are skipped.
func (s *Store) ListHearingsForDiscovery(ctx context.Context, biennium string) ([]HearingForDiscovery, error) {
	const q = `
SELECT h.id, b.id, b.prefix, b.number,
       h.committee_name, COALESCE(h.committee_acronym, ''), h.chamber,
       h.meeting_datetime, h.source_record_id
  FROM hearing h
  JOIN bill b ON b.id = h.bill_id
 WHERE b.biennium = $1
   AND h.tvw_event_id IS NULL
   AND h.committee_schedule_agenda_id IS NULL
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
			&h.MeetingDateTime, &h.SourceRecordID,
		); err != nil {
			return nil, fmt.Errorf("scan hearing: %w", err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// DiscoveredAgendaItemRow is one (agenda_item, bill) pair that
// auto-discovery has populated and is ready for full pipeline ingestion
// (CSI testifiers + TVW captions + segments + speakers + PDC).
type DiscoveredAgendaItemRow struct {
	CSIAgendaItemID string
	Biennium        string
	BillPrefix      string
	BillNumber      int
}

// ListDiscoveredAgendaItems returns agenda_item rows where (a) the
// hearing has a tvw_event_id (so transcript ingest can run) and (b) no
// testifier rows have been ingested yet for that agenda_item. Used by
// `wa-dd ingest-hearings` to feed buildOne.
func (s *Store) ListDiscoveredAgendaItems(ctx context.Context, biennium string) ([]DiscoveredAgendaItemRow, error) {
	const q = `
SELECT a.csi_agenda_item_id, b.biennium, b.prefix, b.number
  FROM agenda_item a
  JOIN hearing h ON h.id = a.hearing_id
  JOIN bill    b ON b.id = a.bill_id
 WHERE b.biennium = $1
   AND h.tvw_event_id IS NOT NULL
   AND NOT EXISTS (
     SELECT 1 FROM testifier t WHERE t.agenda_item_id = a.id
   )
 ORDER BY h.meeting_datetime DESC;`
	rows, err := s.Pool.Query(ctx, q, biennium)
	if err != nil {
		return nil, fmt.Errorf("list discovered agenda items: %w", err)
	}
	defer rows.Close()
	out := []DiscoveredAgendaItemRow{}
	for rows.Next() {
		var r DiscoveredAgendaItemRow
		if err := rows.Scan(&r.CSIAgendaItemID, &r.Biennium, &r.BillPrefix, &r.BillNumber); err != nil {
			return nil, fmt.Errorf("scan agenda item: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Transcript full-text search — backs `/api/v1/search/transcripts`.
// ---------------------------------------------------------------------------

// TranscriptSearchHit is one row returned by SearchTranscripts. The bill
// fields can all be empty when the agenda_item / bill joins miss
// (transcript_segment.agenda_item_id is nullable until SegmentTranscript
// has run on the row).
type TranscriptSearchHit struct {
	ID              int64
	BillID          string // "HB 1501" — empty when bill join misses
	Biennium        string
	BillPrefix      string
	BillNumber      int
	AgendaItemLabel string
	CommitteeName   string
	MeetingDateTime time.Time // zero when hearing join misses
	StartMS         int
	EndMS           int
	Text            string
	SpeakerLabel    string
	TVWEventID      string
}

// SearchTranscripts runs a websearch_to_tsquery full-text search over
// the transcript_segment table and joins the result back to bill +
// hearing + agenda_item for context.
//
// Returns ([]hits, total, err). total comes from a COUNT(*) OVER ()
// window function so the caller can render "showing N of TOTAL" without
// a second query.
//
// Empty / whitespace-only query returns ([]hits=nil, total=0, err=nil)
// without touching Postgres.
//
// limit and offset are caller-provided; the API handler is responsible
// for clamping them.
func (s *Store) SearchTranscripts(
	ctx context.Context, query string, limit, offset int,
) ([]TranscriptSearchHit, int, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, 0, nil
	}
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	const sql = `
WITH q AS (SELECT websearch_to_tsquery('english', $1) AS tsq)
SELECT
  ts.id,
  COALESCE(b.bill_number, '')              AS bill_id,
  COALESCE(b.biennium, '')                 AS biennium,
  COALESCE(b.prefix, '')                   AS bill_prefix,
  COALESCE(b.number, 0)                    AS bill_number,
  COALESCE(a.label, '')                    AS agenda_item_label,
  COALESCE(h.committee_name, '')           AS committee_name,
  h.meeting_datetime,
  ts.start_ms,
  ts.end_ms,
  ts.text,
  COALESCE(ts.speaker_label, '')           AS speaker_label,
  COALESCE(h.tvw_event_id, ts.tvw_event_id, '') AS tvw_event_id,
  COUNT(*) OVER ()                         AS total_count
FROM transcript_segment ts
LEFT JOIN agenda_item a ON a.id = ts.agenda_item_id
LEFT JOIN hearing     h ON h.id = a.hearing_id
LEFT JOIN bill        b ON b.id = a.bill_id
WHERE to_tsvector('english', ts.text) @@ (SELECT tsq FROM q)
ORDER BY ts_rank_cd(to_tsvector('english', ts.text), (SELECT tsq FROM q)) DESC,
         ts.start_ms ASC
LIMIT $2 OFFSET $3;`

	rows, err := s.Pool.Query(ctx, sql, q, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("search transcripts: %w", err)
	}
	defer rows.Close()

	out := []TranscriptSearchHit{}
	total := 0
	for rows.Next() {
		var h TranscriptSearchHit
		var meetingTS pgtype.Timestamptz
		if err := rows.Scan(
			&h.ID, &h.BillID, &h.Biennium, &h.BillPrefix, &h.BillNumber,
			&h.AgendaItemLabel, &h.CommitteeName, &meetingTS,
			&h.StartMS, &h.EndMS, &h.Text, &h.SpeakerLabel,
			&h.TVWEventID, &total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan transcript hit: %w", err)
		}
		if meetingTS.Valid {
			h.MeetingDateTime = meetingTS.Time
		}
		out = append(out, h)
	}
	return out, total, rows.Err()
}
