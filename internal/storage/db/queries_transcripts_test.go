package db

import "testing"

func TestBillMentionPattern(t *testing.T) {
	cases := []struct {
		prefix string
		num    int
		want   string
	}{
		{"HB", 1234, `\m(HB|house bill)\s*1234\M`},
		{"SB", 5001, `\m(SB|senate bill)\s*5001\M`},
		{"HJR", 1, `\m(HJR|house joint resolution)\s*1\M`},
		{"SJR", 9, `\m(SJR|senate joint resolution)\s*9\M`},
		// Unknown prefix → no chamber-word alternation.
		{"ESHB", 100, `\mESHB\s*100\M`},
	}
	for _, c := range cases {
		if got := billMentionPattern(c.prefix, c.num); got != c.want {
			t.Errorf("billMentionPattern(%q,%d) = %q, want %q", c.prefix, c.num, got, c.want)
		}
	}
}
