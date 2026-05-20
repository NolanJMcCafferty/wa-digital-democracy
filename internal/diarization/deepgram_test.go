package diarization

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeepgramProviderDiarizeURL(t *testing.T) {
	var sawAuth, sawDiarize, sawURL bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization") == "Token test-key"
		sawDiarize = r.URL.Query().Get("diarize") == "true" && r.URL.Query().Get("utterances") == "true" && r.URL.Query().Get("detect_entities") == "true"
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		sawURL = body["url"] == "https://example.com/audio.mp3"
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"results": {
				"utterances": [
					{"start": 1.2, "end": 3.4, "speaker": 0, "transcript": "hello there", "confidence": 0.91},
					{"start": 4.0, "end": 5.0, "speaker": 1, "transcript": "thank you", "confidence": 0.88}
				],
				"channels": [{"alternatives": [{"words": [
					{"word": "hello", "start": 1.2, "end": 1.5, "speaker": 0, "speaker_confidence": 0.9},
					{"word": "there", "start": 1.6, "end": 3.4, "speaker": 0, "speaker_confidence": 0.92}
				], "entities": [
					{"label": "PERSON", "value": "Jane Doe", "confidence": 0.77, "start_word": 0, "end_word": 2}
				]}]}]
			}
		}`))
	}))
	defer srv.Close()

	p, err := NewDeepgramProvider(DeepgramConfig{APIKey: "test-key", Endpoint: srv.URL, Model: "nova-test"})
	if err != nil {
		t.Fatalf("NewDeepgramProvider: %v", err)
	}
	res, err := p.Diarize(context.Background(), AudioInput{URL: "https://example.com/audio.mp3", EventID: "evt-1"})
	if err != nil {
		t.Fatalf("Diarize: %v", err)
	}
	if !sawAuth || !sawDiarize || !sawURL {
		t.Fatalf("request not formed correctly: auth=%v diarize=%v url=%v", sawAuth, sawDiarize, sawURL)
	}
	if res.Provider != "deepgram" || res.Model != "nova-test" || res.EventID != "evt-1" {
		t.Fatalf("metadata = %#v", res)
	}
	if len(res.Segments) != 2 {
		t.Fatalf("segments len = %d, want 2", len(res.Segments))
	}
	if got := res.Segments[0]; got.StartMS != 1200 || got.EndMS != 3400 || got.SpeakerCluster != "SPEAKER_00" || got.Text != "hello there" {
		t.Fatalf("segment[0] = %#v", got)
	}
	if len(res.Words) != 2 || res.Words[1].SpeakerCluster != "SPEAKER_00" {
		t.Fatalf("words = %#v", res.Words)
	}
	if len(res.Entities) != 1 || res.Entities[0].Text != "Jane Doe" || res.Entities[0].Type != "PERSON" {
		t.Fatalf("entities = %#v", res.Entities)
	}
	if res.Entities[0].StartMS == nil || *res.Entities[0].StartMS != 1200 {
		t.Fatalf("entity timing = %#v", res.Entities[0])
	}
}

func TestDeepgramProviderFallsBackToWordSegments(t *testing.T) {
	raw := []byte(`{"results":{"channels":[{"alternatives":[{"words":[
		{"word":"one","start":0,"end":0.5,"speaker":0},
		{"word":"two","start":0.6,"end":1.0,"speaker":0},
		{"word":"three","start":1.1,"end":1.5,"speaker":1}
	]}]}]}}`)
	res, err := parseDeepgram(raw)
	if err != nil {
		t.Fatalf("parseDeepgram: %v", err)
	}
	if len(res.Segments) != 2 {
		t.Fatalf("segments len = %d, want 2: %#v", len(res.Segments), res.Segments)
	}
	if res.Segments[0].Text != "one two" || res.Segments[1].SpeakerCluster != "SPEAKER_01" {
		t.Fatalf("segments = %#v", res.Segments)
	}
}

func TestMergeConsecutiveSegments(t *testing.T) {
	conf := func(v float64) *float64 { return &v }
	in := []Segment{
		{StartMS: 0, EndMS: 1000, SpeakerCluster: "SPEAKER_00", Text: "We'll", Confidence: conf(0.9)},
		{StartMS: 1000, EndMS: 7000, SpeakerCluster: "SPEAKER_00", Text: "call to order today's meeting.", Confidence: conf(0.8)},
		{StartMS: 7000, EndMS: 8000, SpeakerCluster: "SPEAKER_00", Text: "twenty twenty six.", Confidence: conf(0.7)},
		{StartMS: 8000, EndMS: 9000, SpeakerCluster: "SPEAKER_01", Text: "Thank you.", Confidence: conf(0.95)},
		{StartMS: 9000, EndMS: 10000, SpeakerCluster: "SPEAKER_00", Text: "Welcome.", Confidence: conf(0.6)},
	}
	got := MergeConsecutiveSegments(in)
	if len(got) != 3 {
		t.Fatalf("len=%d, want 3: %#v", len(got), got)
	}
	if got[0].StartMS != 0 || got[0].EndMS != 8000 || got[0].SpeakerCluster != "SPEAKER_00" {
		t.Fatalf("merged seg = %#v", got[0])
	}
	if got[0].Text != "We'll call to order today's meeting. twenty twenty six." {
		t.Fatalf("merged text = %q", got[0].Text)
	}
	if got[1].SpeakerCluster != "SPEAKER_01" || got[1].StartMS != 8000 || got[1].EndMS != 9000 {
		t.Fatalf("seg[1] = %#v", got[1])
	}
	if got[2].SpeakerCluster != "SPEAKER_00" || got[2].StartMS != 9000 {
		t.Fatalf("seg[2] = %#v", got[2])
	}
}

func TestMergeConsecutiveSegmentsEmpty(t *testing.T) {
	if got := MergeConsecutiveSegments(nil); len(got) != 0 {
		t.Fatalf("len=%d, want 0", len(got))
	}
}

func TestDeepgramProviderRequiresKey(t *testing.T) {
	_, err := NewDeepgramProvider(DeepgramConfig{})
	if err == nil || !strings.Contains(err.Error(), "API key") {
		t.Fatalf("err = %v, want API key error", err)
	}
}
