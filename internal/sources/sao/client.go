// Package sao implements the Washington State Auditor ReportSearch client.
package sao

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/connector"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
)

const (
	SystemName     = "sao_reportsearch"
	DefaultBaseURL = "https://portal.sao.wa.gov/ReportSearch"
)

var descriptor = connector.Descriptor{
	System:      SystemName,
	BaseURL:     DefaultBaseURL,
	Description: "WA State Auditor ReportSearch portal",
}

func init() { connector.Register(descriptor) }

type Client struct {
	HTTP    *httpx.Client
	BaseURL string
}

func New(h *httpx.Client) *Client { return &Client{HTTP: h, BaseURL: DefaultBaseURL} }

// Descriptor implements connector.Source.
func (c *Client) Descriptor() connector.Descriptor { return descriptor }

type GovType struct {
	Code string
	Name string
	Raw  map[string]any
}

type AuditType struct {
	ID   string
	Name string
	Raw  map[string]any
}

type Entity struct {
	MCAG    string
	Name    string
	GovType string
	Raw     map[string]any
}

type SearchParams struct {
	PageSize                      int
	PageNumber                    int
	StartDate                     time.Time
	EndDate                       time.Time
	MCAGList                      string
	GovTypeCodeList               string
	AuditTypeID                   string
	Keyword                       string
	HasFindings                   *bool
	StateGovernment               bool
	LocalGovernment               bool
	PerformanceAudits             bool
	SpecialInvestigations         bool
	UseOfDeadlyForceInvestigation bool
	PoliceCertificationAudit      bool
	SortField                     string
	SortDir                       string
}

type SearchResult struct {
	Reports []Report
	Total   int
}

type Report struct {
	AuditNumber       string
	AuditReportNumber string
	ReportTitle       string
	AuditTypeName     string
	GovTypeDesc       string
	DateReleased      *time.Time
	BeginAuditPeriod  *time.Time
	EndAuditPeriod    *time.Time
	Findings          bool
	AuditReportLink   string
	FindingsLink      string
	Raw               map[string]any
}

func (c *Client) GovTypes(ctx context.Context) ([]GovType, error) {
	fetch, err := c.get(ctx, "api.Reports.GovTypes", c.BaseURL+"/api/Reports/GovTypes")
	if err != nil {
		return nil, err
	}
	var raws []map[string]any
	if err := json.Unmarshal(fetch.Body, &raws); err != nil {
		return nil, fmt.Errorf("sao gov types: %w", err)
	}
	out := make([]GovType, 0, len(raws))
	for _, r := range raws {
		out = append(out, GovType{Code: first(r, "Code", "GovTypeCode", "Value"), Name: first(r, "Name", "Text", "GovTypeDesc"), Raw: r})
	}
	return out, nil
}

func (c *Client) AuditTypes(ctx context.Context) ([]AuditType, error) {
	fetch, err := c.get(ctx, "api.Reports.AuditTypes", c.BaseURL+"/api/Reports/AuditTypes")
	if err != nil {
		return nil, err
	}
	var raws []map[string]any
	if err := json.Unmarshal(fetch.Body, &raws); err != nil {
		return nil, fmt.Errorf("sao audit types: %w", err)
	}
	out := make([]AuditType, 0, len(raws))
	for _, r := range raws {
		out = append(out, AuditType{ID: first(r, "AuditTypeID", "ID", "Value"), Name: first(r, "AuditTypeName", "Name", "Text"), Raw: r})
	}
	return out, nil
}

func (c *Client) GetEntities(ctx context.Context, nameStartsWith string) ([]Entity, error) {
	q := url.Values{}
	q.Set("NameStartsWith", nameStartsWith)
	fetch, err := c.get(ctx, "Home.GetEntities", c.BaseURL+"/Home/GetEntities?"+q.Encode())
	if err != nil {
		return nil, err
	}
	var raws []map[string]any
	if err := json.Unmarshal(fetch.Body, &raws); err != nil {
		return nil, fmt.Errorf("sao entities: %w", err)
	}
	out := make([]Entity, 0, len(raws))
	for _, r := range raws {
		out = append(out, Entity{MCAG: first(r, "MCAG", "Mcag", "Value", "id"), Name: first(r, "Name", "Text", "EntityName"), GovType: first(r, "GovType", "GovTypeDesc"), Raw: r})
	}
	return out, nil
}

