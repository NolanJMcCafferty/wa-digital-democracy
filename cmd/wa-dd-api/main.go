// Command wa-dd-api is the read-only HTTP API that the Next.js frontend
// reads to render page-specific JSON objects from Postgres.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
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
		mountRoutes(api, scopeAPI, stores)
		api.Route("/admin", func(admin chi.Router) {
			admin.Use(adminAuth)
			admin.Group(func(view chi.Router) {
				view.Use(requireAdminRole(adminRoleViewer))
				mountRoutes(view, scopeAdminViewer, stores)
			})
			admin.Group(func(write chi.Router) {
				write.Use(requireAdminRole(adminRoleReviewer))
				mountRoutes(write, scopeAdminReviewer, stores)
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

func mountRoutes(r chi.Router, scope routeScope, stores *storeProvider) {
	for _, rt := range routeRegistry {
		if rt.Scope != scope {
			continue
		}
		var h http.HandlerFunc
		if rt.Plain != nil {
			h = rt.Plain()
		} else {
			h = withStore(stores, rt.Store)
		}
		switch rt.Method {
		case "GET":
			r.Get(rt.Path, h)
		case "POST":
			r.Post(rt.Path, h)
		case "PUT":
			r.Put(rt.Path, h)
		case "DELETE":
			r.Delete(rt.Path, h)
		case "PATCH":
			r.Patch(rt.Path, h)
		default:
			panic(fmt.Sprintf("mountRoutes: unsupported method %q for %s", rt.Method, rt.Path))
		}
	}
}
