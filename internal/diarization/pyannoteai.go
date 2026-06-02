package diarization

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"time"
)

const (
	defaultPyannoteAIEndpoint = "https://api.pyannote.ai"
	defaultPyannoteAIModel    = "precision-2"
)

// PyannoteAIConfig configures the pyannoteAI Precision-2 diarization adapter.
// pyannoteAI runs an async job: POST /v1/diarize → poll /v1/jobs/{id} until
// "succeeded". Output is diarization-only — no transcript words.
type PyannoteAIConfig struct {
	APIKey     string
	Endpoint   string
	Model      string
	HTTPClient *http.Client

	PollInterval time.Duration
	MaxWait      time.Duration

	// Confidence asks pyannoteAI to return per-segment confidence in the
	// `0..1` range. Defaults to true.
	Confidence bool
}

// PyannoteAIProvider implements diarization.Provider against pyannoteAI's
// hosted Precision-2 model. Use it as a second-pass diarizer over the
// cached TVW WAV/MP3 to get clusters that don't suffer from Deepgram's
// long-gap false-merge behavior.
type PyannoteAIProvider struct {
	cfg  PyannoteAIConfig
	http *http.Client
}

// NewPyannoteAIProvider constructs the adapter. APIKey is required.
func NewPyannoteAIProvider(cfg PyannoteAIConfig) (*PyannoteAIProvider, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("pyannoteai: API key is required")
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = defaultPyannoteAIEndpoint
	}
	cfg.Endpoint = strings.TrimRight(cfg.Endpoint, "/")
	if cfg.Model == "" {
		cfg.Model = defaultPyannoteAIModel
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 5 * time.Second
	}
	if cfg.MaxWait <= 0 {
		cfg.MaxWait = 30 * time.Minute
	}
	h := cfg.HTTPClient
	if h == nil {
		h = &http.Client{Timeout: 60 * time.Second}
	}
	return &PyannoteAIProvider{cfg: cfg, http: h}, nil
}

// Diarize submits an audio URL to pyannoteAI, polls until the job
// finishes, and returns provider-neutral segments. AudioInput.URL is
// required; AudioInput.Bytes is not supported (pyannoteAI requires a URL
// or a presigned upload URL — direct uploads would need a separate
// /v1/media/input round trip).
func (p *PyannoteAIProvider) Diarize(ctx context.Context, in AudioInput) (*Result, error) {
	if strings.TrimSpace(in.URL) == "" {
		return nil, errors.New("pyannoteai: AudioInput.URL is required (direct byte uploads not yet supported)")
	}

	log.Printf("pyannoteai: submitting job model=%s url=%s", p.cfg.Model, in.URL)
	jobID, err := p.submitJob(ctx, in.URL)
	if err != nil {
		return nil, err
	}
	log.Printf("pyannoteai: job submitted id=%s — polling every %s (max %s)", jobID, p.cfg.PollInterval, p.cfg.MaxWait)

	rawJSON, status, err := p.waitForJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if status != "succeeded" {
		return nil, fmt.Errorf("pyannoteai: job %s ended in status %q: %s", jobID, status, trimForError(rawJSON))
	}
	log.Printf("pyannoteai: job %s succeeded (%d bytes of result JSON)", jobID, len(rawJSON))

	res, err := parsePyannoteAI(rawJSON)
	if err != nil {
		return nil, err
	}
	log.Printf("pyannoteai: parsed %d segments from job %s", len(res.Segments), jobID)
	res.Provider = "pyannoteai"
	res.Model = p.cfg.Model
	res.EventID = in.EventID
	res.Raw = rawJSON
	return res, nil
}

type pyannoteSubmitRequest struct {
	URL        string `json:"url"`
	Model      string `json:"model,omitempty"`
	Confidence bool   `json:"confidence,omitempty"`
}

type pyannoteSubmitResponse struct {
	JobID  string `json:"jobId"`
	Status string `json:"status"`
}

