package bls

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

func TestParseTimeSeries(t *testing.T) {
	got, err := ParseTimeSeries([]byte(`{"status":"REQUEST_SUCCEEDED","responseTime":1,"message":[],"Results":{"series":[{"seriesID":"CUUR0000SA0","data":[{"year":"2026","period":"M01","periodName":"January","value":"320.1","footnotes":[{"code":"P","text":"preliminary"}]}]}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "REQUEST_SUCCEEDED" || got.ResponseTime != 1 || len(got.Results.Series) != 1 || got.Results.Series[0].Data[0].Footnotes[0].Code != "P" {
		t.Fatalf("got=%#v", got)
	}
}

func TestParseTimeSeriesError(t *testing.T) {
	if _, err := ParseTimeSeries([]byte(`not-json`)); err == nil {
		t.Fatal("expected error")
	}
}

func TestTimeSeriesPostsRequestAndAddsRegistrationKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/timeseries/data/" {
			t.Fatalf("method/path = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Accept") != "application/json" {
			t.Fatalf("headers = %q/%q", r.Header.Get("Content-Type"), r.Header.Get("Accept"))
		}
		var req TimeSeriesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.RegistrationKey != "reg-key" || len(req.SeriesID) != 1 || req.SeriesID[0] != "CUUR0000SA0" {
			t.Fatalf("request = %#v", req)
		}
		_, _ = w.Write([]byte(`{"status":"REQUEST_SUCCEEDED","Results":{"series":[]}}`))
	}))
	defer srv.Close()

	c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}}), "reg-key")
	c.BaseURL = srv.URL
	got, err := c.TimeSeries(context.Background(), TimeSeriesRequest{SeriesID: []string{"CUUR0000SA0"}, StartYear: "2026", EndYear: "2026"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "REQUEST_SUCCEEDED" {
		t.Fatalf("got=%#v", got)
	}
}
