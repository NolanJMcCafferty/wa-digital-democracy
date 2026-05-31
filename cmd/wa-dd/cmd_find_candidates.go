package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/candidate"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/csi"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

func init() {
	register(Command{
		Name:     "find-candidates",
		Synopsis: "Score candidate bill/hearing pairs for a given issue",
		Run:      runFindCandidates,
	})
}
func runFindCandidates(args []string) int {
	fs := flag.NewFlagSet("find-candidates", flag.ContinueOnError)
	var (
		issue       = fs.String("issue", "housing", "issue keyword (currently: housing)")
		maxItems    = fs.Int("max", 50, "max agenda items to inspect")
		maxMeetings = fs.Int("max-meetings", 8, "max recent meetings per committee")
		out         = fs.String("out", "data/processed/candidates.json", "output JSON path")
		topN        = fs.Int("top", 10, "show top-N candidates in stderr summary")
		rateLimit   = fs.Float64("rate", 10.0, "max requests/sec to app.leg.wa.gov")
		quiet       = fs.Bool("quiet", false, "suppress per-step progress logs")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	client := httpx.New(httpx.Config{
		UserAgent:    userAgent,
		Sink:         httpx.NopSink{}, // read-only; raw bytes not persisted
		Timeout:      30 * time.Second,
		MaxRetries:   2,
		RetryBackoff: 500 * time.Millisecond,
		HostRateLimit: map[string]float64{
			"app.leg.wa.gov": *rateLimit,
		},
	})
	finder := candidate.NewFinder(csi.New(client))

	progress := func(s string) {
		if !*quiet {
			fmt.Fprintln(os.Stderr, s)
		}
	}

	start := time.Now()
	cands, err := finder.Find(ctx, candidate.FindOptions{
		Issue:              *issue,
		MaxAgendaItems:     *maxItems,
		MaxMeetingsPerComm: *maxMeetings,
		OnProgress:         progress,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "find-candidates: %v\n", err)
		return 1
	}

	if err := writeJSON(*out, map[string]any{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"issue":        *issue,
		"count":        len(cands),
		"candidates":   cands,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "find-candidates: write %s: %v\n", *out, err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "\nfind-candidates: scanned %d agenda items in %s\n",
		len(cands), time.Since(start).Round(time.Millisecond))
	fmt.Fprintf(os.Stderr, "wrote %s\n\n", *out)
	printTop(cands, *topN)
	return 0
}
