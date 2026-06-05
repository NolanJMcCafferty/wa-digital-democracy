package jobs

import "testing"

func TestDetectBillDiscussionWindows_SplitsMultipleAgendaItems(t *testing.T) {
	cues := []segmentCue{
		{StartMS: 0, EndMS: 1000, Text: "Welcome to the committee."},
		{StartMS: 10_000, EndMS: 11_000, Text: "We will now hear House Bill 1234."},
		{StartMS: 20_000, EndMS: 21_000, Text: "Public testimony on HB 1234 is underway."},
		{StartMS: 30_000, EndMS: 31_000, Text: "That closes the public hearing on House Bill 1234."},
		{StartMS: 40_000, EndMS: 41_000, Text: "Next item is Senate Bill 7777."},
		{StartMS: 500_000, EndMS: 501_000, Text: "Returning to HB 1234 for executive session."},
		{StartMS: 510_000, EndMS: 511_000, Text: "Questions on House Bill 1234?"},
	}

	got := DetectBillDiscussionWindows(cues, "HB", 1234)
	if len(got) != 2 {
		t.Fatalf("windows = %#v, want 2", got)
	}
	if got[0].StartMS != 0 || got[0].EndMS != 41_000 || got[0].Mentions != 3 {
		t.Fatalf("first window = %#v, want start 0 end 41000 mentions 3", got[0])
	}
	if got[1].StartMS != 440_000 || got[1].EndMS != 571_000 || got[1].Mentions != 2 {
		t.Fatalf("second window = %#v, want start 440000 end 571000 mentions 2", got[1])
	}
}

func TestDetectBillDiscussionWindows_MergesNearbyMentions(t *testing.T) {
	cues := []segmentCue{
		{StartMS: 100_000, EndMS: 101_000, Text: "Opening HB 1501."},
		{StartMS: 130_000, EndMS: 131_000, Text: "More testimony on House Bill 1501."},
	}

	got := DetectBillDiscussionWindows(cues, "HB", 1501)
	if len(got) != 1 {
		t.Fatalf("windows = %#v, want 1", got)
	}
	if got[0].StartMS != 40_000 || got[0].EndMS != 191_000 || got[0].Mentions != 2 {
		t.Fatalf("window = %#v, want merged padded span", got[0])
	}
}

func TestDetectBillDiscussionWindows_NoMentions(t *testing.T) {
	got := DetectBillDiscussionWindows([]segmentCue{{StartMS: 1, EndMS: 2, Text: "No bill here."}}, "HB", 9999)
	if len(got) != 0 {
		t.Fatalf("windows = %#v, want none", got)
	}
}

func TestBillDiscussionMentionPattern_PrefixCoverage(t *testing.T) {
	// All bare prefixes, all engrossment/substitution chrome forms, and
	// the corresponding spoken English variants. Each case must match
	// for the bare prefix the bill row carries — that's the contract
	// SegmentTranscript relies on after the LWS-prefix normalizer.
	cases := []struct {
		bare   string
		number int
		text   string
	}{
		// HB / House Bill
		{"HB", 1501, "we will now hear HB 1501"},
		{"HB", 1501, "HB1501 is up next"},
		{"HB", 1501, "house bill 1501 is open"},
		{"HB", 1501, "Engrossed Substitute House Bill 1501"},
		{"HB", 1501, "ESHB 1501"},
		{"HB", 1859, "Second Substitute House Bill 1859"},
		{"HB", 1859, "2SHB 1859"},
		{"HB", 1859, "Engrossed Second Substitute House Bill 1859"},
		{"HB", 1859, "E2SHB 1859"},
		{"HB", 2354, "Substitute House Bill 2354"},
		{"HB", 2354, "SHB 2354"},

		// SB / Senate Bill
		{"SB", 6054, "Engrossed Senate Bill 6054"},
		{"SB", 6054, "ESB 6054"},
		{"SB", 6200, "Engrossed Substitute Senate Bill 6200"},
		{"SB", 6200, "ESSB 6200"},

		// HJR / House Joint Resolution — previously fell through to
		// the bare-prefix branch with no spoken-form coverage.
		{"HJR", 4002, "House Joint Resolution 4002"},
		{"HJR", 4002, "HJR 4002"},
		{"HJR", 4002, "Substitute House Joint Resolution 4002"},

		// HCR / House Concurrent Resolution — entirely new coverage.
		{"HCR", 4400, "House Concurrent Resolution 4400"},
		{"HCR", 4400, "HCR 4400"},

		// SJM / Senate Joint Memorial — entirely new coverage.
		{"SJM", 8001, "Senate Joint Memorial 8001"},
		{"SJM", 8001, "SJM 8001"},
	}
	for _, c := range cases {
		cues := []segmentCue{{StartMS: 0, EndMS: 1000, Text: c.text}}
		got := DetectBillDiscussionWindows(cues, c.bare, c.number)
		if len(got) != 1 || got[0].Mentions != 1 {
			t.Errorf("DetectBillDiscussionWindows(%q, %q, %d) = %#v, want 1 window with 1 mention",
				c.text, c.bare, c.number, got)
		}
	}
}

func TestBillDiscussionMentionPattern_DoesNotOvermatch(t *testing.T) {
	// Should NOT match: wrong number, different bill, near-misses.
	cases := []struct {
		bare   string
		number int
		text   string
	}{
		{"HB", 1501, "HB 1502 is on the docket"},       // different number
		{"HB", 1501, "in 1501 there was a discussion"}, // bare digits, no prefix
		{"HB", 1501, "we are on SB 1501"},              // different chamber
		{"HCR", 4400, "house bill 4400"},               // wrong long form
	}
	for _, c := range cases {
		cues := []segmentCue{{StartMS: 0, EndMS: 1000, Text: c.text}}
		got := DetectBillDiscussionWindows(cues, c.bare, c.number)
		if len(got) != 0 {
			t.Errorf("DetectBillDiscussionWindows(%q, %q, %d) = %#v, want no windows",
				c.text, c.bare, c.number, got)
		}
	}
}
