package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestFirstPageHandler_BadSlug exercises the slug-parse path without
// touching Postgres — the regex check happens before any DB call, so a
// nil store is unreachable and a real *db.Store isn't required.
func TestFirstPageHandler_BadSlug(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/api/v1/bills/{biennium}/{billNumber}/first-page", firstPageHandler(nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bills/2025-26/notaslug/first-page", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid bill slug") {
		t.Errorf("body = %q, want 'invalid bill slug' message", w.Body.String())
	}
}

func TestUpper(t *testing.T) {
	cases := []struct{ in, want string }{
		{"hb", "HB"}, {"HB", "HB"}, {"hB", "HB"}, {"2shb", "2SHB"},
	}
	for _, c := range cases {
		if got := upper(c.in); got != c.want {
			t.Errorf("upper(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
