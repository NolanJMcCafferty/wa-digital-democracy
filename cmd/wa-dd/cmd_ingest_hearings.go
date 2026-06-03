package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/common"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/diarization"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/jobs"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func init() {
	register(Command{
		Name:     "ingest-hearings",
		Synopsis: "Run the full pipeline for every discovered hearing",
		Run:      runIngestHearings,
	})
}
func runIngestHearings(args []string) int {
	fs := flag.NewFlagSet("ingest-hearings", flag.ContinueOnError)
	var (
		biennium               = fs.String("biennium", "2025-26", "Biennium to scan, e.g. 2025-26")
		dsn                    = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		rawDir                 = fs.String("raw-dir", "data/raw", "filesystem root for raw API responses")
		outDir                 = fs.String("out-dir", "data/processed", "where _ingest.json is written")
		rateLimit              = fs.Float64("rate", 10.0, "max requests/sec per legislative host")
		limit                  = fs.Int("limit", 0, "stop after N hearings (0 = no limit). For smoke tests.")
		workers                = fs.Int("workers", 4, "number of hearings to ingest in parallel")
		diarizationConcurrency = fs.Int("diarization-concurrency", 2, "max provider diarization jobs to run in parallel")
		provider               = fs.String("provider", "pyannoteai", "diarization provider: pyannoteai or deepgram")
		model                  = fs.String("model", "precision-2", "provider model")
		diarizationOutDir      = fs.String("diarization-out-dir", "data/processed/diarization", "raw diarization JSON output root")
		apiKey                 = fs.String("api-key", "", "provider API key (defaults to PYANNOTEAI_API_KEY or DEEPGRAM_API_KEY based on --provider)")
		useURL                 = fs.Bool("use-source-url", true, "send original TVW/Invintus URL to provider instead of uploading local normalized WAV")
		maxAttempts            = fs.Int("max-attempts", 5, "max diarization attempts per event before giving up")
		transcription          = fs.Bool("transcription", true, "(pyannoteai only) request transcription with diarization so segments include text")
		asrModel               = fs.String("asr-model", "faster-whisper-large-v3-turbo", "(pyannoteai only) ASR backend: faster-whisper-large-v3-turbo or parakeet-tdt-0.6b-v3")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	deps, cleanup, err := newBuildDeps(ctx, *dsn, *rawDir, *rateLimit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-hearings: %v\n", err)
		return 1
	}
	defer cleanup()

	rows, err := deps.store.ListDiscoveredHearingsForIngest(ctx, *biennium)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-hearings: %v\n", err)
		return 1
	}
	if *limit > 0 && len(rows) > *limit {
		rows = rows[:*limit]
	}
	fmt.Fprintf(os.Stderr, "==> %d hearings to ingest\n", len(rows))
	if len(rows) == 0 {
		return 0
	}

	key := *apiKey
	if strings.TrimSpace(key) == "" {
		switch strings.ToLower(*provider) {
		case "deepgram":
			key = env("DEEPGRAM_API_KEY", "")
		default:
			key = env("PYANNOTEAI_API_KEY", "")
		}
	}
	if strings.ToLower(*provider) == "deepgram" && *model == "precision-2" {
		*model = "nova-3"
	}
	diarizer, err := newDiarizationProvider(*provider, key, *model, *transcription, *asrModel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-hearings: %v\n", err)
		return 2
	}

	type result struct {
		HearingID       int64  `json:"hearing_id"`
		TVWEventID      string `json:"tvw_event_id"`
		AgendaItemCount int    `json:"agenda_item_count"`
		Status          string `json:"status"`
		DurationMS      int64  `json:"duration_ms"`
		Error           string `json:"error,omitempty"`
	}
	results := make([]result, len(rows))
	startedAt := time.Now()
	var failures atomic.Int64

	workerCount := *workers
	if workerCount < 1 {
		workerCount = 1
	}
	if workerCount > len(rows) {
		workerCount = len(rows)
	}
	diarizationSlots := *diarizationConcurrency
	if diarizationSlots < 1 {
		diarizationSlots = 1
	}
	diarizationSem := make(chan struct{}, diarizationSlots)
	attempts := *maxAttempts
	if attempts < 1 {
		attempts = 1
	}
	fmt.Fprintf(os.Stderr, "==> hearing workers=%d diarization-concurrency=%d\n", workerCount, diarizationSlots)

	type workItem struct {
		index int
		row   db.DiscoveredHearingRow
	}
	work := make(chan workItem)
	var wg sync.WaitGroup
	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range work {
				r := item.row
				prefix := fmt.Sprintf("[%d/%d] hearing=%d event=%s", item.index+1, len(rows), r.HearingID, r.TVWEventID)
				fmt.Fprintf(os.Stderr, "==> %s starting\n", prefix)
				t0 := time.Now()
				logf := func(s string) { fmt.Fprintf(os.Stderr, "    %s\n", s) }
				err := ingestHearing(ctx, deps, r, diarizer, *provider, *model, *diarizationOutDir, *useURL, attempts, diarizationSem, logf)
				dur := time.Since(t0)
				res := result{
					HearingID:       r.HearingID,
					TVWEventID:      r.TVWEventID,
					AgendaItemCount: r.AgendaItemCount,
					DurationMS:      dur.Milliseconds(),
				}
				if err != nil {
					failures.Add(1)
					res.Status = "failed"
					res.Error = err.Error()
					fmt.Fprintf(os.Stderr, "    %s FAIL (%s): %v\n", prefix, dur.Round(time.Millisecond), err)
				} else {
					res.Status = "ok"
					fmt.Fprintf(os.Stderr, "    %s ok (%s)\n", prefix, dur.Round(time.Millisecond))
				}
				results[item.index] = res
			}
		}()
	}
	for i, r := range rows {
		if ctx.Err() != nil {
			break
		}
		work <- workItem{index: i, row: r}
	}
	close(work)
	wg.Wait()

	failureCount := int(failures.Load())
	summary := struct {
		StartedAt  time.Time `json:"started_at"`
		FinishedAt time.Time `json:"finished_at"`
		Biennium   string    `json:"biennium"`
		Total      int       `json:"total"`
		Succeeded  int       `json:"succeeded"`
		Failed     int       `json:"failed"`
		Results    []result  `json:"results"`
	}{
		StartedAt: startedAt, FinishedAt: time.Now(),
		Biennium: *biennium, Total: len(results),
		Succeeded: len(results) - failureCount, Failed: failureCount,
		Results: results,
	}
	if err := writeJSON(filepath.Join(*outDir, "_ingest.json"), summary); err != nil {
		fmt.Fprintf(os.Stderr, "ingest-hearings: write _ingest.json: %v\n", err)
		if failureCount == 0 {
			return 1
		}
	}
	fmt.Fprintf(os.Stderr, "==> done: %d ok, %d failed (%s)\n",
		summary.Succeeded, summary.Failed,
		summary.FinishedAt.Sub(summary.StartedAt).Round(time.Millisecond))
	if failureCount > 0 {
		return 1
	}
	return 0
}

