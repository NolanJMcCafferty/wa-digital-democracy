// Package firstpage assembles source-linked page payloads from Postgres state.
package firstpage

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/domain"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

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
	ContextSummary    []string `json:"context_summary,omitempty"`
}

type Source struct {
	System    string    `json:"system"`
	Endpoint  string    `json:"endpoint"`
	URL       string    `json:"url"`
	FetchedAt time.Time `json:"fetched_at"`
}

// BillPage is the page-level response shape for the bill detail route.
// It has bill-level fields plus explicit per-hearing sections.
type BillPage struct {
	GeneratedAt      time.Time        `json:"generated_at"`
	Bill             Bill             `json:"bill"`
	Status           Status           `json:"status"`
	Hearings         []HearingSection `json:"hearings"`
	Sources          []Source         `json:"sources"`
	KnownLimitations []string         `json:"known_limitations,omitempty"`
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
	demo := &domain.BillAgendaTarget{
		Bill: domain.BillKey{Biennium: biennium, Prefix: prefix, Number: number},
	}
	bill, status, err := loadBill(ctx, store, demo)
	if err != nil {
		if errors.Is(err, ErrBillNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("bill: %w", err)
	}

	demos, err := LookupBillAgendaTargetsForBill(ctx, store, biennium, prefix, number)
	if err != nil {
		return nil, fmt.Errorf("hearing lookup: %w", err)
	}
	hearings := make([]HearingSection, 0, len(demos))
	for _, demo := range demos {
		section, err := BuildHearingSection(ctx, store, demo)
		if err != nil {
			return nil, err
		}
		hearings = append(hearings, *section)
	}

	sources, err := loadSourcesForSections(ctx, store, hearings)
	if err != nil {
		return nil, fmt.Errorf("sources: %w", err)
	}
	page := &BillPage{
		GeneratedAt: time.Now().UTC(),
		Bill:        bill,
		Status:      status,
		Hearings:    hearings,
		Sources:     sources,
	}
	page.KnownLimitations = computeBillPageLimitations(page)
	return page, nil
}

// BuildHearingSection returns the page section tied to one agenda_item. The
// hearing-detail handler reuses this for each agenda item rendered under a
// committee hearing.
func BuildHearingSection(ctx context.Context, store *db.Store, demo *domain.BillAgendaTarget) (*HearingSection, error) {
	hearing, err := loadHearingAndAgenda(ctx, store, demo)
	if err != nil {
		return nil, fmt.Errorf("hearing: %w", err)
	}
	testifiers, err := loadTestifiers(ctx, store, demo)
	if err != nil {
		return nil, fmt.Errorf("testifiers: %w", err)
	}
	transcript, err := loadTranscript(ctx, store, demo)
	if err != nil {
		return nil, fmt.Errorf("transcript: %w", err)
	}
	organizations, err := loadOrganizations(ctx, store, demo)
	if err != nil {
		return nil, fmt.Errorf("organizations: %w", err)
	}
	return &HearingSection{
		Hearing:       hearing,
		Testifiers:    testifiers,
		Transcript:    transcript,
		Organizations: organizations,
	}, nil
}

func loadBill(ctx context.Context, store *db.Store, demo *domain.BillAgendaTarget) (Bill, Status, error) {
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
	err := store.Pool.QueryRow(ctx, q, demo.Bill.Biennium, demo.Bill.Prefix, demo.Bill.Number).Scan(
		&billRowID, &biennium, &billNumber,
		&title, &description, &chamberOrigin,
		&currentStatus, &statusDate, &officialURL,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Bill{}, Status{}, fmt.Errorf("%w: %s in %s", ErrBillNotFound, demo.Bill.ID(), demo.Bill.Biennium)
	}
	if err != nil {
		return Bill{}, Status{}, err
	}
	bill := Bill{
		Biennium:      biennium,
		BillID:        billNumber,
		Title:         deref(title),
		Description:   deref(description),
		ChamberOrigin: deref(chamberOrigin),
		OfficialURL:   deref(officialURL),
	}
	status := Status{
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
		return Bill{}, Status{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var s Sponsor
		var ch *string
		var lwsSponsorID string
		if err := rows.Scan(&s.Name, &ch, &s.SponsorType, &lwsSponsorID); err != nil {
			return Bill{}, Status{}, err
		}
		s.Chamber = deref(ch)
		s.PhotoURL, s.ThumbnailURL = legislatorPhotoURLs(lwsSponsorID)
		bill.Sponsors = append(bill.Sponsors, s)
	}
	if err := rows.Err(); err != nil {
		return Bill{}, Status{}, err
	}

	// Status timeline.
	const tlQ = `
SELECT action_date, history_line FROM bill_status_change
 WHERE bill_id = $1 ORDER BY action_date ASC, id ASC;`
	rows2, err := store.Pool.Query(ctx, tlQ, billRowID)
	if err != nil {
		return Bill{}, Status{}, err
	}
	defer rows2.Close()
	for rows2.Next() {
		var e StatusEntry
		if err := rows2.Scan(&e.ActionDate, &e.HistoryLine); err != nil {
			return Bill{}, Status{}, err
		}
		status.Timeline = append(status.Timeline, e)
	}
	return bill, status, rows2.Err()
}

func loadHearingAndAgenda(ctx context.Context, store *db.Store, demo *domain.BillAgendaTarget) (Hearing, error) {
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
	err := store.Pool.QueryRow(ctx, q, demo.AgendaItem.CSIAgendaItemID).Scan(
		&hearingID,
		&commName, &commAcronym, &chamber, &meetingDT,
		&location, &officialAgendaURL, &tvwURL, &tvwEventID,
		&label, &csiAID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Hearing{}, fmt.Errorf("no agenda_item row for csi_agenda_item_id=%s", demo.AgendaItem.CSIAgendaItemID)
	}
	if err != nil {
		return Hearing{}, err
	}
	return Hearing{
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
	}, nil
}

func loadTestifiers(ctx context.Context, store *db.Store, demo *domain.BillAgendaTarget) ([]Testifier, error) {
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
	rows, err := store.Pool.Query(ctx, q, demo.AgendaItem.CSIAgendaItemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Testifier{}
	for rows.Next() {
		var (
			t        Testifier
			rawOrg   *string
			signedAt *time.Time
			orgID    *int64
		)
		if err := rows.Scan(&t.RawName, &rawOrg, &t.Position, &t.Testified, &signedAt, &orgID); err != nil {
			return nil, err
		}
		t.RawOrganization = deref(rawOrg)
		t.TimeSignedIn = signedAt
		t.OrganizationID = orgID
		out = append(out, t)
	}
	return out, rows.Err()
}

func loadTranscript(ctx context.Context, store *db.Store, demo *domain.BillAgendaTarget) (*Transcript, error) {
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
	err := store.Pool.QueryRow(ctx, headQ, demo.AgendaItem.CSIAgendaItemID).Scan(&captionURL, &startMS, &endMS)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil // no transcript yet — render with empty section
	}
	if err != nil {
		return nil, err
	}
	tr := &Transcript{CaptionURL: deref(captionURL)}
	if startMS != nil {
		tr.BillSegmentStart = *startMS
	}
	if endMS != nil {
		tr.BillSegmentEnd = *endMS
	}
	if tr.BillSegmentEnd <= tr.BillSegmentStart {
		return tr, nil
	}
	// Read the windows that SegmentTranscript decided on, rather than
	// re-deriving them in SQL with a different gap threshold. The
	// segmenter's defaultBillSegmentPaddingMS and the in-SQL 120000ms
	// disagreed, which caused window counts to flip in the band where
	// only one of them split a span.
	stored, err := store.ListAgendaItemWindowsByAgendaItem(ctx, demo.AgendaItem.CSIAgendaItemID)
	if err != nil {
		return nil, err
	}
	for _, w := range stored {
		tr.Windows = append(tr.Windows, TranscriptWindow{
			StartMS: w.StartMS,
			EndMS:   w.EndMS,
		})
	}
	if len(tr.Windows) == 0 && startMS != nil && endMS != nil {
		// Fallback for hearings ingested before the persisted-window
		// migration: synthesize a single window from the head bounds.
		tr.Windows = append(tr.Windows, TranscriptWindow{StartMS: *startMS, EndMS: *endMS})
	}

	// Pull segments in all bill discussion windows.
	const segQ = `
SELECT ts.start_ms, ts.end_ms, ts.text, ts.speaker_label, ts.speaker_confidence::text
  FROM transcript_segment ts
  JOIN agenda_item a ON a.id = ts.agenda_item_id
 WHERE a.csi_agenda_item_id = $1
 ORDER BY ts.start_ms ASC;`
	rows, err := store.Pool.Query(ctx, segQ, demo.AgendaItem.CSIAgendaItemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			seg   TranscriptSegment
			label *string
		)
		if err := rows.Scan(&seg.StartMS, &seg.EndMS, &seg.Text, &label, &seg.SpeakerConfidence); err != nil {
			return nil, err
		}
		seg.SpeakerLabel = deref(label)
		tr.Segments = append(tr.Segments, seg)
	}
	return tr, rows.Err()
}

func loadOrganizations(ctx context.Context, store *db.Store, demo *domain.BillAgendaTarget) ([]Organization, error) {
	const q = `
SELECT o.id, o.canonical_name, o.aliases, o.match_confidence::text, o.match_notes,
       MIN(t.position::text) AS pos,
       count(t.id) AS n_testifiers
  FROM organization o
  JOIN testifier t ON t.normalized_org_id = o.id
  JOIN agenda_item a ON a.id = t.agenda_item_id
 WHERE a.csi_agenda_item_id = $1
 GROUP BY o.id;`
	rows, err := store.Pool.Query(ctx, q, demo.AgendaItem.CSIAgendaItemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Organization{}
	for rows.Next() {
		var (
			o     Organization
			id    int64
			notes *string
			pos   *string
			nTest int
		)
		if err := rows.Scan(&id, &o.CanonicalName, &o.Aliases, &o.MatchConfidence, &notes, &pos, &nTest); err != nil {
			return nil, err
		}
		o.MatchNotes = deref(notes)
		o.TestifierPosition = deref(pos)
		o.TestifierCount = nTest
		contexts, err := store.GetOrganizationPublicContexts(ctx, id)
		if err != nil {
			return nil, err
		}
		o.ContextSummary = summarizeOrganizationContexts(contexts)
		out = append(out, o)
	}
	return out, rows.Err()
}

func summarizeOrganizationContexts(contexts []db.OrganizationPublicContext) []string {
	seen := map[string]struct{}{}
	for _, c := range contexts {
		label := ""
		switch c.ContextType {
		case "lobbying_registration":
			label = "Lobbying record"
		case "state_contract":
			label = "Contract record"
		case "state_vendor":
			label = "Vendor record"
		case "state_vendor_payment":
			label = "Payment record"
		case "federal_award":
			label = "Federal award"
		default:
			label = strings.TrimSpace(c.SourceLabel)
		}
		if label != "" {
			seen[label] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for label := range seen {
		out = append(out, label)
	}
	sort.Strings(out)
	return out
}

func loadSourcesForSections(ctx context.Context, store *db.Store, sections []HearingSection) ([]Source, error) {
	csiIDs := make([]string, 0, len(sections))
	for _, section := range sections {
		if section.Hearing.CSIAgendaItemID != "" {
			csiIDs = append(csiIDs, section.Hearing.CSIAgendaItemID)
		}
	}
	return loadSourcesForAgendaItems(ctx, store, csiIDs)
}

// loadSourcesForAgendaItems surfaces every distinct source_record touched by
// the rendered agenda items (joined via bill, hearing, agenda_item, testifier,
// tvw_event, transcript_segment, and bill_status_change).
//
// Per the wiki: "every public fact needs provenance" — the source panel
// is part of the product, not engineering metadata.
func loadSourcesForAgendaItems(ctx context.Context, store *db.Store, csiIDs []string) ([]Source, error) {
	if len(csiIDs) == 0 {
		return []Source{}, nil
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
		return nil, err
	}
	defer rows.Close()
	out := []Source{}
	for rows.Next() {
		var s Source
		if err := rows.Scan(&s.System, &s.Endpoint, &s.URL, &s.FetchedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func computeBillPageLimitations(page *BillPage) []string {
	var out []string
	if len(page.Bill.Sponsors) == 0 {
		out = append(out, "Sponsors not yet ingested.")
	}
	if len(page.Status.Timeline) == 0 {
		out = append(out, "Status timeline not available — LWS may have returned no history changes.")
	}
	if len(page.Hearings) == 0 {
		out = append(out, "No hearing has been ingested for this bill yet.")
		return out
	}
	hasTVW := false
	hasTranscriptSegments := false
	hasOrganizations := false
	for _, section := range page.Hearings {
		if section.Hearing.TVWEventID != "" {
			hasTVW = true
		}
		if section.Transcript != nil && len(section.Transcript.Segments) > 0 {
			hasTranscriptSegments = true
		}
		if len(section.Organizations) > 0 {
			hasOrganizations = true
		}
	}
	if !hasTVW {
		out = append(out, "TVW event not linked to any ingested hearing.")
	}
	if !hasTranscriptSegments {
		out = append(out, "No transcript segments matched this bill discussion (manual transcript_override may help).")
	}
	if !hasOrganizations {
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
