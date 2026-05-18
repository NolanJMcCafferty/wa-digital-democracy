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
	"strconv"
	"strings"
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
  build-bundles      Build bundles for every entry in selected_bills.yml (daily batch)
  ingest-session     Ingest LWS metadata for every bill in a biennium (no hearings)
  discover-hearings  Auto-fill CSI/TVW IDs on every LWS hearing in a biennium
  ingest-hearings    Run the full pipeline for every discovered agenda item
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
	case "build-bundles":
		os.Exit(runBuildBundles(args))
	case "ingest-session":
		os.Exit(runIngestSession(args))
	case "discover-hearings":
		os.Exit(runDiscoverHearings(args))
	case "ingest-hearings":
		os.Exit(runIngestHearings(args))
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
		rateLimit   = fs.Float64("rate", 5.0, "max requests/sec to app.leg.wa.gov")
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
		rateLimit = fs.Float64("rate", 5.0, "max requests/sec for legislative APIs")
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

	deps, cleanup, err := newBuildDeps(ctx, *dsn, *rawDir, *rateLimit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build-bundle: %v\n", err)
		return 1
	}
	defer cleanup()

	logf := func(s string) { fmt.Fprintln(os.Stderr, s) }
	logf(fmt.Sprintf("==> demo: %s (%s, agenda %s)",
		demo.BillID(), demo.Biennium, demo.Agenda.CSIAgendaItemID))

	if _, err := buildOne(ctx, deps, demo, *outDir, logf); err != nil {
		fmt.Fprintf(os.Stderr, "build-bundle: %v\n", err)
		return 1
	}
	return 0
}

// buildDeps groups the long-lived process-wide dependencies that build-bundle
// and build-bundles share. Construct once per process via newBuildDeps.
type buildDeps struct {
	store      *db.Store
	httpClient *httpx.Client
	pdcClient  *pdc.Client
	lwsClient  *lws.Client
	csiClient  *csi.Client
	tvwClient  *tvw.Client
}

func newBuildDeps(ctx context.Context, dsn, rawDir string, rateLimit float64) (*buildDeps, func(), error) {
	store, err := db.Open(ctx, dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("db open: %w", err)
	}
	cleanup := func() { store.Close() }

	objs, err := objectstore.NewFS(rawDir)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("objectstore: %w", err)
	}
	sink := db.RawSink{Store: store, Objects: objs, TransformVersion: "v0"}

	embedderKey := os.Getenv("INVINTUS_EMBEDDER_KEY")
	if embedderKey == "" {
		cleanup()
		return nil, nil, fmt.Errorf("INVINTUS_EMBEDDER_KEY is required for the TVW step")
	}

	httpClient := httpx.New(httpx.Config{
		UserAgent:    userAgent,
		Sink:         sink,
		Timeout:      45 * time.Second,
		MaxRetries:   2,
		RetryBackoff: 750 * time.Millisecond,
		HostRateLimit: map[string]float64{
			"app.leg.wa.gov":           rateLimit,
			"wslwebservices.leg.wa.gov": rateLimit,
			"tvw.org":                  rateLimit,
			"api.v3.invintus.com":      rateLimit,
			"data.wa.gov":              rateLimit,
		},
	})

	return &buildDeps{
		store:      store,
		httpClient: httpClient,
		pdcClient:  pdc.New(httpClient, os.Getenv("SOCRATA_APP_TOKEN")),
		lwsClient:  lws.New(httpClient),
		csiClient:  csi.New(httpClient),
		tvwClient:  tvw.New(httpClient, embedderKey),
	}, cleanup, nil
}

// metadataDeps is the trimmed dependency set for ingest-session: just LWS
// + storage + httpx. We don't need CSI/TVW/PDC for the metadata-only pass,
// and we don't want to require INVINTUS_EMBEDDER_KEY for it.
type metadataDeps struct {
	store      *db.Store
	httpClient *httpx.Client
	lwsClient  *lws.Client
}

