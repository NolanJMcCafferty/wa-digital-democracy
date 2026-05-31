package main

import (
	"context"
	"net/http"
	"sync"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func healthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

type storeProvider struct {
	dsn   string
	mu    sync.RWMutex
	store *db.Store
}

func newStoreProvider(dsn string) *storeProvider {
	return &storeProvider{dsn: dsn}
}

func (p *storeProvider) Get(ctx context.Context) (*db.Store, error) {
	p.mu.RLock()
	store := p.store
	p.mu.RUnlock()
	if store != nil {
		return store, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.store != nil {
		return p.store, nil
	}

	store, err := db.Open(ctx, p.dsn)
	if err != nil {
		return nil, err
	}
	p.store = store
	return store, nil
}

func (p *storeProvider) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.store != nil {
		p.store.Close()
	}
}

func withStore(provider *storeProvider, next func(*db.Store) http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		store, err := provider.Get(req.Context())
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
			return
		}
		next(store)(w, req)
	}
}

func readinessHandler(provider *storeProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		store, err := provider.Get(req.Context())
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unhealthy", "error": err.Error()})
			return
		}
		if err := store.Pool.Ping(req.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unhealthy", "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
