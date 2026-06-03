package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/common"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/jobs"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/lws"
)

func init() {
	register(Command{
		Name:     "ingest-session",
		Synopsis: "Ingest LWS metadata for every bill in a biennium (no hearings)",
		Run:      runIngestSession,
	})
}
func runIngestSession(args []string) int {
	fs := flag.NewFlagSet("ingest-session", flag.ContinueOnError)
	var (
		biennium  = fs.String("biennium", "2025-26", "Biennium to ingest, e.g. 2025-26")
		dsn       = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		rawDir    = fs.String("raw-dir", "data/raw", "filesystem root for raw API responses")
		outDir    = fs.String("out-dir", "data/processed", "where _session.json is written")
		rateLimit = fs.Float64("rate", 25.0, "max requests/sec for the LWS host")
		workers   = fs.Int("workers", 4, "number of concurrent bill workers. HTTP calls still respect --rate per LWS host.")
		limit     = fs.Int("limit", 0, "stop after N bills (0 = no limit). For smoke tests.")
		onlyTypes = fs.String("only-types", "", "comma-separated list of bill prefixes to keep (e.g. \"HB,SB\"). Empty = all.")
		skipFresh = fs.Duration("skip-fresh", 24*time.Hour, "skip bills whose DB row was upserted within this window (0 disables). Lets a killed run resume without re-fetching already-ingested bills.")
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
	skipped := 0
	if *skipFresh > 0 {
		fresh, err := deps.store.ListFreshBillKeys(ctx, *biennium, *skipFresh)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ingest-session: list fresh bills: %v\n", err)
			return 1
		}
		filtered := bills[:0]
		for _, b := range bills {
			key := fmt.Sprintf("%s|%d", b.prefix, b.number)
			if _, ok := fresh[key]; ok {
				skipped++
				continue
			}
			filtered = append(filtered, b)
		}
		bills = filtered
	}
	if *limit > 0 && len(bills) > *limit {
		bills = bills[:*limit]
	}
	if skipped > 0 {
		fmt.Fprintf(os.Stderr, "==> %d unique bills to ingest (%d skipped as fresh within %s)\n", len(bills), skipped, *skipFresh)
	} else {
		fmt.Fprintf(os.Stderr, "==> %d unique bills to ingest\n", len(bills))
	}

	// 2. Process with per-bill failure isolation. Multiple workers improve
	// wall-clock time while the shared httpx client still enforces the LWS
	// per-host token bucket from --rate. Results are sorted back into input
	// order so _session.json stays stable.
	type result struct {
		Index      int    `json:"-"`
		Bill       string `json:"bill"`
		Status     string `json:"status"`
		DurationMS int64  `json:"duration_ms"`
		Error      string `json:"error,omitempty"`
	}
	startedAt := time.Now()
	workerCount := *workers
	if workerCount <= 0 {
		workerCount = 1
	}
	if workerCount > len(bills) && len(bills) > 0 {
		workerCount = len(bills)
	}
	if workerCount == 0 {
		workerCount = 1
	}
	fmt.Fprintf(os.Stderr, "==> ingest-session workers=%d rate=%.2f req/sec\n", workerCount, *rateLimit)

	type job struct {
		index int
		bill  billRef
	}
	jobsCh := make(chan job)
	resultsCh := make(chan result, len(bills))
	var wg sync.WaitGroup
	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := range jobsCh {
				b := j.bill
				billAgendaTarget := &common.BillAgendaTarget{
					Bill: common.BillKey{Biennium: b.biennium, Prefix: b.prefix, Number: b.number},
				}
				prefix := fmt.Sprintf("[%d/%d] %s", j.index+1, len(bills), billAgendaTarget.Bill.ID())
				t0 := time.Now()
				pipeline := &jobs.Pipeline{
					Store:            deps.store,
					LWS:              deps.lwsClient,
					BillAgendaTarget: billAgendaTarget,
				}
				ids := jobs.NewIDs()
				stepErr := pipeline.RunMetadataOnly(ctx, func(string) {}, ids)
				dur := time.Since(t0)
				res := result{Index: j.index, Bill: billAgendaTarget.Bill.ID(), DurationMS: dur.Milliseconds()}
				if stepErr != nil {
					res.Status = "failed"
					res.Error = stepErr.Error()
					fmt.Fprintf(os.Stderr, "%s FAIL worker=%d (%s): %v\n", prefix, workerID, dur.Round(time.Millisecond), stepErr)
				} else {
					res.Status = "ok"
					// Light progress every 50 bills so long runs don't go silent.
					if (j.index+1)%50 == 0 || j.index+1 == len(bills) {
						fmt.Fprintf(os.Stderr, "%s ok worker=%d (%s)\n", prefix, workerID, dur.Round(time.Millisecond))
					}
				}
				select {
				case resultsCh <- res:
				case <-ctx.Done():
					return
				}
				if ctx.Err() != nil {
					return
				}
			}
		}(w + 1)
	}

	go func() {
		defer close(jobsCh)
		for i, b := range bills {
			select {
			case jobsCh <- job{index: i, bill: b}:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	results := make([]result, 0, len(bills))
	failures := 0
	for res := range resultsCh {
		if res.Status == "failed" {
			failures++
		}
		results = append(results, res)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Index < results[j].Index })

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
