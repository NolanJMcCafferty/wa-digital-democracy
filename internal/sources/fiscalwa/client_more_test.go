package fiscalwa

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

func TestNewDefaults(t *testing.T) {
	c := New(httpx.New(httpx.Config{Sink: httpx.NopSink{}}))
	if c.BaseURL != DefaultBaseURL {
		t.Fatalf("BaseURL = %q", c.BaseURL)
	}
}

func TestParseVendorPaymentsXLSX(t *testing.T) {
	rows, err := ParseVendorPaymentsXLSX(tinyVendorPaymentsXLSX(t))
	if err != nil {
		t.Fatalf("ParseVendorPaymentsXLSX: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	first := rows[0]
	if first.Biennium != "2025-27" || first.FiscalYear != 2026 || first.VendorName != "HOME CARE MASTERS LLC" || first.Amount != "1402.27" {
		t.Fatalf("first row = %+v", first)
	}
	if first.SourceRowID == "" {
		t.Fatal("missing SourceRowID")
	}
	second := rows[1]
	if second.AgencyName != "Dept of Ecology" || second.VendorName != "ACME INC" || second.Amount != "99.50" {
		t.Fatalf("second row = %+v", second)
	}
}

func TestFetchVendorPaymentsWithSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != VendorPaymentsPath {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
			t.Fatalf("Accept = %q", got)
		}
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		_, _ = w.Write(tinyVendorPaymentsXLSX(t))
	}))
	defer srv.Close()

	c := New(httpx.New(httpx.Config{Sink: &fiscalSourceIDSink{id: 77}, HTTP: srv.Client()}))
	c.BaseURL = srv.URL
	rows, fetch, err := c.FetchVendorPaymentsWithSource(context.Background())
	if err != nil {
		t.Fatalf("FetchVendorPaymentsWithSource: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	if fetch.SourceRecordID != 77 || fetch.Endpoint != "vendor_payments_2025_27_xlsx" || fetch.System != SystemName {
		t.Fatalf("fetch = %+v", fetch)
	}
}

type fiscalSourceIDSink struct{ id int64 }

func (s *fiscalSourceIDSink) Record(ctx context.Context, f *httpx.RawFetch) error {
	f.SourceRecordID = s.id
	return nil
}

func tinyVendorPaymentsXLSX(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writeZipFile(t, zw, "xl/sharedStrings.xml", `<?xml version="1.0" encoding="UTF-8"?>
<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <si><t>Bien</t></si><si><t>FY</t></si><si><t>FMonth</t></si><si><t>Agy</t></si><si><t>Agency</t></si>
  <si><t>Object</t></si><si><t>Category</t></si><si><t>Subobj</t></si><si><t>SubCategory</t></si><si><t>Vendor</t></si><si><t>Amount</t></si>
  <si><t>2025-27</t></si><si><t>01</t></si><si><t>300</t></si><si><t>Social and Health Services</t></si>
  <si><t>E</t></si><si><t>Goods and Services</t></si><si><t>ER</t></si><si><t>Other Contractual Services</t></si><si><t>HOME CARE MASTERS LLC</t></si><si><t>1402.27</t></si>
  <si><r><t>Dept </t></r><r><t>of Ecology</t></r></si><si><t>ACME INC</t></si><si><t>99.50</t></si>
</sst>`)
	writeZipFile(t, zw, "xl/worksheets/sheet1.xml", `<?xml version="1.0" encoding="UTF-8"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>
  <row r="1">
    <c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="C1" t="s"><v>2</v></c><c r="D1" t="s"><v>3</v></c><c r="E1" t="s"><v>4</v></c>
    <c r="F1" t="s"><v>5</v></c><c r="G1" t="s"><v>6</v></c><c r="H1" t="s"><v>7</v></c><c r="I1" t="s"><v>8</v></c><c r="J1" t="s"><v>9</v></c><c r="K1" t="s"><v>10</v></c>
  </row>
  <row r="2">
    <c r="A2" t="s"><v>11</v></c><c r="B2"><v>2026</v></c><c r="C2" t="s"><v>12</v></c><c r="D2" t="s"><v>13</v></c><c r="E2" t="s"><v>14</v></c>
    <c r="F2" t="s"><v>15</v></c><c r="G2" t="s"><v>16</v></c><c r="H2" t="s"><v>17</v></c><c r="I2" t="s"><v>18</v></c><c r="J2" t="s"><v>19</v></c><c r="K2" t="s"><v>20</v></c>
  </row>
  <row r="3"><c r="A3" t="s"><v>11</v></c><c r="B3"><v>2026</v></c><c r="E3" t="s"><v>21</v></c><c r="J3" t="s"><v>22</v></c><c r="K3" t="s"><v>23</v></c></row>
  <row r="4"><c r="A4"><v></v></c><c r="J4"><v></v></c></row>
</sheetData></worksheet>`)
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func writeZipFile(t *testing.T, zw *zip.Writer, name, body string) {
	t.Helper()
	w, err := zw.Create(name)
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
