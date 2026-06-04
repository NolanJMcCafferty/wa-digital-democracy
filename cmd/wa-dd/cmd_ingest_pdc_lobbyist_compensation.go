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
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/pdc"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func init() {
	register(Command{
		Name:     "ingest-pdc-lobbyist-compensation",
		Synopsis: "Pull PDC lobbyist compensation data (9nnw-c693) into Postgres",
		Run:      runIngestPDCLobbyistCompensation,
	})
}

func runIngestPDCLobbyistCompensation(args []string) int {
	fs := flag.NewFlagSet("ingest-pdc-lobbyist-compensation", flag.ContinueOnError)
	var (
		dsn       = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		rateLimit = fs.Float64("rate", 5.0, "max requests/sec for data.wa.gov")
		pageSize  = fs.Int("page-size", 2000, "Socrata page size")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-pdc-lobbyist-compensation: db open: %v\n", err)
		return 1
	}
	defer store.Close()

	httpClient := httpx.New(httpx.Config{
		UserAgent:    userAgent,
		Timeout:      120 * time.Second,
		MaxRetries:   2,
		RetryBackoff: 750 * time.Millisecond,
		HostRateLimit: map[string]float64{
			"data.wa.gov": *rateLimit,
		},
	})
	client := pdc.New(httpClient, os.Getenv("SOCRATA_APP_TOKEN"))

	// Iterate 9nnw-c693 (Lobbyist Compensation and Expenses by Source). Each row
	// represents compensation paid by an employer (client) to a filer (lobbying firm).
	q := pdc.Query{Order: ":id", Limit: *pageSize}
	offset := 0
	page := 0
	upserted := 0
	skipped := 0
	start := time.Now()

	for {
		if err := ctx.Err(); err != nil {
			break
		}
		q.Offset = offset
		fmt.Fprintf(os.Stderr, "  fetching page=%d offset=%d limit=%d elapsed=%s\n",
			page+1, offset, q.Limit, time.Since(start).Round(time.Second))

		rows, _, err := client.FetchPageWithSource(ctx, pdc.DatasetLobbyistCompensation, q)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ingest-pdc-lobbyist-compensation: fetch page %d: %v\n", page, err)
			return 1
		}

		for i, r := range rows {
			compRow := pdc.NormalizeLobbyistCompensation(r)
			if compRow.FilerID == "" || compRow.EmployerID == "" || compRow.FilingPeriod == "" {
				skipped++
				continue
			}
			if compRow.FilerName == "" || compRow.EmployerName == "" {
				skipped++
				continue
			}

			raw := map[string]any(r)
			if err := store.UpsertPDCLobbyistCompensation(ctx, db.UpsertPDCLobbyistCompensationParams{
				FilerID:         compRow.FilerID,
				FilerName:       compRow.FilerName,
				FundingSourceID: compRow.FundingSourceID,
				FundingSource:   compRow.FundingSource,
				FilingPeriod:    compRow.FilingPeriod,
				EmployerID:      compRow.EmployerID,
				EmployerName:    compRow.EmployerName,
				Compensation:    compRow.Compensation,
				TotalExpenses:   compRow.TotalExpenses,
				NetTotal:        compRow.NetTotal,
				URL:             compRow.URL,
				Raw:             raw,
			}); err != nil {
				fmt.Fprintf(os.Stderr, "ingest-pdc-lobbyist-compensation: upsert %s/%s/%s: %v\n",
					compRow.FilerID, compRow.EmployerID, compRow.FilingPeriod, err)
				return 1
			}

			upserted++
			if upserted%500 == 0 {
				fmt.Fprintf(os.Stderr, "  upserted=%d skipped=%d page=%d row=%d/%d elapsed=%s\n",
					upserted, skipped, page+1, i+1, len(rows), time.Since(start).Round(time.Second))
			}
		}

		page++
		fmt.Fprintf(os.Stderr, "  page=%d fetched=%d upserted=%d skipped=%d elapsed=%s\n",
			page, len(rows), upserted, skipped, time.Since(start).Round(time.Second))

		if len(rows) < q.Limit {
			break
		}
		offset += q.Limit
	}

	fmt.Fprintf(os.Stderr, "==> ingest-pdc-lobbyist-compensation: upserted %d rows, skipped %d rows in %s\n",
		upserted, skipped, time.Since(start).Round(time.Second))
	return 0
}
