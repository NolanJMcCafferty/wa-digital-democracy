// Command wa-dd-api is the read-only HTTP API that the Next.js frontend
// reads to render page-specific JSON objects from Postgres.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func main() {
	var (
		addr = flag.String("addr", defaultAddr(), "HTTP listen address")
		dsn  = flag.String("dsn", env("WADD_DSN", "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"), "Postgres connection string")
	)
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	stores := newStoreProvider(*dsn)
	defer stores.Close()

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	r.Get("/healthz", healthHandler())
	r.Get("/readyz", readinessHandler(stores))

	apiAuth := internalAPIAuthMiddleware(env("WADD_INTERNAL_API_TOKEN", ""))
	adminAuth, err := newAdminAuthenticator(ctx, adminAuthConfigFromEnv())
	if err != nil {
		log.Printf("admin auth disabled: %v", err)
		adminAuth = func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "admin authentication is not configured"})
			})
		}
	}
	r.Route("/api/v1", func(api chi.Router) {
		api.Use(apiAuth)
		api.Get("/addresses/suggest", suggestAddressesHandler())
		api.Get("/bills", withStore(stores, listBillsHandler))
		api.Get("/bills/{biennium}/{billNumber}/page", withStore(stores, billPageHandler))
		// Back-compat alias for older frontend/code paths. Returns the same
		// page-level shape as /page; despite the historical name, this is no
		// longer a generic legacy snapshot endpoint.
		api.Get("/bills/{biennium}/{billNumber}/first-page", withStore(stores, billPageHandler))
		api.Get("/legislators", withStore(stores, listLegislatorsHandler))
		api.Get("/legislators/lookup", withStore(stores, lookupLegislatorsByAddressHandler))
		api.Get("/legislators/{slug}", withStore(stores, getLegislatorHandler))
		api.Get("/organizations", withStore(stores, listOrganizationsHandler))
		api.Get("/organizations/{slug}", withStore(stores, getOrganizationHandler))
		api.Get("/hearings", withStore(stores, listHearingsHandler))
		api.Get("/hearings/{hearingId}", withStore(stores, getHearingHandler))
		api.Get("/sources", withStore(stores, listSourcesHandler))
		api.Get("/search/transcripts", withStore(stores, searchTranscriptsHandler))
		api.Route("/admin", func(admin chi.Router) {
			admin.Use(adminAuth)
			admin.Group(func(view chi.Router) {
				view.Use(requireAdminRole(adminRoleViewer))
				view.Get("/review/speakers", withStore(stores, adminListSpeakerReviewTasksHandler))
				view.Get("/review/speakers/events", withStore(stores, adminListSpeakerReviewEventsHandler))
				view.Get("/review/speakers/events/{tvwEventId}", withStore(stores, adminGetSpeakerReviewEventHandler))
				view.Get("/review/speakers/clusters/{clusterId}", withStore(stores, adminGetSpeakerClusterReviewHandler))
				view.Get("/review/speakers/{taskId}", withStore(stores, adminGetSpeakerReviewTaskHandler))
				view.Get("/review/entities/candidates", withStore(stores, adminListEntityMatchCandidatesHandler))
			})
			admin.Group(func(write chi.Router) {
				write.Use(requireAdminRole(adminRoleReviewer))
				write.Post("/review/speakers/clusters/{clusterId}/assign", withStore(stores, adminManualAssignSpeakerClusterHandler))
				write.Post("/review/speakers/{taskId}/accept", withStore(stores, func(store *db.Store) http.HandlerFunc {
					return adminSpeakerReviewDecisionHandler(store, "accept")
				}))
				write.Post("/review/speakers/{taskId}/reject", withStore(stores, func(store *db.Store) http.HandlerFunc {
					return adminSpeakerReviewDecisionHandler(store, "reject")
				}))
				write.Post("/review/speakers/{taskId}/needs-more-evidence", withStore(stores, func(store *db.Store) http.HandlerFunc {
					return adminSpeakerReviewDecisionHandler(store, "needs_more_evidence")
				}))
				write.Post("/review/entities/candidates/{candidateId}/decide", withStore(stores, adminDecideEntityMatchHandler))
			})
		})
	})

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
