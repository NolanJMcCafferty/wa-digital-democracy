# Testing posture

The repo has three explicit test lanes. Keep new coverage in the narrowest lane that proves the behavior. This follows the standard test-pyramid tradeoff: lots of cheap, deterministic package tests; fewer backend integration tests; and a small number of full end-to-end tests for the flows that must work as a product.

## Test lanes

| Lane | Purpose | Scope | Must not | Command | CI |
| --- | --- | --- | --- | --- | --- |
| Package tests | File/function-level correctness plus isolated store behavior | Pure functions, parsers, mappers, middleware with mocks/`httptest`; `internal/storage/db` uses a PostGIS testcontainer | External network, long sleeps, browser | `make test` | `.github/workflows/integration-tests.yml` |
| Backend integration | Whole backend behavior with real dependencies | Go API/storage/source code against real Postgres or controlled upstreams | Start the frontend or browser | `make integration` | `.github/workflows/integration-tests.yml` |
| End-to-end | Product behavior through real services | Browser/client → Next.js frontend → Go API → Postgres | Reach directly into implementation details when user-visible checks work | `make e2e` | `.github/workflows/e2e-tests.yml` |

## Principles

1. **Prefer the smallest useful test.** If a unit test proves the behavior, do not add an e2e test. E2e tests are valuable but expensive and flake-prone.
2. **Every test should be deterministic.** Avoid wall-clock sleeps, random data without a stable seed, order-dependent assertions, and live third-party services unless the test is explicitly a live-source integration test.
3. **Control test data.** Database-backed tests should create or seed the records they need, then clean them up or run against an ephemeral database. Do not depend on Nolan's local daily-ingest state.
4. **Test behavior, not implementation details.** Especially in Playwright, prefer visible text, accessible roles, URLs, and API responses over CSS selectors or component internals.
5. **Keep tests isolated.** Tests should pass when run alone, in parallel, or in a different order.
6. **Treat flakiness as a bug.** Do not paper over flakes with arbitrary sleeps. Tighten data setup, assertions, readiness checks, or test boundaries.
7. **No secrets in tests.** Use synthetic tokens (`e2e-internal-token`), generated JWT keys, and local service containers.

## 1. Package tests

**Purpose:** file/function-level behavior with no real network services, plus isolated store behavior for `internal/storage/db`.

**Naming/location:** normal Go `*_test.go` files without build tags, colocated with the package under test. Frontend unit tests should use `*.test.ts` / `*.test.tsx` when a JS test runner is added.

**Run locally:**

```sh
make test
```

`make test` runs `go test ./...`. Most packages are pure unit tests. The
`internal/storage/db` package starts a PostGIS testcontainer, runs Goose
migrations, snapshots the database, and restores the snapshot around store-level
tests. Set `WADD_SKIP_DB_TESTS=1` only when Docker is unavailable and you
intentionally want to skip those DB tests.

**CI:** `.github/workflows/integration-tests.yml` runs backend tests and `make coverage`.

**Good unit-test candidates:**

- parsers and normalizers
- role/permission helpers
- JWT claim validation with generated local keys
- HTTP middleware using `httptest`
- query-parameter mapping and response-shape mapping

**Package-test rules:**

- No browser, Clerk, live WA Legislature calls, or external network.
- Keep Postgres/Docker usage scoped to the `internal/storage/db` testcontainer harness unless there is a clear reason to promote the test to the integration lane.
- Use table-driven tests for edge cases.
- Use `t.Parallel()` only when the test has no shared global state or environment-variable mutation.
- When testing env-var behavior, use `t.Setenv` and avoid `t.Parallel()`.

Examples:

- `cmd/wa-dd-api/main_test.go`
- `internal/*/*_test.go`

## 2. Backend integration tests

**Purpose:** backend flows that require a real Postgres database, live external source checks, or multiple backend packages wired together. These tests do **not** start the frontend.

**Naming/location:** `*_integration_test.go` or `integration_test.go` with:

```go
//go:build integration
// +build integration
```

**Run locally:**

```sh
make integration
```

This starts the local PostGIS/Postgres service, applies migrations, and runs `go test -tags=integration ./...`.

If you already have a test database, override it with:

```sh
WADD_TEST_DSN='postgres://...' make integration
```

**Integration-test rules:**

