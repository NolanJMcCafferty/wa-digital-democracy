package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

type csiEvidenceAgenda struct {
	ID              int64
	CSIAgendaItemID string
	Label           string
	StartMS         int
	EndMS           int
	Testifiers      []csiEvidenceTestifier
}

type csiEvidenceTestifier struct {
	ID              int64
	RawName         string
	RawOrganization string
	Position        string
	CSIOrder        int
	Testified       bool
}

func generateCSISpeakerEvidence(ctx context.Context, store *db.Store, jobID int64, eventID string, segments []diarizedSegmentForEvidence) (int, int, error) {
	agendas, err := loadCSIEvidenceAgendas(ctx, store, eventID)
	if err != nil {
		return 0, 0, fmt.Errorf("load CSI speaker evidence agendas: %w", err)
	}
	if len(agendas) == 0 || len(segments) == 0 {
		return 0, 0, nil
	}
	createdEvidence, createdTasks := 0, 0
	for _, agenda := range agendas {
		if agenda.StartMS == 0 && agenda.EndMS == 0 {
			continue
		}
		windowSegs := segmentsInWindow(segments, agenda.StartMS, agenda.EndMS)
		if len(windowSegs) == 0 {
			continue
		}
		nameByNorm := map[string]csiEvidenceTestifier{}
		for _, t := range agenda.Testifiers {
			if !t.Testified {
				continue
			}
			nameByNorm[normalizeSpokenName(t.RawName)] = t
		}
		for _, seg := range windowSegs {
			// Chair-call / direct-name evidence: if nearby transcript text names a
			// CSI testifier in this agenda window, create a strong review task.
			textNorm := normalizeSpokenName(seg.Text)
			for norm, t := range nameByNorm {
				if norm == "" || !strings.Contains(textNorm, norm) {
					continue
				}
				eid, err := store.UpsertSpeakerIdentityEvidence(ctx, db.SpeakerIdentityEvidenceParams{
					EvidenceKey:             fmt.Sprintf("job:%d:seg:%d:chair_call:testifier:%d", jobID, seg.ID, t.ID),
					DiarizationJobID:        jobID,
					SpeakerClusterID:        seg.SpeakerClusterID,
					DiarizedSpeechSegmentID: seg.ID,
					EvidenceType:            "chair_call",
					EvidenceText:            seg.Text,
					CandidateKind:           "testifier",
					CandidateID:             t.ID,
					CandidateLabel:          t.RawName,
					Confidence:              0.78,
					StartMS:                 seg.StartMS,
					EndMS:                   seg.EndMS,
					Raw: map[string]any{
						"agenda_item_id":     agenda.ID,
						"csi_agenda_item_id": agenda.CSIAgendaItemID,
						"csi_order":          t.CSIOrder,
						"raw_organization":   t.RawOrganization,
						"position":           t.Position,
						"match":              "normalized_name_substring",
					},
				})
				if err != nil {
					return createdEvidence, createdTasks, fmt.Errorf("store chair-call evidence: %w", err)
				}
				createdEvidence++
				if _, err := store.UpsertSpeakerReviewTask(ctx, db.SpeakerReviewTaskParams{
					DiarizationJobID: jobID,
					SpeakerClusterID: seg.SpeakerClusterID,
					Priority:         145,
					CandidateKind:    "testifier",
					CandidateID:      t.ID,
					CandidateLabel:   t.RawName,
					Confidence:       0.78,
					EvidenceIDs:      []int64{eid},
				}); err != nil {
					return createdEvidence, createdTasks, fmt.Errorf("store chair-call review task: %w", err)
				}
				createdTasks++
			}
		}

		// CSI-order evidence: low-confidence candidate priors based on roster
		// order and first-seen public speaker clusters in the agenda window.
		testifiers := testifiedRoster(agenda.Testifiers)
		clusters := firstClusterTurns(windowSegs)
		limit := len(testifiers)
		if len(clusters) < limit {
			limit = len(clusters)
		}
		for i := 0; i < limit; i++ {
			t := testifiers[i]
			seg := clusters[i]
			eid, err := store.UpsertSpeakerIdentityEvidence(ctx, db.SpeakerIdentityEvidenceParams{
				EvidenceKey:             fmt.Sprintf("job:%d:agenda:%d:csi_order:%d:cluster:%d:testifier:%d", jobID, agenda.ID, i+1, seg.SpeakerClusterID, t.ID),
				DiarizationJobID:        jobID,
				SpeakerClusterID:        seg.SpeakerClusterID,
				DiarizedSpeechSegmentID: seg.ID,
				EvidenceType:            "csi_order",
				EvidenceText:            fmt.Sprintf("CSI order suggests %s may correspond to speaker cluster %s in agenda item %s", t.RawName, seg.ClusterLabel, agenda.Label),
				CandidateKind:           "testifier",
				CandidateID:             t.ID,
				CandidateLabel:          t.RawName,
				Confidence:              0.38,
				StartMS:                 seg.StartMS,
				EndMS:                   seg.EndMS,
				Raw: map[string]any{
					"agenda_item_id":     agenda.ID,
					"csi_agenda_item_id": agenda.CSIAgendaItemID,
					"csi_order":          t.CSIOrder,
					"cluster_rank":       i + 1,
					"raw_organization":   t.RawOrganization,
					"position":           t.Position,
				},
			})
			if err != nil {
				return createdEvidence, createdTasks, fmt.Errorf("store csi-order evidence: %w", err)
			}
			createdEvidence++
			if _, err := store.UpsertSpeakerReviewTask(ctx, db.SpeakerReviewTaskParams{
				DiarizationJobID: jobID,
				SpeakerClusterID: seg.SpeakerClusterID,
				Priority:         55,
				CandidateKind:    "testifier",
				CandidateID:      t.ID,
				CandidateLabel:   t.RawName,
				Confidence:       0.38,
				EvidenceIDs:      []int64{eid},
			}); err != nil {
				return createdEvidence, createdTasks, fmt.Errorf("store csi-order review task: %w", err)
			}
			createdTasks++
		}
	}
	return createdEvidence, createdTasks, nil
}

