# Railway Deployment

Railway is the first hosted demo target. Keep it disposable: the durable
architecture is containers, Postgres migrations, env vars, and R2/S3 object
storage.

## Services

Create four Railway services from this repo:

```txt
web
  root: apps/web
  dockerfile: apps/web/Dockerfile
  command: default
  variables: WADD_API_URL/API_BASE_URL/NEXT_PUBLIC_API_URL, NEXT_PUBLIC_SITE_URL

api
  root: repo root
  dockerfile: Dockerfile
  target: api
  command: default
  variables: DATABASE_URL, PORT, LOG_LEVEL

migrate
  root: repo root
  dockerfile: Dockerfile
  target: migrate
  command: default
  run manually before API/web deploys that need new schema

daily
  root: repo root
  dockerfile: Dockerfile
  target: cli
  type: cron
  schedule: manual first, then daily
  command: daily --biennium 2025-26
  variables: DATABASE_URL, OBJECT_STORE=s3, S3_*, INVINTUS_EMBEDDER_KEY
```

Use a PostGIS-capable Postgres service/template. The migrations enable
`postgis`, `pg_trgm`, and `unaccent`; a non-PostGIS Postgres image will fail
when migration `0022_enable_postgis_and_job_locks.sql` runs.

## First Run

1. Create the R2 bucket and S3 API token.
2. Add variables from `variables.example.env`.
3. Deploy/run `migrate`; its default command applies all migrations.

4. Deploy `api`, verify `/healthz`.
5. Deploy `web`, verify public pages render through the API.
6. Run `daily` manually once with a small smoke limit:

```sh
wa-dd daily --biennium 2025-26 --session-limit 25 --hearing-limit 5
```

7. Remove limits and enable the cron schedule.

## Readiness Gates

- Migrations run cleanly from an empty Railway DB.
- `postgis`, `pg_trgm`, and `unaccent` are installed.
- `api` can reach Postgres through `DATABASE_URL`.
- `daily` exits cleanly and writes raw artifacts to R2.
- A second overlapping `daily` exits without running because of the advisory lock.
- A `pg_dump --format=custom --no-owner --no-acl "$DATABASE_URL"` backup can be created.
- A backup can be restored into a fresh local or alternate provider DB.
