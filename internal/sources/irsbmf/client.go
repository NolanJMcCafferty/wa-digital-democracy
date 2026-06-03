// Package irsbmf is a connector for the IRS Exempt Organization Business
// Master File (BMF) state extract. The WA-specific file lives at
// https://www.irs.gov/pub/irs-soi/eo_wa.csv and contains every active
// 501(c) organization with a WA mailing address. We use it as one of two
// authoritative cross-source references when promoting a CSI organization
// string to a verified canonical organization row.
package irsbmf

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/connector"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

const (
	SystemName      = "irs_bmf"
	DefaultStateURL = "https://www.irs.gov/pub/irs-soi/eo_wa.csv"
)

var descriptor = connector.Descriptor{
	System:      SystemName,
	BaseURL:     DefaultStateURL,
	Description: "IRS Exempt Organization Business Master File (WA extract)",
}

func init() { connector.Register(descriptor) }

// Row is a single decoded BMF record. Field names mirror the published header
// (https://www.irs.gov/pub/irs-soi/eo_info.pdf). Numeric fields are parsed
// where useful; the original raw map is preserved for forward compatibility.
type Row struct {
	EIN             string
	Name            string
	ICO             string
	Street          string
	City            string
	State           string
	Zip             string
	GroupExemption  string
	Subsection      string
	Affiliation     string
	Classification  string
	Ruling          string
	Deductibility   string
	Foundation      string
	Activity        string
	Organization    string
	Status          string
	TaxPeriod       string
	AssetCode       string
	IncomeCode      string
	FilingReqCode   string
	PFFilingReqCode string
	AcctPeriod      string
	AssetAmount     int64
	IncomeAmount    int64
	RevenueAmount   int64
	NTEECode        string
	SortName        string
	Raw             map[string]string
}

type Client struct {
	HTTP    *httpx.Client
	BaseURL string // override for tests
}

func New(h *httpx.Client) *Client {
	return &Client{HTTP: h, BaseURL: DefaultStateURL}
}

// Descriptor implements connector.Source.
func (c *Client) Descriptor() connector.Descriptor { return descriptor }

// Fetch downloads the WA extract and returns the raw CSV bytes plus the
// httpx.RawFetch metadata so callers can persist a source_record for it.
func (c *Client) Fetch(ctx context.Context) ([]byte, httpx.RawFetch, error) {
	headers := http.Header{}
	headers.Set("Accept", "text/csv")
	fetch, err := c.HTTP.Do(ctx, httpx.Request{
		System:   SystemName,
		Endpoint: "eo_wa.csv",
		URL:      c.BaseURL,
		Headers:  headers,
	})
	if err != nil {
		return nil, fetch, err
	}
	if fetch.Status < 200 || fetch.Status >= 300 {
		return nil, fetch, fmt.Errorf("irsbmf: fetch %s: status %d", c.BaseURL, fetch.Status)
	}
	return fetch.Body, fetch, nil
}

// ParseAll decodes the CSV, calling yield for each successfully parsed row.
// Returning false from yield stops iteration. ParseAll skips the header row
// and tolerates short/long records (the IRS occasionally extends the schema).
func ParseAll(body []byte, yield func(Row) bool) error {
	r := csv.NewReader(strings.NewReader(string(body)))
	r.FieldsPerRecord = -1 // tolerate schema drift
	r.LazyQuotes = true
	header, err := r.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("irsbmf: read header: %w", err)
	}
	cols := make(map[string]int, len(header))
	for i, h := range header {
		cols[strings.ToUpper(strings.TrimSpace(h))] = i
	}
	for {
		rec, err := r.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("irsbmf: read row: %w", err)
		}
		row := decode(rec, cols)
		if row.EIN == "" {
			continue
		}
		if !yield(row) {
			return nil
		}
	}
}

func decode(rec []string, cols map[string]int) Row {
	get := func(name string) string {
		i, ok := cols[name]
		if !ok || i >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[i])
	}
	raw := make(map[string]string, len(cols))
	for name, i := range cols {
		if i < len(rec) {
			raw[name] = strings.TrimSpace(rec[i])
		}
	}
	return Row{
		EIN:             get("EIN"),
		Name:            get("NAME"),
		ICO:             get("ICO"),
		Street:          get("STREET"),
		City:            get("CITY"),
		State:           get("STATE"),
		Zip:             get("ZIP"),
		GroupExemption:  get("GROUP"),
		Subsection:      get("SUBSECTION"),
		Affiliation:     get("AFFILIATION"),
		Classification:  get("CLASSIFICATION"),
		Ruling:          get("RULING"),
		Deductibility:   get("DEDUCTIBILITY"),
		Foundation:      get("FOUNDATION"),
		Activity:        get("ACTIVITY"),
		Organization:    get("ORGANIZATION"),
		Status:          get("STATUS"),
		TaxPeriod:       get("TAX_PERIOD"),
		AssetCode:       get("ASSET_CD"),
		IncomeCode:      get("INCOME_CD"),
		FilingReqCode:   get("FILING_REQ_CD"),
		PFFilingReqCode: get("PF_FILING_REQ_CD"),
		AcctPeriod:      get("ACCT_PD"),
		AssetAmount:     atoi64(get("ASSET_AMT")),
		IncomeAmount:    atoi64(get("INCOME_AMT")),
		RevenueAmount:   atoi64(get("REVENUE_AMT")),
		NTEECode:        get("NTEE_CD"),
		SortName:        get("SORT_NAME"),
		Raw:             raw,
	}
}

func atoi64(s string) int64 {
	if s == "" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}
