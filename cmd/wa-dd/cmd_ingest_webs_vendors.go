package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/datawa"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/socrata"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/objectstore"
)

func init() {
	register(Command{
		Name:     "ingest-webs-vendors",
		Synopsis: "Pull DataWA WEBS vendor rows into Postgres",
		Run:      runIngestWEBSVendors,
	})
}
func runIngestWEBSVendors(args []string) int {
	fs := flag.NewFlagSet("ingest-webs-vendors", flag.ContinueOnError)
	var (
		limit     = fs.Int("limit", 1000, "maximum rows to fetch (0 = Socrata page default)")
		dsn       = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		rawDir    = fs.String("raw-dir", "data/raw", "filesystem root for raw API responses")
		rateLimit = fs.Float64("rate", 5.0, "max requests/sec for data.wa.gov")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-webs-vendors: db open: %v\n", err)
		return 1
	}
	defer store.Close()
	objs, err := objectstore.NewConfigured(ctx, *rawDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-webs-vendors: objectstore: %v\n", err)
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
	rows, fetch, err := client.FetchWEBSVendorsWithSource(ctx, query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-webs-vendors: fetch: %v\n", err)
		return 1
	}
	if fetch.SourceRecordID == 0 {
		fmt.Fprintln(os.Stderr, "ingest-webs-vendors: source record was not captured")
		return 1
	}

	var upserted int
	for _, row := range rows {
		vendor := datawa.NormalizeWEBSVendor(row)
		if vendor.SourceRowID == "" {
			vendor.SourceRowID = datawa.StableRowID(row)
		}
		if err := store.UpsertDataWAWEBSVendor(ctx, db.UpsertDataWAWEBSVendorParams{
			SourceDatasetID:       vendor.SourceDatasetID,
			SourceRowID:           vendor.SourceRowID,
			CompanyName:           vendor.CompanyName,
			NormalizedCompanyName: vendor.NormalizedCompanyName,
			DBAName:               vendor.DBAName,
			PhoneNumber:           vendor.PhoneNumber,
			ContactEmail:          vendor.ContactEmail,
			City:                  vendor.City,
			State:                 vendor.State,
			WebAddress:            vendor.WebAddress,
			CommodityCode:         vendor.CommodityCode,
			DescriptionOfWork:     vendor.DescriptionOfWork,
			SmallBusiness:         vendor.SmallBusiness,
			VeteranOwned:          vendor.VeteranOwned,
			OtherCert:             vendor.OtherCert,
			OtherCert2:            vendor.OtherCert2,
			Warnings:              vendor.NormalizationWarning,
			RawFields:             row,
			SourceRecordID:        fetch.SourceRecordID,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "ingest-webs-vendors: upsert row %s: %v\n", vendor.SourceRowID, err)
			return 1
		}
		upserted++
	}
	fmt.Fprintf(os.Stderr, "==> ingested %d %s WEBS vendor rows\n", upserted, datawa.DatasetWEBSVendors)
	return 0
}

// runGenerateVendorEntityMatches generates reviewable candidate links between
// procurement/vendor source rows and existing organizations. It stores evidence
