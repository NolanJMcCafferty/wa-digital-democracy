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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/candidate"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/config"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/diarization"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/entitymatch"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/jobs"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/render/firstpage"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/csi"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/datawa"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/fiscalwa"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/lws"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/pdc"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/seattle"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/socrata"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/tvw"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/usaspending"
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
  ingest-legislators Pull the full House+Senate roster for a biennium from LWS
  ingest-session     Ingest LWS metadata for every bill in a biennium (no hearings)
  discover-hearings  Auto-fill CSI/TVW IDs on every LWS hearing in a biennium
  ingest-hearings    Run the full pipeline for every discovered agenda item
  ingest-contracts   Pull DataWA agency contract rows into Postgres
  ingest-master-contract-sales
                     Pull DataWA statewide/master-contract sales rows into Postgres
  ingest-it-contracts
                     Pull DataWA IT contracts report rows into Postgres
  ingest-webs-vendors
                     Pull DataWA WEBS vendor rows into Postgres
  generate-vendor-entity-matches
                     Generate reviewable vendor/customer organization match candidates
  ingest-usaspending-wa-awards
                     Pull USAspending award rows performed in Washington into Postgres
  ingest-seattle-operating-budget
                     Pull Seattle operating budget rows into Postgres
  ingest-fiscal-vendor-payments
                     Pull fiscal.wa.gov Open Checkbook vendor payments into Postgres
  audio-cache        Download and normalize TVW/Invintus audio for diarization
  diarize-event      Run provider diarization for a cached TVW event audio asset
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
	case "ingest-legislators":
		os.Exit(runIngestLegislators(args))
	case "ingest-session":
		os.Exit(runIngestSession(args))
	case "discover-hearings":
		os.Exit(runDiscoverHearings(args))
	case "ingest-hearings":
		os.Exit(runIngestHearings(args))
	case "ingest-contracts":
		os.Exit(runIngestContracts(args))
	case "ingest-master-contract-sales":
		os.Exit(runIngestMasterContractSales(args))
	case "ingest-it-contracts":
		os.Exit(runIngestITContracts(args))
	case "ingest-webs-vendors":
		os.Exit(runIngestWEBSVendors(args))
	case "generate-vendor-entity-matches":
		os.Exit(runGenerateVendorEntityMatches(args))
	case "ingest-usaspending-wa-awards":
		os.Exit(runIngestUSASpendingWAAwards(args))
	case "ingest-seattle-operating-budget":
		os.Exit(runIngestSeattleOperatingBudget(args))
	case "ingest-fiscal-vendor-payments":
		os.Exit(runIngestFiscalVendorPayments(args))
	case "audio-cache":
		os.Exit(runAudioCache(args))
	case "diarize-event":
		os.Exit(runDiarizeEvent(args))
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
		rateLimit   = fs.Float64("rate", 10.0, "max requests/sec to app.leg.wa.gov")
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
//  1. Load selected_demo.yml.
//  2. Open Postgres + the filesystem object store and wire them into the
//     shared httpx.RawSink so every fetch records provenance.
//  3. Run the eight pipeline jobs (Blueprint Steps 1–8 minus the optional
//     Committee Schedules enrichment, which the operator covered by
//     pasting tvw.event_id into the demo config).
//  4. Assemble the JSON bundle and write to data/processed/bundles/.
func runBuildBundle(args []string) int {
	fs := flag.NewFlagSet("build-bundle", flag.ContinueOnError)
	var (
		cfgPath   = fs.String("config", "config/selected_demo.yml", "path to selected_demo.yml")
		dsn       = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		rawDir    = fs.String("raw-dir", "data/raw", "filesystem root for raw API responses")
		outDir    = fs.String("out-dir", "data/processed/bundles", "where the JSON bundle is written")
		rateLimit = fs.Float64("rate", 10.0, "max requests/sec for legislative APIs")
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
// and ingest-hearings share. Construct once per process via newBuildDeps.
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
			"app.leg.wa.gov":            rateLimit,
			"wslwebservices.leg.wa.gov": rateLimit,
			"tvw.org":                   rateLimit,
			"api.v3.invintus.com":       rateLimit,
			"data.wa.gov":               rateLimit,
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

// runIngestSession implements `wa-dd ingest-session`: bulk-pull every
// bill in a biennium from LWS GetLegislationByYear and run the metadata
// -only pipeline (just IngestBill) per bill. Hearings/testimony are
// filled in by `wa-dd discover-hearings` + `wa-dd ingest-hearings`.
func runIngestSession(args []string) int {
	fs := flag.NewFlagSet("ingest-session", flag.ContinueOnError)
	var (
		biennium  = fs.String("biennium", "2025-26", "Biennium to ingest, e.g. 2025-26")
		dsn       = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		rawDir    = fs.String("raw-dir", "data/raw", "filesystem root for raw API responses")
		outDir    = fs.String("out-dir", "data/processed", "where _session.json is written")
		rateLimit = fs.Float64("rate", 10.0, "max requests/sec for the LWS host")
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
		rateLimit = fs.Float64("rate", 10.0, "max requests/sec per CSI/TVW host")
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
		HearingID   int64  `json:"hearing_id"`
		Bill        string `json:"bill"`
		Status      string `json:"status"` // "ok" | "failed" | "no-tvw" | "no-csi-meeting" | "no-agenda-item" | "no-committee"
		CSIAgendaID string `json:"csi_agenda_item_id,omitempty"`
		TVWEventID  string `json:"tvw_event_id,omitempty"`
		Error       string `json:"error,omitempty"`
		DurationMS  int64  `json:"duration_ms"`
	}
	results := make([]result, 0, len(hearings))
	startedAt := time.Now()
	failures, partial := 0, 0
	noMeeting, noAgenda, noCommittee := 0, 0, 0

	for i, h := range hearings {
		bill := fmt.Sprintf("%s %d", h.BillPrefix, h.BillNumber)
		t0 := time.Now()
		res, err := disc.DiscoverOne(ctx, h)
		dur := time.Since(t0)
		if err != nil {
			if status, ok := nonfatalDiscoveryStatus(err); ok {
				switch status {
				case "no-csi-meeting":
					noMeeting++
				case "no-agenda-item":
					noAgenda++
				case "no-committee":
					noCommittee++
				}
				if (i+1)%50 == 0 || i+1 == len(hearings) {
					fmt.Fprintf(os.Stderr, "[%d/%d] %s %s (%s)\n", i+1, len(hearings), bill, status, dur.Round(time.Millisecond))
				}
				results = append(results, result{
					HearingID: h.HearingID, Bill: bill, Status: status,
					Error: err.Error(), DurationMS: dur.Milliseconds(),
				})
				continue
			}
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
		StartedAt          time.Time `json:"started_at"`
		FinishedAt         time.Time `json:"finished_at"`
		Biennium           string    `json:"biennium"`
		Total              int       `json:"total"`
		Succeeded          int       `json:"succeeded"`
		PartialNoTVW       int       `json:"partial_no_tvw"`
		SkippedNoMeeting   int       `json:"skipped_no_meeting"`
		SkippedNoAgenda    int       `json:"skipped_no_agenda"`
		SkippedNoCommittee int       `json:"skipped_no_committee"`
		Failed             int       `json:"failed"`
		Results            []result  `json:"results"`
	}{
		StartedAt:          startedAt,
		FinishedAt:         time.Now(),
		Biennium:           *biennium,
		Total:              len(results),
		Succeeded:          len(results) - failures - partial - noMeeting - noAgenda - noCommittee,
		PartialNoTVW:       partial,
		SkippedNoMeeting:   noMeeting,
		SkippedNoAgenda:    noAgenda,
		SkippedNoCommittee: noCommittee,
		Failed:             failures,
		Results:            results,
	}
	if err := writeJSON(filepath.Join(*outDir, "_discovery.json"), summary); err != nil {
		fmt.Fprintf(os.Stderr, "discover-hearings: write _discovery.json: %v\n", err)
		if failures == 0 {
			return 1
		}
	}
	fmt.Fprintf(os.Stderr, "==> done: %d ok, %d no-tvw, %d no-csi-meeting, %d no-agenda-item, %d no-committee, %d failed (%s)\n",
		summary.Succeeded, summary.PartialNoTVW, summary.SkippedNoMeeting,
		summary.SkippedNoAgenda, summary.SkippedNoCommittee, summary.Failed,
		summary.FinishedAt.Sub(summary.StartedAt).Round(time.Millisecond))
	if failures > 0 {
		return 1
	}
	return 0
}

func nonfatalDiscoveryStatus(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "meeting: no CSI meeting within "):
		return "no-csi-meeting", true
	case strings.HasPrefix(msg, "agenda item: no agenda item for bill number "):
		return "no-agenda-item", true
	case strings.HasPrefix(msg, "committee: no CSI committee match for "):
		return "no-committee", true
	default:
		return "", false
	}
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
		rateLimit = fs.Float64("rate", 10.0, "max requests/sec per legislative host")
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
				Bill:            fmt.Sprintf("%s %d", r.BillPrefix, r.BillNumber),
				CSIAgendaItemID: r.CSIAgendaItemID,
				Status:          "failed", Error: err.Error(),
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

// runIngestContracts implements `wa-dd ingest-contracts`: pulls one DataWA
// agency-contract fiscal-year dataset into the normalized datawa_contract table.
// This is the first narrow budget/spending/contracts connector for Phase 4.
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

	objs, err := objectstore.NewFS(*rawDir)
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

func normalizeMoney(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), "$", ""), ",", "")
}

