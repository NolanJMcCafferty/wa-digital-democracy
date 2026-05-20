// Package firstpage assembles bill page payloads and legacy first-page
// snapshot JSON from Postgres state. New live API routes should expose
// explicit page objects; Bundle remains for the curated snapshot path.
package firstpage

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/config"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

// Bundle is the JSON payload Phase 5 renders.
//
// Hearing-dependent sections (Hearing, Testifiers, Transcript, Organizations)
// are pointer/optional because metadata-only ingest (`wa-dd ingest-session`)
// produces bills without any agenda_item rows. The frontend hides those
// sections when absent.
type Bundle struct {
	GeneratedAt time.Time `json:"generated_at"`
	Bill        Bill      `json:"bill"`
	Status      Status    `json:"status"`
	// Hearing/Testifiers/Transcript/Organizations mirror the most recent
	// hearing for back-compat with older clients. The full list of hearings
	// on the bill lives in Hearings; the bill-detail page renders one section
	// per entry.
	Hearing          *Hearing         `json:"hearing,omitempty"`
	Testifiers       []Testifier      `json:"testifiers"`
	Transcript       *Transcript      `json:"transcript,omitempty"`
	Organizations    []Organization   `json:"organizations"`
	Hearings         []HearingSection `json:"hearings,omitempty"`
	Sources          []Source         `json:"sources"`
	KnownLimitations []string         `json:"known_limitations,omitempty"`
}

// HearingSection groups everything tied to a single agenda_item: the
// hearing metadata plus the testifiers, transcript, and organizations
// scoped to it. The bill-detail page uses one of these per hearing on
// the bill.
type HearingSection struct {
	Hearing       Hearing        `json:"hearing"`
	Testifiers    []Testifier    `json:"testifiers"`
	Transcript    *Transcript    `json:"transcript,omitempty"`
	Organizations []Organization `json:"organizations"`
}

type Bill struct {
	Biennium      string    `json:"biennium"`
	BillID        string    `json:"bill_id"` // "HB 1234"
	Title         string    `json:"title,omitempty"`
	Description   string    `json:"description,omitempty"`
	ChamberOrigin string    `json:"chamber_origin,omitempty"`
	OfficialURL   string    `json:"official_url,omitempty"`
	Sponsors      []Sponsor `json:"sponsors,omitempty"`
}

