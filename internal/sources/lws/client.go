// Package lws is the Washington Legislative Web Services connector.
//
// LWS is SOAP/XML over HTTP. We hand-roll envelopes with text/template +
// encoding/xml rather than pulling in a SOAP library — the operation
// surface we need for the first page is small, and the response shapes are
// straightforward enough that struct-based xml.Unmarshal works cleanly.
//
// Per data-sources/01 §"Failure modes": some op names in the wiki are
// approximate. Verified against the live WSDL on 2026-05-15:
//
//   - The SOAP namespace is http://WSLWebServices.leg.wa.gov/  (capital WSL).
//   - GetSponsors takes <biennium> + <billId> (e.g. "HB 1234"), not <billNumber>.
//   - The wiki's "GetLegislativeStatusChanges" is actually exposed as
//     GetLegislativeStatusChangesByBillNumber / ByBillId / ByDateRange.
//
// First-page operations:
//   - LegislationService.GetLegislation              (biennium, billNumber)
//   - LegislationService.GetCurrentStatus            (biennium, billNumber)
//   - LegislationService.GetLegislativeStatusChangesByBillNumber
//     (biennium, billNumber, beginDate, endDate)
//   - LegislationService.GetSponsors                 (biennium, billId)
//   - LegislationService.GetHearings                 (biennium, billNumber)
//   - LegislationService.GetRollCalls                (biennium, billNumber)
//   - LegislationService.GetLegislationByYear        (year)
package lws

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"text/template"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/connector"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

const (
	SystemName     = "lws"
	DefaultBaseURL = "https://wslwebservices.leg.wa.gov"
	SOAPNamespace  = "http://WSLWebServices.leg.wa.gov/"
)

var descriptor = connector.Descriptor{
	System:      SystemName,
	BaseURL:     DefaultBaseURL,
	Description: "Washington Legislative Web Services (SOAP/XML)",
}

func init() { connector.Register(descriptor) }

// Client wraps an httpx.Client.
type Client struct {
	HTTP    *httpx.Client
	BaseURL string
}

// New returns a Client.
func New(h *httpx.Client) *Client {
	return &Client{HTTP: h, BaseURL: DefaultBaseURL}
}

// Descriptor implements connector.Source.
func (c *Client) Descriptor() connector.Descriptor { return descriptor }

// soapEnvelope is rendered around any operation body. The {{.Inner}} block
// is the operation element with parameters.
const soapEnvelopeTmpl = `<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"
               xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
               xmlns:xsd="http://www.w3.org/2001/XMLSchema">
  <soap:Body>
    {{.Inner}}
  </soap:Body>
</soap:Envelope>`

var envelopeTmpl = template.Must(template.New("env").Parse(soapEnvelopeTmpl))

// callRaw posts a SOAP envelope and returns the parsed body. The op name is
// used for the SOAPAction header and request metadata.
// inner must be a valid <Op xmlns="…">…</Op> XML fragment.
func (c *Client) callRaw(ctx context.Context, service, op, inner string) ([]byte, error) {
	var env bytes.Buffer
	if err := envelopeTmpl.Execute(&env, map[string]string{"Inner": inner}); err != nil {
		return nil, fmt.Errorf("render envelope: %w", err)
	}

	headers := http.Header{}
	headers.Set("Content-Type", "text/xml; charset=utf-8")
	headers.Set("SOAPAction", SOAPNamespace+op)
	headers.Set("Accept", "text/xml")

	fetch, err := c.HTTP.Do(ctx, httpx.Request{
		System:   SystemName,
		Endpoint: service + "." + op,
		Method:   http.MethodPost,
		URL:      c.BaseURL + "/" + service + ".asmx",
		Headers:  headers,
		Body:     env.Bytes(),
	})
	if err != nil {
		return nil, err
	}
	return fetch.Body, nil
}

// renderOp renders a <Op xmlns="..."><k1>v1</k1>...</Op> fragment from a
// param map preserving insertion order.
func renderOp(op string, params [][2]string) string {
	var b bytes.Buffer
	fmt.Fprintf(&b, `<%s xmlns="%s">`, op, SOAPNamespace)
	for _, kv := range params {
		fmt.Fprintf(&b, `<%s>%s</%s>`, kv[0], xmlEscape(kv[1]), kv[0])
	}
	fmt.Fprintf(&b, `</%s>`, op)
	return b.String()
}

// GetLegislation calls LegislationService.GetLegislation.
func (c *Client) GetLegislation(ctx context.Context, biennium, billNumber string) (*Legislation, error) {
	body, err := c.callRaw(ctx, "LegislationService", "GetLegislation", renderOp("GetLegislation", [][2]string{
		{"biennium", biennium},
		{"billNumber", billNumber},
	}))
	if err != nil {
		return nil, err
	}
	return ParseGetLegislation(body)
}

