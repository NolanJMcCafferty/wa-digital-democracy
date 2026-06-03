package socrata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

func TestResourceURL(t *testing.T) {
	c := New(httpx.New(httpx.Config{}), "test", "https://data.example.gov", "tok")
	u, err := c.ResourceURL("abcd-1234", Query{Select: "count(*)", Where: "name='x'", Order: ":id", Limit: 10, Offset: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"https://data.example.gov/resource/abcd-1234.json?",
		"%24select=count%28%2A%29",
		"%24where=name%3D%27x%27",
		"%24%24app_token=tok",
		"%24limit=10",
		"%24offset=20",
	} {
		if !strings.Contains(u, want) {
			t.Fatalf("ResourceURL = %s, missing %s", u, want)
		}
	}
}

func TestResourceURLErrorWithoutDataset(t *testing.T) {
	c := New(nil, "test", "https://data.example.gov", "")
	if _, err := c.ResourceURL("", Query{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestFetchPageRecordsRequestShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/resource/abcd-1234.json" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("$limit") != "1" {
			t.Fatalf("query = %s", r.URL.RawQuery)
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Fatalf("Accept = %q", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"A"}]`))
	}))
	defer srv.Close()

	c := New(httpx.New(httpx.Config{}), "test_socrata", srv.URL, "")
	rows, err := c.FetchPage(context.Background(), "abcd-1234", Query{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["name"] != "A" {
		t.Fatalf("rows = %#v", rows)
	}
}

func TestPageAllPaginatesAndStops(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 && r.URL.Query().Get("$offset") != "" {
			t.Fatalf("first offset = %q", r.URL.Query().Get("$offset"))
		}
		if calls == 2 && r.URL.Query().Get("$offset") != "2" {
			t.Fatalf("second offset = %q", r.URL.Query().Get("$offset"))
		}
		if calls == 1 {
			_, _ = w.Write([]byte(`[{"id":"1"},{"id":"2"}]`))
			return
		}
		_, _ = w.Write([]byte(`[{"id":"3"}]`))
	}))
	defer srv.Close()

	c := New(httpx.New(httpx.Config{}), "test_socrata", srv.URL, "")
	var ids []string
	if err := c.PageAll(context.Background(), "abcd-1234", Query{Limit: 2}, func(r Row) bool {
		ids = append(ids, r["id"].(string))
		return true
	}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || strings.Join(ids, ",") != "1,2,3" {
		t.Fatalf("calls=%d ids=%v", calls, ids)
	}
}

func TestCountParsesStringAndFloat(t *testing.T) {
	for name, body := range map[string]string{
		"string": `[{"count":"42"}]`,
		"float":  `[{"count":42}]`,
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer srv.Close()
			c := New(httpx.New(httpx.Config{}), "test_socrata", srv.URL, "")
			got, err := c.Count(context.Background(), "abcd-1234", "")
			if err != nil || got != 42 {
				t.Fatalf("Count = %d, %v", got, err)
			}
		})
	}
}

func TestMetadataAndCatalog(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/views/abcd-1234":
			_, _ = w.Write([]byte(`{"id":"abcd-1234","name":"Dataset","domain":"data.example.gov","webUri":"https://x","dataUri":"https://y"}`))
		case "/api/views/metadata/v1":
			_, _ = w.Write([]byte(`[{"id":"efgh-5678","name":"Other"}]`))
		default:
			t.Fatalf("path = %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	c := New(httpx.New(httpx.Config{}), "test_socrata", srv.URL, "")
	m, err := c.Metadata(context.Background(), "abcd-1234")
	if err != nil || m.ID != "abcd-1234" || m.Raw["name"] != "Dataset" {
		t.Fatalf("metadata = %#v err=%v", m, err)
	}
	cat, err := c.Catalog(context.Background())
	if err != nil || len(cat) != 1 || cat[0].ID != "efgh-5678" {
		t.Fatalf("catalog = %#v err=%v", cat, err)
	}
}

func TestParseRowsErrors(t *testing.T) {
	if _, err := ParseRows([]byte(`not-json`)); err == nil {
		t.Fatal("expected row parse error")
	}
	if _, err := ParseMetadata([]byte(`not-json`)); err == nil {
		t.Fatal("expected metadata parse error")
	}
	if _, err := ParseCatalog([]byte(`not-json`)); err == nil {
		t.Fatal("expected catalog parse error")
	}
}
