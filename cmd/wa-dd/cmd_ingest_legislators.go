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

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/lws"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func init() {
	register(Command{
		Name:     "ingest-legislators",
		Synopsis: "Pull the full House+Senate roster for a biennium from LWS",
		Run:      runIngestLegislators,
	})
}
func runIngestLegislators(args []string) int {
	fs := flag.NewFlagSet("ingest-legislators", flag.ContinueOnError)
	var (
		biennium  = fs.String("biennium", "2025-26", "Biennium to fetch, e.g. 2025-26")
		dsn       = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		rawDir    = fs.String("raw-dir", "data/raw", "filesystem root for raw API responses")
		outDir    = fs.String("out-dir", "data/processed", "where _legislators.json is written")
		rateLimit = fs.Float64("rate", 10.0, "max requests/sec for the LWS host")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	deps, cleanup, err := newMetadataDeps(ctx, *dsn, *rawDir, *rateLimit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-legislators: %v\n", err)
		return 1
	}
	defer cleanup()

	fmt.Fprintf(os.Stderr, "==> GetSenateSponsors(%s)\n", *biennium)
	senate, err := deps.lwsClient.GetSenateSponsors(ctx, *biennium)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-legislators: senate: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "    received %d senators\n", len(senate))

	fmt.Fprintf(os.Stderr, "==> GetHouseSponsors(%s)\n", *biennium)
	house, err := deps.lwsClient.GetHouseSponsors(ctx, *biennium)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-legislators: house: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "    received %d representatives\n", len(house))

	type result struct {
		LWSID   string `json:"lws_sponsor_id"`
		Name    string `json:"name"`
		Chamber string `json:"chamber"`
		Status  string `json:"status"`
		Error   string `json:"error,omitempty"`
	}
	results := make([]result, 0, len(senate)+len(house))
	startedAt := time.Now()
	failures := 0

	upsert := func(m lws.Member) {
		_, err := deps.store.UpsertLegislatorRosterMembership(ctx, db.UpsertLegislatorParams{
			Biennium:     *biennium,
			LWSSponsorID: m.ID,
			Name:         m.LongName,
			Chamber:      m.Agency,
			District:     m.District,
			Party:        m.Party,
			FirstName:    m.FirstName,
			LastName:     m.LastName,
			Email:        m.Email,
			Phone:        m.Phone,
			Acronym:      m.Acronym,
		})
		if err != nil {
			failures++
			fmt.Fprintf(os.Stderr, "  FAIL %s %s: %v\n", m.Agency, m.LongName, err)
			results = append(results, result{
				LWSID: m.ID, Name: m.LongName, Chamber: m.Agency,
				Status: "failed", Error: err.Error(),
			})
			return
		}
		results = append(results, result{
			LWSID: m.ID, Name: m.LongName, Chamber: m.Agency,
			Status: "ok",
		})
	}

	for _, m := range senate {
		upsert(m)
	}
	for _, m := range house {
		upsert(m)
	}

	summary := struct {
		StartedAt  time.Time `json:"started_at"`
		FinishedAt time.Time `json:"finished_at"`
		Biennium   string    `json:"biennium"`
		Senate     int       `json:"senate"`
		House      int       `json:"house"`
		Total      int       `json:"total"`
		Failed     int       `json:"failed"`
		Results    []result  `json:"results"`
	}{
		StartedAt:  startedAt,
		FinishedAt: time.Now(),
		Biennium:   *biennium,
		Senate:     len(senate),
		House:      len(house),
		Total:      len(results),
		Failed:     failures,
		Results:    results,
	}
	if err := writeJSON(filepath.Join(*outDir, "_legislators.json"), summary); err != nil {
		fmt.Fprintf(os.Stderr, "ingest-legislators: write _legislators.json: %v\n", err)
		if failures == 0 {
			return 1
		}
	}
	fmt.Fprintf(os.Stderr, "==> done: %d senators + %d reps = %d (%d failed) in %s\n",
		len(senate), len(house), len(results), failures,
		summary.FinishedAt.Sub(summary.StartedAt).Round(time.Millisecond))
	if failures > 0 {
		return 1
	}
	return 0
}
