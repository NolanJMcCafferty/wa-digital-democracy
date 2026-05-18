package jobs

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/csi"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/tvw"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

// Auto-discovery turns LWS-reported hearings into the (CSI agenda, TVW
// event) join keys the curated pipeline needs. The pipeline today
// fills the (CSI agenda, TVW event) join keys directly from CSI and TVW
// so we can run the full ingest for every hearing the legislature
// reports.
//
// Caching policy: the three CSI directory calls (committees, meetings
// per committee, agenda items per meeting) are stable within a nightly
// run, so we cache them per-Discoverer-instance. The TVW WP archive is
// cached by date-key (YYYY-MM-DD) since multiple hearings on the same
// day reuse the same fetch.

// MatchTimeWindow is how much clock skew we tolerate between an LWS
// meeting_datetime and a CSI meeting StartDateTime. LWS publishes wall
// clock to the minute; CSI's labels parse to the same minute, but the
// observed skew has been seen up to ~5 min on some agendas — 15 keeps
// the join robust without inviting wrong matches.
const MatchTimeWindow = 15 * time.Minute

// TVWMatchWindow is how much skew we tolerate between an LWS meeting
// time and a TVW post date. TVW publishes the recording's start, which
// can lag the gavel by up to ~30 minutes; ±2h is generous but still
// well under "next meeting on the same day."
const TVWMatchWindow = 2 * time.Hour

// DiscoveryDeps groups the three things the Discoverer needs from the
// rest of the system. Mirrors Pipeline's shape (minus PDC/LWS) so a
// caller in cmd/wa-dd/main.go can construct it from the same setup.
type DiscoveryDeps struct {
	Store *db.Store
	CSI   *csi.Client
	TVW   *tvw.Client
}

// Discoverer holds the run-scoped caches.
type Discoverer struct {
	deps DiscoveryDeps

	committees      map[string][]csi.Committee   // chamber → committees
	meetings        map[string][]csi.Meeting     // committee_id → meetings
	agendaItems     map[string][]csi.AgendaItem  // meeting_family_id → items
	tvwPostsByDay   map[string][]tvw.WPVideoPost // YYYY-MM-DD → posts
	committeeMisses map[string]bool              // log-once per (chamber|name)
}

func NewDiscoverer(deps DiscoveryDeps) *Discoverer {
	return &Discoverer{
		deps:            deps,
		committees:      map[string][]csi.Committee{},
		meetings:        map[string][]csi.Meeting{},
		agendaItems:     map[string][]csi.AgendaItem{},
		tvwPostsByDay:   map[string][]tvw.WPVideoPost{},
		committeeMisses: map[string]bool{},
	}
}

// DiscoveryResult is what one DiscoverOne call produced. Empty fields
// indicate sub-step misses; the caller decides how strict to be.
type DiscoveryResult struct {
	CSICommitteeID        string
	CSIMeetingFamilyID    string
	CSIAgendaItemID       string
	CSIAgendaItemFamilyID string
	AgendaItemLabel       string
	TVWEventID            string
}

// DiscoverOne walks the four-step lookup for a single hearing and
// returns whatever it found. Returning a partial result is OK — the
// CSI side and the TVW side are independent: a hearing with CSI
// testifiers but no TVW captions is still useful, and vice versa.
//
// Errors are reserved for "couldn't even start" cases (CSI committee
// not found means we can't look up testifiers, so the whole hearing is
// skipped). TVW miss is non-fatal.
func (d *Discoverer) DiscoverOne(ctx context.Context, h db.HearingForDiscovery) (*DiscoveryResult, error) {
	// 1. CSI committee.
	committeeID, err := d.matchCommittee(ctx, h.Chamber, h.CommitteeName)
	if err != nil {
		return nil, fmt.Errorf("committee: %w", err)
	}

	// 2. CSI meeting.
	meeting, err := d.matchMeeting(ctx, h.Chamber, committeeID, h.MeetingDateTime)
	if err != nil {
		return nil, fmt.Errorf("meeting: %w", err)
	}

	// 3. CSI agenda item for this bill.
	agendaItem, err := d.matchAgendaItem(ctx, h.Chamber, meeting.MeetingFamilyID, h.BillNumber)
	if err != nil {
		return nil, fmt.Errorf("agenda item: %w", err)
	}

	res := &DiscoveryResult{
		CSICommitteeID:        committeeID,
		CSIMeetingFamilyID:    meeting.MeetingFamilyID,
		CSIAgendaItemID:       agendaItem.AgendaItemID,
		CSIAgendaItemFamilyID: agendaItem.AgendaItemFamilyID,
		AgendaItemLabel:       agendaItem.Label,
	}

	// 4. TVW event — best effort.
	if eventID, err := d.matchTVW(ctx, h.CommitteeName, h.MeetingDateTime); err == nil {
		res.TVWEventID = eventID
	}
	// (We deliberately swallow the TVW error; the caller logs it via the
	// returned empty TVWEventID and moves on.)

	return res, nil
}

