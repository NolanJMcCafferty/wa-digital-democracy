package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/pageassembly"
)

func init() {
	register(Command{
		Name:     "ingest-hearings",
		Synopsis: "Run the full pipeline for every discovered agenda item",
		Run:      runIngestHearings,
	})
}
func runIngestHearings(args []string) int {
	fs := flag.NewFlagSet("ingest-hearings", flag.ContinueOnError)
	var (
		biennium  = fs.String("biennium", "2025-26", "Biennium to scan, e.g. 2025-26")
		dsn       = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		rawDir    = fs.String("raw-dir", "data/raw", "filesystem root for raw API responses")
		outDir    = fs.String("out-dir", "data/processed", "where _ingest.json is written")
		rateLimit = fs.Float64("rate", 10.0, "max requests/sec per legislative host")
		limit     = fs.Int("limit", 0, "stop after N agenda items (0 = no limit). For smoke tests.")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	deps, cleanup, err := newBuildDeps(ctx, *dsn, *rawDir, *rateLimit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-hearings: %v\n", err)
		return 1
	}
	defer cleanup()

	rows, err := deps.store.ListDiscoveredAgendaItems(ctx, *biennium)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-hearings: %v\n", err)
		return 1
	}
	if *limit > 0 && len(rows) > *limit {
		rows = rows[:*limit]
	}
	fmt.Fprintf(os.Stderr, "==> %d agenda items to ingest\n", len(rows))

	type result struct {
		Bill            string `json:"bill"`
		CSIAgendaItemID string `json:"csi_agenda_item_id"`
		Status          string `json:"status"`
		DurationMS      int64  `json:"duration_ms"`
		Error           string `json:"error,omitempty"`
	}
	results := make([]result, 0, len(rows))
	startedAt := time.Now()
	failures := 0

	for i, r := range rows {
		demo, err := pageassembly.LookupBillAgendaTargetByAgendaItem(ctx, deps.store, r.CSIAgendaItemID)
		if err != nil {
			failures++
			fmt.Fprintf(os.Stderr, "[%d/%d] %s lookup FAIL: %v\n", i+1, len(rows), r.CSIAgendaItemID, err)
			results = append(results, result{
				Bill:            fmt.Sprintf("%s %d", r.BillPrefix, r.BillNumber),
				CSIAgendaItemID: r.CSIAgendaItemID,
				Status:          "failed", Error: err.Error(),
			})
			continue
		}
		prefix := fmt.Sprintf("[%d/%d] %s", i+1, len(rows), demo.Bill.ID())
		fmt.Fprintf(os.Stderr, "==> %s starting (agenda=%s)\n", prefix, r.CSIAgendaItemID)
		t0 := time.Now()
		logf := func(s string) { fmt.Fprintf(os.Stderr, "    %s\n", s) }
		err = ingestOne(ctx, deps, demo, logf)
		dur := time.Since(t0)
		if err != nil {
			failures++
			fmt.Fprintf(os.Stderr, "    %s FAIL (%s): %v\n", prefix, dur.Round(time.Millisecond), err)
			results = append(results, result{
				Bill: demo.Bill.ID(), CSIAgendaItemID: r.CSIAgendaItemID,
				Status: "failed", Error: err.Error(),
				DurationMS: dur.Milliseconds(),
			})
			if ctx.Err() != nil {
				break
			}
			continue
		}
		fmt.Fprintf(os.Stderr, "    %s ok (%s)\n", prefix, dur.Round(time.Millisecond))
		results = append(results, result{
			Bill: demo.Bill.ID(), CSIAgendaItemID: r.CSIAgendaItemID,
			Status: "ok", DurationMS: dur.Milliseconds(),
		})
	}

	summary := struct {
		StartedAt  time.Time `json:"started_at"`
		FinishedAt time.Time `json:"finished_at"`
		Biennium   string    `json:"biennium"`
		Total      int       `json:"total"`
		Succeeded  int       `json:"succeeded"`
		Failed     int       `json:"failed"`
		Results    []result  `json:"results"`
	}{
		StartedAt: startedAt, FinishedAt: time.Now(),
		Biennium: *biennium, Total: len(results),
		Succeeded: len(results) - failures, Failed: failures,
		Results: results,
	}
	if err := writeJSON(filepath.Join(*outDir, "_ingest.json"), summary); err != nil {
		fmt.Fprintf(os.Stderr, "ingest-hearings: write _ingest.json: %v\n", err)
		if failures == 0 {
			return 1
		}
	}
	fmt.Fprintf(os.Stderr, "==> done: %d ok, %d failed (%s)\n",
		summary.Succeeded, summary.Failed,
		summary.FinishedAt.Sub(summary.StartedAt).Round(time.Millisecond))
	if failures > 0 {
		return 1
	}
	return 0
}

// runIngestContracts implements `wa-dd ingest-contracts`: pulls one DataWA
// agency-contract fiscal-year dataset into the normalized datawa_contract table.
