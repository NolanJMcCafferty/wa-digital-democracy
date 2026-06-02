package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func init() {
	register(Command{
		Name:     "backfill-speaker-evidence",
		Synopsis: "Run extract-speaker-evidence over every succeeded diarization_job",
		Run:      runBackfillSpeakerEvidence,
	})
}

func runBackfillSpeakerEvidence(args []string) int {
	fs := flag.NewFlagSet("backfill-speaker-evidence", flag.ContinueOnError)
	var (
		dsn    = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		limit  = fs.Int("limit", 0, "max diarization jobs to process (0 = all)")
		dryRun = fs.Bool("dry-run", false, "print the jobs that would be processed without writing evidence")
		skip   = fs.Bool("skip-existing", true, "skip jobs that already have speaker_identity_evidence rows")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "backfill-speaker-evidence: db open: %v\n", err)
		return 1
	}
	defer store.Close()

	jobs, err := loadSucceededDiarizationJobs(ctx, store, *limit, *skip)
	if err != nil {
		fmt.Fprintf(os.Stderr, "backfill-speaker-evidence: list jobs: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "backfill-speaker-evidence: %d jobs to process\n", len(jobs))
	if *dryRun {
		for _, j := range jobs {
			fmt.Printf("%d\t%s\n", j.ID, j.EventID)
		}
		return 0
	}

	start := time.Now()
	totalScanned, totalEvidence, totalTasks, failed := 0, 0, 0, 0
	for i, j := range jobs {
		select {
		case <-ctx.Done():
			fmt.Fprintf(os.Stderr, "backfill-speaker-evidence: interrupted after %d/%d jobs\n", i, len(jobs))
			return 1
		default:
		}
		scanned, ev, tk, err := extractSpeakerEvidenceForJob(ctx, store, j.ID, j.EventID, 0)
		if err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "backfill-speaker-evidence: job %d (event %s) failed: %v\n", j.ID, j.EventID, err)
			continue
		}
		totalScanned += scanned
		totalEvidence += ev
		totalTasks += tk
		if (i+1)%25 == 0 || ev > 0 {
			fmt.Fprintf(os.Stderr, "backfill-speaker-evidence: [%d/%d] job=%d event=%s segments=%d evidence=%d tasks=%d\n",
				i+1, len(jobs), j.ID, j.EventID, scanned, ev, tk)
		}
	}
	fmt.Fprintf(os.Stderr, "backfill-speaker-evidence: done in %s — jobs=%d failed=%d segments=%d evidence=%d tasks=%d\n",
		time.Since(start).Round(time.Second), len(jobs), failed, totalScanned, totalEvidence, totalTasks)
	if failed > 0 {
		return 1
	}
	return 0
}

type diarizationJobRef struct {
	ID      int64
	EventID string
}

func loadSucceededDiarizationJobs(ctx context.Context, store *db.Store, limit int, skipExisting bool) ([]diarizationJobRef, error) {
	// One job per (event_id) — pick the most recent succeeded job, matching
	// the convention used elsewhere when joining diarized_speech_segment.
	q := `
WITH latest AS (
  SELECT DISTINCT ON (tvw_event_id) id, tvw_event_id, finished_at
    FROM diarization_job
   WHERE status = 'succeeded'
   ORDER BY tvw_event_id, finished_at DESC NULLS LAST, id DESC
)
SELECT l.id, l.tvw_event_id
  FROM latest l
 WHERE ($1::bool = false)
    OR NOT EXISTS (
         SELECT 1 FROM speaker_identity_evidence e WHERE e.diarization_job_id = l.id
       )
 ORDER BY l.finished_at DESC NULLS LAST, l.id DESC`
	args := []any{skipExisting}
	if limit > 0 {
		q += " LIMIT $2"
		args = append(args, limit)
	}
	rows, err := store.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []diarizationJobRef{}
	for rows.Next() {
		var j diarizationJobRef
		if err := rows.Scan(&j.ID, &j.EventID); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
