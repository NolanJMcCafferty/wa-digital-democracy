package config

import (
	"os"
	"path/filepath"
	"strings"
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

func TestLoadSelectedBills_OK(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bills.yml")
	if err := os.WriteFile(path, []byte(`
bills:
  - biennium: "2025-26"
    bill_prefix: "HB"
    bill_number: 1501
    chamber: "Senate"
    committee: { acronym: "HOUS", csi_id: "34078" }
    agenda:
      csi_meeting_family_id: "33828"
      csi_agenda_item_family_id: "169654"
      csi_agenda_item_id: "27885"
      label: "EHB 1501 CIC unit owner inquiries"
    tvw: { event_id: "2026021085" }
  - biennium: "2025-26"
    bill_prefix: "SB"
    bill_number: 6200
    chamber: "House"
    committee: { acronym: "HOUS", csi_id: "31633" }
    agenda:
      csi_meeting_family_id: "34051"
      csi_agenda_item_family_id: "171165"
      csi_agenda_item_id: "28474"
      label: "ESSB 6200 Tenant cooling devices"
    tvw: { event_id: "2026022014" }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := LoadSelectedBills(path)
	if err != nil {
		t.Fatalf("LoadSelectedBills: %v", err)
	}
	if len(b.Bills) != 2 {
		t.Fatalf("len(Bills) = %d, want 2", len(b.Bills))
	}
	if b.Bills[0].BillID() != "HB 1501" || b.Bills[1].BillID() != "SB 6200" {
		t.Errorf("BillIDs = %q, %q", b.Bills[0].BillID(), b.Bills[1].BillID())
	}
}

func TestLoadSelectedBills_InvalidEntryReportsIndex(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bills.yml")
	// Second entry is missing tvw.event_id.
	os.WriteFile(path, []byte(`
bills:
  - biennium: "2025-26"
    bill_prefix: "HB"
    bill_number: 1501
    chamber: "Senate"
    committee: { acronym: "HOUS", csi_id: "34078" }
    agenda:
      csi_meeting_family_id: "33828"
      csi_agenda_item_family_id: "169654"
      csi_agenda_item_id: "27885"
      label: "EHB 1501 CIC unit owner inquiries"
    tvw: { event_id: "2026021085" }
  - biennium: "2025-26"
    bill_prefix: "SB"
    bill_number: 6200
    chamber: "House"
    committee: { acronym: "HOUS", csi_id: "31633" }
    agenda:
      csi_meeting_family_id: "34051"
      csi_agenda_item_family_id: "171165"
      csi_agenda_item_id: "28474"
      label: "ESSB 6200 Tenant cooling devices"
`), 0o644)
	_, err := LoadSelectedBills(path)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "bills[1]") {
		t.Errorf("error should name the offending index, got: %v", err)
	}
}

func TestLoadSelectedBills_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bills.yml")
	os.WriteFile(path, []byte(`bills: []`), 0o644)
	_, err := LoadSelectedBills(path)
	if err == nil {
		t.Fatal("expected error for empty list")
	}
}
