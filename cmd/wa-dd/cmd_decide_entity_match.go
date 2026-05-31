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
		Name:     "decide-entity-match",
		Synopsis: "Record a confirmed/rejected/needs_review entity-match decision",
		Run:      runDecideEntityMatch,
	})
}
func runDecideEntityMatch(args []string) int {
	fs := flag.NewFlagSet("decide-entity-match", flag.ContinueOnError)
	var (
		candidateID = fs.Int64("candidate-id", 0, "vendor_entity_match_candidate id")
		decision    = fs.String("decision", "needs_review", "decision: confirmed, rejected, needs_review")
		confidence  = fs.String("confidence", "possible", "reviewed confidence: confirmed, probable, possible, unmatched")
		reviewer    = fs.String("reviewed-by", env("USER", "operator"), "reviewer/operator label")
		notes       = fs.String("notes", "", "review notes")
		orgID       = fs.Int64("organization-id", 0, "override organization id; defaults to candidate organization")
		dsn         = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *candidateID <= 0 {
		fmt.Fprintln(os.Stderr, "decide-entity-match: --candidate-id is required")
		return 2
	}
	switch *decision {
	case "confirmed", "rejected", "needs_review":
	default:
		fmt.Fprintln(os.Stderr, "decide-entity-match: --decision must be confirmed, rejected, or needs_review")
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "decide-entity-match: db open: %v\n", err)
		return 1
	}
	defer store.Close()
	organizationID := *orgID
	if organizationID == 0 {
		if err := store.Pool.QueryRow(ctx, `SELECT organization_id FROM vendor_entity_match_candidate WHERE id = $1`, *candidateID).Scan(&organizationID); err != nil {
			fmt.Fprintf(os.Stderr, "decide-entity-match: lookup candidate %d: %v\n", *candidateID, err)
			return 1
		}
	}
	decisionID, err := store.UpsertVendorEntityMatchDecision(ctx, db.InsertVendorEntityMatchDecisionParams{
		CandidateID:    *candidateID,
		OrganizationID: organizationID,
		Decision:       *decision,
		Confidence:     *confidence,
		ReviewedBy:     *reviewer,
		ReviewNotes:    *notes,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "decide-entity-match: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "==> recorded entity-match decision %d for candidate %d\n", decisionID, *candidateID)
	return 0
}
