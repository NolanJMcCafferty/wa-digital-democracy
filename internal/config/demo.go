// Package config holds shared ingestion configuration types.
package config

import "fmt"

// SelectedDemo identifies a bill + agenda item selected from normalized DB
// state. It is reconstructed from Postgres for routine ingestion and page
// assembly; there is no operator-edited selected-demo YAML path anymore.
type SelectedDemo struct {
	Biennium   string `yaml:"biennium"`
	BillPrefix string `yaml:"bill_prefix"`
	BillNumber int    `yaml:"bill_number"`
	Chamber    string `yaml:"chamber"`

	Committee struct {
		Acronym string `yaml:"acronym"`
		CSIID   string `yaml:"csi_id"`
	} `yaml:"committee"`

	Agenda struct {
		CSIMeetingFamilyID    string `yaml:"csi_meeting_family_id"`
		CSIAgendaItemFamilyID string `yaml:"csi_agenda_item_family_id"`
		CSIAgendaItemID       string `yaml:"csi_agenda_item_id"`
		Label                 string `yaml:"label"`
	} `yaml:"agenda"`

	TVW struct {
		EventID string `yaml:"event_id"`
	} `yaml:"tvw"`

	TranscriptOverride struct {
		StartMS int `yaml:"start_ms"`
		EndMS   int `yaml:"end_ms"`
	} `yaml:"transcript_override"`
}

// BillID returns "HB 1234"-style identifier.
func (d SelectedDemo) BillID() string {
	if d.BillPrefix == "" || d.BillNumber == 0 {
		return ""
	}
	return fmt.Sprintf("%s %d", d.BillPrefix, d.BillNumber)
}
