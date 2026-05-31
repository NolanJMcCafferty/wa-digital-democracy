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
		Name:     "verify-organizations",
		Synopsis: "Cross-match seeded organizations against IRS BMF + PDC employers",
		Run:      runVerifyOrganizations,
	})
}
func runVerifyOrganizations(args []string) int {
	fs := flag.NewFlagSet("verify-organizations", flag.ContinueOnError)
	var (
		dsn             = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		dryRun          = fs.Bool("dry-run", false, "report matches without updating organization rows")
		deleteUnmatched = fs.Bool("delete-unmatched", false, "delete possible-confidence organizations that don't match any cross-source")
		quiet           = fs.Bool("quiet", false, "suppress per-row progress")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "verify-organizations: db open: %v\n", err)
		return 1
	}
	defer store.Close()
	progress := func(scanned, matched int, name string) {
		if *quiet {
			return
		}
		if scanned%200 == 0 {
			fmt.Fprintf(os.Stderr, "  scanned=%d matched=%d last=%q\n", scanned, matched, name)
		}
	}
	stats, err := store.VerifyOrganizationsAgainstSources(ctx, db.VerifyOrganizationsOptions{
		DryRun:          *dryRun,
		DeleteUnmatched: *deleteUnmatched,
	}, progress)
	if err != nil {
		fmt.Fprintf(os.Stderr, "verify-organizations: %v\n", err)
		return 1
	}
	verifyMode := "verified"
	deleteMode := "deleted"
	if *dryRun {
		verifyMode = "would verify"
		deleteMode = "would delete"
	}
	if *deleteUnmatched {
		fmt.Fprintf(os.Stderr, "verify-organizations: scanned=%d %s=%d (irs_bmf=%d, pdc_employer=%d) %s=%d unmatched\n",
			stats.Scanned, verifyMode, stats.Matched, stats.IRSMatches, stats.PDCMatches, deleteMode, stats.Deleted)
	} else {
		fmt.Fprintf(os.Stderr, "verify-organizations: scanned=%d %s=%d (irs_bmf=%d, pdc_employer=%d)\n",
			stats.Scanned, verifyMode, stats.Matched, stats.IRSMatches, stats.PDCMatches)
	}
	return 0
}
