// csi-history answers Phase 0 question 3: how far back does CSI retain
// public testifier sign-in / meeting data?
//
// Approach (per data-sources/02 Washington Legislature CSI Client.md):
//
//  1. GET https://app.leg.wa.gov/csi/<chamber> — parse <select id="SelectedCommitteeId">
//     options for committee IDs.
//  2. For each committee, GET /csi/Home/GetMeetings?chamber=&committeeId= —
//     JSON option list whose `text` looks like "MM/DD/YY HH:MM AM/PM".
//  3. Find the earliest date returned per chamber and across all chambers.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	csiBase           = "https://app.leg.wa.gov/csi"
	requestTimeoutSec = 30
)

// <option value="31634">Appropriations</option>
var optionRe = regexp.MustCompile(`(?is)<option\s+[^>]*value="(\d+)"[^>]*>([^<]+)</option>`)

// "03/05/26 8:00 AM" — CSI's `text` format
var meetingDateLayouts = []string{
	"01/02/06 3:04 PM",
	"01/02/06 03:04 PM",
	"01/02/2006 3:04 PM",
	"01/02/2006 03:04 PM",
}

type committee struct {
	Chamber string `json:"chamber"`
	ID      string `json:"id"`
	Name    string `json:"name"`
}

type meetingOption struct {
	Text  string `json:"text"`
	Value string `json:"value"`
}

type committeeFinding struct {
	Chamber           string    `json:"chamber"`
	CommitteeID       string    `json:"committee_id"`
	CommitteeName     string    `json:"committee_name"`
	MeetingCount      int       `json:"meeting_count"`
	EarliestMeeting   *time.Time `json:"earliest_meeting,omitempty"`
	LatestMeeting     *time.Time `json:"latest_meeting,omitempty"`
	UnparsedDates     []string  `json:"unparsed_dates,omitempty"`
	Error             string    `json:"error,omitempty"`
}

type chamberSummary struct {
	Chamber                string     `json:"chamber"`
	CommitteesDiscovered   int        `json:"committees_discovered"`
	CommitteesWithMeetings int        `json:"committees_with_meetings"`
	EarliestAcrossChamber  *time.Time `json:"earliest_across_chamber,omitempty"`
	LatestAcrossChamber    *time.Time `json:"latest_across_chamber,omitempty"`
}

type report struct {
	GeneratedAt    time.Time          `json:"generated_at"`
	Chambers       []chamberSummary   `json:"chambers"`
	ByCommittee    []committeeFinding `json:"by_committee"`
}

