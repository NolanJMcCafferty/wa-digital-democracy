# Railway Service Notes

## API

The API service uses the default final image in the root `Dockerfile` and the
root `railway.json` config:

```sh
wa-dd-api
```

It reads:

```txt
PORT
DATABASE_URL
```

`DATABASE_URL` is accepted anywhere the local code previously used `WADD_DSN`.

Health check:

```txt
/healthz
```

## Web

The web service uses root directory `apps/web`, config path
`/apps/web/railway.json`, and builds from `apps/web/Dockerfile`. Set the API
URL in all three forms for now so both server-side fetches and rewrites behave
predictably:

```txt
WADD_API_URL
API_BASE_URL
NEXT_PUBLIC_API_URL
NEXT_PUBLIC_SITE_URL
```

## Cron

Use the default final image in the root `Dockerfile` and the start command in
`infra/railway/config/daily.railway.json`:

```sh
wa-dd daily --biennium "${BIENNIUM:-2025-26}" ${DAILY_EXTRA_ARGS:-}
```

For a smoke test, set this service variable temporarily:

```txt
DAILY_EXTRA_ARGS=--session-limit 25 --hearing-limit 5
```

The `daily` command holds a Postgres advisory lock for the full chain, so an
overlapping cron run exits successfully without doing duplicate work.

## Migrations

Use the default final image in the root `Dockerfile` and the start command in
`infra/railway/config/migrate.railway.json`:

```sh
goose -dir /app/db/migrations postgres "$DATABASE_URL" up
```

Run the migration service manually before enabling cron.
