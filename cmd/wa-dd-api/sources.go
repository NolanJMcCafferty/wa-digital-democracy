package main

import (
	"net/http"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func init() {
	registerRoute(route{Method: "GET", Path: "/sources", Scope: scopeAPI, Store: listSourcesHandler})
}

func listSourcesHandler(_ *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		writeJSON(w, http.StatusOK, []any{})
	}
}
