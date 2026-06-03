package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/entitymatch"
)

type VendorEntityMatchCandidate struct {
	ID                  int64
	SourceKind          string
	SourceTable         string
	SourcePK            int64
	SourceDatasetID     string
	SourceRowID         string
	SourceName          string
	NormalizedName      string
	OrganizationID      int64
	CanonicalName       string
	CandidateConfidence string
	Evidence            []string
	Decision            string
	ReviewedConfidence  string
}

type VendorEntityMatchProgress struct {
	Phase         string
	Scanned       int
	Upserted      int
	AutoConfirmed int
	Skipped       int
	SourceKind    string
	ElapsedTime   time.Duration
}

type UpsertVendorEntityMatchCandidateParams struct {
	SourceKind          string
	SourceTable         string
	SourcePK            int64
	SourceDatasetID     string
	SourceRowID         string
	SourceName          string
	NormalizedName      string
	OrganizationID      int64
	CandidateConfidence string
	Evidence            []string
}

func (s *Store) UpsertVendorEntityMatchCandidate(ctx context.Context, p UpsertVendorEntityMatchCandidateParams) (int64, error) {
	evidence, err := json.Marshal(entitymatch.UniqueStrings(p.Evidence))
	if err != nil {
		return 0, fmt.Errorf("marshal evidence: %w", err)
	}
	const q = `
INSERT INTO vendor_entity_match_candidate (
  source_kind, source_table, source_pk, source_dataset_id, source_row_id,
  source_name, normalized_name, organization_id, candidate_confidence,
  evidence
) VALUES (
  $1::entity_match_source_kind,$2,NULLIF($3,0),$4,$5,$6,$7,$8,$9::org_match_confidence,$10::jsonb
)
ON CONFLICT (source_kind, source_dataset_id, source_row_id, source_name, organization_id) DO UPDATE SET
  source_table = EXCLUDED.source_table,
  source_pk = EXCLUDED.source_pk,
  normalized_name = EXCLUDED.normalized_name,
  candidate_confidence = EXCLUDED.candidate_confidence,
  evidence = EXCLUDED.evidence,
  updated_at = NOW()
RETURNING id;`
	var id int64
	if err := s.Pool.QueryRow(ctx, q,
		p.SourceKind, p.SourceTable, p.SourcePK, strOrNull(p.SourceDatasetID), strOrNull(p.SourceRowID),
		p.SourceName, p.NormalizedName, p.OrganizationID, defaultStr(p.CandidateConfidence, "possible"), string(evidence),
	).Scan(&id); err != nil {
		return 0, fmt.Errorf("upsert vendor entity match candidate: %w", err)
	}
	return id, nil
}

type InsertVendorEntityMatchDecisionParams struct {
	CandidateID    int64
	OrganizationID int64
	Decision       string
	Confidence     string
	ReviewedBy     string
	ReviewNotes    string
}

type AdminAuditLogParams struct {
	ActorUserID   string
	ActorEmail    string
	ActorName     string
	ActorRole     string
	Route         string
	Action        string
	TargetType    string
	TargetID      string
	PreviousState []byte
	NewState      []byte
	ReviewerNotes string
	RequestID     string
	IPAddress     string
	UserAgent     string
}

func (s *Store) InsertAdminAuditLog(ctx context.Context, p AdminAuditLogParams) (int64, error) {
	const q = `
INSERT INTO admin_audit_log (
  actor_user_id, actor_email, actor_name, actor_role, route, action, target_type, target_id,
  previous_state, new_state, reviewer_notes, request_id, ip_address, user_agent
) VALUES ($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,NULLIF($9,'')::jsonb,NULLIF($10,'')::jsonb,NULLIF($11,''),NULLIF($12,''),NULLIF($13,'')::inet,NULLIF($14,''))
RETURNING id;`
	var id int64
	if err := s.Pool.QueryRow(ctx, q,
		p.ActorUserID, p.ActorEmail, p.ActorName, p.ActorRole, p.Route, p.Action, p.TargetType, p.TargetID,
		string(p.PreviousState), string(p.NewState), p.ReviewerNotes, p.RequestID, p.IPAddress, p.UserAgent,
	).Scan(&id); err != nil {
		return 0, fmt.Errorf("insert admin audit log: %w", err)
	}
	return id, nil
}

