package lws

import (
	"strconv"
	"strings"
	"time"
)

// NormalizedBill is the subset of `bill` table fields populated from a
// Legislation + CurrentStatus pair.
type NormalizedBill struct {
	Biennium       string
	Prefix         string // "HB" | "SB" | "HJR" | etc.
	Number         int    // 1234
	Title          string // ShortDescription
	Description    string // LongDescription
	ChamberOrigin  string // OriginalAgency
	CurrentStatus  string // CurrentStatus.HistoryLine
	StatusDate     time.Time
	OfficialURL    string
}

// NormalizeBill maps a Legislation into the canonical bill shape. The
// official-URL pattern is documented in Blueprint line 97:
//   https://app.leg.wa.gov/billsummary?BillNumber=<n>&Year=<biennium-start>
func NormalizeBill(l *Legislation) NormalizedBill {
	prefix, num := splitBillID(l.BillID)
	if num == 0 {
		// Fall back to <BillNumber>.
		if n, err := strconv.Atoi(strings.TrimSpace(l.BillNumber)); err == nil {
			num = n
		}
	}
	out := NormalizedBill{
		Biennium:      l.Biennium,
		Prefix:        prefix,
		Number:        num,
		Title:         l.ShortDescription,
		Description:   l.LongDescription,
		ChamberOrigin: l.OriginalAgency,
		OfficialURL:   officialBillURL(l.Biennium, num),
	}
	if l.CurrentStatus != nil {
		out.CurrentStatus = l.CurrentStatus.HistoryLine
		out.StatusDate, _ = parseLWSDate(l.CurrentStatus.ActionDate)
	}
	return out
}

// NormalizedSponsor maps onto the legislator + bill_sponsor join.
type NormalizedSponsor struct {
	LWSSponsorID string
	Name         string
	LongName     string
	Chamber      string
	SponsorType  string // "Primary" | "Secondary"
}

// NormalizeSponsors maps the Sponsor list. Caller derives chamber from
// Sponsor.Agency (House/Senate) and uses LWSSponsorID as the legislator key.
func NormalizeSponsors(in []Sponsor) []NormalizedSponsor {
	out := make([]NormalizedSponsor, 0, len(in))
	for _, s := range in {
		out = append(out, NormalizedSponsor{
			LWSSponsorID: s.ID,
			Name:         s.LongName,
			LongName:     s.LongName,
			Chamber:      s.Agency,
			SponsorType:  s.Type,
		})
	}
	return out
}

// NormalizedHearing matches the `hearing` table shape (the rows we can fill
// from LWS alone — committee_schedule_*  and tvw_event_id come from
// elsewhere).
type NormalizedHearing struct {
	BillID            string
	Biennium          string
	CommitteeName     string
	CommitteeAcronym  string
	Chamber           string
	MeetingDateTime   time.Time
	Location          string
	LWSMeetingID      string // we reuse AgendaId as the meeting key
	HearingType       string
	HearingDescription string
}

// NormalizeHearings maps hearings.
func NormalizeHearings(in []Hearing) []NormalizedHearing {
	out := make([]NormalizedHearing, 0, len(in))
	for _, h := range in {
		nh := NormalizedHearing{
			BillID:             h.BillID,
			Biennium:           h.Biennium,
			Chamber:            h.CommitteeMeeting.Agency,
			Location:           strings.TrimSpace(h.CommitteeMeeting.Room + " " + h.CommitteeMeeting.Building),
			LWSMeetingID:       h.CommitteeMeeting.AgendaID,
			HearingType:        h.HearingType,
			HearingDescription: h.HearingTypeDescription,
		}
		nh.MeetingDateTime, _ = parseLWSDate(h.CommitteeMeeting.Date)
		if len(h.CommitteeMeeting.Committees) > 0 {
			nh.CommitteeName = h.CommitteeMeeting.Committees[0].LongName
			nh.CommitteeAcronym = h.CommitteeMeeting.Committees[0].Acronym
		}
		out = append(out, nh)
	}
	return out
}

// splitBillID parses "HB 1234" → ("HB", 1234). Returns ("",0) on failure.
//
// LWS reports BillID as the bill's *current* form, including engrossment
// and substitution chrome ("ESSB 6054", "2SHB 1859"). The bill's identity
// — the chamber+number that humans cite and that public bill URLs use —
// is the bare form, so we strip the chrome here to keep one row per bill
// across the legislative cycle.
func splitBillID(billID string) (string, int) {
	parts := strings.Fields(billID)
	if len(parts) != 2 {
		return "", 0
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0
	}
	return baseBillPrefix(parts[0]), n
}

// baseBillPrefix removes engrossment ("E"), Nth-substitute ("2"/"3"), and
// substitute ("S") chrome from a bill prefix.
//
//	"HB"    → "HB"     (already bare)
//	"SHB"   → "HB"     (Substitute House Bill)
//	"2SHB"  → "HB"     (Second Substitute House Bill)
//	"ESHB"  → "HB"     (Engrossed Substitute House Bill)
//	"E2SSB" → "SB"     (Engrossed Second Substitute Senate Bill)
//	"HJR"   → "HJR"    (House Joint Resolution — not a bill)
//
// Anything that doesn't end in a known base type ("HB"/"SB"/"HJR"/"SJR"/
// "HCR"/"SCR"/"HJM"/"SJM") is returned unchanged so unknown shapes stay
// observable rather than getting silently rewritten.
func baseBillPrefix(prefix string) string {
	for _, base := range []string{"HB", "SB", "HJR", "SJR", "HCR", "SCR", "HJM", "SJM"} {
		if strings.HasSuffix(prefix, base) {
			return base
		}
	}
	return prefix
}

// parseLWSDate parses LWS's "2025-01-13T00:00:00" timestamps. Trailing
// fractional seconds (e.g. "2025-01-16T14:02:35.103") are also handled.
//
// LWS emits these as Pacific wall-clock values without an explicit zone —
// the WA Legislature is in Olympia year-round, so we parse them in
// America/Los_Angeles to keep DST correct. Pure date fields (with
// 00:00:00) come out as midnight Pacific, which is the right behavior for
// the bill_status_change action_date column (date-typed in Postgres).
//
// Returns the zero time.Time on failure.
func parseLWSDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		loc = time.UTC
	}
	for _, layout := range []string{
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05",
	} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, nil
}

// officialBillURL builds the public bill-summary URL for a (biennium, number)
// pair. biennium is "2025-26"; the URL takes the start year ("2025").
func officialBillURL(biennium string, number int) string {
	year := biennium
	if i := strings.IndexByte(biennium, '-'); i > 0 {
		year = biennium[:i]
	}
	return "https://app.leg.wa.gov/billsummary?BillNumber=" + strconv.Itoa(number) + "&Year=" + year
}
