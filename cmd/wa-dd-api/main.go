// Command wa-dd-api is the read-only HTTP API that the Next.js frontend
// reads to render page-specific JSON objects from Postgres.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"

	firstpage "github.com/nolan-mccafferty/wa-digital-democracy/internal/render/firstpage"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func main() {
	var (
		addr = flag.String("addr", ":8080", "HTTP listen address")
		dsn  = flag.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres connection string")
	)
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	store, err := db.Open(ctx, *dsn)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer store.Close()

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	r.Get("/healthz", healthHandler(store))
	r.Get("/api/v1/addresses/suggest", suggestAddressesHandler())
	r.Get("/api/v1/bills", listBillsHandler(store))
	r.Get("/api/v1/bills/{biennium}/{billNumber}/page", billPageHandler(store))
	// Back-compat alias for older frontend/code paths. Returns the same
	// page-level shape as /page; despite the historical name, this is no
	// longer a generic legacy snapshot endpoint.
	r.Get("/api/v1/bills/{biennium}/{billNumber}/first-page", billPageHandler(store))
	r.Get("/api/v1/legislators", listLegislatorsHandler(store))
	r.Get("/api/v1/legislators/lookup", lookupLegislatorsByAddressHandler(store))
	r.Get("/api/v1/legislators/{slug}", getLegislatorHandler(store))
	r.Get("/api/v1/organizations", listOrganizationsHandler(store))
	r.Get("/api/v1/organizations/{slug}", getOrganizationHandler(store))
	r.Get("/api/v1/hearings", listHearingsHandler(store))
	r.Get("/api/v1/hearings/{hearingId}", getHearingHandler(store))
	r.Get("/api/v1/sources", listSourcesHandler(store))
	r.Get("/api/v1/search/transcripts", searchTranscriptsHandler(store))
	r.Get("/api/v1/admin/review/speakers", adminListSpeakerReviewTasksHandler(store))
	r.Get("/api/v1/admin/review/speakers/events", adminListSpeakerReviewEventsHandler(store))
	r.Get("/api/v1/admin/review/speakers/events/{tvwEventId}", adminGetSpeakerReviewEventHandler(store))
	r.Get("/api/v1/admin/review/speakers/clusters/{clusterId}", adminGetSpeakerClusterReviewHandler(store))
	r.Post("/api/v1/admin/review/speakers/clusters/{clusterId}/assign", adminManualAssignSpeakerClusterHandler(store))
	r.Get("/api/v1/admin/review/speakers/{taskId}", adminGetSpeakerReviewTaskHandler(store))
	r.Post("/api/v1/admin/review/speakers/{taskId}/accept", adminSpeakerReviewDecisionHandler(store, "accept"))
	r.Post("/api/v1/admin/review/speakers/{taskId}/reject", adminSpeakerReviewDecisionHandler(store, "reject"))
	r.Post("/api/v1/admin/review/speakers/{taskId}/needs-more-evidence", adminSpeakerReviewDecisionHandler(store, "needs_more_evidence"))
	r.Get("/api/v1/admin/review/entities/candidates", adminListEntityMatchCandidatesHandler(store))
	r.Post("/api/v1/admin/review/entities/candidates/{candidateId}/decide", adminDecideEntityMatchHandler(store))

	srv := &http.Server{
		Addr:              *addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("wa-dd-api listening on %s", *addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

func healthHandler(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if err := store.Pool.Ping(req.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unhealthy", "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

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

		page, err := firstpage.BuildBillPage(req.Context(), store, biennium, prefix, number)
		if err != nil {
			if errors.Is(err, firstpage.ErrBillNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "bill not ingested"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, page)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// splitCSV parses a "HB1006,SB5001" query-string value into trimmed,
// non-empty parts. Returns nil for empty input so SearchBills's
// len-check skips the filter.
func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func upper(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}

// ---------------------------------------------------------------------------
// Aggregation handlers — back the home/index pages so the frontend avoids
// fanning out one page-detail request per bill.
// ---------------------------------------------------------------------------

func listLegislatorsHandler(store *db.Store) http.HandlerFunc {
	type item struct {
		Slug         string `json:"slug"`
		Name         string `json:"name"`         // "Senator Alvarado"
		DisplayName  string `json:"display_name"` // "Emily Alvarado"
		FirstName    string `json:"first_name,omitempty"`
		LastName     string `json:"last_name,omitempty"`
		Role         string `json:"role,omitempty"` // "State Senator" | "State Representative"
		Chamber      string `json:"chamber,omitempty"`
		District     string `json:"district,omitempty"`
		Party        string `json:"party,omitempty"`
		Email        string `json:"email,omitempty"`
		Phone        string `json:"phone,omitempty"`
		OfficialURL  string `json:"official_url,omitempty"`
		PhotoURL     string `json:"photo_url,omitempty"`
		ThumbnailURL string `json:"thumbnail_url,omitempty"`
		BillCount    int    `json:"bill_count"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		legs, err := store.ListLegislators(req.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		out := make([]item, 0, len(legs))
		for _, l := range legs {
			photoURL, thumbnailURL := legislatorPhotoURLs(l.LWSSponsorID)
			out = append(out, item{
				Slug:         slugify(l.Name), // unchanged so existing /legislators/{slug} routes still resolve
				Name:         l.Name,
				DisplayName:  strings.TrimSpace(l.FirstName + " " + l.LastName),
				FirstName:    l.FirstName,
				LastName:     l.LastName,
				Role:         legislatorRole(l.Chamber),
				Chamber:      l.Chamber,
				District:     l.District,
				Party:        l.Party,
				Email:        l.Email,
				Phone:        l.Phone,
				OfficialURL:  l.OfficialURL,
				PhotoURL:     photoURL,
				ThumbnailURL: thumbnailURL,
				BillCount:    l.BillCount,
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func suggestAddressesHandler() http.HandlerFunc {
	type suggestion struct {
		Text     string `json:"text"`
		MagicKey string `json:"magic_key,omitempty"`
	}
	type body struct {
		Query       string       `json:"query"`
		Suggestions []suggestion `json:"suggestions"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		query := strings.TrimSpace(req.URL.Query().Get("query"))
		if len(query) < 4 {
			writeJSON(w, http.StatusOK, body{Query: query, Suggestions: []suggestion{}})
			return
		}

		suggestions, err := suggestWashingtonAddresses(req.Context(), query, 6)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		out := make([]suggestion, 0, len(suggestions))
		for _, s := range suggestions {
			out = append(out, suggestion{Text: s.Text, MagicKey: s.MagicKey})
		}
		writeJSON(w, http.StatusOK, body{Query: query, Suggestions: out})
	}
}

func lookupLegislatorsByAddressHandler(store *db.Store) http.HandlerFunc {
	type legislatorItem struct {
		Slug         string `json:"slug"`
		Name         string `json:"name"`
		DisplayName  string `json:"display_name,omitempty"`
		Role         string `json:"role,omitempty"`
		Chamber      string `json:"chamber,omitempty"`
		District     string `json:"district,omitempty"`
		Party        string `json:"party,omitempty"`
		PhotoURL     string `json:"photo_url,omitempty"`
		ThumbnailURL string `json:"thumbnail_url,omitempty"`
		BillCount    int    `json:"bill_count"`
	}
	type body struct {
		QueryAddress   string           `json:"query_address"`
		MatchedAddress string           `json:"matched_address,omitempty"`
		District       string           `json:"district"`
		Legislators    []legislatorItem `json:"legislators"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		address := strings.TrimSpace(req.URL.Query().Get("address"))
		magicKey := strings.TrimSpace(req.URL.Query().Get("magic_key"))
		if len(address) < 5 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "address is required"})
			return
		}

		lookup, err := lookupLegislativeDistrict(req.Context(), address, magicKey)
		if err != nil {
			switch {
			case errors.Is(err, errAddressNotFound):
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "address not found"})
			case errors.Is(err, errDistrictNotFound):
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "legislative district not found"})
			default:
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			}
			return
		}

		legs, err := store.ListLegislatorsByDistrict(req.Context(), lookup.District)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		out := make([]legislatorItem, 0, len(legs))
		senators := 0
		representatives := 0
		for _, l := range legs {
			switch l.Chamber {
			case "Senate":
				if senators >= 1 {
					continue
				}
				senators++
			case "House":
				if representatives >= 2 {
					continue
				}
				representatives++
			}
			photoURL, thumbnailURL := legislatorPhotoURLs(l.LWSSponsorID)
			out = append(out, legislatorItem{
				Slug:         slugify(l.Name),
				Name:         l.Name,
				DisplayName:  strings.TrimSpace(l.FirstName + " " + l.LastName),
				Role:         legislatorRole(l.Chamber),
				Chamber:      l.Chamber,
				District:     l.District,
				Party:        l.Party,
				PhotoURL:     photoURL,
				ThumbnailURL: thumbnailURL,
				BillCount:    l.BillCount,
			})
		}
		writeJSON(w, http.StatusOK, body{
			QueryAddress:   address,
			MatchedAddress: lookup.MatchedAddress,
			District:       lookup.District,
			Legislators:    out,
		})
	}
}

func getLegislatorHandler(store *db.Store) http.HandlerFunc {
	type appearance struct {
		Biennium      string `json:"biennium"`
		BillID        string `json:"bill_id"`
		BillPrefix    string `json:"bill_prefix"`
		BillNumber    int    `json:"bill_number"`
		BillTitle     string `json:"bill_title,omitempty"`
		SponsorType   string `json:"sponsor_type,omitempty"`
		ChamberOrigin string `json:"chamber_origin,omitempty"`
		CurrentStatus string `json:"current_status,omitempty"`
		StatusBucket  string `json:"status_bucket,omitempty"`
		LeadSponsor   string `json:"lead_sponsor,omitempty"`
		LeadDisplay   string `json:"lead_display,omitempty"`
		LeadParty     string `json:"lead_party,omitempty"`
		LeadSlug      string `json:"lead_slug,omitempty"`
	}
	type body struct {
		Slug         string       `json:"slug"`
		Name         string       `json:"name"`
		DisplayName  string       `json:"display_name,omitempty"`
		FirstName    string       `json:"first_name,omitempty"`
		LastName     string       `json:"last_name,omitempty"`
		Chamber      string       `json:"chamber,omitempty"`
		District     string       `json:"district,omitempty"`
		Party        string       `json:"party,omitempty"`
		PhotoURL     string       `json:"photo_url,omitempty"`
		ThumbnailURL string       `json:"thumbnail_url,omitempty"`
		Appearances  []appearance `json:"appearances"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		slug := chi.URLParam(req, "slug")
		legs, err := store.ListLegislators(req.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		var match *db.LegislatorAggregate
		for i := range legs {
			if slugify(legs[i].Name) == slug {
				match = &legs[i]
				break
			}
		}
		if match == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "legislator not found"})
			return
		}
		bills, err := store.GetLegislatorBills(req.Context(), match.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		apps := make([]appearance, 0, len(bills))
		for _, b := range bills {
			display := strings.TrimSpace(b.LeadFirstName + " " + b.LeadLastName)
			apps = append(apps, appearance{
				Biennium:      b.Biennium,
				BillID:        b.BillID,
				BillPrefix:    b.BillPrefix,
				BillNumber:    b.BillNumber,
				BillTitle:     b.BillTitle,
				SponsorType:   b.SponsorType,
				ChamberOrigin: b.ChamberOrigin,
				CurrentStatus: b.CurrentStatus,
				StatusBucket:  bucketStatus(b.CurrentStatus),
				LeadSponsor:   b.LeadSponsor,
				LeadDisplay:   display,
				LeadParty:     b.LeadParty,
				LeadSlug:      slugify(b.LeadSponsor),
			})
		}
		display := strings.TrimSpace(match.FirstName + " " + match.LastName)
		photoURL, thumbnailURL := legislatorPhotoURLs(match.LWSSponsorID)
		writeJSON(w, http.StatusOK, body{
			Slug:         slug,
			Name:         match.Name,
			DisplayName:  display,
			FirstName:    match.FirstName,
			LastName:     match.LastName,
			Chamber:      match.Chamber,
			District:     match.District,
			Party:        match.Party,
			PhotoURL:     photoURL,
			ThumbnailURL: thumbnailURL,
			Appearances:  apps,
		})
	}
}

var (
	errAddressNotFound  = errors.New("address not found")
	errDistrictNotFound = errors.New("legislative district not found")
	districtHTTPClient  = &http.Client{Timeout: 12 * time.Second}
)

const (
	censusGeocoderURL = "https://geocoding.geo.census.gov/geocoder/locations/onelineaddress"
	esriSuggestURL    = "https://geocode.arcgis.com/arcgis/rest/services/World/GeocodeServer/suggest"
	esriCandidateURL  = "https://geocode.arcgis.com/arcgis/rest/services/World/GeocodeServer/findAddressCandidates"
	legDistrictURL    = "https://services7.arcgis.com/zJ5hF9SNB8WMMiGf/ArcGIS/rest/services/Legislative_Districts/FeatureServer/0/query"
	washingtonExtent  = "-124.85,45.54,-116.91,49.01"
)

type districtLookupResult struct {
	District       string
	MatchedAddress string
}

func lookupLegislativeDistrict(ctx context.Context, address, magicKey string) (districtLookupResult, error) {
	point, err := geocodeAddress(ctx, address, magicKey)
	if err != nil {
		return districtLookupResult{}, err
	}
	district, err := legislativeDistrictForPoint(ctx, point.lon, point.lat)
	if err != nil {
		return districtLookupResult{}, err
	}
	return districtLookupResult{District: district, MatchedAddress: point.matchedAddress}, nil
}

type geocodedPoint struct {
	lon            float64
	lat            float64
	matchedAddress string
}

type addressSuggestion struct {
	Text     string
	MagicKey string
}

func suggestWashingtonAddresses(ctx context.Context, query string, limit int) ([]addressSuggestion, error) {
	var suggestions []addressSuggestion
	seen := map[string]bool{}
	for _, variant := range washingtonAddressQueryVariants(query) {
		next, err := suggestWashingtonAddressVariant(ctx, variant, limit)
		if err != nil {
			return nil, err
		}
		for _, suggestion := range next {
			if !strings.Contains(suggestion.Text, ", WA,") || seen[suggestion.Text] {
				continue
			}
			seen[suggestion.Text] = true
			suggestions = append(suggestions, suggestion)
			if len(suggestions) >= limit {
				return suggestions, nil
			}
		}
	}
	if suggestions == nil {
		suggestions = []addressSuggestion{}
	}
	return suggestions, nil
}

func suggestWashingtonAddressVariant(ctx context.Context, query string, limit int) ([]addressSuggestion, error) {
	u, err := url.Parse(esriSuggestURL)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("f", "json")
	q.Set("text", query)
	q.Set("countryCode", "USA")
	q.Set("category", "Address")
	q.Set("searchExtent", washingtonExtent)
	q.Set("maxSuggestions", strconv.Itoa(limit))
	u.RawQuery = q.Encode()

	var body struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Suggestions []struct {
			Text     string `json:"text"`
			MagicKey string `json:"magicKey"`
		} `json:"suggestions"`
	}
	if err := fetchJSON(ctx, u.String(), &body); err != nil {
		return nil, fmt.Errorf("suggest address: %w", err)
	}
	if body.Error != nil {
		return nil, fmt.Errorf("suggest address: %s", body.Error.Message)
	}
	out := make([]addressSuggestion, 0, len(body.Suggestions))
	for _, s := range body.Suggestions {
		out = append(out, addressSuggestion{Text: s.Text, MagicKey: s.MagicKey})
	}
	return out, nil
}

func washingtonAddressQueryVariants(query string) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	variants := []string{query}
	lower := strings.ToLower(query)
	if !strings.Contains(lower, " wa") && !strings.Contains(lower, "washington") {
		variants = append(variants, query+" WA")
	}
	return variants
}

func geocodeAddress(ctx context.Context, address, magicKey string) (geocodedPoint, error) {
	if magicKey != "" {
		return geocodeAddressWithESRI(ctx, address, magicKey)
	}
	point, err := geocodeAddressWithCensus(ctx, address)
	if err == nil {
		return point, nil
	}
	if errors.Is(err, errAddressNotFound) {
		return geocodeAddressWithESRI(ctx, address, "")
	}
	return geocodedPoint{}, err
}

func geocodeAddressWithCensus(ctx context.Context, address string) (geocodedPoint, error) {
	u, err := url.Parse(censusGeocoderURL)
	if err != nil {
		return geocodedPoint{}, err
	}
	q := u.Query()
	q.Set("address", address)
	q.Set("benchmark", "Public_AR_Current")
	q.Set("format", "json")
	u.RawQuery = q.Encode()

	var body struct {
		Result struct {
			AddressMatches []struct {
				MatchedAddress string `json:"matchedAddress"`
				Coordinates    struct {
					X float64 `json:"x"`
					Y float64 `json:"y"`
				} `json:"coordinates"`
			} `json:"addressMatches"`
		} `json:"result"`
	}
	if err := fetchJSON(ctx, u.String(), &body); err != nil {
		return geocodedPoint{}, fmt.Errorf("geocode address: %w", err)
	}
	if len(body.Result.AddressMatches) == 0 {
		return geocodedPoint{}, errAddressNotFound
	}
	match := body.Result.AddressMatches[0]
	return geocodedPoint{
		lon:            match.Coordinates.X,
		lat:            match.Coordinates.Y,
		matchedAddress: match.MatchedAddress,
	}, nil
}

func geocodeAddressWithESRI(ctx context.Context, address, magicKey string) (geocodedPoint, error) {
	u, err := url.Parse(esriCandidateURL)
	if err != nil {
		return geocodedPoint{}, err
	}
	q := u.Query()
	q.Set("f", "json")
	q.Set("singleLine", address)
	q.Set("countryCode", "USA")
	q.Set("category", "Address")
	q.Set("searchExtent", washingtonExtent)
	q.Set("outFields", "Match_addr,Region")
	q.Set("maxLocations", "1")
	if magicKey != "" {
		q.Set("magicKey", magicKey)
	}
	u.RawQuery = q.Encode()

	var body struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Candidates []struct {
			Address  string  `json:"address"`
			Score    float64 `json:"score"`
			Location struct {
				X float64 `json:"x"`
				Y float64 `json:"y"`
			} `json:"location"`
			Attributes struct {
				Region string `json:"Region"`
			} `json:"attributes"`
		} `json:"candidates"`
	}
	if err := fetchJSON(ctx, u.String(), &body); err != nil {
		return geocodedPoint{}, fmt.Errorf("geocode address: %w", err)
	}
	if body.Error != nil {
		return geocodedPoint{}, fmt.Errorf("geocode address: %s", body.Error.Message)
	}
	if len(body.Candidates) == 0 {
		return geocodedPoint{}, errAddressNotFound
	}
	match := body.Candidates[0]
	if match.Score < 80 || !strings.EqualFold(match.Attributes.Region, "Washington") {
		return geocodedPoint{}, errAddressNotFound
	}
	return geocodedPoint{
		lon:            match.Location.X,
		lat:            match.Location.Y,
		matchedAddress: match.Address,
	}, nil
}

func legislativeDistrictForPoint(ctx context.Context, lon, lat float64) (string, error) {
	u, err := url.Parse(legDistrictURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("f", "json")
	q.Set("geometry", fmt.Sprintf("%.8f,%.8f", lon, lat))
	q.Set("geometryType", "esriGeometryPoint")
	q.Set("inSR", "4326")
	q.Set("spatialRel", "esriSpatialRelIntersects")
	q.Set("outFields", "DISTRICT,DISTRICTN")
	q.Set("returnGeometry", "false")
	u.RawQuery = q.Encode()

	var body struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Features []struct {
			Attributes struct {
				District  string  `json:"DISTRICT"`
				DistrictN float64 `json:"DISTRICTN"`
			} `json:"attributes"`
		} `json:"features"`
	}
	if err := fetchJSON(ctx, u.String(), &body); err != nil {
		return "", fmt.Errorf("district lookup: %w", err)
	}
	if body.Error != nil {
		return "", fmt.Errorf("district lookup: %s", body.Error.Message)
	}
	if len(body.Features) == 0 {
		return "", errDistrictNotFound
	}
	attrs := body.Features[0].Attributes
	district := normalizeDistrict(attrs.District)
	if district == "" && attrs.DistrictN > 0 {
		district = strconv.Itoa(int(attrs.DistrictN))
	}
	if district == "" {
		return "", errDistrictNotFound
	}
	return district, nil
}

func fetchJSON(ctx context.Context, endpoint string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "wa-digital-democracy/0.1 (+https://github.com/nolan-mccafferty/wa-digital-democracy)")
	res, err := districtHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("status %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(target)
}

func normalizeDistrict(s string) string {
	var digits strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	out := strings.TrimLeft(digits.String(), "0")
	if out == "" && digits.Len() > 0 {
		return "0"
	}
	return out
}

func legislatorRole(chamber string) string {
	switch chamber {
	case "Senate":
		return "State Senator"
	case "House":
		return "State Representative"
	default:
		return "Legislator"
	}
}

func legislatorPhotoURLs(lwsSponsorID string) (string, string) {
	id := strings.TrimSpace(lwsSponsorID)
	if id == "" {
		return "", ""
	}
	escaped := url.PathEscape(id)
	return "https://leg.wa.gov/memberphoto/" + escaped + ".jpg",
		"https://leg.wa.gov/memberthumbnail/" + escaped + ".jpg"
}

func listOrganizationsHandler(store *db.Store) http.HandlerFunc {
	type item struct {
		Slug            string         `json:"slug"`
		CanonicalName   string         `json:"canonical_name"`
		Aliases         []string       `json:"aliases"`
		MatchConfidence string         `json:"match_confidence"`
		MatchNotes      string         `json:"match_notes,omitempty"`
		TestifierCount  int            `json:"testifier_count"`
		Positions       map[string]int `json:"positions"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		orgs, err := store.ListOrganizations(req.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		out := make([]item, 0, len(orgs))
		for _, o := range orgs {
			aliases := o.Aliases
			if aliases == nil {
				aliases = []string{}
			}
			out = append(out, item{
				Slug:            slugify(o.CanonicalName),
				CanonicalName:   o.CanonicalName,
				Aliases:         aliases,
				MatchConfidence: o.MatchConfidence,
				MatchNotes:      o.MatchNotes,
				TestifierCount:  o.TestifierCount,
				Positions: map[string]int{
					"Pro":     o.ProCount,
					"Con":     o.ConCount,
					"Other":   o.OtherCount,
					"Unknown": o.UnknownCount,
				},
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func getOrganizationHandler(store *db.Store) http.HandlerFunc {
	type appearance struct {
		Biennium        string    `json:"biennium"`
		BillID          string    `json:"bill_id"`
		BillPrefix      string    `json:"bill_prefix"`
		BillNumber      int       `json:"bill_number"`
		CSIAgendaItemID string    `json:"csi_agenda_item_id,omitempty"`
		HearingID       int64     `json:"hearing_id"`
		HearingTitle    string    `json:"hearing_title"`
		CommitteeName   string    `json:"committee_name"`
		Chamber         string    `json:"chamber,omitempty"`
		MeetingDateTime time.Time `json:"meeting_datetime"`
		Position        string    `json:"position,omitempty"`
		TestifierCount  int       `json:"testifier_count"`
	}
	type publicContext struct {
		ContextType     string   `json:"context_type"`
		SourceKind      string   `json:"source_kind"`
		SourceLabel     string   `json:"source_label"`
		SourceName      string   `json:"source_name"`
		Detail          string   `json:"detail,omitempty"`
		Amount          string   `json:"amount,omitempty"`
		RecordYear      int      `json:"record_year,omitempty"`
		RecordDate      string   `json:"record_date,omitempty"`
		URL             string   `json:"url,omitempty"`
		SourceRecordID  int64    `json:"source_record_id,omitempty"`
		MatchConfidence string   `json:"match_confidence"`
		Evidence        []string `json:"evidence"`
	}
	type body struct {
		Slug            string          `json:"slug"`
		CanonicalName   string          `json:"canonical_name"`
		Aliases         []string        `json:"aliases"`
		MatchConfidence string          `json:"match_confidence"`
		MatchNotes      string          `json:"match_notes,omitempty"`
		TestifierCount  int             `json:"testifier_count"`
		Positions       map[string]int  `json:"positions"`
		Appearances     []appearance    `json:"appearances"`
		Contexts        []publicContext `json:"contexts"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		slug := chi.URLParam(req, "slug")
		orgs, err := store.ListOrganizations(req.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		var match *db.OrganizationAggregate
		for i := range orgs {
			if slugify(orgs[i].CanonicalName) == slug {
				match = &orgs[i]
				break
			}
		}
		if match == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "organization not found"})
			return
		}
		appsRaw, err := store.GetOrganizationAppearances(req.Context(), match.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		contextsRaw, err := store.GetOrganizationPublicContexts(req.Context(), match.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		apps := make([]appearance, 0, len(appsRaw))
		for _, a := range appsRaw {
			apps = append(apps, appearance{
				Biennium: a.Biennium, BillID: a.BillID,
				BillPrefix: a.BillPrefix, BillNumber: a.BillNumber,
				CSIAgendaItemID: a.CSIAgendaItemID,
				HearingID:       a.HearingID,
				HearingTitle:    a.HearingTitle,
				CommitteeName:   a.CommitteeName,
				Chamber:         a.Chamber,
				MeetingDateTime: a.MeetingDateTime,
				Position:        a.Position,
				TestifierCount:  a.TestifierCount,
			})
		}
		contexts := make([]publicContext, 0, len(contextsRaw))
		for _, c := range contextsRaw {
			evidence := c.Evidence
			if evidence == nil {
				evidence = []string{}
			}
			contexts = append(contexts, publicContext{
				ContextType:     c.ContextType,
				SourceKind:      c.SourceKind,
				SourceLabel:     c.SourceLabel,
				SourceName:      c.SourceName,
				Detail:          c.Detail,
				Amount:          c.Amount,
				RecordYear:      c.RecordYear,
				RecordDate:      c.RecordDate,
				URL:             c.URL,
				SourceRecordID:  c.SourceRecordID,
				MatchConfidence: c.MatchConfidence,
				Evidence:        evidence,
			})
		}
		aliases := match.Aliases
		if aliases == nil {
			aliases = []string{}
		}
		writeJSON(w, http.StatusOK, body{
			Slug:            slug,
			CanonicalName:   match.CanonicalName,
			Aliases:         aliases,
			MatchConfidence: match.MatchConfidence,
			MatchNotes:      match.MatchNotes,
			TestifierCount:  match.TestifierCount,
			Positions: map[string]int{
				"Pro": match.ProCount, "Con": match.ConCount,
				"Other": match.OtherCount, "Unknown": match.UnknownCount,
			},
			Appearances: apps,
			Contexts:    contexts,
		})
	}
}

const (
	hearingsDefaultLimit = 50
	hearingsMaxLimit     = 100
)

type hearingAgendaItemResponse struct {
	CSIAgendaItemID string                    `json:"csi_agenda_item_id"`
	AgendaItemLabel string                    `json:"agenda_item_label"`
	Biennium        string                    `json:"biennium"`
	BillID          string                    `json:"bill_id"`
	BillPrefix      string                    `json:"bill_prefix"`
	BillNumber      int                       `json:"bill_number"`
	TestifierCount  int                       `json:"testifier_count"`
	TestifiedCount  int                       `json:"testified_count"`
	Section         *firstpage.HearingSection `json:"section,omitempty"`
}

type hearingResponse struct {
	HearingID          int64                       `json:"hearing_id"`
	CommitteeName      string                      `json:"committee_name"`
	Chamber            string                      `json:"chamber"`
	MeetingDateTime    time.Time                   `json:"meeting_datetime"`
	Location           string                      `json:"location,omitempty"`
	TVWURL             string                      `json:"tvw_url,omitempty"`
	TVWEventID         string                      `json:"tvw_event_id,omitempty"`
	AgendaItems        []hearingAgendaItemResponse `json:"agenda_items"`
	DiarizedTranscript *diarizedTranscriptResponse `json:"diarized_transcript,omitempty"`
}

type diarizedTranscriptResponse struct {
	Segments []diarizedSegmentResponse `json:"segments"`
}

type diarizedSegmentResponse struct {
	StartMS      int    `json:"start_ms"`
	EndMS        int    `json:"end_ms"`
	Text         string `json:"text"`
	ClusterLabel string `json:"cluster_label,omitempty"`
	SpeakerLabel string `json:"speaker_label,omitempty"`
	SpeakerKind  string `json:"speaker_kind,omitempty"`
	ReviewStatus string `json:"review_status,omitempty"`
	Reviewed     bool   `json:"reviewed"`
}

func (s diarizedSegmentResponse) PublicSpeakerLabel() string {
	if !s.Reviewed {
		return ""
	}
	return s.SpeakerLabel
}

func mapHearingResponse(h db.HearingAggregate) hearingResponse {
	items := make([]hearingAgendaItemResponse, 0, len(h.AgendaItems))
	for _, a := range h.AgendaItems {
		items = append(items, hearingAgendaItemResponse{
			CSIAgendaItemID: a.CSIAgendaItemID,
			AgendaItemLabel: a.AgendaItemLabel,
			Biennium:        a.Biennium,
			BillID:          a.BillID,
			BillPrefix:      a.BillPrefix,
			BillNumber:      a.BillNumber,
			TestifierCount:  a.TestifierCount,
			TestifiedCount:  a.TestifiedCount,
		})
	}
	return hearingResponse{
		HearingID:       h.HearingID,
		CommitteeName:   h.CommitteeName,
		Chamber:         h.Chamber,
		MeetingDateTime: h.MeetingDateTime,
		Location:        h.Location,
		TVWURL:          h.TVWURL,
		TVWEventID:      h.TVWEventID,
		AgendaItems:     items,
	}
}

func listHearingsHandler(store *db.Store) http.HandlerFunc {
	type body struct {
		Hearings []hearingResponse      `json:"hearings"`
		Total    int                    `json:"total"`
		Limit    int                    `json:"limit"`
		Offset   int                    `json:"offset"`
		Facets   db.HearingSearchFacets `json:"facets"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		q := req.URL.Query()
		params := db.HearingSearchParams{
			Committee:     strings.TrimSpace(q.Get("committee")),
			Bill:          strings.TrimSpace(q.Get("bill")),
			Speaker:       strings.TrimSpace(q.Get("speaker")),
			Chambers:      q["chamber"],
			TopicKeywords: q["topic_keyword"],
			Biennium:      strings.TrimSpace(q.Get("biennium")),
		}
		params.Limit = hearingsDefaultLimit
		if v := q.Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				params.Limit = n
			}
		}
		if params.Limit > hearingsMaxLimit {
			params.Limit = hearingsMaxLimit
		}
		if v := q.Get("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				params.Offset = n
			}
		}

		hs, total, err := store.SearchHearings(req.Context(), params)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		facets, err := store.ListHearingSearchFacets(req.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		out := body{
			Hearings: make([]hearingResponse, 0, len(hs)),
			Total:    total,
			Limit:    params.Limit,
			Offset:   params.Offset,
			Facets:   facets,
		}
		for _, h := range hs {
			out.Hearings = append(out.Hearings, mapHearingResponse(h))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func getHearingHandler(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		idParam := chi.URLParam(req, "hearingId")
		hearingID, err := strconv.ParseInt(idParam, 10, 64)
		if err != nil || hearingID <= 0 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "hearing not found"})
			return
		}
		hearing, err := store.GetHearing(req.Context(), hearingID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "hearing not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		resp := mapHearingResponse(*hearing)
		// Populate the full per-agenda-item sections (testifiers,
		// transcript, organizations) so /hearings/{id} can render them.
		// The list endpoint deliberately omits these to keep the payload
		// small.
		for i := range resp.AgendaItems {
			demo, err := firstpage.LookupSelectedDemoByAgendaItem(req.Context(), store, resp.AgendaItems[i].CSIAgendaItemID)
			if err != nil {
				if errors.Is(err, firstpage.ErrBillNotFound) {
					continue
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			section, err := firstpage.BuildHearingSection(req.Context(), store, demo)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			resp.AgendaItems[i].Section = section
		}
		if resp.TVWEventID != "" {
			segs, err := store.ListDiarizedSegmentsByTVWEvent(req.Context(), resp.TVWEventID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			if len(segs) > 0 {
				out := make([]diarizedSegmentResponse, 0, len(segs))
				for _, s := range segs {
					out = append(out, diarizedSegmentResponse{
						StartMS: s.StartMS, EndMS: s.EndMS, Text: s.Text, ClusterLabel: s.ClusterLabel,
						SpeakerLabel: s.SpeakerLabel, SpeakerKind: s.SpeakerKind,
						ReviewStatus: s.ReviewStatus, Reviewed: s.Reviewed,
					})
				}
				resp.DiarizedTranscript = &diarizedTranscriptResponse{Segments: out}
			}
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func listSourcesHandler(store *db.Store) http.HandlerFunc {
	type item struct {
		System          string    `json:"system"`
		Calls           int       `json:"calls"`
		LatestFetchedAt time.Time `json:"latest_fetched_at"`
		Endpoints       []string  `json:"endpoints"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		summaries, err := store.ListSourceSummaries(req.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		out := make([]item, 0, len(summaries))
		for _, s := range summaries {
			eps := s.Endpoints
			if eps == nil {
				eps = []string{}
			}
			out = append(out, item{
				System: s.System, Calls: s.Calls,
				LatestFetchedAt: s.LatestFetchedAt,
				Endpoints:       eps,
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// slugify mirrors apps/web/src/lib/api.ts:slugify exactly:
//
//	s.toLowerCase()
//	  .replace(/&/g, " and ")
//	  .replace(/[^a-z0-9]+/g, "-")
//	  .replace(/^-+|-+$/g, "")
//
// Identifiers produced server-side must match what the frontend emits in
// <Link> hrefs and route slugs, otherwise /api/v1/legislators/{slug}
// lookups silently miss.
func slugify(s string) string {
	// 1. lowercase, expand `&` → " and ".
	step1 := make([]byte, 0, len(s)+8)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c == '&' {
			step1 = append(step1, ' ', 'a', 'n', 'd', ' ')
			continue
		}
		step1 = append(step1, c)
	}
	// 2. collapse runs of [^a-z0-9] to single '-'.
	step2 := make([]byte, 0, len(step1))
	prevDash := false
	for _, c := range step1 {
		ok := (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
		if ok {
			step2 = append(step2, c)
			prevDash = false
			continue
		}
		if !prevDash {
			step2 = append(step2, '-')
			prevDash = true
		}
	}
	// 3. trim leading/trailing dashes.
	start, end := 0, len(step2)
	for start < end && step2[start] == '-' {
		start++
	}
	for end > start && step2[end-1] == '-' {
		end--
	}
	return string(step2[start:end])
}

// ---------------------------------------------------------------------------
// Transcript full-text search.
// ---------------------------------------------------------------------------

const (
	searchDefaultLimit = 20
	searchMaxLimit     = 50
)

func searchTranscriptsHandler(store *db.Store) http.HandlerFunc {
	type hit struct {
		ID              int64     `json:"id"`
		BillID          string    `json:"bill_id,omitempty"`
		Biennium        string    `json:"biennium,omitempty"`
		BillPrefix      string    `json:"bill_prefix,omitempty"`
		BillNumber      int       `json:"bill_number,omitempty"`
		AgendaItemLabel string    `json:"agenda_item_label,omitempty"`
		CommitteeName   string    `json:"committee_name,omitempty"`
		MeetingDateTime time.Time `json:"meeting_datetime,omitempty"`
		StartMS         int       `json:"start_ms"`
		EndMS           int       `json:"end_ms"`
		Text            string    `json:"text"`
		SpeakerLabel    string    `json:"speaker_label,omitempty"`
		TVWEventID      string    `json:"tvw_event_id,omitempty"`
	}
	type body struct {
		Query  string `json:"query"`
		Total  int    `json:"total"`
		Limit  int    `json:"limit"`
		Offset int    `json:"offset"`
		Hits   []hit  `json:"hits"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		q := req.URL.Query().Get("q")

		// Soft parsing — match the bill page handler style. Bad limit/offset
		// values fall back to defaults rather than 400'ing.
		limit := searchDefaultLimit
		if v := req.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}
		if limit > searchMaxLimit {
			limit = searchMaxLimit
		}
		offset := 0
		if v := req.URL.Query().Get("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				offset = n
			}
		}

		hitsRaw, total, err := store.SearchTranscripts(req.Context(), q, limit, offset)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		out := body{
			Query:  q,
			Total:  total,
			Limit:  limit,
			Offset: offset,
			Hits:   make([]hit, 0, len(hitsRaw)),
		}
		for _, h := range hitsRaw {
			out.Hits = append(out.Hits, hit{
				ID:              h.ID,
				BillID:          h.BillID,
				Biennium:        h.Biennium,
				BillPrefix:      h.BillPrefix,
				BillNumber:      h.BillNumber,
				AgendaItemLabel: h.AgendaItemLabel,
				CommitteeName:   h.CommitteeName,
				MeetingDateTime: h.MeetingDateTime,
				StartMS:         h.StartMS,
				EndMS:           h.EndMS,
				Text:            h.Text,
				SpeakerLabel:    h.SpeakerLabel,
				TVWEventID:      h.TVWEventID,
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func adminListSpeakerReviewTasksHandler(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		status := strings.TrimSpace(req.URL.Query().Get("status"))
		limit := 50
		if v := req.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}
		tasks, err := store.ListSpeakerReviewTasks(req.Context(), status, limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
	}
}

func adminListSpeakerReviewEventsHandler(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		limit := 50
		if v := req.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}
		events, err := store.ListSpeakerReviewEvents(req.Context(), limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"events": events})
	}
}

func adminGetSpeakerReviewEventHandler(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		tvwEventID := strings.TrimSpace(chi.URLParam(req, "tvwEventId"))
		if tvwEventID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing event id"})
			return
		}
		clusters, err := store.ListSpeakerClustersForEvent(req.Context(), tvwEventID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tvw_event_id": tvwEventID, "clusters": clusters})
	}
}

func adminGetSpeakerClusterReviewHandler(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		id, err := strconv.ParseInt(chi.URLParam(req, "clusterId"), 10, 64)
		if err != nil || id <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad cluster id"})
			return
		}
		cluster, err := store.GetSpeakerClusterReview(req.Context(), id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, cluster)
	}
}

func adminManualAssignSpeakerClusterHandler(store *db.Store) http.HandlerFunc {
	type body struct {
		Kind     string `json:"kind"`
		Label    string `json:"label"`
		Reviewer string `json:"reviewer"`
		Notes    string `json:"notes"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		id, err := strconv.ParseInt(chi.URLParam(req, "clusterId"), 10, 64)
		if err != nil || id <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad cluster id"})
			return
		}
		var b body
		_ = json.NewDecoder(req.Body).Decode(&b)
		if err := store.ManualAssignSpeakerCluster(req.Context(), id, b.Kind, b.Label, b.Reviewer, b.Notes); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func adminGetSpeakerReviewTaskHandler(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		id, err := strconv.ParseInt(chi.URLParam(req, "taskId"), 10, 64)
		if err != nil || id <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad task id"})
			return
		}
		task, err := store.GetSpeakerReviewTask(req.Context(), id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, task)
	}
}

func adminListEntityMatchCandidatesHandler(store *db.Store) http.HandlerFunc {
	type segment struct {
		StartMS      int    `json:"start_ms"`
		EndMS        int    `json:"end_ms"`
		Text         string `json:"text"`
		ClusterLabel string `json:"cluster_label,omitempty"`
	}
	type transcriptContext struct {
		TVWEventID        string    `json:"tvw_event_id"`
		DiarizationJobID  int64     `json:"diarization_job_id,omitempty"`
		MentionStartMS    int       `json:"mention_start_ms"`
		MentionEndMS      int       `json:"mention_end_ms"`
		MentionText       string    `json:"mention_text"`
		MentionConfidence float64   `json:"mention_confidence"`
		Surrounding       []segment `json:"surrounding"`
	}
	type item struct {
		ID                  int64              `json:"id"`
		SourceKind          string             `json:"source_kind"`
		SourceTable         string             `json:"source_table"`
		SourcePK            int64              `json:"source_pk,omitempty"`
		SourceDatasetID     string             `json:"source_dataset_id,omitempty"`
		SourceRowID         string             `json:"source_row_id,omitempty"`
		SourceName          string             `json:"source_name"`
		NormalizedName      string             `json:"normalized_name"`
		OrganizationID      int64              `json:"organization_id"`
		CanonicalName       string             `json:"canonical_name"`
		CandidateConfidence string             `json:"candidate_confidence"`
		Evidence            []string           `json:"evidence"`
		SourceRecordID      int64              `json:"source_record_id,omitempty"`
		Decision            string             `json:"decision"`
		ReviewedConfidence  string             `json:"reviewed_confidence,omitempty"`
		Transcript          *transcriptContext `json:"transcript,omitempty"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		q := req.URL.Query()
		sourceKind := strings.TrimSpace(q.Get("source_kind"))
		decision := strings.TrimSpace(q.Get("decision"))
		limit := 50
		if v := q.Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}
		if limit > 200 {
			limit = 200
		}
		offset := 0
		if v := q.Get("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				offset = n
			}
		}
		candidates, total, err := store.ListVendorEntityMatchCandidatesPage(req.Context(), sourceKind, decision, limit, offset)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		ids := make([]int64, 0, len(candidates))
		for _, c := range candidates {
			if c.SourceKind == "deepgram_organization_mention" {
				ids = append(ids, c.ID)
			}
		}
		ctxByID, err := store.ListEntityMatchTranscriptContext(req.Context(), ids)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		out := make([]item, 0, len(candidates))
		for _, c := range candidates {
			evidence := c.Evidence
			if evidence == nil {
				evidence = []string{}
			}
			it := item{
				ID: c.ID, SourceKind: c.SourceKind, SourceTable: c.SourceTable,
				SourcePK: c.SourcePK, SourceDatasetID: c.SourceDatasetID, SourceRowID: c.SourceRowID,
				SourceName: c.SourceName, NormalizedName: c.NormalizedName,
				OrganizationID: c.OrganizationID, CanonicalName: c.CanonicalName,
				CandidateConfidence: c.CandidateConfidence, Evidence: evidence,
				SourceRecordID: c.SourceRecordID,
				Decision:       c.Decision, ReviewedConfidence: c.ReviewedConfidence,
			}
			if tc, ok := ctxByID[c.ID]; ok {
				segs := make([]segment, 0, len(tc.Surrounding))
				for _, s := range tc.Surrounding {
					segs = append(segs, segment{StartMS: s.StartMS, EndMS: s.EndMS, Text: s.Text, ClusterLabel: s.ClusterLabel})
				}
				it.Transcript = &transcriptContext{
					TVWEventID:        tc.TVWEventID,
					DiarizationJobID:  tc.DiarizationJobID,
					MentionStartMS:    tc.MentionStartMS,
					MentionEndMS:      tc.MentionEndMS,
					MentionText:       tc.MentionText,
					MentionConfidence: tc.MentionConfidence,
					Surrounding:       segs,
				}
			}
			out = append(out, it)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"candidates": out,
			"total":      total,
			"limit":      limit,
			"offset":     offset,
		})
	}
}

func adminDecideEntityMatchHandler(store *db.Store) http.HandlerFunc {
	type body struct {
		Decision   string `json:"decision"`
		Confidence string `json:"confidence"`
		Reviewer   string `json:"reviewer"`
		Notes      string `json:"notes"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		candidateID, err := strconv.ParseInt(chi.URLParam(req, "candidateId"), 10, 64)
		if err != nil || candidateID <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad candidate id"})
			return
		}
		var b body
		_ = json.NewDecoder(req.Body).Decode(&b)
		switch b.Decision {
		case "confirmed", "rejected", "needs_review":
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "decision must be confirmed, rejected, or needs_review"})
			return
		}
		var organizationID int64
		if err := store.Pool.QueryRow(req.Context(), `SELECT organization_id FROM vendor_entity_match_candidate WHERE id = $1`, candidateID).Scan(&organizationID); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "candidate not found"})
			return
		}
		if _, err := store.UpsertVendorEntityMatchDecision(req.Context(), db.InsertVendorEntityMatchDecisionParams{
			CandidateID:    candidateID,
			OrganizationID: organizationID,
			Decision:       b.Decision,
			Confidence:     b.Confidence,
			ReviewedBy:     b.Reviewer,
			ReviewNotes:    b.Notes,
		}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func adminSpeakerReviewDecisionHandler(store *db.Store, action string) http.HandlerFunc {
	type body struct {
		Reviewer string `json:"reviewer"`
		Notes    string `json:"notes"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		id, err := strconv.ParseInt(chi.URLParam(req, "taskId"), 10, 64)
		if err != nil || id <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad task id"})
			return
		}
		var b body
		_ = json.NewDecoder(req.Body).Decode(&b)
		switch action {
		case "accept":
			err = store.AcceptSpeakerReviewTask(req.Context(), id, b.Reviewer, b.Notes)
		case "reject":
			err = store.RejectSpeakerReviewTask(req.Context(), id, b.Reviewer, b.Notes)
		case "needs_more_evidence":
			err = store.NeedsMoreEvidenceSpeakerReviewTask(req.Context(), id, b.Reviewer, b.Notes)
		default:
			err = fmt.Errorf("unsupported action")
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
