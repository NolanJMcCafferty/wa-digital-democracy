package diarization

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const defaultDeepgramEndpoint = "https://api.deepgram.com/v1/listen"

// DeepgramConfig configures the Deepgram prerecorded-audio diarization adapter.
type DeepgramConfig struct {
	APIKey     string
	Endpoint   string
	Model      string
	HTTPClient *http.Client
	Timeout    time.Duration
}

// DeepgramProvider calls Deepgram's prerecorded transcription endpoint with
// diarization enabled, then normalizes word/utterance speaker labels into
// anonymous time spans.
type DeepgramProvider struct {
	cfg  DeepgramConfig
	http *http.Client
}

// NewDeepgramProvider constructs a Deepgram adapter. APIKey is required.
func NewDeepgramProvider(cfg DeepgramConfig) (*DeepgramProvider, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("deepgram: API key is required")
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = defaultDeepgramEndpoint
	}
	if cfg.Model == "" {
		cfg.Model = "nova-3"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Minute
	}
	h := cfg.HTTPClient
	if h == nil {
		h = &http.Client{Timeout: cfg.Timeout}
	}
	return &DeepgramProvider{cfg: cfg, http: h}, nil
}

// Diarize sends either an audio URL or raw audio bytes to Deepgram and returns
// provider-neutral segments. URL input is preferred for TVW audio assets.
func (p *DeepgramProvider) Diarize(ctx context.Context, in AudioInput) (*Result, error) {
	if strings.TrimSpace(in.URL) == "" && len(in.Bytes) == 0 {
		return nil, errors.New("deepgram: AudioInput.URL or AudioInput.Bytes is required")
	}

	u, err := url.Parse(p.cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("deepgram endpoint parse: %w", err)
	}
	q := u.Query()
	q.Set("diarize", "true")
	q.Set("punctuate", "true")
	q.Set("utterances", "true")
	q.Set("detect_entities", "true")
	q.Set("model", p.cfg.Model)
	u.RawQuery = q.Encode()

	var body io.Reader
	contentType := in.ContentType
	if strings.TrimSpace(in.URL) != "" {
		payload, err := json.Marshal(map[string]string{"url": in.URL})
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(payload)
		contentType = "application/json"
	} else {
		body = bytes.NewReader(in.Bytes)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Token "+p.cfg.APIKey)
	req.Header.Set("Content-Type", contentType)

	resp, err := p.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("deepgram request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("deepgram read: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("deepgram status %d: %s", resp.StatusCode, trimForError(raw))
	}

	parsed, err := parseDeepgram(raw)
	if err != nil {
		return nil, err
	}
	parsed.Provider = "deepgram"
	parsed.Model = p.cfg.Model
	parsed.EventID = in.EventID
	parsed.Raw = raw
	return parsed, nil
}

func trimForError(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 500 {
		return s[:500] + "…"
	}
	return s
}

type deepgramResponse struct {
	Results struct {
		Utterances []struct {
			Start      float64  `json:"start"`
			End        float64  `json:"end"`
			Speaker    *int     `json:"speaker"`
			Transcript string   `json:"transcript"`
			Confidence *float64 `json:"confidence"`
		} `json:"utterances"`
		Channels []struct {
			Alternatives []struct {
				Words []struct {
					Word              string   `json:"word"`
					Start             float64  `json:"start"`
					End               float64  `json:"end"`
					Speaker           *int     `json:"speaker"`
					SpeakerConfidence *float64 `json:"speaker_confidence"`
					Confidence        *float64 `json:"confidence"`
				} `json:"words"`
				Entities []struct {
					Label      string   `json:"label"`
					Value      string   `json:"value"`
					Confidence *float64 `json:"confidence"`
					StartWord  *int     `json:"start_word"`
					EndWord    *int     `json:"end_word"`
				} `json:"entities"`
			} `json:"alternatives"`
		} `json:"channels"`
	} `json:"results"`
}

func parseDeepgram(raw []byte) (*Result, error) {
	var dg deepgramResponse
	if err := json.Unmarshal(raw, &dg); err != nil {
		return nil, fmt.Errorf("deepgram decode: %w", err)
	}

	words := deepgramWords(dg)
	segments := deepgramUtteranceSegments(dg)
	if len(segments) == 0 {
		segments = segmentsFromWords(words)
	}
	entities := deepgramEntities(dg, words)
	return &Result{Segments: segments, Words: words, Entities: entities}, nil
}

func deepgramEntities(dg deepgramResponse, words []Word) []EntityMention {
	var out []EntityMention
	for _, ch := range dg.Results.Channels {
		for _, alt := range ch.Alternatives {
			for _, e := range alt.Entities {
				text := strings.TrimSpace(e.Value)
				if text == "" {
					continue
				}
				m := EntityMention{
					Text:           text,
					NormalizedText: strings.ToLower(text),
					Type:           strings.TrimSpace(e.Label),
					StartWord:      e.StartWord,
					EndWord:        e.EndWord,
					Confidence:     e.Confidence,
					Raw: map[string]any{
						"label": e.Label,
						"value": e.Value,
					},
				}
				if e.StartWord != nil && *e.StartWord >= 0 && *e.StartWord < len(words) {
					v := words[*e.StartWord].StartMS
					m.StartMS = &v
				}
				if e.EndWord != nil && *e.EndWord > 0 && *e.EndWord <= len(words) {
					v := words[*e.EndWord-1].EndMS
					m.EndMS = &v
				}
				out = append(out, m)
			}
		}
	}
	return out
}

func deepgramWords(dg deepgramResponse) []Word {
	var out []Word
	for _, ch := range dg.Results.Channels {
		for _, alt := range ch.Alternatives {
			for _, w := range alt.Words {
				cluster := ""
				if w.Speaker != nil {
					cluster = speakerLabel(*w.Speaker)
				}
				conf := w.SpeakerConfidence
				if conf == nil {
					conf = w.Confidence
				}
				out = append(out, Word{
					Word:           w.Word,
					StartMS:        secondsToMS(w.Start),
					EndMS:          secondsToMS(w.End),
					SpeakerCluster: cluster,
					Confidence:     conf,
				})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartMS < out[j].StartMS })
	return out
}

func deepgramUtteranceSegments(dg deepgramResponse) []Segment {
	out := make([]Segment, 0, len(dg.Results.Utterances))
	for _, u := range dg.Results.Utterances {
		if u.Speaker == nil {
			continue
		}
		out = append(out, Segment{
			StartMS:        secondsToMS(u.Start),
			EndMS:          secondsToMS(u.End),
			SpeakerCluster: speakerLabel(*u.Speaker),
			Confidence:     u.Confidence,
			Text:           strings.TrimSpace(u.Transcript),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartMS < out[j].StartMS })
	return out
}

func segmentsFromWords(words []Word) []Segment {
	const maxGapMS = 1200
	var out []Segment
	var cur *Segment
	var text []string
	flush := func() {
		if cur == nil {
			return
		}
		cur.Text = strings.Join(text, " ")
		out = append(out, *cur)
		cur = nil
		text = nil
	}
	for _, w := range words {
		if w.SpeakerCluster == "" {
			continue
		}
		if cur == nil || w.SpeakerCluster != cur.SpeakerCluster || w.StartMS-cur.EndMS > maxGapMS {
			flush()
			cur = &Segment{StartMS: w.StartMS, EndMS: w.EndMS, SpeakerCluster: w.SpeakerCluster, Confidence: w.Confidence}
			text = []string{w.Word}
			continue
		}
		cur.EndMS = w.EndMS
		text = append(text, w.Word)
		if cur.Confidence == nil {
			cur.Confidence = w.Confidence
		}
	}
	flush()
	return out
}

func speakerLabel(n int) string { return fmt.Sprintf("SPEAKER_%02d", n) }

func secondsToMS(s float64) int { return int(math.Round(s * 1000)) }
