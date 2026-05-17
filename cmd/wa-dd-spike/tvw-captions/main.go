// tvw-captions answers Phase 0 question 1: what share of TVW legislative-committee
// events have a non-null Invintus captionPath VTT?
//
// Flow (per data-sources/04 TVW Invintus Client.md):
//
//  1. GET https://tvw.org/wp-json/tvw/v1/schedule?start=...&end=...
//  2. For each event, POST https://api.v3.invintus.com/v2/Event/getDetailed
//     with {eventID, clientID:"9375922947"} and headers
//     authorization: embedder, wsc-api-key: $INVINTUS_EMBEDDER_KEY.
//  3. Group by category (committee/floor/agency/other) and report
//     captionPath coverage.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	tvwScheduleURL    = "https://tvw.org/wp-json/tvw/v1/schedule"
	tvwInvintusVideo  = "https://tvw.org/wp-json/wp/v2/invintus_video"
	invintusEventURL  = "https://api.v3.invintus.com/v2/Event/getDetailed"
	invintusClientID  = "9375922947"
	requestTimeoutSec = 30
)

// scheduleEvent matches both /tvw/v1/schedule (data:[…]) and a flattened
// /wp/v2/invintus_video shape we build below.
type scheduleEvent struct {
	EventID       string `json:"eventID"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	EventDateTime string `json:"eventDateTime"`
	EventStatus   string `json:"eventStatus"`
}

// wpInvintusVideo is the WordPress REST shape for /wp/v2/invintus_video.
type wpInvintusVideo struct {
	ID    int    `json:"id"`
	Date  string `json:"date"`
	Title struct {
		Rendered string `json:"rendered"`
	} `json:"title"`
	Content struct {
		Rendered string `json:"rendered"`
	} `json:"content"`
	Excerpt struct {
		Rendered string `json:"rendered"`
	} `json:"excerpt"`
	Meta map[string]any `json:"meta"`
}

// data-eventid="2025031249" — the Invintus event ID is embedded in the
// rendered HTML of each WP post (per spec 04 lines 78–81).
var eventIDRe = regexp.MustCompile(`data-eventid="(\d+)"`)

type eventDetailResp struct {
	Data struct {
		EventID       string   `json:"eventID"`
		Title         string   `json:"title"`
		StartDateTime string   `json:"startDateTime"`
		Categories    []string `json:"categories"`
		LocationName  string   `json:"locationName"`
		CaptionPath   *string  `json:"captionPath"`
	} `json:"data"`
}

type result struct {
	EventID       string   `json:"event_id"`
	Title         string   `json:"title"`
	StartDateTime string   `json:"start_datetime"`
	Categories    []string `json:"categories"`
	Bucket        string   `json:"bucket"`
	HasCaption    bool     `json:"has_caption"`
	CaptionPath   string   `json:"caption_path,omitempty"`
	FetchError    string   `json:"fetch_error,omitempty"`
}

type bucketStats struct {
	Total       int `json:"total"`
	WithCaption int `json:"with_caption"`
}

type report struct {
	GeneratedAt   time.Time              `json:"generated_at"`
	WindowStart   string                 `json:"window_start"`
	WindowEnd     string                 `json:"window_end"`
	TotalEvents   int                    `json:"total_events"`
	TotalCaption  int                    `json:"total_with_caption"`
	ByBucket      map[string]bucketStats `json:"by_bucket"`
	FetchFailures int                    `json:"fetch_failures"`
	Events        []result               `json:"events"`
}

func main() {
	var (
		days     = flag.Int("days", 30, "number of days to scan")
		out      = flag.String("out", "data/spike/tvw-captions.json", "output JSON path")
		ua       = flag.String("ua", "wa-digital-democracy-spike/0.1 (research; nolan-mccafferty)", "User-Agent header")
		anchor   = flag.String("anchor", "", "YYYY-MM-DD end date (default: today). window = [anchor-days, anchor]")
		source   = flag.String("source", "wp", "event source: 'wp' (paginate /wp/v2/invintus_video by date) or 'schedule' (currently airing)")
		maxItems = flag.Int("max", 200, "max events to inspect")
		// Throttle conservatively — the wiki warns to rate-limit Invintus heavily.
		delayMs = flag.Int("delay-ms", 250, "delay between Invintus requests (ms)")
	)
	flag.Parse()

	key := os.Getenv("INVINTUS_EMBEDDER_KEY")
	if key == "" {
		log.Fatal("INVINTUS_EMBEDDER_KEY is required (see cmd/wa-dd-spike/README.md)")
	}

	end := time.Now().UTC()
	if *anchor != "" {
		t, err := time.Parse("2006-01-02", *anchor)
		if err != nil {
			log.Fatalf("parse -anchor: %v", err)
		}
		end = t
	}
	start := end.AddDate(0, 0, -*days)
	startStr := start.Format("2006-01-02")
	endStr := end.Format("2006-01-02")

	httpClient := &http.Client{Timeout: requestTimeoutSec * time.Second}

	var events []scheduleEvent
	var err error
	switch *source {
	case "schedule":
		log.Printf("fetching TVW schedule (currently-airing) %s..%s", startStr, endStr)
		events, err = fetchSchedule(httpClient, *ua, startStr, endStr)
	case "wp":
		log.Printf("paginating /wp/v2/invintus_video by date %s..%s (max %d)", startStr, endStr, *maxItems)
		events, err = fetchWPInvintusVideos(httpClient, *ua, start, end, *maxItems)
	default:
		log.Fatalf("unknown -source: %s", *source)
	}
	if err != nil {
		log.Fatalf("fetch events: %v", err)
	}
	log.Printf("got %d events", len(events))

	rep := report{
		GeneratedAt: time.Now().UTC(),
		WindowStart: startStr,
		WindowEnd:   endStr,
		TotalEvents: len(events),
		ByBucket:    map[string]bucketStats{},
	}

	for i, ev := range events {
		log.Printf("[%d/%d] event %s — %s", i+1, len(events), ev.EventID, truncate(ev.Title, 60))
		detail, err := fetchEventDetail(httpClient, *ua, key, ev.EventID)
		r := result{
			EventID:       ev.EventID,
			Title:         ev.Title,
			StartDateTime: ev.EventDateTime,
		}
		if err != nil {
			r.FetchError = err.Error()
			rep.FetchFailures++
		} else {
			r.Categories = detail.Data.Categories
			r.HasCaption = detail.Data.CaptionPath != nil && *detail.Data.CaptionPath != ""
			if r.HasCaption {
				r.CaptionPath = *detail.Data.CaptionPath
				rep.TotalCaption++
			}
		}
		r.Bucket = classify(ev.Title, r.Categories)
		bs := rep.ByBucket[r.Bucket]
		bs.Total++
		if r.HasCaption {
			bs.WithCaption++
		}
		rep.ByBucket[r.Bucket] = bs
		rep.Events = append(rep.Events, r)

		time.Sleep(time.Duration(*delayMs) * time.Millisecond)
	}

	if err := writeJSON(*out, rep); err != nil {
		log.Fatalf("write %s: %v", *out, err)
	}
	log.Printf("wrote %s", *out)
	log.Printf("total events=%d with-caption=%d (%.1f%%) failures=%d",
		rep.TotalEvents, rep.TotalCaption, pct(rep.TotalCaption, rep.TotalEvents), rep.FetchFailures)
	for bucket, stats := range rep.ByBucket {
		log.Printf("  bucket=%-12s total=%-3d with-caption=%-3d (%.1f%%)",
			bucket, stats.Total, stats.WithCaption, pct(stats.WithCaption, stats.Total))
	}
}

func fetchSchedule(c *http.Client, ua, start, end string) ([]scheduleEvent, error) {
	url := fmt.Sprintf("%s?start=%s&end=%s", tvwScheduleURL, start, end)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, body)
	}
	// TVW wraps the schedule list in {success, data:[…]}.
	var envelope struct {
		Success bool            `json:"success"`
		Data    []scheduleEvent `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decode schedule: %w", err)
	}
	return envelope.Data, nil
}

