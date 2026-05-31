package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func init() {
	register(Command{
		Name:     "list-entity-match-candidates",
		Synopsis: "List reviewable organization/entity match candidates",
		Run:      runListEntityMatchCandidates,
	})
}
func runListEntityMatchCandidates(args []string) int {
	fs := flag.NewFlagSet("list-entity-match-candidates", flag.ContinueOnError)
	var (
		sourceKind = fs.String("source-kind", "", "optional entity_match_source_kind filter")
		decision   = fs.String("decision", "", "optional decision filter: confirmed, rejected, needs_review")
		limit      = fs.Int("limit", 100, "maximum candidates to list")
		dsn        = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		jsonOut    = fs.Bool("json", false, "write candidates as JSON to stdout")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "list-entity-match-candidates: db open: %v\n", err)
		return 1
	}
	defer store.Close()
	candidates, err := store.ListVendorEntityMatchCandidates(ctx, strings.TrimSpace(*sourceKind), strings.TrimSpace(*decision), *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "list-entity-match-candidates: %v\n", err)
		return 1
	}
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(candidates); err != nil {
			fmt.Fprintf(os.Stderr, "list-entity-match-candidates: encode: %v\n", err)
			return 1
		}
		return 0
	}
	for _, c := range candidates {
		fmt.Printf("%d\t%s\t%s\t%s\torg=%d\t%s\tcandidate=%s\tdecision=%s\n",
			c.ID, c.SourceKind, c.SourceName, c.NormalizedName, c.OrganizationID,
			c.CanonicalName, c.CandidateConfidence, c.Decision)
	}
	return 0
}
