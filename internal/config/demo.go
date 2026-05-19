// Package config loads operator-edited YAML configs that drive Phase 4.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// SelectedDemo mirrors config/selected_demo.yml. The operator picks a
// candidate from `wa-dd find-candidates` and pastes its IDs here.
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

// LoadSelectedDemo reads and validates a selected_demo.yml.
func LoadSelectedDemo(path string) (*SelectedDemo, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read demo config: %w", err)
	}
	var d SelectedDemo
	if err := yaml.Unmarshal(body, &d); err != nil {
		return nil, fmt.Errorf("parse demo config: %w", err)
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}
	return &d, nil
}

// Validate checks that required fields are present.
func (d SelectedDemo) Validate() error {
	var missing []string
	if d.Biennium == "" {
		missing = append(missing, "biennium")
	}
	if d.BillPrefix == "" {
		missing = append(missing, "bill_prefix")
	}
	if d.BillNumber == 0 {
		missing = append(missing, "bill_number")
	}
	if d.Chamber == "" {
		missing = append(missing, "chamber")
	}
	if d.Agenda.CSIAgendaItemID == "" {
		missing = append(missing, "agenda.csi_agenda_item_id")
	}
	if d.Agenda.CSIMeetingFamilyID == "" {
		missing = append(missing, "agenda.csi_meeting_family_id")
	}
	if d.Agenda.Label == "" {
		missing = append(missing, "agenda.label")
	}
	// committee.csi_id is required for re-fetching meetings/agenda IDs from CSI.
	if d.Committee.CSIID == "" {
		missing = append(missing, "committee.csi_id")
	}
	// tvw.event_id is the join key for video/transcript ingestion.
	if d.TVW.EventID == "" {
		missing = append(missing, "tvw.event_id")
	}
	if len(missing) > 0 {
		return fmt.Errorf("selected_demo.yml missing required fields: %v", missing)
	}
	return nil
}
