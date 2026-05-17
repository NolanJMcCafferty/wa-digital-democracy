package seattle

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

func TestFetchBuildingPermits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/resource/5rc4-5s78.json" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{":id":"row1","permitnum":"7000000-CN"}]`))
	}))
	defer srv.Close()
	c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}}), "")
	c.BaseURL = srv.URL
	rows, err := c.FetchBuildingPermits(context.Background(), socrata.Query{Limit: 1})
	if err != nil || len(rows) != 1 || rows[0]["permitnum"] != "7000000-CN" {
		t.Fatalf("rows=%#v err=%v", rows, err)
	}
}

func TestNormalizePermitAllFieldsAndFallbacks(t *testing.T) {
	p := NormalizePermit(DatasetBuildingPermitMap, map[string]any{
		":id":                 "row1",
		"permit_number":       "7000000-CN",
		"permit_status":       "Issued",
		"original_address":    "600 4TH AVE",
		"project_description": "Alterations",
		"permit_category":     "Construction",
		"type":                "Building",
		"valuation":           "1000",
		"latitude":            "47.6",
		"longitude":           "-122.3",
	})
	if p.SourceRowID != "row1" || p.PermitNumber != "7000000-CN" || p.Status != "Issued" || p.Address != "600 4TH AVE" || p.PermitType != "Building" || p.Raw[":id"] != "row1" {
		t.Fatalf("permit = %#v", p)
	}
}
