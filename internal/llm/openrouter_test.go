package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/diarization"
)

func TestNewOpenRouterClientMissingAPIKey(t *testing.T) {
	_, err := NewOpenRouterClient(OpenRouterConfig{})
	if !errors.Is(err, ErrMissingOpenRouterAPIKey) {
		t.Fatalf("NewOpenRouterClient error = %v, want ErrMissingOpenRouterAPIKey", err)
	}
}

func TestOpenRouterSpeakerEvidenceRequestShape(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %q", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("X-Title") != "WA Digital Democracy" {
			t.Fatalf("X-Title = %q", r.Header.Get("X-Title"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices":[{
				"message":{
					"content":"{\"candidates\":[{\"segment_id\":101,\"cluster_label\":\"SPEAKER_01\",\"candidate_kind\":\"legislator\",\"candidate_label\":\"Senator Smith\",\"candidate_id\":\"7\",\"evidence_type\":\"llm_candidate\",\"evidence_text_excerpt\":\"Senator Smith from the 32nd\",\"confidence\":0.91,\"reasoning\":\"The speaker self-identifies by title and district.\"}]}"
				}
			}],
			"usage":{"prompt_tokens":12,"completion_tokens":34,"total_tokens":46,"prompt_tokens_details":{"cached_tokens":8}}
		}`))
	}))
	defer srv.Close()

	client, err := NewOpenRouterClient(OpenRouterConfig{
		APIKey:   "test-key",
		Endpoint: srv.URL,
		Model:    "anthropic/claude-sonnet-4-test",
	})
	if err != nil {
		t.Fatalf("NewOpenRouterClient: %v", err)
	}
	resp, err := client.ExtractSpeakerEvidence(context.Background(), diarization.SpeakerEvidenceLLMRequest{
		Agenda: diarization.SpeakerEvidenceAgenda{
			TVWEventID: "evt-1",
			Legislators: []diarization.SpeakerEvidenceRef{{
				ID: 7, Label: "Senator Jane Smith", District: "32",
			}},
		},
		Segments: []diarization.SpeakerEvidenceLLMSegment{{
			ID: 101, ClusterLabel: "SPEAKER_01", Text: "Senator Smith from the 32nd",
		}},
	})
	if err != nil {
		t.Fatalf("ExtractSpeakerEvidence: %v", err)
	}
	if len(resp.Candidates) != 1 || resp.Candidates[0].CandidateID != "7" {
		t.Fatalf("candidates = %#v", resp.Candidates)
	}
	if resp.Usage.InputTokens != 12 || resp.Usage.OutputTokens != 34 || resp.Usage.CacheReadInputTokens != 8 {
		t.Fatalf("usage = %#v", resp.Usage)
	}

	if got["model"] != "anthropic/claude-sonnet-4-test" {
		t.Fatalf("model = %q", got["model"])
	}
	cacheControl, ok := got["cache_control"].(map[string]any)
	if !ok || cacheControl["type"] != "ephemeral" {
		t.Fatalf("cache_control = %#v", got["cache_control"])
	}
	format := got["response_format"].(map[string]any)
	if format["type"] != "json_schema" {
		t.Fatalf("response_format = %#v", format)
	}
	jsonSchema := format["json_schema"].(map[string]any)
	if jsonSchema["name"] != "speaker_evidence" || jsonSchema["strict"] != true {
		t.Fatalf("json_schema = %#v", jsonSchema)
	}
	schema := jsonSchema["schema"].(map[string]any)
	props := schema["properties"].(map[string]any)
	if _, ok := props["candidates"]; !ok {
		t.Fatalf("schema missing candidates: %#v", schema)
	}
}
