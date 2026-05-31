package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func init() {
	register(Command{
		Name:     "diarize-pending",
		Synopsis: "Diarize every TVW event that has not yet been successfully diarized",
		Run:      runDiarizePending,
	})
}
func runDiarizePending(args []string) int {
	fs := flag.NewFlagSet("diarize-pending", flag.ContinueOnError)
	var (
		dsn         = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		provider    = fs.String("provider", "deepgram", "diarization provider (currently: deepgram)")
		model       = fs.String("model", "nova-3", "provider model")
		outDir      = fs.String("out-dir", "data/processed/diarization", "raw diarization JSON output root")
		apiKey      = fs.String("api-key", env("DEEPGRAM_API_KEY", ""), "provider API key (defaults to DEEPGRAM_API_KEY)")
		useURL      = fs.Bool("use-source-url", true, "send original TVW/Invintus URL to provider instead of uploading local normalized WAV")
		limit       = fs.Int("limit", 0, "max events to diarize (0 = all pending)")
		dryRun      = fs.Bool("dry-run", false, "print pending event IDs without diarizing them")
		concurrency = fs.Int("concurrency", 10, "max diarization jobs to run in parallel")
		maxAttempts = fs.Int("max-attempts", 5, "max attempts per event before giving up")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "diarize-pending: db open: %v\n", err)
		return 1
	}
	defer store.Close()

	pending, err := store.ListEventsPendingDiarization(ctx, *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "diarize-pending: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "diarize-pending: %d events without a successful diarization_job\n", len(pending))
	if *dryRun {
		for _, id := range pending {
			fmt.Println(id)
		}
		return 0
	}
	if len(pending) == 0 {
		return 0
	}

	p, err := newDiarizationProvider(*provider, *apiKey, *model)
	if err != nil {
		fmt.Fprintf(os.Stderr, "diarize-pending: %v\n", err)
		return 2
	}

	workers := *concurrency
	if workers < 1 {
		workers = 1
	}
	if workers > len(pending) {
		workers = len(pending)
	}
	fmt.Fprintf(os.Stderr, "diarize-pending: running with concurrency=%d\n", workers)

	jobs := make(chan struct {
		index int
		id    string
	})
	var wg sync.WaitGroup
	var failed atomic.Int64
	var done atomic.Int64

	attempts := *maxAttempts
	if attempts < 1 {
		attempts = 1
	}

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if err := ctx.Err(); err != nil {
					return
				}
				n := done.Add(1)
				fmt.Fprintf(os.Stderr, "==> [%d/%d] %s\n", n, len(pending), j.id)
				var lastErr error
				for attempt := 1; attempt <= attempts; attempt++ {
					if err := ctx.Err(); err != nil {
						lastErr = err
						break
					}
					err := diarizeOneEvent(ctx, store, p, j.id, *provider, *model, *outDir, *useURL)
					if err == nil {
						lastErr = nil
						break
					}
					lastErr = err
					if attempt >= attempts {
						break
					}
					wait := time.Duration(1<<attempt) * time.Second
					if wait > 60*time.Second {
						wait = 60 * time.Second
					}
					fmt.Fprintf(os.Stderr, "diarize-pending: event %s attempt %d/%d failed: %v — retrying in %s\n", j.id, attempt, attempts, err, wait)
					select {
					case <-ctx.Done():
						lastErr = ctx.Err()
					case <-time.After(wait):
						continue
					}
					break
				}
				if lastErr != nil {
					failed.Add(1)
					fmt.Fprintf(os.Stderr, "diarize-pending: event %s: gave up after %d attempts: %v\n", j.id, attempts, lastErr)
				}
			}
		}()
	}

	for i, id := range pending {
		select {
		case <-ctx.Done():
			fmt.Fprintf(os.Stderr, "diarize-pending: aborted: %v\n", ctx.Err())
			close(jobs)
			wg.Wait()
			return 1
		case jobs <- struct {
			index int
			id    string
		}{i, id}:
		}
	}
	close(jobs)
	wg.Wait()

	failedN := int(failed.Load())
	fmt.Fprintf(os.Stderr, "diarize-pending: done — %d succeeded, %d failed\n", len(pending)-failedN, failedN)
	if failedN > 0 {
		return 1
	}
	return 0
}

// runMergeSegments rewrites diarized_speech_segment rows so that adjacent
// segments belonging to the same speaker cluster are collapsed into one row
// per speaker turn. New diarizations already get this in-pipeline (see
// diarization.MergeConsecutiveSegments inside parseDeepgram); this command
// exists to re-merge events processed before that change shipped, by