func ingestHearing(
	ctx context.Context,
	deps *buildDeps,
	hearing db.DiscoveredHearingRow,
	diarizer diarization.Provider,
	provider, model, diarizationOutDir string,
	useURL bool,
	maxAttempts int,
	diarizationSem chan struct{},
	logf func(string),
) error {
	runID, err := deps.store.StartIngestionRun(ctx, "hearing-pipeline", map[string]any{
		"hearing_id":   hearing.HearingID,
		"tvw_event_id": hearing.TVWEventID,
	})
	if err != nil {
		return fmt.Errorf("start hearing-pipeline run: %w", err)
	}
	stepErr := ingestHearingSteps(ctx, deps, hearing, diarizer, provider, model, diarizationOutDir, useURL, maxAttempts, diarizationSem, logf)
	status := "succeeded"
	if stepErr != nil {
		status = "failed"
	}
	if err := deps.store.FinishIngestionRun(ctx, runID, status, 0, 0, stepErr); err != nil {
		logf(fmt.Sprintf("warning: finish hearing-pipeline: %v", err))
	}
	if stepErr != nil {
		return stepErr
	}
	return nil
}

func ingestHearingSteps(
	ctx context.Context,
	deps *buildDeps,
	hearing db.DiscoveredHearingRow,
	diarizer diarization.Provider,
	provider, model, diarizationOutDir string,
	useURL bool,
	maxAttempts int,
	diarizationSem chan struct{},
	logf func(string),
) error {
	items, err := deps.store.ListAgendaItemsForHearing(ctx, hearing.HearingID)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return fmt.Errorf("hearing %d has no discovered agenda items", hearing.HearingID)
	}

	logf(fmt.Sprintf("==> ingest-csi (%d agenda items)", len(items)))
	for i := range items {
		item := &items[i]
		pipeline := hearingItemPipeline(deps, *item)
		ids := jobs.NewIDs()
		ids.HearingID = hearing.HearingID
		ids.BillID = item.BillID
		if err := pipeline.IngestCSI(ctx, ids); err != nil {
			return fmt.Errorf("ingest-csi agenda=%s: %w", item.CSIAgendaItemID, err)
		}
		item.AgendaItemID = ids.AgendaItemID
	}

	if err := withEventAdvisoryLock(ctx, deps.store, hearing.TVWEventID, func() error {
		logf("==> ingest-tvw")
		tvwPipeline := &jobs.Pipeline{
			Store:            deps.store,
			TVW:              deps.tvwClient,
			BillAgendaTarget: &common.BillAgendaTarget{TVW: common.TVWRef{EventID: hearing.TVWEventID}},
		}
		tvwIDs := jobs.NewIDs()
		tvwIDs.HearingID = hearing.HearingID
		if err := tvwPipeline.IngestTVW(ctx, tvwIDs); err != nil {
			return fmt.Errorf("ingest-tvw event=%s: %w", hearing.TVWEventID, err)
		}

		logf("==> diarize-event")
		if err := ensureDiarizationSucceeded(ctx, deps.store, diarizer, hearing.TVWEventID, provider, model, diarizationOutDir, useURL, maxAttempts, diarizationSem); err != nil {
			return fmt.Errorf("diarize-event %s: %w", hearing.TVWEventID, err)
		}
		return nil
	}); err != nil {
		return err
	}

	logf(fmt.Sprintf("==> segment-transcript (%d agenda items)", len(items)))
	for _, item := range items {
		pipeline := hearingItemPipeline(deps, item)
		ids := jobs.NewIDs()
		ids.AgendaItemID = item.AgendaItemID
		ids.TVWEventID = hearing.TVWEventID
		if err := pipeline.SegmentTranscript(ctx, ids); err != nil {
			return fmt.Errorf("segment-transcript agenda=%s: %w", item.CSIAgendaItemID, err)
		}
	}

	logf("==> populate-organizations")
	stats, err := deps.store.PopulateOrganizationsFromCSIForHearing(ctx, hearing.HearingID)
	if err != nil {
		return fmt.Errorf("populate-organizations hearing=%d: %w", hearing.HearingID, err)
	}
	logf(fmt.Sprintf("seeded %d organizations from CSI and linked %d testifier rows", stats.OrganizationsUpserted, stats.TestifiersLinked))
	return nil
}

