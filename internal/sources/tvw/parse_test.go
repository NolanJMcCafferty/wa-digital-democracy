package tvw

import (
	"os"
	"path/filepath"
	"testing"
)

func read(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return b
}

func TestParseSchedule(t *testing.T) {
	events, err := ParseSchedule(read(t, "schedule.json"))
	if err != nil {
		t.Fatalf("ParseSchedule: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len = %d, want 2", len(events))
	}
	if events[0].EventID != "2026041162" {
		t.Errorf("event[0].EventID = %q", events[0].EventID)
	}
	if events[1].EventStatus != "VOD" {
		t.Errorf("event[1].EventStatus = %q", events[1].EventStatus)
	}
}

func TestParseEventDetailWithCaption(t *testing.T) {
	ev, err := ParseEventDetail(read(t, "event-detail.json"))
	if err != nil {
		t.Fatalf("ParseEventDetail: %v", err)
	}
	if ev.EventID != "2025031249" {
		t.Errorf("EventID = %q", ev.EventID)
	}
	if ev.CaptionPath != "https://m-download.invintus.com/9375922947/abc.vtt" {
		t.Errorf("CaptionPath = %q", ev.CaptionPath)
	}
	if len(ev.Categories) != 2 || ev.Categories[1] != "House Housing" {
		t.Errorf("Categories = %v", ev.Categories)
	}
	if got := ParseInvintusStartDateTime(ev.StartDateTime); got.IsZero() {
		t.Errorf("StartDateTime parse failed for %q", ev.StartDateTime)
	}
	if len(ev.MediaAssets) != 2 {
		t.Fatalf("MediaAssets len = %d, want 2", len(ev.MediaAssets))
	}
	if ev.MediaAssets[1].Type != "video" || ev.MediaAssets[1].FileURL == "" {
		t.Errorf("video asset = %+v", ev.MediaAssets[1])
	}
	if string(ev.StreamingURIs) == "{}" {
		t.Errorf("StreamingURIs unexpectedly empty")
	}
}

func TestParseEventDetailNullCaption(t *testing.T) {
	ev, err := ParseEventDetail(read(t, "event-detail-no-caption.json"))
	if err != nil {
		t.Fatalf("ParseEventDetail: %v", err)
	}
	if ev.CaptionPath != "" {
		t.Errorf("CaptionPath should be empty for null, got %q", ev.CaptionPath)
	}
}

func TestParseEventDetailDownloadLinks(t *testing.T) {
	body := []byte(`{
	  "errors": {"hasError": false, "message": null},
	  "data": {
	    "eventID": "2026051107",
	    "clientID": "9375922947",
	    "startDateTime": "2026-05-18 13:30:00",
	    "title": "House Environment & Energy",
	    "captionPath": "https://m-download.invintus.com/9375922947/caption.vtt",
	    "downloadLinks": {
	      "audioDownloadURI": "https://m-download.invintus.com/9375922947/event_audio.mp3",
	      "videoDownloadURI": "https://m-download.invintus.com/9375922947/event.mp4"
	    },
	    "mediaAssets": []
	  },
	  "meta": null
	}`)
	ev, err := ParseEventDetail(body)
	if err != nil {
		t.Fatalf("ParseEventDetail: %v", err)
	}
	if ev.AudioDownloadURI != "https://m-download.invintus.com/9375922947/event_audio.mp3" {
		t.Fatalf("AudioDownloadURI = %q", ev.AudioDownloadURI)
	}
	if ev.VideoDownloadURI != "https://m-download.invintus.com/9375922947/event.mp4" {
		t.Fatalf("VideoDownloadURI = %q", ev.VideoDownloadURI)
	}
	norm := NormalizeFromInvintus(ev, nil)
	if norm.AudioDownloadURL != ev.AudioDownloadURI || norm.VideoDownloadURL != ev.VideoDownloadURI {
		t.Fatalf("normalized download URLs missing: audio=%q video=%q", norm.AudioDownloadURL, norm.VideoDownloadURL)
	}
}

