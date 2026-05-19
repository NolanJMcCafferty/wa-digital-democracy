package entitymatch

import "testing"

func TestNormalizedName(t *testing.T) {
	tests := map[string]string{
		"The Acme Technologies, Inc.":        "ACME TECHNOLOGIES",
		"ACME Tech & Consulting LLC":         "ACME TECH AND CONSULTING",
		"  Washington Housing Alliance  ":    "WASHINGTON HOUSING ALLIANCE",
		"Northwest Energy Association, Ltd.": "NORTHWEST ENERGY",
	}
	for in, want := range tests {
		if got := NormalizedName(in); got != want {
			t.Fatalf("NormalizedName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestConfidenceFor(t *testing.T) {
	tests := []struct {
		name      string
		canonical string
		aliases   []string
		want      string
	}{
		{name: "Acme Technologies", canonical: "Acme Technologies", want: ConfidenceConfirmed},
		{name: "ACME TECHNOLOGIES LLC", canonical: "Acme Technologies Inc.", want: ConfidenceProbable},
		{name: "Sunrise Integrated Solutions", canonical: "Sunrise Technologies", aliases: []string{"Sunrise Integrated Solutions LLC"}, want: ConfidenceConfirmed},
		{name: "Housing Alliance", canonical: "Washington Housing Alliance", want: ConfidencePossible},
		{name: "Alliance", canonical: "Washington Housing Alliance", want: ""},
	}
	for _, tc := range tests {
		got, evidence := ConfidenceFor(tc.name, tc.canonical, tc.aliases)
		if got != tc.want {
			t.Fatalf("ConfidenceFor(%q,%q,%v) = %q evidence=%v, want %q", tc.name, tc.canonical, tc.aliases, got, evidence, tc.want)
		}
		if got != "" && len(evidence) == 0 {
			t.Fatalf("ConfidenceFor(%q) returned no evidence", tc.name)
		}
	}
}

func TestFalsePositiveRisk(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "", want: true},
		{name: "LLC", want: true},
		{name: "STATE", want: true},
		{name: "A", want: true},
		{name: "ACME", want: true},
		{name: "ACME HOUSING", want: false},
	}
	for _, tc := range tests {
		if got := FalsePositiveRisk(tc.name); got != tc.want {
			t.Fatalf("FalsePositiveRisk(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}
