#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: scripts/seed-test-fixtures.sh --dsn <postgres-url> [--fixture db/fixtures/minimal.sql]

Applies deterministic test fixture data to an already-migrated database.
The script does not create or migrate the database; use `make integration-db`
for local integration DB setup, or point --dsn at a real/staging e2e env DB.

Environment fallback:
  WADD_TEST_DSN     used when --dsn is omitted
  WADD_E2E_DSN      used when --dsn and WADD_TEST_DSN are omitted
  WADD_DSN          used last
USAGE
}

DSN="${WADD_TEST_DSN:-${WADD_E2E_DSN:-${WADD_DSN:-}}}"
FIXTURE="db/fixtures/minimal.sql"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dsn)
      DSN="${2:-}"
      shift 2
      ;;
    --fixture)
      FIXTURE="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ -z "$DSN" ]]; then
  echo "Missing database DSN. Pass --dsn or set WADD_TEST_DSN/WADD_E2E_DSN/WADD_DSN." >&2
  exit 2
fi

if [[ ! -f "$FIXTURE" ]]; then
  echo "Fixture file not found: $FIXTURE" >&2
  exit 2
fi

if command -v psql >/dev/null 2>&1; then
  psql "$DSN" -v ON_ERROR_STOP=1 -f "$FIXTURE"
  exit 0
fi

if command -v docker >/dev/null 2>&1; then
  docker run --rm \
    --network host \
    -v "$PWD:/workspace:ro" \
    -w /workspace \
    postgres:16-alpine \
    psql "$DSN" -v ON_ERROR_STOP=1 -f "$FIXTURE"
  exit 0
fi

echo "Neither psql nor docker is available; cannot apply fixture SQL." >&2
exit 127
