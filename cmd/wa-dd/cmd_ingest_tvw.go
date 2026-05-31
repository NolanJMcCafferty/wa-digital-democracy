package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/domain"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/jobs"
)

func init() {
	register(Command{
		Name:     "ingest-tvw",
		Synopsis: "Pull TVW/Invintus event detail + VTT",
		Run:      runIngestTVW,
	})
}
func runIngestTVW(args []string) int {
	fs := flag.NewFlagSet("ingest-tvw", flag.ContinueOnError)
	var (
		eventID   = fs.String("event-id", "", "TVW/Invintus event ID")
		dsn       = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		rawDir    = fs.String("raw-dir", "data/raw", "filesystem root for raw API responses")
		rateLimit = fs.Float64("rate", 10.0, "max requests/sec for TVW/Invintus APIs")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*eventID) == "" {
		fmt.Fprintln(os.Stderr, "ingest-tvw: --event-id is required")
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	deps, cleanup, err := newBuildDeps(ctx, *dsn, *rawDir, *rateLimit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-tvw: %v\n", err)
		return 1
	}
	defer cleanup()

	demo := &domain.BillAgendaTarget{}
	demo.TVW.EventID = strings.TrimSpace(*eventID)
	pipeline := &jobs.Pipeline{
		Store: deps.store,
		TVW:   deps.tvwClient,
		Demo:  demo,
	}
	ids := jobs.NewIDs()
	fmt.Fprintf(os.Stderr, "==> ingesting TVW/Invintus event %s\n", demo.TVW.EventID)
	if err := pipeline.IngestTVW(ctx, ids); err != nil {
		fmt.Fprintf(os.Stderr, "ingest-tvw: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "==> stored TVW event %s\n", ids.TVWEventID)
	return 0
}

// runIngestSession implements `wa-dd ingest-session`: bulk-pull every
// bill in a biennium from LWS GetLegislationByYear and run the metadata
// -only pipeline (just IngestBill) per bill. Hearings/testimony are
