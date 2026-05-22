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
WADD_INTERNAL_API_TOKEN
```

`DATABASE_URL` is accepted anywhere the local code previously used `WADD_DSN`.
`WADD_INTERNAL_API_TOKEN` is a shared server-only bearer token required by all
Go `/api/v1/*` endpoints; set the same value on the web service so the Next.js
server can call/proxy API requests.

Health check:

```txt
/healthz
```

`/healthz` is a container liveness check and does not require Postgres to be
ready. Use `/readyz` when you specifically want to verify database
connectivity.

## Web

The web service uses root directory `apps/web`, config path
`/apps/web/railway.json`, and builds from `apps/web/Dockerfile`. Set the API URL and internal token for server-side API calls/proxying:

```txt
WADD_API_URL
API_BASE_URL
WADD_INTERNAL_API_TOKEN
NEXT_PUBLIC_SITE_URL
```

Prefer Railway private networking for `WADD_API_URL` / `API_BASE_URL`, e.g.
`http://${{api.RAILWAY_PRIVATE_DOMAIN}}:8080`, so normal web→API traffic stays
inside the project network. Do not set or use a `NEXT_PUBLIC_*` API token.
Browser-originated `/api/v1/*` requests go through the Next.js route proxy,
which injects the bearer token server-side.

## Cron

Use the default final image in the root `Dockerfile` and the start command in
`infra/railway/config/daily.railway.json`:

```sh
wa-dd daily --biennium "${BIENNIUM:-2025-26}"
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