func hearingItemPipeline(deps *buildDeps, item db.HearingAgendaItemRow) *jobs.Pipeline {
	return &jobs.Pipeline{
		Store: deps.store,
		CSI:   deps.csiClient,
		TVW:   deps.tvwClient,
		BillAgendaTarget: &common.BillAgendaTarget{
			Bill: common.BillKey{Biennium: item.Biennium, Prefix: item.BillPrefix, Number: item.BillNumber},
			Committee: common.CommitteeRef{
				Chamber: item.Chamber,
				Acronym: item.CommitteeAcronym,
			},
			AgendaItem: common.AgendaItemRef{
				CSIMeetingFamilyID:    item.CSIMeetingFamilyID,
				CSIAgendaItemFamilyID: item.CSIAgendaItemFamilyID,
				CSIAgendaItemID:       item.CSIAgendaItemID,
				Label:                 item.Label,
			},
			TVW: common.TVWRef{EventID: item.TVWEventID},
		},
	}
}

func ensureDiarizationSucceeded(
	ctx context.Context,
	store *db.Store,
	diarizer diarization.Provider,
	eventID, provider, model, outDir string,
	useURL bool,
	maxAttempts int,
	diarizationSem chan struct{},
) error {
	ok, err := store.HasSucceededDiarization(ctx, eventID)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}

	select {
	case diarizationSem <- struct{}{}:
		defer func() { <-diarizationSem }()
	case <-ctx.Done():
		return ctx.Err()
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := diarizeOneEvent(ctx, store, diarizer, eventID, provider, model, outDir, useURL); err != nil {
			lastErr = err
			if attempt < maxAttempts {
				wait := time.Duration(1<<attempt) * time.Second
				if wait > 60*time.Second {
					wait = 60 * time.Second
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(wait):
				}
			}
			continue
		}
		return nil
	}
	return lastErr
}

func withEventAdvisoryLock(ctx context.Context, store *db.Store, eventID string, fn func() error) error {
	conn, err := store.Pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire advisory-lock connection: %w", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1::text, 0))`, eventID); err != nil {
		return fmt.Errorf("acquire event advisory lock: %w", err)
	}
	defer func() {
		var unlocked bool
		_ = conn.QueryRow(context.Background(), `SELECT pg_advisory_unlock(hashtextextended($1::text, 0))`, eventID).Scan(&unlocked)
	}()
	return fn()
}
