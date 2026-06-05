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
	defaultAnthropicEndpoint = "https://api.anthropic.com"
	defaultAnthropicModel    = "claude-3-5-sonnet-20241022"
)

var ErrMissingAnthropicAPIKey = errors.New("anthropic: ANTHROPIC_API_KEY is required")

type AnthropicConfig struct {
	APIKey     string
	Endpoint   string
	Model      string
	HTTPClient *http.Client
	MaxTokens  int
}

type AnthropicClient struct {
	cfg  AnthropicConfig
	http *http.Client
}

func NewAnthropicClient(cfg AnthropicConfig) (*AnthropicClient, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, ErrMissingAnthropicAPIKey
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = defaultAnthropicEndpoint
	}
	cfg.Endpoint = strings.TrimRight(cfg.Endpoint, "/")
	if cfg.Model == "" {
		cfg.Model = defaultAnthropicModel
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 4000
	}
	h := cfg.HTTPClient
	if h == nil {
		h = &http.Client{Timeout: 120 * time.Second}
	}
	return &AnthropicClient{cfg: cfg, http: h}, nil
}

func (c *AnthropicClient) ExtractSpeakerEvidence(ctx context.Context, req diarization.SpeakerEvidenceLLMRequest) (diarization.SpeakerEvidenceLLMResponse, error) {
	body, err := json.Marshal(c.messagesRequest(req))
	if err != nil {
		return diarization.SpeakerEvidenceLLMResponse{}, fmt.Errorf("anthropic speaker evidence request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.Endpoint+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return diarization.SpeakerEvidenceLLMResponse{}, err
	}
	httpReq.Header.Set("x-api-key", c.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("anthropic-beta", "prompt-caching-2024-07-31")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return diarization.SpeakerEvidenceLLMResponse{}, fmt.Errorf("anthropic speaker evidence: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return diarization.SpeakerEvidenceLLMResponse{}, fmt.Errorf("anthropic speaker evidence status %d: %s", resp.StatusCode, trimForError(raw))
	}
	var decoded anthropicMessagesResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return diarization.SpeakerEvidenceLLMResponse{}, fmt.Errorf("anthropic speaker evidence decode: %w (body=%s)", err, trimForError(raw))
	}
	candidates, err := decoded.speakerEvidenceCandidates()
	if err != nil {
		return diarization.SpeakerEvidenceLLMResponse{}, err
	}
	return diarization.SpeakerEvidenceLLMResponse{
		Candidates: candidates,
		Usage: diarization.SpeakerEvidenceLLMUsage{
			InputTokens:              decoded.Usage.InputTokens,
			OutputTokens:             decoded.Usage.OutputTokens,
			CacheCreationInputTokens: decoded.Usage.CacheCreationInputTokens,
			CacheReadInputTokens:     decoded.Usage.CacheReadInputTokens,
		},
	}, nil
}

func (c *AnthropicClient) messagesRequest(req diarization.SpeakerEvidenceLLMRequest) anthropicMessagesRequest {
	system := strings.TrimSpace(`You extract reviewable speaker identity evidence from Washington State legislative hearing transcript segments.

Use only the closed lists in the agenda context for legislators and CSI testifiers. Return a legislator or testifier candidate only when the speaker can be grounded to that list; otherwise return kind "person" with an empty candidate_id. Do not guess. Evidence excerpts must be short verbatim spans from the transcript. The result is review evidence only and is never automatically accepted.

When processing transcripts:
1. Look for explicit self-introductions ("my name is...", "I'm Senator Smith", "Representative Jones here")
2. Look for role-based references ("the gentleman from the 5th district", "Senator Smith from the 32nd")
3. Look for contextual clues ("Mr. Chair", references to committee positions)
4. Match against the provided legislator roster and CSI testifier list
5. Only return candidates when you have reasonable confidence in the identity match
6. Provide confidence scores between 0.5-1.0, with higher scores for more explicit identifications

Return structured results with candidate_kind, candidate_label, candidate_id (for legislators/testifiers), evidence_type as "llm_candidate", evidence_text_excerpt from the transcript, confidence, and reasoning.`)

	userContent := "Agenda context:\n" + mustJSON(map[string]any{"agenda": req.Agenda}) +
		"\n\nExtract speaker identity evidence from these diarized transcript segments:\n" +
		mustJSON(map[string]any{"segments": req.Segments})

	return anthropicMessagesRequest{
		Model:     c.cfg.Model,
		MaxTokens: c.cfg.MaxTokens,
		System: []anthropicSystemMessage{
			{
				Type:         "text",
				Text:         system,
				CacheControl: &anthropicCacheControl{Type: "ephemeral"},
			},
		},
		Messages: []anthropicMessage{
			{
				Role: "user",
				Content: []anthropicContentBlock{
					{Type: "text", Text: userContent},
				},
			},
		},
		Tools: []anthropicTool{
			{
				Name:        "extract_speaker_evidence",
				Description: "Extract speaker identity evidence from transcript segments",
				InputSchema: speakerEvidenceToolSchema(),
			},
		},
		ToolChoice: anthropicToolChoice{
			Type: "tool",
			Name: "extract_speaker_evidence",
		},
	}
}

type anthropicMessagesRequest struct {
	Model      string                   `json:"model"`
	MaxTokens  int                      `json:"max_tokens"`
	System     []anthropicSystemMessage `json:"system,omitempty"`
	Messages   []anthropicMessage       `json:"messages"`
	Tools      []anthropicTool          `json:"tools,omitempty"`
	ToolChoice anthropicToolChoice      `json:"tool_choice,omitempty"`
}

type anthropicSystemMessage struct {
	Type         string                 `json:"type"`
	Text         string                 `json:"text"`
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type anthropicToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

type anthropicCacheControl struct {
	Type string `json:"type"`
}

func speakerEvidenceToolSchema() map[string]any {
	candidateSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"segment_id": map[string]any{
				"type":        "integer",
				"description": "The exact segment_id from the transcript segment where the evidence appears.",
			},
			"cluster_label": map[string]any{"type": "string"},
			"candidate_kind": map[string]any{
				"type":        "string",
				"enum":        []string{"legislator", "testifier", "person"},
				"description": "Use 'legislator' for members of the legislature, 'testifier' for CSI hearing testifiers, 'person' for unidentified speakers.",
			},
			"candidate_label": map[string]any{
				"type":        "string",
				"description": "The name of the speaker as it should be displayed. Use canonical form from the agenda for legislators/testifiers.",
			},
			"candidate_id": map[string]any{
				"type":        "string",
				"description": "Use the numeric closed-list id for legislators/testifiers. Use an empty string for freeform person candidates.",
			},
			"evidence_type": map[string]any{
				"type":        "string",
				"enum":        []string{diarization.LLMSpeakerEvidenceType},
				"description": "Must be 'llm_candidate'",
			},
			"evidence_text_excerpt": map[string]any{
				"type":        "string",
				"description": "Short verbatim excerpt from the transcript that provides evidence for this speaker identity.",
			},
			"confidence": map[string]any{
				"type":        "number",
				"minimum":     0.5,
				"maximum":     1.0,
				"description": "Confidence in the speaker identification, 0.5-1.0. Use higher values for explicit self-introductions.",
			},
			"reasoning": map[string]any{
				"type":        "string",
				"description": "Brief explanation of why this candidate matches the speaker.",
			},
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
				"type":        "array",
				"items":       candidateSchema,
				"description": "Array of speaker identity candidates extracted from the transcript segments.",
			},
		},
		"required":             []string{"candidates"},
		"additionalProperties": false,
	}
}

type anthropicMessagesResponse struct {
	Content []anthropicResponseContent `json:"content"`
	Usage   anthropicUsage             `json:"usage"`
}

type anthropicResponseContent struct {
	Type  string                 `json:"type"`
	Name  string                 `json:"name,omitempty"`
	Input map[string]interface{} `json:"input,omitempty"`
}

type anthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

func (r anthropicMessagesResponse) speakerEvidenceCandidates() ([]diarization.SpeakerEvidenceLLMResult, error) {
	for _, content := range r.Content {
		if content.Type == "tool_use" && content.Name == "extract_speaker_evidence" && content.Input != nil {
			if candidates, ok := content.Input["candidates"]; ok {
				candidatesJSON, err := json.Marshal(candidates)
				if err != nil {
					return nil, fmt.Errorf("anthropic speaker evidence: marshal candidates: %w", err)
				}
				var out []diarization.SpeakerEvidenceLLMResult
				if err := json.Unmarshal(candidatesJSON, &out); err != nil {
					return nil, fmt.Errorf("anthropic speaker evidence: decode candidates: %w", err)
				}
				return out, nil
			}
		}
	}
	return nil, fmt.Errorf("anthropic speaker evidence: no tool use content found in response")
}