// runIngestMasterContractSales implements `wa-dd ingest-master-contract-sales`:
// pulls the DataWA statewide/master-contract sales dataset into its own
// normalized table. This keeps annual agency contract rows separate from vendor
// reported sales by customer, contract, and vendor.
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
	objs, err := objectstore.NewFS(*rawDir)
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
// follow-on because its grain is category/period spending rather than contracts.
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
	objs, err := objectstore.NewFS(*rawDir)
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
// command only stores source vendor facts and normalized candidate names.
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
	objs, err := objectstore.NewFS(*rawDir)
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
// and confidence labels but does not mark uncertain matches authoritative.
func runGenerateVendorEntityMatches(args []string) int {
	fs := flag.NewFlagSet("generate-vendor-entity-matches", flag.ContinueOnError)
	var (
		limit   = fs.Int("limit", 0, "maximum source rows to inspect (0 = no practical limit)")
		dsn     = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		jsonOut = fs.Bool("json", false, "write generated candidates as JSON to stdout")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate-vendor-entity-matches: db open: %v\n", err)
		return 1
	}
	defer store.Close()

	candidates, err := store.GenerateVendorEntityMatchCandidates(ctx, *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate-vendor-entity-matches: %v\n", err)
		return 1
	}
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(candidates); err != nil {
			fmt.Fprintf(os.Stderr, "generate-vendor-entity-matches: encode: %v\n", err)
			return 1
		}
	}
	fmt.Fprintf(os.Stderr, "==> generated %d reviewable vendor/entity match candidates\n", len(candidates))
	return 0
}

