package main

import "testing"

func TestNormalizeSpokenName(t *testing.T) {
	cases := map[string]string{
		"Strege, Neil":                "neil strege",
		"Strege, Neil, Jr.":           "neil strege",
		"Dr. Neil Strege":             "neil strege",
		"Representative Jane Doe":     "jane doe",
		"Jane-Marie O’Neil":           "jane marie o neil",
		"  Sen. María Cantwell III  ": "maría cantwell",
		"Cockburn, James":             "james cockburn",
		"McAleenan, Mellani":          "mellani mcaleenan",
		"Chair Vice":                  "",
	}
	for in, want := range cases {
		if got := normalizeSpokenName(in); got != want {
			t.Errorf("normalizeSpokenName(%q) = %q, want %q", in, got, want)
		}
	}
}
