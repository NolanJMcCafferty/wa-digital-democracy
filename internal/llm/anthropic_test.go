package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/diarization"
)

func TestNewAnthropicClient(t *testing.T) {
	tests := []struct {
		name    string
		cfg     AnthropicConfig
		wantErr bool
		errType error
	}{
		{
			name:    "missing API key",
			cfg:     AnthropicConfig{},
			wantErr: true,
			errType: ErrMissingAnthropicAPIKey,
		},
		{
			name: "empty API key",
			cfg: AnthropicConfig{
				APIKey: "   ",
			},
			wantErr: true,
			errType: ErrMissingAnthropicAPIKey,
		},
		{
			name: "valid API key",
			cfg: AnthropicConfig{
				APIKey: "sk-test-key",
			},
			wantErr: false,
		},
		{
			name: "valid API key with custom settings",
			cfg: AnthropicConfig{
				APIKey:    "sk-test-key",
				Endpoint:  "https://custom.endpoint.com/",
				Model:     "claude-3-opus-20240229",
				MaxTokens: 8000,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewAnthropicClient(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errType != nil && err != tt.errType {
					t.Fatalf("expected error %v, got %v", tt.errType, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if client == nil {
				t.Fatal("expected client, got nil")
			}

			// Check defaults
			if tt.cfg.Endpoint == "" && client.cfg.Endpoint != defaultAnthropicEndpoint {
				t.Errorf("expected default endpoint %s, got %s", defaultAnthropicEndpoint, client.cfg.Endpoint)
			}
			if tt.cfg.Model == "" && client.cfg.Model != defaultAnthropicModel {
				t.Errorf("expected default model %s, got %s", defaultAnthropicModel, client.cfg.Model)
			}
			if tt.cfg.MaxTokens <= 0 && client.cfg.MaxTokens != 4000 {
				t.Errorf("expected default max tokens 4000, got %d", client.cfg.MaxTokens)
			}
			// Check trailing slash removal
			if tt.cfg.Endpoint == "https://custom.endpoint.com/" && client.cfg.Endpoint != "https://custom.endpoint.com" {
				t.Errorf("expected endpoint without trailing slash, got %s", client.cfg.Endpoint)
			}
		})
	}
}

func TestAnthropicClient_ExtractSpeakerEvidence(t *testing.T) {
	// Mock server that returns a valid Anthropic Messages API response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request headers
		if r.Header.Get("x-api-key") == "" {
			t.Error("missing x-api-key header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("wrong content type")
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Error("wrong anthropic version")
		}
		if r.Header.Get("anthropic-beta") != "prompt-caching-2024-07-31" {
			t.Error("missing prompt caching beta header")
		}

		// Verify request body structure
		var reqBody anthropicMessagesRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		// Verify system message has cache control
		if len(reqBody.System) == 0 {
			t.Error("missing system message")
		} else if reqBody.System[0].CacheControl == nil {
			t.Error("system message missing cache control")
		} else if reqBody.System[0].CacheControl.Type != "ephemeral" {
			t.Error("wrong cache control type")
		}

		// Verify tool use setup
		if len(reqBody.Tools) == 0 {
			t.Error("missing tools")
		} else if reqBody.Tools[0].Name != "extract_speaker_evidence" {
			t.Error("wrong tool name")
		}

		if reqBody.ToolChoice.Type != "tool" || reqBody.ToolChoice.Name != "extract_speaker_evidence" {
			t.Error("wrong tool choice")
		}

		// Return mock response
		resp := anthropicMessagesResponse{
			Content: []anthropicResponseContent{
				{
					Type: "tool_use",
					Name: "extract_speaker_evidence",
					Input: map[string]interface{}{
						"candidates": []map[string]interface{}{
							{
								"segment_id":            int64(123),
								"cluster_label":         "SPEAKER_00",
								"candidate_kind":        "legislator",
								"candidate_label":       "Sen. John Doe",
								"candidate_id":          "456",
								"evidence_type":         "llm_candidate",
								"evidence_text_excerpt": "Senator Doe from the 32nd district",
								"confidence":            0.85,
								"reasoning":             "Speaker identifies as Senator Doe from district 32",
							},
						},
					},
				},
			},
			Usage: anthropicUsage{
				InputTokens:              1500,
				OutputTokens:             200,
				CacheCreationInputTokens: 800,
				CacheReadInputTokens:     700,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewAnthropicClient(AnthropicConfig{
		APIKey:   "sk-test-key",
		Endpoint: server.URL,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	req := diarization.SpeakerEvidenceLLMRequest{
		Agenda: diarization.SpeakerEvidenceAgenda{
			TVWEventID: "test-event",
			Legislators: []diarization.SpeakerEvidenceRef{
				{ID: 456, Label: "Sen. John Doe", Chamber: "senate"},
			},
		},
		Segments: []diarization.SpeakerEvidenceLLMSegment{
			{
				ID:           123,
				ClusterLabel: "SPEAKER_00",
				Text:         "Thank you Mr. Chair. Senator Doe from the 32nd district here.",
			},
		},
	}

	resp, err := client.ExtractSpeakerEvidence(context.Background(), req)
	if err != nil {
		t.Fatalf("ExtractSpeakerEvidence failed: %v", err)
	}

	if len(resp.Candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(resp.Candidates))
	}

	cand := resp.Candidates[0]
	if cand.SegmentID != 123 {
		t.Errorf("expected segment ID 123, got %d", cand.SegmentID)
	}
	if cand.CandidateKind != "legislator" {
		t.Errorf("expected kind 'legislator', got '%s'", cand.CandidateKind)
	}
	if cand.CandidateLabel != "Sen. John Doe" {
		t.Errorf("expected label 'Sen. John Doe', got '%s'", cand.CandidateLabel)
	}
	if cand.CandidateID != "456" {
		t.Errorf("expected candidate ID '456', got '%s'", cand.CandidateID)
	}
	if cand.Confidence != 0.85 {
		t.Errorf("expected confidence 0.85, got %f", cand.Confidence)
	}

	// Check usage stats
	if resp.Usage.InputTokens != 1500 {
		t.Errorf("expected 1500 input tokens, got %d", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 200 {
		t.Errorf("expected 200 output tokens, got %d", resp.Usage.OutputTokens)
	}
	if resp.Usage.CacheCreationInputTokens != 800 {
		t.Errorf("expected 800 cache creation tokens, got %d", resp.Usage.CacheCreationInputTokens)
	}
	if resp.Usage.CacheReadInputTokens != 700 {
		t.Errorf("expected 700 cache read tokens, got %d", resp.Usage.CacheReadInputTokens)
	}
}

func TestAnthropicClient_ExtractSpeakerEvidence_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Rate limited", http.StatusTooManyRequests)
	}))
	defer server.Close()

	client, err := NewAnthropicClient(AnthropicConfig{
		APIKey:   "sk-test-key",
		Endpoint: server.URL,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	req := diarization.SpeakerEvidenceLLMRequest{}
	_, err = client.ExtractSpeakerEvidence(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for HTTP 429, got nil")
	}
	if !containsString(err.Error(), "status 429") {
		t.Errorf("expected error to mention status 429, got: %v", err)
	}
}

func TestAnthropicClient_ExtractSpeakerEvidence_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"invalid": json}`))
	}))
	defer server.Close()

	client, err := NewAnthropicClient(AnthropicConfig{
		APIKey:   "sk-test-key",
		Endpoint: server.URL,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	req := diarization.SpeakerEvidenceLLMRequest{}
	_, err = client.ExtractSpeakerEvidence(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestAnthropicClient_ExtractSpeakerEvidence_NoToolUse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := anthropicMessagesResponse{
			Content: []anthropicResponseContent{
				{Type: "text", Name: "", Input: nil},
			},
			Usage: anthropicUsage{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewAnthropicClient(AnthropicConfig{
		APIKey:   "sk-test-key",
		Endpoint: server.URL,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	req := diarization.SpeakerEvidenceLLMRequest{}
	_, err = client.ExtractSpeakerEvidence(context.Background(), req)
	if err == nil {
		t.Fatal("expected error when no tool use found, got nil")
	}
	if !containsString(err.Error(), "no tool use content found") {
		t.Errorf("expected error about missing tool use, got: %v", err)
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			func() bool {
				for i := 1; i <= len(s)-len(substr); i++ {
					if s[i:i+len(substr)] == substr {
						return true
					}
				}
				return false
			}())))
}
