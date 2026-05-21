#!/usr/bin/env sh
set -eu

missing=""
for name in POSTGRES_USER POSTGRES_PASSWORD POSTGRES_DB; do
  eval "value=\${$name:-}"
  if [ -z "$value" ]; then
    missing="$missing $name"
  fi
done

if [ -n "$missing" ]; then
  echo "missing required Postgres init variables:$missing" >&2
  exit 1
fi

export PGDATA="${PGDATA:-/var/lib/postgresql/data/pgdata}"

exec docker-entrypoint.sh "$@"