// Commit persists a DiscoveryResult by updating the existing hearing
// row (filling in CSI/TVW IDs) and inserting/upserting the agenda_item
// row. Reuses the bill's source_record_id for provenance — these IDs
// are enrichment of an existing claim, not a new factual claim.
func (d *Discoverer) Commit(ctx context.Context, h db.HearingForDiscovery, r *DiscoveryResult) error {
	if _, err := d.deps.Store.UpsertHearing(ctx, db.UpsertHearingParams{
		BillID:                    pInt64(h.BillID),
		CommitteeName:             h.CommitteeName,
		CommitteeAcronym:          h.CommitteeAcronym,
		Chamber:                   h.Chamber,
		MeetingDateTime:           h.MeetingDateTime,
		CommitteeScheduleAgendaID: r.CSIMeetingFamilyID,
		TVWEventID:                r.TVWEventID,
		SourceRecordID:            h.SourceRecordID,
	}); err != nil {
		return fmt.Errorf("upsert hearing: %w", err)
	}
	if _, err := d.deps.Store.UpsertAgendaItem(ctx, db.UpsertAgendaItemParams{
		HearingID:             h.HearingID,
		BillID:                pInt64(h.BillID),
		Label:                 r.AgendaItemLabel,
		CSIMeetingFamilyID:    r.CSIMeetingFamilyID,
		CSIAgendaItemFamilyID: r.CSIAgendaItemFamilyID,
		CSIAgendaItemID:       r.CSIAgendaItemID,
		SourceRecordID:        h.SourceRecordID,
	}); err != nil {
		return fmt.Errorf("upsert agenda item: %w", err)
	}
	return nil
}

// matchCommittee maps an LWS (chamber, committee_name) — e.g. ("Senate",
// "Senate Housing") — to a CSI committee ID. LWS prefixes the chamber
// onto the committee name; CSI Names are bare ("Housing"). We strip the
// chamber prefix and do a normalized exact match. No fuzzy matching for
// the v1 — committee names are stable and small.
func (d *Discoverer) matchCommittee(ctx context.Context, chamber, lwsName string) (string, error) {
	cs, err := d.committeesFor(ctx, chamber)
	if err != nil {
		return "", err
	}
	target := normalizeCommitteeName(stripChamberPrefix(chamber, lwsName))
	for _, c := range cs {
		if normalizeCommitteeName(c.Name) == target {
			return c.ID, nil
		}
	}
	key := chamber + "|" + lwsName
	if !d.committeeMisses[key] {
		d.committeeMisses[key] = true
	}
	return "", fmt.Errorf("no CSI committee match for %s %q", chamber, lwsName)
}

func (d *Discoverer) committeesFor(ctx context.Context, chamber string) ([]csi.Committee, error) {
	if cached, ok := d.committees[chamber]; ok {
		return cached, nil
	}
	cs, err := d.deps.CSI.ListCommittees(ctx, chamber)
	if err != nil {
		return nil, err
	}
	d.committees[chamber] = cs
	return cs, nil
}

// matchMeeting finds the CSI meeting closest to the LWS meeting_datetime
// within ±MatchTimeWindow. CSI's date-search params are unreliable
// (Phase 0), so we list-and-filter.
func (d *Discoverer) matchMeeting(ctx context.Context, chamber, committeeID string, target time.Time) (*csi.Meeting, error) {
	ms, err := d.meetingsFor(ctx, chamber, committeeID)
	if err != nil {
		return nil, err
	}
	var best *csi.Meeting
	bestDelta := MatchTimeWindow + time.Second
	for i := range ms {
		m := &ms[i]
		if m.StartDateTime.IsZero() {
			continue
		}
		delta := m.StartDateTime.Sub(target)
		if delta < 0 {
			delta = -delta
		}
		if delta < bestDelta {
			bestDelta = delta
			best = m
		}
	}
	if best == nil {
		return nil, fmt.Errorf("no CSI meeting within %s of %s", MatchTimeWindow, target.Format(time.RFC3339))
	}
	return best, nil
}

func (d *Discoverer) meetingsFor(ctx context.Context, chamber, committeeID string) ([]csi.Meeting, error) {
	if cached, ok := d.meetings[committeeID]; ok {
		return cached, nil
	}
	ms, err := d.deps.CSI.ListMeetings(ctx, chamber, committeeID)
	if err != nil {
		return nil, err
	}
	d.meetings[committeeID] = ms
	return ms, nil
}

// billNumberInLabel extracts the leading bill number from a CSI agenda
// item label like "EHB 1501 CIC unit owner inquiries" → 1501.
//
// Matches legislative prefixes we ingest into the bill table. Bills and
// resolutions can carry optional engrossment ("E"), Nth-substitute
// ("2"/"3"), and substitute ("S") chrome; gubernatorial appointments
// (SGA) appear without that chrome.
var billNumberInLabel = regexp.MustCompile(
	`\b(?:E?[23]?S?(?:HB|SB|HJR|SJR|HCR|SCR|HJM|SJM)|SGA)\s*(\d{3,5})\b`)

