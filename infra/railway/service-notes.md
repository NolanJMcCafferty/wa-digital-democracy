# Railway Service Notes

## API

The API container target is `api` in the root `Dockerfile`. It reads:

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

The web service builds from `apps/web/Dockerfile`. Set the API URL in all three
forms for now so both server-side fetches and rewrites behave predictably:

```txt
WADD_API_URL
API_BASE_URL
NEXT_PUBLIC_API_URL
NEXT_PUBLIC_SITE_URL
```

## Cron

Use the root `Dockerfile` target `cli` and start command:

```sh
daily --biennium 2025-26
```

For a smoke test:

```sh
daily --biennium 2025-26 --session-limit 25 --hearing-limit 5
```

The `daily` command holds a Postgres advisory lock for the full chain, so an
overlapping cron run exits successfully without doing duplicate work.

## Migrations

Use the root `Dockerfile` target `migrate`. Its default command is:

```sh
goose -dir /app/db/migrations postgres "$DATABASE_URL" up
```

Run the migration service manually before enabling cron.
