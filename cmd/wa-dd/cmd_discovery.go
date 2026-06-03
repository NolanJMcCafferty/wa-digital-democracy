package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/jobs"
)

const sessionDiscoveryRateLimit = 10.0

func runHearingDiscovery(ctx context.Context, deps *discoveryDeps, biennium, outDir, logPrefix string) int {
	hearings, err := deps.store.ListHearingsForDiscovery(ctx, biennium)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", logPrefix, err)
		return 1
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
		Biennium:           biennium,
		Total:              len(results),
		Succeeded:          len(results) - failures - partial - noMeeting - noAgenda - noCommittee,
		PartialNoTVW:       partial,
		SkippedNoMeeting:   noMeeting,
		SkippedNoAgenda:    noAgenda,
		SkippedNoCommittee: noCommittee,
		Failed:             failures,
		Results:            results,
	}
	if err := writeJSON(filepath.Join(outDir, "_discovery.json"), summary); err != nil {
		fmt.Fprintf(os.Stderr, "%s: write _discovery.json: %v\n", logPrefix, err)
		if failures == 0 {
			return 1
		}
	}
	fmt.Fprintf(os.Stderr, "==> discovery done: %d ok, %d no-tvw, %d no-csi-meeting, %d no-agenda-item, %d no-committee, %d failed (%s)\n",
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
