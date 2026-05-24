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

func TestNormalizeLobbyistCompensationAndContribution(t *testing.T) {
	comp := NormalizeLobbyistCompensation(Row{
		"filer_id": "L123", "filer_name": "Doe, Jane", "funding_source_id": 456.0, "funding_source": "Roundtable",
		"filing_period": "2026", "employer_id": "E1", "employer_name": "Washington Roundtable",
		"compensation": "123.45", "total_expenses": 67.0, "net_total": nil, "url": "https://example.test/comp",
	})
	if comp.FilerID != "L123" || comp.FundingSourceID != "456" || comp.Compensation != 123.45 || comp.TotalExpenses != 67 || comp.NetTotal != 0 {
		t.Fatalf("comp = %+v", comp)
	}

	contrib := NormalizeContribution(Row{
		"id": "C1", "filer_id": "F1", "filer_name": "Candidate", "office": "LEG", "legislative_district": "43",
		"party": true, "election_year": "2026", "amount": "250.50", "cash_or_in_kind": "Cash",
		"receipt_date": "2026-01-02", "contributor_name": "Donor", "contributor_category": "Individual", "url": "https://example.test/contrib",
	})
	if contrib.ID != "C1" || contrib.Party != "true" || contrib.Amount != 250.50 || contrib.ContributorName != "Donor" {
		t.Fatalf("contrib = %+v", contrib)
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

func TestClient_NewDefaults(t *testing.T) {
	c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}}), "TKN")
	if c.BaseURL != DefaultBaseURL || c.AppToken != "TKN" {
		t.Fatalf("client = %+v", c)
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

func TestClient_CountAndMetadata(t *testing.T) {
	var sawCount, sawMetadata bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/resource/" + DatasetContributions + ".json":
			sawCount = true
			q := r.URL.Query()
			if q.Get("$select") != "count(*)" || q.Get("$where") != "election_year=2026" || q.Get("$limit") != "1" {
				t.Fatalf("count query = %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`[{"count":"42"}]`))
		case "/api/views/" + DatasetContributions:
			sawMetadata = true
			if r.URL.Query().Get("$$app_token") != "TKN" {
				t.Fatalf("metadata app token = %q", r.URL.Query().Get("$$app_token"))
			}
			_, _ = w.Write([]byte(`{"id":"2jwd-akfb"}`))
		default:
			t.Fatalf("unexpected path = %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := &Client{HTTP: httpx.New(httpx.Config{Sink: httpx.NopSink{}, HTTP: srv.Client()}), BaseURL: srv.URL, AppToken: "TKN"}
	count, err := c.Count(context.Background(), DatasetContributions, "election_year=2026")
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 42 {
		t.Fatalf("count = %d, want 42", count)
	}
	body, err := c.Metadata(context.Background(), DatasetContributions)
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if !strings.Contains(string(body), `"2jwd-akfb"`) {
		t.Fatalf("metadata body = %s", body)
	}
	if !sawCount || !sawMetadata {
		t.Fatalf("sawCount=%v sawMetadata=%v", sawCount, sawMetadata)
	}
}

func TestClient_CountHandlesNumericEmptyAndUnexpectedTypes(t *testing.T) {
	responses := [][]byte{
		[]byte(`[{"count":7}]`),
		[]byte(`[]`),
		[]byte(`[{"count":true}]`),
	}
	wantCounts := []int64{7, 0, 0}
	wantErr := []bool{false, false, true}
	for i, body := range responses {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(body) }))
		c := &Client{HTTP: httpx.New(httpx.Config{Sink: httpx.NopSink{}, HTTP: srv.Client()}), BaseURL: srv.URL}
		got, err := c.Count(context.Background(), DatasetContributions, "")
		srv.Close()
		if (err != nil) != wantErr[i] {
			t.Fatalf("case %d err = %v, wantErr %v", i, err, wantErr[i])
		}
		if got != wantCounts[i] {
			t.Fatalf("case %d count = %d, want %d", i, got, wantCounts[i])
		}
	}
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
