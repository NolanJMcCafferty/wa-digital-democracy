package common

import "testing"

func TestBillKeyID(t *testing.T) {
	cases := []struct {
		name string
		in   BillKey
		want string
	}{
		{"bill", BillKey{Biennium: "2025-26", Prefix: "HB", Number: 1234}, "HB 1234"},
		{"missing prefix", BillKey{Number: 1234}, ""},
		{"missing number", BillKey{Prefix: "SB"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.in.ID(); got != c.want {
				t.Fatalf("ID() = %q, want %q", got, c.want)
			}
		})
	}
}
