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
		Synopsis: "Run the hosted nightly chain: roster, session, hearings, PDC",
		Run:      runDaily,
	})
}
func runDaily(args []string) int {
	fs := flag.NewFlagSet("daily", flag.ContinueOnError)
	var (
		biennium       = fs.String("biennium", "2025-26", "Biennium to ingest, e.g. 2025-26")
		dsn            = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		rawDir         = fs.String("raw-dir", "data/raw", "filesystem root for raw API responses when OBJECT_STORE=local")
		outDir         = fs.String("out-dir", "data/processed", "where run summary JSON files are written")
		rateLimit      = fs.Float64("rate", 10.0, "max requests/sec per legislative host for discovery/hearing steps")
		sessionRate    = fs.Float64("session-rate", 25.0, "max requests/sec for LWS session metadata")
		sessionWorkers = fs.Int("session-workers", 4, "number of ingest-session workers")
		sessionLimit   = fs.Int("session-limit", 0, "stop ingest-session after N bills (0 = no limit)")
		hearingLimit   = fs.Int("hearing-limit", 0, "stop discover/ingest-hearings after N hearings (0 = no limit)")
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

	base := []string{"--biennium", *biennium, "--dsn", *dsn, "--raw-dir", *rawDir, "--out-dir", *outDir}
	pdcBase := []string{"--dsn", *dsn, "--raw-dir", *rawDir}
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
			name: "discover-hearings",
			code: runDiscoverHearings,
			args: append(append([]string{}, base...),
				"--rate", fmt.Sprintf("%g", *rateLimit),
				"--limit", strconv.Itoa(*hearingLimit),
			),
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
			name: "ingest-pdc-employers",
			code: runIngestPDCEmployers,
			args: append(append([]string{}, pdcBase...), "--rate", fmt.Sprintf("%g", *rateLimit)),
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
