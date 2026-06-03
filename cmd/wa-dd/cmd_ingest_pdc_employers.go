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
		Name:     "ingest-pdc-employers",
		Synopsis: "Pull PDC lobbyist-employer registrations (xhn7-64im) into Postgres",
		Run:      runIngestPDCEmployers,
	})
}
func runIngestPDCEmployers(args []string) int {
	fs := flag.NewFlagSet("ingest-pdc-employers", flag.ContinueOnError)
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
		fmt.Fprintf(os.Stderr, "ingest-pdc-employers: db open: %v\n", err)
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

	// Iterate xhn7-64im (Lobbyist Employment Registrations). Each row binds a
	// lobbyist registration to an employer; we collapse to distinct employers
	// (employer_id) keeping the most recent employment_year.
	type emp struct {
		id     string
		row    pdc.LobbyistEmployment
		raw    pdc.Row
		seenAt int
	}
	seen := map[string]*emp{}
	q := pdc.Query{Order: ":id", Limit: *pageSize}
	offset := 0
	page := 0
	start := time.Now()
	for {
		if err := ctx.Err(); err != nil {
			break
		}
		q.Offset = offset
		rows, _, err := client.FetchPageWithSource(ctx, pdc.DatasetLobbyistEmployment, q)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ingest-pdc-employers: fetch page %d: %v\n", page, err)
			return 1
		}
		for _, r := range rows {
			emRow := pdc.NormalizeLobbyistEmployment(r)
			if emRow.EmployerID == "" || emRow.EmployerName == "" {
				continue
			}
			normalized := pdc.NormalizeOrgName(emRow.EmployerName)
			if normalized == "" {
				continue
			}
			raw := map[string]any(r)
			if err := store.UpsertPDCEmployer(ctx, db.UpsertPDCEmployerParams{
				EmployerID:         emRow.EmployerID,
				Name:               emRow.EmployerName,
				NormalizedName:     normalized,
				LastEmploymentYear: emRow.EmploymentYear,
				LastReportNumber:   emRow.ReportNumber,
				LastEmploymentURL:  emRow.EmploymentURL,
				Raw:                raw,
			}); err != nil {
				fmt.Fprintf(os.Stderr, "ingest-pdc-employers: upsert %s: %v\n", emRow.EmployerID, err)
				return 1
			}
			if err := store.UpsertPDCLobbyistAffiliation(ctx, db.UpsertPDCLobbyistAffiliationParams{
				ReportNumber:     emRow.ReportNumber,
				LobbyistID:       emRow.LobbyistID,
				LobbyistName:     emRow.LobbyistName,
				EmployerID:       emRow.EmployerID,
				EmployerName:     emRow.EmployerName,
				EmploymentYear:   emRow.EmploymentYear,
				EmploymentURL:    emRow.EmploymentURL,
				EmploymentPeriod: emRow.EmploymentPeriod,
				Raw:              raw,
			}); err != nil {
				fmt.Fprintf(os.Stderr, "ingest-pdc-employers: upsert lobbyist affiliation %s/%s: %v\n", emRow.LobbyistID, emRow.EmployerID, err)
				return 1
			}
			seen[emRow.EmployerID] = &emp{id: emRow.EmployerID, row: emRow, raw: r}
		}
		page++
		fmt.Fprintf(os.Stderr, "  page=%d fetched=%d distinct-employers=%d elapsed=%s\n", page, len(rows), len(seen), time.Since(start).Round(time.Second))
		if len(rows) < q.Limit {
			break
		}
		offset += q.Limit
	}
	fmt.Fprintf(os.Stderr, "==> ingest-pdc-employers: %d distinct employers ingested in %s\n", len(seen), time.Since(start).Round(time.Second))
	return 0
}
