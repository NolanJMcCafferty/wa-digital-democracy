package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/entitymatch"
)

// nonEmptyStrings returns trimmed, non-empty entries from in. Used by
// query builders that accept multi-value filters.
func nonEmptyStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if t := strings.TrimSpace(v); t != "" {
			out = append(out, t)
		}
	}
	return out
}

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
	TVWEventID          string
	WPPostID            *int64
	WPSlug              string
	WPLink              string
	Title               string
	Description         string
	StartDateTime       time.Time
	CaptionURL          string
	ThumbnailURL        string
	CustomID            string
	LocationName        string
	TotalRuntime        string
	TotalRuntimeSeconds int
	PublishedAudioURL   string
	AudioDownloadURL    string
	VideoDownloadURL    string
	StreamingURIs       any
	RawCategories       []string
	RawKeywords         []string
	RawWPTags           []int
	RawWPCategories     []int
	SourceRecordID      int64
}

func (s *Store) UpsertTVWEvent(ctx context.Context, p UpsertTVWEventParams) (int64, error) {
	streaming, err := marshalJSONDefault(p.StreamingURIs, map[string]any{})
	if err != nil {
		return 0, fmt.Errorf("marshal streaming uris: %w", err)
	}
	wpTags, err := json.Marshal(p.RawWPTags)
	if err != nil {
		return 0, fmt.Errorf("marshal wp tags: %w", err)
	}
	wpCategories, err := json.Marshal(p.RawWPCategories)
	if err != nil {
		return 0, fmt.Errorf("marshal wp categories: %w", err)
	}
	const q = `
INSERT INTO tvw_event (tvw_event_id, wp_post_id, wp_slug, wp_link, title,
                       description, start_datetime, caption_url, thumbnail_url,
                       custom_id, location_name, total_runtime,
                       total_runtime_seconds, published_audio_url,
                       audio_download_url, video_download_url, streaming_uris,
                       raw_categories, raw_keywords, raw_wp_tags,
                       raw_wp_categories, source_record_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,NULLIF($13,0),$14,$15,$16,$17::jsonb,$18,$19,$20::jsonb,$21::jsonb,$22)
ON CONFLICT (tvw_event_id) DO UPDATE SET
  wp_post_id     = COALESCE(EXCLUDED.wp_post_id, tvw_event.wp_post_id),
  wp_slug        = COALESCE(EXCLUDED.wp_slug, tvw_event.wp_slug),
  wp_link        = COALESCE(EXCLUDED.wp_link, tvw_event.wp_link),
  title          = EXCLUDED.title,
  description    = EXCLUDED.description,
  start_datetime = EXCLUDED.start_datetime,
  caption_url    = EXCLUDED.caption_url,
  thumbnail_url  = EXCLUDED.thumbnail_url,
  custom_id      = COALESCE(EXCLUDED.custom_id, tvw_event.custom_id),
  location_name  = COALESCE(EXCLUDED.location_name, tvw_event.location_name),
  total_runtime  = COALESCE(EXCLUDED.total_runtime, tvw_event.total_runtime),
  total_runtime_seconds = COALESCE(EXCLUDED.total_runtime_seconds, tvw_event.total_runtime_seconds),
  published_audio_url = COALESCE(EXCLUDED.published_audio_url, tvw_event.published_audio_url),
  audio_download_url = COALESCE(EXCLUDED.audio_download_url, tvw_event.audio_download_url),
  video_download_url = COALESCE(EXCLUDED.video_download_url, tvw_event.video_download_url),
  streaming_uris = COALESCE(EXCLUDED.streaming_uris, tvw_event.streaming_uris),
  raw_categories = EXCLUDED.raw_categories,
  raw_keywords = EXCLUDED.raw_keywords,
  raw_wp_tags = EXCLUDED.raw_wp_tags,
  raw_wp_categories = EXCLUDED.raw_wp_categories,
  source_record_id = EXCLUDED.source_record_id,
  updated_at     = NOW()
RETURNING id;`
	var id int64
	err = s.Pool.QueryRow(ctx, q,
		p.TVWEventID, p.WPPostID, strOrNull(p.WPSlug), strOrNull(p.WPLink), strOrNull(p.Title),
		strOrNull(p.Description), timeOrNull(p.StartDateTime), strOrNull(p.CaptionURL), strOrNull(p.ThumbnailURL),
		strOrNull(p.CustomID), strOrNull(p.LocationName), strOrNull(p.TotalRuntime), p.TotalRuntimeSeconds,
		strOrNull(p.PublishedAudioURL), strOrNull(p.AudioDownloadURL), strOrNull(p.VideoDownloadURL), string(streaming),
		nonNilStrings(p.RawCategories), nonNilStrings(p.RawKeywords), string(wpTags), string(wpCategories), p.SourceRecordID,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert tvw_event: %w", err)
	}
	return id, nil
}

func nonNilStrings(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}

func fileSizeOrNull(n int64) any {
	if n == 0 {
		return nil
	}
	return n
}

func marshalJSONDefault(v any, def any) ([]byte, error) {
	if v == nil {
		v = def
	}
	return json.Marshal(v)
}

type UpsertTVWMediaAssetParams struct {
	TVWEventID          string
	AssetID             string
	AssetType           string
	Name                string
	FileURL             string
	ThumbnailURL        string
	SpriteURL           string
	PreviewURL          string
	FileSizeBytes       int64
	TotalRuntime        string
	TotalRuntimeSeconds int
	CurrentStatus       string
	DateCreated         time.Time
	AdvancedDetails     any
	SourceRecordID      int64
}

func (s *Store) ReplaceTVWMediaAssets(ctx context.Context, tvwEventID string, rows []UpsertTVWMediaAssetParams) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM tvw_media_asset WHERE tvw_event_id = $1`, tvwEventID); err != nil {
		return fmt.Errorf("delete tvw media assets: %w", err)
	}
	const q = `
INSERT INTO tvw_media_asset (tvw_event_id, asset_id, asset_type, name, file_url,
                             thumbnail_url, sprite_url, preview_url,
                             file_size_bytes, total_runtime,
                             total_runtime_seconds, current_status,
                             date_created, advanced_details, source_record_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9::bigint,0),$10,NULLIF($11,0),$12,$13,$14::jsonb,$15);`
	for _, r := range rows {
		advanced, err := marshalJSONDefault(r.AdvancedDetails, map[string]any{})
		if err != nil {
			return fmt.Errorf("marshal advanced details: %w", err)
		}
		if _, err := tx.Exec(ctx, q,
			r.TVWEventID, r.AssetID, r.AssetType, strOrNull(r.Name), strOrNull(r.FileURL),
			strOrNull(r.ThumbnailURL), strOrNull(r.SpriteURL), strOrNull(r.PreviewURL),
			fileSizeOrNull(r.FileSizeBytes), strOrNull(r.TotalRuntime), r.TotalRuntimeSeconds,
			strOrNull(r.CurrentStatus), timeOrNull(r.DateCreated), string(advanced), r.SourceRecordID,
		); err != nil {
			return fmt.Errorf("insert tvw media asset %s: %w", r.AssetID, err)
		}
	}
	return tx.Commit(ctx)
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

// AgendaItemWindow is one detected bill-discussion span on a TVW event.
// SegmentTranscript writes these atomically with the corresponding
// transcript_segment.agenda_item_id assignments; the bundle assembler
// reads them back literally so window boundaries are stable across
// re-renders and aren't re-derived in SQL with a different threshold.
type AgendaItemWindow struct {
	StartMS  int
	EndMS    int
	Mentions int
}

// AssignSegmentsToAgendaItemWindows is the canonical write path for
// transcript segmentation. In one transaction it (a) clears any prior
// agenda_item_id pointing at this agenda item, (b) re-tags transcript
// segments whose start falls inside any supplied window, and (c)
// rewrites the agenda_item_window rows for that agenda item.
//
// Re-segmentation is idempotent: running this with the same input set
// twice converges to the same DB state. Running it after a heuristic
// change correctly evicts stale segments and stale windows.
func (s *Store) AssignSegmentsToAgendaItemWindows(
	ctx context.Context,
	tvwEventID string,
	agendaItemID int64,
	windows []AgendaItemWindow,
) (int64, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	// Clear prior tags so segments that fell out of the new window set
	// stop pointing at this agenda item.
	if _, err := tx.Exec(ctx, `UPDATE transcript_segment SET agenda_item_id = NULL WHERE tvw_event_id = $1 AND agenda_item_id = $2`, tvwEventID, agendaItemID); err != nil {
		return 0, err
	}
	// Clear prior persisted windows for the same reason.
	if _, err := tx.Exec(ctx, `DELETE FROM agenda_item_window WHERE agenda_item_id = $1`, agendaItemID); err != nil {
		return 0, err
	}

	const tagQ = `
UPDATE transcript_segment SET agenda_item_id = $2
 WHERE tvw_event_id = $1 AND start_ms >= $3 AND start_ms <= $4;`
	const insWinQ = `
INSERT INTO agenda_item_window (agenda_item_id, start_ms, end_ms, mentions)
VALUES ($1, $2, $3, $4);`

	var total int64
	for _, w := range windows {
		if w.EndMS <= w.StartMS {
			continue
		}
		tag, err := tx.Exec(ctx, tagQ, tvwEventID, agendaItemID, w.StartMS, w.EndMS)
		if err != nil {
			return 0, err
		}
		total += tag.RowsAffected()
		if _, err := tx.Exec(ctx, insWinQ, agendaItemID, w.StartMS, w.EndMS, w.Mentions); err != nil {
			return 0, err
		}
	}
	return total, tx.Commit(ctx)
}

// ListAgendaItemWindowsByAgendaItem returns the persisted windows for a
// CSI agenda-item ID, ordered by start_ms. Used by the bundle assembler.
func (s *Store) ListAgendaItemWindowsByAgendaItem(ctx context.Context, csiAgendaItemID string) ([]AgendaItemWindow, error) {
	const q = `
