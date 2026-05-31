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
		Name:     "ingest-it-contracts",
		Synopsis: "Pull DataWA IT contracts report rows into Postgres",
		Run:      runIngestITContracts,
	})
}
func runIngestITContracts(args []string) int {
	fs := flag.NewFlagSet("ingest-it-contracts", flag.ContinueOnError)
	var (
		fiscalYear = fs.Int("fiscal-year", 2025, "DataWA IT Contracts Report fiscal year to ingest")
		limit      = fs.Int("limit", 1000, "maximum rows to fetch (0 = Socrata page default)")
		dsn        = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		rawDir     = fs.String("raw-dir", "data/raw", "filesystem root for raw API responses")
		rateLimit  = fs.Float64("rate", 5.0, "max requests/sec for data.wa.gov")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-it-contracts: db open: %v\n", err)
		return 1
	}
	defer store.Close()
	objs, err := objectstore.NewConfigured(ctx, *rawDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-it-contracts: objectstore: %v\n", err)
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
	datasetID := datawa.ITContractFiscalYears[*fiscalYear]
	if datasetID == "" {
		fmt.Fprintf(os.Stderr, "ingest-it-contracts: unsupported fiscal year %d\n", *fiscalYear)
		return 2
	}
	query := socrata.Query{Order: ":id"}
	if *limit > 0 {
		query.Limit = *limit
	}
	rows, fetch, err := client.FetchITContractsWithSource(ctx, *fiscalYear, query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-it-contracts: fetch: %v\n", err)
		return 1
	}
	if fetch.SourceRecordID == 0 {
		fmt.Fprintln(os.Stderr, "ingest-it-contracts: source record was not captured")
		return 1
	}

	var upserted int
	for _, row := range rows {
		contract := datawa.NormalizeITContract(datasetID, *fiscalYear, row)
		if contract.SourceRowID == "" {
			contract.SourceRowID = datawa.StableRowID(row)
		}
		if err := store.UpsertDataWAITContract(ctx, db.UpsertDataWAITContractParams{
			SourceDatasetID:           contract.SourceDatasetID,
			SourceRowID:               contract.SourceRowID,
			ReportFiscalYear:          contract.ReportFiscalYear,
			AgencyNumberAgencyName:    contract.AgencyNumberAgencyName,
			AgencyNumber:              contract.AgencyNumber,
			AgencyName:                contract.AgencyName,
			ContractNumber:            contract.ContractNumber,
			ContractorName:            contract.ContractorName,
			NormalizedContractorName:  entitymatch.NormalizedName(contract.ContractorName),
			ContractorDBA:             contract.ContractorDBA,
			NormalizedContractorDBA:   entitymatch.NormalizedName(contract.ContractorDBA),
			CooperativePurchase:       contract.CooperativePurchase,
			CooperativeName:           contract.CooperativeName,
			StatewideContractPurchase: contract.StatewideContractPurchase,
			ContractStartDate:         contract.ContractStartDate,
			ContractEndDate:           contract.ContractEndDate,
			FiscalYearStart:           contract.FiscalYearStart,
			FiscalYearEnd:             contract.FiscalYearEnd,
			ITTowerApplication:        normalizeMoney(contract.ITTowerApplication),
			ITTowerCompute:            normalizeMoney(contract.ITTowerCompute),
			ITTowerDataCenter:         normalizeMoney(contract.ITTowerDataCenter),
			ITTowerDelivery:           normalizeMoney(contract.ITTowerDelivery),
			ITTowerEndUser:            normalizeMoney(contract.ITTowerEndUser),
			ITTowerITManagement:       normalizeMoney(contract.ITTowerITManagement),
			ITTowerNetwork:            normalizeMoney(contract.ITTowerNetwork),
			ITTowerOutput:             normalizeMoney(contract.ITTowerOutput),
			ITTowerPlatform:           normalizeMoney(contract.ITTowerPlatform),
			ITTowerSecurity:           normalizeMoney(contract.ITTowerSecurity),
			ITTowerStorage:            normalizeMoney(contract.ITTowerStorage),
			OtherNonIT:                normalizeMoney(contract.OtherNonIT),
			TotalPercentage:           normalizeMoney(contract.TotalPercentage),
			ContractAmountFY20:        normalizeMoney(contract.ContractAmountFY20),
			ContractAmountFY21:        normalizeMoney(contract.ContractAmountFY21),
			ContractAmountFY22:        normalizeMoney(contract.ContractAmountFY22),
			ContractAmountFY23:        normalizeMoney(contract.ContractAmountFY23),
			ContractAmountFY24:        normalizeMoney(contract.ContractAmountFY24),
			ContractAmountFY25:        normalizeMoney(contract.ContractAmountFY25),
			ContractAmountFY26:        normalizeMoney(contract.ContractAmountFY26),
			ContractAmountFY27:        normalizeMoney(contract.ContractAmountFY27),
			ContractAmountFY28:        normalizeMoney(contract.ContractAmountFY28),
			ContractAmountFY29:        normalizeMoney(contract.ContractAmountFY29),
			ContractAmountFY30:        normalizeMoney(contract.ContractAmountFY30),
			TotalContractAmount:       normalizeMoney(contract.TotalContractAmount),
			ContractAmountExplanation: contract.ContractAmountExplanation,
			Warnings:                  contract.NormalizationWarning,
			RawFields:                 row,
			SourceRecordID:            fetch.SourceRecordID,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "ingest-it-contracts: upsert row %s: %v\n", contract.SourceRowID, err)
			return 1
		}
		upserted++
	}
	fmt.Fprintf(os.Stderr, "==> ingested %d %s IT contract rows for FY%d\n", upserted, datasetID, *fiscalYear)
	return 0
}

// runIngestWEBSVendors implements `wa-dd ingest-webs-vendors`: pulls the DataWA
// WEBS vendor dataset into a procurement-entity table that can later power
// contractor/org matching. Matching remains explicit and reviewable; this
