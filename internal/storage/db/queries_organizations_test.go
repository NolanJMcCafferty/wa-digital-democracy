package db

import "testing"

func TestJunkOrganizationName(t *testing.T) {
	cases := []struct {
		name             string
		raw, normalized  string
		junk             bool
	}{
		{"empty raw", "", "anything", true},
		{"empty normalized", "Foo", "", true},
		{"too short normalized", "Hi", "HI", true},
		{"exact self-descriptor", "self", "SELF", true},
		{"exact none", "N/A", "N/A", true},
		{"hashtag", "#NotABot", "NOTABOT", true},
		{"wrapped parens", "(Retired)", "RETIRED", true},
		{"wrapped brackets", "[Self]", "SELF", true},
		{"wrapped quotes", `"Home"`, "HOME", true},
		{"placeholder please select", "Please select", "PLEASE SELECT", true},
		{"placeholder enter your", "enter your name", "ENTER YOUR NAME", true},
		{"all symbols", "###!", "###!", true},
		{"mostly digits", "1234567", "1234567", true},
		{"real org", "Sierra Club", "SIERRA CLUB", false},
		{"real org with letters", "ACLU of Washington", "ACLU OF WASHINGTON", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := junkOrganizationName(c.raw, c.normalized); got != c.junk {
				t.Errorf("junkOrganizationName(%q, %q) = %v, want %v", c.raw, c.normalized, got, c.junk)
			}
			if got := JunkOrganizationName(c.raw, c.normalized); got != c.junk {
				t.Errorf("JunkOrganizationName mismatch with internal helper")
			}
		})
	}
}