SELECT w.start_ms, w.end_ms, w.mentions
  FROM agenda_item_window w
  JOIN agenda_item a ON a.id = w.agenda_item_id
 WHERE a.csi_agenda_item_id = $1
 ORDER BY w.start_ms ASC;`
	rows, err := s.Pool.Query(ctx, q, csiAgendaItemID)
	if err != nil {
		return nil, fmt.Errorf("list agenda_item_window: %w", err)
	}
	defer rows.Close()
	var out []AgendaItemWindow
	for rows.Next() {
		var w AgendaItemWindow
		if err := rows.Scan(&w.StartMS, &w.EndMS, &w.Mentions); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
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

// ListedBill is the row shape the bills-index API endpoint surfaces.
// Lead-sponsor and status fields can be empty when the bill_sponsor /
// bill_status_change joins miss (rare for ingested bills, but the
// `Primary` sponsor isn't always populated).
type ListedBill struct {
	Biennium      string
	Prefix        string
	Number        int
	BillID        string // generated column, e.g. "HB 1501"
	Title         string
	ChamberOrigin string // "House" | "Senate"
	CurrentStatus string
	StatusDate    time.Time
	LeadSponsor   string // legislator.name e.g. "Senator Reed" — empty when no Primary sponsor
	LeadFirstName string
	LeadLastName  string
	LeadParty     string // "D" | "R"
	LeadSlug      string // computed in API handler; left empty here
}

// BillSearchParams are the filter knobs SearchBills accepts. Empty
// strings are no-ops. The handler is responsible for clamping limit
// and offset to safe ranges.
type BillSearchParams struct {
	Query       string // matches title or bill_number (ILIKE)
	Prefix      string // exact match on bill.prefix (HB, SB, HJR, …)
	Chamber     string // "House" | "Senate"
	Party       string // "D" | "R" — filters on lead sponsor's party
	Status      string // "in_progress" | "passed" | "failed" | "" — bucketed from current_status
	Sponsor     string // legislator slug; matches any sponsor row
	LeadSponsor string // legislator slug; matches the Primary sponsor only
	Limit       int
	Offset      int
}

// BillSearchFacets carries the distinct values we render in the
// sidebar so the UI doesn't hard-code lists. Counts here are over the
// full unfiltered set; the API handler attaches them once per query.
type BillSearchFacets struct {
	Prefixes []string // ordered: HB, SB, HJR, SJR, HCR, SCR, HJM, SJM, then alphabetical
	Chambers []string // House, Senate
	Parties  []string // D, R, …
	Statuses []string // in_progress, passed, failed
}

// ListIngestedBills returns every bill row in stable display order
// (newest biennium first, then prefix, then number). Filters out the
// occasional placeholder row with number=0 from broken upserts.
//
// Returns the same shape as SearchBills with no filters but no total
// count — kept for back-compat with callers that don't paginate.
func (s *Store) ListIngestedBills(ctx context.Context) ([]ListedBill, error) {
	hits, _, err := s.SearchBills(ctx, BillSearchParams{Limit: 100000, Offset: 0})
	return hits, err
}

// SearchBills is the paginated, filtered query backing /api/v1/bills.
// Returns (hits, total, err). total is the count of matches across all
// pages (not just the page returned), so the frontend can render
// numbered pagination without a second query.
func (s *Store) SearchBills(ctx context.Context, p BillSearchParams) ([]ListedBill, int, error) {
	if p.Limit <= 0 {
		p.Limit = 50
	}
	if p.Offset < 0 {
		p.Offset = 0
	}

	// Build the WHERE fragment dynamically. Push a value once, take its
	// $N index, use it any number of times in the matching fragment.
	args := []any{}
	where := []string{"b.number > 0"}
	push := func(v any) int {
		args = append(args, v)
		return len(args)
	}
	if q := strings.TrimSpace(p.Query); q != "" {
		idx := push(q)
		where = append(where, fmt.Sprintf(
			"(b.title ILIKE '%%' || $%d || '%%' OR b.bill_number ILIKE '%%' || $%d || '%%')", idx, idx))
	}
	if p.Prefix != "" {
		idx := push(p.Prefix)
		where = append(where, fmt.Sprintf("b.prefix = $%d", idx))
	}
	if p.Chamber != "" {
		idx := push(p.Chamber)
		where = append(where, fmt.Sprintf("b.chamber_origin = $%d", idx))
	}
	if p.Party != "" {
		idx := push(p.Party)
		where = append(where, fmt.Sprintf("primary_sponsor.party = $%d", idx))
	}
	if p.Sponsor != "" {
		idx := push(p.Sponsor)
		where = append(where, fmt.Sprintf(`
EXISTS (
  SELECT 1
    FROM bill_sponsor sponsor_filter_bs
    JOIN legislator sponsor_filter_l ON sponsor_filter_l.id = sponsor_filter_bs.legislator_id
   WHERE sponsor_filter_bs.bill_id = b.id
     AND trim(both '-' from regexp_replace(replace(lower(sponsor_filter_l.name), '&', ' and '), '[^a-z0-9]+', '-', 'g')) = $%d
)`, idx))
	}
	if p.LeadSponsor != "" {
		idx := push(p.LeadSponsor)
		where = append(where, fmt.Sprintf("trim(both '-' from regexp_replace(replace(lower(primary_sponsor.name), '&', ' and '), '[^a-z0-9]+', '-', 'g')) = $%d", idx))
	}
	if p.Status != "" {
		// Status accepts either a coarse bucket ("passed", "failed",
		// "in_progress") or one of the fine-grained stage labels mirrored
		// in apps/web/src/lib/billStatus.ts. Stages map to the same SQL
		// CASE expression used elsewhere; keep the two in sync.
		switch strings.ToLower(p.Status) {
		case "passed":
			where = append(where, "(b.current_status ILIKE '%effective date%' OR b.current_status ILIKE '%governor signed%' OR b.current_status ILIKE '%chapter %2026 laws%')")
		case "failed":
			where = append(where, "(b.current_status ILIKE '%died%' OR b.current_status ILIKE '%vetoed%' OR b.current_status ILIKE '%not passed%')")
		case "in_progress":
			where = append(where,
				"NOT (b.current_status ILIKE '%effective date%' OR b.current_status ILIKE '%governor signed%' OR b.current_status ILIKE '%chapter %2026 laws%' OR b.current_status ILIKE '%died%' OR b.current_status ILIKE '%vetoed%' OR b.current_status ILIKE '%not passed%')")
		default:
			idx := push(p.Status)
			where = append(where, fmt.Sprintf("(%s) = $%d", billStageCaseSQL("b.current_status"), idx))
		}
	}

	whereSQL := strings.Join(where, " AND ")

	// One join to the legislator that's the Primary sponsor (DISTINCT
	// ON keeps it to one row per bill if there are duplicate Primary
	// rows from re-ingestion).
	const baseFROM = `
FROM bill b
LEFT JOIN LATERAL (
  SELECT l.name, COALESCE(l.first_name, '') AS first_name,
         COALESCE(l.last_name, '') AS last_name,
         COALESCE(l.party, '') AS party,
         COALESCE(l.lws_sponsor_id, '') AS lws_sponsor_id
    FROM bill_sponsor bs
    JOIN legislator l ON l.id = bs.legislator_id
   WHERE bs.bill_id = b.id AND bs.sponsor_type = 'Primary'
   ORDER BY l.id
   LIMIT 1
) primary_sponsor ON TRUE`

	// Count total matches once (not per page).
	countQ := "SELECT COUNT(*) " + baseFROM + " WHERE " + whereSQL
	var total int
	if err := s.Pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count bills: %w", err)
	}

	// Selection page.
	args = append(args, p.Limit, p.Offset)
	q := `
SELECT b.biennium, b.prefix, b.number, b.bill_number,
       COALESCE(b.title, ''),
       COALESCE(b.chamber_origin, ''),
       COALESCE(b.current_status, ''),
       b.status_date,
       COALESCE(primary_sponsor.name, ''),
       COALESCE(primary_sponsor.first_name, ''),
       COALESCE(primary_sponsor.last_name, ''),
       COALESCE(primary_sponsor.party, '')
` + baseFROM + ` WHERE ` + whereSQL + `
 ORDER BY b.biennium DESC, b.prefix, b.number
 LIMIT $` + fmt.Sprintf("%d", len(args)-1) + ` OFFSET $` + fmt.Sprintf("%d", len(args))
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("search bills: %w", err)
	}
	defer rows.Close()
	out := []ListedBill{}
	for rows.Next() {
		var b ListedBill
		var statusDate pgtype.Date
		if err := rows.Scan(&b.Biennium, &b.Prefix, &b.Number, &b.BillID, &b.Title,
			&b.ChamberOrigin, &b.CurrentStatus, &statusDate,
			&b.LeadSponsor, &b.LeadFirstName, &b.LeadLastName, &b.LeadParty); err != nil {
			return nil, 0, fmt.Errorf("scan bill: %w", err)
		}
		if statusDate.Valid {
			b.StatusDate = statusDate.Time
		}
		out = append(out, b)
	}
	return out, total, rows.Err()
}

// billStageCaseSQL returns a SQL expression that classifies a
// current_status string into one of the lifecycle-stage labels mirrored
// in apps/web/src/lib/billStatus.ts. Rules are evaluated end-of-life
// first, matching the JS classifyBillStage rule order.
func billStageCaseSQL(col string) string {
	return `CASE
  WHEN ` + col + ` ILIKE '%effective date%' OR ` + col + ` ~* 'chapter [0-9]+,' OR ` + col + ` ILIKE '%filed with secretary of state%' THEN 'Session law'
  WHEN ` + col + ` ILIKE '%governor%vetoed%' THEN 'Vetoed'
  WHEN ` + col + ` ILIKE '%governor signed%' THEN 'Signed by Governor'
  WHEN ` + col + ` ILIKE '%delivered to governor%' THEN 'On Governor''s desk'
  WHEN ` + col + ` ILIKE '%speaker signed%' OR ` + col + ` ILIKE '%president signed%' THEN 'Passed Legislature'
  WHEN ` + col + ` ILIKE '%"x" file%' THEN 'Shelved'
  WHEN ` + col + ` ILIKE '%by resolution, reintroduced%' THEN 'Reintroduced'
  WHEN ` + col + ` ~* '\bdied\b' THEN 'Died'
  WHEN ` + col + ` ILIKE '%third reading, passed%' THEN 'Passed chamber'
  WHEN ` + col + ` ILIKE '%placed on second reading%' OR ` + col + ` ILIKE '%placed on third reading%' OR ` + col + ` ILIKE '%second reading%' THEN 'On floor calendar'
  WHEN ` + col + ` ILIKE '%referred to%' OR ` + col + ` ILIKE '%public hearing%' OR ` + col + ` ILIKE '%executive action%' OR ` + col + ` ILIKE '%executive session%' OR ` + col + ` ILIKE '%majority report%' OR ` + col + ` ILIKE '%minority report%' OR ` + col + ` ILIKE '%passed to rules%' THEN 'In committee'
  WHEN ` + col + ` ILIKE '%first reading%' OR ` + col + ` ILIKE '%prefiled%' OR ` + col + ` ILIKE '%introduced%' THEN 'Introduced'
  ELSE 'In progress'
END`
}

// ListBillSearchFacets returns the distinct prefix/chamber/party/status
// values present in the bill table, for the sidebar facet list.
func (s *Store) ListBillSearchFacets(ctx context.Context) (BillSearchFacets, error) {
	var f BillSearchFacets

	// Prefixes — keep only the first 12 to avoid degenerate noise; the
	// schema allows arbitrary text but in practice WA has ~10 distinct.
	rows, err := s.Pool.Query(ctx, `SELECT DISTINCT prefix FROM bill WHERE number > 0 AND prefix <> '' ORDER BY prefix`)
	if err != nil {
		return f, fmt.Errorf("facets prefix: %w", err)
	}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return f, err
		}
		f.Prefixes = append(f.Prefixes, p)
	}
	rows.Close()

	rows, err = s.Pool.Query(ctx, `SELECT DISTINCT chamber_origin FROM bill WHERE chamber_origin IS NOT NULL AND chamber_origin <> '' ORDER BY chamber_origin`)
	if err != nil {
		return f, fmt.Errorf("facets chamber: %w", err)
	}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			rows.Close()
			return f, err
		}
		f.Chambers = append(f.Chambers, c)
	}
	rows.Close()

	rows, err = s.Pool.Query(ctx, `SELECT DISTINCT party FROM legislator WHERE party IS NOT NULL AND party <> '' ORDER BY party`)
	if err != nil {
		return f, fmt.Errorf("facets party: %w", err)
	}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return f, err
		}
		f.Parties = append(f.Parties, p)
	}
	rows.Close()

	// Statuses come from the lifecycle-stage classifier. Return only
	// stages that actually have rows backing them, so the sidebar
	// doesn't show stages that match nothing in the current dataset.
	stageQ := `SELECT DISTINCT stage FROM (
  SELECT ` + billStageCaseSQL("current_status") + ` AS stage
    FROM bill WHERE number > 0 AND current_status IS NOT NULL AND current_status <> ''
) s WHERE stage <> '' ORDER BY stage`
	rows, err = s.Pool.Query(ctx, stageQ)
	if err != nil {
		return f, fmt.Errorf("facets status: %w", err)
	}
	for rows.Next() {
		var st string
		if err := rows.Scan(&st); err != nil {
			rows.Close()
			return f, err
		}
		f.Statuses = append(f.Statuses, st)
	}
	rows.Close()
	return f, nil
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
// Returns (hits, total, err).
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
		where = append(where, fmt.Sprintf(
			"(b.bill_number ILIKE '%%' || $%d || '%%' OR b.title ILIKE '%%' || $%d || '%%')", idx, idx))
	}
	if sp := strings.TrimSpace(p.Speaker); sp != "" {
		idx := push(sp)
		where = append(where, fmt.Sprintf(
			"EXISTS (SELECT 1 FROM testifier t WHERE t.agenda_item_id = a.id AND t.raw_name ILIKE '%%' || $%d || '%%')", idx))
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
			ors = append(ors, fmt.Sprintf(
				"(b.title ILIKE '%%' || $%d || '%%' OR COALESCE(b.description,'') ILIKE '%%' || $%d || '%%' OR a.label ILIKE '%%' || $%d || '%%' OR h.committee_name ILIKE '%%' || $%d || '%%')",
				idx, idx, idx, idx))
		}
		where = append(where, "("+strings.Join(ors, " OR ")+")")
	}
	if p.Biennium != "" {
		idx := push(p.Biennium)
		where = append(where, fmt.Sprintf("b.biennium = $%d", idx))
	}
	whereSQL := strings.Join(where, " AND ")

	const baseFROM = `
