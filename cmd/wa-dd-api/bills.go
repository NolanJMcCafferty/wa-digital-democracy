package main

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/pageassembly"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

const (
	billsDefaultLimit = 50
	billsMaxLimit     = 100
)

func listBillsHandler(store *db.Store) http.HandlerFunc {
	type item struct {
		Biennium      string    `json:"biennium"`
		BillPrefix    string    `json:"bill_prefix"`
		BillNumber    int       `json:"bill_number"`
		BillID        string    `json:"bill_id"`
		Title         string    `json:"title,omitempty"`
		ChamberOrigin string    `json:"chamber_origin,omitempty"`
		CurrentStatus string    `json:"current_status,omitempty"`
		StatusDate    time.Time `json:"status_date,omitempty"`
		StatusBucket  string    `json:"status_bucket,omitempty"` // in_progress | passed | failed
		LeadSponsor   string    `json:"lead_sponsor,omitempty"`  // "Senator Reed" — back-compat label
		LeadDisplay   string    `json:"lead_display,omitempty"`  // "Julia Reed" when first/last present
		LeadParty     string    `json:"lead_party,omitempty"`
		LeadSlug      string    `json:"lead_slug,omitempty"`
	}
	type body struct {
		Bills  []item              `json:"bills"`
		Total  int                 `json:"total"`
		Limit  int                 `json:"limit"`
		Offset int                 `json:"offset"`
		Facets db.BillSearchFacets `json:"facets"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		q := req.URL.Query()
		params := db.BillSearchParams{
			Query:         strings.TrimSpace(q.Get("q")),
			Prefix:        strings.ToUpper(strings.TrimSpace(q.Get("prefix"))),
			Chamber:       strings.TrimSpace(q.Get("chamber")),
			Party:         strings.ToUpper(strings.TrimSpace(q.Get("party"))),
			Status:        strings.TrimSpace(q.Get("status")),
			Sponsor:       strings.TrimSpace(q.Get("sponsor")),
			LeadSponsor:   strings.TrimSpace(q.Get("lead_sponsor")),
			BillIDs:       splitCSV(q.Get("bill_ids")),
			TopicKeywords: q["topic_keyword"],
		}
		// Soft-parse limit/offset; bad values fall back to defaults.
		params.Limit = billsDefaultLimit
		if v := q.Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				params.Limit = n
			}
		}
		if params.Limit > billsMaxLimit {
			params.Limit = billsMaxLimit
		}
		if v := q.Get("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				params.Offset = n
			}
		}

		bills, total, err := store.SearchBills(req.Context(), params)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		facets, err := store.ListBillSearchFacets(req.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		out := body{
			Bills:  make([]item, 0, len(bills)),
			Total:  total,
			Limit:  params.Limit,
			Offset: params.Offset,
			Facets: facets,
		}
		for _, b := range bills {
			display := strings.TrimSpace(b.LeadFirstName + " " + b.LeadLastName)
			out.Bills = append(out.Bills, item{
				Biennium:      b.Biennium,
				BillPrefix:    b.Prefix,
				BillNumber:    b.Number,
				BillID:        b.BillID,
				Title:         b.Title,
				ChamberOrigin: b.ChamberOrigin,
				CurrentStatus: b.CurrentStatus,
				StatusDate:    b.StatusDate,
				StatusBucket:  bucketStatus(b.CurrentStatus),
				LeadSponsor:   b.LeadSponsor,
				LeadDisplay:   display,
				LeadParty:     b.LeadParty,
				LeadSlug:      slugify(b.LeadSponsor),
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// bucketStatus mirrors SearchBills's status filter — keep them in sync.
func bucketStatus(s string) string {
	if s == "" {
		return ""
	}
	low := strings.ToLower(s)
	if strings.Contains(low, "effective date") ||
		strings.Contains(low, "governor signed") ||
		strings.Contains(low, "chapter ") && strings.Contains(low, "2026 laws") {
		return "passed"
	}
	if strings.Contains(low, "died") ||
		strings.Contains(low, "vetoed") ||
		strings.Contains(low, "not passed") {
		return "failed"
	}
	return "in_progress"
}

// billSlugRe matches "HB1501", "SB6200", "HJR4002", etc. — the slug shape
// the frontend route uses (mirrored from apps/web's parseBillSlug).
var billSlugRe = regexp.MustCompile(`^([A-Za-z]+)([0-9]+)$`)

func billPageHandler(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		biennium := chi.URLParam(req, "biennium")
		slug := chi.URLParam(req, "billNumber")
		m := billSlugRe.FindStringSubmatch(slug)
		if m == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": fmt.Sprintf("invalid bill slug %q (expected e.g. HB1501)", slug),
			})
			return
		}
		prefix := m[1]
		// Normalize to uppercase so /bills/2025-26/hb1501 also resolves.
		prefix = upper(prefix)
		number, err := strconv.Atoi(m[2])
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		page, err := pageassembly.BuildBillDetailResponse(req.Context(), store, biennium, prefix, number)
		if err != nil {
			if errors.Is(err, pageassembly.ErrBillNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "bill not ingested"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, page)
	}
}
