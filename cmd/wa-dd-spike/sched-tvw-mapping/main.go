// sched-tvw-mapping answers Phase 0 question 2: what share of Committee
// Schedules meetings expose a TVW event ID directly (vs. needing a date/title
// fuzzy match against the TVW schedule API)?
//
// Approach: POST /committeeschedules/Home/Search/ with a date range, parse the
// HTML, and look for showVideoModal(id, eventID) tokens in inline JS.
//
// Caveat (per data-sources/03 line 142): ASP.NET form fields and anti-forgery
// tokens may need to be discovered first by hand from a browser session. This
// spike attempts a best-effort search; if it returns 0 rows or HTTP errors,
// the operator should capture a real form submission with browser devtools and
// hardcode the form fields/tokens here.
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
	"strings"
	"time"
)

const (
	csBase            = "https://app.leg.wa.gov/committeeschedules"
	requestTimeoutSec = 30
)

// showVideoModal(<videoModalID>, <tvwEventID>) — both are quoted or unquoted ints
var videoModalRe = regexp.MustCompile(`showVideoModal\(\s*['"]?(\d+)['"]?\s*,\s*['"]?(\d+)['"]?\s*\)`)

// onclick="showAgendaDetailModal(123)" or similar — used to count meetings
var agendaModalRe = regexp.MustCompile(`showAgendaDetailModal\(\s*['"]?(\d+)['"]?\s*\)`)

type findings struct {
	GeneratedAt          time.Time `json:"generated_at"`
	WindowStart          string    `json:"window_start"`
	WindowEnd            string    `json:"window_end"`
	HTTPStatus           int       `json:"http_status"`
	ResponseBytes        int       `json:"response_bytes"`
	AgendaModalIDsFound  int       `json:"agenda_modal_ids_found"`
	VideoModalIDsFound   int       `json:"video_modal_ids_found"`
	TVWEventIDsFound     int       `json:"tvw_event_ids_found"`
	UniqueTVWEventIDs    []string  `json:"unique_tvw_event_ids"`
	RawSnapshotPath      string    `json:"raw_snapshot_path"`
	NeedsManualFormDiscovery bool  `json:"needs_manual_form_discovery"`
	Notes                []string  `json:"notes"`
}

func main() {
	var (
		days   = flag.Int("days", 30, "number of days to scan, centered on -anchor")
		out    = flag.String("out", "data/spike/sched-tvw-mapping.json", "output JSON path")
		raw    = flag.String("raw", "data/spike/sched-tvw-mapping.html", "raw HTML snapshot path")
		ua     = flag.String("ua", "wa-digital-democracy-spike/0.1 (research; nolan-mccafferty)", "User-Agent header")
		mode   = flag.String("mode", "Schedule", "SearchType: Schedule, Agenda, or Bill")
		anchor = flag.String("anchor", "", "YYYY-MM-DD center date (default: today)")
	)
	flag.Parse()

	center := time.Now().UTC()
	if *anchor != "" {
		t, err := time.Parse("2006-01-02", *anchor)
		if err != nil {
			log.Fatalf("parse -anchor: %v", err)
		}
		center = t
	}
	start := center.AddDate(0, 0, -*days/2)
	end := center.AddDate(0, 0, *days/2)

	form := url.Values{}
	form.Set("StartDate", start.Format("01022006")) // MMDDYYYY per spec 03 line 60
	form.Set("EndDate", end.Format("01022006"))
	form.Set("Chamber", "")
	form.Set("Committee", "")
	form.Set("BillNumber", "")
	form.Set("SearchType", *mode)

	httpClient := &http.Client{Timeout: requestTimeoutSec * time.Second}
	req, _ := http.NewRequest(http.MethodPost, csBase+"/Home/Search/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html, */*")
	req.Header.Set("User-Agent", *ua)
	req.Header.Set("X-Requested-With", "XMLHttpRequest") // many ASP.NET endpoints expect this

	log.Printf("POST %s/Home/Search/  Start=%s End=%s SearchType=%s",
		csBase, form.Get("StartDate"), form.Get("EndDate"), *mode)
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("read body: %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(*raw), 0o755); err != nil {
		log.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(*raw, body, 0o644); err != nil {
		log.Fatalf("write raw: %v", err)
	}

	f := findings{
		GeneratedAt:     time.Now().UTC(),
		WindowStart:     start.Format("2006-01-02"),
		WindowEnd:       end.Format("2006-01-02"),
		HTTPStatus:      resp.StatusCode,
		ResponseBytes:   len(body),
		RawSnapshotPath: *raw,
	}
	html := string(body)

	agendaIDs := uniqueGroup(agendaModalRe, html, 1)
	videoTokens := videoModalRe.FindAllStringSubmatch(html, -1)
	tvwIDs := map[string]struct{}{}
	for _, m := range videoTokens {
		// m[1] = committee_schedule_video_id, m[2] = tvw_event_id
		tvwIDs[m[2]] = struct{}{}
	}

	f.AgendaModalIDsFound = len(agendaIDs)
	f.VideoModalIDsFound = len(videoTokens)
	f.TVWEventIDsFound = len(tvwIDs)
	for id := range tvwIDs {
		f.UniqueTVWEventIDs = append(f.UniqueTVWEventIDs, id)
	}

	// Heuristic: if the response is small and we got no modal IDs, the
	// search form probably needs anti-forgery tokens or session cookies.
	if resp.StatusCode != http.StatusOK || (f.AgendaModalIDsFound == 0 && f.VideoModalIDsFound == 0) {
		f.NeedsManualFormDiscovery = true
		f.Notes = append(f.Notes, "no modal IDs parsed; capture a real browser POST and replay form fields/tokens (spec 03 lines 142–143).")
	}
	if resp.StatusCode != http.StatusOK {
		f.Notes = append(f.Notes, fmt.Sprintf("HTTP %d — non-OK response, raw saved at %s", resp.StatusCode, *raw))
	}

	if err := writeJSON(*out, f); err != nil {
		log.Fatalf("write %s: %v", *out, err)
	}
	log.Printf("wrote %s", *out)
	log.Printf("HTTP %d  bytes=%d  agendaModalIDs=%d  videoModalIDs=%d  tvwEventIDs=%d  needs-manual-form-discovery=%v",
		f.HTTPStatus, f.ResponseBytes, f.AgendaModalIDsFound, f.VideoModalIDsFound, f.TVWEventIDsFound, f.NeedsManualFormDiscovery)
}

func uniqueGroup(re *regexp.Regexp, s string, group int) []string {
	matches := re.FindAllStringSubmatch(s, -1)
	seen := map[string]struct{}{}
	out := []string{}
	for _, m := range matches {
		if group >= len(m) {
			continue
		}
		if _, ok := seen[m[group]]; ok {
			continue
		}
		seen[m[group]] = struct{}{}
		out = append(out, m[group])
	}
	return out
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
