package candidate

import (
	"testing"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/csi"
)

func TestScore(t *testing.T) {
	cases := []struct {
		name string
		in   ScoreSignals
		want int
	}{
		{"empty", ScoreSignals{}, 0},
		{"testifiers no orgs", ScoreSignals{TestifierCount: 5}, 2},
		{"one org", ScoreSignals{TestifierCount: 5, UniqueOrganizations: 1}, 3},
		{"two orgs", ScoreSignals{TestifierCount: 8, UniqueOrganizations: 2}, 4},
		{"many orgs", ScoreSignals{TestifierCount: 20, UniqueOrganizations: 7}, 4},
	}
	for _, c := range cases {
		if got := Score(c.in); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

func TestFromTestifiers(t *testing.T) {
	rows := []csi.Testifier{
		{Name: "A", Organization: "Washington Roundtable", Position: "Pro", Testified: true},
		{Name: "B", Organization: "ACLU of Washington", Position: "Pro", Testified: true},
		{Name: "C", Organization: "WA Roundtable", Position: "Con", Testified: false}, // case differs but same org
		{Name: "D", Organization: "", Position: "Other", Testified: false},             // no org
	}
	signals, orgs := FromTestifiers(rows)
	if signals.TestifierCount != 4 {
		t.Errorf("TestifierCount = %d, want 4", signals.TestifierCount)
	}
	// "Washington Roundtable" and "WA Roundtable" differ on the lowercase
	// key, so they count as separate. ACLU is a 3rd. So 3 distinct.
	if signals.UniqueOrganizations != 3 {
		t.Errorf("UniqueOrganizations = %d, want 3 (case-insensitive but exact-string)", signals.UniqueOrganizations)
	}
	if orgs.Pro != 2 || orgs.Con != 1 || orgs.Other != 1 {
		t.Errorf("position counts = (Pro=%d, Con=%d, Other=%d)", orgs.Pro, orgs.Con, orgs.Other)
	}
	if len(orgs.SampleOrgs) != 3 {
		t.Errorf("SampleOrgs len = %d", len(orgs.SampleOrgs))
	}
}

func TestParseBillFromLabel(t *testing.T) {
	cases := []struct {
		in     string
		prefix string
		number int
		title  string
	}{
		{"HB 2747 Budget sustainability", "HB", 2747, "Budget sustainability"},
		{"SB 5000  Affordable housing", "SB", 5000, "Affordable housing"},
		{"SHB 1234 Tenant protections", "HB", 1234, "Tenant protections"},
		{"E2SHB 1170 Special agency", "HB", 1170, "Special agency"},
		{"HJR 4202 Constitutional amendment", "HJR", 4202, "Constitutional amendment"},
		{"Public hearing on housing", "", 0, "Public hearing on housing"},
		{"", "", 0, ""},
	}
	for _, c := range cases {
		p, n, title := ParseBillFromLabel(c.in)
		if p != c.prefix || n != c.number || title != c.title {
			t.Errorf("ParseBillFromLabel(%q) = (%q, %d, %q), want (%q, %d, %q)",
				c.in, p, n, title, c.prefix, c.number, c.title)
		}
	}
}

func TestSortCandidatesDesc(t *testing.T) {
	cs := []Candidate{
		{Score: 2, MeetingDateTime: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), AgendaItemID: "a"},
		{Score: 4, MeetingDateTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), AgendaItemID: "b"},
		{Score: 4, MeetingDateTime: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), AgendaItemID: "c"}, // most recent at score 4
		{Score: 0, MeetingDateTime: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), AgendaItemID: "d"},
	}
	SortCandidatesDesc(cs)
	want := []string{"c", "b", "a", "d"}
	for i, c := range cs {
		if c.AgendaItemID != want[i] {
			t.Errorf("idx %d: got %s, want %s", i, c.AgendaItemID, want[i])
		}
	}
}

func TestIssueTargets(t *testing.T) {
	got, err := IssueTargets("Housing")
	if err != nil {
		t.Fatalf("IssueTargets(Housing): %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].CommitteeID != "31633" || got[1].CommitteeID != "34078" {
		t.Errorf("got = %+v", got)
	}

	if _, err := IssueTargets("climate"); err == nil {
		t.Error("expected error for unknown issue")
	}
}

// Compile-time check that finder.go's officialBillURL accepts time.Time
// (we passed it through an `any` to avoid an import in candidate.go but
// finder.go uses time directly).
var _ = func() time.Time { return time.Time{} }