func newMetadataDeps(ctx context.Context, dsn, rawDir string, rateLimit float64) (*metadataDeps, func(), error) {
	store, err := db.Open(ctx, dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("db open: %w", err)
	}
	cleanup := func() { store.Close() }

	objs, err := objectstore.NewFS(rawDir)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("objectstore: %w", err)
	}
	sink := db.RawSink{Store: store, Objects: objs, TransformVersion: "v0"}

	httpClient := httpx.New(httpx.Config{
		UserAgent:    userAgent,
		Sink:         sink,
		Timeout:      45 * time.Second,
		MaxRetries:   2,
		RetryBackoff: 750 * time.Millisecond,
		HostRateLimit: map[string]float64{
			"wslwebservices.leg.wa.gov": rateLimit,
		},
	})

	return &metadataDeps{
		store:      store,
		httpClient: httpClient,
		lwsClient:  lws.New(httpClient),
	}, cleanup, nil
}

// buildOne runs the full ingest+bundle pipeline for a single bill and writes
// the bundle JSON to outDir. Returns the absolute path of the written bundle.
// All errors are wrapped with the bill identifier so callers can log per-bill
// status without re-decoding.
func buildOne(ctx context.Context, deps *buildDeps, demo *config.SelectedDemo, outDir string, logf func(string)) (string, error) {
	pipeline := &jobs.Pipeline{
		Store: deps.store,
		LWS:   deps.lwsClient,
		CSI:   deps.csiClient,
		TVW:   deps.tvwClient,
		PDC:   deps.pdcClient,
		Demo:  demo,
	}
	ids := jobs.NewIDs()
	if err := pipeline.Run(ctx, logf, ids); err != nil {
		return "", err
	}

	logf("==> assembling bundle")
	bundle, err := firstpage.Build(ctx, deps.store, demo)
	if err != nil {
		return "", err
	}
	outName := fmt.Sprintf("wa_%s_%s%d.json", demo.Biennium, demo.BillPrefix, demo.BillNumber)
	outPath := filepath.Join(outDir, outName)
	if err := writeJSON(outPath, bundle); err != nil {
		return "", fmt.Errorf("write: %w", err)
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
	return outPath, nil
}

// runBuildBundles implements `wa-dd build-bundles`: the daily-batch driver.
// Reads config/selected_bills.yml, runs buildOne per entry, isolates per-bill
// errors, and writes a run-summary JSON next to the bundles. Exits non-zero if
// any bill failed so cron mail flags the run.
func runBuildBundles(args []string) int {
	fs := flag.NewFlagSet("build-bundles", flag.ContinueOnError)
	var (
		cfgPath   = fs.String("config", "config/selected_bills.yml", "path to selected_bills.yml")
		dsn       = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		rawDir    = fs.String("raw-dir", "data/raw", "filesystem root for raw API responses")
		outDir    = fs.String("out-dir", "data/processed/bundles", "where the JSON bundles are written")
		rateLimit = fs.Float64("rate", 5.0, "max requests/sec per legislative host")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	bills, err := config.LoadSelectedBills(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build-bundles: %v\n", err)
		return 1
	}

	deps, cleanup, err := newBuildDeps(ctx, *dsn, *rawDir, *rateLimit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build-bundles: %v\n", err)
		return 1
	}
	defer cleanup()

	type result struct {
		Bill        string `json:"bill"`
		Status      string `json:"status"` // "ok" | "failed"
		DurationMS  int64  `json:"duration_ms"`
		BundlePath  string `json:"bundle_path,omitempty"`
		Error       string `json:"error,omitempty"`
	}
	results := make([]result, 0, len(bills.Bills))
	startedAt := time.Now()
	failures := 0

	for i := range bills.Bills {
		demo := &bills.Bills[i]
		prefix := fmt.Sprintf("[%d/%d] %s", i+1, len(bills.Bills), demo.BillID())
		fmt.Fprintf(os.Stderr, "==> %s starting\n", prefix)

		logf := func(s string) { fmt.Fprintf(os.Stderr, "    %s\n", s) }
		t0 := time.Now()
		bundlePath, err := buildOne(ctx, deps, demo, *outDir, logf)
		dur := time.Since(t0)
		if err != nil {
			failures++
			fmt.Fprintf(os.Stderr, "    %s FAIL (%s): %v\n", prefix, dur.Round(time.Millisecond), err)
			results = append(results, result{
				Bill:       demo.BillID(),
				Status:     "failed",
				DurationMS: dur.Milliseconds(),
				Error:      err.Error(),
			})
			// Honor cancellation — don't keep iterating after Ctrl-C / SIGTERM.
			if ctx.Err() != nil {
				break
			}
			continue
		}
		fmt.Fprintf(os.Stderr, "    %s ok (%s)\n", prefix, dur.Round(time.Millisecond))
		results = append(results, result{
			Bill:       demo.BillID(),
			Status:     "ok",
			DurationMS: dur.Milliseconds(),
			BundlePath: filepath.Base(bundlePath),
		})
	}

	summary := struct {
		StartedAt  time.Time `json:"started_at"`
		FinishedAt time.Time `json:"finished_at"`
		Total      int       `json:"total"`
		Succeeded  int       `json:"succeeded"`
		Failed     int       `json:"failed"`
		Results    []result  `json:"results"`
	}{
		StartedAt:  startedAt,
		FinishedAt: time.Now(),
		Total:      len(results),
		Succeeded:  len(results) - failures,
		Failed:     failures,
		Results:    results,
	}
	summaryPath := filepath.Join(*outDir, "_run.json")
	if err := writeJSON(summaryPath, summary); err != nil {
		fmt.Fprintf(os.Stderr, "build-bundles: write _run.json: %v\n", err)
		// Don't mask a successful batch with a write-summary failure.
		if failures == 0 {
			return 1
		}
	}

	fmt.Fprintf(os.Stderr, "==> done: %d ok, %d failed (%s)\n",
		summary.Succeeded, summary.Failed, summary.FinishedAt.Sub(summary.StartedAt).Round(time.Millisecond))
	if failures > 0 {
		return 1
	}
	return 0
}

// runIngestSession implements `wa-dd ingest-session`: bulk-pull every
// bill in a biennium from LWS GetLegislationByYear and run the metadata
// -only pipeline (just IngestBill) per bill. Hearings/testimony stay
// curated via `wa-dd build-bundles` + selected_bills.yml.
func runIngestSession(args []string) int {
	fs := flag.NewFlagSet("ingest-session", flag.ContinueOnError)
	var (
		biennium  = fs.String("biennium", "2025-26", "Biennium to ingest, e.g. 2025-26")
		dsn       = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		rawDir    = fs.String("raw-dir", "data/raw", "filesystem root for raw API responses")
		outDir    = fs.String("out-dir", "data/processed", "where _session.json is written")
		rateLimit = fs.Float64("rate", 5.0, "max requests/sec for the LWS host")
		limit     = fs.Int("limit", 0, "stop after N bills (0 = no limit). For smoke tests.")
		onlyTypes = fs.String("only-types", "", "comma-separated list of bill prefixes to keep (e.g. \"HB,SB\"). Empty = all.")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	years, err := bienniumYears(*biennium)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-session: %v\n", err)
		return 1
	}

	allowedTypes := parseTypeFilter(*onlyTypes)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	deps, cleanup, err := newMetadataDeps(ctx, *dsn, *rawDir, *rateLimit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-session: %v\n", err)
		return 1
	}
	defer cleanup()

	// 1. Fetch the year-level bill index for each year of the biennium.
	type billRef struct {
		biennium string
		prefix   string
		number   int
	}
	seen := map[string]struct{}{}
	bills := make([]billRef, 0, 2000)
	for _, year := range years {
		fmt.Fprintf(os.Stderr, "==> GetLegislationByYear(%d)\n", year)
		infos, err := deps.lwsClient.GetLegislationByYear(ctx, year)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ingest-session: GetLegislationByYear(%d): %v\n", year, err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "    received %d bills\n", len(infos))
		for _, info := range infos {
			prefix, number := lws.SplitBillID(info.BillID)
			if number == 0 {
				continue
			}
			if len(allowedTypes) > 0 {
				if _, ok := allowedTypes[prefix]; !ok {
					continue
				}
			}
			key := info.Biennium + "|" + info.BillID
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			bills = append(bills, billRef{
				biennium: info.Biennium,
				prefix:   prefix,
				number:   number,
			})
		}
	}
	if *limit > 0 && len(bills) > *limit {
		bills = bills[:*limit]
	}
	fmt.Fprintf(os.Stderr, "==> %d unique bills to ingest\n", len(bills))

	// 2. Loop with per-bill failure isolation.
	type result struct {
		Bill       string `json:"bill"`
		Status     string `json:"status"`
		DurationMS int64  `json:"duration_ms"`
		Error      string `json:"error,omitempty"`
	}
	results := make([]result, 0, len(bills))
	startedAt := time.Now()
	failures := 0

	for i, b := range bills {
		demo := &config.SelectedDemo{
			Biennium:   b.biennium,
			BillPrefix: b.prefix,
			BillNumber: b.number,
		}
		prefix := fmt.Sprintf("[%d/%d] %s", i+1, len(bills), demo.BillID())
		t0 := time.Now()

		pipeline := &jobs.Pipeline{
			Store: deps.store,
			LWS:   deps.lwsClient,
			Demo:  demo,
		}
		ids := jobs.NewIDs()
		// Quiet per-bill log: ingest-bill makes 4 SOAP calls; logging
		// every step at this scale would drown the output. Only failures
		// surface to stderr.
		stepErr := pipeline.RunMetadataOnly(ctx, func(string) {}, ids)
		dur := time.Since(t0)

		if stepErr != nil {
			failures++
			fmt.Fprintf(os.Stderr, "%s FAIL (%s): %v\n", prefix, dur.Round(time.Millisecond), stepErr)
			results = append(results, result{
				Bill: demo.BillID(), Status: "failed",
				DurationMS: dur.Milliseconds(), Error: stepErr.Error(),
			})
			if ctx.Err() != nil {
				break
			}
			continue
		}
		// Light progress every 50 bills so an hour-long run doesn't go silent.
		if (i+1)%50 == 0 || i+1 == len(bills) {
			fmt.Fprintf(os.Stderr, "%s ok (%s)\n", prefix, dur.Round(time.Millisecond))
		}
		results = append(results, result{
			Bill: demo.BillID(), Status: "ok",
			DurationMS: dur.Milliseconds(),
		})
	}

	summary := struct {
		StartedAt  time.Time `json:"started_at"`
		FinishedAt time.Time `json:"finished_at"`
		Biennium   string    `json:"biennium"`
		Total      int       `json:"total"`
		Succeeded  int       `json:"succeeded"`
		Failed     int       `json:"failed"`
		Results    []result  `json:"results"`
	}{
		StartedAt:  startedAt,
		FinishedAt: time.Now(),
		Biennium:   *biennium,
		Total:      len(results),
		Succeeded:  len(results) - failures,
		Failed:     failures,
		Results:    results,
	}
	summaryPath := filepath.Join(*outDir, "_session.json")
	if err := writeJSON(summaryPath, summary); err != nil {
		fmt.Fprintf(os.Stderr, "ingest-session: write _session.json: %v\n", err)
		if failures == 0 {
			return 1
		}
	}

	fmt.Fprintf(os.Stderr, "==> done: %d ok, %d failed (%s)\n",
		summary.Succeeded, summary.Failed, summary.FinishedAt.Sub(summary.StartedAt).Round(time.Millisecond))
	if failures > 0 {
		return 1
	}
	return 0
}

// bienniumYears parses "2025-26" → [2025, 2026]. Errors on malformed input.
func bienniumYears(b string) ([]int, error) {
	parts := strings.SplitN(b, "-", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid biennium %q (expected e.g. 2025-26)", b)
	}
	startYear, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid biennium start year: %w", err)
	}
	endYear := startYear + 1
	if len(parts[1]) == 2 {
		// "26" → 2026
		yy, err := strconv.Atoi(parts[1])
		if err == nil {
			endYear = (startYear/100)*100 + yy
		}
	} else if len(parts[1]) == 4 {
		yy, err := strconv.Atoi(parts[1])
		if err == nil {
			endYear = yy
		}
	}
	return []int{startYear, endYear}, nil
}

func parseTypeFilter(s string) map[string]struct{} {
	if s == "" {
		return nil
	}
	out := map[string]struct{}{}
	for _, t := range strings.Split(s, ",") {
		t = strings.TrimSpace(strings.ToUpper(t))
		if t != "" {
			out[t] = struct{}{}
		}
	}
	return out
}

// discoveryDeps adds CSI + TVW to the metadata-only set. TVW is
// constructed without an embedder key — only FetchWPVideoArchive is
// used during discovery, and that endpoint doesn't require it.
type discoveryDeps struct {
	store      *db.Store
	httpClient *httpx.Client
	csiClient  *csi.Client
	tvwClient  *tvw.Client
}

func newDiscoveryDeps(ctx context.Context, dsn, rawDir string, rateLimit float64) (*discoveryDeps, func(), error) {
	store, err := db.Open(ctx, dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("db open: %w", err)
	}
	cleanup := func() { store.Close() }

	objs, err := objectstore.NewFS(rawDir)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("objectstore: %w", err)
	}
	sink := db.RawSink{Store: store, Objects: objs, TransformVersion: "v0"}

	httpClient := httpx.New(httpx.Config{
		UserAgent:    userAgent,
		Sink:         sink,
		Timeout:      45 * time.Second,
		MaxRetries:   2,
		RetryBackoff: 750 * time.Millisecond,
		HostRateLimit: map[string]float64{
			"app.leg.wa.gov":      rateLimit,
			"tvw.org":             rateLimit,
			"api.v3.invintus.com": rateLimit,
		},
	})

	return &discoveryDeps{
		store:      store,
		httpClient: httpClient,
		csiClient:  csi.New(httpClient),
		// Empty embedder key is fine: discovery only calls FetchWPVideoArchive,
		// which hits TVW's WP API and doesn't need it.
		tvwClient: tvw.New(httpClient, ""),
	}, cleanup, nil
}

