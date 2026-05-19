// Package fiscalwa implements clients/parsers for fiscal.wa.gov budget and
// spending datasets.
package fiscalwa

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

const (
	SystemName              = "fiscal_wa"
	DefaultBaseURL          = "https://fiscal.wa.gov"
	VendorPaymentsDatasetID = "vendor-payments-2025-27"
	VendorPaymentsPath      = "/Spending/VendorPayments2527.xlsx"
)

type Client struct {
	HTTP    *httpx.Client
	BaseURL string
}

func New(h *httpx.Client) *Client { return &Client{HTTP: h, BaseURL: DefaultBaseURL} }

type VendorPayment struct {
	SourceDatasetID string
	SourceRowID     string
	Biennium        string
	FiscalYear      int
	FiscalMonth     string
	AgencyNumber    string
	AgencyName      string
	ObjectCode      string
	ObjectCategory  string
	SubobjectCode   string
	SubobjectName   string
	VendorName      string
	Amount          string
}

func (c *Client) FetchVendorPaymentsWithSource(ctx context.Context) ([]VendorPayment, httpx.RawFetch, error) {
	u, err := url.Parse(strings.TrimRight(c.BaseURL, "/") + VendorPaymentsPath)
	if err != nil {
		return nil, httpx.RawFetch{}, err
	}
	fetch, err := c.HTTP.Do(ctx, httpx.Request{
		System:   SystemName,
		Endpoint: "vendor_payments_2025_27_xlsx",
		URL:      u.String(),
		Headers:  http.Header{"Accept": []string{"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"}},
	})
	if err != nil {
		return nil, httpx.RawFetch{}, err
	}
	rows, err := ParseVendorPaymentsXLSX(fetch.Body)
	if err != nil {
		return nil, fetch, err
	}
	return rows, fetch, nil
}

func ParseVendorPaymentsXLSX(body []byte) ([]VendorPayment, error) {
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, fmt.Errorf("open xlsx: %w", err)
	}
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}
	shared, err := readSharedStrings(files["xl/sharedStrings.xml"])
	if err != nil {
		return nil, err
	}
	out := []VendorPayment{}
	for name, f := range files {
		if !strings.HasPrefix(name, "xl/worksheets/sheet") || !strings.HasSuffix(name, ".xml") {
			continue
		}
		rows, err := parseVendorPaymentSheet(f, shared)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		out = append(out, rows...)
	}
	return out, nil
}

func NormalizeVendorPayment(row map[string]string) VendorPayment {
	fy, _ := strconv.Atoi(strings.TrimSpace(row["FY"]))
	payment := VendorPayment{
		SourceDatasetID: VendorPaymentsDatasetID,
		Biennium:        strings.TrimSpace(row["Bien"]),
		FiscalYear:      fy,
		FiscalMonth:     strings.TrimSpace(row["FMonth"]),
		AgencyNumber:    strings.TrimSpace(row["Agy"]),
		AgencyName:      strings.TrimSpace(row["Agency"]),
		ObjectCode:      strings.TrimSpace(row["Object"]),
		ObjectCategory:  strings.TrimSpace(row["Category"]),
		SubobjectCode:   strings.TrimSpace(row["Subobj"]),
		SubobjectName:   strings.TrimSpace(row["SubCategory"]),
		VendorName:      strings.TrimSpace(row["Vendor"]),
		Amount:          strings.TrimSpace(row["Amount"]),
	}
	payment.SourceRowID = StableVendorPaymentRowID(payment)
	return payment
}

func StableVendorPaymentRowID(p VendorPayment) string {
	parts := []string{p.Biennium, strconv.Itoa(p.FiscalYear), p.FiscalMonth, p.AgencyNumber, p.ObjectCode, p.SubobjectCode, p.VendorName, p.Amount}
	return strings.Join(parts, "|")
}

func readSharedStrings(f *zip.File) ([]string, error) {
	if f == nil {
		return nil, nil
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	var doc struct {
		SI []struct {
			T []string `xml:"t"`
			R []struct {
				T string `xml:"t"`
			} `xml:"r"`
		} `xml:"si"`
	}
	if err := xml.NewDecoder(rc).Decode(&doc); err != nil {
		return nil, fmt.Errorf("shared strings: %w", err)
	}
	stringsOut := make([]string, 0, len(doc.SI))
	for _, si := range doc.SI {
		var b strings.Builder
		for _, t := range si.T {
			b.WriteString(t)
		}
		for _, r := range si.R {
			b.WriteString(r.T)
		}
		stringsOut = append(stringsOut, b.String())
	}
	return stringsOut, nil
}

func parseVendorPaymentSheet(f *zip.File, shared []string) ([]VendorPayment, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	headers := []string{}
	out := []VendorPayment{}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != "row" {
			continue
		}
		cells, err := readRow(dec, start, shared)
		if err != nil {
			return nil, err
		}
		if len(headers) == 0 {
			headers = cells
			continue
		}
		m := map[string]string{}
		for i, h := range headers {
			if i < len(cells) {
				m[h] = cells[i]
			}
		}
		if strings.TrimSpace(m["Bien"]) == "" && strings.TrimSpace(m["Vendor"]) == "" {
			continue
		}
		out = append(out, NormalizeVendorPayment(m))
	}
	return out, nil
}

func readRow(dec *xml.Decoder, rowStart xml.StartElement, shared []string) ([]string, error) {
	cellsByIndex := map[int]string{}
	maxIdx := -1
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "c" {
				idx, val, err := readCell(dec, t, shared)
				if err != nil {
					return nil, err
				}
				cellsByIndex[idx] = val
				if idx > maxIdx {
					maxIdx = idx
				}
			}
		case xml.EndElement:
			if t.Name.Local == "row" {
				cells := make([]string, maxIdx+1)
				for i := 0; i <= maxIdx; i++ {
					cells[i] = cellsByIndex[i]
				}
				return cells, nil
			}
		}
	}
}

var cellRefRE = regexp.MustCompile(`^[A-Z]+`)

func readCell(dec *xml.Decoder, start xml.StartElement, shared []string) (int, string, error) {
	ref, typ := "", ""
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "r":
			ref = a.Value
		case "t":
			typ = a.Value
		}
	}
	idx := cellIndex(ref)
	var value string
	for {
		tok, err := dec.Token()
		if err != nil {
			return idx, "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "v" || t.Name.Local == "t" {
				var s string
				if err := dec.DecodeElement(&s, &t); err != nil {
					return idx, "", err
				}
				value = s
			}
		case xml.EndElement:
			if t.Name.Local == "c" {
				if typ == "s" && value != "" {
					n, err := strconv.Atoi(value)
					if err == nil && n >= 0 && n < len(shared) {
						value = shared[n]
					}
				}
				return idx, value, nil
			}
		}
	}
}

func cellIndex(ref string) int {
	letters := cellRefRE.FindString(ref)
	idx := 0
	for _, r := range letters {
		idx = idx*26 + int(r-'A'+1)
	}
	if idx == 0 {
		return 0
	}
	return idx - 1
}
