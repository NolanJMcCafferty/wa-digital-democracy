package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/jobs"
)

func init() {
	register(Command{
		Name:     "discover-hearings",
		Synopsis: "Auto-fill CSI/TVW IDs on every LWS hearing in a biennium",
		Run:      runDiscoverHearings,
	})
}
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
// pageassembly.LookupBillAgendaTargetByAgendaItem so we don't re-derive the
