package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func init() {
	register(Command{
		Name:     "audio-cache",
		Synopsis: "Download and normalize TVW/Invintus audio for diarization",
		Run:      runAudioCache,
	})
}
func runAudioCache(args []string) int {
	fs := flag.NewFlagSet("audio-cache", flag.ContinueOnError)
	var (
		eventID    = fs.String("event-id", "", "TVW/Invintus event ID to cache audio for")
		dsn        = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		outDir     = fs.String("out-dir", "data/audio", "audio cache output root")
		rateLimit  = fs.Float64("rate", 4.0, "max requests/sec for media download host")
		keepSource = fs.Bool("keep-source", true, "keep downloaded original media next to normalized WAV")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*eventID) == "" {
		fmt.Fprintln(os.Stderr, "audio-cache: --event-id is required")
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "audio-cache: db open: %v\n", err)
		return 1
	}
	defer store.Close()

	candidates, err := store.ListTVWAudioSourceCandidates(ctx, *eventID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "audio-cache: list audio source candidates: %v\n", err)
		return 1
	}
	if len(candidates) == 0 {
		fmt.Fprintf(os.Stderr, "audio-cache: no usable media source candidates found for event %s\n", *eventID)
	} else {
		fmt.Fprintf(os.Stderr, "==> media source candidates for event %s\n", *eventID)
		for i, c := range candidates {
			selected := ""
			if i == 0 {
				selected = " selected"
			}
			fmt.Fprintf(os.Stderr, "  priority=%d kind=%s%s url=%s\n", c.Priority, c.Kind, selected, c.URL)
		}
		if strings.Contains(candidates[0].Kind, "video") {
			fmt.Fprintln(os.Stderr, "  note: selected a video fallback because no higher-priority audio source URL was present")
		}
	}

	src, err := store.BestTVWAudioSource(ctx, *eventID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "audio-cache: find audio source: %v\n", err)
		return 1
	}
	cacheDir := filepath.Join(*outDir, "tvw", safePathPart(*eventID))
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "audio-cache: mkdir: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "==> downloading %s (%s)\n", src.URL, src.Kind)
	origPath, hash, err := downloadMedia(ctx, src.URL, cacheDir, *rateLimit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "audio-cache: download: %v\n", err)
		return 1
	}
	wavPath := filepath.Join(cacheDir, "mono_16k.wav")
	if err := normalizeAudio(ctx, origPath, wavPath); err != nil {
		fmt.Fprintf(os.Stderr, "audio-cache: ffmpeg normalize: %v\n", err)
		return 1
	}
	meta := probeAudio(ctx, wavPath)
	id, err := store.UpsertTVWAudioAsset(ctx, db.UpsertTVWAudioAssetParams{
		TVWEventID:     *eventID,
		SourceURL:      src.URL,
		SourceKind:     src.Kind,
		OriginalPath:   origPath,
		NormalizedPath: wavPath,
		ContentHash:    hash,
		DurationMS:     meta.DurationMS,
		SampleRate:     meta.SampleRate,
		Channels:       meta.Channels,
		Codec:          meta.Codec,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "audio-cache: store metadata: %v\n", err)
		return 1
	}
	if !*keepSource {
		_ = os.Remove(origPath)
	}
	fmt.Fprintf(os.Stderr, "cached audio_asset_id=%d\noriginal=%s\nnormalized=%s\nsha256=%s\n", id, origPath, wavPath, hash)
	return 0
}