FROM agenda_item a
JOIN hearing h ON h.id = a.hearing_id
JOIN bill    b ON b.id = a.bill_id`

	countQ := "SELECT COUNT(*) " + baseFROM + " WHERE " + whereSQL
	var total int
	if err := s.Pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count hearings: %w", err)
	}

	args = append(args, p.Limit, p.Offset)
	q := `
SELECT a.csi_agenda_item_id, a.label,
       h.committee_name, h.chamber, h.meeting_datetime,
       b.biennium, b.bill_number, b.prefix, b.number,
       (h.tvw_event_id IS NOT NULL) AS has_tvw
` + baseFROM + ` WHERE ` + whereSQL + `
 ORDER BY h.meeting_datetime DESC
 LIMIT $` + fmt.Sprintf("%d", len(args)-1) + ` OFFSET $` + fmt.Sprintf("%d", len(args))
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("search hearings: %w", err)
	}
	defer rows.Close()
	out := []HearingAggregate{}
	for rows.Next() {
		var hh HearingAggregate
		if err := rows.Scan(&hh.CSIAgendaItemID, &hh.AgendaItemLabel,
			&hh.CommitteeName, &hh.Chamber, &hh.MeetingDateTime,
			&hh.Biennium, &hh.BillID, &hh.BillPrefix, &hh.BillNumber,
			&hh.HasTVW); err != nil {
			return nil, 0, fmt.Errorf("scan hearing: %w", err)
		}
		out = append(out, hh)
	}
	return out, total, rows.Err()
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

// UpsertDataWAContractParams is the normalized row shape for datawa_contract.
type UpsertDataWAContractParams struct {
	SourceDatasetID          string
	SourceRowID              string
	FiscalYear               int
	AgencyName               string
	AgencyNumber             string
	ContractNumber           string
	AmendmentNumber          string
	ContractorName           string
	NormalizedContractorName string
	StatewideVendorNumber    string
	Description              string
	StartDate                *time.Time
	EndDate                  *time.Time
	PeriodStart              *time.Time
	PeriodEnd                *time.Time
	FederalAmount            string
	StateAmount              string
	OtherAmount              string
	TotalAmount              string
	ProcurementType          string
	MinorityWomanOwned       string
	SmallBusiness            string
	VeteranOwned             string
	Warnings                 []string
	RawFields                map[string]any
	SourceRecordID           int64
}

// UpsertDataWAContract inserts or updates one normalized data.wa.gov contract row.
func (s *Store) UpsertDataWAContract(ctx context.Context, p UpsertDataWAContractParams) error {
	warnings, err := json.Marshal(p.Warnings)
	if err != nil {
		return fmt.Errorf("marshal warnings: %w", err)
	}
	raw, err := json.Marshal(p.RawFields)
	if err != nil {
		return fmt.Errorf("marshal raw fields: %w", err)
	}
	const q = `
INSERT INTO datawa_contract (
  source_dataset_id, source_row_id, fiscal_year, agency_name, agency_number,
  contract_number, amendment_number, contractor_name, normalized_contractor_name,
  statewide_vendor_number, description, start_date, end_date, period_start, period_end,
  federal_amount, state_amount, other_amount, total_amount, procurement_type,
  minority_woman_owned, small_business, veteran_owned, normalization_warnings,
  raw_fields, source_record_id
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,
  NULLIF($16,'')::numeric, NULLIF($17,'')::numeric, NULLIF($18,'')::numeric, NULLIF($19,'')::numeric,
  $20,$21,$22,$23,$24::jsonb,$25::jsonb,$26
)
ON CONFLICT (source_dataset_id, source_row_id) DO UPDATE SET
  fiscal_year = EXCLUDED.fiscal_year,
  agency_name = EXCLUDED.agency_name,
  agency_number = EXCLUDED.agency_number,
  contract_number = EXCLUDED.contract_number,
  amendment_number = EXCLUDED.amendment_number,
  contractor_name = EXCLUDED.contractor_name,
  normalized_contractor_name = EXCLUDED.normalized_contractor_name,
  statewide_vendor_number = EXCLUDED.statewide_vendor_number,
  description = EXCLUDED.description,
  start_date = EXCLUDED.start_date,
  end_date = EXCLUDED.end_date,
  period_start = EXCLUDED.period_start,
  period_end = EXCLUDED.period_end,
  federal_amount = EXCLUDED.federal_amount,
  state_amount = EXCLUDED.state_amount,
  other_amount = EXCLUDED.other_amount,
  total_amount = EXCLUDED.total_amount,
  procurement_type = EXCLUDED.procurement_type,
  minority_woman_owned = EXCLUDED.minority_woman_owned,
  small_business = EXCLUDED.small_business,
  veteran_owned = EXCLUDED.veteran_owned,
  normalization_warnings = EXCLUDED.normalization_warnings,
  raw_fields = EXCLUDED.raw_fields,
  source_record_id = EXCLUDED.source_record_id,
  updated_at = NOW();`
	_, err = s.Pool.Exec(ctx, q,
		p.SourceDatasetID, p.SourceRowID, p.FiscalYear, strOrNull(p.AgencyName), strOrNull(p.AgencyNumber),
		strOrNull(p.ContractNumber), strOrNull(p.AmendmentNumber), strOrNull(p.ContractorName), strOrNull(defaultStr(p.NormalizedContractorName, entitymatch.NormalizedName(p.ContractorName))),
		strOrNull(p.StatewideVendorNumber), strOrNull(p.Description), datePtrOrNull(p.StartDate), datePtrOrNull(p.EndDate), datePtrOrNull(p.PeriodStart), datePtrOrNull(p.PeriodEnd),
		p.FederalAmount, p.StateAmount, p.OtherAmount, p.TotalAmount, strOrNull(p.ProcurementType),
		strOrNull(p.MinorityWomanOwned), strOrNull(p.SmallBusiness), strOrNull(p.VeteranOwned), string(warnings), string(raw), p.SourceRecordID,
	)
	if err != nil {
		return fmt.Errorf("upsert datawa_contract: %w", err)
	}
	return nil
}

func datePtrOrNull(t *time.Time) pgtype.Date {
	if t == nil || t.IsZero() {
		return pgtype.Date{Valid: false}
	}
	return pgtype.Date{Time: *t, Valid: true}
}

// UpsertDataWAMasterContractSaleParams is the normalized row shape for
// datawa_master_contract_sale.
type UpsertDataWAMasterContractSaleParams struct {
	SourceDatasetID        string
	SourceRowID            string
	CustomerType           string
	CustomerName           string
	NormalizedCustomerName string
	ContractNumber         string
	ContractTitle          string
	VendorName             string
	NormalizedVendorName   string
	ReportYear             int
	Q1SalesReported        string
	Q2SalesReported        string
	Q3SalesReported        string
	Q4SalesReported        string
	TotalSalesReported     string
	OMWBE                  string
	VeteranOwned           string
	SmallBusiness          string
	DiverseOptions         string
	Warnings               []string
	RawFields              map[string]any
	SourceRecordID         int64
}

// UpsertDataWAMasterContractSale inserts or updates one normalized DataWA
// statewide/master-contract sales row.
func (s *Store) UpsertDataWAMasterContractSale(ctx context.Context, p UpsertDataWAMasterContractSaleParams) error {
	warnings, err := json.Marshal(p.Warnings)
	if err != nil {
		return fmt.Errorf("marshal warnings: %w", err)
	}
	raw, err := json.Marshal(p.RawFields)
	if err != nil {
		return fmt.Errorf("marshal raw fields: %w", err)
	}
	const q = `
