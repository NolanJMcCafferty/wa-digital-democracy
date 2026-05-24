package committeeschedules

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

func TestNewDefaultsAndHelpers(t *testing.T) {
	c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}}))
	if c.BaseURL != DefaultBaseURL {
		t.Fatalf("BaseURL = %q", c.BaseURL)
	}
	if got := FormatSearchDate(time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC)); got != "05242026" {
		t.Fatalf("FormatSearchDate = %q", got)
	}
	if got := defaultSearchType(""); got != "Schedule" {
		t.Fatalf("defaultSearchType empty = %q", got)
	}
	if got := defaultSearchType("Agenda"); got != "Agenda" {
		t.Fatalf("defaultSearchType Agenda = %q", got)
	}
	if got := htmlAccept().Get("Accept"); got != "text/html, */*" {
		t.Fatalf("htmlAccept = %q", got)
	}
}

func TestSearchPostsExpectedFormAndParsesRows(t *testing.T) {
	var sawSearch bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Home/Search/" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %q", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Fatalf("Content-Type = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "text/html, */*" {
			t.Fatalf("Accept = %q", got)
		}
		if got := r.Header.Get("X-Requested-With"); got != "XMLHttpRequest" {
			t.Fatalf("X-Requested-With = %q", got)
		}
		body := readFormBody(t, r)
		checkFormValue(t, body, "StartDate", "05242026")
		checkFormValue(t, body, "EndDate", "05252026")
		checkFormValue(t, body, "Chamber", "House")
		checkFormValue(t, body, "Committee", "Housing")
		checkFormValue(t, body, "BillNumber", "9001")
		checkFormValue(t, body, "SearchType", "Schedule")
		checkFormValue(t, body, "__RequestVerificationToken", "csrf-token")
		checkFormValue(t, body, "Extra", "one")
		checkFormValue(t, body, "Extra", "two")
		sawSearch = true
		_, _ = w.Write([]byte(`showAgendaDetailModal(1)
showVideoModal(1, 100)`))
	}))
	defer srv.Close()

	c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}, HTTP: srv.Client()}))
	c.BaseURL = srv.URL
	rows, err := c.Search(context.Background(), SearchParams{
		StartDate:                time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC),
		EndDate:                  time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC),
		Chamber:                  "House",
		Committee:                "Housing",
		BillNumber:               "9001",
		RequestVerificationToken: "csrf-token",
		ExtraForm:                url.Values{"Extra": []string{"one", "two"}},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !sawSearch {
		t.Fatal("server did not see search")
	}
	if len(rows) != 1 || rows[0].AgendaID != "1" || rows[0].TVWEventID != "100" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestSearchUsesExplicitSearchTypeAndPropagatesErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := readFormBody(t, r)
		checkFormValue(t, body, "SearchType", "Bill")
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}, HTTP: srv.Client(), RetryOn: []int{429}}))
	c.BaseURL = srv.URL
	_, err := c.Search(context.Background(), SearchParams{SearchType: "Bill"})
	if err == nil {
		t.Fatal("Search succeeded, want error")
	}
}

func TestGetAgendaAndVideoModals(t *testing.T) {
	seen := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "text/html, */*" {
			t.Fatalf("Accept = %q", got)
		}
		seen[r.URL.String()] = true
		switch r.URL.String() {
		case "/Home/Agenda/agenda%20id/?modal=true":
			_, _ = w.Write([]byte(`<div>agenda modal</div>`))
		case "/Home/Video/video%2Fid/?modal=true":
			_, _ = w.Write([]byte(`<div>video modal</div>`))
		default:
			t.Fatalf("unexpected URL = %q", r.URL.String())
		}
	}))
	defer srv.Close()

	c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}, HTTP: srv.Client()}))
	c.BaseURL = srv.URL
	agenda, err := c.GetAgendaModal(context.Background(), "agenda id")
	if err != nil {
		t.Fatalf("GetAgendaModal: %v", err)
	}
	video, err := c.GetVideoModal(context.Background(), "video/id")
	if err != nil {
		t.Fatalf("GetVideoModal: %v", err)
	}
	if !strings.Contains(string(agenda), "agenda modal") || !strings.Contains(string(video), "video modal") {
		t.Fatalf("agenda=%s video=%s", agenda, video)
	}
	if !seen["/Home/Agenda/agenda%20id/?modal=true"] || !seen["/Home/Video/video%2Fid/?modal=true"] {
		t.Fatalf("seen = %+v", seen)
	}
}

func readFormBody(t *testing.T, r *http.Request) url.Values {
	t.Helper()
	if err := r.ParseForm(); err != nil {
		t.Fatalf("ParseForm: %v", err)
	}
	return r.PostForm
}

func checkFormValue(t *testing.T, form url.Values, key, want string) {
	t.Helper()
	for _, got := range form[key] {
		if got == want {
			return
		}
	}
	t.Fatalf("form[%s] = %q, want value %q", key, form[key], want)
}
