package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func init() {
	register(Command{
		Name:     "prune-junk-organizations",
		Synopsis: "Delete previously-seeded organizations whose names are placeholders or self-descriptors",
		Run:      runPruneJunkOrganizations,
	})
}
func runPruneJunkOrganizations(args []string) int {
	fs := flag.NewFlagSet("prune-junk-organizations", flag.ContinueOnError)
	var (
		dsn    = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		dryRun = fs.Bool("dry-run", false, "list rows that would be deleted without modifying the DB")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prune-junk-organizations: db open: %v\n", err)
		return 1
	}
	defer store.Close()
	stats, err := store.PruneJunkOrganizations(ctx, *dryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prune-junk-organizations: %v\n", err)
		return 1
	}
	mode := "deleted"
	if *dryRun {
		mode = "would delete"
	}
	fmt.Fprintf(os.Stderr, "prune-junk-organizations: scanned=%d junk-detected=%d %s=%d kept-non-possible=%d\n",
		stats.Scanned, stats.JunkDetected, mode, stats.Deleted, stats.SkippedKeptID)
	return 0
}
