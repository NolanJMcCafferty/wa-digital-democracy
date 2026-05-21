# Copy Local Postgres Data to Railway Production

This runbook copies the local `wa_dd` Postgres database into the Railway
`postgis` service. It is destructive: production database contents are replaced
with the local database contents.

Use this only when local is the source of truth for the same project data.

## Prerequisites

- Local Postgres is running:

```sh
make up
```

- Railway CLI is installed, authenticated, and linked to the production project:

```sh
railway login
railway link
```

- The Railway `postgis` service is running.
- The `postgis` volume has enough space for the dump and restore. If Postgres
  logs show `No space left on device`, resize the Railway volume first. For
  this project, use at least 5 GB.
- The prod `postgis` variables are correct:

```txt
POSTGRES_USER=wadd
POSTGRES_PASSWORD=wadd
POSTGRES_DB=wa_dd
PGDATA=/var/lib/postgresql/data/pgdata
```

## Verify Prod DB Is Reachable

```sh
railway ssh --service postgis -- psql \
  -U wadd \
  -d wa_dd \
  -c "select current_database(), current_user;"
```

If Railway says the container is not running, inspect the `postgis` deployment
logs and restart/redeploy the service before continuing.

## Restore Local Into Prod

Stream a plain SQL dump from the local Docker Postgres container directly into
`psql` inside the Railway `postgis` container:

```sh
docker exec wa-dd-postgres pg_dump \
  -U wadd \
  -d wa_dd \
  --no-owner \
  --no-acl \
  --clean \
  --if-exists \
| railway ssh --service postgis -- psql \
  -U wadd \
  -d wa_dd \
  -v ON_ERROR_STOP=1
```

Do not wrap the remote command in `sh -c`; Railway SSH argument handling can
split the script incorrectly. Pipe the SQL stream directly to `psql`.

## Verify The App

```sh
curl -fsS https://wa-dd-api-init.up.railway.app/readyz
curl -fsS 'https://wa-dd-api-init.up.railway.app/api/v1/bills?limit=1'
```

Then open the production web URL:

```sh
terraform -chdir=infra/railway/terraform output -raw web_public_url
```

## Troubleshooting

`password authentication failed for user "wadd"` means the database role
password inside Postgres does not match the service variables. Reset it from
inside the `postgis` container:

```sh
railway ssh --service postgis -- psql \
  -U wadd \
  -d wa_dd \
  -c "ALTER USER wadd WITH PASSWORD 'wadd';"
```

`No space left on device` means the Railway volume is too small or full. Resize
the attached `postgis` volume in Railway before retrying. If the prod DB is
disposable and already partially restored, wipe the volume after resizing, let
Postgres initialize cleanly, then rerun the restore.

If the restore is interrupted after `DROP SCHEMA`, prod will be empty or
partial. Fix the underlying issue, then rerun the restore command.