func (s *Store) UpsertVendorEntityMatchDecision(ctx context.Context, p InsertVendorEntityMatchDecisionParams) (int64, error) {
	const q = `
INSERT INTO vendor_entity_match_decision (
  candidate_id, organization_id, decision, reviewed_confidence, reviewed_by, review_notes
) VALUES ($1,$2,$3::entity_match_decision,$4::org_match_confidence,$5,$6)
ON CONFLICT (candidate_id) DO UPDATE SET
  organization_id = EXCLUDED.organization_id,
  decision = EXCLUDED.decision,
  reviewed_confidence = EXCLUDED.reviewed_confidence,
  reviewed_by = EXCLUDED.reviewed_by,
  review_notes = EXCLUDED.review_notes,
  reviewed_at = NOW()
RETURNING id;`
	var id int64
	if err := s.Pool.QueryRow(ctx, q,
		p.CandidateID, p.OrganizationID, defaultStr(p.Decision, "needs_review"), defaultStr(p.Confidence, "possible"),
		strOrNull(p.ReviewedBy), strOrNull(p.ReviewNotes),
	).Scan(&id); err != nil {
		return 0, fmt.Errorf("upsert vendor entity match decision: %w", err)
	}
	if p.Decision == "confirmed" {
		var sourceName string
		if err := s.Pool.QueryRow(ctx, `SELECT source_name FROM vendor_entity_match_candidate WHERE id = $1`, p.CandidateID).Scan(&sourceName); err != nil {
			return 0, fmt.Errorf("lookup confirmed entity-match source name: %w", err)
		}
		if err := s.AddOrganizationAlias(ctx, p.OrganizationID, sourceName); err != nil {
			return 0, err
		}
	}
	return id, nil
}

func (s *Store) ListVendorEntityMatchCandidates(ctx context.Context, sourceKind, decision string, limit int) ([]VendorEntityMatchCandidate, error) {
	out, _, err := s.ListVendorEntityMatchCandidatesPage(ctx, sourceKind, decision, limit, 0)
	return out, err
}

func (s *Store) GetVendorEntityMatchCandidate(ctx context.Context, id int64) (VendorEntityMatchCandidate, error) {
	const q = `
SELECT c.id, c.source_kind::text, c.source_table, COALESCE(c.source_pk,0),
       COALESCE(c.source_dataset_id,''), COALESCE(c.source_row_id,''), c.source_name,
       c.normalized_name, c.organization_id, o.canonical_name,
       c.candidate_confidence::text, c.evidence,
       COALESCE(d.decision::text, 'needs_review'), COALESCE(d.reviewed_confidence::text, '')
  FROM vendor_entity_match_candidate c
  JOIN organization o ON o.id = c.organization_id
  LEFT JOIN vendor_entity_match_decision d ON d.candidate_id = c.id
 WHERE c.id = $1;`
	var c VendorEntityMatchCandidate
	var evidence []byte
	if err := s.Pool.QueryRow(ctx, q, id).Scan(&c.ID, &c.SourceKind, &c.SourceTable, &c.SourcePK,
		&c.SourceDatasetID, &c.SourceRowID, &c.SourceName, &c.NormalizedName,
		&c.OrganizationID, &c.CanonicalName, &c.CandidateConfidence,
		&evidence, &c.Decision, &c.ReviewedConfidence); err != nil {
		return VendorEntityMatchCandidate{}, fmt.Errorf("get vendor entity match candidate: %w", err)
	}
	if len(evidence) > 0 {
		_ = json.Unmarshal(evidence, &c.Evidence)
	}
	return c, nil
}

