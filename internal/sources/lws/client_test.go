package lws

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

func TestNewDefaults(t *testing.T) {
	c := New(httpx.New(httpx.Config{}))
	if c.BaseURL != DefaultBaseURL {
		t.Fatalf("BaseURL = %q", c.BaseURL)
	}
}

func TestRenderOpAndXMLEscape(t *testing.T) {
	got := renderOp("GetSponsors", [][2]string{{"biennium", "2025&26"}, {"billId", `HB <1234> "A'`}})
	wantParts := []string{
		`<GetSponsors xmlns="` + SOAPNamespace + `">`,
		`<biennium>2025&amp;26</biennium>`,
		`<billId>HB &lt;1234&gt; &quot;A&apos;</billId>`,
		`</GetSponsors>`,
	}
	for _, want := range wantParts {
		if !strings.Contains(got, want) {
			t.Fatalf("renderOp missing %q in %s", want, got)
		}
	}
}

func TestClientOperationsPostSOAPAndParse(t *testing.T) {
	tests := []struct {
		name       string
		call       func(*Client) error
		service    string
		op         string
		wantParams []string
		response   string
	}{
		{
			name: "GetLegislation",
			call: func(c *Client) error {
				leg, err := c.GetLegislation(context.Background(), "2025-26", "1234")
				if err == nil && leg.BillID != "HB 1234" {
					t.Fatalf("leg = %+v", leg)
				}
				return err
			},
			service: "LegislationService", op: "GetLegislation",
			wantParams: []string{"<biennium>2025-26</biennium>", "<billNumber>1234</billNumber>"},
			response:   string(read(t, "get-legislation.xml")),
		},
		{
			name: "GetCurrentStatus",
			call: func(c *Client) error {
				cs, err := c.GetCurrentStatus(context.Background(), "2025-26", "1234")
				if err == nil && cs.BillID != "HB 1234" {
					t.Fatalf("status = %+v", cs)
				}
				return err
			},
			service: "LegislationService", op: "GetCurrentStatus",
			wantParams: []string{"<biennium>2025-26</biennium>", "<billNumber>1234</billNumber>"},
			response:   string(read(t, "get-current-status.xml")),
		},
		{
			name: "GetSponsors",
			call: func(c *Client) error {
				rows, err := c.GetSponsors(context.Background(), "2025-26", "HB 1234")
				if err == nil && len(rows) == 0 {
					t.Fatal("no sponsors")
				}
				return err
			},
			service: "LegislationService", op: "GetSponsors",
			wantParams: []string{"<biennium>2025-26</biennium>", "<billId>HB 1234</billId>"},
			response:   string(read(t, "get-sponsors.xml")),
		},
		{
			name: "GetHearings",
			call: func(c *Client) error {
				rows, err := c.GetHearings(context.Background(), "2025-26", "1234")
				if err == nil && len(rows) == 0 {
					t.Fatal("no hearings")
				}
				return err
			},
			service: "LegislationService", op: "GetHearings",
			wantParams: []string{"<biennium>2025-26</biennium>", "<billNumber>1234</billNumber>"},
			response:   string(read(t, "get-hearings.xml")),
		},
		{
			name: "GetRollCalls",
			call: func(c *Client) error {
				rows, err := c.GetRollCalls(context.Background(), "2025-26", "1234")
				if err == nil && (len(rows) != 1 || rows[0].YeaVotes != 55) {
					t.Fatalf("rollcalls = %+v", rows)
				}
				return err
			},
			service: "LegislationService", op: "GetRollCalls",
			wantParams: []string{"<biennium>2025-26</biennium>", "<billNumber>1234</billNumber>"},
			response:   lwsEnvelope("GetRollCallsResponse", `<GetRollCallsResult><RollCall><BillId>HB 1234</BillId><Agency>House</Agency><Motion>Final passage</Motion><SequenceNumber>1</SequenceNumber><VoteDate>2026-02-01T00:00:00</VoteDate><YeaVotes>55</YeaVotes><NayVotes>40</NayVotes><AbsentVotes>2</AbsentVotes><ExcusedVotes>1</ExcusedVotes></RollCall></GetRollCallsResult>`),
		},
		{
			name: "GetLegislativeStatusChangesByBillNumber",
			call: func(c *Client) error {
				rows, err := c.GetLegislativeStatusChangesByBillNumber(context.Background(), "2025-26", "1234", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC))
				if err == nil && (len(rows) != 1 || rows[0].Status != "Passed") {
					t.Fatalf("changes = %+v", rows)
				}
				return err
			},
			service: "LegislationService", op: "GetLegislativeStatusChangesByBillNumber",
			wantParams: []string{"<biennium>2025-26</biennium>", "<billNumber>1234</billNumber>", "<beginDate>2026-01-02</beginDate>", "<endDate>2026-03-04</endDate>"},
			response:   lwsEnvelope("GetLegislativeStatusChangesByBillNumberResponse", `<GetLegislativeStatusChangesByBillNumberResult><LegislativeStatus><BillId>HB 1234</BillId><HistoryLine>Passed House</HistoryLine><ActionDate>2026-02-01T00:00:00</ActionDate><Status>Passed</Status></LegislativeStatus></GetLegislativeStatusChangesByBillNumberResult>`),
		},
		{
			name: "GetSenateSponsors",
			call: func(c *Client) error {
				rows, err := c.GetSenateSponsors(context.Background(), "2025-26")
				if err == nil && len(rows) < 40 {
					t.Fatalf("senators = %d", len(rows))
				}
				return err
			},
			service: "SponsorService", op: "GetSenateSponsors",
			wantParams: []string{"<biennium>2025-26</biennium>"},
			response:   string(read(t, "get-senate-sponsors.xml")),
		},
		{
			name: "GetHouseSponsors",
			call: func(c *Client) error {
				rows, err := c.GetHouseSponsors(context.Background(), "2025-26")
				if err == nil && len(rows) < 90 {
					t.Fatalf("house = %d", len(rows))
				}
				return err
			},
			service: "SponsorService", op: "GetHouseSponsors",
			wantParams: []string{"<biennium>2025-26</biennium>"},
			response:   string(read(t, "get-house-sponsors.xml")),
		},
		{
			name: "GetLegislationByYear",
			call: func(c *Client) error {
				rows, err := c.GetLegislationByYear(context.Background(), 2026)
				if err == nil && (len(rows) != 1 || rows[0].BillID != "HB 1234") {
					t.Fatalf("items = %+v", rows)
				}
				return err
			},
			service: "LegislationService", op: "GetLegislationByYear",
			wantParams: []string{"<year>2026</year>"},
			response:   lwsEnvelope("GetLegislationByYearResponse", `<GetLegislationByYearResult><LegislationInfo><Biennium>2025-26</Biennium><BillId>HB 1234</BillId><BillNumber>1234</BillNumber><OriginalAgency>House</OriginalAgency><Active>true</Active><ShortLegislationType><ShortLegislationType>HB</ShortLegislationType><LongLegislationType>House Bill</LongLegislationType></ShortLegislationType></LegislationInfo></GetLegislationByYearResult>`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Fatalf("method = %q", r.Method)
				}
				if r.URL.Path != "/"+tt.service+".asmx" {
					t.Fatalf("path = %q", r.URL.Path)
				}
				if got := r.Header.Get("SOAPAction"); got != SOAPNamespace+tt.op {
					t.Fatalf("SOAPAction = %q", got)
				}
				if got := r.Header.Get("Content-Type"); got != "text/xml; charset=utf-8" {
					t.Fatalf("Content-Type = %q", got)
				}
				body := readRequestBody(t, r)
				if !strings.Contains(body, "<"+tt.op+` xmlns="`+SOAPNamespace+`">`) {
					t.Fatalf("request body missing op: %s", body)
				}
				for _, want := range tt.wantParams {
					if !strings.Contains(body, want) {
						t.Fatalf("request body missing %q: %s", want, body)
					}
				}
				w.Header().Set("Content-Type", "text/xml")
				_, _ = w.Write([]byte(tt.response))
			}))
			defer srv.Close()

			c := New(httpx.New(httpx.Config{HTTP: srv.Client()}))
			c.BaseURL = srv.URL
			if err := tt.call(c); err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}
		})
	}
}

func readRequestBody(t *testing.T, r *http.Request) string {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	return string(body)
}

func lwsEnvelope(responseName, inner string) string {
	return `<?xml version="1.0" encoding="utf-8"?><soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><` + responseName + ` xmlns="` + SOAPNamespace + `">` + inner + `</` + responseName + `></soap:Body></soap:Envelope>`
}