INSERT INTO datawa_master_contract_sale (
  source_dataset_id, source_row_id, customer_type, customer_name,
  normalized_customer_name, contract_number, contract_title, vendor_name,
  normalized_vendor_name, report_year,
  q1_sales_reported, q2_sales_reported, q3_sales_reported, q4_sales_reported,
  total_sales_reported, omwbe, veteran_owned, small_business, diverse_options,
  normalization_warnings, raw_fields, source_record_id
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10, 0),
  NULLIF($11,'')::numeric, NULLIF($12,'')::numeric, NULLIF($13,'')::numeric, NULLIF($14,'')::numeric,
  NULLIF($15,'')::numeric, $16,$17,$18,$19,$20::jsonb,$21::jsonb,$22
)
ON CONFLICT (source_dataset_id, source_row_id) DO UPDATE SET
  customer_type = EXCLUDED.customer_type,
  customer_name = EXCLUDED.customer_name,
  normalized_customer_name = EXCLUDED.normalized_customer_name,
  contract_number = EXCLUDED.contract_number,
  contract_title = EXCLUDED.contract_title,
  vendor_name = EXCLUDED.vendor_name,
  normalized_vendor_name = EXCLUDED.normalized_vendor_name,
  report_year = EXCLUDED.report_year,
  q1_sales_reported = EXCLUDED.q1_sales_reported,
  q2_sales_reported = EXCLUDED.q2_sales_reported,
  q3_sales_reported = EXCLUDED.q3_sales_reported,
  q4_sales_reported = EXCLUDED.q4_sales_reported,
  total_sales_reported = EXCLUDED.total_sales_reported,
  omwbe = EXCLUDED.omwbe,
  veteran_owned = EXCLUDED.veteran_owned,
  small_business = EXCLUDED.small_business,
  diverse_options = EXCLUDED.diverse_options,
  normalization_warnings = EXCLUDED.normalization_warnings,
  raw_fields = EXCLUDED.raw_fields,
  source_record_id = EXCLUDED.source_record_id,
  updated_at = NOW();`
	_, err = s.Pool.Exec(ctx, q,
		p.SourceDatasetID, p.SourceRowID, strOrNull(p.CustomerType), strOrNull(p.CustomerName),
		strOrNull(defaultStr(p.NormalizedCustomerName, entitymatch.NormalizedName(p.CustomerName))), strOrNull(p.ContractNumber), strOrNull(p.ContractTitle), strOrNull(p.VendorName),
		strOrNull(defaultStr(p.NormalizedVendorName, entitymatch.NormalizedName(p.VendorName))), p.ReportYear,
		p.Q1SalesReported, p.Q2SalesReported, p.Q3SalesReported, p.Q4SalesReported, p.TotalSalesReported,
		strOrNull(p.OMWBE), strOrNull(p.VeteranOwned), strOrNull(p.SmallBusiness), strOrNull(p.DiverseOptions),
		string(warnings), string(raw), p.SourceRecordID,
	)
	if err != nil {
		return fmt.Errorf("upsert datawa_master_contract_sale: %w", err)
	}
	return nil
}

// UpsertDataWAITContractParams is the normalized row shape for
// datawa_it_contract.
type UpsertDataWAITContractParams struct {
	SourceDatasetID           string
	SourceRowID               string
	ReportFiscalYear          int
	AgencyNumberAgencyName    string
	AgencyNumber              string
	AgencyName                string
	ContractNumber            string
	ContractorName            string
	NormalizedContractorName  string
	ContractorDBA             string
	NormalizedContractorDBA   string
	CooperativePurchase       *bool
	CooperativeName           string
	StatewideContractPurchase *bool
	ContractStartDate         *time.Time
	ContractEndDate           *time.Time
	FiscalYearStart           string
	FiscalYearEnd             string
	ITTowerApplication        string
	ITTowerCompute            string
	ITTowerDataCenter         string
	ITTowerDelivery           string
	ITTowerEndUser            string
	ITTowerITManagement       string
	ITTowerNetwork            string
	ITTowerOutput             string
	ITTowerPlatform           string
	ITTowerSecurity           string
	ITTowerStorage            string
	OtherNonIT                string
	TotalPercentage           string
	ContractAmountFY20        string
	ContractAmountFY21        string
	ContractAmountFY22        string
	ContractAmountFY23        string
	ContractAmountFY24        string
	ContractAmountFY25        string
	ContractAmountFY26        string
	ContractAmountFY27        string
	ContractAmountFY28        string
	ContractAmountFY29        string
	ContractAmountFY30        string
	TotalContractAmount       string
	ContractAmountExplanation string
	Warnings                  []string
	RawFields                 map[string]any
	SourceRecordID            int64
}

// UpsertDataWAITContract inserts or updates one normalized DataWA IT contract
// report row.
func (s *Store) UpsertDataWAITContract(ctx context.Context, p UpsertDataWAITContractParams) error {
	warnings, err := json.Marshal(p.Warnings)
	if err != nil {
		return fmt.Errorf("marshal warnings: %w", err)
	}
	raw, err := json.Marshal(p.RawFields)
	if err != nil {
		return fmt.Errorf("marshal raw fields: %w", err)
	}
	const q = `
INSERT INTO datawa_it_contract (
  source_dataset_id, source_row_id, report_fiscal_year, agency_number_agency_name,
  agency_number, agency_name, contract_number, contractor_name,
  normalized_contractor_name, contractor_dba, normalized_contractor_dba,
  cooperative_purchase, cooperative_name, statewide_contract_purchase,
  contract_start_date, contract_end_date, fiscal_year_start, fiscal_year_end,
  it_tower_application, it_tower_compute, it_tower_data_center, it_tower_delivery,
  it_tower_end_user, it_tower_it_management, it_tower_network, it_tower_output,
  it_tower_platform, it_tower_security, it_tower_storage, other_non_it,
  total_percentage, contract_amount_fy20, contract_amount_fy21, contract_amount_fy22,
  contract_amount_fy23, contract_amount_fy24, contract_amount_fy25, contract_amount_fy26,
  contract_amount_fy27, contract_amount_fy28, contract_amount_fy29, contract_amount_fy30,
  total_contract_amount, contract_amount_explanation, normalization_warnings, raw_fields,
  source_record_id
) VALUES (
  $1,$2,NULLIF($3,0),$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,
  NULLIF($19,'')::numeric, NULLIF($20,'')::numeric, NULLIF($21,'')::numeric, NULLIF($22,'')::numeric,
  NULLIF($23,'')::numeric, NULLIF($24,'')::numeric, NULLIF($25,'')::numeric, NULLIF($26,'')::numeric,
  NULLIF($27,'')::numeric, NULLIF($28,'')::numeric, NULLIF($29,'')::numeric, NULLIF($30,'')::numeric,
  NULLIF($31,'')::numeric, NULLIF($32,'')::numeric, NULLIF($33,'')::numeric, NULLIF($34,'')::numeric,
  NULLIF($35,'')::numeric, NULLIF($36,'')::numeric, NULLIF($37,'')::numeric, NULLIF($38,'')::numeric,
  NULLIF($39,'')::numeric, NULLIF($40,'')::numeric, NULLIF($41,'')::numeric, NULLIF($42,'')::numeric,
  NULLIF($43,'')::numeric, $44, $45::jsonb, $46::jsonb, $47
)
ON CONFLICT (source_dataset_id, source_row_id) DO UPDATE SET
  report_fiscal_year = EXCLUDED.report_fiscal_year,
  agency_number_agency_name = EXCLUDED.agency_number_agency_name,
  agency_number = EXCLUDED.agency_number,
  agency_name = EXCLUDED.agency_name,
  contract_number = EXCLUDED.contract_number,
  contractor_name = EXCLUDED.contractor_name,
  normalized_contractor_name = EXCLUDED.normalized_contractor_name,
  contractor_dba = EXCLUDED.contractor_dba,
  normalized_contractor_dba = EXCLUDED.normalized_contractor_dba,
  cooperative_purchase = EXCLUDED.cooperative_purchase,
  cooperative_name = EXCLUDED.cooperative_name,
  statewide_contract_purchase = EXCLUDED.statewide_contract_purchase,
  contract_start_date = EXCLUDED.contract_start_date,
  contract_end_date = EXCLUDED.contract_end_date,
  fiscal_year_start = EXCLUDED.fiscal_year_start,
  fiscal_year_end = EXCLUDED.fiscal_year_end,
  it_tower_application = EXCLUDED.it_tower_application,
  it_tower_compute = EXCLUDED.it_tower_compute,
  it_tower_data_center = EXCLUDED.it_tower_data_center,
  it_tower_delivery = EXCLUDED.it_tower_delivery,
  it_tower_end_user = EXCLUDED.it_tower_end_user,
  it_tower_it_management = EXCLUDED.it_tower_it_management,
  it_tower_network = EXCLUDED.it_tower_network,
  it_tower_output = EXCLUDED.it_tower_output,
  it_tower_platform = EXCLUDED.it_tower_platform,
  it_tower_security = EXCLUDED.it_tower_security,
  it_tower_storage = EXCLUDED.it_tower_storage,
  other_non_it = EXCLUDED.other_non_it,
  total_percentage = EXCLUDED.total_percentage,
  contract_amount_fy20 = EXCLUDED.contract_amount_fy20,
  contract_amount_fy21 = EXCLUDED.contract_amount_fy21,
  contract_amount_fy22 = EXCLUDED.contract_amount_fy22,
  contract_amount_fy23 = EXCLUDED.contract_amount_fy23,
  contract_amount_fy24 = EXCLUDED.contract_amount_fy24,
  contract_amount_fy25 = EXCLUDED.contract_amount_fy25,
  contract_amount_fy26 = EXCLUDED.contract_amount_fy26,
  contract_amount_fy27 = EXCLUDED.contract_amount_fy27,
  contract_amount_fy28 = EXCLUDED.contract_amount_fy28,
  contract_amount_fy29 = EXCLUDED.contract_amount_fy29,
  contract_amount_fy30 = EXCLUDED.contract_amount_fy30,
  total_contract_amount = EXCLUDED.total_contract_amount,
  contract_amount_explanation = EXCLUDED.contract_amount_explanation,
  normalization_warnings = EXCLUDED.normalization_warnings,
  raw_fields = EXCLUDED.raw_fields,
  source_record_id = EXCLUDED.source_record_id,
  updated_at = NOW();`
	_, err = s.Pool.Exec(ctx, q,
		p.SourceDatasetID, p.SourceRowID, p.ReportFiscalYear, strOrNull(p.AgencyNumberAgencyName),
		strOrNull(p.AgencyNumber), strOrNull(p.AgencyName), strOrNull(p.ContractNumber), strOrNull(p.ContractorName),
		strOrNull(defaultStr(p.NormalizedContractorName, entitymatch.NormalizedName(p.ContractorName))), strOrNull(p.ContractorDBA),
		strOrNull(defaultStr(p.NormalizedContractorDBA, entitymatch.NormalizedName(p.ContractorDBA))), boolPtrOrNull(p.CooperativePurchase),
		strOrNull(p.CooperativeName), boolPtrOrNull(p.StatewideContractPurchase),
		datePtrOrNull(p.ContractStartDate), datePtrOrNull(p.ContractEndDate), strOrNull(p.FiscalYearStart), strOrNull(p.FiscalYearEnd),
		p.ITTowerApplication, p.ITTowerCompute, p.ITTowerDataCenter, p.ITTowerDelivery, p.ITTowerEndUser, p.ITTowerITManagement,
		p.ITTowerNetwork, p.ITTowerOutput, p.ITTowerPlatform, p.ITTowerSecurity, p.ITTowerStorage, p.OtherNonIT, p.TotalPercentage,
		p.ContractAmountFY20, p.ContractAmountFY21, p.ContractAmountFY22, p.ContractAmountFY23, p.ContractAmountFY24, p.ContractAmountFY25,
		p.ContractAmountFY26, p.ContractAmountFY27, p.ContractAmountFY28, p.ContractAmountFY29, p.ContractAmountFY30, p.TotalContractAmount,
		strOrNull(p.ContractAmountExplanation), string(warnings), string(raw), p.SourceRecordID,
	)
	if err != nil {
		return fmt.Errorf("upsert datawa_it_contract: %w", err)
	}
	return nil
}

func boolPtrOrNull(b *bool) any {
	if b == nil {
		return nil
	}
	return *b
}

// UpsertDataWAWEBSVendorParams is the normalized row shape for datawa_webs_vendor.
type UpsertDataWAWEBSVendorParams struct {
	SourceDatasetID       string
	SourceRowID           string
	CompanyName           string
	NormalizedCompanyName string
	DBAName               string
	PhoneNumber           string
	ContactEmail          string
	City                  string
	State                 string
	WebAddress            string
	CommodityCode         string
	DescriptionOfWork     string
	SmallBusiness         string
	VeteranOwned          string
	OtherCert             string
	OtherCert2            string
	Warnings              []string
	RawFields             map[string]any
	SourceRecordID        int64
}

// UpsertDataWAWEBSVendor inserts or updates one normalized WEBS vendor row.
func (s *Store) UpsertDataWAWEBSVendor(ctx context.Context, p UpsertDataWAWEBSVendorParams) error {
	warnings, err := json.Marshal(p.Warnings)
	if err != nil {
		return fmt.Errorf("marshal warnings: %w", err)
	}
	raw, err := json.Marshal(p.RawFields)
	if err != nil {
		return fmt.Errorf("marshal raw fields: %w", err)
	}
	const q = `