// GetCurrentStatus calls LegislationService.GetCurrentStatus.
func (c *Client) GetCurrentStatus(ctx context.Context, biennium, billNumber string) (*CurrentStatus, error) {
	body, err := c.callRaw(ctx, "LegislationService", "GetCurrentStatus", renderOp("GetCurrentStatus", [][2]string{
		{"biennium", biennium},
		{"billNumber", billNumber},
	}))
	if err != nil {
		return nil, err
	}
	return ParseGetCurrentStatus(body)
}

// GetLegislativeStatusChangesByBillNumber calls the eponymous op. Date
// strings are formatted "YYYY-MM-DD".
func (c *Client) GetLegislativeStatusChangesByBillNumber(ctx context.Context, biennium, billNumber string, begin, end time.Time) ([]StatusChange, error) {
	body, err := c.callRaw(ctx, "LegislationService", "GetLegislativeStatusChangesByBillNumber", renderOp("GetLegislativeStatusChangesByBillNumber", [][2]string{
		{"biennium", biennium},
		{"billNumber", billNumber},
		{"beginDate", begin.Format("2006-01-02")},
		{"endDate", end.Format("2006-01-02")},
	}))
	if err != nil {
		return nil, err
	}
	return ParseStatusChanges(body)
}

// GetSponsors calls LegislationService.GetSponsors. Note billId, not billNumber.
func (c *Client) GetSponsors(ctx context.Context, biennium, billID string) ([]Sponsor, error) {
	body, err := c.callRaw(ctx, "LegislationService", "GetSponsors", renderOp("GetSponsors", [][2]string{
		{"biennium", biennium},
		{"billId", billID},
	}))
	if err != nil {
		return nil, err
	}
	return ParseSponsors(body)
}

// GetHearings calls LegislationService.GetHearings.
func (c *Client) GetHearings(ctx context.Context, biennium, billNumber string) ([]Hearing, error) {
	body, err := c.callRaw(ctx, "LegislationService", "GetHearings", renderOp("GetHearings", [][2]string{
		{"biennium", biennium},
		{"billNumber", billNumber},
	}))
	if err != nil {
		return nil, err
	}
	return ParseHearings(body)
}

// GetRollCalls calls LegislationService.GetRollCalls.
func (c *Client) GetRollCalls(ctx context.Context, biennium, billNumber string) ([]RollCall, error) {
	body, err := c.callRaw(ctx, "LegislationService", "GetRollCalls", renderOp("GetRollCalls", [][2]string{
		{"biennium", biennium},
		{"billNumber", billNumber},
	}))
	if err != nil {
		return nil, err
	}
	return ParseRollCalls(body)
}

// GetSenateSponsors calls SponsorService.GetSenateSponsors and returns
// the full Senate roster for the biennium (~54 members).
//
// SponsorService is the legislative-service member roster: rosters are
// keyed by biennium, returning every seated member regardless of
// whether they've sponsored a bill in that biennium.
func (c *Client) GetSenateSponsors(ctx context.Context, biennium string) ([]Member, error) {
	body, err := c.callRaw(ctx, "SponsorService", "GetSenateSponsors", renderOp("GetSenateSponsors", [][2]string{
		{"biennium", biennium},
	}))
	if err != nil {
		return nil, err
	}
	return ParseSenateSponsors(body)
}

// GetHouseSponsors calls SponsorService.GetHouseSponsors and returns
// the full House roster for the biennium (~98–104 members including
// mid-biennium turnover).
func (c *Client) GetHouseSponsors(ctx context.Context, biennium string) ([]Member, error) {
	body, err := c.callRaw(ctx, "SponsorService", "GetHouseSponsors", renderOp("GetHouseSponsors", [][2]string{
		{"biennium", biennium},
	}))
	if err != nil {
		return nil, err
	}
	return ParseHouseSponsors(body)
}

// GetLegislationByYear calls LegislationService.GetLegislationByYear (used
// by the candidate finder in Phase 3).
func (c *Client) GetLegislationByYear(ctx context.Context, year int) ([]LegislationInfo, error) {
	body, err := c.callRaw(ctx, "LegislationService", "GetLegislationByYear", renderOp("GetLegislationByYear", [][2]string{
		{"year", fmt.Sprintf("%d", year)},
	}))
	if err != nil {
		return nil, err
	}
	return ParseLegislationByYear(body)
}

// xmlEscape escapes the five XML reserved characters. We render parameter
// VALUES as text content, not attributes, so we don't need attribute-quote
// escaping.
func xmlEscape(s string) string {
	var b bytes.Buffer
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
