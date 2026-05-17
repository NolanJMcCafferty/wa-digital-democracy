// Command wa-dd is the operator-facing CLI for ingestion, matching, and
// rendering. The first-page subcommand surface (per Blueprint §"Suggested CLI
// commands"):
//
//	wa-dd find-candidates  --issue housing
//	wa-dd inspect-candidate --bill HB1234 --hearing <id>
//	wa-dd ingest-bill      --biennium 2025-26 --bill HB1234
//	wa-dd ingest-csi       --chamber House --committee-id <id> --meeting-family-id <id>
//	wa-dd ingest-tvw       --event-id <event_id>
//	wa-dd match-hearing    --bill HB1234 --event-id <event_id>
//	wa-dd build-bundle     --config config/selected_demo.yml
//
// Phase 3 ships find-candidates; the rest are stubs until Phase 4.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/candidate"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/config"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/jobs"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/render/firstpage"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/csi"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/lws"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/pdc"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/tvw"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/objectstore"
)

const usage = `wa-dd — Washington Digital Democracy operator CLI

USAGE:
  wa-dd <subcommand> [flags]

SUBCOMMANDS:
  find-candidates    Score candidate bill/hearing pairs for a given issue
  inspect-candidate  Print joined source state for a single candidate
  ingest-bill        Pull LWS bill bundle into Postgres
  ingest-csi         Pull CSI testifier list for an agenda item
  ingest-tvw         Pull TVW/Invintus event detail + VTT
  match-hearing      Compute meeting<->TVW match for a candidate
  build-bundle       Assemble the JSON bundle for a selected demo
  version            Print version info

Run 'wa-dd <subcommand> -h' for subcommand flags.
`

const userAgent = "wa-dd/0.0.1 (https://github.com/nolan-mccafferty/wa-digital-democracy; nolan-mccafferty)"

// version is overridden via -ldflags="-X main.version=…" at release time.
var version = "0.0.0-dev"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	cmd, args := os.Args[1], os.Args[2:]
	switch cmd {
	case "version":
		fmt.Println(version)
	case "find-candidates":
		os.Exit(runFindCandidates(args))
	case "inspect-candidate":
		runStub(cmd, args, "phase 3")
	case "build-bundle":
		os.Exit(runBuildBundle(args))
	case "ingest-bill", "ingest-csi", "ingest-tvw", "match-hearing":
		// All four are implemented as steps inside `build-bundle`. Direct
		// per-step invocation isn't shipped in v1.
		fmt.Fprintf(os.Stderr, "wa-dd %s: run via 'wa-dd build-bundle' (per-step CLI not yet exposed)\n", cmd)
		os.Exit(64)
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "wa-dd: unknown subcommand %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
}

func runStub(cmd string, args []string, phase string) {
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "wa-dd %s: not yet implemented (planned for %s)\n", cmd, phase)
	}
	_ = fs.Parse(args)
	fmt.Fprintf(os.Stderr, "wa-dd %s: not yet implemented (planned for %s)\n", cmd, phase)
	os.Exit(64)
}