func loadCSIEvidenceAgendas(ctx context.Context, store *db.Store, eventID string) ([]csiEvidenceAgenda, error) {
	const agendaQ = `
SELECT a.id, COALESCE(a.csi_agenda_item_id,''), a.label,
       COALESCE(MIN(w.start_ms),0), COALESCE(MAX(w.end_ms),0)
  FROM agenda_item a
  JOIN hearing h ON h.id = a.hearing_id
  LEFT JOIN agenda_item_window w ON w.agenda_item_id = a.id
 WHERE h.tvw_event_id = $1
 GROUP BY a.id, a.csi_agenda_item_id, a.label
 ORDER BY a.order_index NULLS LAST, a.id;`
	rows, err := store.Pool.Query(ctx, agendaQ, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	agendas := []csiEvidenceAgenda{}
	for rows.Next() {
		var a csiEvidenceAgenda
		if err := rows.Scan(&a.ID, &a.CSIAgendaItemID, &a.Label, &a.StartMS, &a.EndMS); err != nil {
			return nil, err
		}
		agendas = append(agendas, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	const testifierQ = `
SELECT id, raw_name, COALESCE(raw_organization,''), position::text, COALESCE(csi_order,0), testified
  FROM testifier
 WHERE agenda_item_id = $1
   AND COALESCE(active, TRUE)
 ORDER BY testified DESC, csi_order NULLS LAST, id;`
	for i := range agendas {
		tr, err := store.Pool.Query(ctx, testifierQ, agendas[i].ID)
		if err != nil {
			return nil, err
		}
		for tr.Next() {
			var t csiEvidenceTestifier
			if err := tr.Scan(&t.ID, &t.RawName, &t.RawOrganization, &t.Position, &t.CSIOrder, &t.Testified); err != nil {
				tr.Close()
				return nil, err
			}
			agendas[i].Testifiers = append(agendas[i].Testifiers, t)
		}
		if err := tr.Err(); err != nil {
			tr.Close()
			return nil, err
		}
		tr.Close()
	}
	return agendas, nil
}

func segmentsInWindow(segments []diarizedSegmentForEvidence, startMS, endMS int) []diarizedSegmentForEvidence {
	out := []diarizedSegmentForEvidence{}
	for _, s := range segments {
		if s.StartMS < endMS && s.EndMS > startMS {
			out = append(out, s)
		}
	}
	return out
}

func testifiedRoster(in []csiEvidenceTestifier) []csiEvidenceTestifier {
	out := []csiEvidenceTestifier{}
	for _, t := range in {
		if t.Testified {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CSIOrder == 0 || out[j].CSIOrder == 0 {
			return out[i].ID < out[j].ID
		}
		return out[i].CSIOrder < out[j].CSIOrder
	})
	return out
}

func firstClusterTurns(segments []diarizedSegmentForEvidence) []diarizedSegmentForEvidence {
	seen := map[int64]bool{}
	out := []diarizedSegmentForEvidence{}
	for _, s := range segments {
		if strings.TrimSpace(s.Text) == "" || seen[s.SpeakerClusterID] {
			continue
		}
		if isLikelyProceduralTurn(s.Text) {
			continue
		}
		seen[s.SpeakerClusterID] = true
		out = append(out, s)
	}
	return out
}

func isLikelyProceduralTurn(text string) bool {
	low := strings.ToLower(text)
	patterns := []string{"public hearing", "we'll now", "we will now", "next bill", "next item", "that closes", "that concludes", "members", "staff"}
	for _, p := range patterns {
		if strings.Contains(low, p) {
			return true
		}
	}
	return false
}
