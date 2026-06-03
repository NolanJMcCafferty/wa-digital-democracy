package db

import (
	"context"
	"fmt"
)

// ListEventsPendingDiarization returns tvw_event_ids that have at least one
// resolvable audio source URL and no successful diarization_job yet. Used by
// `wa-dd diarize-pending` to skip already-processed events idempotently.
func (s *Store) ListEventsPendingDiarization(ctx context.Context, limit int) ([]string, error) {
	const q = `
SELECT e.tvw_event_id
  FROM tvw_event e
 WHERE (
        NULLIF(e.audio_download_url, '') IS NOT NULL
     OR NULLIF(e.published_audio_url, '') IS NOT NULL
     OR NULLIF(e.video_download_url, '') IS NOT NULL
     OR EXISTS (
          SELECT 1 FROM tvw_media_asset m
           WHERE m.tvw_event_id = e.tvw_event_id
             AND (m.asset_type ILIKE 'audio' OR m.asset_type ILIKE 'video')
             AND NULLIF(m.file_url, '') IS NOT NULL
       )
   )
   AND NOT EXISTS (
        SELECT 1 FROM diarization_job j
         WHERE j.tvw_event_id = e.tvw_event_id
           AND j.status = 'succeeded'
   )
 ORDER BY e.start_datetime DESC NULLS LAST, e.tvw_event_id
 LIMIT NULLIF($1, 0);`
	rows, err := s.Pool.Query(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("list events pending diarization: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan event id: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) HasSucceededDiarization(ctx context.Context, tvwEventID string) (bool, error) {
	const q = `
SELECT EXISTS (
  SELECT 1
    FROM diarization_job
   WHERE tvw_event_id = $1
     AND status = 'succeeded'
);`
	var ok bool
	if err := s.Pool.QueryRow(ctx, q, tvwEventID).Scan(&ok); err != nil {
		return false, fmt.Errorf("has succeeded diarization: %w", err)
	}
	return ok, nil
}

type CreateDiarizationJobParams struct {
	TVWEventID   string
	AudioAssetID int64
	Provider     string
	Model        string
}

func (s *Store) CreateDiarizationJob(ctx context.Context, p CreateDiarizationJobParams) (int64, error) {
	const q = `
INSERT INTO diarization_job (tvw_event_id, audio_asset_id, provider, model, status, submitted_at)
VALUES ($1, $2, $3, NULLIF($4,''), 'running', NOW())
RETURNING id;`
	var id int64
	if err := s.Pool.QueryRow(ctx, q, p.TVWEventID, p.AudioAssetID, p.Provider, p.Model).Scan(&id); err != nil {
		return 0, fmt.Errorf("create diarization_job: %w", err)
	}
	return id, nil
}

func (s *Store) FailDiarizationJob(ctx context.Context, jobID int64, msg string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE diarization_job SET status = 'failed', finished_at = NOW(), error = $2 WHERE id = $1`, jobID, msg)
	if err != nil {
		return fmt.Errorf("fail diarization_job: %w", err)
	}
	return nil
}

type InsertDiarizationResultParams struct {
	JobID         int64
	TVWEventID    string
	Provider      string
	Model         string
	RawResultPath string
	Segments      []DiarizedSegmentParams
	Entities      []EntityMentionParams
}

type DiarizedSegmentParams struct {
	ClusterLabel string
	StartMS      int
	EndMS        int
	Confidence   *float64
	Text         string
	Raw          map[string]any
}

type EntityMentionParams struct {
	SourceKind     string
	SourceID       int64
	Extractor      string
	Model          string
	EntityType     string
	Text           string
	NormalizedText string
	StartMS        *int
	EndMS          *int
	StartWord      *int
	EndWord        *int
	Confidence     *float64
	Raw            map[string]any
}

func (s *Store) InsertDiarizationResult(ctx context.Context, p InsertDiarizationResultParams) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM entity_mention WHERE diarization_job_id = $1`, p.JobID); err != nil {
		return fmt.Errorf("delete entity mentions: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM diarized_speech_segment WHERE diarization_job_id = $1`, p.JobID); err != nil {
		return fmt.Errorf("delete diarized segments: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM speaker_cluster WHERE diarization_job_id = $1`, p.JobID); err != nil {
		return fmt.Errorf("delete speaker clusters: %w", err)
	}

	type agg struct{ total, count int }
	aggs := map[string]agg{}
	for _, seg := range p.Segments {
		a := aggs[seg.ClusterLabel]
		a.total += seg.EndMS - seg.StartMS
		a.count++
		aggs[seg.ClusterLabel] = a
	}

	clusterIDs := map[string]int64{}
	for label, a := range aggs {
		var id int64
		if err := tx.QueryRow(ctx, `
INSERT INTO speaker_cluster (diarization_job_id, tvw_event_id, cluster_label, total_speech_ms, turn_count)
VALUES ($1,$2,$3,$4,$5)
RETURNING id;`, p.JobID, p.TVWEventID, label, a.total, a.count).Scan(&id); err != nil {
			return fmt.Errorf("insert speaker_cluster: %w", err)
		}
		clusterIDs[label] = id
	}

	const segQ = `
INSERT INTO diarized_speech_segment (diarization_job_id, speaker_cluster_id, tvw_event_id,
                                     cluster_label, start_ms, end_ms, confidence, text, raw)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9);`
	for _, seg := range p.Segments {
		raw, err := marshalJSONDefault(seg.Raw, map[string]any{})
		if err != nil {
			return fmt.Errorf("marshal diarized segment raw: %w", err)
		}
		if _, err := tx.Exec(ctx, segQ, p.JobID, clusterIDs[seg.ClusterLabel], p.TVWEventID,
			seg.ClusterLabel, seg.StartMS, seg.EndMS, floatPtrOrNull(seg.Confidence), strOrNull(seg.Text), string(raw)); err != nil {
			return fmt.Errorf("insert diarized segment: %w", err)
		}
	}

	const entQ = `
INSERT INTO entity_mention (tvw_event_id, diarization_job_id, source_kind, source_id,
                            extractor, model, entity_type, text, normalized_text,
                            start_ms, end_ms, start_word, end_word, confidence, raw)
VALUES ($1,$2,$3,NULLIF($4,0),$5,NULLIF($6,''),$7,$8,NULLIF($9,''),$10,$11,$12,$13,$14,$15);`
	for _, ent := range p.Entities {
		raw, err := marshalJSONDefault(ent.Raw, map[string]any{})
		if err != nil {
			return fmt.Errorf("marshal entity mention raw: %w", err)
		}
		sourceKind := defaultStr(ent.SourceKind, "diarization_job")
		extractor := defaultStr(ent.Extractor, p.Provider)
		model := ent.Model
		if model == "" {
			model = p.Model
		}
		if _, err := tx.Exec(ctx, entQ, p.TVWEventID, p.JobID, sourceKind, ent.SourceID,
			extractor, model, ent.EntityType, ent.Text, ent.NormalizedText,
			intPtrOrNull(ent.StartMS), intPtrOrNull(ent.EndMS), intPtrOrNull(ent.StartWord),
			intPtrOrNull(ent.EndWord), floatPtrOrNull(ent.Confidence), string(raw)); err != nil {
			return fmt.Errorf("insert entity mention: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `UPDATE diarization_job SET status = 'succeeded', finished_at = NOW(), raw_result_path = NULLIF($2,'') WHERE id = $1`, p.JobID, p.RawResultPath); err != nil {
		return fmt.Errorf("update diarization_job: %w", err)
	}
	return tx.Commit(ctx)
}

// DiarizedHearingSegment is one row from the most recent succeeded
// diarization job for a hearing's TVW event. Covers the entire hearing,
// not just one bill discussion (per-bill slicing happens via agenda_item_window).
type DiarizedHearingSegment struct {
	StartMS      int
	EndMS        int
	Text         string
	ClusterLabel string
	SpeakerLabel string
	SpeakerKind  string
	ReviewStatus string
	Reviewed     bool
}

func (s *Store) ListDiarizedSegmentsByTVWEvent(ctx context.Context, tvwEventID string) ([]DiarizedHearingSegment, error) {
	const q = `
SELECT d.start_ms, d.end_ms, COALESCE(d.text,''), d.cluster_label,
       COALESCE(sa.speaker_label, ''), COALESCE(sa.speaker_kind::text, ''),
       COALESCE(sa.review_status::text, ''), (sa.id IS NOT NULL) AS reviewed
  FROM diarized_speech_segment d
  JOIN diarization_job j ON j.id = d.diarization_job_id
  LEFT JOIN speaker_assignment sa
    ON sa.diarization_job_id = d.diarization_job_id
   AND sa.speaker_cluster_id = d.speaker_cluster_id
   AND sa.review_status = 'accepted'
 WHERE d.tvw_event_id = $1
   AND j.status = 'succeeded'
   AND j.id = (
     SELECT id FROM diarization_job
      WHERE tvw_event_id = $1 AND status = 'succeeded'
      ORDER BY finished_at DESC NULLS LAST, id DESC LIMIT 1
   )
 ORDER BY d.start_ms ASC;`
	rows, err := s.Pool.Query(ctx, q, tvwEventID)
	if err != nil {
		return nil, fmt.Errorf("list diarized segments: %w", err)
	}
	defer rows.Close()
	out := []DiarizedHearingSegment{}
	for rows.Next() {
		var seg DiarizedHearingSegment
		if err := rows.Scan(&seg.StartMS, &seg.EndMS, &seg.Text, &seg.ClusterLabel,
			&seg.SpeakerLabel, &seg.SpeakerKind, &seg.ReviewStatus, &seg.Reviewed); err != nil {
			return nil, fmt.Errorf("scan diarized segment: %w", err)
		}
		out = append(out, seg)
	}
	return out, rows.Err()
}
