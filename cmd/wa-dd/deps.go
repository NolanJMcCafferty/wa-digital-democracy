package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/domain"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/jobs"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/csi"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/lws"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/pdc"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/tvw"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/objectstore"
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
	pdcClient  *pdc.Client
	lwsClient  *lws.Client
	csiClient  *csi.Client
	tvwClient  *tvw.Client
}

func newBuildDeps(ctx context.Context, dsn, rawDir string, rateLimit float64) (*buildDeps, func(), error) {
	store, err := db.Open(ctx, dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("db open: %w", err)
	}
	cleanup := func() { store.Close() }

	objs, err := objectstore.NewConfigured(ctx, rawDir)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("objectstore: %w", err)
	}
	sink := db.RawSink{Store: store, Objects: objs, TransformVersion: "v0"}

	embedderKey := os.Getenv("INVINTUS_EMBEDDER_KEY")
	if embedderKey == "" {
		cleanup()
		return nil, nil, fmt.Errorf("INVINTUS_EMBEDDER_KEY is required for the TVW step")
	}

	httpClient := httpx.New(httpx.Config{
		UserAgent:    userAgent,
		Sink:         sink,
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
		pdcClient:  pdc.New(httpClient, os.Getenv("SOCRATA_APP_TOKEN")),
		lwsClient:  lws.New(httpClient),
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

func newMetadataDeps(ctx context.Context, dsn, rawDir string, rateLimit float64) (*metadataDeps, func(), error) {
	store, err := db.Open(ctx, dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("db open: %w", err)
	}
	cleanup := func() { store.Close() }

	objs, err := objectstore.NewConfigured(ctx, rawDir)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("objectstore: %w", err)
	}
	sink := db.RawSink{Store: store, Objects: objs, TransformVersion: "v0"}

	httpClient := httpx.New(httpx.Config{
		UserAgent:    userAgent,
		Sink:         sink,
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

// ingestOne runs the full hearing-ingestion pipeline for one agenda item.
// Page JSON is assembled on demand by wa-dd-api; this routine only writes
// normalized source-linked records to Postgres.
func ingestOne(ctx context.Context, deps *buildDeps, demo *domain.BillAgendaTarget, logf func(string)) error {
	pipeline := &jobs.Pipeline{
		Store: deps.store,
		LWS:   deps.lwsClient,
		CSI:   deps.csiClient,
		TVW:   deps.tvwClient,
		PDC:   deps.pdcClient,
		Demo:  demo,
	}
	ids := jobs.NewIDs()
	return pipeline.Run(ctx, logf, ids)
}

type discoveryDeps struct {
	store      *db.Store
	httpClient *httpx.Client
	csiClient  *csi.Client
	tvwClient  *tvw.Client
}

func newDiscoveryDeps(ctx context.Context, dsn, rawDir string, rateLimit float64) (*discoveryDeps, func(), error) {
	store, err := db.Open(ctx, dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("db open: %w", err)
	}
	cleanup := func() { store.Close() }

	objs, err := objectstore.NewConfigured(ctx, rawDir)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("objectstore: %w", err)
	}
	sink := db.RawSink{Store: store, Objects: objs, TransformVersion: "v0"}

	httpClient := httpx.New(httpx.Config{
		UserAgent:    userAgent,
		Sink:         sink,
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
