package jobs

import "testing"

func TestStripChamberPrefix(t *testing.T) {
	cases := []struct {
		chamber, in, want string
	}{
		{"Senate", "Senate Housing", "Housing"},
		{"House", "House Housing", "Housing"},
		{"Senate", "Senate Ways & Means", "Ways & Means"},
		{"Joint", "Joint Transportation", "Transportation"},
		{"Senate", "Housing", "Housing"},       // already bare
		{"House", "Senate Housing", "Housing"}, // wrong chamber prefix still stripped
		{"Senate", "", ""},
	}
	for _, c := range cases {
		if got := stripChamberPrefix(c.chamber, c.in); got != c.want {
			t.Errorf("stripChamberPrefix(%q, %q) = %q, want %q", c.chamber, c.in, got, c.want)
		}
	}
}

func TestNormalizeCommitteeName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Housing", "housing"},
		{"Ways & Means", "ways and means"},
		{"Ways and Means", "ways and means"},
		{"State Government, Tribal Affairs & Elections", "state government tribal affairs and elections"},
		{"  Local  Government  ", "local government"},
		{"K-12 Education", "k 12 education"},
		{"", ""},
	}
	for _, c := range cases {
		if got := normalizeCommitteeName(c.in); got != c.want {
			t.Errorf("normalizeCommitteeName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBillNumberInLabel(t *testing.T) {
	// matchAgendaItem extracts the leading bill number from the
	// agenda item's label and compares it against bill.number.
	cases := []struct {
		label string
		want  string // first capture group; "" if no match
	}{
		{"EHB 1501 CIC unit owner inquiries", "1501"},
		{"HB 2664 Unlawful detainer notices", "2664"},
		{"2SHB 1859 Housing dev./religious orgs.", "1859"},
		{"ESSB 6054 Wildfire home hardening/CICs", "6054"},
		{"SB 6200 Tenant cooling devices", "6200"},
		{"SHB 2354 Common interest communities", "2354"},
		{"SGA 9280 AARON T. MCGRATH", "9280"},
		{"Just words", ""},
	}
	for _, c := range cases {
		m := billNumberInLabel.FindStringSubmatch(c.label)
		var got string
		if len(m) >= 2 {
			got = m[1]
		}
		if got != c.want {
			t.Errorf("billNumberInLabel(%q) = %q, want %q", c.label, got, c.want)
		}
	}
}
