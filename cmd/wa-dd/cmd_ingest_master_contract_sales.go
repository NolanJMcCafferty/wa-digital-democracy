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
		Name:     "ingest-master-contract-sales",
		Synopsis: "Pull DataWA statewide/master-contract sales rows into Postgres",
		Run:      runIngestMasterContractSales,
	})
}
func runIngestMasterContractSales(args []string) int {
	fs := flag.NewFlagSet("ingest-master-contract-sales", flag.ContinueOnError)
	var (
		limit     = fs.Int("limit", 1000, "maximum rows to fetch (0 = Socrata page default)")
		dsn       = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		rawDir    = fs.String("raw-dir", "data/raw", "filesystem root for raw API responses")
		rateLimit = fs.Float64("rate", 10.0, "max requests/sec for data.wa.gov")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-master-contract-sales: db open: %v\n", err)
		return 1
	}
	defer store.Close()
	objs, err := objectstore.NewConfigured(ctx, *rawDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-master-contract-sales: objectstore: %v\n", err)
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
	query := socrata.Query{Order: ":id"}
	if *limit > 0 {
		query.Limit = *limit
	}
	rows, fetch, err := client.FetchMasterContractSalesWithSource(ctx, query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-master-contract-sales: fetch: %v\n", err)
		return 1
	}
	if fetch.SourceRecordID == 0 {
		fmt.Fprintln(os.Stderr, "ingest-master-contract-sales: source record was not captured")
		return 1
	}

	var upserted int
	for _, row := range rows {
		sale := datawa.NormalizeMasterContractSale(row)
		if sale.SourceRowID == "" {
			sale.SourceRowID = datawa.StableRowID(row)
		}
		if err := store.UpsertDataWAMasterContractSale(ctx, db.UpsertDataWAMasterContractSaleParams{
			SourceDatasetID:        sale.SourceDatasetID,
			SourceRowID:            sale.SourceRowID,
			CustomerType:           sale.CustomerType,
			CustomerName:           sale.CustomerName,
			NormalizedCustomerName: entitymatch.NormalizedName(sale.CustomerName),
			ContractNumber:         sale.ContractNumber,
			ContractTitle:          sale.ContractTitle,
			VendorName:             sale.VendorName,
			NormalizedVendorName:   entitymatch.NormalizedName(sale.VendorName),
			ReportYear:             sale.ReportYear,
			Q1SalesReported:        normalizeMoney(sale.Q1SalesReported),
			Q2SalesReported:        normalizeMoney(sale.Q2SalesReported),
			Q3SalesReported:        normalizeMoney(sale.Q3SalesReported),
			Q4SalesReported:        normalizeMoney(sale.Q4SalesReported),
			TotalSalesReported:     normalizeMoney(sale.TotalSalesReported),
			OMWBE:                  sale.OMWBE,
			VeteranOwned:           sale.VeteranOwned,
			SmallBusiness:          sale.SmallBusiness,
			DiverseOptions:         sale.DiverseOptions,
			Warnings:               sale.NormalizationWarning,
			RawFields:              row,
			SourceRecordID:         fetch.SourceRecordID,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "ingest-master-contract-sales: upsert row %s: %v\n", sale.SourceRowID, err)
			return 1
		}
		upserted++
	}
	fmt.Fprintf(os.Stderr, "==> ingested %d %s master-contract sales rows\n", upserted, datawa.DatasetMasterContractSales)
	return 0
}

// runIngestITContracts implements `wa-dd ingest-it-contracts`: pulls one DataWA
// IT Contracts Report fiscal-year dataset into datawa_it_contract. It is scoped
// to the annual IT contracts report family; monthly IT spend remains a separate
