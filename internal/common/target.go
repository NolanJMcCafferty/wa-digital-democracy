// Package common holds small shared civic value objects used across packages.
package common

import "fmt"

// BillKey identifies one bill within a biennium.
type BillKey struct {
	Biennium string
	Prefix   string
	Number   int
}

// ID returns the canonical display identifier, e.g. "HB 1234".
func (b BillKey) ID() string {
	if b.Prefix == "" || b.Number == 0 {
		return ""
	}
	return fmt.Sprintf("%s %d", b.Prefix, b.Number)
}

// CommitteeRef identifies the committee context for an agenda target.
type CommitteeRef struct {
	Chamber string
	Acronym string
	CSIID   string
}

// AgendaItemRef identifies one CSI agenda item within a committee meeting.
type AgendaItemRef struct {
	CSIMeetingFamilyID    string
	CSIAgendaItemFamilyID string
	CSIAgendaItemID       string
	Label                 string
}

// TVWRef identifies TVW/Invintus media associated with a hearing.
type TVWRef struct {
	EventID string
}

// TranscriptWindow optionally overrides detected bill-discussion bounds.
type TranscriptWindow struct {
	StartMS int
	EndMS   int
}

// BillAgendaTarget is the normalized target for ingesting or rendering one
// bill agenda item in one hearing.
type BillAgendaTarget struct {
	Bill               BillKey
	Committee          CommitteeRef
	AgendaItem         AgendaItemRef
	TVW                TVWRef
	TranscriptOverride TranscriptWindow
}
