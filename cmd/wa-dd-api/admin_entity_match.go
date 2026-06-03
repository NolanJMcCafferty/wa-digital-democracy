package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func init() {
	registerRoute(route{Method: "GET", Path: "/review/entities/candidates", Scope: scopeAdminViewer, Store: adminListEntityMatchCandidatesHandler})
	registerRoute(route{Method: "POST", Path: "/review/entities/candidates/{candidateId}/decide", Scope: scopeAdminReviewer, Store: adminDecideEntityMatchHandler})
}

func adminListEntityMatchCandidatesHandler(store *db.Store) http.HandlerFunc {
	type segment struct {
		StartMS      int    `json:"start_ms"`
		EndMS        int    `json:"end_ms"`
		Text         string `json:"text"`
		ClusterLabel string `json:"cluster_label,omitempty"`
	}
	type transcriptContext struct {
		TVWEventID        string    `json:"tvw_event_id"`
		DiarizationJobID  int64     `json:"diarization_job_id,omitempty"`
		MentionStartMS    int       `json:"mention_start_ms"`
		MentionEndMS      int       `json:"mention_end_ms"`
		MentionText       string    `json:"mention_text"`
		MentionConfidence float64   `json:"mention_confidence"`
		Surrounding       []segment `json:"surrounding"`
	}
	type item struct {
		ID                  int64              `json:"id"`
		SourceKind          string             `json:"source_kind"`
		SourceTable         string             `json:"source_table"`
		SourcePK            int64              `json:"source_pk,omitempty"`
		SourceDatasetID     string             `json:"source_dataset_id,omitempty"`
		SourceRowID         string             `json:"source_row_id,omitempty"`
		SourceName          string             `json:"source_name"`
		NormalizedName      string             `json:"normalized_name"`
		OrganizationID      int64              `json:"organization_id"`
		CanonicalName       string             `json:"canonical_name"`
		CandidateConfidence string             `json:"candidate_confidence"`
		Evidence            []string           `json:"evidence"`
		Decision            string             `json:"decision"`
		ReviewedConfidence  string             `json:"reviewed_confidence,omitempty"`
		Transcript          *transcriptContext `json:"transcript,omitempty"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		q := req.URL.Query()
		sourceKind := strings.TrimSpace(q.Get("source_kind"))
		decision := strings.TrimSpace(q.Get("decision"))
		limit := 50
		if v := q.Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}
		if limit > 200 {
			limit = 200
		}
		offset := 0
		if v := q.Get("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				offset = n
			}
		}
		candidates, total, err := store.ListVendorEntityMatchCandidatesPage(req.Context(), sourceKind, decision, limit, offset)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		ids := make([]int64, 0, len(candidates))
		for _, c := range candidates {
			if c.SourceKind == "deepgram_organization_mention" {
				ids = append(ids, c.ID)
			}
		}
		ctxByID, err := store.ListEntityMatchTranscriptContext(req.Context(), ids)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		out := make([]item, 0, len(candidates))
		for _, c := range candidates {
			evidence := c.Evidence
			if evidence == nil {
				evidence = []string{}
			}
			it := item{
				ID: c.ID, SourceKind: c.SourceKind, SourceTable: c.SourceTable,
				SourcePK: c.SourcePK, SourceDatasetID: c.SourceDatasetID, SourceRowID: c.SourceRowID,
				SourceName: c.SourceName, NormalizedName: c.NormalizedName,
				OrganizationID: c.OrganizationID, CanonicalName: c.CanonicalName,
				CandidateConfidence: c.CandidateConfidence, Evidence: evidence,
				Decision: c.Decision, ReviewedConfidence: c.ReviewedConfidence,
			}
			if tc, ok := ctxByID[c.ID]; ok {
				segs := make([]segment, 0, len(tc.Surrounding))
				for _, s := range tc.Surrounding {
					segs = append(segs, segment{StartMS: s.StartMS, EndMS: s.EndMS, Text: s.Text, ClusterLabel: s.ClusterLabel})
				}
				it.Transcript = &transcriptContext{
					TVWEventID:        tc.TVWEventID,
					DiarizationJobID:  tc.DiarizationJobID,
					MentionStartMS:    tc.MentionStartMS,
					MentionEndMS:      tc.MentionEndMS,
					MentionText:       tc.MentionText,
					MentionConfidence: tc.MentionConfidence,
					Surrounding:       segs,
				}
			}
			out = append(out, it)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"candidates": out,
			"total":      total,
			"limit":      limit,
			"offset":     offset,
		})
	}
}

func adminDecideEntityMatchHandler(store *db.Store) http.HandlerFunc {
	type body struct {
		Decision   string `json:"decision"`
		Confidence string `json:"confidence"`
		Notes      string `json:"notes"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		candidateID, err := strconv.ParseInt(chi.URLParam(req, "candidateId"), 10, 64)
		if err != nil || candidateID <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad candidate id"})
			return
		}
		var b body
		_ = json.NewDecoder(req.Body).Decode(&b)
		switch b.Decision {
		case "confirmed", "rejected", "needs_review":
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "decision must be confirmed, rejected, or needs_review"})
			return
		}
		var organizationID int64
		if err := store.Pool.QueryRow(req.Context(), `SELECT organization_id FROM vendor_entity_match_candidate WHERE id = $1`, candidateID).Scan(&organizationID); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "candidate not found"})
			return
		}
		previousState, _ := store.GetVendorEntityMatchCandidate(req.Context(), candidateID)
		user, _ := adminUserFromContext(req.Context())
		if _, err := store.UpsertVendorEntityMatchDecision(req.Context(), db.InsertVendorEntityMatchDecisionParams{
			CandidateID:    candidateID,
			OrganizationID: organizationID,
			Decision:       b.Decision,
			Confidence:     b.Confidence,
			ReviewedBy:     user.Email,
			ReviewNotes:    b.Notes,
		}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		newState, _ := store.GetVendorEntityMatchCandidate(req.Context(), candidateID)
		adminMutationAudit(store, req, "entity_match_decide", "entity_match_candidate", strconv.FormatInt(candidateID, 10), previousState, newState, b.Notes)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
