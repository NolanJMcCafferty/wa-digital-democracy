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
