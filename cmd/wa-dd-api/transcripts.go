package main

import (
	"net/http"
	"strconv"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

const (
	searchDefaultLimit = 20
	searchMaxLimit     = 50
)

func searchTranscriptsHandler(store *db.Store) http.HandlerFunc {
	type hit struct {
		ID              int64     `json:"id"`
		BillID          string    `json:"bill_id,omitempty"`
		Biennium        string    `json:"biennium,omitempty"`
		BillPrefix      string    `json:"bill_prefix,omitempty"`
		BillNumber      int       `json:"bill_number,omitempty"`
		AgendaItemLabel string    `json:"agenda_item_label,omitempty"`
		CommitteeName   string    `json:"committee_name,omitempty"`
		MeetingDateTime time.Time `json:"meeting_datetime,omitempty"`
		StartMS         int       `json:"start_ms"`
		EndMS           int       `json:"end_ms"`
		Text            string    `json:"text"`
		SpeakerLabel    string    `json:"speaker_label,omitempty"`
		TVWEventID      string    `json:"tvw_event_id,omitempty"`
	}
	type body struct {
		Query  string `json:"query"`
		Total  int    `json:"total"`
		Limit  int    `json:"limit"`
		Offset int    `json:"offset"`
		Hits   []hit  `json:"hits"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		q := req.URL.Query().Get("q")

		// Soft parsing — match the bill page handler style. Bad limit/offset
		// values fall back to defaults rather than 400'ing.
		limit := searchDefaultLimit
		if v := req.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}
		if limit > searchMaxLimit {
			limit = searchMaxLimit
		}
		offset := 0
		if v := req.URL.Query().Get("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				offset = n
			}
		}

		hitsRaw, total, err := store.SearchTranscripts(req.Context(), q, limit, offset)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		out := body{
			Query:  q,
			Total:  total,
			Limit:  limit,
			Offset: offset,
			Hits:   make([]hit, 0, len(hitsRaw)),
		}
		for _, h := range hitsRaw {
			out.Hits = append(out.Hits, hit{
				ID:              h.ID,
				BillID:          h.BillID,
				Biennium:        h.Biennium,
				BillPrefix:      h.BillPrefix,
				BillNumber:      h.BillNumber,
				AgendaItemLabel: h.AgendaItemLabel,
				CommitteeName:   h.CommitteeName,
				MeetingDateTime: h.MeetingDateTime,
				StartMS:         h.StartMS,
				EndMS:           h.EndMS,
				Text:            h.Text,
				SpeakerLabel:    h.SpeakerLabel,
				TVWEventID:      h.TVWEventID,
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}
