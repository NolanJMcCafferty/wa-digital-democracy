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
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/irsbmf"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/pdc"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func init() {
	register(Command{
		Name:     "ingest-irs-bmf-wa",
		Synopsis: "Pull the IRS BMF Washington 501(c) extract into Postgres",
		Run:      runIngestIRSBMFWA,
	})
}
func runIngestIRSBMFWA(args []string) int {
	fs := flag.NewFlagSet("ingest-irs-bmf-wa", flag.ContinueOnError)
	var (
		dsn       = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		url       = fs.String("url", irsbmf.DefaultStateURL, "IRS BMF state-extract CSV URL")
		rateLimit = fs.Float64("rate", 4.0, "max requests/sec for irs.gov")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-irs-bmf-wa: db open: %v\n", err)
		return 1
	}
	defer store.Close()
	httpClient := httpx.New(httpx.Config{
		UserAgent:    userAgent,
		Timeout:      120 * time.Second,
		MaxRetries:   2,
		RetryBackoff: 750 * time.Millisecond,
		HostRateLimit: map[string]float64{
			"www.irs.gov": *rateLimit,
		},
	})
	client := irsbmf.New(httpClient)
	client.BaseURL = *url

	body, _, err := client.Fetch(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-irs-bmf-wa: fetch: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "==> fetched %d bytes from %s\n", len(body), *url)

	var upserted int
	start := time.Now()
	err = irsbmf.ParseAll(body, func(row irsbmf.Row) bool {
		if err := ctx.Err(); err != nil {
			return false
		}
		normalized := pdc.NormalizeOrgName(row.Name)
		if normalized == "" {
			return true
		}
		err := store.UpsertIRSBMFOrganization(ctx, db.UpsertIRSBMFParams{
			EIN:               row.EIN,
			Name:              row.Name,
			NormalizedName:    normalized,
			SortName:          row.SortName,
			Street:            row.Street,
			City:              row.City,
			State:             row.State,
			Zip:               row.Zip,
			SubsectionCode:    row.Subsection,
			Classification:    row.Classification,
			DeductibilityCode: row.Deductibility,
			ActivityCodes:     row.Activity,
			FoundationCode:    row.Foundation,
			OrganizationCode:  row.Organization,
			StatusCode:        row.Status,
			RulingDate:        row.Ruling,
			NTEECode:          row.NTEECode,
			IncomeAmount:      row.IncomeAmount,
			RevenueAmount:     row.RevenueAmount,
			AssetAmount:       row.AssetAmount,
			Raw:               row.Raw,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "ingest-irs-bmf-wa: upsert %s: %v\n", row.EIN, err)
			return false
		}
		if _, err := store.EnsureOrganizationFromIRSBMF(ctx, row.EIN, row.Name, normalized); err != nil {
			fmt.Fprintf(os.Stderr, "ingest-irs-bmf-wa: ensure org %s: %v\n", row.EIN, err)
			return false
		}
		upserted++
		if upserted%2000 == 0 {
			fmt.Fprintf(os.Stderr, "  upserted=%d elapsed=%s\n", upserted, time.Since(start).Round(time.Second))
		}
		return true
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-irs-bmf-wa: parse: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "==> ingest-irs-bmf-wa: upserted %d organizations in %s\n", upserted, time.Since(start).Round(time.Second))
	return 0
}
