// Command wa-dd-api is the read-only HTTP API that the Next.js frontend
// reads to render bill-hearing pages. The bundle JSON shape is the same
// one wa-dd build-bundle writes to disk; this server regenerates it per
// request from Postgres so the frontend doesn't have to read files.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/render/firstpage"
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
	r.Get("/api/v1/bills", listBillsHandler(store))
	r.Get("/api/v1/bills/{biennium}/{billNumber}/first-page", firstPageHandler(store))
	r.Get("/api/v1/legislators", listLegislatorsHandler(store))
	r.Get("/api/v1/legislators/{slug}", getLegislatorHandler(store))
	r.Get("/api/v1/organizations", listOrganizationsHandler(store))
	r.Get("/api/v1/organizations/{slug}", getOrganizationHandler(store))
	r.Get("/api/v1/hearings", listHearingsHandler(store))
	r.Get("/api/v1/hearings/{csiAgendaItemId}", getHearingHandler(store))
	r.Get("/api/v1/sources", listSourcesHandler(store))
	r.Get("/api/v1/search/transcripts", searchTranscriptsHandler(store))

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

func listBillsHandler(store *db.Store) http.HandlerFunc {
	type item struct {
		Biennium   string `json:"biennium"`
		BillPrefix string `json:"bill_prefix"`
		BillNumber int    `json:"bill_number"`
		BillID     string `json:"bill_id"`
		Title      string `json:"title,omitempty"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		bills, err := store.ListIngestedBills(req.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		out := make([]item, 0, len(bills))
		for _, b := range bills {
			out = append(out, item{
				Biennium:   b.Biennium,
				BillPrefix: b.Prefix,
				BillNumber: b.Number,
				BillID:     b.BillID,
				Title:      b.Title,
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// billSlugRe matches "HB1501", "SB6200", "HJR4002", etc. — the slug shape
// the frontend route uses (mirrored from apps/web's parseBillSlug).
var billSlugRe = regexp.MustCompile(`^([A-Za-z]+)([0-9]+)$`)

func firstPageHandler(store *db.Store) http.HandlerFunc {
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

		demo, err := firstpage.LookupSelectedDemo(req.Context(), store, biennium, prefix, number)
		if err != nil {
			if errors.Is(err, firstpage.ErrBillNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "bill not ingested"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		bundle, err := firstpage.Build(req.Context(), store, demo)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, bundle)
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
// Aggregation handlers — back the home/index pages so the frontend stops
// fanning out one /first-page request per bill.
// ---------------------------------------------------------------------------

func listLegislatorsHandler(store *db.Store) http.HandlerFunc {
	type item struct {
		Slug      string `json:"slug"`
		Name      string `json:"name"`
		Chamber   string `json:"chamber,omitempty"`
		BillCount int    `json:"bill_count"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		legs, err := store.ListLegislators(req.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		out := make([]item, 0, len(legs))
		for _, l := range legs {
			out = append(out, item{
				Slug:      slugify(l.Name),
				Name:      l.Name,
				Chamber:   l.Chamber,
				BillCount: l.BillCount,
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func getLegislatorHandler(store *db.Store) http.HandlerFunc {
	type appearance struct {
		Biennium    string `json:"biennium"`
		BillID      string `json:"bill_id"`
		BillPrefix  string `json:"bill_prefix"`
		BillNumber  int    `json:"bill_number"`
		BillTitle   string `json:"bill_title,omitempty"`
		SponsorType string `json:"sponsor_type,omitempty"`
	}
	type body struct {
		Slug        string       `json:"slug"`
		Name        string       `json:"name"`
		Chamber     string       `json:"chamber,omitempty"`
		Appearances []appearance `json:"appearances"`
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
			apps = append(apps, appearance{
				Biennium:    b.Biennium,
				BillID:      b.BillID,
				BillPrefix:  b.BillPrefix,
				BillNumber:  b.BillNumber,
				BillTitle:   b.BillTitle,
				SponsorType: b.SponsorType,
			})
		}
		writeJSON(w, http.StatusOK, body{
			Slug: slug, Name: match.Name, Chamber: match.Chamber,
			Appearances: apps,
		})
	}
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
		ContextCount    int            `json:"context_count"`
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
				ContextCount: o.ContextCount,
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
		HearingTitle    string    `json:"hearing_title"`
		CommitteeName   string    `json:"committee_name"`
		MeetingDateTime time.Time `json:"meeting_datetime"`
		Position        string    `json:"position,omitempty"`
		TestifierCount  int       `json:"testifier_count"`
	}
	type body struct {
		Slug            string         `json:"slug"`
		CanonicalName   string         `json:"canonical_name"`
		Aliases         []string       `json:"aliases"`
		MatchConfidence string         `json:"match_confidence"`
		MatchNotes      string         `json:"match_notes,omitempty"`
		TestifierCount  int            `json:"testifier_count"`
		Positions       map[string]int `json:"positions"`
		ContextCount    int            `json:"context_count"`
		Appearances     []appearance   `json:"appearances"`
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
		apps := make([]appearance, 0, len(appsRaw))
		for _, a := range appsRaw {
			apps = append(apps, appearance{
				Biennium: a.Biennium, BillID: a.BillID,
				BillPrefix: a.BillPrefix, BillNumber: a.BillNumber,
				CSIAgendaItemID: a.CSIAgendaItemID,
				HearingTitle:    a.HearingTitle,
				CommitteeName:   a.CommitteeName,
				MeetingDateTime: a.MeetingDateTime,
				Position:        a.Position,
				TestifierCount:  a.TestifierCount,
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
			ContextCount: match.ContextCount,
			Appearances:  apps,
		})
	}
}

func listHearingsHandler(store *db.Store) http.HandlerFunc {
	type item struct {
		CSIAgendaItemID string    `json:"csi_agenda_item_id"`
		AgendaItemLabel string    `json:"agenda_item_label"`
		CommitteeName   string    `json:"committee_name"`
		Chamber         string    `json:"chamber"`
		MeetingDateTime time.Time `json:"meeting_datetime"`
		Biennium        string    `json:"biennium"`
		BillID          string    `json:"bill_id"`
		BillPrefix      string    `json:"bill_prefix"`
		BillNumber      int       `json:"bill_number"`
	}
	return func(w http.ResponseWriter, req *http.Request) {
		hs, err := store.ListHearings(req.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		out := make([]item, 0, len(hs))
		for _, h := range hs {
			out = append(out, item{
				CSIAgendaItemID: h.CSIAgendaItemID,
				AgendaItemLabel: h.AgendaItemLabel,
				CommitteeName:   h.CommitteeName,
				Chamber:         h.Chamber,
				MeetingDateTime: h.MeetingDateTime,
				Biennium:        h.Biennium,
				BillID:          h.BillID,
				BillPrefix:      h.BillPrefix,
				BillNumber:      h.BillNumber,
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func getHearingHandler(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		csiID := chi.URLParam(req, "csiAgendaItemId")
		demo, err := firstpage.LookupSelectedDemoByAgendaItem(req.Context(), store, csiID)
		if err != nil {
			if errors.Is(err, firstpage.ErrBillNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "hearing not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		bundle, err := firstpage.Build(req.Context(), store, demo)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, bundle)
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

// slugify mirrors apps/web/src/lib/loadBundle.ts:slugify exactly:
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

		// Soft parsing — match the firstPageHandler style. Bad limit/offset
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