type Sponsor struct {
	Name         string `json:"name"`
	Chamber      string `json:"chamber,omitempty"`
	SponsorType  string `json:"sponsor_type,omitempty"`
	PhotoURL     string `json:"photo_url,omitempty"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
}

type Status struct {
	Current    string        `json:"current,omitempty"`
	StatusDate *time.Time    `json:"status_date,omitempty"`
	Timeline   []StatusEntry `json:"timeline,omitempty"`
}

type StatusEntry struct {
	ActionDate  time.Time `json:"action_date"`
	HistoryLine string    `json:"history_line"`
}

type Hearing struct {
	HearingID         int64     `json:"hearing_id,omitempty"`
	CommitteeName     string    `json:"committee_name"`
	CommitteeAcronym  string    `json:"committee_acronym,omitempty"`
	Chamber           string    `json:"chamber"`
	MeetingDateTime   time.Time `json:"meeting_datetime"`
	Location          string    `json:"location,omitempty"`
	OfficialAgendaURL string    `json:"official_agenda_url,omitempty"`
	TVWURL            string    `json:"tvw_url,omitempty"`
	TVWEventID        string    `json:"tvw_event_id,omitempty"`
	AgendaItemLabel   string    `json:"agenda_item_label,omitempty"`
	CSIAgendaItemID   string    `json:"csi_agenda_item_id,omitempty"`
}

type Testifier struct {
	RawName         string     `json:"raw_name"`
	RawOrganization string     `json:"raw_organization,omitempty"`
	Position        string     `json:"position"`
	Testified       bool       `json:"testified"`
	TimeSignedIn    *time.Time `json:"time_signed_in,omitempty"`
	OrganizationID  *int64     `json:"organization_id,omitempty"`
}

type Transcript struct {
	CaptionURL       string              `json:"caption_url,omitempty"`
	BillSegmentStart int                 `json:"bill_segment_start_ms,omitempty"`
	BillSegmentEnd   int                 `json:"bill_segment_end_ms,omitempty"`
	Windows          []TranscriptWindow  `json:"windows,omitempty"`
	Segments         []TranscriptSegment `json:"segments,omitempty"`
}

type TranscriptWindow struct {
	StartMS int `json:"start_ms"`
	EndMS   int `json:"end_ms"`
}

type TranscriptSegment struct {
	StartMS           int    `json:"start_ms"`
	EndMS             int    `json:"end_ms"`
	Text              string `json:"text"`
	SpeakerLabel      string `json:"speaker_label,omitempty"`
	SpeakerConfidence string `json:"speaker_confidence"`
}

type Organization struct {
	CanonicalName     string   `json:"canonical_name"`
	Aliases           []string `json:"aliases,omitempty"`
	MatchConfidence   string   `json:"match_confidence"`
	MatchNotes        string   `json:"match_notes,omitempty"`
	TestifierPosition string   `json:"testifier_position,omitempty"`
	TestifierCount    int      `json:"testifier_count,omitempty"`
}

type Source struct {
	System    string    `json:"system"`
	Endpoint  string    `json:"endpoint"`
	URL       string    `json:"url"`
	FetchedAt time.Time `json:"fetched_at"`
}

// BillPage is the page-level response shape for the bill detail route.
// Unlike Bundle, it does not carry legacy top-level hearing mirrors; the
// page has bill-level fields plus explicit per-hearing sections.
type BillPage struct {
	GeneratedAt      time.Time        `json:"generated_at"`
	Bill             Bill             `json:"bill"`
	Status           Status           `json:"status"`
	Hearings         []HearingSection `json:"hearings"`
	Sources          []Source         `json:"sources"`
	KnownLimitations []string         `json:"known_limitations,omitempty"`
}

// Build assembles a legacy snapshot payload for the configured demo from Postgres state.
// Requires a CSI agenda item id on the demo — this path is for the curated
// hearing-bound view. For metadata-only bills (no agenda item), call
// BuildByBill instead.
func Build(ctx context.Context, store *db.Store, demo *config.SelectedDemo) (*Bundle, error) {
	// Initialize collection fields to non-nil empty slices so JSON serializes
	// to [] rather than null when a section has no data —
	// the frontend treats these as arrays unconditionally.
	b := &Bundle{
		GeneratedAt:   time.Now().UTC(),
		Testifiers:    []Testifier{},
		Organizations: []Organization{},
		Sources:       []Source{},
	}

	if err := loadBill(ctx, store, demo, b); err != nil {
		return nil, fmt.Errorf("bill: %w", err)
	}
	if err := loadHearingAndAgenda(ctx, store, demo, b); err != nil {
		return nil, fmt.Errorf("hearing: %w", err)
	}
	if err := loadTestifiers(ctx, store, demo, b); err != nil {
		return nil, fmt.Errorf("testifiers: %w", err)
	}
	if err := loadTranscript(ctx, store, demo, b); err != nil {
		return nil, fmt.Errorf("transcript: %w", err)
	}
	if err := loadOrganizations(ctx, store, demo, b); err != nil {
		return nil, fmt.Errorf("organizations: %w", err)
	}
	if err := loadSources(ctx, store, b); err != nil {
		return nil, fmt.Errorf("sources: %w", err)
	}

	b.KnownLimitations = computeLimitations(b)
	return b, nil
}

// BuildByBill assembles the legacy snapshot shape for callers that still need
// it. New bill-detail HTTP callers should use BuildBillPage.
func BuildByBill(
	ctx context.Context,
	store *db.Store,
	biennium, prefix string,
	number int,
) (*Bundle, error) {
	page, err := BuildBillPage(ctx, store, biennium, prefix, number)
	if err != nil {
		return nil, err
	}
	b := &Bundle{
		GeneratedAt:      page.GeneratedAt,
		Bill:             page.Bill,
		Status:           page.Status,
		Testifiers:       []Testifier{},
		Organizations:    []Organization{},
		Hearings:         page.Hearings,
		Sources:          page.Sources,
		KnownLimitations: page.KnownLimitations,
	}
	if len(page.Hearings) > 0 {
		first := page.Hearings[0]
		hearingCopy := first.Hearing
		b.Hearing = &hearingCopy
		b.Testifiers = first.Testifiers
		b.Transcript = first.Transcript
		b.Organizations = first.Organizations
	}
	return b, nil
}

// BuildBillPage assembles the page-level bill detail response for any bill row
// in Postgres. Metadata-only bills return snapshot/status/sources with an empty
// hearings array; enriched bills return one HearingSection per agenda item.
func BuildBillPage(
	ctx context.Context,
	store *db.Store,
	biennium, prefix string,
	number int,
) (*BillPage, error) {
	b := &Bundle{
		GeneratedAt:   time.Now().UTC(),
		Testifiers:    []Testifier{},
		Organizations: []Organization{},
		Sources:       []Source{},
	}

	demo := &config.SelectedDemo{
		Biennium:   biennium,
		BillPrefix: prefix,
		BillNumber: number,
	}
	if err := loadBill(ctx, store, demo, b); err != nil {
		if errors.Is(err, ErrBillNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("bill: %w", err)
	}

	demos, err := LookupAllSelectedDemos(ctx, store, biennium, prefix, number)
	if err != nil {
		return nil, fmt.Errorf("hearing lookup: %w", err)
	}
	for _, demo := range demos {
		section, err := BuildHearingSection(ctx, store, demo)
		if err != nil {
			return nil, err
		}
		b.Hearings = append(b.Hearings, *section)
	}

	if len(b.Hearings) > 0 {
		// Populate the legacy mirror fields so the existing source/limitation
		// helpers can run unchanged while BillPage stays clean at the boundary.
		first := b.Hearings[0]
		hearingCopy := first.Hearing
		b.Hearing = &hearingCopy
		b.Testifiers = first.Testifiers
		b.Transcript = first.Transcript
		b.Organizations = first.Organizations
	}
	if err := loadSources(ctx, store, b); err != nil {
		return nil, fmt.Errorf("sources: %w", err)
	}
	b.KnownLimitations = computeLimitations(b)

	return &BillPage{
		GeneratedAt:      b.GeneratedAt,
		Bill:             b.Bill,
		Status:           b.Status,
		Hearings:         b.Hearings,
		Sources:          b.Sources,
		KnownLimitations: b.KnownLimitations,
	}, nil
}

// BuildHearingSection runs the per-hearing loaders against a scratch
// scratch payload and pulls the resulting fields into a HearingSection. Exported
// so the hearing-detail handler can reuse it for the per-agenda-item
// blocks rendered on /hearings/{id}.
func BuildHearingSection(ctx context.Context, store *db.Store, demo *config.SelectedDemo) (*HearingSection, error) {
	scratch := &Bundle{
		Testifiers:    []Testifier{},
		Organizations: []Organization{},
	}
	if err := loadHearingAndAgenda(ctx, store, demo, scratch); err != nil {
		return nil, fmt.Errorf("hearing: %w", err)
	}
	if err := loadTestifiers(ctx, store, demo, scratch); err != nil {
		return nil, fmt.Errorf("testifiers: %w", err)
	}
	if err := loadTranscript(ctx, store, demo, scratch); err != nil {
		return nil, fmt.Errorf("transcript: %w", err)
	}
	if err := loadOrganizations(ctx, store, demo, scratch); err != nil {
		return nil, fmt.Errorf("organizations: %w", err)
	}
	section := &HearingSection{
		Testifiers:    scratch.Testifiers,
		Transcript:    scratch.Transcript,
		Organizations: scratch.Organizations,
	}
	if scratch.Hearing != nil {
		section.Hearing = *scratch.Hearing
	}
	return section, nil
}

func loadBill(ctx context.Context, store *db.Store, demo *config.SelectedDemo, b *Bundle) error {
	const q = `
SELECT id, biennium, bill_number, title, description, chamber_origin,
       current_status, status_date, official_url
  FROM bill
 WHERE biennium = $1 AND prefix = $2 AND number = $3;`
	var (
		billRowID                         int64
		biennium, billNumber              string
		title, description, chamberOrigin *string
		currentStatus                     *string
		statusDate                        *time.Time
		officialURL                       *string
	)
	err := store.Pool.QueryRow(ctx, q, demo.Biennium, demo.BillPrefix, demo.BillNumber).Scan(
		&billRowID, &biennium, &billNumber,
		&title, &description, &chamberOrigin,
		&currentStatus, &statusDate, &officialURL,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %s in %s", ErrBillNotFound, demo.BillID(), demo.Biennium)
	}
	if err != nil {
		return err
	}
	b.Bill = Bill{
		Biennium:      biennium,
		BillID:        billNumber,
		Title:         deref(title),
		Description:   deref(description),
		ChamberOrigin: deref(chamberOrigin),
		OfficialURL:   deref(officialURL),
	}
	b.Status = Status{
		Current:    deref(currentStatus),
		StatusDate: statusDate,
	}

	// Sponsors.
	const sponsorQ = `
SELECT l.name, l.chamber, bs.sponsor_type, COALESCE(l.lws_sponsor_id, '')
  FROM bill_sponsor bs
  JOIN legislator l ON l.id = bs.legislator_id
 WHERE bs.bill_id = $1
 ORDER BY CASE bs.sponsor_type WHEN 'Primary' THEN 0 ELSE 1 END, l.name;`
	rows, err := store.Pool.Query(ctx, sponsorQ, billRowID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var s Sponsor
		var ch *string
		var lwsSponsorID string
		if err := rows.Scan(&s.Name, &ch, &s.SponsorType, &lwsSponsorID); err != nil {
			return err
		}
		s.Chamber = deref(ch)
		s.PhotoURL, s.ThumbnailURL = legislatorPhotoURLs(lwsSponsorID)
		b.Bill.Sponsors = append(b.Bill.Sponsors, s)
	}

	// Status timeline.
	const tlQ = `
SELECT action_date, history_line FROM bill_status_change
 WHERE bill_id = $1 ORDER BY action_date ASC, id ASC;`
	rows2, err := store.Pool.Query(ctx, tlQ, billRowID)
	if err != nil {
		return err
	}
	defer rows2.Close()
	for rows2.Next() {
		var e StatusEntry
		if err := rows2.Scan(&e.ActionDate, &e.HistoryLine); err != nil {
			return err
		}
		b.Status.Timeline = append(b.Status.Timeline, e)
	}
	return rows2.Err()
}

func loadHearingAndAgenda(ctx context.Context, store *db.Store, demo *config.SelectedDemo, b *Bundle) error {
	const q = `
SELECT h.id, h.committee_name, h.committee_acronym, h.chamber, h.meeting_datetime,
       h.location, h.official_agenda_url, h.tvw_url, h.tvw_event_id,
       a.label, a.csi_agenda_item_id
  FROM agenda_item a
  JOIN hearing h ON h.id = a.hearing_id
 WHERE a.csi_agenda_item_id = $1
 LIMIT 1;`
	var (
		hearingID                                                         int64
		commName, chamber, label, csiAID                                  string
		commAcronym, location, officialAgendaURL, tvwURL, tvwEventID, tmp *string
	)
	_ = tmp
	var meetingDT time.Time
	err := store.Pool.QueryRow(ctx, q, demo.Agenda.CSIAgendaItemID).Scan(
		&hearingID,
		&commName, &commAcronym, &chamber, &meetingDT,
		&location, &officialAgendaURL, &tvwURL, &tvwEventID,
		&label, &csiAID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("no agenda_item row for csi_agenda_item_id=%s", demo.Agenda.CSIAgendaItemID)
	}
	if err != nil {
		return err
	}
	b.Hearing = &Hearing{
		HearingID:         hearingID,
		CommitteeName:     commName,
		CommitteeAcronym:  deref(commAcronym),
		Chamber:           chamber,
		MeetingDateTime:   meetingDT.UTC(),
		Location:          deref(location),
		OfficialAgendaURL: deref(officialAgendaURL),
		TVWURL:            deref(tvwURL),
		TVWEventID:        deref(tvwEventID),
		AgendaItemLabel:   label,
		CSIAgendaItemID:   csiAID,
	}
	return nil
}

func loadTestifiers(ctx context.Context, store *db.Store, demo *config.SelectedDemo, b *Bundle) error {
	const q = `
SELECT t.raw_name, t.raw_organization, t.position, t.testified,
       t.time_signed_in, t.normalized_org_id
  FROM testifier t
  JOIN agenda_item a ON a.id = t.agenda_item_id
 WHERE a.csi_agenda_item_id = $1
 ORDER BY
   CASE t.position::text WHEN 'Pro' THEN 0 WHEN 'Con' THEN 1 WHEN 'Other' THEN 2 ELSE 3 END,
   t.raw_organization NULLS LAST,
   t.raw_name;`
	rows, err := store.Pool.Query(ctx, q, demo.Agenda.CSIAgendaItemID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			t        Testifier
			rawOrg   *string
			signedAt *time.Time
			orgID    *int64
		)
		if err := rows.Scan(&t.RawName, &rawOrg, &t.Position, &t.Testified, &signedAt, &orgID); err != nil {
			return err
		}
		t.RawOrganization = deref(rawOrg)
		t.TimeSignedIn = signedAt
		t.OrganizationID = orgID
		b.Testifiers = append(b.Testifiers, t)
	}
	return rows.Err()
}

func loadTranscript(ctx context.Context, store *db.Store, demo *config.SelectedDemo, b *Bundle) error {
	// Caption URL + agenda-bound segment range.
	const headQ = `
SELECT te.caption_url,
       MIN(ts.start_ms) FILTER (WHERE ts.agenda_item_id = a.id),
       MAX(ts.end_ms)   FILTER (WHERE ts.agenda_item_id = a.id)
  FROM agenda_item a
  JOIN hearing h ON h.id = a.hearing_id
  LEFT JOIN tvw_event te ON te.tvw_event_id = h.tvw_event_id
  LEFT JOIN transcript_segment ts ON ts.tvw_event_id = h.tvw_event_id
 WHERE a.csi_agenda_item_id = $1
 GROUP BY te.caption_url, a.id;`
	var (
		captionURL *string
		startMS    *int
		endMS      *int
	)
	err := store.Pool.QueryRow(ctx, headQ, demo.Agenda.CSIAgendaItemID).Scan(&captionURL, &startMS, &endMS)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // no transcript yet — render with empty section
	}
	if err != nil {
		return err
	}
	b.Transcript = &Transcript{}
	b.Transcript.CaptionURL = deref(captionURL)
	if startMS != nil {
		b.Transcript.BillSegmentStart = *startMS
	}
	if endMS != nil {
		b.Transcript.BillSegmentEnd = *endMS
	}
	if b.Transcript.BillSegmentEnd <= b.Transcript.BillSegmentStart {
		return nil
	}
	// Read the windows that SegmentTranscript decided on, rather than
	// re-deriving them in SQL with a different gap threshold. The
	// segmenter's defaultBillSegmentPaddingMS and the in-SQL 120000ms
	// disagreed, which caused window counts to flip in the band where
	// only one of them split a span.
	stored, err := store.ListAgendaItemWindowsByAgendaItem(ctx, demo.Agenda.CSIAgendaItemID)
	if err != nil {
		return err
	}
	for _, w := range stored {
		b.Transcript.Windows = append(b.Transcript.Windows, TranscriptWindow{
			StartMS: w.StartMS,
			EndMS:   w.EndMS,
		})
	}
	if len(b.Transcript.Windows) == 0 && startMS != nil && endMS != nil {
		// Fallback for hearings ingested before the persisted-window
		// migration: synthesize a single window from the head bounds.
		b.Transcript.Windows = append(b.Transcript.Windows, TranscriptWindow{StartMS: *startMS, EndMS: *endMS})
	}

	// Pull segments in all bill discussion windows.
	const segQ = `
SELECT ts.start_ms, ts.end_ms, ts.text, ts.speaker_label, ts.speaker_confidence::text
  FROM transcript_segment ts
  JOIN agenda_item a ON a.id = ts.agenda_item_id
 WHERE a.csi_agenda_item_id = $1
 ORDER BY ts.start_ms ASC;`
	rows, err := store.Pool.Query(ctx, segQ, demo.Agenda.CSIAgendaItemID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			s     TranscriptSegment
			label *string
		)
		if err := rows.Scan(&s.StartMS, &s.EndMS, &s.Text, &label, &s.SpeakerConfidence); err != nil {
			return err
		}
		s.SpeakerLabel = deref(label)
		b.Transcript.Segments = append(b.Transcript.Segments, s)
	}
	return rows.Err()
}

func loadOrganizations(ctx context.Context, store *db.Store, demo *config.SelectedDemo, b *Bundle) error {
	const q = `
SELECT o.id, o.canonical_name, o.aliases, o.match_confidence::text, o.match_notes,
       MIN(t.position::text) AS pos,
       count(t.id) AS n_testifiers
  FROM organization o
  JOIN testifier t ON t.normalized_org_id = o.id
  JOIN agenda_item a ON a.id = t.agenda_item_id
 WHERE a.csi_agenda_item_id = $1
 GROUP BY o.id;`
	rows, err := store.Pool.Query(ctx, q, demo.Agenda.CSIAgendaItemID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			o     Organization
			id    int64
			notes *string
			pos   *string
			nTest int
		)
		if err := rows.Scan(&id, &o.CanonicalName, &o.Aliases, &o.MatchConfidence, &notes, &pos, &nTest); err != nil {
			return err
		}
		o.MatchNotes = deref(notes)
		o.TestifierPosition = deref(pos)
		o.TestifierCount = nTest
		b.Organizations = append(b.Organizations, o)
	}
	return rows.Err()
}

// loadSources surfaces every distinct source_record we touched while
// building this page (joined via the ids attached to bill, hearing,
// agenda_item, testifier, tvw_event, and transcript_segment).
//
// Per the wiki: "every public fact needs provenance" — the source panel
// is part of the product, not engineering metadata.
func loadSources(ctx context.Context, store *db.Store, b *Bundle) error {
	// Collect every CSI agenda item id we're rendering on this page so the
	// source panel reflects provenance for all hearings, not just the most
	// recent one.
	var csiIDs []string
	if len(b.Hearings) > 0 {
		for _, h := range b.Hearings {
			if h.Hearing.CSIAgendaItemID != "" {
				csiIDs = append(csiIDs, h.Hearing.CSIAgendaItemID)
			}
		}
	} else if b.Hearing != nil && b.Hearing.CSIAgendaItemID != "" {
		csiIDs = append(csiIDs, b.Hearing.CSIAgendaItemID)
	}
	if len(csiIDs) == 0 {
		return nil
	}
	const q = `
SELECT DISTINCT sr.source_system, sr.source_endpoint, sr.source_url, sr.fetched_at
  FROM source_record sr
 WHERE sr.id IN (
   SELECT source_record_id FROM bill                WHERE id IN (SELECT bill_id FROM agenda_item WHERE csi_agenda_item_id = ANY($1))
   UNION SELECT source_record_id FROM hearing       WHERE id IN (SELECT hearing_id FROM agenda_item WHERE csi_agenda_item_id = ANY($1))
   UNION SELECT source_record_id FROM agenda_item   WHERE csi_agenda_item_id = ANY($1)
   UNION SELECT source_record_id FROM testifier     WHERE agenda_item_id IN (SELECT id FROM agenda_item WHERE csi_agenda_item_id = ANY($1))
   UNION SELECT source_record_id FROM tvw_event     WHERE tvw_event_id IN (SELECT tvw_event_id FROM hearing WHERE id IN (SELECT hearing_id FROM agenda_item WHERE csi_agenda_item_id = ANY($1)))
   UNION SELECT source_record_id FROM transcript_segment WHERE tvw_event_id IN (SELECT tvw_event_id FROM hearing WHERE id IN (SELECT hearing_id FROM agenda_item WHERE csi_agenda_item_id = ANY($1)))
   UNION SELECT source_record_id FROM bill_status_change WHERE bill_id IN (SELECT bill_id FROM agenda_item WHERE csi_agenda_item_id = ANY($1))
 )
 ORDER BY sr.fetched_at DESC;`
	rows, err := store.Pool.Query(ctx, q, csiIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var s Source
		if err := rows.Scan(&s.System, &s.Endpoint, &s.URL, &s.FetchedAt); err != nil {
			return err
		}
		b.Sources = append(b.Sources, s)
	}
	return rows.Err()
}

func computeLimitations(b *Bundle) []string {
	var out []string
	if len(b.Bill.Sponsors) == 0 {
		out = append(out, "Sponsors not yet ingested.")
	}
	if len(b.Status.Timeline) == 0 {
		out = append(out, "Status timeline not available — LWS may have returned no history changes.")
	}
	if b.Hearing == nil {
		out = append(out, "No hearing has been ingested for this bill yet.")
		return out
	}
	if b.Hearing.TVWEventID == "" {
		out = append(out, "TVW event not linked to this hearing.")
	}
	if b.Transcript == nil || len(b.Transcript.Segments) == 0 {
		if b.Transcript == nil || b.Transcript.CaptionURL == "" {
			out = append(out, "No captions available for this hearing's TVW event.")
		} else {
			out = append(out, "No transcript segments matched this bill discussion (manual transcript_override may help).")
		}
	}
	if len(b.Organizations) == 0 {
		out = append(out, "Organization context section is empty — run populate-organizations and source-specific entity matching to surface reviewed organization records.")
	}
	return out
}

func legislatorPhotoURLs(lwsSponsorID string) (string, string) {
	id := strings.TrimSpace(lwsSponsorID)
	if id == "" {
		return "", ""
	}
	escaped := url.PathEscape(id)
	return "https://leg.wa.gov/memberphoto/" + escaped + ".jpg",
		"https://leg.wa.gov/memberthumbnail/" + escaped + ".jpg"
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
