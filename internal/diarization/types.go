// Package diarization normalizes speaker-diarization provider outputs into
// time-bounded anonymous speaker clusters that downstream code can align to
// TVW WebVTT caption cues.
package diarization

import "context"

// Provider is the narrow boundary between WA Digital Democracy and an external
// diarization engine. Implementations may be synchronous APIs, async APIs, or
// cloud-GPU workers, but they should all return provider-neutral segments.
type Provider interface {
	Diarize(ctx context.Context, in AudioInput) (*Result, error)
}

// AudioInput describes the audio to diarize. Prefer URL when the provider can
// fetch TVW/Invintus audio directly; use Bytes for tests or providers that
// require direct upload.
type AudioInput struct {
	URL         string
	Bytes       []byte
	ContentType string
	EventID     string
}

// Result is provider-neutral diarization output. Speaker labels are anonymous
// cluster IDs (for example, SPEAKER_00). Identity assignment happens later via
// text/context evidence and human review.
type Result struct {
	Provider string          `json:"provider"`
	Model    string          `json:"model,omitempty"`
	EventID  string          `json:"event_id,omitempty"`
	Segments []Segment       `json:"segments"`
	Words    []Word          `json:"words,omitempty"`
	Entities []EntityMention `json:"entities,omitempty"`
	Raw      []byte          `json:"-"`
}

// Segment is a contiguous time span assigned to one anonymous speaker cluster.
type Segment struct {
	StartMS        int      `json:"start_ms"`
	EndMS          int      `json:"end_ms"`
	SpeakerCluster string   `json:"speaker_cluster"`
	Confidence     *float64 `json:"confidence,omitempty"`
	Text           string   `json:"text,omitempty"`
}

// EntityMention is one extracted named-entity mention from a provider or
// downstream extractor. Mentions are evidence, not canonical resolved facts.
type EntityMention struct {
	Text           string         `json:"text"`
	NormalizedText string         `json:"normalized_text,omitempty"`
	Type           string         `json:"type"`
	StartMS        *int           `json:"start_ms,omitempty"`
	EndMS          *int           `json:"end_ms,omitempty"`
	StartWord      *int           `json:"start_word,omitempty"`
	EndWord        *int           `json:"end_word,omitempty"`
	Confidence     *float64       `json:"confidence,omitempty"`
	Raw            map[string]any `json:"raw,omitempty"`
}

// Word is optional word-level output for providers that return it. It is useful
// for later transcript/VTT alignment diagnostics, but the stable storage layer
// can be built from Segments alone.
type Word struct {
	Word           string   `json:"word"`
	StartMS        int      `json:"start_ms"`
	EndMS          int      `json:"end_ms"`
	SpeakerCluster string   `json:"speaker_cluster,omitempty"`
	Confidence     *float64 `json:"confidence,omitempty"`
}
