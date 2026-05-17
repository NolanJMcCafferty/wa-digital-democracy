package pdc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

func read(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return b
}

func TestParseRows(t *testing.T) {
	rows, err := ParseRows(read(t, "lobbyist-employment.json"))
	if err != nil {
		t.Fatalf("ParseRows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len = %d", len(rows))
	}
	if str(rows[0], "lobbyist_name") != "Doe, Jane" {
		t.Errorf("rows[0].lobbyist_name = %q", str(rows[0], "lobbyist_name"))
	}
}

func TestNormalizeLobbyistEmployment(t *testing.T) {
	rows, _ := ParseRows(read(t, "lobbyist-employment.json"))
	got := NormalizeLobbyistEmployment(rows[0])
	if got.LobbyistName != "Doe, Jane" || got.EmployerName != "Washington Roundtable" {
		t.Errorf("got = %+v", got)
	}
	if got.EmploymentURL != "https://web.pdc.wa.gov/example/1" {
		t.Errorf("EmploymentURL = %q", got.EmploymentURL)
	}
}

func TestNormalizeOrgName(t *testing.T) {
	cases := map[string]string{
		"Washington Roundtable":          "washington roundtable",
		"  ACLU of   WASHINGTON  ":       "aclu of washington",
		"Smith, Doe & Co.":               "smith doe co",
		"AT&T Wireless":                  "att wireless",
		"(Public) Affairs / Group, LLC.": "public affairs group llc",
	}
	for in, want := range cases {
		if got := NormalizeOrgName(in); got != want {
			t.Errorf("NormalizeOrgName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClient_FetchPageWithSourceReturnsRawFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"1"}]`))
	}))
	defer srv.Close()

	sink := &sourceIDSink{id: 42}
	c := &Client{HTTP: httpx.New(httpx.Config{Sink: sink}), BaseURL: srv.URL}
	rows, fetch, err := c.FetchPageWithSource(context.Background(), DatasetContributions, Query{Limit: 1})
	if err != nil {
		t.Fatalf("FetchPageWithSource: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if fetch.SourceRecordID != 42 {
		t.Fatalf("SourceRecordID = %d, want 42", fetch.SourceRecordID)
	}
	if fetch.Endpoint != "resource."+DatasetContributions {
		t.Fatalf("Endpoint = %q", fetch.Endpoint)
	}
}

type sourceIDSink struct{ id int64 }

func (s *sourceIDSink) Record(ctx context.Context, f *httpx.RawFetch) error {
	f.SourceRecordID = s.id
	return nil
}

func TestClient_URLEncoding(t *testing.T) {
	// Capture the request URL the client builds. We use a recorder server
	// because URL-building is private and we want to verify SoQL params.
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	c := &Client{HTTP: httpx.New(httpx.Config{Sink: httpx.NopSink{}}), BaseURL: srv.URL, AppToken: "TKN"}
	_, err := c.FetchPage(context.Background(), DatasetContributions, Query{
		Select: "id,filer_name,amount",
		Where:  "election_year=2026",
		Order:  ":id",
		Limit:  1000,
		Offset: 5000,
	})
	if err != nil {
		t.Fatalf("FetchPage: %v", err)
	}
	u, _ := url.Parse(got)
	q := u.Query()
	check := func(k, want string) {
		if q.Get(k) != want {
			t.Errorf("%s = %q, want %q", k, q.Get(k), want)
		}
	}
	if !strings.HasPrefix(u.Path, "/resource/2jwd-akfb.json") {
		t.Errorf("path = %q", u.Path)
	}
	check("$select", "id,filer_name,amount")
	check("$where", "election_year=2026")
	check("$order", ":id")
	check("$limit", "1000")
	check("$offset", "5000")
	check("$$app_token", "TKN")
}

func TestClient_PageAllStopsOnShortPage(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		offset := r.URL.Query().Get("$offset")
		w.Header().Set("Content-Type", "application/json")
		switch offset {
		case "", "0":
			_, _ = w.Write([]byte(`[{"id":"1"},{"id":"2"}]`))
		default:
			_, _ = w.Write([]byte(`[{"id":"3"}]`)) // short page → stop after this
		}
	}))
	defer srv.Close()

	c := &Client{HTTP: httpx.New(httpx.Config{Sink: httpx.NopSink{}}), BaseURL: srv.URL}
	var ids []string
	err := c.PageAll(context.Background(), "x-y", Query{Limit: 2}, func(r Row) bool {
		ids = append(ids, str(r, "id"))
		return true
	})
	if err != nil {
		t.Fatalf("PageAll: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	if strings.Join(ids, ",") != "1,2,3" {
		t.Fatalf("ids = %v", ids)
	}
}