INSERT INTO datawa_webs_vendor (
  source_dataset_id, source_row_id, company_name, normalized_company_name,
  dba_name, phone_number, contact_email, city, state, web_address,
  commodity_code, description_of_work, small_business, veteran_owned,
  other_cert, other_cert_2, normalization_warnings, raw_fields, source_record_id
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17::jsonb,$18::jsonb,$19
)
ON CONFLICT (source_dataset_id, source_row_id) DO UPDATE SET
  company_name = EXCLUDED.company_name,
  normalized_company_name = EXCLUDED.normalized_company_name,
  dba_name = EXCLUDED.dba_name,
  phone_number = EXCLUDED.phone_number,
  contact_email = EXCLUDED.contact_email,
  city = EXCLUDED.city,
  state = EXCLUDED.state,
  web_address = EXCLUDED.web_address,
  commodity_code = EXCLUDED.commodity_code,
  description_of_work = EXCLUDED.description_of_work,
  small_business = EXCLUDED.small_business,
  veteran_owned = EXCLUDED.veteran_owned,
  other_cert = EXCLUDED.other_cert,
  other_cert_2 = EXCLUDED.other_cert_2,
  normalization_warnings = EXCLUDED.normalization_warnings,
  raw_fields = EXCLUDED.raw_fields,
  source_record_id = EXCLUDED.source_record_id,
  updated_at = NOW();`
	_, err = s.Pool.Exec(ctx, q,
		p.SourceDatasetID, p.SourceRowID, strOrNull(p.CompanyName), strOrNull(p.NormalizedCompanyName),
		strOrNull(p.DBAName), strOrNull(p.PhoneNumber), strOrNull(p.ContactEmail), strOrNull(p.City), strOrNull(p.State), strOrNull(p.WebAddress),
		strOrNull(p.CommodityCode), strOrNull(p.DescriptionOfWork), strOrNull(p.SmallBusiness), strOrNull(p.VeteranOwned),
		strOrNull(p.OtherCert), strOrNull(p.OtherCert2), string(warnings), string(raw), p.SourceRecordID,
	)
	if err != nil {
		return fmt.Errorf("upsert datawa_webs_vendor: %w", err)
	}
	return nil
}

// Vendor/entity match candidates and decisions.

type VendorEntityMatchCandidate struct {
	ID                  int64
	SourceKind          string
	SourceTable         string
	SourcePK            int64
	SourceDatasetID     string
	SourceRowID         string
	SourceName          string
	NormalizedName      string
	OrganizationID      int64
	CanonicalName       string
	CandidateConfidence string
	Evidence            []string
	SourceRecordID      int64
}

type UpsertVendorEntityMatchCandidateParams struct {
	SourceKind          string
	SourceTable         string
	SourcePK            int64
	SourceDatasetID     string
	SourceRowID         string
	SourceName          string
	NormalizedName      string
	OrganizationID      int64
	CandidateConfidence string
	Evidence            []string
	SourceRecordID      int64
}

func (s *Store) UpsertVendorEntityMatchCandidate(ctx context.Context, p UpsertVendorEntityMatchCandidateParams) (int64, error) {
	evidence, err := json.Marshal(entitymatch.UniqueStrings(p.Evidence))
	if err != nil {
		return 0, fmt.Errorf("marshal evidence: %w", err)
	}
	const q = `
INSERT INTO vendor_entity_match_candidate (
  source_kind, source_table, source_pk, source_dataset_id, source_row_id,
  source_name, normalized_name, organization_id, candidate_confidence,
  evidence, source_record_id
) VALUES (
  $1::entity_match_source_kind,$2,NULLIF($3,0),$4,$5,$6,$7,$8,$9::org_match_confidence,$10::jsonb,NULLIF($11,0)
)
ON CONFLICT (source_kind, source_dataset_id, source_row_id, source_name, organization_id) DO UPDATE SET
  source_table = EXCLUDED.source_table,
  source_pk = EXCLUDED.source_pk,
  normalized_name = EXCLUDED.normalized_name,
  candidate_confidence = EXCLUDED.candidate_confidence,
  evidence = EXCLUDED.evidence,
  source_record_id = EXCLUDED.source_record_id,
  updated_at = NOW()
RETURNING id;`
	var id int64
	if err := s.Pool.QueryRow(ctx, q,
		p.SourceKind, p.SourceTable, p.SourcePK, strOrNull(p.SourceDatasetID), strOrNull(p.SourceRowID),
		p.SourceName, p.NormalizedName, p.OrganizationID, defaultStr(p.CandidateConfidence, "possible"), string(evidence), p.SourceRecordID,
	).Scan(&id); err != nil {
		return 0, fmt.Errorf("upsert vendor entity match candidate: %w", err)
	}
	return id, nil
}

type InsertVendorEntityMatchDecisionParams struct {
	CandidateID    int64
	OrganizationID int64
	Decision       string
	Confidence     string
	ReviewedBy     string
	ReviewNotes    string
}

func (s *Store) UpsertVendorEntityMatchDecision(ctx context.Context, p InsertVendorEntityMatchDecisionParams) (int64, error) {
	const q = `
INSERT INTO vendor_entity_match_decision (
  candidate_id, organization_id, decision, reviewed_confidence, reviewed_by, review_notes
) VALUES ($1,$2,$3::entity_match_decision,$4::org_match_confidence,$5,$6)
ON CONFLICT (candidate_id) DO UPDATE SET
  organization_id = EXCLUDED.organization_id,
  decision = EXCLUDED.decision,
  reviewed_confidence = EXCLUDED.reviewed_confidence,
  reviewed_by = EXCLUDED.reviewed_by,
  review_notes = EXCLUDED.review_notes,
  reviewed_at = NOW()
RETURNING id;`
	var id int64
	if err := s.Pool.QueryRow(ctx, q,
		p.CandidateID, p.OrganizationID, defaultStr(p.Decision, "needs_review"), defaultStr(p.Confidence, "possible"),
		strOrNull(p.ReviewedBy), strOrNull(p.ReviewNotes),
	).Scan(&id); err != nil {
		return 0, fmt.Errorf("upsert vendor entity match decision: %w", err)
	}
	return id, nil
}

func (s *Store) GenerateVendorEntityMatchCandidates(ctx context.Context, limit int) ([]VendorEntityMatchCandidate, error) {
	rows, err := s.Pool.Query(ctx, vendorCandidateSourceQuery, limit)
	if err != nil {
		return nil, fmt.Errorf("vendor entity candidate source query: %w", err)
	}
	defer rows.Close()

	var out []VendorEntityMatchCandidate
	for rows.Next() {
		var sourceKind, sourceTable, sourceDatasetID, sourceRowID, sourceName, normalizedName string
		var sourcePK, sourceRecordID int64
		var orgID int64
		var canonical string
		var aliases []string
		if err := rows.Scan(&sourceKind, &sourceTable, &sourcePK, &sourceDatasetID, &sourceRowID, &sourceName, &normalizedName, &sourceRecordID, &orgID, &canonical, &aliases); err != nil {
			return nil, fmt.Errorf("scan vendor entity candidate source: %w", err)
		}
		if entitymatch.FalsePositiveRisk(normalizedName) {
			continue
		}
		confidence, evidence := entitymatch.ConfidenceFor(sourceName, canonical, aliases)
		if confidence == "" {
			continue
		}
		id, err := s.UpsertVendorEntityMatchCandidate(ctx, UpsertVendorEntityMatchCandidateParams{
			SourceKind:          sourceKind,
			SourceTable:         sourceTable,
			SourcePK:            sourcePK,
			SourceDatasetID:     sourceDatasetID,
			SourceRowID:         sourceRowID,
			SourceName:          sourceName,
			NormalizedName:      normalizedName,
			OrganizationID:      orgID,
			CandidateConfidence: confidence,
			Evidence:            evidence,
			SourceRecordID:      sourceRecordID,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, VendorEntityMatchCandidate{
			ID: id, SourceKind: sourceKind, SourceTable: sourceTable, SourcePK: sourcePK,
			SourceDatasetID: sourceDatasetID, SourceRowID: sourceRowID, SourceName: sourceName,
			NormalizedName: normalizedName, OrganizationID: orgID, CanonicalName: canonical,
			CandidateConfidence: confidence, Evidence: evidence, SourceRecordID: sourceRecordID,
		})
	}
	return out, rows.Err()
}

const vendorCandidateSourceQuery = `
WITH source_names AS (
  SELECT 'datawa_contract_contractor'::text AS source_kind, 'datawa_contract'::text AS source_table,
         id AS source_pk, source_dataset_id, source_row_id, contractor_name AS source_name,
         normalized_contractor_name AS normalized_name, source_record_id
    FROM datawa_contract
   WHERE contractor_name IS NOT NULL AND normalized_contractor_name IS NOT NULL
  UNION ALL
  SELECT 'datawa_master_contract_vendor', 'datawa_master_contract_sale',
         id, source_dataset_id, source_row_id, vendor_name, normalized_vendor_name, source_record_id
    FROM datawa_master_contract_sale
   WHERE vendor_name IS NOT NULL AND normalized_vendor_name IS NOT NULL
  UNION ALL
  SELECT 'datawa_master_contract_customer', 'datawa_master_contract_sale',
         id, source_dataset_id, source_row_id, customer_name, normalized_customer_name, source_record_id
    FROM datawa_master_contract_sale
   WHERE customer_name IS NOT NULL AND normalized_customer_name IS NOT NULL
  UNION ALL
  SELECT 'datawa_it_contract_contractor', 'datawa_it_contract',
         id, source_dataset_id, source_row_id, contractor_name, normalized_contractor_name, source_record_id
    FROM datawa_it_contract
   WHERE contractor_name IS NOT NULL AND normalized_contractor_name IS NOT NULL
  UNION ALL
  SELECT 'datawa_it_contract_dba', 'datawa_it_contract',
         id, source_dataset_id, source_row_id, contractor_dba, normalized_contractor_dba, source_record_id
    FROM datawa_it_contract
   WHERE contractor_dba IS NOT NULL AND normalized_contractor_dba IS NOT NULL
  UNION ALL
  SELECT 'datawa_webs_vendor', 'datawa_webs_vendor',
         id, source_dataset_id, source_row_id, company_name, normalized_company_name, source_record_id
    FROM datawa_webs_vendor
   WHERE company_name IS NOT NULL AND normalized_company_name IS NOT NULL
)
SELECT s.source_kind, s.source_table, s.source_pk, s.source_dataset_id, s.source_row_id,
       s.source_name, s.normalized_name, s.source_record_id,
       o.id, o.canonical_name, o.aliases
  FROM source_names s
  JOIN organization o
    ON s.normalized_name = wa_dd_normalize_entity_name(o.canonical_name)
    OR s.normalized_name = ANY(
       SELECT wa_dd_normalize_entity_name(alias)
         FROM unnest(o.aliases) alias
    )
 ORDER BY s.source_kind, s.normalized_name, o.canonical_name
 LIMIT CASE WHEN $1 > 0 THEN $1 ELSE 100000 END;`

// UpsertFederalAwardParams is the normalized row shape for federal_award.
type UpsertFederalAwardParams struct {
	AwardID        string
	RecipientName  string
	RecipientUEI   string
	AwardingAgency string
	FundingAgency  string
	AwardType      string
	AwardAmount    string
	StartDate      *time.Time
	EndDate        *time.Time
	PlaceStateCode string
	PlaceCounty    string
	RawFields      map[string]any
	SourceRecordID int64
}

// UpsertFederalAward inserts or updates one USAspending award row.
func (s *Store) UpsertFederalAward(ctx context.Context, p UpsertFederalAwardParams) error {
	raw, err := json.Marshal(p.RawFields)
	if err != nil {
		return fmt.Errorf("marshal raw fields: %w", err)
	}
	const q = `