- Migrations must run before tests.
- Tests should create the records they need; do not require seeded production-like data unless the test explicitly skips when fixtures are absent.
- Prefer unique synthetic IDs / bienniums / hashes and cleanup with `t.Cleanup`.
- Use real SQL and real `db.Store`; avoid mocking the layer whose integration you are trying to prove.
- For live upstream-source tests, keep them behind the `integration` build tag and make failures diagnostically clear.
- If a test needs an external HTTP dependency that we do control, use `httptest.Server` rather than the public internet.

Examples:

- `cmd/wa-dd-api/integration_test.go`
- `internal/storage/db/integration_test.go`
- `internal/storage/db/queries_integration_test.go`
- `internal/sources/lws/integration_test.go`

The `internal/storage/db` package already uses Testcontainers for isolated
store-level tests. Broader integration tests still use Make/Compose locally and
GitHub service containers in CI.

## 3. End-to-end tests

**Purpose:** full product flow through a real frontend server and a real backend server, with a real database. Browser/client traffic should enter through the frontend and reach the Go API through the same proxy/server-component paths production uses.

**Naming/location:** `apps/web/e2e/*.spec.ts` using Playwright.

**Run locally:**

```sh
make e2e
```

By default this target builds the local Go API binary and Next.js app, seeds `db/fixtures/minimal.sql`, runs Playwright with the API on `:18080` and the Next.js production server on `:13000`, then removes the fixture rows before exiting.

`make integration` follows the same fixture boundary for backend integration
tests: seed before `go test -tags=integration`, then clean deterministic rows on
exit. If you run `seed-test-fixtures.sh` manually for a single test, pair it
with `scripts/cleanup-test-fixtures.sh --dsn <postgres-url>` before returning to
normal local development.

For e2e against an existing real/staging database, point the normal local API
and Next.js test servers at that DSN:

```sh
WADD_E2E_DSN='postgres://...' make e2e
```

The `make e2e` target seeds and cleans the DSN you provide, but it does not
create or migrate that database. It runs Playwright against local API/Next.js
servers using ports `:18080` and `:13000` by default.

Use `make e2e-install` once to install Playwright browsers. If your local OS is newer than Playwright's downloadable browser support matrix but Chrome is installed, run with:

```sh
PLAYWRIGHT_CHROMIUM_CHANNEL=chrome make e2e
```

**Playwright rules:**

- Test user-visible behavior: `getByRole`, `getByLabel`, `getByText`, and URL assertions before CSS/XPath.
- Use Playwright web-first assertions: `await expect(locator).toBeVisible()` instead of `expect(await locator.isVisible()).toBe(true)`.
- Keep each test independent; no test should rely on a previous test's browser state or database mutations.
- Do not test third-party websites. Mock or avoid external dependencies unless the point of the test is explicitly our integration with that service.
- Prefer a few high-value smoke/regression flows over broad UI snapshot coverage.
- Capture traces on retry; do not commit `test-results/` or `playwright-report/`.

**Good e2e candidates:**

- public pages load through Next server components and the internal API token path
- browser calls to `/api/v1/*` are proxied by Next and authenticated to Go
- admin pages redirect unauthenticated users to Clerk sign-in
- admin API mutations reject cross-origin requests
- one happy-path admin review flow once we have deterministic Clerk/session fixtures

## CI expectations

- Package tests should be fast and required on every PR.
- Backend integration and e2e tests use GitHub Actions service containers with PostGIS/Postgres and health checks.
- E2e runs upload Playwright traces/reports on failure.
- Keep workflow databases empty by default; tests must set up their own data or tolerate empty state.

## Quick decision rule

- Pure function, parser, mapper, middleware edge case using mocks/`httptest` only → **package test**.
- Backend behavior needing Postgres or live upstream source → **backend integration**.
- User-visible route/page/API proxy flow through running Next.js + running Go API → **end-to-end**.

## Sources informing this guide

- Google Testing Blog: test-pyramid guidance — many small unit tests, some integration tests, few e2e tests.
- Playwright best-practices docs: user-visible behavior, isolated tests, controlled data, locators, and web-first assertions.
- GitHub Actions docs: PostgreSQL service containers with health checks for CI integration tests.
- Testcontainers for Go docs: containerized dependencies are a good option when per-run dependency lifecycle needs to move into Go code.
