package main

import (
	"context"
	"encoding/json"
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
		Name:     "generate-vendor-entity-matches",
		Synopsis: "Generate reviewable vendor/customer organization match candidates",
		Run:      runGenerateVendorEntityMatches,
	})
}
func runGenerateVendorEntityMatches(args []string) int {
	fs := flag.NewFlagSet("generate-vendor-entity-matches", flag.ContinueOnError)
	var (
		limit   = fs.Int("limit", 0, "maximum source rows to inspect (0 = no practical limit)")
		dsn     = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		jsonOut = fs.Bool("json", false, "write generated candidates as JSON to stdout")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate-vendor-entity-matches: db open: %v\n", err)
		return 1
	}
	defer store.Close()

	fmt.Fprintf(os.Stderr, "==> generate-vendor-entity-matches: starting (limit=%d)\n", *limit)
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
				fmt.Fprintf(os.Stderr, "  checkpoint: still generating entity-match candidates (elapsed=%s)\n", time.Since(started).Round(time.Second))
			case <-stopWatchdog:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	logProgress := func(p db.VendorEntityMatchProgress) {
		source := ""
		if p.SourceKind != "" {
			source = " source=" + p.SourceKind
		}
		switch p.Phase {
		case "querying":
			fmt.Fprintf(os.Stderr, "  checkpoint: querying source rows and current organizations\n")
		case "processing":
			fmt.Fprintf(os.Stderr, "  checkpoint: processed=%d upserted=%d auto_confirmed=%d skipped=%d elapsed=%s%s\n",
				p.Scanned, p.Upserted, p.AutoConfirmed, p.Skipped, p.ElapsedTime, source)
		case "complete":
			fmt.Fprintf(os.Stderr, "  checkpoint: complete processed=%d upserted=%d auto_confirmed=%d skipped=%d elapsed=%s\n",
				p.Scanned, p.Upserted, p.AutoConfirmed, p.Skipped, p.ElapsedTime)
		}
	}
	candidates, err := store.GenerateVendorEntityMatchCandidatesWithProgress(ctx, *limit, logProgress)
	close(stopWatchdog)
	<-watchdogDone
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate-vendor-entity-matches: %v\n", err)
		return 1
	}
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(candidates); err != nil {
			fmt.Fprintf(os.Stderr, "generate-vendor-entity-matches: encode: %v\n", err)
			return 1
		}
	}
	fmt.Fprintf(os.Stderr, "==> generated %d reviewable vendor/entity match candidates\n", len(candidates))
	return 0
}
