package fiscalwa

import "testing"

func TestNormalizeVendorPayment(t *testing.T) {
	p := NormalizeVendorPayment(map[string]string{
		"Bien":        "2025-27",
		"FY":          "2026",
		"FMonth":      "01",
		"Agy":         "300",
		"Agency":      "Social and Health Services         ",
		"Object":      "E",
		"Category":    "Goods and Services                 ",
		"Subobj":      "ER",
		"SubCategory": "Other Contractual Services         ",
		"Vendor":      "HOME CARE MASTERS LLC                                                                               ",
		"Amount":      "1402.27",
	})
	if p.SourceDatasetID != VendorPaymentsDatasetID || p.Biennium != "2025-27" || p.FiscalYear != 2026 || p.FiscalMonth != "01" {
		t.Fatalf("unexpected payment identity: %#v", p)
	}
	if p.AgencyName != "Social and Health Services" || p.VendorName != "HOME CARE MASTERS LLC" || p.Amount != "1402.27" {
		t.Fatalf("unexpected normalized payment: %#v", p)
	}
	if p.SourceRowID == "" {
		t.Fatal("missing source row id")
	}
}

func TestCellIndex(t *testing.T) {
	cases := map[string]int{"A1": 0, "B1": 1, "Z9": 25, "AA1": 26, "AB44": 27}
	for ref, want := range cases {
		if got := cellIndex(ref); got != want {
			t.Fatalf("cellIndex(%q) = %d, want %d", ref, got, want)
		}
	}
}
