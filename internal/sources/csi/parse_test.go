package csi

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func read(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return b
}

func TestParseChamberCommittees(t *testing.T) {
	cs, err := ParseChamberCommittees(read(t, "chamber-house.html"))
	if err != nil {
		t.Fatalf("ParseChamberCommittees: %v", err)
	}
	if len(cs) != 4 {
		t.Fatalf("len = %d, want 4 (placeholder + decoy ignored)", len(cs))
	}
	want := map[string]string{
		"31649": "Agriculture & Natural Resources",
		"31634": "Appropriations",
		"31636": "Civil Rights & Judiciary",
		"31633": "Housing",
	}
	for _, c := range cs {
		if want[c.ID] != c.Name {
			t.Errorf("committee[%s].Name = %q, want %q", c.ID, c.Name, want[c.ID])
		}
	}
}

func TestParseMeetings(t *testing.T) {
	ms, err := ParseMeetings(read(t, "get-meetings.json"))
	if err != nil {
		t.Fatalf("ParseMeetings: %v", err)
	}
	if len(ms) != 3 {
		t.Fatalf("len = %d, want 3 (placeholder filtered)", len(ms))
	}
	if ms[0].MeetingFamilyID != "34109" {
		t.Errorf("MeetingFamilyID = %q", ms[0].MeetingFamilyID)
	}
	// "03/05/26 8:00 AM" is parsed as Pacific time (the WA Legislature is in
	// Olympia year-round). Compare on instant equality so the test is
	// timezone-agnostic.
	loc, _ := time.LoadLocation("America/Los_Angeles")
	want := time.Date(2026, 3, 5, 8, 0, 0, 0, loc)
	if !ms[0].StartDateTime.Equal(want) {
		t.Errorf("StartDateTime = %v, want %v", ms[0].StartDateTime, want)
	}
}

func TestParseAgendaItems(t *testing.T) {
	items, err := ParseAgendaItems(read(t, "get-agenda-items.html"))
	if err != nil {
		t.Fatalf("ParseAgendaItems: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len = %d, want 2", len(items))
	}
	first := items[0]
	if first.MeetingFamilyID != "34109" || first.AgendaItemFamilyID != "171540" || first.AgendaItemID != "28599" {
		t.Errorf("first IDs = (%s,%s,%s)", first.MeetingFamilyID, first.AgendaItemFamilyID, first.AgendaItemID)
	}
	if first.Label != "HB 2747 Budget sustainability" {
		t.Errorf("first label = %q", first.Label)
	}
	second := items[1]
	if second.AgendaItemID != "28600" {
		t.Errorf("second AgendaItemID = %q", second.AgendaItemID)
	}
	if second.Label != "HB 1234 Affordable housing & tenant protections" {
		t.Errorf("second label = %q", second.Label)
	}
}

func TestParseTestifiers(t *testing.T) {
	rows, err := ParseTestifiers(read(t, "get-other-testifiers.html"))
	if err != nil {
		t.Fatalf("ParseTestifiers: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("len = %d, want 3", len(rows))
	}
	// First two rows should be testifying=true.
	if !rows[0].Testified || !rows[1].Testified {
		t.Errorf("expected rows[0..1].Testified = true")
	}
	if rows[2].Testified {
		t.Errorf("expected rows[2].Testified = false")
	}
	if rows[0].Name != "Strege, Neil" || rows[0].Organization != "Washington Roundtable" || rows[0].Position != "Pro" {
		t.Errorf("rows[0] = %+v", rows[0])
	}
	if rows[2].Organization != "ACLU of Washington" {
		t.Errorf("rows[2].Organization = %q", rows[2].Organization)
	}
	loc, _ := time.LoadLocation("America/Los_Angeles")
	want0 := time.Date(2026, 3, 4, 14, 40, 15, 553000000, loc)
	if !rows[0].TimeSignedIn.Equal(want0) {
		t.Errorf("rows[0].TimeSignedIn = %v, want %v", rows[0].TimeSignedIn, want0)
	}
}

func TestParseTestifiers_Empty(t *testing.T) {
	rows, err := ParseTestifiers(read(t, "get-other-testifiers-empty.html"))
	if err != nil {
		t.Fatalf("ParseTestifiers (empty): %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}

func TestNormalize(t *testing.T) {
	in := Testifier{Name: "  Smith, Alice ", Organization: "ACLU", Position: "PRO", Testified: true}
	got := Normalize(in, "28599")
	if got.AgendaItemSourceKey != "28599" {
		t.Errorf("AgendaItemSourceKey = %q", got.AgendaItemSourceKey)
	}
	if got.Position != "Pro" {
		t.Errorf("Position = %q (canonicalization broken)", got.Position)
	}
	if got.RawName != "Smith, Alice" {
		t.Errorf("RawName not trimmed: %q", got.RawName)
	}
}

func TestCanonicalPosition(t *testing.T) {
	cases := map[string]string{
		"Pro": "Pro", "PRO": "Pro", "pro ": "Pro",
		"Con": "Con", "Other": "Other", "Unknown": "Unknown", "": "Unknown", "weird": "Unknown",
	}
	for in, want := range cases {
		if got := CanonicalPosition(in); got != want {
			t.Errorf("CanonicalPosition(%q) = %q, want %q", in, got, want)
		}
	}
}
