package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func init() {
	registerRoute(route{Method: "GET", Path: "/hearings", Scope: scopeAPI, Store: listHearingsHandler})
	registerRoute(route{Method: "GET", Path: "/hearings/{hearingId}", Scope: scopeAPI, Store: getHearingHandler})
}

const (
	hearingsDefaultLimit = 50
	hearingsMaxLimit     = 100
)

type hearingAgendaItemResponse struct {
	CSIAgendaItemID string             `json:"csi_agenda_item_id"`
	AgendaItemLabel string             `json:"agenda_item_label"`
	Biennium        string             `json:"biennium"`
	BillID          string             `json:"bill_id"`
	BillPrefix      string             `json:"bill_prefix"`
	BillNumber      int                `json:"bill_number"`
	TestifierCount  int                `json:"testifier_count"`
	TestifiedCount  int                `json:"testified_count"`
	Section         *AgendaItemSection `json:"section,omitempty"`
}

type hearingResponse struct {
	HearingID          int64                       `json:"hearing_id"`
	CommitteeName      string                      `json:"committee_name"`
	Chamber            string                      `json:"chamber"`
	MeetingDateTime    time.Time                   `json:"meeting_datetime"`
	Location           string                      `json:"location,omitempty"`
	TVWURL             string                      `json:"tvw_url,omitempty"`
	TVWEventID         string                      `json:"tvw_event_id,omitempty"`
	AgendaItems        []hearingAgendaItemResponse `json:"agenda_items"`
	DiarizedTranscript *diarizedTranscriptResponse `json:"diarized_transcript,omitempty"`
}

type diarizedTranscriptResponse struct {
	Segments []diarizedSegmentResponse `json:"segments"`
}

type diarizedSegmentResponse struct {
	StartMS      int    `json:"start_ms"`
	EndMS        int    `json:"end_ms"`
	Text         string `json:"text"`
	ClusterID    int64  `json:"cluster_id,omitempty"`
	ClusterLabel string `json:"cluster_label,omitempty"`
	SpeakerLabel string `json:"speaker_label,omitempty"`
	SpeakerKind  string `json:"speaker_kind,omitempty"`
	ReviewStatus string `json:"review_status,omitempty"`
	Reviewed     bool   `json:"reviewed"`
}

func (s diarizedSegmentResponse) PublicSpeakerLabel() string {
	if !s.Reviewed {
		return ""
	}
	return s.SpeakerLabel
}

func mapHearingResponse(h db.HearingAggregate) hearingResponse {
	items := make([]hearingAgendaItemResponse, 0, len(h.AgendaItems))
	for _, a := range h.AgendaItems {
		items = append(items, hearingAgendaItemResponse{
			CSIAgendaItemID: a.CSIAgendaItemID,
			AgendaItemLabel: a.AgendaItemLabel,
			Biennium:        a.Biennium,
			BillID:          a.BillID,
			BillPrefix:      a.BillPrefix,
			BillNumber:      a.BillNumber,
			TestifierCount:  a.TestifierCount,
			TestifiedCount:  a.TestifiedCount,
		})
	}
	return hearingResponse{
		HearingID:       h.HearingID,
		CommitteeName:   h.CommitteeName,
		Chamber:         h.Chamber,
		MeetingDateTime: h.MeetingDateTime,
		Location:        h.Location,
		TVWURL:          h.TVWURL,
		TVWEventID:      h.TVWEventID,
		AgendaItems:     items,
	}
}

func listHearingsHandler(store *db.Store) http.HandlerFunc {
	type body struct {
		Hearings []hearingResponse      `json:"hearings"`
		Total    int                    `json:"total"`
		Limit    int                    `json:"limit"`
		Offset   int                    `json:"offset"`
		Facets   db.HearingSearchFacets `json:"facets"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		q := req.URL.Query()
		params := db.HearingSearchParams{
			Committee:     strings.TrimSpace(q.Get("committee")),
			Bill:          strings.TrimSpace(q.Get("bill")),
			Speaker:       strings.TrimSpace(q.Get("speaker")),
			Chambers:      q["chamber"],
			TopicKeywords: q["topic_keyword"],
			Biennium:      strings.TrimSpace(q.Get("biennium")),
		}
		params.Limit = hearingsDefaultLimit
		if v := q.Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				params.Limit = n
			}
		}
		if params.Limit > hearingsMaxLimit {
			params.Limit = hearingsMaxLimit
		}
		if v := q.Get("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				params.Offset = n
			}
		}

		hs, total, err := store.SearchHearings(req.Context(), params)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		facets, err := store.ListHearingSearchFacets(req.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		out := body{
			Hearings: make([]hearingResponse, 0, len(hs)),
			Total:    total,
			Limit:    params.Limit,
			Offset:   params.Offset,
			Facets:   facets,
		}
		for _, h := range hs {
			out.Hearings = append(out.Hearings, mapHearingResponse(h))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func getHearingHandler(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		idParam := chi.URLParam(req, "hearingId")
		hearingID, err := strconv.ParseInt(idParam, 10, 64)
		if err != nil || hearingID <= 0 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "hearing not found"})
			return
		}
		hearing, err := store.GetHearing(req.Context(), hearingID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "hearing not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		resp := mapHearingResponse(*hearing)
		// Populate the full per-agenda-item sections (testifiers,
		// transcript, organizations) so /hearings/{id} can render them.
		// The list endpoint deliberately omits these to keep the payload
		// small.
		for i := range resp.AgendaItems {
			billAgendaTarget, err := store.LookupBillAgendaTargetByAgendaItem(req.Context(), resp.AgendaItems[i].CSIAgendaItemID)
			if err != nil {
				if errors.Is(err, db.ErrBillNotFound) {
					continue
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			section, err := BuildAgendaItemSection(req.Context(), store, billAgendaTarget)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			resp.AgendaItems[i].Section = section
		}
		if resp.TVWEventID != "" {
			segs, err := store.ListDiarizedSegmentsByTVWEvent(req.Context(), resp.TVWEventID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			if len(segs) > 0 {
				out := make([]diarizedSegmentResponse, 0, len(segs))
				for _, s := range segs {
					out = append(out, diarizedSegmentResponse{
						StartMS: s.StartMS, EndMS: s.EndMS, Text: s.Text,
						ClusterID: s.ClusterID, ClusterLabel: s.ClusterLabel,
						SpeakerLabel: s.SpeakerLabel, SpeakerKind: s.SpeakerKind,
						ReviewStatus: s.ReviewStatus, Reviewed: s.Reviewed,
					})
				}
				resp.DiarizedTranscript = &diarizedTranscriptResponse{Segments: out}
			}
		}
		writeJSON(w, http.StatusOK, resp)
	}
}
