package db

import (
	"context"
	"fmt"
	"strings"
)

type SpeakerIdentityEvidenceParams struct {
	EvidenceKey             string
	DiarizationJobID        int64
	SpeakerClusterID        int64
	DiarizedSpeechSegmentID int64
	EvidenceType            string
	EvidenceText            string
	CandidateKind           string
	CandidateID             int64
	CandidateLabel          string
	Confidence              float64
	StartMS                 int
	EndMS                   int
	Raw                     map[string]any
}

func (s *Store) UpsertSpeakerIdentityEvidence(ctx context.Context, p SpeakerIdentityEvidenceParams) (int64, error) {
	raw, err := marshalJSONDefault(p.Raw, map[string]any{})
	if err != nil {
		return 0, fmt.Errorf("marshal speaker evidence raw: %w", err)
	}
	const q = `
INSERT INTO speaker_identity_evidence (evidence_key, diarization_job_id, speaker_cluster_id,
                                       diarized_speech_segment_id, evidence_type, evidence_text,
                                       candidate_kind, candidate_id, candidate_label, confidence,
                                       start_ms, end_ms, raw)
VALUES ($1,$2,$3,NULLIF($4,0),$5::speaker_evidence_type,$6,$7::speaker_candidate_kind,
        NULLIF($8,0),$9,NULLIF($10::numeric,0),NULLIF($11,0),NULLIF($12,0),$13::jsonb)
ON CONFLICT (evidence_key) DO UPDATE SET
  evidence_text = EXCLUDED.evidence_text,
  confidence = EXCLUDED.confidence,
  raw = EXCLUDED.raw
RETURNING id;`
	var id int64
	if err := s.Pool.QueryRow(ctx, q, p.EvidenceKey, p.DiarizationJobID, p.SpeakerClusterID,
		p.DiarizedSpeechSegmentID, p.EvidenceType, p.EvidenceText, p.CandidateKind,
		p.CandidateID, p.CandidateLabel, p.Confidence, p.StartMS, p.EndMS, string(raw)).Scan(&id); err != nil {
		return 0, fmt.Errorf("upsert speaker_identity_evidence: %w", err)
	}
	return id, nil
}

type SpeakerReviewTaskParams struct {
	DiarizationJobID int64
	SpeakerClusterID int64
	Priority         int
	CandidateKind    string
	CandidateID      int64
	CandidateLabel   string
	Confidence       float64
	EvidenceIDs      []int64
}

func (s *Store) UpsertSpeakerReviewTask(ctx context.Context, p SpeakerReviewTaskParams) (int64, error) {
	const q = `
INSERT INTO speaker_review_task (diarization_job_id, speaker_cluster_id, priority,
                                 proposed_candidate_kind, proposed_candidate_id,
                                 proposed_label, proposed_confidence, evidence_ids)
VALUES ($1,$2,$3,$4::speaker_candidate_kind,NULLIF($5,0),$6,NULLIF($7::numeric,0),$8)
ON CONFLICT (diarization_job_id, speaker_cluster_id, proposed_candidate_kind, COALESCE(proposed_candidate_id, 0), proposed_label)
DO UPDATE SET
  priority = GREATEST(speaker_review_task.priority, EXCLUDED.priority),
  proposed_confidence = GREATEST(COALESCE(speaker_review_task.proposed_confidence,0), COALESCE(EXCLUDED.proposed_confidence,0)),
  evidence_ids = EXCLUDED.evidence_ids,
  updated_at = NOW()
RETURNING id;`
	var id int64
	if err := s.Pool.QueryRow(ctx, q, p.DiarizationJobID, p.SpeakerClusterID, p.Priority,
		p.CandidateKind, p.CandidateID, p.CandidateLabel, p.Confidence, p.EvidenceIDs).Scan(&id); err != nil {
		return 0, fmt.Errorf("upsert speaker_review_task: %w", err)
	}
	return id, nil
}

type SpeakerReviewTask struct {
	ID                  int64
	DiarizationJobID    int64
	TVWEventID          string
	ClusterID           int64
	ClusterLabel        string
	TotalSpeechMS       int
	TurnCount           int
	Status              string
	Priority            int
	CandidateKind       string
	CandidateID         int64
	CandidateLabel      string
	CandidateConfidence float64
	EvidenceIDs         []int64
	EvidenceText        string
	EvidenceStartMS     int
	EvidenceEndMS       int
	CurrentSpeakerLabel string
	CurrentReviewStatus string
	SampleSegments      []SpeakerReviewSegment
}

