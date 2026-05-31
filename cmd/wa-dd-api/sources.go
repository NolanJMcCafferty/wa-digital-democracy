package main

import (
	"net/http"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func listSourcesHandler(store *db.Store) http.HandlerFunc {
	type item struct {
		System          string    `json:"system"`
		Calls           int       `json:"calls"`
		LatestFetchedAt time.Time `json:"latest_fetched_at"`
		Endpoints       []string  `json:"endpoints"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		summaries, err := store.ListSourceSummaries(req.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		out := make([]item, 0, len(summaries))
		for _, s := range summaries {
			eps := s.Endpoints
			if eps == nil {
				eps = []string{}
			}
			out = append(out, item{
				System: s.System, Calls: s.Calls,
				LatestFetchedAt: s.LatestFetchedAt,
				Endpoints:       eps,
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}
