package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/diarization"
)

const (
	defaultOpenRouterEndpoint = "https://openrouter.ai/api/v1"
	defaultOpenRouterModel    = "openai/gpt-5.5"
)

var ErrMissingOpenRouterAPIKey = errors.New("openrouter: OPENROUTER_API_KEY is required")

type OpenRouterConfig struct {
	APIKey     string
	Endpoint   string
	Model      string
	HTTPClient *http.Client
	MaxTokens  int
	Referer    string
	Title      string
}

type OpenRouterClient struct {
	cfg  OpenRouterConfig
	http *http.Client
}

func NewOpenRouterClient(cfg OpenRouterConfig) (*OpenRouterClient, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, ErrMissingOpenRouterAPIKey
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = defaultOpenRouterEndpoint
	}
	cfg.Endpoint = strings.TrimRight(cfg.Endpoint, "/")
	if cfg.Model == "" {
		cfg.Model = defaultOpenRouterModel
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 2048
	}
	if cfg.Title == "" {
		cfg.Title = "WA Digital Democracy"
	}
	h := cfg.HTTPClient
	if h == nil {
		h = &http.Client{Timeout: 90 * time.Second}
	}
	return &OpenRouterClient{cfg: cfg, http: h}, nil
}

func (c *OpenRouterClient) ExtractSpeakerEvidence(ctx context.Context, req diarization.SpeakerEvidenceLLMRequest) (diarization.SpeakerEvidenceLLMResponse, error) {
	body, err := json.Marshal(c.chatCompletionRequest(req))
	if err != nil {
		return diarization.SpeakerEvidenceLLMResponse{}, fmt.Errorf("openrouter speaker evidence request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.Endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return diarization.SpeakerEvidenceLLMResponse{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	if c.cfg.Referer != "" {
		httpReq.Header.Set("HTTP-Referer", c.cfg.Referer)
	}
	if c.cfg.Title != "" {
		httpReq.Header.Set("X-Title", c.cfg.Title)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return diarization.SpeakerEvidenceLLMResponse{}, fmt.Errorf("openrouter speaker evidence: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return diarization.SpeakerEvidenceLLMResponse{}, fmt.Errorf("openrouter speaker evidence status %d: %s", resp.StatusCode, trimForError(raw))
	}
	var decoded openRouterChatCompletionResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return diarization.SpeakerEvidenceLLMResponse{}, fmt.Errorf("openrouter speaker evidence decode: %w (body=%s)", err, trimForError(raw))
	}
	candidates, err := decoded.speakerEvidenceCandidates()
	if err != nil {
		return diarization.SpeakerEvidenceLLMResponse{}, err
	}
	return diarization.SpeakerEvidenceLLMResponse{
		Candidates: candidates,
		Usage: diarization.SpeakerEvidenceLLMUsage{
			InputTokens:              decoded.Usage.PromptTokens,
			OutputTokens:             decoded.Usage.CompletionTokens,
			CacheCreationInputTokens: 0,
			CacheReadInputTokens:     decoded.Usage.PromptTokensDetails.CachedTokens,
		},
	}, nil
}

func (c *OpenRouterClient) chatCompletionRequest(req diarization.SpeakerEvidenceLLMRequest) openRouterChatCompletionRequest {
	system := strings.TrimSpace(`You extract reviewable speaker identity evidence from Washington State legislative hearing transcript segments.
Use only the closed lists in the agenda context for legislators and CSI testifiers. Return a legislator or testifier candidate only when the speaker can be grounded to that list; otherwise return kind "person" with an empty candidate_id. Do not guess. Evidence excerpts must be short verbatim spans from the transcript. The result is review evidence only and is never automatically accepted.`)
	user := "Agenda context:\n" + mustJSON(map[string]any{"agenda": req.Agenda}) +
		"\n\nExtract speaker identity evidence from these diarized transcript segments:\n" +
		mustJSON(map[string]any{"segments": req.Segments})

	return openRouterChatCompletionRequest{
		Model:       c.cfg.Model,
		Messages:    []openRouterMessage{{Role: "system", Content: system}, {Role: "user", Content: user}},
		MaxTokens:   c.cfg.MaxTokens,
		Temperature: 0,
		ResponseFormat: openRouterResponseFormat{
			Type: "json_schema",
			JSONSchema: openRouterJSONSchema{
				Name:   "speaker_evidence",
				Strict: true,
				Schema: speakerEvidenceSchema(),
			},
		},
		CacheControl: &openRouterCacheControl{Type: "ephemeral"},
	}
}

type openRouterChatCompletionRequest struct {
	Model          string                   `json:"model"`
	Messages       []openRouterMessage      `json:"messages"`
	MaxTokens      int                      `json:"max_tokens"`
	Temperature    float64                  `json:"temperature"`
	ResponseFormat openRouterResponseFormat `json:"response_format"`
	CacheControl   *openRouterCacheControl  `json:"cache_control,omitempty"`
}

type openRouterMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openRouterCacheControl struct {
	Type string `json:"type"`
}

type openRouterResponseFormat struct {
	Type       string               `json:"type"`
	JSONSchema openRouterJSONSchema `json:"json_schema"`
}

type openRouterJSONSchema struct {
	Name   string         `json:"name"`
	Strict bool           `json:"strict"`
	Schema map[string]any `json:"schema"`
}

func speakerEvidenceSchema() map[string]any {
	candidateSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"segment_id": map[string]any{
				"type":        "integer",
				"description": "The exact segment_id from the transcript segment where the evidence appears.",
			},
			"cluster_label": map[string]any{"type": "string"},
			"candidate_kind": map[string]any{
				"type": "string",
				"enum": []string{"legislator", "testifier", "person"},
			},
			"candidate_label": map[string]any{"type": "string"},
			"candidate_id": map[string]any{
				"type":        "string",
				"description": "Use the numeric closed-list id for legislators/testifiers. Use an empty string for freeform person candidates.",
			},
			"evidence_type": map[string]any{
				"type": "string",
				"enum": []string{diarization.LLMSpeakerEvidenceType},
			},
			"evidence_text_excerpt": map[string]any{"type": "string"},
			"confidence": map[string]any{
				"type":    "number",
				"minimum": 0,
				"maximum": 1,
			},
			"reasoning": map[string]any{"type": "string"},
		},
		"required": []string{
			"segment_id",
			"cluster_label",
			"candidate_kind",
			"candidate_label",
			"candidate_id",
			"evidence_type",
			"evidence_text_excerpt",
			"confidence",
			"reasoning",
		},
		"additionalProperties": false,
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"candidates": map[string]any{
				"type":  "array",
				"items": candidateSchema,
			},
		},
		"required":             []string{"candidates"},
		"additionalProperties": false,
	}
}

type openRouterChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		TotalTokens         int `json:"total_tokens"`
		PromptTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
}

func (r openRouterChatCompletionResponse) speakerEvidenceCandidates() ([]diarization.SpeakerEvidenceLLMResult, error) {
	if len(r.Choices) == 0 {
		return nil, fmt.Errorf("openrouter speaker evidence: response had no choices")
	}
	content := strings.TrimSpace(r.Choices[0].Message.Content)
	if content == "" {
		return nil, fmt.Errorf("openrouter speaker evidence: response content was empty")
	}
	var out struct {
		Candidates []diarization.SpeakerEvidenceLLMResult `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		return nil, fmt.Errorf("openrouter speaker evidence structured decode: %w (content=%s)", err, trimForError([]byte(content)))
	}
	return out.Candidates, nil
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func trimForError(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	if len(s) > 500 {
		return s[:500] + "..."
	}
	return s
}