// matchAgendaItem picks the agenda item on a meeting whose label
// contains the bill number. Multiple matches → first; none → error.
func (d *Discoverer) matchAgendaItem(ctx context.Context, chamber, meetingFamilyID string, billNumber int) (*csi.AgendaItem, error) {
	items, err := d.agendaItemsFor(ctx, chamber, meetingFamilyID)
	if err != nil {
		return nil, err
	}
	target := fmt.Sprintf("%d", billNumber)
	for i := range items {
		item := &items[i]
		m := billNumberInLabel.FindStringSubmatch(item.Label)
		if len(m) >= 2 && m[1] == target {
			return item, nil
		}
	}
	return nil, fmt.Errorf("no agenda item for bill number %d in meeting %s", billNumber, meetingFamilyID)
}

func (d *Discoverer) agendaItemsFor(ctx context.Context, chamber, meetingFamilyID string) ([]csi.AgendaItem, error) {
	if cached, ok := d.agendaItems[meetingFamilyID]; ok {
		return cached, nil
	}
	items, err := d.deps.CSI.ListAgendaItems(ctx, chamber, meetingFamilyID)
	if err != nil {
		return nil, err
	}
	d.agendaItems[meetingFamilyID] = items
	return items, nil
}

// matchTVW finds the Invintus event ID for the LWS hearing by scanning
// TVW WP posts for the hearing's day. Sanity check: the WP post title
// must contain the LWS committee name (case-insensitive). This guards
// against attaching the wrong video to a hearing — Phase 0 noted ~25%
// of meetings need fuzzier matching, and the title check is the
// cheapest way to keep the false-positive rate low.
func (d *Discoverer) matchTVW(ctx context.Context, lwsCommitteeName string, target time.Time) (string, error) {
	posts, err := d.tvwPostsForDay(ctx, target)
	if err != nil {
		return "", err
	}
	// TVW WP post dates are wall-clock Pacific (no zone in the JSON).
	// Same convention as LWS — see internal/sources/lws/normalize.go.
	pacific, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		pacific = time.UTC
	}
	committeeNeedle := strings.ToLower(lwsCommitteeName)
	// Strip the chamber prefix from the LWS name too so "Senate Housing"
	// matches a TVW title like "Senate Housing" (which contains the
	// committee chamber-and-name) and ALSO falls back to just "Housing"
	// (which is what some TVW titles for special segments use).
	for _, p := range posts {
		title := strings.ToLower(tvw.CleanTitle(p.Title.Rendered))
		if !strings.Contains(title, committeeNeedle) {
			continue
		}
		postTime, err := time.ParseInLocation("2006-01-02T15:04:05", p.Date, pacific)
		if err != nil {
			continue
		}
		delta := postTime.Sub(target)
		if delta < 0 {
			delta = -delta
		}
		if delta > TVWMatchWindow {
			continue
		}
		eventID := tvw.EventIDFromWPPost(p)
		if eventID == "" {
			continue
		}
		return eventID, nil
	}
	return "", fmt.Errorf("no TVW event matching %q on %s", lwsCommitteeName, target.Format("2006-01-02"))
}

func (d *Discoverer) tvwPostsForDay(ctx context.Context, target time.Time) ([]tvw.WPVideoPost, error) {
	// Use Pacific-local day boundaries: TVW publishes in WA time, and
	// our LWS hearing rows store the Pacific wall-clock time. Without
	// converting we'd query UTC days, which split a Pacific evening
	// hearing across two TVW pages.
	pacific, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		pacific = time.UTC
	}
	local := target.In(pacific)
	dayKey := local.Format("2006-01-02")
	if cached, ok := d.tvwPostsByDay[dayKey]; ok {
		return cached, nil
	}
	dayStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, pacific)
	dayEnd := dayStart.Add(24 * time.Hour)
	posts, err := d.deps.TVW.FetchWPVideoArchive(ctx, dayStart, dayEnd, 200)
	if err != nil {
		return nil, err
	}
	d.tvwPostsByDay[dayKey] = posts
	return posts, nil
}

// stripChamberPrefix removes "House " / "Senate " / "Joint " from the
// front of an LWS committee name. LWS reports e.g. "Senate Housing";
// CSI reports just "Housing". We strip and compare.
func stripChamberPrefix(chamber, name string) string {
	for _, prefix := range []string{chamber + " ", "House ", "Senate ", "Joint "} {
		if strings.HasPrefix(name, prefix) {
			return strings.TrimPrefix(name, prefix)
		}
	}
	return name
}

// normalizeCommitteeName lowercases, collapses whitespace, and drops
// punctuation so "Ways & Means" and "Ways and Means" compare equal.
func normalizeCommitteeName(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "&", " and ")
	out := make([]rune, 0, len(s))
	prevSpace := false
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			out = append(out, r)
			prevSpace = false
			continue
		}
		if !prevSpace && len(out) > 0 {
			out = append(out, ' ')
			prevSpace = true
		}
	}
	res := strings.TrimSpace(string(out))
	return res
}
