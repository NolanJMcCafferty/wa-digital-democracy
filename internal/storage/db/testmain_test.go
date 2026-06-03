package db_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

// Test harness for the db package.
//
// TestMain spins up a Postgres container (postgis image — matches infra/docker-compose.yml),
// runs all goose migrations once, snapshots the result, and exposes the connection
// string. Each test calls newTestStore(t), which restores the snapshot in t.Cleanup,
// so tests see a clean post-migration database without re-running migrations.
//
// If Docker is not available (CI sandbox, etc.) tests skip with t.Skip rather than
// failing — the pure-helper tests in queries_helpers_test.go still run.

var (
	pgContainer   *tcpostgres.PostgresContainer
	pgConnString  string
	pgSetupErr    error
	pgSetupOnce   sync.Once
	dockerSkipMsg string
)

func TestMain(m *testing.M) {
	// Skip container setup entirely if WADD_SKIP_DB_TESTS is set (useful for
	// running just the pure-helper unit tests without Docker).
	if os.Getenv("WADD_SKIP_DB_TESTS") == "" {
		setupContainer()
	} else {
		dockerSkipMsg = "WADD_SKIP_DB_TESTS set"
	}

	code := m.Run()

	if pgContainer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_ = pgContainer.Terminate(ctx)
		cancel()
	}
	os.Exit(code)
}

func setupContainer() {
	pgSetupOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		c, err := tcpostgres.Run(ctx,
			"postgis/postgis:16-3.5-alpine",
			tcpostgres.WithDatabase("wa_dd_test"),
			tcpostgres.WithUsername("wadd"),
			tcpostgres.WithPassword("wadd"),
			tcpostgres.WithSQLDriver("pgx"),
			testcontainers.WithWaitStrategy(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(2*time.Minute),
			),
		)
		if err != nil {
			pgSetupErr = fmt.Errorf("start container: %w", err)
			dockerSkipMsg = pgSetupErr.Error()
			return
		}
		pgContainer = c

		dsn, err := c.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			pgSetupErr = fmt.Errorf("connection string: %w", err)
			return
		}
		pgConnString = dsn

		if err := runMigrations(ctx, dsn); err != nil {
			pgSetupErr = fmt.Errorf("migrations: %w", err)
			return
		}
		if err := c.Snapshot(ctx); err != nil {
			pgSetupErr = fmt.Errorf("snapshot: %w", err)
			return
		}
		log.Printf("test postgres ready at %s (migrations applied, snapshot taken)", dsn)
	})
}

func runMigrations(ctx context.Context, dsn string) error {
	migrationsDir, err := findMigrationsDir()
	if err != nil {
		return err
	}
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	goose.ResetGlobalMigrations()
	return goose.UpContext(ctx, sqlDB, migrationsDir)
}

// findMigrationsDir resolves <repo-root>/db/migrations relative to this test file.
func findMigrationsDir() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("runtime.Caller failed")
	}
	// internal/storage/db/testmain_test.go → repo root is three dirs up.
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	dir := filepath.Join(root, "db", "migrations")
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("migrations dir %s: %w", dir, err)
	}
	return dir, nil
}

// newTestStore returns a *db.Store wired to the shared test container, with the
// snapshot restored in t.Cleanup so every test starts from a freshly-migrated DB.
//
// Tests that need this helper run sequentially within the package — Restore
// drops/recreates the database, which is incompatible with t.Parallel().
func newTestStore(t *testing.T) *db.Store {
	t.Helper()
	if pgContainer == nil {
		if dockerSkipMsg != "" {
			t.Skipf("skipping: %s", dockerSkipMsg)
		}
		t.Fatalf("test container not initialized: %v", pgSetupErr)
	}
	ctx := context.Background()
	store, err := db.Open(ctx, pgConnString)
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() {
		store.Close()
		// Restore must run after the pool is closed — Restore drops connections.
		rctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := pgContainer.Restore(rctx); err != nil {
			t.Errorf("snapshot restore: %v", err)
		}
	})
	return store
}