// runIngestUSASpendingWAAwards implements `wa-dd ingest-usaspending-wa-awards`:
// pulls a scoped USAspending award search page for awards performed in
// Washington. The first scope is deliberately broad-but-bounded: place of
// performance = WA over a caller-specified date range, sorted by award amount.
// Entity matching remains candidate-only and reviewable; this command stores
// source awards without asserting joins to local entities.
func runIngestUSASpendingWAAwards(args []string) int {
	fs := flag.NewFlagSet("ingest-usaspending-wa-awards", flag.ContinueOnError)
	var (
		startDate = fs.String("start-date", "2025-10-01", "award action date range start, YYYY-MM-DD")
		endDate   = fs.String("end-date", "2026-09-30", "award action date range end, YYYY-MM-DD")
		limit     = fs.Int("limit", 100, "maximum awards to request from USAspending")
		dsn       = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		rawDir    = fs.String("raw-dir", "data/raw", "filesystem root for raw API responses")
		rateLimit = fs.Float64("rate", 10.0, "max requests/sec for api.usaspending.gov")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *limit <= 0 || *limit > 100 {
		fmt.Fprintln(os.Stderr, "ingest-usaspending-wa-awards: --limit must be between 1 and 100 for the first MVP page")
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-usaspending-wa-awards: db open: %v\n", err)
		return 1
	}
	defer store.Close()
	objs, err := objectstore.NewFS(*rawDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-usaspending-wa-awards: objectstore: %v\n", err)
		return 1
	}
	httpClient := httpx.New(httpx.Config{
		UserAgent:    userAgent,
		Sink:         db.RawSink{Store: store, Objects: objs, TransformVersion: "v0"},
		Timeout:      45 * time.Second,
		MaxRetries:   2,
		RetryBackoff: 750 * time.Millisecond,
		HostRateLimit: map[string]float64{
			"api.usaspending.gov": *rateLimit,
		},
	})
	client := usaspending.New(httpClient)
	req := usaspending.WashingtonAwardSearchRequest(*startDate, *endDate)
	req.Limit = *limit
	resp, fetch, err := client.SearchAwardsWithSource(ctx, req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-usaspending-wa-awards: fetch: %v\n", err)
		return 1
	}
	if fetch.SourceRecordID == 0 {
		fmt.Fprintln(os.Stderr, "ingest-usaspending-wa-awards: source record was not captured")
		return 1
	}

	var upserted int
	for _, row := range resp.Results {
		award := usaspending.NormalizeAward(row)
		if award.AwardID == "" {
			fmt.Fprintln(os.Stderr, "ingest-usaspending-wa-awards: skipping award with empty Award ID")
			continue
		}
		if err := store.UpsertFederalAward(ctx, db.UpsertFederalAwardParams{
			AwardID:        award.AwardID,
			RecipientName:  award.RecipientName,
			RecipientUEI:   award.RecipientUEI,
			AwardingAgency: award.AwardingAgency,
			FundingAgency:  award.FundingAgency,
			AwardType:      award.AwardType,
			AwardAmount:    normalizeMoney(award.AwardAmount),
			StartDate:      award.StartDate,
			EndDate:        award.EndDate,
			PlaceStateCode: award.PlaceStateCode,
			PlaceCounty:    award.PlaceCounty,
			RawFields:      award.Raw,
			SourceRecordID: fetch.SourceRecordID,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "ingest-usaspending-wa-awards: upsert award %s: %v\n", award.AwardID, err)
			return 1
		}
		upserted++
	}
	fmt.Fprintf(os.Stderr, "==> ingested %d USAspending WA award rows\n", upserted)
	return 0
}

// runIngestSeattleOperatingBudget implements `wa-dd ingest-seattle-operating-budget`:
// pulls the City of Seattle Operating Budget Socrata dataset into a normalized
// table keyed by fiscal year, department, program, fund, and expense category.
func runIngestSeattleOperatingBudget(args []string) int {
	fs := flag.NewFlagSet("ingest-seattle-operating-budget", flag.ContinueOnError)
	var (
		limit     = fs.Int("limit", 1000, "maximum rows to fetch (0 = Socrata page default)")
		dsn       = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		rawDir    = fs.String("raw-dir", "data/raw", "filesystem root for raw API responses")
		rateLimit = fs.Float64("rate", 10.0, "max requests/sec for data.seattle.gov")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-seattle-operating-budget: db open: %v\n", err)
		return 1
	}
	defer store.Close()
	objs, err := objectstore.NewFS(*rawDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-seattle-operating-budget: objectstore: %v\n", err)
		return 1
	}
	httpClient := httpx.New(httpx.Config{
		UserAgent:    userAgent,
		Sink:         db.RawSink{Store: store, Objects: objs, TransformVersion: "v0"},
		Timeout:      45 * time.Second,
		MaxRetries:   2,
		RetryBackoff: 750 * time.Millisecond,
		HostRateLimit: map[string]float64{
			"data.seattle.gov": *rateLimit,
		},
	})
	client := seattle.New(httpClient, os.Getenv("SOCRATA_APP_TOKEN"))
	query := socrata.Query{Order: ":id"}
	if *limit > 0 {
		query.Limit = *limit
	}
	rows, fetch, err := client.FetchOperatingBudgetWithSource(ctx, query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-seattle-operating-budget: fetch: %v\n", err)
		return 1
	}
	if fetch.SourceRecordID == 0 {
		fmt.Fprintln(os.Stderr, "ingest-seattle-operating-budget: source record was not captured")
		return 1
	}

	var upserted int
	for _, row := range rows {
		budget := seattle.NormalizeOperatingBudget(row)
		if budget.SourceRowID == "" {
			budget.SourceRowID = seattle.StableRowID(row)
		}
		if err := store.UpsertSeattleOperatingBudget(ctx, db.UpsertSeattleOperatingBudgetParams{
			SourceDatasetID: budget.SourceDatasetID,
			SourceRowID:     budget.SourceRowID,
			FiscalYear:      budget.FiscalYear,
			Service:         budget.Service,
			Department:      budget.Department,
			Program:         budget.Program,
			Fund:            budget.Fund,
			FundType:        budget.FundType,
			ExpenseType:     budget.ExpenseType,
			Description:     budget.Description,
			ApprovedAmount:  normalizeMoney(budget.ApprovedAmount),
			RawFields:       row,
			SourceRecordID:  fetch.SourceRecordID,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "ingest-seattle-operating-budget: upsert row %s: %v\n", budget.SourceRowID, err)
			return 1
		}
		upserted++
	}
	fmt.Fprintf(os.Stderr, "==> ingested %d %s Seattle operating budget rows\n", upserted, seattle.DatasetOperatingBudget)
	return 0
}

// runIngestFiscalVendorPayments implements `wa-dd ingest-fiscal-vendor-payments`:
// pulls the current fiscal.wa.gov Open Checkbook workbook into a normalized
// vendor-payment table. This is a first budget/spending slice; proposal-level
// operating/capital/transportation budgets remain separate source families.
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
	objs, err := objectstore.NewFS(*rawDir)
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
// that IngestBill alone never sees.
func runIngestLegislators(args []string) int {
	fs := flag.NewFlagSet("ingest-legislators", flag.ContinueOnError)
	var (
		biennium  = fs.String("biennium", "2025-26", "Biennium to fetch, e.g. 2025-26")
		dsn       = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		rawDir    = fs.String("raw-dir", "data/raw", "filesystem root for raw API responses")
		outDir    = fs.String("out-dir", "data/processed", "where _legislators.json is written")
		rateLimit = fs.Float64("rate", 10.0, "max requests/sec for the LWS host")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	deps, cleanup, err := newMetadataDeps(ctx, *dsn, *rawDir, *rateLimit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-legislators: %v\n", err)
		return 1
	}
	defer cleanup()

	fmt.Fprintf(os.Stderr, "==> GetSenateSponsors(%s)\n", *biennium)
	senate, err := deps.lwsClient.GetSenateSponsors(ctx, *biennium)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-legislators: senate: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "    received %d senators\n", len(senate))

	fmt.Fprintf(os.Stderr, "==> GetHouseSponsors(%s)\n", *biennium)
	house, err := deps.lwsClient.GetHouseSponsors(ctx, *biennium)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest-legislators: house: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "    received %d representatives\n", len(house))

	type result struct {
		LWSID   string `json:"lws_sponsor_id"`
		Name    string `json:"name"`
		Chamber string `json:"chamber"`
		Status  string `json:"status"`
		Error   string `json:"error,omitempty"`
	}
	results := make([]result, 0, len(senate)+len(house))
	startedAt := time.Now()
	failures := 0

	upsert := func(m lws.Member) {
		_, err := deps.store.UpsertLegislator(ctx, db.UpsertLegislatorParams{
			LWSSponsorID: m.ID,
			Name:         m.LongName, // "Senator Alvarado" — keeps existing rows that
			//                          IngestBill wrote in this form joinable.
			Chamber:   m.Agency,
			District:  m.District,
			Party:     m.Party,
			FirstName: m.FirstName,
			LastName:  m.LastName,
			Email:     m.Email,
			Phone:     m.Phone,
			Acronym:   m.Acronym,
		})
		if err != nil {
			failures++
			fmt.Fprintf(os.Stderr, "  FAIL %s %s: %v\n", m.Agency, m.LongName, err)
			results = append(results, result{
				LWSID: m.ID, Name: m.LongName, Chamber: m.Agency,
				Status: "failed", Error: err.Error(),
			})
			return
		}
		results = append(results, result{
			LWSID: m.ID, Name: m.LongName, Chamber: m.Agency,
			Status: "ok",
		})
	}

	for _, m := range senate {
		upsert(m)
	}
	for _, m := range house {
		upsert(m)
	}

	summary := struct {
		StartedAt  time.Time `json:"started_at"`
		FinishedAt time.Time `json:"finished_at"`
		Biennium   string    `json:"biennium"`
		Senate     int       `json:"senate"`
		House      int       `json:"house"`
		Total      int       `json:"total"`
		Failed     int       `json:"failed"`
		Results    []result  `json:"results"`
	}{
		StartedAt:  startedAt,
		FinishedAt: time.Now(),
		Biennium:   *biennium,
		Senate:     len(senate),
		House:      len(house),
		Total:      len(results),
		Failed:     failures,
		Results:    results,
	}
	if err := writeJSON(filepath.Join(*outDir, "_legislators.json"), summary); err != nil {
		fmt.Fprintf(os.Stderr, "ingest-legislators: write _legislators.json: %v\n", err)
		if failures == 0 {
			return 1
		}
	}
	fmt.Fprintf(os.Stderr, "==> done: %d senators + %d reps = %d (%d failed) in %s\n",
		len(senate), len(house), len(results), failures,
		summary.FinishedAt.Sub(summary.StartedAt).Round(time.Millisecond))
	if failures > 0 {
		return 1
	}
	return 0
}

