package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/usaspending"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func init() {
	register(Command{
		Name:     "ingest-usaspending-wa-awards",
		Synopsis: "Pull USAspending award rows performed in Washington into Postgres",
		Run:      runIngestUSASpendingWAAwards,
	})
}
func runIngestUSASpendingWAAwards(args []string) int {
	fs := flag.NewFlagSet("ingest-usaspending-wa-awards", flag.ContinueOnError)
	var (
		startDate = fs.String("start-date", "2025-10-01", "award action date range start, YYYY-MM-DD")
		endDate   = fs.String("end-date", "2026-09-30", "award action date range end, YYYY-MM-DD")
		limit     = fs.Int("limit", 100, "maximum awards to request from USAspending")
		dsn       = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		rateLimit = fs.Float64("rate", 10.0, "max requests/sec for api.usaspending.gov")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *limit <= 0 || *limit > 100 {
		fmt.Fprintln(os.Stderr, "ingest-usaspending-wa-awards: --limit must be between 1 and 100 for the first MVP page")
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-usaspending-wa-awards: db open: %v\n", err)
		return 1
	}
	defer store.Close()
	httpClient := httpx.New(httpx.Config{
		UserAgent:    userAgent,
		Timeout:      45 * time.Second,
		MaxRetries:   2,
		RetryBackoff: 750 * time.Millisecond,
		HostRateLimit: map[string]float64{
			"api.usaspending.gov": *rateLimit,
		},
	})
	client := usaspending.New(httpClient)
	req := usaspending.WashingtonAwardSearchRequest(*startDate, *endDate)
	req.Limit = *limit
	resp, _, err := client.SearchAwardsWithSource(ctx, req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-usaspending-wa-awards: fetch: %v\n", err)
		return 1
	}

	var upserted int
	for _, row := range resp.Results {
		award := usaspending.NormalizeAward(row)
		if award.AwardID == "" {
			fmt.Fprintln(os.Stderr, "ingest-usaspending-wa-awards: skipping award with empty Award ID")
			continue
		}
		if err := store.UpsertFederalAward(ctx, db.UpsertFederalAwardParams{
			AwardID:        award.AwardID,
			RecipientName:  award.RecipientName,
			RecipientUEI:   award.RecipientUEI,
			AwardingAgency: award.AwardingAgency,
			FundingAgency:  award.FundingAgency,
			AwardType:      award.AwardType,
			AwardAmount:    normalizeMoney(award.AwardAmount),
			StartDate:      award.StartDate,
			EndDate:        award.EndDate,
			PlaceStateCode: award.PlaceStateCode,
			PlaceCounty:    award.PlaceCounty,
			RawFields:      award.Raw,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "ingest-usaspending-wa-awards: upsert award %s: %v\n", award.AwardID, err)
			return 1
		}
		upserted++
	}
	fmt.Fprintf(os.Stderr, "==> ingested %d USAspending WA award rows\n", upserted)
	return 0
}

// runIngestSeattleOperatingBudget implements `wa-dd ingest-seattle-operating-budget`:
// pulls the City of Seattle Operating Budget Socrata dataset into a normalized
