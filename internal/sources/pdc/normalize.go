package pdc

import (
	"strconv"
	"strings"
)

// LobbyistEmployment is the subset of xhn7-64im fields the page panel uses.
// Per spec 05 §"Entity fields to preserve". Raw map preserved separately.
type LobbyistEmployment struct {
	ReportNumber     string
	LobbyistID       string
	LobbyistName     string
	EmployerID       string
	EmployerName     string
	EmploymentYear   string
	EmploymentURL    string
	EmploymentPeriod string
}

// LobbyistCompensation is the subset of 9nnw-c693 fields used.
type LobbyistCompensation struct {
	FilerID         string
	FilerName       string
	FundingSourceID string
	FundingSource   string
	FilingPeriod    string
	EmployerID      string
	EmployerName    string
	Compensation    float64
	TotalExpenses   float64
	NetTotal        float64
	URL             string
}

// Contribution is the subset of 2jwd-akfb fields used.
type Contribution struct {
	ID                  string
	FilerID             string
	FilerName           string
	Office              string
	LegislativeDistrict string
	Party               string
	ElectionYear        string
	Amount              float64
	CashOrInKind        string
	ReceiptDate         string
	ContributorName     string
	ContributorCategory string
	URL                 string
}

// NormalizeLobbyistEmployment maps a row from xhn7-64im.
func NormalizeLobbyistEmployment(r Row) LobbyistEmployment {
	return LobbyistEmployment{
		ReportNumber:     str(r, "report_number"),
		LobbyistID:       str(r, "lobbyist_id"),
		LobbyistName:     str(r, "lobbyist_name"),
		EmployerID:       str(r, "employer_id"),
		EmployerName:     str(r, "employer_name"),
		EmploymentYear:   str(r, "employment_year"),
		EmploymentURL:    str(r, "employment_url"),
		EmploymentPeriod: str(r, "employment_period"),
	}
}

// NormalizeLobbyistCompensation maps a row from 9nnw-c693.
func NormalizeLobbyistCompensation(r Row) LobbyistCompensation {
	return LobbyistCompensation{
		FilerID:         str(r, "filer_id"),
		FilerName:       str(r, "filer_name"),
		FundingSourceID: str(r, "funding_source_id"),
		FundingSource:   str(r, "funding_source"),
		FilingPeriod:    str(r, "filing_period"),
		EmployerID:      str(r, "employer_id"),
		EmployerName:    str(r, "employer_name"),
		Compensation:    flt(r, "compensation"),
		TotalExpenses:   flt(r, "total_expenses"),
		NetTotal:        flt(r, "net_total"),
		URL:             str(r, "url"),
	}
}

// NormalizeContribution maps a row from 2jwd-akfb.
func NormalizeContribution(r Row) Contribution {
	return Contribution{
		ID:                  str(r, "id"),
		FilerID:             str(r, "filer_id"),
		FilerName:           str(r, "filer_name"),
		Office:              str(r, "office"),
		LegislativeDistrict: str(r, "legislative_district"),
		Party:               str(r, "party"),
		ElectionYear:        str(r, "election_year"),
		Amount:              flt(r, "amount"),
		CashOrInKind:        str(r, "cash_or_in_kind"),
		ReceiptDate:         str(r, "receipt_date"),
		ContributorName:     str(r, "contributor_name"),
		ContributorCategory: str(r, "contributor_category"),
		URL:                 str(r, "url"),
	}
}

// NormalizeOrgName collapses whitespace and strips punctuation that varies
// across PDC submissions. Used as a join key in the organization-matching
// pass; the wiki warns this is the hard part of MVP entity resolution.
func NormalizeOrgName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	// Collapse internal whitespace.
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		// Drop punctuation that's noisy in org names — comma, period, ampersand
		// in some forms ("AT&T" -> "att"), parentheses, slashes.
		switch r {
		case ',', '.', '&', '(', ')', '/', '"', '\'':
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	return b.String()
}

func str(r Row, k string) string {
	v, ok := r[k]
	if !ok || v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	case map[string]any:
		return str(x, "url")
	default:
		return ""
	}
}

func flt(r Row, k string) float64 {
	v, ok := r[k]
	if !ok || v == nil {
		return 0
	}
	switch x := v.(type) {
	case float64:
		return x
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	default:
		return 0
	}
}
