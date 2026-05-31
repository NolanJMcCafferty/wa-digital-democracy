package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func init() {
	register(Command{
		Name:     "populate-organizations",
		Synopsis: "Seed organization rows from CSI testimony organization strings",
		Run:      runPopulateOrganizations,
	})
}
func runPopulateOrganizations(args []string) int {
	fs := flag.NewFlagSet("populate-organizations", flag.ContinueOnError)
	var (
		dsn     = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		jsonOut = fs.Bool("json", false, "write summary as JSON to stdout")
		quiet   = fs.Bool("quiet", false, "suppress progress logs")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "populate-organizations: db open: %v\n", err)
		return 1
	}
	defer store.Close()
	logProgress := func(p db.PopulateOrganizationsProgress) {
		if *quiet {
			return
		}
		switch p.Phase {
		case "listing":
			fmt.Fprintln(os.Stderr, "==> scanning distinct CSI organization names")
		case "processing":
			if p.Stats.CandidatesProcessed == 0 {
				fmt.Fprintln(os.Stderr, "==> processing CSI organization candidates")
				return
			}
			fmt.Fprintf(os.Stderr, "  processed=%d organizations=%d mentions=%d linked=%d skipped=%d elapsed=%s current=%q\n",
				p.Stats.CandidatesProcessed,
				p.Stats.OrganizationsUpserted,
				p.Stats.MentionsUpserted,
				p.Stats.TestifiersLinked,
				p.Stats.Skipped,
				p.ElapsedTime,
				p.SourceName,
			)
		case "complete":
			fmt.Fprintf(os.Stderr, "==> completed CSI organization population in %s\n", p.ElapsedTime)
		}
	}
	stats, err := store.PopulateOrganizationsFromCSIWithProgress(ctx, logProgress)
	if err != nil {
		fmt.Fprintf(os.Stderr, "populate-organizations: %v\n", err)
		return 1
	}
	if *jsonOut {
		_ = json.NewEncoder(os.Stdout).Encode(stats)
	}
	fmt.Fprintf(os.Stderr, "==> seeded %d organizations from CSI, verified %d via cross-source, recorded %d mentions, linked %d testifiers, skipped %d junk names (%d candidates processed)\n", stats.OrganizationsUpserted, stats.Verified, stats.MentionsUpserted, stats.TestifiersLinked, stats.Skipped, stats.CandidatesProcessed)
	return 0
}
