package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func init() {
	register(Command{
		Name:     "generate-deepgram-org-evidence",
		Synopsis: "Generate reviewable organization evidence from Deepgram entity mentions",
		Run:      runGenerateDeepgramOrgEvidence,
	})
}
func runGenerateDeepgramOrgEvidence(args []string) int {
	fs := flag.NewFlagSet("generate-deepgram-org-evidence", flag.ContinueOnError)
	var (
		minConfidence = fs.Float64("min-confidence", 0.85, "minimum Deepgram entity confidence")
		limit         = fs.Int("limit", 0, "maximum entity mentions to inspect")
		dsn           = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate-deepgram-org-evidence: db open: %v\n", err)
		return 1
	}
	defer store.Close()

	fmt.Fprintf(os.Stderr, "==> generate-deepgram-org-evidence: starting (min-confidence=%.2f limit=%d)\n", *minConfidence, *limit)
	stopWatchdog := make(chan struct{})
	watchdogDone := make(chan struct{})
	go func() {
		defer close(watchdogDone)
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		started := time.Now()
		for {
			select {
			case <-ticker.C:
				fmt.Fprintf(os.Stderr, "  checkpoint: still generating Deepgram organization evidence (elapsed=%s)\n", time.Since(started).Round(time.Second))
			case <-stopWatchdog:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	logProgress := func(p db.DeepgramOrganizationEvidenceProgress) {
		last := ""
		if p.LastNormalizedName != "" {
			last = " last=" + p.LastNormalizedName
		}
		switch p.Phase {
		case "querying":
			fmt.Fprintf(os.Stderr, "  checkpoint: querying Deepgram ORGANIZATION mentions\n")
		case "processing":
			fmt.Fprintf(os.Stderr, "  checkpoint: scanned=%d mentions=%d candidates=%d skipped=%d lookup_cache=%d elapsed=%s%s\n",
				p.Stats.Scanned, p.Stats.MentionsUpserted, p.Stats.Candidates,
				p.Stats.Skipped, p.LookupCacheSize, p.ElapsedTime, last)
		case "complete":
			fmt.Fprintf(os.Stderr, "  checkpoint: complete scanned=%d mentions=%d candidates=%d skipped=%d lookup_cache=%d elapsed=%s\n",
				p.Stats.Scanned, p.Stats.MentionsUpserted, p.Stats.Candidates,
				p.Stats.Skipped, p.LookupCacheSize, p.ElapsedTime)
		}
	}
	stats, err := store.GenerateDeepgramOrganizationEvidenceWithProgress(ctx, *minConfidence, *limit, logProgress)
	close(stopWatchdog)
	<-watchdogDone
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate-deepgram-org-evidence: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "==> deepgram org evidence: scanned=%d mentions=%d candidates=%d skipped=%d\n",
		stats.Scanned, stats.MentionsUpserted, stats.Candidates, stats.Skipped)
	return 0
}

// runIngestUSASpendingWAAwards implements `wa-dd ingest-usaspending-wa-awards`:
// pulls a scoped USAspending award search page for awards performed in
// Washington. The first scope is deliberately broad-but-bounded: place of
// performance = WA over a caller-specified date range, sorted by award amount.
// Entity matching remains candidate-only and reviewable; this command stores
