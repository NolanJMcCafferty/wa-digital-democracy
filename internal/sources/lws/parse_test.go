package lws

import (
	"errors"
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

func TestBaseBillPrefix(t *testing.T) {
	cases := []struct{ in, want string }{
		{"HB", "HB"},
		{"SB", "SB"},
		{"SHB", "HB"},
		{"2SHB", "HB"},
		{"3SHB", "HB"},
		{"ESHB", "HB"},
		{"E2SHB", "HB"},
		{"SSB", "SB"},
		{"2SSB", "SB"},
		{"ESSB", "SB"},
		{"E2SSB", "SB"},
		{"HJR", "HJR"},
		{"ESHJR", "HJR"},
		{"WTF", "WTF"}, // unknown shape preserved
	}
	for _, c := range cases {
		if got := baseBillPrefix(c.in); got != c.want {
			t.Errorf("baseBillPrefix(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseGetLegislation_Real(t *testing.T) {
	leg, err := ParseGetLegislation(read(t, "get-legislation.xml"))
	if err != nil {
		t.Fatalf("ParseGetLegislation: %v", err)
	}
	if leg.BillID != "HB 1234" {
		t.Errorf("BillID = %q", leg.BillID)
	}
	if leg.Biennium != "2025-26" {
		t.Errorf("Biennium = %q", leg.Biennium)
	}
	if leg.OriginalAgency != "House" {
		t.Errorf("OriginalAgency = %q", leg.OriginalAgency)
	}
	if leg.PrimeSponsorID != "31527" {
		t.Errorf("PrimeSponsorID = %q", leg.PrimeSponsorID)
	}
	if leg.CurrentStatus == nil {
		t.Fatal("CurrentStatus nil")
	}
	if leg.CurrentStatus.HistoryLine == "" {
		t.Errorf("HistoryLine empty")
	}
}

func TestParseGetCurrentStatus_Real(t *testing.T) {
	cs, err := ParseGetCurrentStatus(read(t, "get-current-status.xml"))
	if err != nil {
		t.Fatalf("ParseGetCurrentStatus: %v", err)
	}
	if cs.BillID != "HB 1234" {
		t.Errorf("BillID = %q", cs.BillID)
	}
	if cs.HistoryLine == "" {
		t.Errorf("HistoryLine empty")
	}
}

func TestParseSponsors_Real(t *testing.T) {
	sps, err := ParseSponsors(read(t, "get-sponsors.xml"))
	if err != nil {
		t.Fatalf("ParseSponsors: %v", err)
	}
	if len(sps) < 2 {
		t.Fatalf("len = %d, want >= 2", len(sps))
	}
	if sps[0].Type != "Primary" {
		t.Errorf("sps[0].Type = %q", sps[0].Type)
	}
	if sps[0].LongName == "" {
		t.Errorf("sps[0].LongName empty")
	}
}

func TestParseHearings_Real(t *testing.T) {
	hs, err := ParseHearings(read(t, "get-hearings.xml"))
	if err != nil {
		t.Fatalf("ParseHearings: %v", err)
	}
	if len(hs) < 2 {
		t.Fatalf("len = %d, want >= 2", len(hs))
	}
	first := hs[0]
	if first.BillID != "HB 1234" {
		t.Errorf("first.BillID = %q", first.BillID)
	}
	if first.HearingType == "" {
		t.Errorf("first.HearingType empty")
	}
	if len(first.CommitteeMeeting.Committees) == 0 {
		t.Fatal("no committees")
	}
	c := first.CommitteeMeeting.Committees[0]
	if c.Acronym == "" || c.LongName == "" {
		t.Errorf("committee = %+v", c)
	}
}

func TestParseSOAPFault(t *testing.T) {
	_, err := ParseSponsors(read(t, "soap-fault.xml"))
	var fault SOAPFault
	if !errors.As(err, &fault) {
		t.Fatalf("err = %v, want SOAPFault", err)
	}
	if fault.Code == "" || fault.Message == "" {
		t.Errorf("fault = %+v", fault)
	}
}

func TestNormalizeBill(t *testing.T) {
	leg, _ := ParseGetLegislation(read(t, "get-legislation.xml"))
	got := NormalizeBill(leg)
	if got.Prefix != "HB" || got.Number != 1234 {
		t.Errorf("Prefix/Number = (%q, %d)", got.Prefix, got.Number)
	}
	if got.OfficialURL != "https://app.leg.wa.gov/billsummary?BillNumber=1234&Year=2025" {
		t.Errorf("OfficialURL = %q", got.OfficialURL)
	}
	if got.Title == "" || got.Description == "" {
		t.Errorf("Title/Description empty")
	}
	if got.StatusDate.IsZero() {
		t.Errorf("StatusDate zero")
	}
}

func TestNormalizeSponsors(t *testing.T) {
	sps, _ := ParseSponsors(read(t, "get-sponsors.xml"))
	out := NormalizeSponsors(sps)
	if len(out) == 0 {
		t.Fatal("no sponsors")
	}
	if out[0].SponsorType != "Primary" {
		t.Errorf("[0].SponsorType = %q", out[0].SponsorType)
	}
	if out[0].LWSSponsorID == "" {
		t.Errorf("[0].LWSSponsorID empty")
	}
}

func TestNormalizeHearings(t *testing.T) {
	hs, _ := ParseHearings(read(t, "get-hearings.xml"))
	out := NormalizeHearings(hs)
	if len(out) < 2 {
		t.Fatalf("len = %d, want >= 2", len(out))
	}
	if out[0].LWSMeetingID == "" {
		t.Errorf("[0].LWSMeetingID empty (want LWS AgendaId)")
	}
	if out[0].MeetingDateTime.IsZero() {
		t.Errorf("[0].MeetingDateTime zero")
	}
	if out[0].CommitteeAcronym == "" {
		t.Errorf("[0].CommitteeAcronym empty")
	}
}

func TestParseLWSDate(t *testing.T) {
	cases := []struct {
		in   string
		zero bool
	}{
		{"2025-01-13T00:00:00", false},
		{"2025-01-16T14:02:35.103", false},
		{"", true},
		{"garbage", true},
	}
	for _, c := range cases {
		got, _ := parseLWSDate(c.in)
		if c.zero && !got.IsZero() {
			t.Errorf("%q: want zero, got %v", c.in, got)
		}
		if !c.zero && got.IsZero() {
			t.Errorf("%q: want non-zero", c.in)
		}
	}
	// Sanity-check one specific time. LWS emits Pacific wall-clock without
	// an explicit zone — we parse in America/Los_Angeles.
	loc, _ := time.LoadLocation("America/Los_Angeles")
	t1, _ := parseLWSDate("2025-01-13T00:00:00")
	want := time.Date(2025, 1, 13, 0, 0, 0, 0, loc)
	if !t1.Equal(want) {
		t.Errorf("got %v, want %v", t1, want)
	}
}

func TestParseSenateSponsors(t *testing.T) {
	ms, err := ParseSenateSponsors(read(t, "get-senate-sponsors.xml"))
	if err != nil {
		t.Fatalf("ParseSenateSponsors: %v", err)
	}
	if len(ms) < 40 {
		t.Errorf("got %d senators, expected near full chamber", len(ms))
	}
	first := ms[0]
	if first.ID == "" || first.Name == "" || first.Agency != "Senate" {
		t.Errorf("first member malformed: %+v", first)
	}
	if first.Party == "" || first.District == "" || first.Email == "" {
		t.Errorf("expected party/district/email populated; got %+v", first)
	}
}

func TestParseHouseSponsors(t *testing.T) {
	ms, err := ParseHouseSponsors(read(t, "get-house-sponsors.xml"))
	if err != nil {
		t.Fatalf("ParseHouseSponsors: %v", err)
	}
	if len(ms) < 90 {
		t.Errorf("got %d house members, expected near full chamber", len(ms))
	}
	first := ms[0]
	if first.Agency != "House" {
		t.Errorf("first member should have Agency=House: %+v", first)
	}
}
