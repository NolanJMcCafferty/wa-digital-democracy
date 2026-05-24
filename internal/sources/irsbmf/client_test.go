package irsbmf

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

const sampleCSV = `EIN,NAME,ICO,STREET,CITY,STATE,ZIP,GROUP,SUBSECTION,AFFILIATION,CLASSIFICATION,RULING,DEDUCTIBILITY,FOUNDATION,ACTIVITY,ORGANIZATION,STATUS,TAX_PERIOD,ASSET_CD,INCOME_CD,FILING_REQ_CD,PF_FILING_REQ_CD,ACCT_PD,ASSET_AMT,INCOME_AMT,REVENUE_AMT,NTEE_CD,SORT_NAME
123456789,ACLU OF WASHINGTON,,PO BOX 1,SEATTLE,WA,98101,0000,03,3,1000,197001,1,15,000000000,1,01,202412,5,4,01,0,12,12345,67890,111213,R60,ACLU WA
,NO EIN,,,,,,,,,,,,,,,,,,,,,,,,,,
987654321,WASHINGTON ROUNDTABLE,,PO BOX 2,OLYMPIA,WA,98501,0000,03,3,1000,198001,1,15,000000000,1,01,202412,not-num,4,01,0,12,not-num,42,,S41,ROUND TABLE,EXTRA
`

func TestNewDefaults(t *testing.T) {
	c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}}))
	if c.BaseURL != DefaultStateURL {
		t.Fatalf("BaseURL = %q", c.BaseURL)
	}
}

func TestFetchDownloadsCSVWithSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/eo_wa.csv" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "text/csv" {
			t.Fatalf("Accept = %q", got)
		}
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte(sampleCSV))
	}))
	defer srv.Close()

	c := New(httpx.New(httpx.Config{Sink: &sourceIDSink{id: 123}, HTTP: srv.Client()}))
	c.BaseURL = srv.URL + "/eo_wa.csv"
	body, fetch, err := c.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(body) != sampleCSV {
		t.Fatalf("body mismatch")
	}
	if fetch.SourceRecordID != 123 || fetch.System != SystemName || fetch.Endpoint != "eo_wa.csv" {
		t.Fatalf("fetch = %+v", fetch)
	}
}

func TestFetchReturnsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}, HTTP: srv.Client(), RetryOn: []int{500}}))
	c.BaseURL = srv.URL
	_, fetch, err := c.Fetch(context.Background())
	if err == nil {
		t.Fatal("Fetch succeeded, want error")
	}
	if fetch.Status != http.StatusNotFound {
		t.Fatalf("status = %d", fetch.Status)
	}
}

func TestParseAllDecodesRowsSkipsMissingEINAndStopsEarly(t *testing.T) {
	var rows []Row
	err := ParseAll([]byte(sampleCSV), func(r Row) bool {
		rows = append(rows, r)
		return len(rows) < 1
	})
	if err != nil {
		t.Fatalf("ParseAll: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want early stop after 1", len(rows))
	}
	row := rows[0]
	if row.EIN != "123456789" || row.Name != "ACLU OF WASHINGTON" || row.City != "SEATTLE" || row.State != "WA" {
		t.Fatalf("row identity = %+v", row)
	}
	if row.AssetAmount != 12345 || row.IncomeAmount != 67890 || row.RevenueAmount != 111213 {
		t.Fatalf("amounts = %+v", row)
	}
	if row.Raw["NAME"] != "ACLU OF WASHINGTON" || row.Raw["ASSET_AMT"] != "12345" {
		t.Fatalf("raw = %+v", row.Raw)
	}
}

func TestParseAllToleratesLongShortAndBadNumericRows(t *testing.T) {
	var rows []Row
	if err := ParseAll([]byte(sampleCSV), func(r Row) bool { rows = append(rows, r); return true }); err != nil {
		t.Fatalf("ParseAll: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2 non-empty EIN rows", len(rows))
	}
	bad := rows[1]
	if bad.EIN != "987654321" || bad.AssetAmount != 0 || bad.IncomeAmount != 42 || bad.RevenueAmount != 0 {
		t.Fatalf("bad numeric row = %+v", bad)
	}
}

func TestParseAllEmptyCSV(t *testing.T) {
	called := false
	if err := ParseAll(nil, func(Row) bool { called = true; return true }); err != nil {
		t.Fatalf("ParseAll empty: %v", err)
	}
	if called {
		t.Fatal("yield called for empty CSV")
	}
}

func TestDecodeMissingColumnsAndAtoi64(t *testing.T) {
	row := decode([]string{"111", " Trimmed Name "}, map[string]int{"EIN": 0, "NAME": 1, "MISSING": 3})
	if row.EIN != "111" || row.Name != "Trimmed Name" || row.State != "" {
		t.Fatalf("row = %+v", row)
	}
	if !reflect.DeepEqual(row.Raw, map[string]string{"EIN": "111", "NAME": "Trimmed Name"}) {
		t.Fatalf("raw = %+v", row.Raw)
	}
	cases := map[string]int64{"": 0, "123": 123, "-5": -5, "oops": 0}
	for in, want := range cases {
		if got := atoi64(in); got != want {
			t.Fatalf("atoi64(%q) = %d, want %d", in, got, want)
		}
	}
}

type sourceIDSink struct{ id int64 }

func (s *sourceIDSink) Record(ctx context.Context, f *httpx.RawFetch) error {
	f.SourceRecordID = s.id
	return nil
}