INSERT INTO federal_award (
  award_id, recipient_name, recipient_uei, awarding_agency, funding_agency,
  award_type, award_amount, start_date, end_date, place_state_code,
  place_county, raw_fields, source_record_id
) VALUES (
  $1,$2,$3,$4,$5,$6,NULLIF($7,'')::numeric,$8,$9,$10,$11,$12::jsonb,$13
)
ON CONFLICT (award_id) DO UPDATE SET
  recipient_name = EXCLUDED.recipient_name,
  recipient_uei = EXCLUDED.recipient_uei,
  awarding_agency = EXCLUDED.awarding_agency,
  funding_agency = EXCLUDED.funding_agency,
  award_type = EXCLUDED.award_type,
  award_amount = EXCLUDED.award_amount,
  start_date = EXCLUDED.start_date,
  end_date = EXCLUDED.end_date,
  place_state_code = EXCLUDED.place_state_code,
  place_county = EXCLUDED.place_county,
  raw_fields = EXCLUDED.raw_fields,
  source_record_id = EXCLUDED.source_record_id,
  updated_at = NOW();`
	_, err = s.Pool.Exec(ctx, q,
		p.AwardID, strOrNull(p.RecipientName), strOrNull(p.RecipientUEI), strOrNull(p.AwardingAgency), strOrNull(p.FundingAgency),
		strOrNull(p.AwardType), p.AwardAmount, datePtrOrNull(p.StartDate), datePtrOrNull(p.EndDate), strOrNull(p.PlaceStateCode),
		strOrNull(p.PlaceCounty), string(raw), p.SourceRecordID,
	)
	if err != nil {
		return fmt.Errorf("upsert federal_award: %w", err)
	}
	return nil
}

// UpsertSeattleOperatingBudgetParams is the normalized row shape for
// seattle_operating_budget.
type UpsertSeattleOperatingBudgetParams struct {
	SourceDatasetID string
	SourceRowID     string
	FiscalYear      int
	Service         string
	Department      string
	Program         string
	Fund            string
	FundType        string
	ExpenseType     string
	Description     string
	ApprovedAmount  string
	RawFields       map[string]any
	SourceRecordID  int64
}

// UpsertSeattleOperatingBudget inserts or updates one Seattle operating budget row.
func (s *Store) UpsertSeattleOperatingBudget(ctx context.Context, p UpsertSeattleOperatingBudgetParams) error {
	raw, err := json.Marshal(p.RawFields)
	if err != nil {
		return fmt.Errorf("marshal raw fields: %w", err)
	}
	const q = `
INSERT INTO seattle_operating_budget (
  source_dataset_id, source_row_id, fiscal_year, service, department, program,
  fund, fund_type, expense_type, description, approved_amount, raw_fields,
  source_record_id
) VALUES (
  $1,$2,NULLIF($3,0),$4,$5,$6,$7,$8,$9,$10,NULLIF($11,'')::numeric,$12::jsonb,$13
)
ON CONFLICT (source_dataset_id, source_row_id) DO UPDATE SET
  fiscal_year = EXCLUDED.fiscal_year,
  service = EXCLUDED.service,
  department = EXCLUDED.department,
  program = EXCLUDED.program,
  fund = EXCLUDED.fund,
  fund_type = EXCLUDED.fund_type,
  expense_type = EXCLUDED.expense_type,
  description = EXCLUDED.description,
  approved_amount = EXCLUDED.approved_amount,
  raw_fields = EXCLUDED.raw_fields,
  source_record_id = EXCLUDED.source_record_id,
  updated_at = NOW();`
	_, err = s.Pool.Exec(ctx, q,
		p.SourceDatasetID, p.SourceRowID, p.FiscalYear, strOrNull(p.Service), strOrNull(p.Department), strOrNull(p.Program),
		strOrNull(p.Fund), strOrNull(p.FundType), strOrNull(p.ExpenseType), strOrNull(p.Description), p.ApprovedAmount,
		string(raw), p.SourceRecordID,
	)
	if err != nil {
		return fmt.Errorf("upsert seattle_operating_budget: %w", err)
	}
	return nil
}

// UpsertFiscalWAVendorPaymentParams is the normalized row shape for
// fiscalwa_vendor_payment.
type UpsertFiscalWAVendorPaymentParams struct {
	SourceDatasetID string
	SourceRowID     string
	Biennium        string
	FiscalYear      int
	FiscalMonth     string
	AgencyNumber    string
	AgencyName      string
	ObjectCode      string
	ObjectCategory  string
	SubobjectCode   string
	SubobjectName   string
	VendorName      string
	Amount          string
	RawFields       map[string]any
	SourceRecordID  int64
}

// UpsertFiscalWAVendorPayment inserts or updates one fiscal.wa.gov vendor
// payment row from the Open Checkbook workbook.
func (s *Store) UpsertFiscalWAVendorPayment(ctx context.Context, p UpsertFiscalWAVendorPaymentParams) error {
	raw, err := json.Marshal(p.RawFields)
	if err != nil {
		return fmt.Errorf("marshal raw fields: %w", err)
	}
	const q = `
INSERT INTO fiscalwa_vendor_payment (
  source_dataset_id, source_row_id, biennium, fiscal_year, fiscal_month,
  agency_number, agency_name, object_code, object_category, subobject_code,
  subobject_name, vendor_name, amount, raw_fields, source_record_id
) VALUES (
  $1,$2,$3,NULLIF($4,0),$5,$6,$7,$8,$9,$10,$11,$12,NULLIF($13,'')::numeric,$14::jsonb,$15
)
ON CONFLICT (source_dataset_id, source_row_id) DO UPDATE SET
  biennium = EXCLUDED.biennium,
  fiscal_year = EXCLUDED.fiscal_year,
  fiscal_month = EXCLUDED.fiscal_month,
  agency_number = EXCLUDED.agency_number,
  agency_name = EXCLUDED.agency_name,
  object_code = EXCLUDED.object_code,
  object_category = EXCLUDED.object_category,
  subobject_code = EXCLUDED.subobject_code,
  subobject_name = EXCLUDED.subobject_name,
  vendor_name = EXCLUDED.vendor_name,
  amount = EXCLUDED.amount,
  raw_fields = EXCLUDED.raw_fields,
  source_record_id = EXCLUDED.source_record_id,
  updated_at = NOW();`
	_, err = s.Pool.Exec(ctx, q,
		p.SourceDatasetID, p.SourceRowID, strOrNull(p.Biennium), p.FiscalYear, strOrNull(p.FiscalMonth),
		strOrNull(p.AgencyNumber), strOrNull(p.AgencyName), strOrNull(p.ObjectCode), strOrNull(p.ObjectCategory),
		strOrNull(p.SubobjectCode), strOrNull(p.SubobjectName), strOrNull(p.VendorName), p.Amount, string(raw), p.SourceRecordID,
	)
	if err != nil {
		return fmt.Errorf("upsert fiscalwa_vendor_payment: %w", err)
	}
	return nil
}

// ListFreshBillKeys returns the set of "PREFIX|NUMBER" keys for bills in
// the given biennium whose row was upserted within the last `since`
// duration. Used by `wa-dd ingest-session --skip-fresh` to resume a
// killed run without re-fetching bills already pulled this cycle.
func (s *Store) ListFreshBillKeys(ctx context.Context, biennium string, since time.Duration) (map[string]struct{}, error) {
	const q = `
SELECT prefix, number
  FROM bill
 WHERE biennium = $1
   AND updated_at >= NOW() - $2::interval;`
	rows, err := s.Pool.Query(ctx, q, biennium, fmt.Sprintf("%d seconds", int64(since.Seconds())))
	if err != nil {
		return nil, fmt.Errorf("list fresh bill keys: %w", err)
	}
	defer rows.Close()
	out := map[string]struct{}{}
	for rows.Next() {
		var prefix string
		var number int
		if err := rows.Scan(&prefix, &number); err != nil {
			return nil, fmt.Errorf("scan fresh bill: %w", err)
		}
		out[fmt.Sprintf("%s|%d", prefix, number)] = struct{}{}
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
// or a Committee Schedules agenda ID) are skipped. Gubernatorial
// appointments (SGA) are also skipped: LWS stores them as bill-like rows,
// but CSI does not expose them as testimony agenda items in the data this
// pipeline ingests.
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
// hearing has a tvw_event_id (so transcript ingest can run) and (b) the
// full ingest pipeline has not yet succeeded end-to-end for that agenda
// item. The terminal step is `pdc-context`: a `succeeded` ingestion_run
// row tagged with this agenda's csi_agenda_item_id proves all 6 steps
// ran. Items that died mid-pipeline (e.g. testifiers ingested but
// transcript segmentation failed) come back into the work list for a
// retry. Used by `wa-dd ingest-hearings` to feed buildOne.
func (s *Store) ListDiscoveredAgendaItems(ctx context.Context, biennium string) ([]DiscoveredAgendaItemRow, error) {
	const q = `
SELECT a.csi_agenda_item_id, b.biennium, b.prefix, b.number
  FROM agenda_item a
  JOIN hearing h ON h.id = a.hearing_id
  JOIN bill    b ON b.id = a.bill_id
 WHERE b.biennium = $1
   AND h.tvw_event_id IS NOT NULL
   AND NOT EXISTS (
     SELECT 1 FROM ingestion_run r
      WHERE r.job = 'pdc-context'
        AND r.status = 'succeeded'
        AND r.args ->> 'agenda_item_id' = a.csi_agenda_item_id
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

// ---------------------------------------------------------------------------
// audio cache + diarization
// ---------------------------------------------------------------------------

type TVWAudioSource struct {
	TVWEventID string
	URL        string
	Kind       string
}

// BestTVWAudioSource returns the preferred downloadable source for an event:
// direct audio first, then audio media assets, then video fallback for ffmpeg
// extraction.
func (s *Store) BestTVWAudioSource(ctx context.Context, eventID string) (TVWAudioSource, error) {
	const q = `
WITH candidates AS (
  SELECT tvw_event_id, audio_download_url AS url, 'audio_download_url' AS kind, 1 AS priority
    FROM tvw_event WHERE tvw_event_id = $1 AND NULLIF(audio_download_url, '') IS NOT NULL
  UNION ALL
  SELECT tvw_event_id, published_audio_url AS url, 'published_audio_url' AS kind, 2 AS priority
    FROM tvw_event WHERE tvw_event_id = $1 AND NULLIF(published_audio_url, '') IS NOT NULL
  UNION ALL
  SELECT tvw_event_id, file_url AS url, 'media_asset_audio' AS kind, 3 AS priority
    FROM tvw_media_asset WHERE tvw_event_id = $1 AND asset_type ILIKE 'audio' AND NULLIF(file_url, '') IS NOT NULL
  UNION ALL
  SELECT tvw_event_id, video_download_url AS url, 'video_download_url' AS kind, 4 AS priority
    FROM tvw_event WHERE tvw_event_id = $1 AND NULLIF(video_download_url, '') IS NOT NULL
  UNION ALL
  SELECT tvw_event_id, file_url AS url, 'media_asset_video' AS kind, 5 AS priority
    FROM tvw_media_asset WHERE tvw_event_id = $1 AND asset_type ILIKE 'video' AND NULLIF(file_url, '') IS NOT NULL
)
SELECT tvw_event_id, url, kind FROM candidates ORDER BY priority LIMIT 1;`
	var out TVWAudioSource
	if err := s.Pool.QueryRow(ctx, q, eventID).Scan(&out.TVWEventID, &out.URL, &out.Kind); err != nil {
		return TVWAudioSource{}, fmt.Errorf("best tvw audio source: %w", err)
	}
	return out, nil
}

type UpsertTVWAudioAssetParams struct {
	TVWEventID     string
	SourceURL      string
	SourceKind     string
	OriginalPath   string
	NormalizedPath string
	ContentHash    string
	DurationMS     int
	SampleRate     int
	Channels       int
	Codec          string
}

func (s *Store) UpsertTVWAudioAsset(ctx context.Context, p UpsertTVWAudioAssetParams) (int64, error) {
	const q = `
INSERT INTO tvw_audio_asset (tvw_event_id, source_url, source_kind, original_path,
                             normalized_path, content_hash, duration_ms,
                             sample_rate, channels, codec)
VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,0),NULLIF($8,0),NULLIF($9,0),NULLIF($10,''))
ON CONFLICT (tvw_event_id, content_hash) DO UPDATE SET
  source_url = EXCLUDED.source_url,
  source_kind = EXCLUDED.source_kind,
  original_path = EXCLUDED.original_path,
  normalized_path = EXCLUDED.normalized_path,
  duration_ms = EXCLUDED.duration_ms,
  sample_rate = EXCLUDED.sample_rate,
  channels = EXCLUDED.channels,
  codec = EXCLUDED.codec
RETURNING id;`
	var id int64
	err := s.Pool.QueryRow(ctx, q, p.TVWEventID, p.SourceURL, p.SourceKind,
		p.OriginalPath, p.NormalizedPath, p.ContentHash, p.DurationMS,
		p.SampleRate, p.Channels, p.Codec).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert tvw_audio_asset: %w", err)
	}
	return id, nil
}