// runDiscoverHearings implements `wa-dd discover-hearings`: walks every
// LWS-ingested hearing in the biennium that's still missing CSI/TVW IDs
// and fills them in via Discoverer. Per-hearing failure isolation;
// summary at data/processed/_discovery.json.
func runDiscoverHearings(args []string) int {
	fs := flag.NewFlagSet("discover-hearings", flag.ContinueOnError)
	var (
		biennium  = fs.String("biennium", "2025-26", "Biennium to scan, e.g. 2025-26")
		dsn       = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		rawDir    = fs.String("raw-dir", "data/raw", "filesystem root for raw API responses")
		outDir    = fs.String("out-dir", "data/processed", "where _discovery.json is written")
		rateLimit = fs.Float64("rate", 5.0, "max requests/sec per CSI/TVW host")
		limit     = fs.Int("limit", 0, "stop after N hearings (0 = no limit). For smoke tests.")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	deps, cleanup, err := newDiscoveryDeps(ctx, *dsn, *rawDir, *rateLimit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "discover-hearings: %v\n", err)
		return 1
	}
	defer cleanup()

	hearings, err := deps.store.ListHearingsForDiscovery(ctx, *biennium)
	if err != nil {
		fmt.Fprintf(os.Stderr, "discover-hearings: %v\n", err)
		return 1
	}
	if *limit > 0 && len(hearings) > *limit {
		hearings = hearings[:*limit]
	}
	fmt.Fprintf(os.Stderr, "==> %d hearings to discover\n", len(hearings))

	disc := jobs.NewDiscoverer(jobs.DiscoveryDeps{
		Store: deps.store,
		CSI:   deps.csiClient,
		TVW:   deps.tvwClient,
	})

	type result struct {
		HearingID    int64  `json:"hearing_id"`
		Bill         string `json:"bill"`
		Status       string `json:"status"` // "ok" | "failed" | "no-tvw"
		CSIAgendaID  string `json:"csi_agenda_item_id,omitempty"`
		TVWEventID   string `json:"tvw_event_id,omitempty"`
		Error        string `json:"error,omitempty"`
		DurationMS   int64  `json:"duration_ms"`
	}
	results := make([]result, 0, len(hearings))
	startedAt := time.Now()
	failures, partial := 0, 0

	for i, h := range hearings {
		bill := fmt.Sprintf("%s %d", h.BillPrefix, h.BillNumber)
		t0 := time.Now()
		res, err := disc.DiscoverOne(ctx, h)
		dur := time.Since(t0)
		if err != nil {
			failures++
			fmt.Fprintf(os.Stderr, "[%d/%d] %s FAIL: %v\n", i+1, len(hearings), bill, err)
			results = append(results, result{
				HearingID: h.HearingID, Bill: bill, Status: "failed",
				Error: err.Error(), DurationMS: dur.Milliseconds(),
			})
			if ctx.Err() != nil {
				break
			}
			continue
		}
		if err := disc.Commit(ctx, h, res); err != nil {
			failures++
			fmt.Fprintf(os.Stderr, "[%d/%d] %s commit FAIL: %v\n", i+1, len(hearings), bill, err)
			results = append(results, result{
				HearingID: h.HearingID, Bill: bill, Status: "failed",
				Error: err.Error(), DurationMS: dur.Milliseconds(),
			})
			continue
		}
		status := "ok"
		if res.TVWEventID == "" {
			status = "no-tvw"
			partial++
		}
		// Quiet per-hearing log; spam at the 50-row mark instead.
		if (i+1)%50 == 0 || i+1 == len(hearings) {
			fmt.Fprintf(os.Stderr, "[%d/%d] %s %s (%s)\n", i+1, len(hearings), bill, status, dur.Round(time.Millisecond))
		}
		results = append(results, result{
			HearingID: h.HearingID, Bill: bill, Status: status,
			CSIAgendaID: res.CSIAgendaItemID, TVWEventID: res.TVWEventID,
			DurationMS: dur.Milliseconds(),
		})
	}

	summary := struct {
		StartedAt    time.Time `json:"started_at"`
		FinishedAt   time.Time `json:"finished_at"`
		Biennium     string    `json:"biennium"`
		Total        int       `json:"total"`
		Succeeded    int       `json:"succeeded"`
		PartialNoTVW int       `json:"partial_no_tvw"`
		Failed       int       `json:"failed"`
		Results      []result  `json:"results"`
	}{
		StartedAt:    startedAt,
		FinishedAt:   time.Now(),
		Biennium:     *biennium,
		Total:        len(results),
		Succeeded:    len(results) - failures - partial,
		PartialNoTVW: partial,
		Failed:       failures,
		Results:      results,
	}
	if err := writeJSON(filepath.Join(*outDir, "_discovery.json"), summary); err != nil {
		fmt.Fprintf(os.Stderr, "discover-hearings: write _discovery.json: %v\n", err)
		if failures == 0 {
			return 1
		}
	}
	fmt.Fprintf(os.Stderr, "==> done: %d ok, %d no-tvw, %d failed (%s)\n",
		summary.Succeeded, summary.PartialNoTVW, summary.Failed,
		summary.FinishedAt.Sub(summary.StartedAt).Round(time.Millisecond))
	if failures > 0 {
		return 1
	}
	return 0
}

