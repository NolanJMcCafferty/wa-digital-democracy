package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/diarization"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func init() {
	register(Command{
		Name:     "merge-segments",
		Synopsis: "Collapse adjacent same-speaker diarized segments into one block per turn",
		Run:      runMergeSegments,
	})
}
func runMergeSegments(args []string) int {
	fs := flag.NewFlagSet("merge-segments", flag.ContinueOnError)
	var (
		eventID = fs.String("event-id", "", "TVW/Invintus event ID to re-merge (empty = all succeeded jobs)")
		dsn     = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		limit   = fs.Int("limit", 0, "max events to process (0 = unlimited)")
		dryRun  = fs.Bool("dry-run", false, "print before/after segment counts without writing")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "merge-segments: db open: %v\n", err)
		return 1
	}
	defer store.Close()

	jobs, err := store.ListSucceededDiarizationJobs(ctx, strings.TrimSpace(*eventID))
	if err != nil {
		fmt.Fprintf(os.Stderr, "merge-segments: %v\n", err)
		return 1
	}
	if len(jobs) == 0 {
		fmt.Fprintln(os.Stderr, "merge-segments: no succeeded diarization jobs found")
		return 0
	}
	if *limit > 0 && len(jobs) > *limit {
		jobs = jobs[:*limit]
	}
	fmt.Fprintf(os.Stderr, "merge-segments: %d job(s) to process (dry_run=%v)\n", len(jobs), *dryRun)

	var processed, skipped, failed int
	for _, j := range jobs {
		if ctx.Err() != nil {
			fmt.Fprintf(os.Stderr, "merge-segments: aborted: %v\n", ctx.Err())
			break
		}
		if strings.TrimSpace(j.RawResultPath) == "" {
			fmt.Fprintf(os.Stderr, "merge-segments: event %s job %d: no raw_result_path on record; skipping\n", j.TVWEventID, j.ID)
			skipped++
			continue
		}
		if strings.ToLower(j.Provider) != "deepgram" {
			fmt.Fprintf(os.Stderr, "merge-segments: event %s job %d: provider %q not supported; skipping\n", j.TVWEventID, j.ID, j.Provider)
			skipped++
			continue
		}
		raw, err := os.ReadFile(j.RawResultPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "merge-segments: event %s job %d: read raw: %v\n", j.TVWEventID, j.ID, err)
			failed++
			continue
		}
		res, err := diarization.ParseDeepgramJSON(raw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "merge-segments: event %s job %d: parse raw: %v\n", j.TVWEventID, j.ID, err)
			failed++
			continue
		}
		if *dryRun {
			fmt.Fprintf(os.Stderr, "merge-segments: event %s job %d: would write %d merged segments\n", j.TVWEventID, j.ID, len(res.Segments))
			processed++
			continue
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
					"provider": j.Provider,
					"model":    j.Model,
					"merged":   true,
				},
			})
		}
		entities := make([]db.EntityMentionParams, 0, len(res.Entities))
		for _, ent := range res.Entities {
			entities = append(entities, db.EntityMentionParams{
				SourceKind:     "diarization_job",
				Extractor:      j.Provider,
				Model:          j.Model,
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
		if err := store.InsertDiarizationResult(ctx, db.InsertDiarizationResultParams{
			JobID:         j.ID,
			TVWEventID:    j.TVWEventID,
			Provider:      j.Provider,
			Model:         j.Model,
			RawResultPath: j.RawResultPath,
			Segments:      segments,
			Entities:      entities,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "merge-segments: event %s job %d: store: %v\n", j.TVWEventID, j.ID, err)
			failed++
			continue
		}
		fmt.Fprintf(os.Stderr, "merge-segments: event %s job %d: wrote %d merged segments\n", j.TVWEventID, j.ID, len(segments))
		processed++
	}
	fmt.Fprintf(os.Stderr, "merge-segments: done — %d processed, %d skipped, %d failed\n", processed, skipped, failed)
	if failed > 0 {
		return 1
	}
	return 0
}
