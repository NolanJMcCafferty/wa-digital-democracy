package csi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

func TestClientDefaultsAndValidation(t *testing.T) {
	c := New(httpx.New(httpx.Config{}))
	if c.BaseURL != DefaultBaseURL {
		t.Fatalf("BaseURL = %q", c.BaseURL)
	}
	if err := validateChamber("House"); err != nil {
		t.Fatalf("validateChamber(House): %v", err)
	}
	if err := validateChamber("house"); err == nil {
		t.Fatal("validateChamber(house) succeeded, want error")
	}
}

func TestListCommitteesFetchesAndAnnotatesChamber(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/House" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "text/html,application/xhtml+xml" {
			t.Fatalf("Accept = %q", got)
		}
		_, _ = w.Write(readCSITestdata(t, "chamber-house.html"))
	}))
	defer srv.Close()

	c := New(httpx.New(httpx.Config{HTTP: srv.Client()}))
	c.BaseURL = srv.URL
	committees, err := c.ListCommittees(context.Background(), "House")
	if err != nil {
		t.Fatalf("ListCommittees: %v", err)
	}
	if len(committees) != 4 {
		t.Fatalf("len = %d, want 4", len(committees))
	}
	if committees[0].Chamber != "House" {
		t.Fatalf("Chamber = %q", committees[0].Chamber)
	}
}

func TestListMeetingsBuildsAjaxJSONRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Home/GetMeetings" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("chamber") != "House" || q.Get("committeeId") != "31633" {
			t.Fatalf("query = %s", r.URL.RawQuery)
		}
		if got := r.Header.Get("X-Requested-With"); got != "XMLHttpRequest" {
			t.Fatalf("X-Requested-With = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json, text/javascript, */*; q=0.01" {
			t.Fatalf("Accept = %q", got)
		}
		_, _ = w.Write(readCSITestdata(t, "get-meetings.json"))
	}))
	defer srv.Close()

	c := New(httpx.New(httpx.Config{HTTP: srv.Client()}))
	c.BaseURL = srv.URL
	meetings, err := c.ListMeetings(context.Background(), "House", "31633")
	if err != nil {
		t.Fatalf("ListMeetings: %v", err)
	}
	if len(meetings) != 3 || meetings[0].MeetingFamilyID != "34109" {
		t.Fatalf("meetings = %+v", meetings)
	}
}

func TestListAgendaItemsBuildsAjaxHTMLRequestAndAnnotatesChamber(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Home/GetAgendaItems" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("chamber") != "Senate" || q.Get("meetingFamilyId") != "34109" {
			t.Fatalf("query = %s", r.URL.RawQuery)
		}
		if got := r.Header.Get("Accept"); got != "text/html, */*" {
			t.Fatalf("Accept = %q", got)
		}
		_, _ = w.Write(readCSITestdata(t, "get-agenda-items.html"))
	}))
	defer srv.Close()

	c := New(httpx.New(httpx.Config{HTTP: srv.Client()}))
	c.BaseURL = srv.URL
	items, err := c.ListAgendaItems(context.Background(), "Senate", "34109")
	if err != nil {
		t.Fatalf("ListAgendaItems: %v", err)
	}
	if len(items) != 2 || items[0].Chamber != "Senate" {
		t.Fatalf("items = %+v", items)
	}
}

func TestGetTestifiersBuildsRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Home/GetOtherTestifiers" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("agendaItemId") != "28599" || q.Get("agendaItemDescription") != "HB 1234" {
			t.Fatalf("query = %s", r.URL.RawQuery)
		}
		if got := r.Header.Get("X-Requested-With"); got != "XMLHttpRequest" {
			t.Fatalf("X-Requested-With = %q", got)
		}
		_, _ = w.Write(readCSITestdata(t, "get-other-testifiers.html"))
	}))
	defer srv.Close()

	c := New(httpx.New(httpx.Config{HTTP: srv.Client()}))
	c.BaseURL = srv.URL
	rows, err := c.GetTestifiers(context.Background(), "28599", "HB 1234")
	if err != nil {
		t.Fatalf("GetTestifiers: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3", len(rows))
	}
}

func TestClientRequiredArgumentErrors(t *testing.T) {
	c := New(httpx.New(httpx.Config{}))
	if _, err := c.ListMeetings(context.Background(), "House", ""); err == nil {
		t.Fatal("ListMeetings missing committeeID succeeded")
	}
	if _, err := c.ListAgendaItems(context.Background(), "House", ""); err == nil {
		t.Fatal("ListAgendaItems missing meetingFamilyID succeeded")
	}
	if _, err := c.GetTestifiers(context.Background(), "", "desc"); err == nil {
		t.Fatal("GetTestifiers missing agendaItemID succeeded")
	}
}

func readCSITestdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return b
}