// runIngestHearings implements `wa-dd ingest-hearings`: for every
// agenda_item whose hearing has a TVW event but no testifiers yet,
// run the full curated pipeline (buildOne) so the hearing's testimony,
// transcript, and PDC context get ingested. Reuses
// firstpage.LookupSelectedDemoByAgendaItem so we don't re-derive the
// SelectedDemo by hand.
func runIngestHearings(args []string) int {
	fs := flag.NewFlagSet("ingest-hearings", flag.ContinueOnError)
	var (
		biennium  = fs.String("biennium", "2025-26", "Biennium to scan, e.g. 2025-26")
		dsn       = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		rawDir    = fs.String("raw-dir", "data/raw", "filesystem root for raw API responses")
		outDir    = fs.String("out-dir", "data/processed/bundles", "where bundle JSON is written")
		rateLimit = fs.Float64("rate", 5.0, "max requests/sec per legislative host")
		limit     = fs.Int("limit", 0, "stop after N agenda items (0 = no limit). For smoke tests.")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	deps, cleanup, err := newBuildDeps(ctx, *dsn, *rawDir, *rateLimit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-hearings: %v\n", err)
		return 1
	}
	defer cleanup()

	rows, err := deps.store.ListDiscoveredAgendaItems(ctx, *biennium)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-hearings: %v\n", err)
		return 1
	}
	if *limit > 0 && len(rows) > *limit {
		rows = rows[:*limit]
	}
	fmt.Fprintf(os.Stderr, "==> %d agenda items to ingest\n", len(rows))

	type result struct {
		Bill            string `json:"bill"`
		CSIAgendaItemID string `json:"csi_agenda_item_id"`
		Status          string `json:"status"`
		DurationMS      int64  `json:"duration_ms"`
		BundlePath      string `json:"bundle_path,omitempty"`
		Error           string `json:"error,omitempty"`
	}
	results := make([]result, 0, len(rows))
	startedAt := time.Now()
	failures := 0

	for i, r := range rows {
		demo, err := firstpage.LookupSelectedDemoByAgendaItem(ctx, deps.store, r.CSIAgendaItemID)
		if err != nil {
			failures++
			fmt.Fprintf(os.Stderr, "[%d/%d] %s lookup FAIL: %v\n", i+1, len(rows), r.CSIAgendaItemID, err)
			results = append(results, result{
				Bill: fmt.Sprintf("%s %d", r.BillPrefix, r.BillNumber),
				CSIAgendaItemID: r.CSIAgendaItemID,
				Status: "failed", Error: err.Error(),
			})
			continue
		}
		prefix := fmt.Sprintf("[%d/%d] %s", i+1, len(rows), demo.BillID())
		fmt.Fprintf(os.Stderr, "==> %s starting (agenda=%s)\n", prefix, r.CSIAgendaItemID)
		t0 := time.Now()
		logf := func(s string) { fmt.Fprintf(os.Stderr, "    %s\n", s) }
		bundlePath, err := buildOne(ctx, deps, demo, *outDir, logf)
		dur := time.Since(t0)
		if err != nil {
			failures++
			fmt.Fprintf(os.Stderr, "    %s FAIL (%s): %v\n", prefix, dur.Round(time.Millisecond), err)
			results = append(results, result{
				Bill: demo.BillID(), CSIAgendaItemID: r.CSIAgendaItemID,
				Status: "failed", Error: err.Error(),
				DurationMS: dur.Milliseconds(),
			})
			if ctx.Err() != nil {
				break
			}
			continue
		}
		fmt.Fprintf(os.Stderr, "    %s ok (%s)\n", prefix, dur.Round(time.Millisecond))
		results = append(results, result{
			Bill: demo.BillID(), CSIAgendaItemID: r.CSIAgendaItemID,
			Status: "ok", DurationMS: dur.Milliseconds(),
			BundlePath: filepath.Base(bundlePath),
		})
	}

	summary := struct {
		StartedAt  time.Time `json:"started_at"`
		FinishedAt time.Time `json:"finished_at"`
		Biennium   string    `json:"biennium"`
		Total      int       `json:"total"`
		Succeeded  int       `json:"succeeded"`
		Failed     int       `json:"failed"`
		Results    []result  `json:"results"`
	}{
		StartedAt: startedAt, FinishedAt: time.Now(),
		Biennium: *biennium, Total: len(results),
		Succeeded: len(results) - failures, Failed: failures,
		Results: results,
	}
	if err := writeJSON(filepath.Join(*outDir, "..", "_ingest.json"), summary); err != nil {
		fmt.Fprintf(os.Stderr, "ingest-hearings: write _ingest.json: %v\n", err)
		if failures == 0 {
			return 1
		}
	}
	fmt.Fprintf(os.Stderr, "==> done: %d ok, %d failed (%s)\n",
		summary.Succeeded, summary.Failed,
		summary.FinishedAt.Sub(summary.StartedAt).Round(time.Millisecond))
	if failures > 0 {
		return 1
	}
	return 0
}
