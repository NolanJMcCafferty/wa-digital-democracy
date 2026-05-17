// Package tvw is the TVW / Invintus connector.
//
// Sources (per data-sources/04 TVW Invintus Client.md):
//
//   - WordPress REST at https://tvw.org/wp-json/
//   - GET /tvw/v1/schedule          (currently-airing event list)
//   - GET /wp/v2/invintus_video     (paginated archive, by post date)
//   - GET /wp/v2/invintus_video/{wp_post_id}
//   - Invintus REST at https://api.v3.invintus.com/v2/
//   - POST /Event/getDetailed       (returns captionPath when available)
//   - Caption files at the captionPath URL (WebVTT)
//
// Phase 0 findings (docs/phase0-spike-report.md):
//
//   - tvw/v1/schedule ignores its date params; use wp/v2/invintus_video for
//     historical sampling.
//   - 100% caption coverage on legislative-committee and floor events
//     sampled — captions are a viable single transcript source for the MVP.
package tvw

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

const (
	// SystemName matches source_record.source_system.
	SystemName = "tvw"
	// InvintusSystemName is used when recording the Invintus event-detail
	// fetch — it's a different host with different terms-of-use.
	InvintusSystemName = "invintus"

	WPBase             = "https://tvw.org/wp-json"
	InvintusBase       = "https://api.v3.invintus.com/v2"
	defaultClientID    = "9375922947"
	defaultEmbedderTag = "embedder"
)

// Client wraps an httpx.Client with TVW + Invintus configuration.
type Client struct {
	HTTP             *httpx.Client
	WPBaseURL        string
	InvintusBaseURL  string
	ClientID         string
	EmbedderKey      string // public Invintus embedder key (wsc-api-key)
	EmbedderAuthName string // header value for "authorization"; default "embedder"
}

// New constructs a Client. EmbedderKey is required for Invintus calls; WP
// REST and caption GETs work without it.
func New(h *httpx.Client, embedderKey string) *Client {
	return &Client{
		HTTP:             h,
		WPBaseURL:        WPBase,
		InvintusBaseURL:  InvintusBase,
		ClientID:         defaultClientID,
		EmbedderKey:      embedderKey,
		EmbedderAuthName: defaultEmbedderTag,
	}
}

// FetchSchedule returns the "currently-airing" schedule list. Date params
// are accepted but ignored by the upstream API — see Phase 0 findings.
func (c *Client) FetchSchedule(ctx context.Context, start, end time.Time) ([]ScheduleEvent, error) {
	q := url.Values{}
	q.Set("start", start.Format("2006-01-02"))
	q.Set("end", end.Format("2006-01-02"))
	u := c.WPBaseURL + "/tvw/v1/schedule?" + q.Encode()

	fetch, err := c.HTTP.Do(ctx, httpx.Request{
		System:   SystemName,
		Endpoint: "tvw.v1.schedule",
		URL:      u,
		Headers:  jsonAccept(),
	})
	if err != nil {
		return nil, err
	}
	return ParseSchedule(fetch.Body)
}

// FetchWPVideoArchive paginates /wp/v2/invintus_video filtered by post date.
// This is the production discovery path; max caps total events returned.
func (c *Client) FetchWPVideoArchive(ctx context.Context, after, before time.Time, max int) ([]WPVideoPost, error) {
	const pageSize = 100
	var out []WPVideoPost
	for page := 1; len(out) < max && page < 100; page++ {
		q := url.Values{}
		q.Set("per_page", fmt.Sprintf("%d", pageSize))
		q.Set("page", fmt.Sprintf("%d", page))
		q.Set("orderby", "date")
		q.Set("order", "desc")
		q.Set("after", after.Format(time.RFC3339))
		q.Set("before", before.Format(time.RFC3339))
		u := c.WPBaseURL + "/wp/v2/invintus_video?" + q.Encode()

		fetch, err := c.HTTP.Do(ctx, httpx.Request{
			System:   SystemName,
			Endpoint: "wp.v2.invintus_video.list",
			URL:      u,
			Headers:  jsonAccept(),
		})
		if err != nil {
			// WordPress returns 400 with rest_post_invalid_page_number when
			// paging past the end; treat as natural termination.
			if isRestPaginationOver(err) {
				break
			}
			return nil, err
		}
		posts, err := ParseWPVideoList(fetch.Body)
		if err != nil {
			return nil, err
		}
		if len(posts) == 0 {
			break
		}
		out = append(out, posts...)
		if len(posts) < pageSize {
			break
		}
	}
	if len(out) > max {
		out = out[:max]
	}
	return out, nil
}

// FetchEventDetail calls Invintus Event/getDetailed for one event ID.
func (c *Client) FetchEventDetail(ctx context.Context, eventID string) (*InvintusEvent, error) {
	if c.EmbedderKey == "" {
		return nil, errors.New("tvw: EmbedderKey required for Invintus calls")
	}
	body, _ := json.Marshal(map[string]string{
		"eventID":  eventID,
		"clientID": c.ClientID,
	})
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("authorization", c.EmbedderAuthName)
	headers.Set("wsc-api-key", c.EmbedderKey)
	headers.Set("Accept", "application/json")

	fetch, err := c.HTTP.Do(ctx, httpx.Request{
		System:   InvintusSystemName,
		Endpoint: "Event.getDetailed",
		Method:   http.MethodPost,
		URL:      c.InvintusBaseURL + "/Event/getDetailed",
		Headers:  headers,
		Body:     body,
	})
	if err != nil {
		return nil, err
	}
	return ParseEventDetail(fetch.Body)
}

// FetchCaptions GETs the WebVTT caption file at captionURL and parses it
// into transcript segments. Returns (nil, nil) if captionURL is empty.
func (c *Client) FetchCaptions(ctx context.Context, eventID, captionURL string) ([]TranscriptSegment, error) {
	if captionURL == "" {
		return nil, nil
	}
	fetch, err := c.HTTP.Do(ctx, httpx.Request{
		System:   InvintusSystemName,
		Endpoint: "captionPath",
		URL:      captionURL,
	})
	if err != nil {
		return nil, err
	}
	segments, err := ParseVTT(fetch.Body)
	if err != nil {
		return nil, err
	}
	for i := range segments {
		segments[i].TVWEventID = eventID
		segments[i].SourceCaptionURL = captionURL
	}
	return segments, nil
}

// EventIDFromWPPost returns the Invintus event ID embedded in a WP post's
// rendered HTML. Returns "" if not present.
func EventIDFromWPPost(p WPVideoPost) string {
	if m := dataEventIDRe.FindStringSubmatch(p.Content.Rendered); m != nil {
		return m[1]
	}
	return ""
}

// CleanTitle normalizes a WP-rendered title (HTML entities → text).
func CleanTitle(s string) string {
	return html.UnescapeString(strings.TrimSpace(s))
}

// dataEventIDRe matches the Invintus player tag: <div ... data-eventid="2025031249">.
// Per spec 04 lines 78–81.
var dataEventIDRe = regexp.MustCompile(`data-eventid="(\d+)"`)

func jsonAccept() http.Header {
	h := http.Header{}
	h.Set("Accept", "application/json")
	return h
}

// isRestPaginationOver returns true if err looks like WP's "page out of range"
// 400 response. We compare on substrings because httpx's error wraps the body.
func isRestPaginationOver(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "rest_post_invalid_page_number") ||
		strings.Contains(msg, "status 400")
}
