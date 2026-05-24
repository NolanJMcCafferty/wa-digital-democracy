package tvw

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

func TestFetchScheduleBuildsRequestAndParses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tvw/v1/schedule" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("start"); got != "2026-05-01" {
			t.Fatalf("start = %q", got)
		}
		if got := r.URL.Query().Get("end"); got != "2026-05-02" {
			t.Fatalf("end = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Fatalf("Accept = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(readTVWTestdata(t, "schedule.json"))
	}))
	defer srv.Close()

	c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}, HTTP: srv.Client()}), "")
	c.WPBaseURL = srv.URL

	got, err := c.FetchSchedule(context.Background(), time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("FetchSchedule: %v", err)
	}
	if len(got) != 2 || got[0].EventID != "2026041162" {
		t.Fatalf("unexpected schedule: %+v", got)
	}
}

func TestFetchWPVideoArchivePaginatesAndClamps(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/wp/v2/invintus_video" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("per_page"); got != "100" {
			t.Fatalf("per_page = %q", got)
		}
		if got := r.URL.Query().Get("orderby"); got != "date" {
			t.Fatalf("orderby = %q", got)
		}
		if calls == 1 {
			_, _ = w.Write(readTVWTestdata(t, "wp-video-list.json"))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}, HTTP: srv.Client()}), "")
	c.WPBaseURL = srv.URL

	posts, err := c.FetchWPVideoArchive(context.Background(), time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC), 2)
	if err != nil {
		t.Fatalf("FetchWPVideoArchive: %v", err)
	}
	if len(posts) != 2 {
		t.Fatalf("len(posts) = %d, want clamp to 2", len(posts))
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 because max reached", calls)
	}
}

func TestFetchWPVideoArchiveStopsOnPaginationOver(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"code":"rest_post_invalid_page_number"}`, http.StatusBadRequest)
	}))
	defer srv.Close()

	c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}, HTTP: srv.Client(), RetryOn: []int{500}}), "")
	c.WPBaseURL = srv.URL

	posts, err := c.FetchWPVideoArchive(context.Background(), time.Now().Add(-24*time.Hour), time.Now(), 10)
	if err != nil {
		t.Fatalf("FetchWPVideoArchive: %v", err)
	}
	if len(posts) != 0 {
		t.Fatalf("len(posts) = %d, want 0", len(posts))
	}
}

func TestFetchWPVideoByEventIDFindsMatchingPost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("search"); got != "2026051301" {
			t.Fatalf("search = %q", got)
		}
		_, _ = w.Write(readTVWTestdata(t, "wp-video-list.json"))
	}))
	defer srv.Close()

	c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}, HTTP: srv.Client()}), "")
	c.WPBaseURL = srv.URL

	post, fetch, err := c.FetchWPVideoByEventID(context.Background(), "2026051301")
	if err != nil {
		t.Fatalf("FetchWPVideoByEventID: %v", err)
	}
	if post == nil || post.Slug != "house-housing-2026051301" {
		t.Fatalf("post = %+v", post)
	}
	if fetch.Endpoint != "wp.v2.invintus_video.search_event" {
		t.Fatalf("Endpoint = %q", fetch.Endpoint)
	}
}

func TestFetchCaptionsParsesAndAnnotates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/caption.vtt" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write(readTVWTestdata(t, "sample.vtt"))
	}))
	defer srv.Close()

	c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}, HTTP: srv.Client()}), "")
	segments, err := c.FetchCaptions(context.Background(), "2026051301", srv.URL+"/caption.vtt")
	if err != nil {
		t.Fatalf("FetchCaptions: %v", err)
	}
	if len(segments) != 3 {
		t.Fatalf("len(segments) = %d, want 3", len(segments))
	}
	if segments[0].TVWEventID != "2026051301" || segments[0].SourceCaptionURL != srv.URL+"/caption.vtt" {
		t.Fatalf("annotations missing: %+v", segments[0])
	}
}

func TestFetchCaptionsEmptyURL(t *testing.T) {
	c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}}), "")
	segments, err := c.FetchCaptions(context.Background(), "event", "")
	if err != nil {
		t.Fatalf("FetchCaptions empty: %v", err)
	}
	if segments != nil {
		t.Fatalf("segments = %#v, want nil", segments)
	}
}

func TestJSONAcceptAndPaginationOverHelpers(t *testing.T) {
	if got := jsonAccept().Get("Accept"); got != "application/json" {
		t.Fatalf("jsonAccept Accept = %q", got)
	}
	if !isRestPaginationOver(assertErr("status 400: anything")) {
		t.Fatal("expected status 400 to be treated as pagination over")
	}
	if isRestPaginationOver(nil) || isRestPaginationOver(assertErr("status 500")) {
		t.Fatal("unexpected pagination-over result")
	}
	if got := CleanTitle(" Field Report &#8212; Test "); !strings.Contains(got, "—") {
		t.Fatalf("CleanTitle = %q", got)
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }

func readTVWTestdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return b
}