func runAudioCache(args []string) int {
	fs := flag.NewFlagSet("audio-cache", flag.ContinueOnError)
	var (
		eventID    = fs.String("event-id", "", "TVW/Invintus event ID to cache audio for")
		dsn        = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		outDir     = fs.String("out-dir", "data/audio", "audio cache output root")
		rateLimit  = fs.Float64("rate", 4.0, "max requests/sec for media download host")
		keepSource = fs.Bool("keep-source", true, "keep downloaded original media next to normalized WAV")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*eventID) == "" {
		fmt.Fprintln(os.Stderr, "audio-cache: --event-id is required")
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "audio-cache: db open: %v\n", err)
		return 1
	}
	defer store.Close()

	src, err := store.BestTVWAudioSource(ctx, *eventID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "audio-cache: find audio source: %v\n", err)
		return 1
	}
	cacheDir := filepath.Join(*outDir, "tvw", safePathPart(*eventID))
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "audio-cache: mkdir: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "==> downloading %s (%s)\n", src.URL, src.Kind)
	origPath, hash, err := downloadMedia(ctx, src.URL, cacheDir, *rateLimit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "audio-cache: download: %v\n", err)
		return 1
	}
	wavPath := filepath.Join(cacheDir, "mono_16k.wav")
	if err := normalizeAudio(ctx, origPath, wavPath); err != nil {
		fmt.Fprintf(os.Stderr, "audio-cache: ffmpeg normalize: %v\n", err)
		return 1
	}
	meta := probeAudio(ctx, wavPath)
	id, err := store.UpsertTVWAudioAsset(ctx, db.UpsertTVWAudioAssetParams{
		TVWEventID:     *eventID,
		SourceURL:      src.URL,
		SourceKind:     src.Kind,
		OriginalPath:   origPath,
		NormalizedPath: wavPath,
		ContentHash:    hash,
		DurationMS:     meta.DurationMS,
		SampleRate:     meta.SampleRate,
		Channels:       meta.Channels,
		Codec:          meta.Codec,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "audio-cache: store metadata: %v\n", err)
		return 1
	}
	if !*keepSource {
		_ = os.Remove(origPath)
	}
	fmt.Fprintf(os.Stderr, "cached audio_asset_id=%d\noriginal=%s\nnormalized=%s\nsha256=%s\n", id, origPath, wavPath, hash)
	return 0
}

