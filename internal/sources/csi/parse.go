package csi

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"
)

// Committee is one row from <select id="SelectedCommitteeId">.
type Committee struct {
	Chamber string
	ID      string // e.g. "31634"
	Name    string // e.g. "Appropriations"
}

// Meeting is one row from /Home/GetMeetings.
type Meeting struct {
	MeetingFamilyID string    // option `value`
	Label           string    // option `text`, e.g. "03/05/26 8:00 AM"
	StartDateTime   time.Time // parsed from Label; zero on failure
}

// AgendaItem is one button.agendaItem from /Home/GetAgendaItems.
type AgendaItem struct {
	Chamber            string
	MeetingFamilyID    string
	AgendaItemFamilyID string
	AgendaItemID       string
	Label              string // e.g. "HB 2747 Budget sustainability"
}

// Testifier is one row from the data-json arrays on the two tables.
type Testifier struct {
	Name            string    `json:"Name"`
	Organization    string    `json:"Organization"`
	Position        string    `json:"Position"` // "Pro" | "Con" | "Other"
	Count           int       `json:"Count"`
	CssClass        string    `json:"CssClass"`
	TimeSignedIn    time.Time `json:"-"`
	TimeSignedInRaw string    `json:"TimeSignedIn"`
	Testified       bool      // true if from #testifyingDataTable, false if #notTestifyingDataTable
}

// ParseChamberCommittees pulls <option> values from the SelectedCommitteeId
// <select> block. We narrow the scan to that block to avoid picking up other
// dropdowns on the page.
func ParseChamberCommittees(body []byte) ([]Committee, error) {
	html := string(body)
	block := narrowToSelect(html, "SelectedCommitteeId")
	matches := optionRe.FindAllStringSubmatch(block, -1)
	out := make([]Committee, 0, len(matches))
	for _, m := range matches {
		id := strings.TrimSpace(m[1])
		name := strings.TrimSpace(unescapeHTML(m[2]))
		if id == "" || name == "" || strings.EqualFold(name, "Select") {
			continue
		}
		out = append(out, Committee{ID: id, Name: name})
	}
	return out, nil
}