func TestParseWPVideoList_AndEventIDExtract(t *testing.T) {
	posts, err := ParseWPVideoList(read(t, "wp-video-list.json"))
	if err != nil {
		t.Fatalf("ParseWPVideoList: %v", err)
	}
	if len(posts) != 3 {
		t.Fatalf("len = %d, want 3", len(posts))
	}
	cases := []struct {
		idx     int
		eventID string
	}{
		{0, "2026051200"},
		{1, "2026051301"},
		{2, ""}, // no <div data-eventid>
	}
	for _, c := range cases {
		if got := EventIDFromWPPost(posts[c.idx]); got != c.eventID {
			t.Errorf("post[%d] event id = %q, want %q", c.idx, got, c.eventID)
		}
	}
	if CleanTitle(posts[0].Title.Rendered) != "Field Report — Billy Frank Jr. Statue" {
		t.Errorf("CleanTitle = %q", CleanTitle(posts[0].Title.Rendered))
	}
}

func TestParseVTT(t *testing.T) {
	segs, err := ParseVTT(read(t, "sample.vtt"))
	if err != nil {
		t.Fatalf("ParseVTT: %v", err)
	}
	if len(segs) != 3 {
		t.Fatalf("len = %d, want 3", len(segs))
	}
	if segs[0].StartMS != 0 || segs[0].EndMS != 3620 {
		t.Errorf("seg[0] timing = (%d, %d)", segs[0].StartMS, segs[0].EndMS)
	}
	if segs[0].Text != "It looks like we have a small group today so far." {
		t.Errorf("seg[0].Text = %q", segs[0].Text)
	}
	// 01:02:15.500 = 1*3600000 + 2*60000 + 15500 = 3735500
	if segs[2].StartMS != 3735500 || segs[2].EndMS != 3738250 {
		t.Errorf("seg[2] timing = (%d, %d)", segs[2].StartMS, segs[2].EndMS)
	}
	if segs[2].Text != "We will now consider HB 1234." {
		t.Errorf("seg[2].Text = %q", segs[2].Text)
	}
}

func TestParseVTT_MultilineCue(t *testing.T) {
	body := []byte("WEBVTT\n\n00:00.000 --> 00:02.000\nLine one\nLine two\n\n")
	segs, err := ParseVTT(body)
	if err != nil {
		t.Fatalf("ParseVTT: %v", err)
	}
	if len(segs) != 1 || segs[0].Text != "Line one Line two" {
		t.Fatalf("got %+v", segs)
	}
}

func TestNormalizeFromInvintus(t *testing.T) {
	ev, _ := ParseEventDetail(read(t, "event-detail.json"))
	wp := int64(72203)
	got := NormalizeFromInvintus(ev, &wp)
	if got.TVWEventID != "2025031249" {
		t.Errorf("TVWEventID = %q", got.TVWEventID)
	}
	if got.WPPostID == nil || *got.WPPostID != 72203 {
		t.Errorf("WPPostID = %v", got.WPPostID)
	}
	if got.CaptionURL == "" {
		t.Errorf("CaptionURL empty")
	}
	if got.TotalRuntimeSeconds != 5401 {
		t.Errorf("TotalRuntimeSeconds = %d, want 5401", got.TotalRuntimeSeconds)
	}
	if got.PublishedAudioURL == "" || got.VideoDownloadURL == "" {
		t.Errorf("download URLs missing: audio=%q video=%q", got.PublishedAudioURL, got.VideoDownloadURL)
	}
	if got.StartDatetime.IsZero() {
		t.Errorf("StartDatetime zero")
	}

	assets := NormalizeMediaAssets(ev)
	if len(assets) != 2 {
		t.Fatalf("assets len = %d, want 2", len(assets))
	}
	if assets[1].AssetType != "video" || assets[1].FileSizeBytes != 2213283547 || assets[1].TotalRuntimeSeconds != 5401 {
		t.Errorf("normalized video asset = %+v", assets[1])
	}
}