// runFindCandidates implements `wa-dd find-candidates`.
//
// It fetches recent CSI agenda items for the issue's target committees,
// scores each by testifier coverage and organization diversity, and writes
// the top N to a JSON file the operator picks one from.
//
// Read-only by design: no DB writes (we use httpx.NopSink). Phase 4 will
// re-fetch through the proper sink when the operator commits to a candidate.
func runFindCandidates(args []string) int {
	fs := flag.NewFlagSet("find-candidates", flag.ContinueOnError)
	var (
		issue       = fs.String("issue", "housing", "issue keyword (currently: housing)")
		maxItems    = fs.Int("max", 50, "max agenda items to inspect")
		maxMeetings = fs.Int("max-meetings", 8, "max recent meetings per committee")
		out         = fs.String("out", "data/processed/candidates.json", "output JSON path")
		topN        = fs.Int("top", 10, "show top-N candidates in stderr summary")
		rateLimit   = fs.Float64("rate", 2.0, "max requests/sec to app.leg.wa.gov")
		quiet       = fs.Bool("quiet", false, "suppress per-step progress logs")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	client := httpx.New(httpx.Config{
		UserAgent:    userAgent,
		Sink:         httpx.NopSink{}, // read-only; raw bytes not persisted
		Timeout:      30 * time.Second,
		MaxRetries:   2,
		RetryBackoff: 500 * time.Millisecond,
		HostRateLimit: map[string]float64{
			"app.leg.wa.gov": *rateLimit,
		},
	})
	finder := candidate.NewFinder(csi.New(client))

	progress := func(s string) {
		if !*quiet {
			fmt.Fprintln(os.Stderr, s)
		}
	}

	start := time.Now()
	cands, err := finder.Find(ctx, candidate.FindOptions{
		Issue:              *issue,
		MaxAgendaItems:     *maxItems,
		MaxMeetingsPerComm: *maxMeetings,
		OnProgress:         progress,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "find-candidates: %v\n", err)
		return 1
	}

	if err := writeJSON(*out, map[string]any{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"issue":        *issue,
		"count":        len(cands),
		"candidates":   cands,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "find-candidates: write %s: %v\n", *out, err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "\nfind-candidates: scanned %d agenda items in %s\n",
		len(cands), time.Since(start).Round(time.Millisecond))
	fmt.Fprintf(os.Stderr, "wrote %s\n\n", *out)
	printTop(cands, *topN)
	return 0
}

func printTop(cs []candidate.Candidate, n int) {
	fmt.Fprintln(os.Stderr, "Top candidates (highest score, most recent first):")
	fmt.Fprintln(os.Stderr, "  score  bill          chamber  meeting             testifiers (Pro/Con/Other)  orgs  agenda_item")
	for i, c := range cs {
		if i >= n {
			break
		}
		bill := c.BillID
		if bill == "" {
			bill = "(no bill)"
		}
		meeting := "-"
		if !c.MeetingDateTime.IsZero() {
			meeting = c.MeetingDateTime.Format("2006-01-02 15:04")
		}
		fmt.Fprintf(os.Stderr, "  %5d  %-13s %-7s  %-19s   %2d (%d/%d/%d)              %4d  %s\n",
			c.Score, bill, c.Chamber, meeting,
			c.TestifierCount, c.ProCount, c.ConCount, c.OtherCount,
			c.UniqueOrganizations, c.AgendaItemID)
	}
	fmt.Fprintln(os.Stderr, "\nPick one and copy its IDs into config/selected_demo.yml.")
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// runBuildBundle implements `wa-dd build-bundle`.
//
// Steps:
//   1. Load selected_demo.yml.
//   2. Open Postgres + the filesystem object store and wire them into the
//      shared httpx.RawSink so every fetch records provenance.
//   3. Run the eight pipeline jobs (Blueprint Steps 1–8 minus the optional
//      Committee Schedules enrichment, which the operator covered by
//      pasting tvw.event_id into the demo config).
//   4. Assemble the JSON bundle and write to data/processed/bundles/.
func runBuildBundle(args []string) int {
	fs := flag.NewFlagSet("build-bundle", flag.ContinueOnError)
	var (
		cfgPath   = fs.String("config", "config/selected_demo.yml", "path to selected_demo.yml")
		dsn       = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		rawDir    = fs.String("raw-dir", "data/raw", "filesystem root for raw API responses")
		outDir    = fs.String("out-dir", "data/processed/bundles", "where the JSON bundle is written")
		rateLimit = fs.Float64("rate", 2.0, "max requests/sec for legislative APIs")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	demo, err := config.LoadSelectedDemo(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build-bundle: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "==> demo: %s (%s, agenda %s)\n",
		demo.BillID(), demo.Biennium, demo.Agenda.CSIAgendaItemID)

	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build-bundle: db open: %v\n", err)
		return 1
	}
	defer store.Close()

	objs, err := objectstore.NewFS(*rawDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build-bundle: objectstore: %v\n", err)
		return 1
	}
	sink := db.RawSink{Store: store, Objects: objs, TransformVersion: "v0"}

	embedderKey := os.Getenv("INVINTUS_EMBEDDER_KEY")
	if embedderKey == "" {
		fmt.Fprintln(os.Stderr, "build-bundle: INVINTUS_EMBEDDER_KEY is required for the TVW step")
		return 1
	}

	httpClient := httpx.New(httpx.Config{
		UserAgent:    userAgent,
		Sink:         sink,
		Timeout:      45 * time.Second,
		MaxRetries:   2,
		RetryBackoff: 750 * time.Millisecond,
		HostRateLimit: map[string]float64{
			"app.leg.wa.gov":         *rateLimit,
			"wslwebservices.leg.wa.gov": *rateLimit,
			"tvw.org":                *rateLimit,
			"api.v3.invintus.com":    *rateLimit,
			"data.wa.gov":            *rateLimit,
		},
	})

	pipeline := &jobs.Pipeline{
		Store: store,
		LWS:   lws.New(httpClient),
		CSI:   csi.New(httpClient),
		TVW:   tvw.New(httpClient, embedderKey),
		PDC:   pdc.New(httpClient, os.Getenv("SOCRATA_APP_TOKEN")),
		Demo:  demo,
	}
	ids := jobs.NewIDs()

	logf := func(s string) { fmt.Fprintln(os.Stderr, s) }
	if err := pipeline.Run(ctx, logf, ids); err != nil {
		fmt.Fprintf(os.Stderr, "build-bundle: %v\n", err)
		return 1
	}

	logf("==> assembling bundle")
	bundle, err := firstpage.Build(ctx, store, demo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build-bundle: %v\n", err)
		return 1
	}
	outName := fmt.Sprintf("wa_%s_%s%d.json", demo.Biennium, demo.BillPrefix, demo.BillNumber)
	outPath := filepath.Join(*outDir, outName)
	if err := writeJSON(outPath, bundle); err != nil {
		fmt.Fprintf(os.Stderr, "build-bundle: write: %v\n", err)
		return 1
	}
	logf(fmt.Sprintf("wrote %s", outPath))
	logf(fmt.Sprintf("  testifiers=%d  segments=%d  organizations=%d  sources=%d",
		len(bundle.Testifiers), len(bundle.Transcript.Segments),
		len(bundle.Organizations), len(bundle.Sources)))
	if len(bundle.KnownLimitations) > 0 {
		logf("  known limitations:")
		for _, l := range bundle.KnownLimitations {
			logf("    - " + l)
		}
	}
	return 0
}