func runDiarizeEvent(args []string) int {
	fs := flag.NewFlagSet("diarize-event", flag.ContinueOnError)
	var (
		eventID  = fs.String("event-id", "", "TVW/Invintus event ID to diarize")
		dsn      = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		provider = fs.String("provider", "deepgram", "diarization provider (currently: deepgram)")
		model    = fs.String("model", "nova-3", "provider model")
		outDir   = fs.String("out-dir", "data/processed/diarization", "raw diarization JSON output root")
		apiKey   = fs.String("api-key", env("DEEPGRAM_API_KEY", ""), "provider API key (defaults to DEEPGRAM_API_KEY)")
		useURL   = fs.Bool("use-source-url", true, "send original TVW/Invintus URL to provider instead of uploading local normalized WAV")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*eventID) == "" {
		fmt.Fprintln(os.Stderr, "diarize-event: --event-id is required")
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "diarize-event: db open: %v\n", err)
		return 1
	}
	defer store.Close()

	audio, err := store.LatestTVWAudioAsset(ctx, *eventID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "diarize-event: latest audio asset: %v\n", err)
		fmt.Fprintln(os.Stderr, "hint: run `wa-dd audio-cache --event-id <id>` first")
		return 1
	}

	var p diarization.Provider
	switch strings.ToLower(*provider) {
	case "deepgram":
		dg, err := diarization.NewDeepgramProvider(diarization.DeepgramConfig{APIKey: *apiKey, Model: *model})
		if err != nil {
			fmt.Fprintf(os.Stderr, "diarize-event: deepgram: %v\n", err)
			return 1
		}
		p = dg
	default:
		fmt.Fprintf(os.Stderr, "diarize-event: unsupported provider %q\n", *provider)
		return 2
	}

	jobID, err := store.CreateDiarizationJob(ctx, db.CreateDiarizationJobParams{TVWEventID: *eventID, AudioAssetID: audio.ID, Provider: *provider, Model: *model})
	if err != nil {
		fmt.Fprintf(os.Stderr, "diarize-event: create job: %v\n", err)
		return 1
	}

	in := diarization.AudioInput{EventID: *eventID}
	if *useURL && audio.SourceURL != "" {
		in.URL = audio.SourceURL
	} else {
		b, err := os.ReadFile(audio.NormalizedPath)
		if err != nil {
			_ = store.FailDiarizationJob(ctx, jobID, err.Error())
			fmt.Fprintf(os.Stderr, "diarize-event: read normalized audio: %v\n", err)
			return 1
		}
		in.Bytes = b
		in.ContentType = "audio/wav"
	}

	fmt.Fprintf(os.Stderr, "==> diarizing event %s with %s (%s), job_id=%d\n", *eventID, *provider, *model, jobID)
	res, err := p.Diarize(ctx, in)
	if err != nil {
		_ = store.FailDiarizationJob(ctx, jobID, err.Error())
		fmt.Fprintf(os.Stderr, "diarize-event: provider: %v\n", err)
		return 1
	}
	rawPath := ""
	if len(res.Raw) > 0 {
		rawPath = filepath.Join(*outDir, safePathPart(*eventID), fmt.Sprintf("%s_job_%d.json", safePathPart(*provider), jobID))
		if err := os.MkdirAll(filepath.Dir(rawPath), 0o755); err != nil {
			_ = store.FailDiarizationJob(ctx, jobID, err.Error())
			fmt.Fprintf(os.Stderr, "diarize-event: mkdir raw output: %v\n", err)
			return 1
		}
		if err := os.WriteFile(rawPath, res.Raw, 0o644); err != nil {
			_ = store.FailDiarizationJob(ctx, jobID, err.Error())
			fmt.Fprintf(os.Stderr, "diarize-event: write raw output: %v\n", err)
			return 1
		}
	}
	segments := make([]db.DiarizedSegmentParams, 0, len(res.Segments))
	for _, seg := range res.Segments {
		segments = append(segments, db.DiarizedSegmentParams{
			ClusterLabel: seg.SpeakerCluster,
			StartMS:      seg.StartMS,
			EndMS:        seg.EndMS,
			Confidence:   seg.Confidence,
			Text:         seg.Text,
			Raw: map[string]any{
				"provider": res.Provider,
				"model":    res.Model,
			},
		})
	}
	entities := make([]db.EntityMentionParams, 0, len(res.Entities))
	for _, ent := range res.Entities {
		entities = append(entities, db.EntityMentionParams{
			SourceKind:     "diarization_job",
			Extractor:      res.Provider,
			Model:          res.Model,
			EntityType:     ent.Type,
			Text:           ent.Text,
			NormalizedText: ent.NormalizedText,
			StartMS:        ent.StartMS,
			EndMS:          ent.EndMS,
			StartWord:      ent.StartWord,
			EndWord:        ent.EndWord,
			Confidence:     ent.Confidence,
			Raw:            ent.Raw,
		})
	}
	if err := store.InsertDiarizationResult(ctx, db.InsertDiarizationResultParams{JobID: jobID, TVWEventID: *eventID, Provider: res.Provider, Model: res.Model, RawResultPath: rawPath, Segments: segments, Entities: entities}); err != nil {
		_ = store.FailDiarizationJob(ctx, jobID, err.Error())
		fmt.Fprintf(os.Stderr, "diarize-event: store result: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "diarize-event: stored %d segments and %d entity mentions for job_id=%d raw=%s\n", len(segments), len(entities), jobID, rawPath)
	return 0
}

type audioProbe struct {
	DurationMS int
	SampleRate int
	Channels   int
	Codec      string
}

func downloadMedia(ctx context.Context, mediaURL, dir string, rateLimit float64) (path, sha string, err error) {
	// Keep this streaming: TVW direct audio is usually manageable, but video
	// fallback assets can be multi-GB. Do not route through httpx.Do, which
	// intentionally buffers source API responses for provenance capture.
	_ = rateLimit // reserved for a future shared streaming downloader limiter
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", userAgent)
	client := &http.Client{Timeout: time.Hour}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	ext := mediaExt(mediaURL, resp.Header.Get("Content-Type"))
	tmp, err := os.CreateTemp(dir, "download_*")
	if err != nil {
		return "", "", err
	}
	tmpPath := tmp.Name()
	h := sha256.New()
	_, copyErr := io.Copy(tmp, io.TeeReader(resp.Body, h))
	closeErr := tmp.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return "", "", copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return "", "", closeErr
	}
	sha = hex.EncodeToString(h.Sum(nil))
	path = filepath.Join(dir, "original_"+sha[:12]+ext)
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return "", "", err
	}
	return path, sha, nil
}

