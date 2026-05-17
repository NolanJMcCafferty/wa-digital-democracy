// Package committeeschedules connects to the legislative Committee Schedules
// ASP.NET app (https://app.leg.wa.gov/committeeschedules/).
//
// Role per data-sources/03: enrichment over LWS. Use this to resolve
// (committee_schedule_video_id, tvw_event_id) pairs and to grab agenda
// document URLs, NOT as a canonical meeting source.
//
// Phase 0 finding: the search form's date params are ignored without an
// anti-forgery token / session cookie. For the first-page MVP we either:
//
//   1. Run the search "as-is" and accept the upstream-default returned set
//      (about 4 meetings, 75% of which carry a TVW event ID), OR
//   2. Capture-and-replay an authenticated browser POST. That capture is
//      operator work; this client just exposes the form fields so the
//      operator can pass a captured __RequestVerificationToken.
package committeeschedules

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

const (
	SystemName     = "committee_schedules"
	DefaultBaseURL = "https://app.leg.wa.gov/committeeschedules"
)

// Client wraps an httpx.Client.
type Client struct {
	HTTP    *httpx.Client
	BaseURL string
}

// New returns a Client.
func New(h *httpx.Client) *Client {
	return &Client{HTTP: h, BaseURL: DefaultBaseURL}
}

// FormatSearchDate is the MMDDYYYY string the search form expects.
func FormatSearchDate(t time.Time) string { return t.Format("01022006") }

// SearchParams describes a Search call. Empty fields are sent as empty
// strings, which the upstream form expects.
type SearchParams struct {
	StartDate                time.Time
	EndDate                  time.Time
	Chamber                  string // "" | "House" | "Senate" | "Joint"
	Committee                string // committee acronym/name
	BillNumber               string // e.g. "1234"
	SearchType               string // "Schedule" | "Agenda" | "Bill" (default Schedule)
	RequestVerificationToken string // optional captured CSRF token
	ExtraForm                url.Values
}

// Search posts to /Home/Search/ and returns the parsed result rows.
func (c *Client) Search(ctx context.Context, p SearchParams) ([]ScheduleRow, error) {
	form := url.Values{}
	if !p.StartDate.IsZero() {
		form.Set("StartDate", FormatSearchDate(p.StartDate))
	}
	if !p.EndDate.IsZero() {
		form.Set("EndDate", FormatSearchDate(p.EndDate))
	}
	form.Set("Chamber", p.Chamber)
	form.Set("Committee", p.Committee)
	form.Set("BillNumber", p.BillNumber)
	form.Set("SearchType", defaultSearchType(p.SearchType))
	if p.RequestVerificationToken != "" {
		form.Set("__RequestVerificationToken", p.RequestVerificationToken)
	}
	for k, vs := range p.ExtraForm {
		for _, v := range vs {
			form.Add(k, v)
		}
	}

	headers := http.Header{}
	headers.Set("Content-Type", "application/x-www-form-urlencoded")
	headers.Set("Accept", "text/html, */*")
	headers.Set("X-Requested-With", "XMLHttpRequest")

	fetch, err := c.HTTP.Do(ctx, httpx.Request{
		System:   SystemName,
		Endpoint: "Home.Search",
		Method:   http.MethodPost,
		URL:      c.BaseURL + "/Home/Search/",
		Headers:  headers,
		Body:     []byte(form.Encode()),
	})
	if err != nil {
		return nil, err
	}
	return ParseSearchResults(fetch.Body)
}

// GetAgendaModal fetches /Home/Agenda/<id>/?modal=true. Returns raw HTML;
// callers parse what they need from it (links, document URLs, etc.).
func (c *Client) GetAgendaModal(ctx context.Context, agendaID string) ([]byte, error) {
	u := c.BaseURL + "/Home/Agenda/" + url.PathEscape(agendaID) + "/?modal=true"
	fetch, err := c.HTTP.Do(ctx, httpx.Request{
		System:   SystemName,
		Endpoint: "Home.Agenda.modal",
		URL:      u,
		Headers:  htmlAccept(),
	})
	if err != nil {
		return nil, err
	}
	return fetch.Body, nil
}

// GetVideoModal fetches /Home/Video/<id>/?modal=true.
func (c *Client) GetVideoModal(ctx context.Context, videoID string) ([]byte, error) {
	u := c.BaseURL + "/Home/Video/" + url.PathEscape(videoID) + "/?modal=true"
	fetch, err := c.HTTP.Do(ctx, httpx.Request{
		System:   SystemName,
		Endpoint: "Home.Video.modal",
		URL:      u,
		Headers:  htmlAccept(),
	})
	if err != nil {
		return nil, err
	}
	return fetch.Body, nil
}

func defaultSearchType(s string) string {
	if s == "" {
		return "Schedule"
	}
	return s
}

func htmlAccept() http.Header {
	h := http.Header{}
	h.Set("Accept", "text/html, */*")
	return h
}
