package diarization

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPyannoteAIProviderSubmitPayloadOmitsASRModel(t *testing.T) {
	var submitBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/diarize":
			if r.Method != http.MethodPost {
				t.Fatalf("submit method = %q", r.Method)
			}
			if r.Header.Get("Authorization") != "Bearer test-key" {
				t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
			}
			if err := json.NewDecoder(r.Body).Decode(&submitBody); err != nil {
				t.Fatalf("decode submit body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jobId":"job-1","status":"pending"}`))
		case "/v1/jobs/job-1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"status":"succeeded",
				"output":{"turnLevelTranscription":[
					{"start":1.0,"end":2.0,"speaker":"SPEAKER_00","text":"hello"}
				]}
			}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := NewPyannoteAIProvider(PyannoteAIConfig{
		APIKey:        "test-key",
		Endpoint:      srv.URL,
		Model:         "precision-2",
		Confidence:    true,
		Transcription: true,
	})
	if err != nil {
		t.Fatalf("NewPyannoteAIProvider: %v", err)
	}
	res, err := p.Diarize(context.Background(), AudioInput{EventID: "evt-1", URL: "https://example.com/audio.mp3"})
	if err != nil {
		t.Fatalf("Diarize: %v", err)
	}
	if _, ok := submitBody["asrModel"]; ok {
		t.Fatalf("submit body included rejected asrModel field: %#v", submitBody)
	}
	if submitBody["transcription"] != true || submitBody["model"] != "precision-2" || submitBody["url"] != "https://example.com/audio.mp3" {
		t.Fatalf("submit body = %#v", submitBody)
	}
	if len(res.Segments) != 1 || res.Segments[0].Text != "hello" {
		t.Fatalf("segments = %#v", res.Segments)
	}
}
