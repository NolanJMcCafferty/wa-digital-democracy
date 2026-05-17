//go:build integration
// +build integration

package db_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/sources/httpx"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/objectstore"
)

// Run with:
//
//   make migrate-fresh
//   go test -tags=integration ./internal/storage/db/...
//
// Reads DSN from WADD_TEST_DSN, defaulting to the docker-compose Postgres.
func TestRawSinkRoundTrip(t *testing.T) {
	dsn := os.Getenv("WADD_TEST_DSN")
	if dsn == "" {
		dsn = "postgres://wadd:wadd@localhost:5432/wa_dd?sslmode=disable"
	}

	ctx := context.Background()
	store, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(store.Close)

	root := filepath.Join(t.TempDir(), "raw")
	objs, err := objectstore.NewFS(root)
	if err != nil {
		t.Fatalf("objectstore: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hello":"world"}`))
	}))
	defer srv.Close()

	sink := db.RawSink{Store: store, Objects: objs, TransformVersion: "v0"}
	c := httpx.New(httpx.Config{Sink: sink})

	got, err := c.Do(ctx, httpx.Request{
		System:   "lws",
		Endpoint: "TestService.Ping",
		URL:      srv.URL,
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}

	// raw file present at expected path?
	rel := filepath.Join("lws", got.Hash+".json")
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read raw: %v", err)
	}
	if string(body) != `{"hello":"world"}` {
		t.Fatalf("raw body mismatch: %q", body)
	}

	// source_record row exists with the right fields?
	var (
		sys, ep, url, ct, raw string
		hash                  string
	)
	err = store.Pool.QueryRow(ctx,
		`SELECT source_system, source_endpoint, source_url, content_type, raw_path, content_hash
		   FROM source_record
		  WHERE content_hash = $1`,
		got.Hash,
	).Scan(&sys, &ep, &url, &ct, &raw, &hash)
	if err != nil {
		t.Fatalf("select source_record: %v", err)
	}
	if sys != "lws" || ep != "TestService.Ping" || raw != rel || hash != got.Hash {
		t.Fatalf("source_record mismatch: sys=%s ep=%s raw=%s hash=%s", sys, ep, raw, hash)
	}

	// Repeat call should be idempotent on (system, hash).
	if _, err := c.Do(ctx, httpx.Request{System: "lws", Endpoint: "TestService.Ping", URL: srv.URL}); err != nil {
		t.Fatalf("second Do: %v", err)
	}
	var count int
	if err := store.Pool.QueryRow(ctx,
		`SELECT count(*) FROM source_record WHERE content_hash = $1`, got.Hash,
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("source_record count = %d, want 1 (idempotent)", count)
	}
}
