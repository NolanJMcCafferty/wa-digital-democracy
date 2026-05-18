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

// TestSlugify_Unit covers the same cases as the integration-tagged test
// but doesn't require the DB. Slugify is the contract between the API and
// the frontend's <Link> hrefs, so it deserves a fast, build-tag-free test.
func TestSlugify_Unit(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Representative Reed", "representative-reed"},
		{"Senator C. Wilson", "senator-c-wilson"},
		{"H2 Government Relations", "h2-government-relations"},
		{"Washington Multi-Family Housing Association", "washington-multi-family-housing-association"},
		{"Cut Spending / No New Taxes", "cut-spending-no-new-taxes"},
		{"AT&T", "at-and-t"},
		{"  trim  spaces  ", "trim-spaces"},
		{"", ""},
	}
	for _, c := range cases {
		if got := slugify(c.in); got != c.want {
			t.Errorf("slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
