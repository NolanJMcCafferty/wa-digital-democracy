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
		Name:     "extract-speaker-evidence",
		Synopsis: "Generate reviewable speaker identity tasks from diarization text",
		Run:      runExtractSpeakerEvidence,
	})
}

func runExtractSpeakerEvidence(args []string) int {
	fs := flag.NewFlagSet("extract-speaker-evidence", flag.ContinueOnError)
	var (
		eventID = fs.String("event-id", "", "TVW/Invintus event ID")
		jobID   = fs.Int64("job-id", 0, "diarization_job id")
		dsn     = fs.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres DSN")
		limit   = fs.Int("limit", 0, "optional max diarized segments to scan")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *eventID == "" || *jobID == 0 {
		fmt.Fprintln(os.Stderr, "extract-speaker-evidence: --event-id and --job-id are required")
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	store, err := db.Open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "extract-speaker-evidence: db open: %v\n", err)
		return 1
	}
	defer store.Close()

	scanned, evidence, tasks, err := extractSpeakerEvidenceForJob(ctx, store, *jobID, *eventID, *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "extract-speaker-evidence: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "extract-speaker-evidence: scanned %d segments, upserted %d evidence rows and %d review tasks\n", scanned, evidence, tasks)
	return 0
}

// extractSpeakerEvidenceForJob runs the self-introduction extractor over every
// diarized segment for (jobID, eventID) and upserts evidence + review tasks.
// Returns (segmentsScanned, evidenceRowsUpserted, reviewTasksUpserted, err).
func extractSpeakerEvidenceForJob(ctx context.Context, store *db.Store, jobID int64, eventID string, limit int) (int, int, int, error) {
	segments, err := loadDiarizedSegmentsForEvidence(ctx, store, jobID, eventID, limit)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("load segments: %w", err)
	}
	createdEvidence, createdTasks := 0, 0
	for _, seg := range segments {
		cands := diarization.ExtractSpeakerEvidence(seg.Text)
		for _, cand := range cands {
			kind, candidateID, label, conf := resolveSpeakerCandidate(ctx, store, cand.Kind, cand.Label, eventID, cand.Confidence)
			evidenceKey := fmt.Sprintf("job:%d:seg:%d:%s:%d:%s", jobID, seg.ID, kind, candidateID, strings.ToLower(label))
			eid, err := store.UpsertSpeakerIdentityEvidence(ctx, db.SpeakerIdentityEvidenceParams{
				EvidenceKey:             evidenceKey,
				DiarizationJobID:        jobID,
				SpeakerClusterID:        seg.SpeakerClusterID,
				DiarizedSpeechSegmentID: seg.ID,
				EvidenceType:            cand.EvidenceType,
				EvidenceText:            cand.EvidenceText,
				CandidateKind:           kind,
				CandidateID:             candidateID,
				CandidateLabel:          label,
				Confidence:              conf,
				StartMS:                 seg.StartMS,
				EndMS:                   seg.EndMS,
				Raw: map[string]any{
					"extracted_label": cand.Label,
					"matched_pattern": cand.MatchedPattern,
				},
			})
			if err != nil {
				return len(segments), createdEvidence, createdTasks, fmt.Errorf("store evidence: %w", err)
			}
			createdEvidence++
			priority := 100
			if kind == "legislator" {
				priority += 50
			}
			if candidateID != 0 {
				priority += 25
			}
			if _, err := store.UpsertSpeakerReviewTask(ctx, db.SpeakerReviewTaskParams{
				DiarizationJobID: jobID,
				SpeakerClusterID: seg.SpeakerClusterID,
				Priority:         priority,
				CandidateKind:    kind,
				CandidateID:      candidateID,
				CandidateLabel:   label,
				Confidence:       conf,
				EvidenceIDs:      []int64{eid},
			}); err != nil {
				return len(segments), createdEvidence, createdTasks, fmt.Errorf("store review task: %w", err)
			}
			createdTasks++
		}
	}
	return len(segments), createdEvidence, createdTasks, nil
}

type diarizedSegmentForEvidence struct {
	ID               int64
	SpeakerClusterID int64
	ClusterLabel     string
	StartMS          int
	EndMS            int
	Text             string
}

func loadDiarizedSegmentsForEvidence(ctx context.Context, store *db.Store, jobID int64, eventID string, limit int) ([]diarizedSegmentForEvidence, error) {
	lim := ""
	args := []any{jobID, eventID}
	if limit > 0 {
		lim = " LIMIT $3"
		args = append(args, limit)
	}
	q := `
SELECT ds.id, ds.speaker_cluster_id, ds.cluster_label, ds.start_ms, ds.end_ms, COALESCE(ds.text,'')
  FROM diarized_speech_segment ds
 WHERE ds.diarization_job_id = $1 AND ds.tvw_event_id = $2 AND COALESCE(ds.text,'') <> ''
 ORDER BY ds.start_ms` + lim
	rows, err := store.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []diarizedSegmentForEvidence{}
	for rows.Next() {
		var s diarizedSegmentForEvidence
		if err := rows.Scan(&s.ID, &s.SpeakerClusterID, &s.ClusterLabel, &s.StartMS, &s.EndMS, &s.Text); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func resolveSpeakerCandidate(ctx context.Context, store *db.Store, kind, label, eventID string, baseConfidence float64) (string, int64, string, float64) {
	if kind == "legislator" {
		if id, display, ok := findLegislatorBySpokenName(ctx, store, label); ok {
			return "legislator", id, display, maxFloat(baseConfidence, 0.95)
		}
		return "person", 0, label, minFloat(baseConfidence, 0.70)
	}
	if id, display, ok := findTestifierBySpokenName(ctx, store, label, eventID); ok {
		return "testifier", id, display, maxFloat(baseConfidence, 0.85)
	}
	return "person", 0, label, baseConfidence
}

func findLegislatorBySpokenName(ctx context.Context, store *db.Store, label string) (int64, string, bool) {
	const q = `
SELECT id, COALESCE(NULLIF(trim(first_name || ' ' || last_name), ''), name)
  FROM legislator
 WHERE lower(COALESCE(NULLIF(trim(first_name || ' ' || last_name), ''), name)) = lower($1)
    OR lower(name) LIKE '%' || lower($1) || '%'
 ORDER BY CASE WHEN lower(COALESCE(NULLIF(trim(first_name || ' ' || last_name), ''), name)) = lower($1) THEN 0 ELSE 1 END
 LIMIT 1;`
	var id int64
	var display string
	if err := store.Pool.QueryRow(ctx, q, label).Scan(&id, &display); err != nil {
		return 0, "", false
	}
	return id, display, true
}

func findTestifierBySpokenName(ctx context.Context, store *db.Store, label, eventID string) (int64, string, bool) {
	const q = `
SELECT t.id, t.raw_name
  FROM testifier t
  JOIN agenda_item a ON a.id = t.agenda_item_id
  JOIN hearing h ON h.id = a.hearing_id
 WHERE h.tvw_event_id = $1 AND lower(t.raw_name) = lower($2)
 LIMIT 1;`
	var id int64
	var display string
	if err := store.Pool.QueryRow(ctx, q, eventID, label).Scan(&id, &display); err != nil {
		return 0, "", false
	}
	return id, display, true
}