// fetchWPInvintusVideos paginates /wp/v2/invintus_video filtered by post date.
// WP REST supports `after`/`before` ISO timestamps, `orderby=date`, and
// `per_page` up to 100.
func fetchWPInvintusVideos(c *http.Client, ua string, start, end time.Time, max int) ([]scheduleEvent, error) {
	const pageSize = 100
	out := []scheduleEvent{}
	for page := 1; len(out) < max && page < 100; page++ {
		q := url.Values{}
		q.Set("per_page", fmt.Sprintf("%d", pageSize))
		q.Set("page", fmt.Sprintf("%d", page))
		q.Set("orderby", "date")
		q.Set("order", "desc")
		q.Set("after", start.Format(time.RFC3339))
		q.Set("before", end.Format(time.RFC3339))
		u := tvwInvintusVideo + "?" + q.Encode()

		req, _ := http.NewRequest(http.MethodGet, u, nil)
		req.Header.Set("User-Agent", ua)
		req.Header.Set("Accept", "application/json")
		resp, err := c.Do(req)
		if err != nil {
			return nil, err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusBadRequest {
			// WP returns 400 when paging past the end with rest_post_invalid_page_number.
			break
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("wp invintus_video status %d: %s", resp.StatusCode, truncate(string(body), 200))
		}
		var posts []wpInvintusVideo
		if err := json.Unmarshal(body, &posts); err != nil {
			return nil, fmt.Errorf("decode wp invintus_video page %d: %w", page, err)
		}
		if len(posts) == 0 {
			break
		}
		for _, p := range posts {
			eventID := ""
			if m := eventIDRe.FindStringSubmatch(p.Content.Rendered); m != nil {
				eventID = m[1]
			}
			if eventID == "" {
				continue // skip posts that don't expose an event ID
			}
			out = append(out, scheduleEvent{
				EventID:       eventID,
				Title:         html.UnescapeString(strings.TrimSpace(p.Title.Rendered)),
				EventDateTime: p.Date,
			})
			if len(out) >= max {
				break
			}
		}
		if len(posts) < pageSize {
			break
		}
	}
	return out, nil
}

func fetchEventDetail(c *http.Client, ua, key, eventID string) (*eventDetailResp, error) {
	body, _ := json.Marshal(map[string]string{
		"eventID":  eventID,
		"clientID": invintusClientID,
	})
	req, _ := http.NewRequest(http.MethodPost, invintusEventURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("authorization", "embedder")
	req.Header.Set("wsc-api-key", key)
	req.Header.Set("User-Agent", ua)
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, b)
	}
	var d eventDetailResp
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil, fmt.Errorf("decode event detail: %w", err)
	}
	return &d, nil
}

// classify groups events into buckets we care about for caption coverage stats.
// "legislative-committee" coverage is the load-bearing question for the MVP.
func classify(title string, categories []string) string {
	hay := strings.ToLower(title)
	for _, c := range categories {
		hay += " " + strings.ToLower(c)
	}
	switch {
	case strings.Contains(hay, "committee"), strings.Contains(hay, "subcommittee"):
		return "legislative-committee"
	case strings.Contains(hay, "floor"), strings.Contains(hay, "session"):
		return "legislative-floor"
	case strings.Contains(hay, "supreme court"), strings.Contains(hay, "court of appeals"):
		return "court"
	case strings.Contains(hay, "agency"), strings.Contains(hay, "board"), strings.Contains(hay, "commission"):
		return "agency-board"
	default:
		return "other"
	}
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
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

func dirOf(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i]
	}
	return "."
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func pct(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return 100 * float64(n) / float64(d)
}
