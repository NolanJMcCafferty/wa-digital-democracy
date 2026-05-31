package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type audioProbe struct {
	DurationMS int
	SampleRate int
	Channels   int
	Codec      string
}

func downloadMedia(ctx context.Context, mediaURL, dir string, rateLimit float64) (path, sha string, err error) {
	// Keep this streaming: TVW direct audio is usually manageable, but video
	// fallback assets can be multi-GB. Do not route through httpx.Do, which
	// intentionally buffers source API responses for provenance capture.
	_ = rateLimit // reserved for a future shared streaming downloader limiter
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", userAgent)
	client := &http.Client{Timeout: time.Hour}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	ext := mediaExt(mediaURL, resp.Header.Get("Content-Type"))
	tmp, err := os.CreateTemp(dir, "download_*")
	if err != nil {
		return "", "", err
	}
	tmpPath := tmp.Name()
	h := sha256.New()
	_, copyErr := io.Copy(tmp, io.TeeReader(resp.Body, h))
	closeErr := tmp.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return "", "", copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return "", "", closeErr
	}
	sha = hex.EncodeToString(h.Sum(nil))
	path = filepath.Join(dir, "original_"+sha[:12]+ext)
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return "", "", err
	}
	return path, sha, nil
}

func normalizeAudio(ctx context.Context, inPath, outPath string) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg not found on PATH: %w", err)
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", inPath, "-ac", "1", "-ar", "16000", "-vn", outPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func probeAudio(ctx context.Context, path string) audioProbe {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return audioProbe{}
	}
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "a:0", "-show_entries", "stream=codec_name,sample_rate,channels,duration", "-of", "json", path)
	out, err := cmd.Output()
	if err != nil {
		return audioProbe{}
	}
	var parsed struct {
		Streams []struct {
			CodecName  string `json:"codec_name"`
			SampleRate string `json:"sample_rate"`
			Channels   int    `json:"channels"`
			Duration   string `json:"duration"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil || len(parsed.Streams) == 0 {
		return audioProbe{}
	}
	s := parsed.Streams[0]
	sr, _ := strconv.Atoi(s.SampleRate)
	durSec, _ := strconv.ParseFloat(s.Duration, 64)
	return audioProbe{DurationMS: int(durSec*1000 + 0.5), SampleRate: sr, Channels: s.Channels, Codec: s.CodecName}
}
