package hud

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

func TestFetchDatasetsPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/html,application/xhtml+xml" {
			t.Fatalf("Accept = %q", r.Header.Get("Accept"))
		}
		_, _ = w.Write([]byte("<html>datasets</html>"))
	}))
	defer srv.Close()

	c := New(httpx.New(httpx.Config{}))
	c.DatasetsURL = srv.URL
	body, err := c.FetchDatasetsPage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "<html>datasets</html>" {
		t.Fatalf("body = %q", body)
	}
}

func TestFetchKnownFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/file.csv" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte("a,b\n1,2\n"))
	}))
	defer srv.Close()

	c := New(httpx.New(httpx.Config{}))
	body, err := c.FetchKnownFile(context.Background(), srv.URL+"/file.csv")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "a,b\n1,2\n" {
		t.Fatalf("body = %q", body)
	}
}
