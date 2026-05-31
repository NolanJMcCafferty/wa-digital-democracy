package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/fiscalwa"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/objectstore"
)

func init() {
	register(Command{
		Name:     "ingest-fiscal-vendor-payments",
		Synopsis: "Pull fiscal.wa.gov Open Checkbook vendor payments into Postgres",
		Run:      runIngestFiscalVendorPayments,
	})
}
func runIngestFiscalVendorPayments(args []string) int {
	fs := flag.NewFlagSet("ingest-fiscal-vendor-payments", flag.ContinueOnError)
	var (
		limit     = fs.Int("limit", 1000, "maximum rows to upsert after parsing (0 = all rows)")
		dsn       = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		rawDir    = fs.String("raw-dir", "data/raw", "filesystem root for raw API responses")
		rateLimit = fs.Float64("rate", 10.0, "max requests/sec for fiscal.wa.gov")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-fiscal-vendor-payments: db open: %v\n", err)
		return 1
	}
	defer store.Close()
	objs, err := objectstore.NewConfigured(ctx, *rawDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-fiscal-vendor-payments: objectstore: %v\n", err)
		return 1
	}
	httpClient := httpx.New(httpx.Config{
		UserAgent:    userAgent,
		Sink:         db.RawSink{Store: store, Objects: objs, TransformVersion: "v0"},
		Timeout:      2 * time.Minute,
		MaxRetries:   2,
		RetryBackoff: 750 * time.Millisecond,
		HostRateLimit: map[string]float64{
			"fiscal.wa.gov": *rateLimit,
		},
	})
	client := fiscalwa.New(httpClient)
	rows, fetch, err := client.FetchVendorPaymentsWithSource(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-fiscal-vendor-payments: fetch/parse: %v\n", err)
		return 1
	}
	if fetch.SourceRecordID == 0 {
		fmt.Fprintln(os.Stderr, "ingest-fiscal-vendor-payments: source record was not captured")
		return 1
	}

	var upserted int
	for _, row := range rows {
		if *limit > 0 && upserted >= *limit {
			break
		}
		if err := store.UpsertFiscalWAVendorPayment(ctx, db.UpsertFiscalWAVendorPaymentParams{
			SourceDatasetID: row.SourceDatasetID,
			SourceRowID:     row.SourceRowID,
			Biennium:        row.Biennium,
			FiscalYear:      row.FiscalYear,
			FiscalMonth:     row.FiscalMonth,
			AgencyNumber:    row.AgencyNumber,
			AgencyName:      row.AgencyName,
			ObjectCode:      row.ObjectCode,
			ObjectCategory:  row.ObjectCategory,
			SubobjectCode:   row.SubobjectCode,
			SubobjectName:   row.SubobjectName,
			VendorName:      row.VendorName,
			Amount:          normalizeMoney(row.Amount),
			RawFields: map[string]any{
				"biennium": row.Biennium, "fiscal_year": row.FiscalYear, "fiscal_month": row.FiscalMonth,
				"agency_number": row.AgencyNumber, "agency_name": row.AgencyName,
				"object_code": row.ObjectCode, "object_category": row.ObjectCategory,
				"subobject_code": row.SubobjectCode, "subobject_name": row.SubobjectName,
				"vendor_name": row.VendorName, "amount": row.Amount,
			},
			SourceRecordID: fetch.SourceRecordID,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "ingest-fiscal-vendor-payments: upsert row %s: %v\n", row.SourceRowID, err)
			return 1
		}
		upserted++
	}
	fmt.Fprintf(os.Stderr, "==> ingested %d %s vendor payment rows\n", upserted, fiscalwa.VendorPaymentsDatasetID)
	return 0
}

// runIngestLegislators implements `wa-dd ingest-legislators`: pulls the
// full House + Senate roster for a biennium from LWS SponsorService and
// upserts each member into the `legislator` table. This populates
// district/party/email/phone fields and surfaces non-sponsoring members
