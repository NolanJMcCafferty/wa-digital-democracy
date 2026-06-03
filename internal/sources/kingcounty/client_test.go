package kingcounty

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/socrata"
)

func TestNewConfiguresSocrataClient(t *testing.T) {
	c := New(nil, "tok")
	if c.SystemName != SystemName || c.BaseURL != DefaultBaseURL || c.AppToken != "tok" {
		t.Fatalf("client=%#v", c.Client)
	}
}

func TestFetchParcels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/resource/2kfd-2c3u.json" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{":id":"row1","pin":"123"}]`))
	}))
	defer srv.Close()
	c := New(httpx.New(httpx.Config{}), "")
	c.BaseURL = srv.URL
	rows, err := c.FetchParcels(context.Background(), socrata.Query{Limit: 1})
	if err != nil || len(rows) != 1 || rows[0]["pin"] != "123" {
		t.Fatalf("rows=%#v err=%v", rows, err)
	}
}

func TestNormalizeParcelAllFieldsAndFallbacks(t *testing.T) {
	p := NormalizeParcel(DatasetParcelViewer, map[string]any{
		":id":          "row1",
		"major_minor":  "1234567890",
		"site_address": "1 MAIN ST",
		"city":         "SEATTLE",
		"latitude":     "47.6",
		"longitude":    "-122.3",
	})
	if p.SourceRowID != "row1" || p.PIN != "1234567890" || p.Address != "1 MAIN ST" || p.Jurisdiction != "SEATTLE" || p.Latitude != "47.6" || p.Raw[":id"] != "row1" {
		t.Fatalf("parcel = %#v", p)
	}
}