func main() {
	var (
		out        = flag.String("out", "data/spike/csi-history.json", "output JSON path")
		ua         = flag.String("ua", "wa-digital-democracy-spike/0.1 (research; nolan-mccafferty)", "User-Agent header")
		chambersIn = flag.String("chambers", "House,Senate,Joint", "comma-separated chambers to scan")
		delayMs    = flag.Int("delay-ms", 200, "delay between CSI requests (ms)")
	)
	flag.Parse()

	chambers := splitCSV(*chambersIn)
	httpClient := &http.Client{Timeout: requestTimeoutSec * time.Second}

	var allCommittees []committee
	for _, ch := range chambers {
		log.Printf("discovering committees: chamber=%s", ch)
		cs, err := listCommittees(httpClient, *ua, ch)
		if err != nil {
			log.Printf("  ERROR: %v", err)
			continue
		}
		log.Printf("  found %d committees", len(cs))
		allCommittees = append(allCommittees, cs...)
	}

	rep := report{GeneratedAt: time.Now().UTC()}
	chamberAgg := map[string]*chamberSummary{}
	for _, ch := range chambers {
		chamberAgg[ch] = &chamberSummary{Chamber: ch}
	}

	for i, c := range allCommittees {
		log.Printf("[%d/%d] meetings for %s/%s (%s)", i+1, len(allCommittees), c.Chamber, c.Name, c.ID)
		f := committeeFinding{Chamber: c.Chamber, CommitteeID: c.ID, CommitteeName: c.Name}

		options, err := getMeetings(httpClient, *ua, c.Chamber, c.ID)
		if err != nil {
			f.Error = err.Error()
			rep.ByCommittee = append(rep.ByCommittee, f)
			time.Sleep(time.Duration(*delayMs) * time.Millisecond)
			continue
		}
		var dates []time.Time
		for _, opt := range options {
			if opt.Value == "" {
				continue // "Select" placeholder row
			}
			t, ok := parseMeetingDate(opt.Text)
			if !ok {
				f.UnparsedDates = append(f.UnparsedDates, opt.Text)
				continue
			}
			dates = append(dates, t)
		}
		f.MeetingCount = len(dates)
		if len(dates) > 0 {
			sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })
			earliest := dates[0]
			latest := dates[len(dates)-1]
			f.EarliestMeeting = &earliest
			f.LatestMeeting = &latest

			cs := chamberAgg[c.Chamber]
			cs.CommitteesWithMeetings++
			if cs.EarliestAcrossChamber == nil || earliest.Before(*cs.EarliestAcrossChamber) {
				e := earliest
				cs.EarliestAcrossChamber = &e
			}
			if cs.LatestAcrossChamber == nil || latest.After(*cs.LatestAcrossChamber) {
				l := latest
				cs.LatestAcrossChamber = &l
			}
		}
		if cs := chamberAgg[c.Chamber]; cs != nil {
			cs.CommitteesDiscovered++
		}
		rep.ByCommittee = append(rep.ByCommittee, f)
		time.Sleep(time.Duration(*delayMs) * time.Millisecond)
	}

	for _, ch := range chambers {
		rep.Chambers = append(rep.Chambers, *chamberAgg[ch])
	}

	if err := writeJSON(*out, rep); err != nil {
		log.Fatalf("write %s: %v", *out, err)
	}
	log.Printf("wrote %s", *out)
	for _, cs := range rep.Chambers {
		log.Printf("  chamber=%-7s committees=%-3d with-meetings=%-3d earliest=%s latest=%s",
			cs.Chamber, cs.CommitteesDiscovered, cs.CommitteesWithMeetings,
			fmtTime(cs.EarliestAcrossChamber), fmtTime(cs.LatestAcrossChamber))
	}
}

func listCommittees(c *http.Client, ua, chamber string) ([]committee, error) {
	u := csiBase + "/" + chamber
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("User-Agent", ua)
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d for %s", resp.StatusCode, u)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	html := string(body)

	// Narrow to the SelectedCommitteeId <select> block to avoid picking up
	// other dropdowns on the page.
	block := narrowToSelect(html, "SelectedCommitteeId")
	matches := optionRe.FindAllStringSubmatch(block, -1)
	out := make([]committee, 0, len(matches))
	for _, m := range matches {
		id, name := m[1], strings.TrimSpace(m[2])
		if id == "" || name == "" || strings.EqualFold(name, "Select") {
			continue
		}
		out = append(out, committee{Chamber: chamber, ID: id, Name: name})
	}
	return out, nil
}

// narrowToSelect returns the substring between the opening <select id="<id>" ...>
// and its closing </select>. If not found, returns the input unchanged so the
// regex still has a chance.
func narrowToSelect(html, selectID string) string {
	idx := strings.Index(html, `id="`+selectID+`"`)
	if idx < 0 {
		return html
	}
	open := strings.LastIndex(html[:idx], "<select")
	if open < 0 {
		return html
	}
	close := strings.Index(html[idx:], "</select>")
	if close < 0 {
		return html
	}
	return html[open : idx+close+len("</select>")]
}

func getMeetings(c *http.Client, ua, chamber, committeeID string) ([]meetingOption, error) {
	v := url.Values{}
	v.Set("chamber", chamber)
	v.Set("committeeId", committeeID)
	u := csiBase + "/Home/GetMeetings?" + v.Encode()
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("User-Agent", ua)
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var out []meetingOption
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return out, nil
}

func parseMeetingDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range meetingDateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func fmtTime(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format("2006-01-02")
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