// ListVendorEntityMatchCandidatesPage returns a page of candidates plus the
// total row count matching the same source_kind/decision filters. limit <= 0
// falls back to 100; offset < 0 is clamped to 0.
func (s *Store) ListVendorEntityMatchCandidatesPage(ctx context.Context, sourceKind, decision string, limit, offset int) ([]VendorEntityMatchCandidate, int, error) {
	if offset < 0 {
		offset = 0
	}
	const countQ = `
SELECT COUNT(*)
  FROM vendor_entity_match_candidate c
  LEFT JOIN vendor_entity_match_decision d ON d.candidate_id = c.id
 WHERE ($1 = '' OR c.source_kind::text = $1)
   AND ($2 = '' OR COALESCE(d.decision::text, 'needs_review') = $2);`
	var total int
	if err := s.Pool.QueryRow(ctx, countQ, sourceKind, decision).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count vendor entity match candidates: %w", err)
	}
	const q = `
SELECT c.id, c.source_kind::text, c.source_table, COALESCE(c.source_pk,0),
       COALESCE(c.source_dataset_id,''), COALESCE(c.source_row_id,''), c.source_name,
       c.normalized_name, c.organization_id, o.canonical_name,
       c.candidate_confidence::text, c.evidence,
       COALESCE(d.decision::text, 'needs_review'), COALESCE(d.reviewed_confidence::text, '')
  FROM vendor_entity_match_candidate c
  JOIN organization o ON o.id = c.organization_id
  LEFT JOIN vendor_entity_match_decision d ON d.candidate_id = c.id
 WHERE ($1 = '' OR c.source_kind::text = $1)
   AND ($2 = '' OR COALESCE(d.decision::text, 'needs_review') = $2)
 ORDER BY c.updated_at DESC, c.id DESC
 LIMIT CASE WHEN $3 > 0 THEN $3 ELSE 100 END
 OFFSET $4;`
	rows, err := s.Pool.Query(ctx, q, sourceKind, decision, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list vendor entity match candidates: %w", err)
	}
	defer rows.Close()
	out := []VendorEntityMatchCandidate{}
	for rows.Next() {
		var c VendorEntityMatchCandidate
		var evidence []byte
		if err := rows.Scan(&c.ID, &c.SourceKind, &c.SourceTable, &c.SourcePK,
			&c.SourceDatasetID, &c.SourceRowID, &c.SourceName, &c.NormalizedName,
			&c.OrganizationID, &c.CanonicalName, &c.CandidateConfidence,
			&evidence, &c.Decision, &c.ReviewedConfidence); err != nil {
			return nil, 0, fmt.Errorf("scan vendor entity match candidate: %w", err)
		}
		if len(evidence) > 0 {
			_ = json.Unmarshal(evidence, &c.Evidence)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

type EntityMatchTestimonyAppearance struct {
	HearingID       int64
	HearingTitle    string
	CommitteeName   string
	MeetingDateTime time.Time
	BillID          string
	BillPrefix      string
	BillNumber      int
	CSIAgendaItemID string
	Position        string
	TestifierName   string
}

type EntityMatchTestimonyContext struct {
	CandidateID    int64
	TestifierCount int
	Appearances    []EntityMatchTestimonyAppearance
}

// ListEntityMatchTestimonyContext returns CSI testimony context for the given
// candidate ids. Only candidates with source_kind = 'csi_testimony_organization'
// produce a row; other source kinds are silently absent from the result map.
//
// For each candidate, returns up to 5 most-recent agenda_item appearances where
// any testifier had raw_organization matching the candidate's source_name (case
// and whitespace insensitive), plus the total count of matching testifier rows.
func (s *Store) ListEntityMatchTestimonyContext(ctx context.Context, candidateIDs []int64) (map[int64]EntityMatchTestimonyContext, error) {
	out := map[int64]EntityMatchTestimonyContext{}
	if len(candidateIDs) == 0 {
		return out, nil
	}
	const q = `
WITH cand AS (
  SELECT id, source_name
    FROM vendor_entity_match_candidate
   WHERE id = ANY($1::bigint[])
     AND source_kind = 'csi_testimony_organization'
), matching_testifiers AS (
  SELECT c.id AS candidate_id, t.id AS testifier_id, t.agenda_item_id,
         t.raw_name, t.position::text AS position
    FROM cand c
    JOIN testifier t ON trim(t.raw_organization) = c.source_name
), counts AS (
  SELECT candidate_id, COUNT(*) AS testifier_count
    FROM matching_testifiers
   GROUP BY candidate_id
), appearances AS (
  SELECT mt.candidate_id,
         h.id AS hearing_id,
         COALESCE(h.committee_name || ' — ' || to_char(h.meeting_datetime, 'YYYY-MM-DD'), '') AS hearing_title,
         h.committee_name,
         h.meeting_datetime,
         COALESCE(b.prefix || ' ' || b.number::text, '') AS bill_id,
         COALESCE(b.prefix, '') AS bill_prefix,
         COALESCE(b.number, 0) AS bill_number,
         COALESCE(a.csi_agenda_item_id, '') AS csi_agenda_item_id,
         mt.position,
         mt.raw_name,
         ROW_NUMBER() OVER (PARTITION BY mt.candidate_id ORDER BY h.meeting_datetime DESC, mt.testifier_id DESC) AS rn
    FROM matching_testifiers mt
    JOIN agenda_item a ON a.id = mt.agenda_item_id
    JOIN hearing h ON h.id = a.hearing_id
    LEFT JOIN bill b ON b.id = a.bill_id
)
SELECT counts.candidate_id, counts.testifier_count,
       COALESCE(jsonb_agg(
         jsonb_build_object(
           'hearing_id', appearances.hearing_id,
           'hearing_title', appearances.hearing_title,
           'committee_name', appearances.committee_name,
           'meeting_datetime', appearances.meeting_datetime,
           'bill_id', appearances.bill_id,
           'bill_prefix', appearances.bill_prefix,
           'bill_number', appearances.bill_number,
           'csi_agenda_item_id', appearances.csi_agenda_item_id,
           'position', appearances.position,
           'testifier_name', appearances.raw_name
         ) ORDER BY appearances.meeting_datetime DESC
       ) FILTER (WHERE appearances.rn IS NOT NULL AND appearances.rn <= 5), '[]'::jsonb) AS apps
  FROM counts
  LEFT JOIN appearances ON appearances.candidate_id = counts.candidate_id
 GROUP BY counts.candidate_id, counts.testifier_count;`
	rows, err := s.Pool.Query(ctx, q, candidateIDs)
	if err != nil {
		return nil, fmt.Errorf("list entity match testimony context: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var c EntityMatchTestimonyContext
		var apps []byte
		if err := rows.Scan(&c.CandidateID, &c.TestifierCount, &apps); err != nil {
			return nil, fmt.Errorf("scan entity match testimony context: %w", err)
		}
		if len(apps) > 0 {
			var raw []struct {
				HearingID       int64     `json:"hearing_id"`
				HearingTitle    string    `json:"hearing_title"`
				CommitteeName   string    `json:"committee_name"`
				MeetingDateTime time.Time `json:"meeting_datetime"`
				BillID          string    `json:"bill_id"`
				BillPrefix      string    `json:"bill_prefix"`
				BillNumber      int       `json:"bill_number"`
				CSIAgendaItemID string    `json:"csi_agenda_item_id"`
				Position        string    `json:"position"`
				TestifierName   string    `json:"testifier_name"`
			}
			if err := json.Unmarshal(apps, &raw); err != nil {
				return nil, fmt.Errorf("unmarshal testimony appearances: %w", err)
			}
			c.Appearances = make([]EntityMatchTestimonyAppearance, 0, len(raw))
			for _, r := range raw {
				c.Appearances = append(c.Appearances, EntityMatchTestimonyAppearance{
					HearingID: r.HearingID, HearingTitle: r.HearingTitle, CommitteeName: r.CommitteeName,
					MeetingDateTime: r.MeetingDateTime, BillID: r.BillID, BillPrefix: r.BillPrefix,
					BillNumber: r.BillNumber, CSIAgendaItemID: r.CSIAgendaItemID,
					Position: r.Position, TestifierName: r.TestifierName,
				})
			}
		}
		out[c.CandidateID] = c
	}
	return out, rows.Err()
}

type EntityMatchTranscriptSegment struct {
	StartMS      int
	EndMS        int
	Text         string
	ClusterLabel string
}

type EntityMatchTranscriptContext struct {
	CandidateID       int64
	TVWEventID        string
	DiarizationJobID  int64
	MentionStartMS    int
	MentionEndMS      int
	MentionText       string
	MentionConfidence float64
	Surrounding       []EntityMatchTranscriptSegment
}

// ListEntityMatchTranscriptContext returns deepgram-mention transcript context
// for the given candidate ids. Only candidates with
// source_kind = 'deepgram_organization_mention' produce a row; other source
// kinds are silently absent from the result map.
func (s *Store) ListEntityMatchTranscriptContext(ctx context.Context, candidateIDs []int64) (map[int64]EntityMatchTranscriptContext, error) {
	out := map[int64]EntityMatchTranscriptContext{}
	if len(candidateIDs) == 0 {
		return out, nil
	}
	const q = `
WITH cand AS (
  SELECT id, source_pk
    FROM vendor_entity_match_candidate
   WHERE id = ANY($1::bigint[])
     AND source_kind = 'deepgram_organization_mention'
     AND source_table = 'entity_mention'
     AND source_pk IS NOT NULL
)
SELECT cand.id,
       COALESCE(em.tvw_event_id, ''),
       COALESCE(em.diarization_job_id, 0),
       COALESCE(em.start_ms, 0),
       COALESCE(em.end_ms, 0),
       COALESCE(em.text, ''),
       COALESCE(em.confidence, 0),
       COALESCE(
         jsonb_agg(
           jsonb_build_object(
             'start_ms', d.start_ms,
             'end_ms', d.end_ms,
             'text', COALESCE(d.text, ''),
             'cluster_label', COALESCE(d.cluster_label, '')
           )
           ORDER BY d.start_ms
         ) FILTER (WHERE d.id IS NOT NULL),
         '[]'::jsonb
       ) AS segments
  FROM cand
  JOIN entity_mention em ON em.id = cand.source_pk
  LEFT JOIN diarized_speech_segment d
    ON d.tvw_event_id = em.tvw_event_id
   AND d.end_ms   >= COALESCE(em.start_ms, 0) - 15000
   AND d.start_ms <= COALESCE(em.end_ms,   0) + 15000
 GROUP BY cand.id, em.tvw_event_id, em.diarization_job_id,
          em.start_ms, em.end_ms, em.text, em.confidence;`
	rows, err := s.Pool.Query(ctx, q, candidateIDs)
	if err != nil {
		return nil, fmt.Errorf("list entity match transcript context: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var c EntityMatchTranscriptContext
		var segs []byte
		if err := rows.Scan(&c.CandidateID, &c.TVWEventID, &c.DiarizationJobID,
			&c.MentionStartMS, &c.MentionEndMS, &c.MentionText, &c.MentionConfidence, &segs); err != nil {
			return nil, fmt.Errorf("scan entity match transcript context: %w", err)
		}
		if len(segs) > 0 {
			var raw []struct {
				StartMS      int    `json:"start_ms"`
				EndMS        int    `json:"end_ms"`
				Text         string `json:"text"`
				ClusterLabel string `json:"cluster_label"`
			}
			if err := json.Unmarshal(segs, &raw); err == nil {
				for _, r := range raw {
					c.Surrounding = append(c.Surrounding, EntityMatchTranscriptSegment{
						StartMS: r.StartMS, EndMS: r.EndMS, Text: r.Text, ClusterLabel: r.ClusterLabel,
					})
				}
			}
		}
		out[c.CandidateID] = c
	}
	return out, rows.Err()
}

func (s *Store) GenerateVendorEntityMatchCandidates(ctx context.Context, limit int) ([]VendorEntityMatchCandidate, error) {
	return s.GenerateVendorEntityMatchCandidatesWithProgress(ctx, limit, nil)
}

func (s *Store) GenerateVendorEntityMatchCandidatesWithProgress(ctx context.Context, limit int, progress func(VendorEntityMatchProgress)) ([]VendorEntityMatchCandidate, error) {
	started := time.Now()
	emit := func(phase string, scanned, upserted, autoConfirmed, skipped int, sourceKind string) {
		if progress == nil {
			return
		}
		progress(VendorEntityMatchProgress{
			Phase:         phase,
			Scanned:       scanned,
			Upserted:      upserted,
			AutoConfirmed: autoConfirmed,
			Skipped:       skipped,
			SourceKind:    sourceKind,
			ElapsedTime:   time.Since(started).Round(time.Second),
		})
	}

	emit("querying", 0, 0, 0, 0, "")
	rows, err := s.Pool.Query(ctx, vendorCandidateSourceQuery, limit)
	if err != nil {
		return nil, fmt.Errorf("vendor entity candidate source query: %w", err)
	}
	defer rows.Close()

	var out []VendorEntityMatchCandidate
	scanned := 0
	skipped := 0
	autoConfirmed := 0
	lastProgress := time.Now()
	emit("processing", scanned, len(out), autoConfirmed, skipped, "")
	for rows.Next() {
		var sourceKind, sourceTable, sourceDatasetID, sourceRowID, sourceName, normalizedName string
		var sourcePK, orgID int64
		var sourceMatchCount int
		var canonical string
		var aliases []string
		if err := rows.Scan(&sourceKind, &sourceTable, &sourcePK, &sourceDatasetID, &sourceRowID, &sourceName, &normalizedName, &orgID, &canonical, &aliases, &sourceMatchCount); err != nil {
			return nil, fmt.Errorf("scan vendor entity candidate source: %w", err)
		}
		scanned++
		if entitymatch.FalsePositiveRisk(normalizedName) {
			skipped++
			if scanned == 1 || scanned%1000 == 0 || time.Since(lastProgress) >= 10*time.Second {
				emit("processing", scanned, len(out), autoConfirmed, skipped, sourceKind)
				lastProgress = time.Now()
			}
			continue
		}
		confidence, evidence := entitymatch.ConfidenceFor(sourceName, canonical, aliases)
		if confidence == "" {
			skipped++
			if scanned == 1 || scanned%1000 == 0 || time.Since(lastProgress) >= 10*time.Second {
				emit("processing", scanned, len(out), autoConfirmed, skipped, sourceKind)
				lastProgress = time.Now()
			}
			continue
		}
		id, err := s.UpsertVendorEntityMatchCandidate(ctx, UpsertVendorEntityMatchCandidateParams{
			SourceKind:          sourceKind,
			SourceTable:         sourceTable,
			SourcePK:            sourcePK,
			SourceDatasetID:     sourceDatasetID,
			SourceRowID:         sourceRowID,
			SourceName:          sourceName,
			NormalizedName:      normalizedName,
			OrganizationID:      orgID,
			CandidateConfidence: confidence,
			Evidence:            evidence,
		})
		if err != nil {
			return nil, err
		}
		decision := ""
		reviewedConfidence := ""
		autoConfirm := sourceMatchCount == 1 &&
			(confidence == entitymatch.ConfidenceConfirmed || confidence == entitymatch.ConfidenceProbable)
		if autoConfirm {
			notes := "Auto-confirmed unique exact organization-name match."
			if confidence == entitymatch.ConfidenceProbable {
				notes = "Auto-confirmed unique normalized-name match (no other org collides)."
			}
			if _, err := s.UpsertVendorEntityMatchDecision(ctx, InsertVendorEntityMatchDecisionParams{
				CandidateID:    id,
				OrganizationID: orgID,
				Decision:       "confirmed",
				Confidence:     entitymatch.ConfidenceConfirmed,
				ReviewedBy:     "system:entitymatch",
				ReviewNotes:    notes,
			}); err != nil {
				return nil, err
			}
			verificationSource := "manual"
			if sourceKind == "csi_testimony_organization" {
				verificationSource = "csi_testimony_review"
			}
			if err := s.MarkOrganizationConfirmedByReview(ctx, orgID, verificationSource, "system:entitymatch", notes); err != nil {
				return nil, err
			}
			decision = "confirmed"
			reviewedConfidence = entitymatch.ConfidenceConfirmed
			autoConfirmed++
		}
		out = append(out, VendorEntityMatchCandidate{
			ID: id, SourceKind: sourceKind, SourceTable: sourceTable, SourcePK: sourcePK,
			SourceDatasetID: sourceDatasetID, SourceRowID: sourceRowID, SourceName: sourceName,
			NormalizedName: normalizedName, OrganizationID: orgID, CanonicalName: canonical,
			CandidateConfidence: confidence, Evidence: evidence,
			Decision: decision, ReviewedConfidence: reviewedConfidence,
		})
		if scanned == 1 || scanned%1000 == 0 || time.Since(lastProgress) >= 10*time.Second {
			emit("processing", scanned, len(out), autoConfirmed, skipped, sourceKind)
			lastProgress = time.Now()
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	emit("complete", scanned, len(out), autoConfirmed, skipped, "")
	return out, nil
}

const vendorCandidateSourceQuery = `
WITH source_names AS (
  SELECT 'pdc_lobbying_organization'::text AS source_kind, 'pdc_employer'::text AS source_table,
         0::bigint AS source_pk, 'xhn7-64im'::text AS source_dataset_id, employer_id AS source_row_id,
         name AS source_name, normalized_name
    FROM pdc_employer
   WHERE name IS NOT NULL AND normalized_name IS NOT NULL
  UNION ALL
  SELECT 'datawa_contract_contractor'::text AS source_kind, 'datawa_contract'::text AS source_table,
         id AS source_pk, source_dataset_id, source_row_id, contractor_name AS source_name,
         normalized_contractor_name AS normalized_name
    FROM datawa_contract
   WHERE contractor_name IS NOT NULL AND normalized_contractor_name IS NOT NULL
  UNION ALL
  SELECT 'datawa_master_contract_vendor', 'datawa_master_contract_sale',
         id, source_dataset_id, source_row_id, vendor_name, normalized_vendor_name
    FROM datawa_master_contract_sale
   WHERE vendor_name IS NOT NULL AND normalized_vendor_name IS NOT NULL
  UNION ALL
  SELECT 'datawa_master_contract_customer', 'datawa_master_contract_sale',
         id, source_dataset_id, source_row_id, customer_name, normalized_customer_name
    FROM datawa_master_contract_sale
   WHERE customer_name IS NOT NULL AND normalized_customer_name IS NOT NULL
  UNION ALL
  SELECT 'datawa_it_contract_contractor', 'datawa_it_contract',
         id, source_dataset_id, source_row_id, contractor_name, normalized_contractor_name
    FROM datawa_it_contract
   WHERE contractor_name IS NOT NULL AND normalized_contractor_name IS NOT NULL
  UNION ALL
  SELECT 'datawa_it_contract_dba', 'datawa_it_contract',
         id, source_dataset_id, source_row_id, contractor_dba, normalized_contractor_dba
    FROM datawa_it_contract
   WHERE contractor_dba IS NOT NULL AND normalized_contractor_dba IS NOT NULL
  UNION ALL
  SELECT 'datawa_webs_vendor', 'datawa_webs_vendor',
         id, source_dataset_id, source_row_id, company_name, normalized_company_name
    FROM datawa_webs_vendor
   WHERE company_name IS NOT NULL AND normalized_company_name IS NOT NULL
  UNION ALL
  SELECT 'fiscalwa_vendor_payment', 'fiscalwa_vendor_payment',
         id, source_dataset_id, source_row_id, vendor_name, normalized_vendor_name
    FROM fiscalwa_vendor_payment
   WHERE vendor_name IS NOT NULL AND normalized_vendor_name IS NOT NULL
  UNION ALL
  SELECT 'federal_award_recipient', 'federal_award',
         id, 'usaspending'::text, award_id, recipient_name, normalized_recipient_name
    FROM federal_award
   WHERE recipient_name IS NOT NULL AND normalized_recipient_name IS NOT NULL
  UNION ALL
  -- CSI testimony sign-ins: each distinct testifier.raw_organization string
  -- becomes one candidate per matching canonical organization. We pick a
  -- representative testifier row (MIN(id)) so admin context queries can drill
  -- down to a real agenda_item without storing one row per testifier.
  SELECT 'csi_testimony_organization'::text AS source_kind,
         'testifier'::text AS source_table,
         MIN(t.id) AS source_pk,
         'csi'::text AS source_dataset_id,
         wa_dd_normalize_entity_name(trim(t.raw_organization)) AS source_row_id,
         trim(t.raw_organization) AS source_name,
         wa_dd_normalize_entity_name(trim(t.raw_organization)) AS normalized_name
    FROM testifier t
   WHERE NULLIF(trim(t.raw_organization), '') IS NOT NULL
     AND wa_dd_normalize_entity_name(trim(t.raw_organization)) IS NOT NULL
   GROUP BY trim(t.raw_organization), wa_dd_normalize_entity_name(trim(t.raw_organization))
), limited_source_names AS (
SELECT *
  FROM source_names
 ORDER BY source_kind, normalized_name, source_name
 LIMIT CASE WHEN $1 > 0 THEN $1 ELSE 100000 END
), org_names AS (
SELECT DISTINCT id, canonical_name, aliases, normalized_name
  FROM (
    SELECT id, canonical_name, aliases, wa_dd_normalize_entity_name(canonical_name) AS normalized_name
      FROM organization
    UNION ALL
    SELECT o.id, o.canonical_name, o.aliases, wa_dd_normalize_entity_name(alias) AS normalized_name
      FROM organization o
      CROSS JOIN LATERAL unnest(o.aliases) alias
  ) names
 WHERE normalized_name IS NOT NULL
), matches AS (
SELECT s.source_kind, s.source_table, s.source_pk, s.source_dataset_id, s.source_row_id,
       s.source_name, s.normalized_name,
       o.id, o.canonical_name, o.aliases,
       COUNT(*) OVER (PARTITION BY s.source_kind, s.source_dataset_id, s.source_row_id, s.source_name) AS source_match_count
  FROM limited_source_names s
  JOIN org_names o ON o.normalized_name = s.normalized_name
)
SELECT s.source_kind, s.source_table, s.source_pk, s.source_dataset_id, s.source_row_id,
       s.source_name, s.normalized_name,
       s.id, s.canonical_name, s.aliases, s.source_match_count
  FROM matches s
 ORDER BY s.source_kind, s.normalized_name, s.canonical_name;`