// ParseMeetings decodes the JSON option list returned by GetMeetings and
// parses each `text` field into a UTC timestamp where possible.
func ParseMeetings(body []byte) ([]Meeting, error) {
	var raw []struct {
		Text  string `json:"text"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode GetMeetings: %w", err)
	}
	out := make([]Meeting, 0, len(raw))
	for _, r := range raw {
		if r.Value == "" {
			continue
		}
		m := Meeting{MeetingFamilyID: r.Value, Label: r.Text}
		m.StartDateTime, _ = parseMeetingDate(r.Text)
		out = append(out, m)
	}
	return out, nil
}

// ParseAgendaItems extracts the (meetingFamilyId, agendaItemFamilyId,
// agendaItemId, label) tuple from each agendaItem button's onclick and inner
// text. The onclick template is:
//
//	WSLApp.Testimony.getTestimonyTypes($(this), '<chamber>', <mfId>, <aifId>, <aiId>)
func ParseAgendaItems(body []byte) ([]AgendaItem, error) {
	html := string(body)
	matches := agendaItemRe.FindAllStringSubmatch(html, -1)
	out := make([]AgendaItem, 0, len(matches))
	for _, m := range matches {
		// m[1] = onclick attr value, m[2] = inner text
		argsMatch := getTestimonyTypesRe.FindStringSubmatch(m[1])
		if argsMatch == nil {
			continue
		}
		// argsMatch: [full, chamber, meetingFamilyId, agendaItemFamilyId, agendaItemId]
		out = append(out, AgendaItem{
			MeetingFamilyID:    argsMatch[2],
			AgendaItemFamilyID: argsMatch[3],
			AgendaItemID:       argsMatch[4],
			Label:              cleanInnerText(m[2]),
		})
	}
	return out, nil
}

// ParseTestifiers extracts both testifying and not-testifying tables from
// the GetOtherTestifiers HTML response.
func ParseTestifiers(body []byte) ([]Testifier, error) {
	html := string(body)
	testifying, err := extractDataJSON(html, "testifyingDataTable")
	if err != nil {
		return nil, fmt.Errorf("testifyingDataTable: %w", err)
	}
	notTestifying, err := extractDataJSON(html, "notTestifyingDataTable")
	if err != nil {
		return nil, fmt.Errorf("notTestifyingDataTable: %w", err)
	}

	out := make([]Testifier, 0, len(testifying)+len(notTestifying))
	for _, t := range testifying {
		t.Testified = true
		t.TimeSignedIn = parseSignInTime(t.TimeSignedInRaw)
		out = append(out, t)
	}
	for _, t := range notTestifying {
		t.Testified = false
		t.TimeSignedIn = parseSignInTime(t.TimeSignedInRaw)
		out = append(out, t)
	}
	return out, nil
}

func extractDataJSON(html, divID string) ([]Testifier, error) {
	// Scan for <... id="<divID>" ...>; then grab its data-json="...". We use
	// a regexp because the full HTML is small and we want resilience to
	// attribute reordering.
	idIdx := strings.Index(html, `id="`+divID+`"`)
	if idIdx < 0 {
		return nil, nil // table not present on this agenda item
	}
	openTag := strings.LastIndex(html[:idIdx], "<")
	if openTag < 0 {
		return nil, nil
	}
	tagEnd := strings.Index(html[openTag:], ">")
	if tagEnd < 0 {
		return nil, nil
	}
	tag := html[openTag : openTag+tagEnd+1]
	dj := dataJSONRe.FindStringSubmatch(tag)
	if dj == nil {
		return nil, nil // no data-json (likely empty table)
	}
	raw := unescapeHTML(dj[1])
	if strings.TrimSpace(raw) == "" || raw == "[]" {
		return nil, nil
	}
	var out []Testifier
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("unmarshal data-json: %w", err)
	}
	return out, nil
}

// narrowToSelect returns the substring covering <select id="<id>" ...>...</select>.
// Falls back to the whole input if the boundaries can't be found.
func narrowToSelect(s, selectID string) string {
	idx := strings.Index(s, `id="`+selectID+`"`)
	if idx < 0 {
		return s
	}
	open := strings.LastIndex(s[:idx], "<select")
	if open < 0 {
		return s
	}
	closeIdx := strings.Index(s[idx:], "</select>")
	if closeIdx < 0 {
		return s
	}
	return s[open : idx+closeIdx+len("</select>")]
}

func cleanInnerText(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	s = collapseSpace(s)
	return strings.TrimSpace(unescapeHTML(s))
}

func collapseSpace(s string) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if r == ' ' {
			if !prevSpace {
				b.WriteRune(' ')
			}
			prevSpace = true
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	return b.String()
}

// parseMeetingDate parses CSI's "MM/DD/YY HH:MM AM/PM" wall-clock label as
// Pacific time (the WA Legislature is in Olympia year-round). We use
// time.LoadLocation("America/Los_Angeles") so DST is handled correctly.
func parseMeetingDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		loc = time.UTC // safety net; shouldn't happen on any real system
	}
	for _, layout := range []string{
		"01/02/06 3:04 PM",
		"01/02/06 03:04 PM",
		"01/02/2006 3:04 PM",
		"01/02/2006 03:04 PM",
	} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// parseSignInTime parses CSI's TimeSignedIn (e.g. "2026-03-04T14:40:15.5530000")
// into UTC. Returns zero on failure. The trailing 0s are sub-microsecond
// precision Go can't natively parse, so we trim to 9 fractional digits.
func parseSignInTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	// Trim fractional seconds to <= 9 digits (Go's time.Parse limit).
	if dot := strings.IndexByte(s, '.'); dot >= 0 {
		end := dot + 1
		for end < len(s) && s[end] >= '0' && s[end] <= '9' {
			end++
		}
		fracLen := end - dot - 1
		if fracLen > 9 {
			s = s[:dot+1+9] + s[end:]
		}
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
			return t.UTC()
		}
	}
	return time.Time{}
}

// unescapeHTML wraps html.UnescapeString and the few hand-rolled escapes we
// see in CSI's HTML (notably &amp;).
func unescapeHTML(s string) string {
	return html.UnescapeString(s)
}

// Regexes — narrow patterns that match observed page output.
var (
	optionRe   = regexp.MustCompile(`(?is)<option\s+[^>]*value="(\d+)"[^>]*>([^<]+)</option>`)
	dataJSONRe = regexp.MustCompile(`(?is)data-json="([^"]*)"`)

	// <button class="agendaItem" id="agendaItem-171540" onclick="...">label</button>
	agendaItemRe = regexp.MustCompile(`(?is)<button[^>]*class="[^"]*\bagendaItem\b[^"]*"[^>]*onclick="([^"]+)"[^>]*>(.*?)</button>`)
	// WSLApp.Testimony.getTestimonyTypes($(this), 'House', 34109, 171540, 28599)
	getTestimonyTypesRe = regexp.MustCompile(`(?i)WSLApp\.Testimony\.getTestimonyTypes\(\s*[^,]+,\s*['"]([^'"]+)['"]\s*,\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*\)`)
)
