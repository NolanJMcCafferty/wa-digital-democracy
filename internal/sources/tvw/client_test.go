package tvw

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

func TestFetchEventDetailWithSourceUsesPlayerDownloadRequest(t *testing.T) {
	const eventID = "2026051107"
	const playerURL = "https://tvw.org/video/house-environment-energy-2026051107/?eventID=2026051107"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Event/getDetailed" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %q", r.Method)
		}
		if r.Header.Get("authorization") != defaultEmbedderTag {
			t.Fatalf("authorization = %q", r.Header.Get("authorization"))
		}
		if r.Header.Get("wsc-api-key") != "test-key" {
			t.Fatalf("wsc-api-key = %q", r.Header.Get("wsc-api-key"))
		}
		if r.Header.Get("Origin") != "https://tvw.org" {
			t.Fatalf("Origin = %q", r.Header.Get("Origin"))
		}
		if r.Header.Get("Referer") != "https://tvw.org/" {
			t.Fatalf("Referer = %q", r.Header.Get("Referer"))
		}
		if r.Header.Get("X-Referer") != playerURL {
			t.Fatalf("X-Referer = %q", r.Header.Get("X-Referer"))
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body["eventID"] != eventID || body["clientID"] != defaultClientID {
			t.Fatalf("bad request identity fields: %#v", body)
		}
		for _, key := range []string{"showEncoder", "showStreams", "VAST", "checkRecentBreak", "showDownloadLinks", "showDocumentAssets"} {
			if body[key] != true {
				t.Fatalf("%s = %#v, want true", key, body[key])
			}
		}
		if body["includePrivate"] != false {
			t.Fatalf("includePrivate = %#v, want false", body["includePrivate"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "errors": {"hasError": false, "message": null},
		  "data": {
		    "eventID": "2026051107",
		    "clientID": "9375922947",
		    "downloadLinks": {
		      "audioDownloadURI": "https://m-download.invintus.com/9375922947/event_audio.mp3",
		      "videoDownloadURI": "https://m-download.invintus.com/9375922947/event.mp4"
		    }
		  },
		  "meta": null
		}`))
	}))
	defer srv.Close()

	c := New(httpx.New(httpx.Config{HTTP: srv.Client()}), "test-key")
	c.InvintusBaseURL = srv.URL

	ev, _, err := c.FetchEventDetailWithSource(context.Background(), eventID, playerURL)
	if err != nil {
		t.Fatalf("FetchEventDetailWithSource: %v", err)
	}
	if ev.AudioDownloadURI == "" || ev.VideoDownloadURI == "" {
		t.Fatalf("download links not parsed: audio=%q video=%q", ev.AudioDownloadURI, ev.VideoDownloadURI)
	}
}