func (c *Client) SearchReports(ctx context.Context, p SearchParams) (*SearchResult, error) {
	q := url.Values{}
	setInt(q, "pageSize", p.PageSize, 100)
	setInt(q, "pageNumber", p.PageNumber, 1)
	if !p.StartDate.IsZero() {
		q.Set("StartDate", p.StartDate.Format("1/2/2006"))
	}
	if !p.EndDate.IsZero() {
		q.Set("EndDate", p.EndDate.Format("1/2/2006"))
	}
	set(q, "MCAGList", p.MCAGList)
	set(q, "GovTypeCodeList", p.GovTypeCodeList)
	set(q, "AuditTypeID", p.AuditTypeID)
	set(q, "Keyword", p.Keyword)
	if p.HasFindings != nil {
		q.Set("HasFindings", strconv.FormatBool(*p.HasFindings))
	}
	q.Set("StateGovernment", strconv.FormatBool(p.StateGovernment))
	q.Set("LocalGovernment", strconv.FormatBool(p.LocalGovernment))
	q.Set("PerformanceAudits", strconv.FormatBool(p.PerformanceAudits))
	q.Set("SpecialInvestigations", strconv.FormatBool(p.SpecialInvestigations))
	q.Set("UseOfDeadlyForceInvestigation", strconv.FormatBool(p.UseOfDeadlyForceInvestigation))
	q.Set("PoliceCertificationAudit", strconv.FormatBool(p.PoliceCertificationAudit))
	set(q, "SortField", p.SortField)
	set(q, "SortDir", p.SortDir)

	fetch, err := c.get(ctx, "Home.SearchReports", c.BaseURL+"/Home/SearchReports?"+q.Encode())
	if err != nil {
		return nil, err
	}
	var raw struct {
		Data  []map[string]any `json:"data"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(fetch.Body, &raw); err != nil {
		return nil, fmt.Errorf("sao search reports: %w", err)
	}
	out := &SearchResult{Total: raw.Total, Reports: make([]Report, 0, len(raw.Data))}
	for _, r := range raw.Data {
		out.Reports = append(out.Reports, normalizeReport(r))
	}
	return out, nil
}

func (c *Client) ReportPDFURL(arn string) string {
	q := url.Values{}
	q.Set("arn", arn)
	q.Set("isFinding", "false")
	q.Set("sp", "false")
	return c.BaseURL + "/Home/ViewReportFile?" + q.Encode()
}

func (c *Client) get(ctx context.Context, endpoint, u string) (httpx.RawFetch, error) {
	return c.HTTP.Do(ctx, httpx.Request{System: SystemName, Endpoint: endpoint, URL: u, Headers: jsonAccept()})
}

func normalizeReport(r map[string]any) Report {
	return Report{
		AuditNumber:       first(r, "AuditNumber"),
		AuditReportNumber: first(r, "AuditReportNumber"),
		ReportTitle:       first(r, "ReportTitle"),
		AuditTypeName:     first(r, "AuditTypeName"),
		GovTypeDesc:       first(r, "GovTypeDesc"),
		DateReleased:      parseDotNetDate(first(r, "DateReleased")),
		BeginAuditPeriod:  parseDotNetDate(first(r, "BeginAuditPeriod")),
		EndAuditPeriod:    parseDotNetDate(first(r, "EndAuditPeriod")),
		Findings:          boolish(r["Findings"]),
		AuditReportLink:   first(r, "AuditReportLink"),
		FindingsLink:      first(r, "FindingsLink"),
		Raw:               r,
	}
}

var dotNetDate = regexp.MustCompile(`^/Date\((-?\d+)(?:[+-]\d+)?\)/$`)

func parseDotNetDate(s string) *time.Time {
	m := dotNetDate.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	ms, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return nil
	}
	t := time.UnixMilli(ms).UTC()
	return &t
}

func first(r map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := r[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func boolish(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		b, _ := strconv.ParseBool(x)
		return b
	default:
		return false
	}
}

func set(q url.Values, k, v string) {
	if v != "" {
		q.Set(k, v)
	}
}

func setInt(q url.Values, k string, v, def int) {
	if v <= 0 {
		v = def
	}
	q.Set(k, strconv.Itoa(v))
}

func jsonAccept() http.Header {
	h := http.Header{}
	h.Set("Accept", "application/json")
	return h
}
