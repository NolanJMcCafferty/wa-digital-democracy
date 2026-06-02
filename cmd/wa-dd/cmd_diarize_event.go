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

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/diarization"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func init() {
	register(Command{
		Name:     "diarize-event",
		Synopsis: "Run provider diarization for a cached TVW event audio asset",
		Run:      runDiarizeEvent,
	})
}
func runDiarizeEvent(args []string) int {
	fs := flag.NewFlagSet("diarize-event", flag.ContinueOnError)
	var (
		eventID  = fs.String("event-id", "", "TVW/Invintus event ID to diarize")
		dsn      = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		provider = fs.String("provider", "deepgram", "diarization provider (currently: deepgram)")
		model    = fs.String("model", "nova-3", "provider model")
		outDir   = fs.String("out-dir", "data/processed/diarization", "raw diarization JSON output root")
		apiKey   = fs.String("api-key", "", "provider API key (defaults to DEEPGRAM_API_KEY or PYANNOTEAI_API_KEY based on --provider)")
		useURL   = fs.Bool("use-source-url", true, "send original TVW/Invintus URL to provider instead of uploading local normalized WAV")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*eventID) == "" {
		fmt.Fprintln(os.Stderr, "diarize-event: --event-id is required")
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "diarize-event: db open: %v\n", err)
		return 1
	}
	defer store.Close()

	key := *apiKey
	if strings.TrimSpace(key) == "" {
		switch strings.ToLower(*provider) {
		case "pyannoteai":
			key = env("PYANNOTEAI_API_KEY", "")
		default:
			key = env("DEEPGRAM_API_KEY", "")
		}
	}
	if strings.ToLower(*provider) == "pyannoteai" && *model == "nova-3" {
		*model = "precision-2"
	}
	p, err := newDiarizationProvider(*provider, key, *model)
	if err != nil {
		fmt.Fprintf(os.Stderr, "diarize-event: %v\n", err)
		return 2
	}

	if err := diarizeOneEvent(ctx, store, p, *eventID, *provider, *model, *outDir, *useURL); err != nil {
		fmt.Fprintf(os.Stderr, "diarize-event: %v\n", err)
		return 1
	}
	return 0
}

func newDiarizationProvider(provider, apiKey, model string) (diarization.Provider, error) {
	switch strings.ToLower(provider) {
	case "deepgram":
		dg, err := diarization.NewDeepgramProvider(diarization.DeepgramConfig{APIKey: apiKey, Model: model})
		if err != nil {
			return nil, fmt.Errorf("deepgram: %w", err)
		}
		return dg, nil
	case "pyannoteai":
		pa, err := diarization.NewPyannoteAIProvider(diarization.PyannoteAIConfig{APIKey: apiKey, Model: model, Confidence: true})
		if err != nil {
			return nil, fmt.Errorf("pyannoteai: %w", err)
		}
		return pa, nil
	default:
		return nil, fmt.Errorf("unsupported provider %q", provider)
	}
}

