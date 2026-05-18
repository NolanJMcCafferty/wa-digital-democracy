package main

import (
	"errors"
	"testing"
)

func TestNonfatalDiscoveryStatus(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status string
		ok     bool
	}{
		{
			name:   "missing CSI meeting",
			err:    errors.New("meeting: no CSI meeting within 15m0s of 2026-02-25T08:00:00-08:00"),
			status: "no-csi-meeting",
			ok:     true,
		},
		{
			name:   "missing agenda item",
			err:    errors.New("agenda item: no agenda item for bill number 6302 in meeting 34090"),
			status: "no-agenda-item",
			ok:     true,
		},
		{
			name:   "missing committee",
			err:    errors.New(`committee: no CSI committee match for Senate "Example Committee"`),
			status: "no-committee",
			ok:     true,
		},
		{
			name: "unexpected agenda client error",
			err:  errors.New("agenda item: GetAgendaItems: 502 Bad Gateway"),
		},
		{
			name: "nil",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, ok := nonfatalDiscoveryStatus(c.err)
			if status != c.status || ok != c.ok {
				t.Fatalf("nonfatalDiscoveryStatus(%v) = (%q, %v), want (%q, %v)", c.err, status, ok, c.status, c.ok)
			}
		})
	}
}
