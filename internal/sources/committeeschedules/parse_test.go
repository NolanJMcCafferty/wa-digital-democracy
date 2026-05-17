package committeeschedules

import (
	"os"
	"path/filepath"
	"testing"
)

// search-default.html is a real captured response from
// POST /committeeschedules/Home/Search/ on 2026-05-15. Phase 0 research
// observed 4 agenda meetings, 3 video modals, and 3 unique TVW event IDs
// (2026051107, 2026051115, 2026051116) — see docs/phase0-spike-report.md.
func read(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return b
}

func TestParseSearchResults_RealCapture(t *testing.T) {
	rows, err := ParseSearchResults(read(t, "search-default.html"))
	if err != nil {
		t.Fatalf("ParseSearchResults: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want 4", len(rows))
	}
	withTVW := 0
	tvwIDs := map[string]bool{}
	for _, r := range rows {
		if r.TVWEventID != "" {
			withTVW++
			tvwIDs[r.TVWEventID] = true
		}
		if r.AgendaID == "" {
			t.Errorf("row missing AgendaID: %+v", r)
		}
	}
	if withTVW != 3 {
		t.Errorf("rows-with-TVW = %d, want 3", withTVW)
	}
	for _, want := range []string{"2026051107", "2026051115", "2026051116"} {
		if !tvwIDs[want] {
			t.Errorf("missing TVW event ID %s", want)
		}
	}
}

func TestParseVideoMappings(t *testing.T) {
	body := []byte(`
<a onclick="showVideoModal(34154, 2026051107)">vid</a>
<a onclick="showVideoModal('34155','2026051108')">vid</a>
`)
	pairs := ParseVideoMappings(body)
	if len(pairs) != 2 {
		t.Fatalf("len = %d, want 2", len(pairs))
	}
	if pairs[0].VideoID != "34154" || pairs[0].TVWEventID != "2026051107" {
		t.Errorf("pair[0] = %+v", pairs[0])
	}
	if pairs[1].VideoID != "34155" || pairs[1].TVWEventID != "2026051108" {
		t.Errorf("pair[1] = %+v", pairs[1])
	}
}

func TestParseSearchResults_AgendaThenVideoOrdering(t *testing.T) {
	body := []byte(`
showAgendaDetailModal(1)
showVideoModal(1, 100)
showAgendaDetailModal(2)
showAgendaDetailModal(3)
showVideoModal(3, 300)
`)
	rows, err := ParseSearchResults(body)
	if err != nil {
		t.Fatalf("ParseSearchResults: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	if rows[0].AgendaID != "1" || rows[0].TVWEventID != "100" {
		t.Errorf("rows[0] = %+v", rows[0])
	}
	if rows[1].AgendaID != "2" || rows[1].TVWEventID != "" {
		t.Errorf("rows[1] = %+v", rows[1])
	}
	if rows[2].AgendaID != "3" || rows[2].TVWEventID != "300" {
		t.Errorf("rows[2] = %+v", rows[2])
	}
}