func (p *PyannoteAIProvider) submitJob(ctx context.Context, audioURL string) (string, error) {
	body, err := json.Marshal(pyannoteSubmitRequest{URL: audioURL, Model: p.cfg.Model, Confidence: p.cfg.Confidence})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.Endpoint+"/v1/diarize", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("pyannoteai submit: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("pyannoteai submit status %d: %s", resp.StatusCode, trimForError(raw))
	}
	var out pyannoteSubmitResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("pyannoteai submit decode: %w (body=%s)", err, trimForError(raw))
	}
	if strings.TrimSpace(out.JobID) == "" {
		return "", fmt.Errorf("pyannoteai submit: no jobId returned: %s", trimForError(raw))
	}
	return out.JobID, nil
}

// waitForJob polls /v1/jobs/{id} until it terminates. Returns the raw
// JSON of the final response so callers can both decode it and persist
// the original payload for offline reanalysis.
func (p *PyannoteAIProvider) waitForJob(ctx context.Context, jobID string) ([]byte, string, error) {
	deadline := time.Now().Add(p.cfg.MaxWait)
	url := p.cfg.Endpoint + "/v1/jobs/" + jobID
	start := time.Now()
	polls := 0
	for {
		polls++
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, "", err
		}
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
		resp, err := p.http.Do(req)
		if err != nil {
			return nil, "", fmt.Errorf("pyannoteai poll: %w", err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, "", fmt.Errorf("pyannoteai poll status %d: %s", resp.StatusCode, trimForError(raw))
		}
		var status struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(raw, &status); err != nil {
			return nil, "", fmt.Errorf("pyannoteai poll decode: %w (body=%s)", err, trimForError(raw))
		}
		st := strings.ToLower(strings.TrimSpace(status.Status))
		switch st {
		case "succeeded", "failed", "cancelled", "canceled":
			log.Printf("pyannoteai: job %s reached terminal status=%s after %s (%d polls)", jobID, st, time.Since(start).Round(time.Second), polls)
			return raw, st, nil
		}
		log.Printf("pyannoteai: job %s status=%s elapsed=%s poll=%d", jobID, st, time.Since(start).Round(time.Second), polls)
		if time.Now().After(deadline) {
			return nil, "", fmt.Errorf("pyannoteai poll: job %s did not finish within %s (last status=%q)", jobID, p.cfg.MaxWait, status.Status)
		}
		select {
		case <-ctx.Done():
			return nil, "", ctx.Err()
		case <-time.After(p.cfg.PollInterval):
		}
	}
}

type pyannoteJobResult struct {
	Status string `json:"status"`
	Output struct {
		Diarization []struct {
			Start      float64  `json:"start"`
			End        float64  `json:"end"`
			Speaker    string   `json:"speaker"`
			Confidence *float64 `json:"confidence,omitempty"`
		} `json:"diarization"`
	} `json:"output"`
}

// parsePyannoteAI converts the pyannoteAI job response into provider-neutral
// segments. The API returns sub-second times in seconds; we round to ms.
// pyannoteAI does not transcribe — Text on each segment is left empty.
func parsePyannoteAI(raw []byte) (*Result, error) {
	var r pyannoteJobResult
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("pyannoteai decode: %w", err)
	}
	segs := make([]Segment, 0, len(r.Output.Diarization))
	for _, d := range r.Output.Diarization {
		startMS := int(math.Round(d.Start * 1000))
		endMS := int(math.Round(d.End * 1000))
		if endMS <= startMS {
			continue
		}
		segs = append(segs, Segment{
			StartMS:        startMS,
			EndMS:          endMS,
			SpeakerCluster: strings.TrimSpace(d.Speaker),
			Confidence:     d.Confidence,
		})
	}
	segs = MergeConsecutiveSegments(segs)
	return &Result{Segments: segs}, nil
}
