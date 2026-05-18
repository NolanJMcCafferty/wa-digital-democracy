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
