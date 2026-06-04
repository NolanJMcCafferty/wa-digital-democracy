package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
)

func init() {
	register(Command{
		Name:     "daily",
		Synopsis: "Run the hosted nightly chain: roster, session, source context, hearings, entity matches",
		Run:      runDaily,
	})
}
func runDaily(args []string) int {
	fs := flag.NewFlagSet("daily", flag.ContinueOnError)
	var (
		biennium       = fs.String("biennium", "2025-26", "Biennium to ingest, e.g. 2025-26")
		dsn            = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		outDir         = fs.String("out-dir", "data/processed", "where run summary JSON files are written")
		rateLimit      = fs.Float64("rate", 10.0, "max requests/sec per legislative host for hearing/source-context steps")
		sessionRate    = fs.Float64("session-rate", 50.0, "max requests/sec for LWS session metadata")
		sessionWorkers = fs.Int("session-workers", 4, "number of ingest-session workers")
		sessionLimit   = fs.Int("session-limit", 0, "stop ingest-session after N bills (0 = no limit)")
		hearingLimit   = fs.Int("hearing-limit", 0, "stop ingest-hearings after N hearings (0 = no limit)")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	release, locked, err := acquireDailyLock(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daily: acquire lock: %v\n", err)
		return 1
	}
	if !locked {
		fmt.Fprintln(os.Stderr, "daily: another daily run is already active; exiting")
		return 0
	}
	defer release()

	base := []string{"--biennium", *biennium, "--dsn", *dsn, "--out-dir", *outDir}
	sourceBase := []string{"--dsn", *dsn}
	steps := []struct {
		name string
		code func([]string) int
		args []string
	}{
		{
			name: "ingest-legislators",
			code: runIngestLegislators,
			args: append(append([]string{}, base...), "--rate", fmt.Sprintf("%g", *rateLimit)),
		},
		{
			name: "ingest-session",
			code: runIngestSession,
			args: append(append([]string{}, base...),
				"--rate", fmt.Sprintf("%g", *sessionRate),
				"--workers", strconv.Itoa(*sessionWorkers),
				"--limit", strconv.Itoa(*sessionLimit),
			),
		},
		{
			name: "ingest-irs-bmf-wa",
			code: runIngestIRSBMFWA,
			args: append(append([]string{}, sourceBase...), "--rate", fmt.Sprintf("%g", *rateLimit)),
		},
		{
			name: "ingest-pdc-employers",
			code: runIngestPDCEmployers,
			args: append(append([]string{}, sourceBase...), "--rate", fmt.Sprintf("%g", *rateLimit)),
		},
		{
			name: "ingest-hearings",
			code: runIngestHearings,
			args: append(append([]string{}, base...),
				"--rate", fmt.Sprintf("%g", *rateLimit),
				"--limit", strconv.Itoa(*hearingLimit),
			),
		},
		{
			name: "verify-organizations",
			code: runVerifyOrganizations,
			args: []string{"--dsn", *dsn, "--quiet"},
		},
		{
			name: "generate-vendor-entity-matches",
			code: runGenerateVendorEntityMatches,
			args: []string{"--dsn", *dsn},
		},
	}
	for _, step := range steps {
		fmt.Fprintf(os.Stderr, "==> daily: %s\n", step.name)
		if code := step.code(step.args); code != 0 {
			fmt.Fprintf(os.Stderr, "daily: %s failed with exit code %d\n", step.name, code)
			return code
		}
	}
	fmt.Fprintln(os.Stderr, "==> daily: complete")
	return 0
}
