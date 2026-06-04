package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

const (
	adminSpeakerReviewDefaultLimit = 50
	adminSpeakerReviewMaxLimit     = 200
)

func init() {
	registerRoute(route{Method: "GET", Path: "/review/speakers", Scope: scopeAdminViewer, Store: adminListSpeakerReviewTasksHandler})
	registerRoute(route{Method: "GET", Path: "/review/speakers/events", Scope: scopeAdminViewer, Store: adminListSpeakerReviewEventsHandler})
	registerRoute(route{Method: "GET", Path: "/review/speakers/events/{hearingId}", Scope: scopeAdminViewer, Store: adminGetSpeakerReviewEventHandler})
	registerRoute(route{Method: "GET", Path: "/review/speakers/clusters/{clusterId}", Scope: scopeAdminViewer, Store: adminGetSpeakerClusterReviewHandler})
	registerRoute(route{Method: "GET", Path: "/review/speakers/{taskId}", Scope: scopeAdminViewer, Store: adminGetSpeakerReviewTaskHandler})

	registerRoute(route{Method: "POST", Path: "/review/speakers/clusters/{clusterId}/assign", Scope: scopeAdminReviewer, Store: adminManualAssignSpeakerClusterHandler})
	registerRoute(route{Method: "POST", Path: "/review/speakers/{taskId}/accept", Scope: scopeAdminReviewer, Store: func(s *db.Store) http.HandlerFunc {
		return adminSpeakerReviewDecisionHandler(s, "accept")
	}})
	registerRoute(route{Method: "POST", Path: "/review/speakers/{taskId}/reject", Scope: scopeAdminReviewer, Store: func(s *db.Store) http.HandlerFunc {
		return adminSpeakerReviewDecisionHandler(s, "reject")
	}})
	registerRoute(route{Method: "POST", Path: "/review/speakers/{taskId}/needs-more-evidence", Scope: scopeAdminReviewer, Store: func(s *db.Store) http.HandlerFunc {
		return adminSpeakerReviewDecisionHandler(s, "needs_more_evidence")
	}})
}

func adminListSpeakerReviewTasksHandler(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		status := strings.TrimSpace(req.URL.Query().Get("status"))
		limit := 50
		if v := req.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}
		tasks, err := store.ListSpeakerReviewTasks(req.Context(), status, limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
	}
}

func adminListSpeakerReviewEventsHandler(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		q := req.URL.Query()
		limit := adminSpeakerReviewDefaultLimit
		if v := q.Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}
		if limit > adminSpeakerReviewMaxLimit {
			limit = adminSpeakerReviewMaxLimit
		}
		offset := 0
		if v := q.Get("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				offset = n
			}
		}
		events, total, err := store.ListSpeakerReviewEvents(req.Context(), limit, offset)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"events": events,
			"total":  total,
			"limit":  limit,
			"offset": offset,
		})
	}
}

func adminGetSpeakerReviewEventHandler(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		hearingID, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(req, "hearingId")), 10, 64)
		if err != nil || hearingID <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad hearing id"})
			return
		}
		tvwEventID, err := store.GetHearingTVWEventID(req.Context(), hearingID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		if tvwEventID == "" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "hearing has no tvw event"})
			return
		}
		clusters, err := store.ListSpeakerClustersForEvent(req.Context(), tvwEventID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"hearing_id": hearingID, "tvw_event_id": tvwEventID, "clusters": clusters})
	}
}

func adminGetSpeakerClusterReviewHandler(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		id, err := strconv.ParseInt(chi.URLParam(req, "clusterId"), 10, 64)
		if err != nil || id <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad cluster id"})
			return
		}
		cluster, err := store.GetSpeakerClusterReview(req.Context(), id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, cluster)
	}
}

func adminManualAssignSpeakerClusterHandler(store *db.Store) http.HandlerFunc {
	type body struct {
		Kind  string `json:"kind"`
		Label string `json:"label"`
		Notes string `json:"notes"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		id, err := strconv.ParseInt(chi.URLParam(req, "clusterId"), 10, 64)
		if err != nil || id <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad cluster id"})
			return
		}
		var b body
		_ = json.NewDecoder(req.Body).Decode(&b)
		previous, _ := store.GetSpeakerClusterReview(req.Context(), id)
		user, _ := adminUserFromContext(req.Context())
		if err := store.ManualAssignSpeakerCluster(req.Context(), id, b.Kind, b.Label, user.Email, b.Notes); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		current, _ := store.GetSpeakerClusterReview(req.Context(), id)
		adminMutationAudit(store, req, "speaker_cluster_assign", "speaker_cluster", strconv.FormatInt(id, 10), previous, current, b.Notes)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func adminGetSpeakerReviewTaskHandler(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		id, err := strconv.ParseInt(chi.URLParam(req, "taskId"), 10, 64)
		if err != nil || id <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad task id"})
			return
		}
		task, err := store.GetSpeakerReviewTask(req.Context(), id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, task)
	}
}

func adminSpeakerReviewDecisionHandler(store *db.Store, action string) http.HandlerFunc {
	type body struct {
		Notes string `json:"notes"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		id, err := strconv.ParseInt(chi.URLParam(req, "taskId"), 10, 64)
		if err != nil || id <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad task id"})
			return
		}
		var b body
		_ = json.NewDecoder(req.Body).Decode(&b)
		previous, _ := store.GetSpeakerReviewTask(req.Context(), id)
		user, _ := adminUserFromContext(req.Context())
		switch action {
		case "accept":
			err = store.AcceptSpeakerReviewTask(req.Context(), id, user.Email, b.Notes)
		case "reject":
			err = store.RejectSpeakerReviewTask(req.Context(), id, user.Email, b.Notes)
		case "needs_more_evidence":
			err = store.NeedsMoreEvidenceSpeakerReviewTask(req.Context(), id, user.Email, b.Notes)
		default:
			err = fmt.Errorf("unsupported action")
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		current, _ := store.GetSpeakerReviewTask(req.Context(), id)
		adminMutationAudit(store, req, "speaker_review_"+action, "speaker_review_task", strconv.FormatInt(id, 10), previous, current, b.Notes)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