type SpeakerReviewSegment struct {
	StartMS int
	EndMS   int
	Text    string
}

func (s *Store) ListSpeakerReviewTasks(ctx context.Context, status string, limit int) ([]SpeakerReviewTask, error) {
	if status == "" {
		status = "pending"
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	const q = `
SELECT t.id, t.diarization_job_id, sc.tvw_event_id, sc.id, sc.cluster_label,
       COALESCE(sc.total_speech_ms,0), COALESCE(sc.turn_count,0), t.status::text,
       t.priority, t.proposed_candidate_kind::text, COALESCE(t.proposed_candidate_id,0),
       t.proposed_label, COALESCE(t.proposed_confidence,0), t.evidence_ids,
       COALESCE(e.evidence_text,''), COALESCE(e.start_ms,0), COALESCE(e.end_ms,0),
       COALESCE(sa.speaker_label, ''), COALESCE(sa.review_status::text, '')
  FROM speaker_review_task t
  JOIN speaker_cluster sc ON sc.id = t.speaker_cluster_id
  LEFT JOIN speaker_assignment sa
    ON sa.diarization_job_id = t.diarization_job_id
   AND sa.speaker_cluster_id = t.speaker_cluster_id
  LEFT JOIN LATERAL (
    SELECT evidence_text, start_ms, end_ms
      FROM speaker_identity_evidence e
     WHERE e.id = ANY(t.evidence_ids)
     ORDER BY confidence DESC NULLS LAST, id
     LIMIT 1
  ) e ON true
 WHERE t.status = $1::speaker_review_status
 ORDER BY t.priority DESC, t.created_at DESC
 LIMIT $2;`
	rows, err := s.Pool.Query(ctx, q, status, limit)
	if err != nil {
		return nil, fmt.Errorf("list speaker review tasks: %w", err)
	}
	defer rows.Close()
	out := []SpeakerReviewTask{}
	for rows.Next() {
		var t SpeakerReviewTask
		if err := rows.Scan(&t.ID, &t.DiarizationJobID, &t.TVWEventID, &t.ClusterID, &t.ClusterLabel,
			&t.TotalSpeechMS, &t.TurnCount, &t.Status, &t.Priority, &t.CandidateKind,
			&t.CandidateID, &t.CandidateLabel, &t.CandidateConfidence, &t.EvidenceIDs,
			&t.EvidenceText, &t.EvidenceStartMS, &t.EvidenceEndMS,
			&t.CurrentSpeakerLabel, &t.CurrentReviewStatus); err != nil {
			return nil, fmt.Errorf("scan speaker review task: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) GetSpeakerReviewTask(ctx context.Context, id int64) (SpeakerReviewTask, error) {
	const q = `
SELECT t.id, t.diarization_job_id, sc.tvw_event_id, sc.id, sc.cluster_label,
       COALESCE(sc.total_speech_ms,0), COALESCE(sc.turn_count,0), t.status::text,
       t.priority, t.proposed_candidate_kind::text, COALESCE(t.proposed_candidate_id,0),
       t.proposed_label, COALESCE(t.proposed_confidence,0), t.evidence_ids,
       COALESCE(e.evidence_text,''), COALESCE(e.start_ms,0), COALESCE(e.end_ms,0),
       COALESCE(sa.speaker_label, ''), COALESCE(sa.review_status::text, '')
  FROM speaker_review_task t
  JOIN speaker_cluster sc ON sc.id = t.speaker_cluster_id
  LEFT JOIN speaker_assignment sa
    ON sa.diarization_job_id = t.diarization_job_id
   AND sa.speaker_cluster_id = t.speaker_cluster_id
  LEFT JOIN LATERAL (
    SELECT evidence_text, start_ms, end_ms
      FROM speaker_identity_evidence e
     WHERE e.id = ANY(t.evidence_ids)
     ORDER BY confidence DESC NULLS LAST, id
     LIMIT 1
  ) e ON true
 WHERE t.id = $1;`
	var t SpeakerReviewTask
	if err := s.Pool.QueryRow(ctx, q, id).Scan(&t.ID, &t.DiarizationJobID, &t.TVWEventID, &t.ClusterID, &t.ClusterLabel,
		&t.TotalSpeechMS, &t.TurnCount, &t.Status, &t.Priority, &t.CandidateKind,
		&t.CandidateID, &t.CandidateLabel, &t.CandidateConfidence, &t.EvidenceIDs,
		&t.EvidenceText, &t.EvidenceStartMS, &t.EvidenceEndMS,
		&t.CurrentSpeakerLabel, &t.CurrentReviewStatus); err != nil {
		return SpeakerReviewTask{}, fmt.Errorf("get speaker review task: %w", err)
	}
	segs, err := s.ListSpeakerReviewTaskSegments(ctx, t.ClusterID, 20)
	if err != nil {
		return SpeakerReviewTask{}, err
	}
	t.SampleSegments = segs
	return t, nil
}

func (s *Store) ListSpeakerReviewTaskSegments(ctx context.Context, clusterID int64, limit int) ([]SpeakerReviewSegment, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	const q = `
SELECT start_ms, end_ms, COALESCE(text,'')
  FROM diarized_speech_segment
 WHERE speaker_cluster_id = $1
 ORDER BY start_ms
 LIMIT $2;`
	rows, err := s.Pool.Query(ctx, q, clusterID, limit)
	if err != nil {
		return nil, fmt.Errorf("list speaker review segments: %w", err)
	}
	defer rows.Close()
	out := []SpeakerReviewSegment{}
	for rows.Next() {
		var seg SpeakerReviewSegment
		if err := rows.Scan(&seg.StartMS, &seg.EndMS, &seg.Text); err != nil {
			return nil, fmt.Errorf("scan speaker review segment: %w", err)
		}
		out = append(out, seg)
	}
	return out, rows.Err()
}

type SpeakerReviewEvent struct {
	TVWEventID             string
	ClusterCount           int
	AssignedCount          int
	PendingTaskCount       int
	UnresolvedClusterCount int
	TotalSpeechMS          int
}

func (s *Store) ListSpeakerReviewEvents(ctx context.Context, limit, offset int) ([]SpeakerReviewEvent, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	const countQ = `
SELECT COUNT(*)
  FROM (
    SELECT sc.tvw_event_id
      FROM speaker_cluster sc
      JOIN diarization_job j ON j.id = sc.diarization_job_id
     WHERE j.status = 'succeeded'
       AND j.id = (
         SELECT id FROM diarization_job
          WHERE tvw_event_id = sc.tvw_event_id AND status = 'succeeded'
          ORDER BY finished_at DESC NULLS LAST, id DESC LIMIT 1
       )
     GROUP BY sc.tvw_event_id
  ) events;`
	var total int
	if err := s.Pool.QueryRow(ctx, countQ).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count speaker review events: %w", err)
	}
	const q = `
SELECT sc.tvw_event_id,
       COUNT(DISTINCT sc.id) AS cluster_count,
       COUNT(DISTINCT sa.speaker_cluster_id) FILTER (WHERE sa.review_status = 'accepted') AS assigned_count,
       COUNT(DISTINCT t.id) FILTER (WHERE t.status = 'pending') AS pending_task_count,
       COUNT(DISTINCT sc.id) FILTER (WHERE sa.id IS NULL) AS unresolved_cluster_count,
       COALESCE(SUM(sc.total_speech_ms), 0) AS total_speech_ms
  FROM speaker_cluster sc
  JOIN diarization_job j ON j.id = sc.diarization_job_id
  LEFT JOIN speaker_assignment sa ON sa.diarization_job_id = sc.diarization_job_id
                                  AND sa.speaker_cluster_id = sc.id
                                  AND sa.review_status = 'accepted'
  LEFT JOIN speaker_review_task t ON t.diarization_job_id = sc.diarization_job_id
                                  AND t.speaker_cluster_id = sc.id
 WHERE j.status = 'succeeded'
   AND j.id = (
     SELECT id FROM diarization_job
      WHERE tvw_event_id = sc.tvw_event_id AND status = 'succeeded'
      ORDER BY finished_at DESC NULLS LAST, id DESC LIMIT 1
   )
 GROUP BY sc.tvw_event_id
 ORDER BY pending_task_count DESC, unresolved_cluster_count DESC, total_speech_ms DESC
 LIMIT $1 OFFSET $2;`
	rows, err := s.Pool.Query(ctx, q, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list speaker review events: %w", err)
	}
	defer rows.Close()
	out := []SpeakerReviewEvent{}
	for rows.Next() {
		var ev SpeakerReviewEvent
		if err := rows.Scan(&ev.TVWEventID, &ev.ClusterCount, &ev.AssignedCount, &ev.PendingTaskCount, &ev.UnresolvedClusterCount, &ev.TotalSpeechMS); err != nil {
			return nil, 0, fmt.Errorf("scan speaker review event: %w", err)
		}
		out = append(out, ev)
	}
	return out, total, rows.Err()
}

type SpeakerClusterReview struct {
	ClusterID           int64
	DiarizationJobID    int64
	TVWEventID          string
	ClusterLabel        string
	TotalSpeechMS       int
	TurnCount           int
	CurrentSpeakerLabel string
	CurrentSpeakerKind  string
	CurrentReviewStatus string
	Tasks               []SpeakerReviewTask
	Evidence            []SpeakerIdentityEvidence
	SampleSegments      []SpeakerReviewSegment
}

type SpeakerIdentityEvidence struct {
	ID             int64
	EvidenceType   string
	EvidenceText   string
	CandidateKind  string
	CandidateID    int64
	CandidateLabel string
	Confidence     float64
	StartMS        int
	EndMS          int
}

func (s *Store) ListSpeakerClustersForEvent(ctx context.Context, tvwEventID string) ([]SpeakerClusterReview, error) {
	const q = `
SELECT sc.id, sc.diarization_job_id, sc.tvw_event_id, sc.cluster_label,
       COALESCE(sc.total_speech_ms,0), COALESCE(sc.turn_count,0),
       COALESCE(sa.speaker_label,''), COALESCE(sa.speaker_kind::text,''), COALESCE(sa.review_status::text,'')
  FROM speaker_cluster sc
  JOIN diarization_job j ON j.id = sc.diarization_job_id
  LEFT JOIN speaker_assignment sa ON sa.diarization_job_id = sc.diarization_job_id
                                  AND sa.speaker_cluster_id = sc.id
                                  AND sa.review_status = 'accepted'
 WHERE sc.tvw_event_id = $1
   AND j.status = 'succeeded'
   AND j.id = (
     SELECT id FROM diarization_job
      WHERE tvw_event_id = $1 AND status = 'succeeded'
      ORDER BY finished_at DESC NULLS LAST, id DESC LIMIT 1
   )
 ORDER BY COALESCE(sc.total_speech_ms,0) DESC, sc.cluster_label;`
	rows, err := s.Pool.Query(ctx, q, tvwEventID)
	if err != nil {
		return nil, fmt.Errorf("list speaker clusters for event: %w", err)
	}
	defer rows.Close()
	out := []SpeakerClusterReview{}
	idx := map[int64]int{}
	for rows.Next() {
		var c SpeakerClusterReview
		if err := rows.Scan(&c.ClusterID, &c.DiarizationJobID, &c.TVWEventID, &c.ClusterLabel,
			&c.TotalSpeechMS, &c.TurnCount, &c.CurrentSpeakerLabel, &c.CurrentSpeakerKind, &c.CurrentReviewStatus); err != nil {
			return nil, fmt.Errorf("scan speaker cluster review: %w", err)
		}
		idx[c.ClusterID] = len(out)
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}
	if err := s.attachSpeakerReviewTasksForEvent(ctx, tvwEventID, out, idx); err != nil {
		return nil, err
	}
	return out, nil
}

// attachSpeakerReviewTasksForEvent loads every speaker_review_task for the
// clusters in `out` (one round trip) and groups them onto each cluster.
// Without this the event view shows "No candidate" even when the extractor
// has populated review tasks.
func (s *Store) attachSpeakerReviewTasksForEvent(ctx context.Context, tvwEventID string, out []SpeakerClusterReview, idx map[int64]int) error {
	const q = `
SELECT t.id, t.diarization_job_id, sc.tvw_event_id, sc.id, sc.cluster_label,
       COALESCE(sc.total_speech_ms,0), COALESCE(sc.turn_count,0), t.status::text,
       t.priority, t.proposed_candidate_kind::text, COALESCE(t.proposed_candidate_id,0),
       t.proposed_label, COALESCE(t.proposed_confidence,0), t.evidence_ids,
       COALESCE(e.evidence_text,''), COALESCE(e.start_ms,0), COALESCE(e.end_ms,0),
       COALESCE(sa.speaker_label, ''), COALESCE(sa.review_status::text, '')
  FROM speaker_review_task t
  JOIN speaker_cluster sc ON sc.id = t.speaker_cluster_id
  JOIN diarization_job j ON j.id = sc.diarization_job_id
  LEFT JOIN speaker_assignment sa
    ON sa.diarization_job_id = t.diarization_job_id
   AND sa.speaker_cluster_id = t.speaker_cluster_id
  LEFT JOIN LATERAL (
    SELECT evidence_text, start_ms, end_ms
      FROM speaker_identity_evidence e
     WHERE e.id = ANY(t.evidence_ids)
     ORDER BY confidence DESC NULLS LAST, id
     LIMIT 1
  ) e ON true
 WHERE sc.tvw_event_id = $1
   AND j.status = 'succeeded'
   AND j.id = (
     SELECT id FROM diarization_job
      WHERE tvw_event_id = $1 AND status = 'succeeded'
      ORDER BY finished_at DESC NULLS LAST, id DESC LIMIT 1
   )
 ORDER BY CASE t.status WHEN 'pending' THEN 0 WHEN 'accepted' THEN 1 ELSE 2 END,
          t.priority DESC, t.proposed_confidence DESC NULLS LAST, t.created_at DESC;`
	rows, err := s.Pool.Query(ctx, q, tvwEventID)
	if err != nil {
		return fmt.Errorf("list speaker review tasks for event: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var t SpeakerReviewTask
		if err := rows.Scan(&t.ID, &t.DiarizationJobID, &t.TVWEventID, &t.ClusterID, &t.ClusterLabel,
			&t.TotalSpeechMS, &t.TurnCount, &t.Status, &t.Priority, &t.CandidateKind,
			&t.CandidateID, &t.CandidateLabel, &t.CandidateConfidence, &t.EvidenceIDs,
			&t.EvidenceText, &t.EvidenceStartMS, &t.EvidenceEndMS,
			&t.CurrentSpeakerLabel, &t.CurrentReviewStatus); err != nil {
			return fmt.Errorf("scan speaker review task for event: %w", err)
		}
		i, ok := idx[t.ClusterID]
		if !ok {
			continue
		}
		out[i].Tasks = append(out[i].Tasks, t)
	}
	return rows.Err()
}

func (s *Store) GetSpeakerClusterReview(ctx context.Context, clusterID int64) (SpeakerClusterReview, error) {
	const q = `
SELECT sc.id, sc.diarization_job_id, sc.tvw_event_id, sc.cluster_label,
       COALESCE(sc.total_speech_ms,0), COALESCE(sc.turn_count,0),
       COALESCE(sa.speaker_label,''), COALESCE(sa.speaker_kind::text,''), COALESCE(sa.review_status::text,'')
  FROM speaker_cluster sc
  JOIN diarization_job j ON j.id = sc.diarization_job_id
  LEFT JOIN speaker_assignment sa ON sa.diarization_job_id = sc.diarization_job_id
                                  AND sa.speaker_cluster_id = sc.id
                                  AND sa.review_status = 'accepted'
 WHERE sc.id = $1;`
	var c SpeakerClusterReview
	if err := s.Pool.QueryRow(ctx, q, clusterID).Scan(&c.ClusterID, &c.DiarizationJobID, &c.TVWEventID, &c.ClusterLabel,
		&c.TotalSpeechMS, &c.TurnCount, &c.CurrentSpeakerLabel, &c.CurrentSpeakerKind, &c.CurrentReviewStatus); err != nil {
		return SpeakerClusterReview{}, fmt.Errorf("get speaker cluster review: %w", err)
	}
	tasks, err := s.listSpeakerReviewTasksForCluster(ctx, clusterID)
	if err != nil {
		return SpeakerClusterReview{}, err
	}
	c.Tasks = tasks
	evidence, err := s.listSpeakerIdentityEvidenceForCluster(ctx, clusterID)
	if err != nil {
		return SpeakerClusterReview{}, err
	}
	c.Evidence = evidence
	segs, err := s.ListSpeakerReviewTaskSegments(ctx, clusterID, 30)
	if err != nil {
		return SpeakerClusterReview{}, err
	}
	c.SampleSegments = segs
	return c, nil
}

func (s *Store) listSpeakerReviewTasksForCluster(ctx context.Context, clusterID int64) ([]SpeakerReviewTask, error) {
	const q = `
SELECT t.id, t.diarization_job_id, sc.tvw_event_id, sc.id, sc.cluster_label,
       COALESCE(sc.total_speech_ms,0), COALESCE(sc.turn_count,0), t.status::text,
       t.priority, t.proposed_candidate_kind::text, COALESCE(t.proposed_candidate_id,0),
       t.proposed_label, COALESCE(t.proposed_confidence,0), t.evidence_ids,
       COALESCE(e.evidence_text,''), COALESCE(e.start_ms,0), COALESCE(e.end_ms,0),
       COALESCE(sa.speaker_label, ''), COALESCE(sa.review_status::text, '')
  FROM speaker_review_task t
  JOIN speaker_cluster sc ON sc.id = t.speaker_cluster_id
  LEFT JOIN speaker_assignment sa
    ON sa.diarization_job_id = t.diarization_job_id
   AND sa.speaker_cluster_id = t.speaker_cluster_id
  LEFT JOIN LATERAL (
    SELECT evidence_text, start_ms, end_ms
      FROM speaker_identity_evidence e
     WHERE e.id = ANY(t.evidence_ids)
     ORDER BY confidence DESC NULLS LAST, id
     LIMIT 1
  ) e ON true
 WHERE sc.id = $1
 ORDER BY CASE t.status WHEN 'pending' THEN 0 WHEN 'accepted' THEN 1 ELSE 2 END,
          t.priority DESC, t.created_at DESC;`
	rows, err := s.Pool.Query(ctx, q, clusterID)
	if err != nil {
		return nil, fmt.Errorf("list speaker review tasks for cluster: %w", err)
	}
	defer rows.Close()
	out := []SpeakerReviewTask{}
	for rows.Next() {
		var t SpeakerReviewTask
		if err := rows.Scan(&t.ID, &t.DiarizationJobID, &t.TVWEventID, &t.ClusterID, &t.ClusterLabel,
			&t.TotalSpeechMS, &t.TurnCount, &t.Status, &t.Priority, &t.CandidateKind,
			&t.CandidateID, &t.CandidateLabel, &t.CandidateConfidence, &t.EvidenceIDs,
			&t.EvidenceText, &t.EvidenceStartMS, &t.EvidenceEndMS,
			&t.CurrentSpeakerLabel, &t.CurrentReviewStatus); err != nil {
			return nil, fmt.Errorf("scan speaker review task for cluster: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) listSpeakerIdentityEvidenceForCluster(ctx context.Context, clusterID int64) ([]SpeakerIdentityEvidence, error) {
	const q = `
SELECT id, evidence_type::text, evidence_text, candidate_kind::text,
       COALESCE(candidate_id,0), candidate_label, COALESCE(confidence,0),
       COALESCE(start_ms,0), COALESCE(end_ms,0)
  FROM speaker_identity_evidence
 WHERE speaker_cluster_id = $1
 ORDER BY confidence DESC NULLS LAST, id;`
	rows, err := s.Pool.Query(ctx, q, clusterID)
	if err != nil {
		return nil, fmt.Errorf("list speaker identity evidence for cluster: %w", err)
	}
	defer rows.Close()
	out := []SpeakerIdentityEvidence{}
	for rows.Next() {
		var e SpeakerIdentityEvidence
		if err := rows.Scan(&e.ID, &e.EvidenceType, &e.EvidenceText, &e.CandidateKind,
			&e.CandidateID, &e.CandidateLabel, &e.Confidence, &e.StartMS, &e.EndMS); err != nil {
			return nil, fmt.Errorf("scan speaker identity evidence: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) ManualAssignSpeakerCluster(ctx context.Context, clusterID int64, kind, label, reviewer, notes string) error {
	kind = strings.TrimSpace(kind)
	label = strings.TrimSpace(label)
	if kind == "" {
		kind = "person"
	}
	if label == "" {
		return fmt.Errorf("speaker label is required")
	}
	cluster, err := s.GetSpeakerClusterReview(ctx, clusterID)
	if err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
INSERT INTO speaker_assignment (diarization_job_id, speaker_cluster_id, speaker_kind,
                                speaker_id, speaker_label, confidence, review_status)
VALUES ($1,$2,$3::speaker_candidate_kind,NULL,$4,NULL,'accepted')
ON CONFLICT (diarization_job_id, speaker_cluster_id) DO UPDATE SET
  speaker_kind = EXCLUDED.speaker_kind,
  speaker_id = NULL,
  speaker_label = EXCLUDED.speaker_label,
  confidence = NULL,
  review_task_id = NULL,
  review_status = 'accepted',
  updated_at = NOW();`, cluster.DiarizationJobID, cluster.ClusterID, kind, label); err != nil {
		return fmt.Errorf("manual speaker assignment: %w", err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE speaker_review_task
   SET status = 'superseded', reviewer = NULLIF($3,''), review_notes = NULLIF($4,''), reviewed_at = NOW(), updated_at = NOW()
 WHERE diarization_job_id = $1
   AND speaker_cluster_id = $2
   AND status = 'pending';`, cluster.DiarizationJobID, cluster.ClusterID, reviewer, notes); err != nil {
		return fmt.Errorf("supersede speaker review tasks: %w", err)
	}
	return tx.Commit(ctx)
}

func (s *Store) AcceptSpeakerReviewTask(ctx context.Context, taskID int64, reviewer, notes string) error {
	t, err := s.GetSpeakerReviewTask(ctx, taskID)
	if err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
UPDATE speaker_review_task
   SET status = 'accepted', reviewer = NULLIF($2,''), review_notes = NULLIF($3,''), reviewed_at = NOW(), updated_at = NOW()
 WHERE id = $1;`, taskID, reviewer, notes); err != nil {
		return fmt.Errorf("accept speaker review task: %w", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO speaker_assignment (diarization_job_id, speaker_cluster_id, speaker_kind,
                                speaker_id, speaker_label, confidence, review_task_id, review_status)
VALUES ($1,$2,$3::speaker_candidate_kind,NULLIF($4,0),$5,NULLIF($6,0),$7,'accepted')
ON CONFLICT (diarization_job_id, speaker_cluster_id) DO UPDATE SET
  speaker_kind = EXCLUDED.speaker_kind,
  speaker_id = EXCLUDED.speaker_id,
  speaker_label = EXCLUDED.speaker_label,
  confidence = EXCLUDED.confidence,
  review_task_id = EXCLUDED.review_task_id,
  review_status = 'accepted',
  updated_at = NOW();`, t.DiarizationJobID, t.ClusterID, t.CandidateKind, t.CandidateID, t.CandidateLabel, t.CandidateConfidence, taskID); err != nil {
		return fmt.Errorf("insert speaker assignment: %w", err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE speaker_review_task
   SET status = 'superseded', updated_at = NOW()
 WHERE diarization_job_id = $1
   AND speaker_cluster_id = $2
   AND id <> $3
   AND status = 'pending';`, t.DiarizationJobID, t.ClusterID, taskID); err != nil {
		return fmt.Errorf("supersede competing speaker review tasks: %w", err)
	}
	return tx.Commit(ctx)
}

func (s *Store) RejectSpeakerReviewTask(ctx context.Context, taskID int64, reviewer, notes string) error {
	_, err := s.Pool.Exec(ctx, `
UPDATE speaker_review_task
   SET status = 'rejected', reviewer = NULLIF($2,''), review_notes = NULLIF($3,''), reviewed_at = NOW(), updated_at = NOW()
 WHERE id = $1;`, taskID, reviewer, notes)
	if err != nil {
		return fmt.Errorf("reject speaker review task: %w", err)
	}
	return nil
}

func (s *Store) NeedsMoreEvidenceSpeakerReviewTask(ctx context.Context, taskID int64, reviewer, notes string) error {
	_, err := s.Pool.Exec(ctx, `
UPDATE speaker_review_task
   SET status = 'needs_more_evidence', reviewer = NULLIF($2,''), review_notes = NULLIF($3,''), reviewed_at = NOW(), updated_at = NOW()
 WHERE id = $1;`, taskID, reviewer, notes)
	if err != nil {
		return fmt.Errorf("needs more evidence speaker review task: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// organization source mentions + CSI organization population
// ---------------------------------------------------------------------------