type LatestTVWAudioAsset struct {
	ID             int64
	TVWEventID     string
	SourceURL      string
	SourceKind     string
	OriginalPath   string
	NormalizedPath string
	ContentHash    string
	DurationMS     int
}

func (s *Store) LatestTVWAudioAsset(ctx context.Context, eventID string) (LatestTVWAudioAsset, error) {
	const q = `
SELECT id, tvw_event_id, source_url, source_kind, original_path,
       normalized_path, content_hash, COALESCE(duration_ms, 0)
  FROM tvw_audio_asset
 WHERE tvw_event_id = $1
 ORDER BY created_at DESC, id DESC
 LIMIT 1;`
	var out LatestTVWAudioAsset
	if err := s.Pool.QueryRow(ctx, q, eventID).Scan(&out.ID, &out.TVWEventID, &out.SourceURL,
		&out.SourceKind, &out.OriginalPath, &out.NormalizedPath, &out.ContentHash, &out.DurationMS); err != nil {
		return LatestTVWAudioAsset{}, fmt.Errorf("latest tvw_audio_asset: %w", err)
	}
	return out, nil
}

type CreateDiarizationJobParams struct {
	TVWEventID   string
	AudioAssetID int64
	Provider     string
	Model        string
}

func (s *Store) CreateDiarizationJob(ctx context.Context, p CreateDiarizationJobParams) (int64, error) {
	const q = `
INSERT INTO diarization_job (tvw_event_id, audio_asset_id, provider, model, status, submitted_at)
VALUES ($1, $2, $3, NULLIF($4,''), 'running', NOW())
RETURNING id;`
	var id int64
	if err := s.Pool.QueryRow(ctx, q, p.TVWEventID, p.AudioAssetID, p.Provider, p.Model).Scan(&id); err != nil {
		return 0, fmt.Errorf("create diarization_job: %w", err)
	}
	return id, nil
}

