package census

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

func TestParseTable(t *testing.T) {
	rows, err := ParseTable([]byte(`[["NAME","DP03_0128PE","state"],["Washington","10.1","53"],["ShortRow"]]`))
	if err != nil || len(rows) != 2 || rows[0]["NAME"] != "Washington" || rows[0]["state"] != "53" || rows[1]["NAME"] != "ShortRow" || rows[1]["state"] != "" {
		t.Fatalf("rows=%#v err=%v", rows, err)
	}
}

func TestParseTableEmptyAndError(t *testing.T) {
	rows, err := ParseTable([]byte(`[]`))
	if err != nil || rows != nil {
		t.Fatalf("empty rows=%#v err=%v", rows, err)
	}
	if _, err := ParseTable([]byte(`not-json`)); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseVariables(t *testing.T) {
	vars, err := ParseVariables([]byte(`{"variables":{"NAME":{"label":"Name","concept":"Geography"},"DP03_0128PE":{"label":"Rent burden","group":"DP03"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if vars["NAME"].Name != "NAME" || vars["DP03_0128PE"].Group != "DP03" {
		t.Fatalf("vars=%#v", vars)
	}
	if _, err := ParseVariables([]byte(`not-json`)); err == nil {
		t.Fatal("expected error")
	}
}

func TestURL(t *testing.T) {
	c := New(nil, "key")
	u, err := c.URL(Query{Year: "2024", Dataset: "acs/acs5/profile", Get: []string{"NAME", "DP03_0128PE"}, For: "tract:*", In: []string{"state:53", "county:033"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"/2024/acs/acs5/profile?", "get=NAME%2CDP03_0128PE", "for=tract%3A%2A", "in=state%3A53", "in=county%3A033", "key=key"} {
		if !strings.Contains(u, want) {
			t.Fatalf("url=%s missing %s", u, want)
		}
	}
}

func TestURLError(t *testing.T) {
	c := New(nil, "")
	if _, err := c.URL(Query{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestFetchAndVariablesUseExpectedEndpoints(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2024/acs/acs5/profile":
			if r.URL.Query().Get("get") != "NAME,DP03_0128PE" || r.Header.Get("Accept") != "application/json" {
				t.Fatalf("query/header = %s %q", r.URL.RawQuery, r.Header.Get("Accept"))
			}
			_, _ = w.Write([]byte(`[["NAME","state"],["Washington","53"]]`))
		case "/2024/acs/acs5/profile/variables.json":
			_, _ = w.Write([]byte(`{"variables":{"NAME":{"label":"Name"}}}`))
		default:
			t.Fatalf("path = %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}}), "")
	c.BaseURL = srv.URL
	rows, err := c.Fetch(context.Background(), Query{Year: "2024", Dataset: "acs/acs5/profile", Get: []string{"NAME", "DP03_0128PE"}, For: "state:53"})
	if err != nil || len(rows) != 1 || rows[0]["NAME"] != "Washington" {
		t.Fatalf("rows=%#v err=%v", rows, err)
	}
	vars, err := c.Variables(context.Background(), "2024", "acs/acs5/profile")
	if err != nil || vars["NAME"].Label != "Name" {
		t.Fatalf("vars=%#v err=%v", vars, err)
	}
}
