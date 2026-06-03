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
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/seattle"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/socrata"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func init() {
	register(Command{
		Name:     "ingest-seattle-operating-budget",
		Synopsis: "Pull Seattle operating budget rows into Postgres",
		Run:      runIngestSeattleOperatingBudget,
	})
}
func runIngestSeattleOperatingBudget(args []string) int {
	fs := flag.NewFlagSet("ingest-seattle-operating-budget", flag.ContinueOnError)
	var (
		limit     = fs.Int("limit", 1000, "maximum rows to fetch (0 = Socrata page default)")
		dsn       = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		rateLimit = fs.Float64("rate", 10.0, "max requests/sec for data.seattle.gov")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-seattle-operating-budget: db open: %v\n", err)
		return 1
	}
	defer store.Close()
	httpClient := httpx.New(httpx.Config{
		UserAgent:    userAgent,
		Timeout:      45 * time.Second,
		MaxRetries:   2,
		RetryBackoff: 750 * time.Millisecond,
		HostRateLimit: map[string]float64{
			"data.seattle.gov": *rateLimit,
		},
	})
	client := seattle.New(httpClient, os.Getenv("SOCRATA_APP_TOKEN"))
	query := socrata.Query{Order: ":id"}
	if *limit > 0 {
		query.Limit = *limit
	}
	rows, _, err := client.FetchOperatingBudgetWithSource(ctx, query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-seattle-operating-budget: fetch: %v\n", err)
		return 1
	}

	var upserted int
	for _, row := range rows {
		budget := seattle.NormalizeOperatingBudget(row)
		if budget.SourceRowID == "" {
			budget.SourceRowID = seattle.StableRowID(row)
		}
		if err := store.UpsertSeattleOperatingBudget(ctx, db.UpsertSeattleOperatingBudgetParams{
			SourceDatasetID: budget.SourceDatasetID,
			SourceRowID:     budget.SourceRowID,
			FiscalYear:      budget.FiscalYear,
			Service:         budget.Service,
			Department:      budget.Department,
			Program:         budget.Program,
			Fund:            budget.Fund,
			FundType:        budget.FundType,
			ExpenseType:     budget.ExpenseType,
			Description:     budget.Description,
			ApprovedAmount:  normalizeMoney(budget.ApprovedAmount),
			RawFields:       row,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "ingest-seattle-operating-budget: upsert row %s: %v\n", budget.SourceRowID, err)
			return 1
		}
		upserted++
	}
	fmt.Fprintf(os.Stderr, "==> ingested %d %s Seattle operating budget rows\n", upserted, seattle.DatasetOperatingBudget)
	return 0
}

// runIngestFiscalVendorPayments implements `wa-dd ingest-fiscal-vendor-payments`:
// pulls the current fiscal.wa.gov Open Checkbook workbook into a normalized
// vendor-payment table. This is a first budget/spending slice; proposal-level