// diarizeOneEvent runs a single diarization pass for eventID, persisting the
// job + segments + entity mentions. Returns nil on success.
func diarizeOneEvent(ctx context.Context, store *db.Store, p diarization.Provider, eventID, provider, model, outDir string, useURL bool) error {
	audio, createdAsset, err := store.EnsureTVWAudioAsset(ctx, eventID)
	if err != nil {
		return fmt.Errorf("resolve audio source: %w", err)
	}
	if createdAsset {
		fmt.Fprintf(os.Stderr, "==> created audio source asset %d from %s (%s)\n", audio.ID, audio.SourceURL, audio.SourceKind)
	}

	jobID, err := store.CreateDiarizationJob(ctx, db.CreateDiarizationJobParams{TVWEventID: eventID, AudioAssetID: audio.ID, Provider: provider, Model: model})
	if err != nil {
		return fmt.Errorf("create job: %w", err)
	}

	in := diarization.AudioInput{EventID: eventID}
	if useURL && audio.SourceURL != "" {
		in.URL = audio.SourceURL
	} else {
		if strings.TrimSpace(audio.NormalizedPath) == "" {
			_ = store.FailDiarizationJob(ctx, jobID, "no normalized audio path; run audio-cache first or omit --use-source-url=false")
			return fmt.Errorf("no normalized audio path; run `wa-dd audio-cache --event-id %s` first or omit --use-source-url=false", eventID)
		}
		b, err := os.ReadFile(audio.NormalizedPath)
		if err != nil {
			_ = store.FailDiarizationJob(ctx, jobID, err.Error())
			return fmt.Errorf("read normalized audio: %w", err)
		}
		in.Bytes = b
		in.ContentType = "audio/wav"
	}

	fmt.Fprintf(os.Stderr, "==> diarizing event %s with %s (%s), job_id=%d\n", eventID, provider, model, jobID)
	res, err := p.Diarize(ctx, in)
	if err != nil {
		_ = store.FailDiarizationJob(ctx, jobID, err.Error())
		return fmt.Errorf("provider: %w", err)
	}
	rawPath := ""
	if len(res.Raw) > 0 {
		rawPath = filepath.Join(outDir, safePathPart(eventID), fmt.Sprintf("%s_job_%d.json", safePathPart(provider), jobID))
		if err := os.MkdirAll(filepath.Dir(rawPath), 0o755); err != nil {
			_ = store.FailDiarizationJob(ctx, jobID, err.Error())
			return fmt.Errorf("mkdir raw output: %w", err)
		}
		if err := os.WriteFile(rawPath, res.Raw, 0o644); err != nil {
			_ = store.FailDiarizationJob(ctx, jobID, err.Error())
			return fmt.Errorf("write raw output: %w", err)
		}
	}
	segments := make([]db.DiarizedSegmentParams, 0, len(res.Segments))
	for _, seg := range res.Segments {
		segments = append(segments, db.DiarizedSegmentParams{
			ClusterLabel: seg.SpeakerCluster,
			StartMS:      seg.StartMS,
			EndMS:        seg.EndMS,
			Confidence:   seg.Confidence,
			Text:         seg.Text,
			Raw: map[string]any{
				"provider": res.Provider,
				"model":    res.Model,
			},
		})
	}
	entities := make([]db.EntityMentionParams, 0, len(res.Entities))
	for _, ent := range res.Entities {
		entities = append(entities, db.EntityMentionParams{
			SourceKind:     "diarization_job",
			Extractor:      res.Provider,
			Model:          res.Model,
			EntityType:     ent.Type,
			Text:           ent.Text,
			NormalizedText: ent.NormalizedText,
			StartMS:        ent.StartMS,
			EndMS:          ent.EndMS,
			StartWord:      ent.StartWord,
			EndWord:        ent.EndWord,
			Confidence:     ent.Confidence,
			Raw:            ent.Raw,
		})
	}
	if err := store.InsertDiarizationResult(ctx, db.InsertDiarizationResultParams{JobID: jobID, TVWEventID: eventID, Provider: res.Provider, Model: res.Model, RawResultPath: rawPath, Segments: segments, Entities: entities}); err != nil {
		_ = store.FailDiarizationJob(ctx, jobID, err.Error())
		return fmt.Errorf("store result: %w", err)
	}
	fmt.Fprintf(os.Stderr, "diarize-event: stored %d segments and %d entity mentions for job_id=%d raw=%s\n", len(segments), len(entities), jobID, rawPath)

	// Auto-populate the speaker identity review queue from self-introduction
	// patterns in the freshly-stored segments. Failures here shouldn't roll
	// back the diarization itself — the backfill command can re-run safely.
	scanned, ev, tk, err := extractSpeakerEvidenceForJob(ctx, store, jobID, eventID, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "diarize-event: speaker-evidence extraction failed for job_id=%d: %v\n", jobID, err)
	} else {
		fmt.Fprintf(os.Stderr, "diarize-event: speaker-evidence scanned %d segments, upserted %d evidence rows and %d review tasks\n", scanned, ev, tk)
	}
	return nil
}
