package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSelectedDemo_OK(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.yml")
	if err := os.WriteFile(path, []byte(`
biennium: "2025-26"
bill_prefix: "SB"
bill_number: 6200
chamber: "House"
committee:
  acronym: "HOUS"
  csi_id: "31633"
agenda:
  csi_meeting_family_id: "34051"
  csi_agenda_item_family_id: "171165"
  csi_agenda_item_id: "28474"
  label: "ESSB 6200 Tenant cooling devices"
tvw:
  event_id: "2026022014"
transcript_override:
  start_ms: 0
  end_ms: 0
`), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := LoadSelectedDemo(path)
	if err != nil {
		t.Fatalf("LoadSelectedDemo: %v", err)
	}
	if d.BillID() != "SB 6200" {
		t.Errorf("BillID = %q", d.BillID())
	}
	if d.Agenda.CSIAgendaItemID != "28474" {
		t.Errorf("CSIAgendaItemID = %q", d.Agenda.CSIAgendaItemID)
	}
}

func TestLoadSelectedDemo_MissingFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.yml")
	os.WriteFile(path, []byte(`biennium: "2025-26"`), 0o644)
	_, err := LoadSelectedDemo(path)
	if err == nil {
		t.Fatal("expected error")
	}
}