func (s *Store) FailDiarizationJob(ctx context.Context, jobID int64, msg string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE diarization_job SET status = 'failed', finished_at = NOW(), error = $2 WHERE id = $1`, jobID, msg)
	if err != nil {
		return fmt.Errorf("fail diarization_job: %w", err)
	}
	return nil
}

type InsertDiarizationResultParams struct {
	JobID         int64
	TVWEventID    string
	Provider      string
	Model         string
	RawResultPath string
	Segments      []DiarizedSegmentParams
	Entities      []EntityMentionParams
}

type DiarizedSegmentParams struct {
	ClusterLabel string
	StartMS      int
	EndMS        int
	Confidence   *float64
	Text         string
	Raw          map[string]any
}

type EntityMentionParams struct {
	SourceKind     string
	SourceID       int64
	Extractor      string
	Model          string
	EntityType     string
	Text           string
	NormalizedText string
	StartMS        *int
	EndMS          *int
	StartWord      *int
	EndWord        *int
	Confidence     *float64
	Raw            map[string]any
}

func (s *Store) InsertDiarizationResult(ctx context.Context, p InsertDiarizationResultParams) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM entity_mention WHERE diarization_job_id = $1`, p.JobID); err != nil {
		return fmt.Errorf("delete entity mentions: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM diarized_speech_segment WHERE diarization_job_id = $1`, p.JobID); err != nil {
		return fmt.Errorf("delete diarized segments: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM speaker_cluster WHERE diarization_job_id = $1`, p.JobID); err != nil {
		return fmt.Errorf("delete speaker clusters: %w", err)
	}

	type agg struct{ total, count int }
	aggs := map[string]agg{}
	for _, seg := range p.Segments {
		a := aggs[seg.ClusterLabel]
		a.total += seg.EndMS - seg.StartMS
		a.count++
		aggs[seg.ClusterLabel] = a
	}

	clusterIDs := map[string]int64{}
	for label, a := range aggs {
		var id int64
		if err := tx.QueryRow(ctx, `
INSERT INTO speaker_cluster (diarization_job_id, tvw_event_id, cluster_label, total_speech_ms, turn_count)
VALUES ($1,$2,$3,$4,$5)
RETURNING id;`, p.JobID, p.TVWEventID, label, a.total, a.count).Scan(&id); err != nil {
			return fmt.Errorf("insert speaker_cluster: %w", err)
		}
		clusterIDs[label] = id
	}

	const segQ = `
INSERT INTO diarized_speech_segment (diarization_job_id, speaker_cluster_id, tvw_event_id,
                                     cluster_label, start_ms, end_ms, confidence, text, raw)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9);`
	for _, seg := range p.Segments {
		raw, err := marshalJSONDefault(seg.Raw, map[string]any{})
		if err != nil {
			return fmt.Errorf("marshal diarized segment raw: %w", err)
		}
		if _, err := tx.Exec(ctx, segQ, p.JobID, clusterIDs[seg.ClusterLabel], p.TVWEventID,
			seg.ClusterLabel, seg.StartMS, seg.EndMS, floatPtrOrNull(seg.Confidence), strOrNull(seg.Text), string(raw)); err != nil {
			return fmt.Errorf("insert diarized segment: %w", err)
		}
	}

	const entQ = `
INSERT INTO entity_mention (tvw_event_id, diarization_job_id, source_kind, source_id,
                            extractor, model, entity_type, text, normalized_text,
                            start_ms, end_ms, start_word, end_word, confidence, raw)
VALUES ($1,$2,$3,NULLIF($4,0),$5,NULLIF($6,''),$7,$8,NULLIF($9,''),$10,$11,$12,$13,$14,$15);`
	for _, ent := range p.Entities {
		raw, err := marshalJSONDefault(ent.Raw, map[string]any{})
		if err != nil {
			return fmt.Errorf("marshal entity mention raw: %w", err)
		}
		sourceKind := defaultStr(ent.SourceKind, "diarization_job")
		extractor := defaultStr(ent.Extractor, p.Provider)
		model := ent.Model
		if model == "" {
			model = p.Model
		}
		if _, err := tx.Exec(ctx, entQ, p.TVWEventID, p.JobID, sourceKind, ent.SourceID,
			extractor, model, ent.EntityType, ent.Text, ent.NormalizedText,
			intPtrOrNull(ent.StartMS), intPtrOrNull(ent.EndMS), intPtrOrNull(ent.StartWord),
			intPtrOrNull(ent.EndWord), floatPtrOrNull(ent.Confidence), string(raw)); err != nil {
			return fmt.Errorf("insert entity mention: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `UPDATE diarization_job SET status = 'succeeded', finished_at = NOW(), raw_result_path = NULLIF($2,'') WHERE id = $1`, p.JobID, p.RawResultPath); err != nil {
		return fmt.Errorf("update diarization_job: %w", err)
	}
	return tx.Commit(ctx)
}

func floatPtrOrNull(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func intPtrOrNull(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

// ---------------------------------------------------------------------------
// speaker identity evidence + review
// ---------------------------------------------------------------------------

type SpeakerIdentityEvidenceParams struct {
	EvidenceKey             string
	DiarizationJobID        int64
	SpeakerClusterID        int64
	DiarizedSpeechSegmentID int64
	EvidenceType            string
	EvidenceText            string
	CandidateKind           string
	CandidateID             int64
	CandidateLabel          string
	Confidence              float64
	StartMS                 int
	EndMS                   int
	Raw                     map[string]any
}

func (s *Store) UpsertSpeakerIdentityEvidence(ctx context.Context, p SpeakerIdentityEvidenceParams) (int64, error) {
	raw, err := marshalJSONDefault(p.Raw, map[string]any{})
	if err != nil {
		return 0, fmt.Errorf("marshal speaker evidence raw: %w", err)
	}
	const q = `
INSERT INTO speaker_identity_evidence (evidence_key, diarization_job_id, speaker_cluster_id,
                                       diarized_speech_segment_id, evidence_type, evidence_text,
                                       candidate_kind, candidate_id, candidate_label, confidence,
                                       start_ms, end_ms, raw)
VALUES ($1,$2,$3,NULLIF($4,0),$5::speaker_evidence_type,$6,$7::speaker_candidate_kind,
        NULLIF($8,0),$9,NULLIF($10,0),NULLIF($11,0),NULLIF($12,0),$13::jsonb)
ON CONFLICT (evidence_key) DO UPDATE SET
  evidence_text = EXCLUDED.evidence_text,
  confidence = EXCLUDED.confidence,
  raw = EXCLUDED.raw
RETURNING id;`
	var id int64
	if err := s.Pool.QueryRow(ctx, q, p.EvidenceKey, p.DiarizationJobID, p.SpeakerClusterID,
		p.DiarizedSpeechSegmentID, p.EvidenceType, p.EvidenceText, p.CandidateKind,
		p.CandidateID, p.CandidateLabel, p.Confidence, p.StartMS, p.EndMS, string(raw)).Scan(&id); err != nil {
		return 0, fmt.Errorf("upsert speaker_identity_evidence: %w", err)
	}
	return id, nil
}

type SpeakerReviewTaskParams struct {
	DiarizationJobID int64
	SpeakerClusterID int64
	Priority         int
	CandidateKind    string
	CandidateID      int64
	CandidateLabel   string
	Confidence       float64
	EvidenceIDs      []int64
}

func (s *Store) UpsertSpeakerReviewTask(ctx context.Context, p SpeakerReviewTaskParams) (int64, error) {
	const q = `
INSERT INTO speaker_review_task (diarization_job_id, speaker_cluster_id, priority,
                                 proposed_candidate_kind, proposed_candidate_id,
                                 proposed_label, proposed_confidence, evidence_ids)
VALUES ($1,$2,$3,$4::speaker_candidate_kind,NULLIF($5,0),$6,NULLIF($7,0),$8)
ON CONFLICT (diarization_job_id, speaker_cluster_id, proposed_candidate_kind, COALESCE(proposed_candidate_id, 0), proposed_label)
DO UPDATE SET
  priority = GREATEST(speaker_review_task.priority, EXCLUDED.priority),
  proposed_confidence = GREATEST(COALESCE(speaker_review_task.proposed_confidence,0), COALESCE(EXCLUDED.proposed_confidence,0)),
  evidence_ids = EXCLUDED.evidence_ids,
  updated_at = NOW()
RETURNING id;`
	var id int64
	if err := s.Pool.QueryRow(ctx, q, p.DiarizationJobID, p.SpeakerClusterID, p.Priority,
		p.CandidateKind, p.CandidateID, p.CandidateLabel, p.Confidence, p.EvidenceIDs).Scan(&id); err != nil {
		return 0, fmt.Errorf("upsert speaker_review_task: %w", err)
	}
	return id, nil
}

type SpeakerReviewTask struct {
	ID                  int64
	DiarizationJobID    int64
	TVWEventID          string
	ClusterID           int64
	ClusterLabel        string
	TotalSpeechMS       int
	TurnCount           int
	Status              string
	Priority            int
	CandidateKind       string
	CandidateID         int64
	CandidateLabel      string
	CandidateConfidence float64
	EvidenceIDs         []int64
	EvidenceText        string
	EvidenceStartMS     int
	EvidenceEndMS       int
	SampleSegments      []SpeakerReviewSegment
}

type SpeakerReviewSegment struct {
	StartMS int
	EndMS   int
	Text    string
}

func (s *Store) ListSpeakerReviewTasks(ctx context.Context, status string, limit int) ([]SpeakerReviewTask, error) {
	if status == "" {
		status = "pending"
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	const q = `
SELECT t.id, t.diarization_job_id, sc.tvw_event_id, sc.id, sc.cluster_label,
       COALESCE(sc.total_speech_ms,0), COALESCE(sc.turn_count,0), t.status::text,
       t.priority, t.proposed_candidate_kind::text, COALESCE(t.proposed_candidate_id,0),
       t.proposed_label, COALESCE(t.proposed_confidence,0), t.evidence_ids,
       COALESCE(e.evidence_text,''), COALESCE(e.start_ms,0), COALESCE(e.end_ms,0)
  FROM speaker_review_task t
  JOIN speaker_cluster sc ON sc.id = t.speaker_cluster_id
  LEFT JOIN LATERAL (
    SELECT evidence_text, start_ms, end_ms
      FROM speaker_identity_evidence e
     WHERE e.id = ANY(t.evidence_ids)
     ORDER BY confidence DESC NULLS LAST, id
     LIMIT 1
  ) e ON true
 WHERE t.status = $1::speaker_review_status
 ORDER BY t.priority DESC, t.created_at DESC
 LIMIT $2;`
	rows, err := s.Pool.Query(ctx, q, status, limit)
	if err != nil {
		return nil, fmt.Errorf("list speaker review tasks: %w", err)
	}
	defer rows.Close()
	out := []SpeakerReviewTask{}
	for rows.Next() {
		var t SpeakerReviewTask
		if err := rows.Scan(&t.ID, &t.DiarizationJobID, &t.TVWEventID, &t.ClusterID, &t.ClusterLabel,
			&t.TotalSpeechMS, &t.TurnCount, &t.Status, &t.Priority, &t.CandidateKind,
			&t.CandidateID, &t.CandidateLabel, &t.CandidateConfidence, &t.EvidenceIDs,
			&t.EvidenceText, &t.EvidenceStartMS, &t.EvidenceEndMS); err != nil {
			return nil, fmt.Errorf("scan speaker review task: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) GetSpeakerReviewTask(ctx context.Context, id int64) (SpeakerReviewTask, error) {
	const q = `
SELECT t.id, t.diarization_job_id, sc.tvw_event_id, sc.id, sc.cluster_label,
       COALESCE(sc.total_speech_ms,0), COALESCE(sc.turn_count,0), t.status::text,
       t.priority, t.proposed_candidate_kind::text, COALESCE(t.proposed_candidate_id,0),
       t.proposed_label, COALESCE(t.proposed_confidence,0), t.evidence_ids,
       COALESCE(e.evidence_text,''), COALESCE(e.start_ms,0), COALESCE(e.end_ms,0)
  FROM speaker_review_task t
  JOIN speaker_cluster sc ON sc.id = t.speaker_cluster_id
  LEFT JOIN LATERAL (
    SELECT evidence_text, start_ms, end_ms
      FROM speaker_identity_evidence e
     WHERE e.id = ANY(t.evidence_ids)
     ORDER BY confidence DESC NULLS LAST, id
     LIMIT 1
  ) e ON true
 WHERE t.id = $1;`
	var t SpeakerReviewTask
	if err := s.Pool.QueryRow(ctx, q, id).Scan(&t.ID, &t.DiarizationJobID, &t.TVWEventID, &t.ClusterID, &t.ClusterLabel,
		&t.TotalSpeechMS, &t.TurnCount, &t.Status, &t.Priority, &t.CandidateKind,
		&t.CandidateID, &t.CandidateLabel, &t.CandidateConfidence, &t.EvidenceIDs,
		&t.EvidenceText, &t.EvidenceStartMS, &t.EvidenceEndMS); err != nil {
		return SpeakerReviewTask{}, fmt.Errorf("get speaker review task: %w", err)
	}
	segs, err := s.ListSpeakerReviewTaskSegments(ctx, t.ClusterID, 20)
	if err != nil {
		return SpeakerReviewTask{}, err
	}
	t.SampleSegments = segs
	return t, nil
}

func (s *Store) ListSpeakerReviewTaskSegments(ctx context.Context, clusterID int64, limit int) ([]SpeakerReviewSegment, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	const q = `
SELECT start_ms, end_ms, COALESCE(text,'')
  FROM diarized_speech_segment
 WHERE speaker_cluster_id = $1
 ORDER BY start_ms
 LIMIT $2;`
	rows, err := s.Pool.Query(ctx, q, clusterID, limit)
	if err != nil {
		return nil, fmt.Errorf("list speaker review segments: %w", err)
	}
	defer rows.Close()
	out := []SpeakerReviewSegment{}
	for rows.Next() {
		var seg SpeakerReviewSegment
		if err := rows.Scan(&seg.StartMS, &seg.EndMS, &seg.Text); err != nil {
			return nil, fmt.Errorf("scan speaker review segment: %w", err)
		}
		out = append(out, seg)
	}
	return out, rows.Err()
}

func (s *Store) AcceptSpeakerReviewTask(ctx context.Context, taskID int64, reviewer, notes string) error {
	t, err := s.GetSpeakerReviewTask(ctx, taskID)
	if err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
UPDATE speaker_review_task
   SET status = 'accepted', reviewer = NULLIF($2,''), review_notes = NULLIF($3,''), reviewed_at = NOW(), updated_at = NOW()
 WHERE id = $1;`, taskID, reviewer, notes); err != nil {
		return fmt.Errorf("accept speaker review task: %w", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO speaker_assignment (diarization_job_id, speaker_cluster_id, speaker_kind,
                                speaker_id, speaker_label, confidence, review_task_id, review_status)
VALUES ($1,$2,$3::speaker_candidate_kind,NULLIF($4,0),$5,NULLIF($6,0),$7,'accepted')
ON CONFLICT (diarization_job_id, speaker_cluster_id) DO UPDATE SET
  speaker_kind = EXCLUDED.speaker_kind,
  speaker_id = EXCLUDED.speaker_id,
  speaker_label = EXCLUDED.speaker_label,
  confidence = EXCLUDED.confidence,
  review_task_id = EXCLUDED.review_task_id,
  review_status = 'accepted',
  updated_at = NOW();`, t.DiarizationJobID, t.ClusterID, t.CandidateKind, t.CandidateID, t.CandidateLabel, t.CandidateConfidence, taskID); err != nil {
		return fmt.Errorf("insert speaker assignment: %w", err)
	}
	return tx.Commit(ctx)
}

func (s *Store) RejectSpeakerReviewTask(ctx context.Context, taskID int64, reviewer, notes string) error {
	_, err := s.Pool.Exec(ctx, `
UPDATE speaker_review_task
   SET status = 'rejected', reviewer = NULLIF($2,''), review_notes = NULLIF($3,''), reviewed_at = NOW(), updated_at = NOW()
 WHERE id = $1;`, taskID, reviewer, notes)
	if err != nil {
		return fmt.Errorf("reject speaker review task: %w", err)
	}
	return nil
}

func (s *Store) NeedsMoreEvidenceSpeakerReviewTask(ctx context.Context, taskID int64, reviewer, notes string) error {
	_, err := s.Pool.Exec(ctx, `
UPDATE speaker_review_task
   SET status = 'needs_more_evidence', reviewer = NULLIF($2,''), review_notes = NULLIF($3,''), reviewed_at = NOW(), updated_at = NOW()
 WHERE id = $1;`, taskID, reviewer, notes)
	if err != nil {
		return fmt.Errorf("needs more evidence speaker review task: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// organization source mentions + CSI organization population
// ---------------------------------------------------------------------------

type PopulateOrganizationsStats struct {
	MentionsUpserted      int
	OrganizationsUpserted int
	TestifiersLinked      int64
	Skipped               int
}

// PopulateOrganizationsFromCSI seeds organization rows from distinct
// testifier.raw_organization values, records source mentions, and links all
// matching testifier rows to the resulting organization. It is intentionally
// source-local: no cross-source merge is asserted here.
func (s *Store) PopulateOrganizationsFromCSI(ctx context.Context) (PopulateOrganizationsStats, error) {
	const q = `
SELECT MIN(t.id) AS source_pk,
       trim(t.raw_organization) AS source_name,
       wa_dd_normalize_entity_name(trim(t.raw_organization)) AS normalized_name,
       COUNT(*) AS occurrence_count,
       MIN(t.source_record_id) AS source_record_id
  FROM testifier t
 WHERE NULLIF(trim(t.raw_organization), '') IS NOT NULL
 GROUP BY trim(t.raw_organization), wa_dd_normalize_entity_name(trim(t.raw_organization))
 ORDER BY COUNT(*) DESC, trim(t.raw_organization);`
	rows, err := s.Pool.Query(ctx, q)
	if err != nil {
		return PopulateOrganizationsStats{}, fmt.Errorf("list CSI organizations: %w", err)
	}
	defer rows.Close()

	var stats PopulateOrganizationsStats
	for rows.Next() {
		var sourcePK, count, sourceRecordID int64
		var sourceName, normalized string
		if err := rows.Scan(&sourcePK, &sourceName, &normalized, &count, &sourceRecordID); err != nil {
			return stats, fmt.Errorf("scan CSI organization: %w", err)
		}
		if junkOrganizationName(sourceName, normalized) {
			stats.Skipped++
			continue
		}
		orgID, err := s.UpsertOrganization(ctx, UpsertOrganizationParams{
			CanonicalName:   sourceName,
			Aliases:         []string{sourceName},
			MatchConfidence: "possible",
			MatchNotes:      "Seeded from CSI testimony organization string; source-local identity only.",
		})
		if err != nil {
			return stats, err
		}
		stats.OrganizationsUpserted++
		if err := s.upsertOrganizationSourceMention(ctx, "csi_testifier", "testifier", sourcePK, sourceName, normalized, orgID, int(count), sourceRecordID, "possible"); err != nil {
			return stats, err
		}
		stats.MentionsUpserted++
		linked, err := s.LinkTestifiersToOrg(ctx, orgID, []string{sourceName})
		if err != nil {
			return stats, err
		}
		stats.TestifiersLinked += linked
	}
	if err := rows.Err(); err != nil {
		return stats, err
	}
	return stats, nil
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
	junk := map[string]bool{
		"none": true, "n/a": true, "na": true, "no": true, "self": true,
		"individual": true, "private citizen": true, "citizen": true,
		"homeowner": true, "home owner": true, "resident": true,
		"not applicable": true, "no organization": true,
	}
	return junk[low]
}