func normalizeAudio(ctx context.Context, inPath, outPath string) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg not found on PATH: %w", err)
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", inPath, "-ac", "1", "-ar", "16000", "-vn", outPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func probeAudio(ctx context.Context, path string) audioProbe {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return audioProbe{}
	}
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "a:0", "-show_entries", "stream=codec_name,sample_rate,channels,duration", "-of", "json", path)
	out, err := cmd.Output()
	if err != nil {
		return audioProbe{}
	}
	var parsed struct {
		Streams []struct {
			CodecName  string `json:"codec_name"`
			SampleRate string `json:"sample_rate"`
			Channels   int    `json:"channels"`
			Duration   string `json:"duration"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil || len(parsed.Streams) == 0 {
		return audioProbe{}
	}
	s := parsed.Streams[0]
	sr, _ := strconv.Atoi(s.SampleRate)
	durSec, _ := strconv.ParseFloat(s.Duration, 64)
	return audioProbe{DurationMS: int(durSec*1000 + 0.5), SampleRate: sr, Channels: s.Channels, Codec: s.CodecName}
}

func mediaExt(rawURL, contentType string) string {
	lowCT := strings.ToLower(contentType)
	if strings.Contains(lowCT, "mpeg") || strings.Contains(lowCT, "mp3") {
		return ".mp3"
	}
	if strings.Contains(lowCT, "mp4") {
		return ".mp4"
	}
	base := strings.ToLower(rawURL)
	for _, ext := range []string{".mp3", ".mp4", ".m4a", ".wav", ".aac"} {
		if strings.Contains(base, ext) {
			return ext
		}
	}
	return ".bin"
}

func safePathPart(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}
