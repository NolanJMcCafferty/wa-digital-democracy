package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/entitymatch"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/datawa"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/socrata"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/objectstore"
)

func init() {
	register(Command{
		Name:     "ingest-contracts",
		Synopsis: "Pull DataWA agency contract rows into Postgres",
		Run:      runIngestContracts,
	})
}
func runIngestContracts(args []string) int {
	fs := flag.NewFlagSet("ingest-contracts", flag.ContinueOnError)
	var (
		fiscalYear = fs.Int("fiscal-year", 2025, "DataWA agency contracts fiscal year to ingest")
		limit      = fs.Int("limit", 1000, "maximum rows to fetch (0 = Socrata page default)")
		dsn        = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		rawDir     = fs.String("raw-dir", "data/raw", "filesystem root for raw API responses")
		rateLimit  = fs.Float64("rate", 10.0, "max requests/sec for data.wa.gov")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-contracts: db open: %v\n", err)
		return 1
	}
	defer store.Close()

	objs, err := objectstore.NewConfigured(ctx, *rawDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-contracts: objectstore: %v\n", err)
		return 1
	}
	httpClient := httpx.New(httpx.Config{
		UserAgent:    userAgent,
		Sink:         db.RawSink{Store: store, Objects: objs, TransformVersion: "v0"},
		Timeout:      45 * time.Second,
		MaxRetries:   2,
		RetryBackoff: 750 * time.Millisecond,
		HostRateLimit: map[string]float64{
			"data.wa.gov": *rateLimit,
		},
	})
	client := datawa.New(httpClient, os.Getenv("SOCRATA_APP_TOKEN"))
	datasetID := datawa.AgencyContractFiscalYears[*fiscalYear]
	if datasetID == "" {
		fmt.Fprintf(os.Stderr, "ingest-contracts: unsupported fiscal year %d\n", *fiscalYear)
		return 2
	}

	query := socrata.Query{Order: ":id"}
	if *limit > 0 {
		query.Limit = *limit
	}
	rows, fetch, err := client.FetchAgencyContractsWithSource(ctx, *fiscalYear, query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-contracts: fetch: %v\n", err)
		return 1
	}
	if fetch.SourceRecordID == 0 {
		fmt.Fprintln(os.Stderr, "ingest-contracts: source record was not captured")
		return 1
	}

	var upserted int
	for _, row := range rows {
		contract := datawa.NormalizeContract(datasetID, *fiscalYear, row)
		if contract.SourceRowID == "" {
			contract.SourceRowID = datawa.StableRowID(row)
		}
		if err := store.UpsertDataWAContract(ctx, db.UpsertDataWAContractParams{
			SourceDatasetID:          contract.SourceDatasetID,
			SourceRowID:              contract.SourceRowID,
			FiscalYear:               contract.FiscalYear,
			AgencyName:               contract.AgencyName,
			AgencyNumber:             contract.AgencyNumber,
			ContractNumber:           contract.ContractNumber,
			AmendmentNumber:          contract.AmendmentNumber,
			ContractorName:           contract.ContractorName,
			NormalizedContractorName: entitymatch.NormalizedName(contract.ContractorName),
			StatewideVendorNumber:    contract.StatewideVendorNum,
			Description:              contract.Description,
			StartDate:                contract.StartDate,
			EndDate:                  contract.EndDate,
			PeriodStart:              contract.PeriodStart,
			PeriodEnd:                contract.PeriodEnd,
			FederalAmount:            normalizeMoney(contract.FederalAmount),
			StateAmount:              normalizeMoney(contract.StateAmount),
			OtherAmount:              normalizeMoney(contract.OtherAmount),
			TotalAmount:              normalizeMoney(contract.TotalAmount),
			ProcurementType:          contract.ProcurementType,
			MinorityWomanOwned:       contract.MinorityWomanOwned,
			SmallBusiness:            contract.SmallBusiness,
			VeteranOwned:             contract.VeteranOwned,
			Warnings:                 contract.NormalizationWarning,
			RawFields:                row,
			SourceRecordID:           fetch.SourceRecordID,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "ingest-contracts: upsert row %s: %v\n", contract.SourceRowID, err)
			return 1
		}
		upserted++
	}
	fmt.Fprintf(os.Stderr, "==> ingested %d %s contract rows for FY%d\n", upserted, datasetID, *fiscalYear)
	return 0
}
