package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestBillPageHandler_BadSlug exercises the slug-parse path without
// touching Postgres — the regex check happens before any DB call, so a
// nil store is unreachable and a real *db.Store isn't required.
func TestBillPageHandler_BadSlug(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/api/v1/bills/{biennium}/{billNumber}/page", billPageHandler(nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bills/2025-26/notaslug/page", nil)
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

func TestLookupLegislatorsByAddressHandler_MissingAddress(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/api/v1/legislators/lookup", lookupLegislatorsByAddressHandler(nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/legislators/lookup?address=", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "address is required") {
		t.Errorf("body = %q, want address error", w.Body.String())
	}
}

func TestSuggestAddressesHandler_ShortQuery(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/api/v1/addresses/suggest", suggestAddressesHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/addresses/suggest?query=105", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"suggestions":[]`) {
		t.Errorf("body = %q, want empty suggestions array", w.Body.String())
	}
}

func TestWashingtonAddressQueryVariants(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"1052 E", []string{"1052 E", "1052 E WA"}},
		{"1052 E Seattle WA", []string{"1052 E Seattle WA"}},
		{"1052 E Washington Ave", []string{"1052 E Washington Ave"}},
	}
	for _, c := range cases {
		got := washingtonAddressQueryVariants(c.in)
		if len(got) != len(c.want) {
			t.Fatalf("washingtonAddressQueryVariants(%q) = %#v, want %#v", c.in, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("washingtonAddressQueryVariants(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

func TestNormalizeDistrict(t *testing.T) {
	cases := []struct{ in, want string }{
		{"34", "34"},
		{"District 034", "34"},
		{"LD-07", "7"},
		{"", ""},
	}
	for _, c := range cases {
		if got := normalizeDistrict(c.in); got != c.want {
			t.Errorf("normalizeDistrict(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestLegislatorRole(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Senate", "State Senator"},
		{"House", "State Representative"},
		{"", "Legislator"},
	}
	for _, c := range cases {
		if got := legislatorRole(c.in); got != c.want {
			t.Errorf("legislatorRole(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDiarizedSegmentPublicSpeakerLabel(t *testing.T) {
	cases := []struct {
		name string
		seg  diarizedSegmentResponse
		want string
	}{
		{
			name: "reviewed assignment",
			seg:  diarizedSegmentResponse{ClusterLabel: "SPEAKER_01", SpeakerLabel: "Jane Smith", Reviewed: true},
			want: "Jane Smith",
		},
		{
			name: "unreviewed cluster stays anonymous",
			seg:  diarizedSegmentResponse{ClusterLabel: "SPEAKER_02", Reviewed: false},
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.seg.PublicSpeakerLabel(); got != c.want {
				t.Fatalf("PublicSpeakerLabel() = %q, want %q", got, c.want)
			}
		})
	}
}
