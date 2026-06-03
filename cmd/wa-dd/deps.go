package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/csi"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/lws"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/tvw"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

func acquireDailyLock(ctx context.Context, dsn string) (func(), bool, error) {
	store, err := db.Open(ctx, dsn)
	if err != nil {
		return nil, false, err
	}
	var locked bool
	if err := store.Pool.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtext('wa-dd:daily')::bigint)`).Scan(&locked); err != nil {
		store.Close()
		return nil, false, err
	}
	release := func() {
		_, _ = store.Pool.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtext('wa-dd:daily')::bigint)`)
		store.Close()
	}
	return release, locked, nil
}

// buildDeps groups the long-lived process-wide dependencies that ingest-hearings needs.
// Construct once per process via newBuildDeps.
type buildDeps struct {
	store      *db.Store
	httpClient *httpx.Client
	csiClient  *csi.Client
	tvwClient  *tvw.Client
}

func newBuildDeps(ctx context.Context, dsn string, rateLimit float64) (*buildDeps, func(), error) {
	store, err := db.Open(ctx, dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("db open: %w", err)
	}
	cleanup := func() { store.Close() }

	embedderKey := os.Getenv("INVINTUS_EMBEDDER_KEY")
	if embedderKey == "" {
		cleanup()
		return nil, nil, fmt.Errorf("INVINTUS_EMBEDDER_KEY is required for the TVW step")
	}

	httpClient := httpx.New(httpx.Config{
		UserAgent:    userAgent,
		Timeout:      45 * time.Second,
		MaxRetries:   2,
		RetryBackoff: 750 * time.Millisecond,
		HostRateLimit: map[string]float64{
			"app.leg.wa.gov":            rateLimit,
			"wslwebservices.leg.wa.gov": rateLimit,
			"tvw.org":                   rateLimit,
			"api.v3.invintus.com":       rateLimit,
			"data.wa.gov":               rateLimit,
		},
	})

	return &buildDeps{
		store:      store,
		httpClient: httpClient,
		csiClient:  csi.New(httpClient),
		tvwClient:  tvw.New(httpClient, embedderKey),
	}, cleanup, nil
}

// metadataDeps is the trimmed dependency set for ingest-session: just LWS
// + storage + httpx. We don't need CSI/TVW/PDC for the metadata-only pass,
// and we don't want to require INVINTUS_EMBEDDER_KEY for it.
type metadataDeps struct {
	store      *db.Store
	httpClient *httpx.Client
	lwsClient  *lws.Client
}

func newMetadataDeps(ctx context.Context, dsn string, rateLimit float64) (*metadataDeps, func(), error) {
	store, err := db.Open(ctx, dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("db open: %w", err)
	}
	cleanup := func() { store.Close() }

	httpClient := httpx.New(httpx.Config{
		UserAgent:    userAgent,
		Timeout:      45 * time.Second,
		MaxRetries:   2,
		RetryBackoff: 750 * time.Millisecond,
		HostRateLimit: map[string]float64{
			"wslwebservices.leg.wa.gov": rateLimit,
		},
	})

	return &metadataDeps{
		store:      store,
		httpClient: httpClient,
		lwsClient:  lws.New(httpClient),
	}, cleanup, nil
}

type discoveryDeps struct {
	store      *db.Store
	httpClient *httpx.Client
	csiClient  *csi.Client
	tvwClient  *tvw.Client
}

func newDiscoveryDeps(ctx context.Context, dsn string, rateLimit float64) (*discoveryDeps, func(), error) {
	store, err := db.Open(ctx, dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("db open: %w", err)
	}
	cleanup := func() { store.Close() }

	httpClient := httpx.New(httpx.Config{
		UserAgent:    userAgent,
		Timeout:      45 * time.Second,
		MaxRetries:   2,
		RetryBackoff: 750 * time.Millisecond,
		HostRateLimit: map[string]float64{
			"app.leg.wa.gov":      rateLimit,
			"tvw.org":             rateLimit,
			"api.v3.invintus.com": rateLimit,
		},
	})

	return &discoveryDeps{
		store:      store,
		httpClient: httpClient,
		csiClient:  csi.New(httpClient),
		// Empty embedder key is fine: discovery only calls FetchWPVideoArchive,
		// which hits TVW's WP API and doesn't need it.
		tvwClient: tvw.New(httpClient, ""),
	}, cleanup, nil
}
