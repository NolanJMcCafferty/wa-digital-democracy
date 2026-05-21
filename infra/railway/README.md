# Railway Deployment

Railway is the first hosted-demo target, but only if it stays simple. The
primary setup path is the Railway dashboard plus the config files committed in
this repo. Terraform is included as an optional later path, not as the required
way to get the demo online.

## Deployment Decision

Use Railway if this happy path works:

1. Create one Railway project.
2. Add a PostGIS-capable database.
3. Add `api` from the repository root. Railway reads `railway.json`.
4. Add `web` from `apps/web` with config path `/apps/web/railway.json`.
5. Add `migrate` from the repository root with a custom config path.
6. Add `daily` from the repository root with a custom config path and cron.
7. Store raw artifacts in Cloudflare R2, not Railway disk.

If Railway cannot deploy this layout without custom platform workarounds, use a
different host for the demo. Render, Fly.io, or Cloud Run would be better than
making Railway the hard part of the project.

## Target Topology

```txt
Railway project: wa-digital-democracy
Environment: production

  postgis
    type: Railway PostGIS template/marketplace service, preferred
    purpose: durable application database

  api
    source: GitHub repo
    root directory: /
    config file: railway.json, auto-detected
    public domain: yes
    health check: /healthz
    command: wa-dd-api

  web
    source: GitHub repo
    root directory: apps/web
    config path: /apps/web/railway.json
    public domain: yes
    command: pnpm start

  migrate
    source: GitHub repo
    root directory: /
    custom config path: /infra/railway/config/migrate.railway.json
    public domain: no
    command: goose -dir /app/db/migrations postgres "$DATABASE_URL" up
    schedule: manual run only

  daily
    source: GitHub repo
    root directory: /
    custom config path: /infra/railway/config/daily.railway.json
    public domain: no
    command: wa-dd daily --biennium "${BIENNIUM:-2025-26}" ${DAILY_EXTRA_ARGS:-}
    schedule: 30 3 * * * after smoke test

Cloudflare R2
  bucket: wa-dd-raw-prod
  purpose: immutable raw upstream responses
```

## Repo Files

```txt
Dockerfile
  Default image for Railway root services.
  Contains wa-dd-api, wa-dd, goose, db/migrations/, and config/.

railway.json
  API service build/deploy config for a root-directory Railway service.

apps/web/Dockerfile
  Next.js production image.

apps/web/railway.json
  Web service build/deploy config for an apps/web root-directory service.
  Set this as the service config path: /apps/web/railway.json.

infra/railway/config/migrate.railway.json
  One-shot migration service config.

infra/railway/config/daily.railway.json
  Cron ingestion service config.

infra/railway/variables.example.env
  Copy source for service variables.

infra/railway/terraform/main.tf.example
  Optional scaffold for later IaC. Not required for the first deployment.
```

The root `Dockerfile` also has named `api`, `cli`, and `migrate` targets for
local verification. Railway should build the default final image for root
services.

## Required External Resources

### Railway

You need:

- Railway account.
- GitHub account connected to Railway.
- Railway access to this repository.
- Permission to create services, variables, domains, and cron jobs.

You do not need Terraform for the first deployment.

### Cloudflare R2

Create one bucket for production raw artifacts:

```txt
wa-dd-raw-prod
```

Create an R2 API token with object read/write access to that bucket. Record:

```txt
S3_ENDPOINT_URL=https://<account-id>.r2.cloudflarestorage.com
S3_REGION=auto
S3_BUCKET_RAW=wa-dd-raw-prod
S3_ACCESS_KEY_ID=<r2 access key>
S3_SECRET_ACCESS_KEY=<r2 secret key>
S3_FORCE_PATH_STYLE=false
S3_PREFIX=raw
```

Set `OBJECT_STORE=s3` on ingestion services. Do not use Railway service-local
disk for hosted raw artifacts; it is not the durable source archive.

### PostGIS-Capable Postgres

The migration `db/migrations/0022_enable_postgis_and_job_locks.sql` runs:

```sql
CREATE EXTENSION IF NOT EXISTS postgis;
```

Use a PostGIS-capable database:

- Preferred first demo: Railway PostGIS template/marketplace service.
- Fallback: deploy `postgis/postgis:16-3.5-alpine` with a persistent Railway
  volume.
- Not enough: plain Postgres unless `CREATE EXTENSION postgis` succeeds.

Also verify `pg_trgm` and `unaccent`; migration `0001_initial.sql` enables
both.

## Manual Setup

### 1. Create the Project

1. Railway dashboard -> New Project.
2. Choose Empty Project.
3. Name it `wa-digital-democracy`.
4. Use or create the `production` environment.

### 2. Add PostGIS

Preferred:

1. Add a PostGIS-capable Railway database template/service.
2. Wait for it to deploy.
3. Copy its private/internal `DATABASE_URL`.
4. Use that private URL for app services.

Fallback container database:

1. Add a service from Docker image `postgis/postgis:16-3.5-alpine`.
2. Attach a persistent volume:

```txt
mount path: /var/lib/postgresql/data
size: at least 10 GB for demo
```

3. Set:

```txt
POSTGRES_USER=wadd
POSTGRES_DB=wa_dd
POSTGRES_PASSWORD=<strong generated password>
```

4. Build the app-facing private URL:

```txt
postgres://wadd:<password>@<postgis-private-domain>:5432/wa_dd?sslmode=disable
```

### 3. Add the API Service

1. New service -> GitHub Repo -> this repository.
2. Service name: `api`.
3. Root directory: leave blank, or `/`.
4. Config path: leave blank. Railway should use root `railway.json`.
5. Add variables:

```txt
APP_ENV=production
LOG_LEVEL=info
PORT=8080
DATABASE_URL=<private PostGIS URL>
SOURCE_USER_AGENT=wa-dd/0.0.1 (https://github.com/nolan-mccafferty/wa-digital-democracy; contact: nolan-mccafferty)
```

6. Deploy.
7. Generate a public domain.
8. Verify:

```sh
curl -fsS https://<api-domain>/healthz
```

`DATABASE_URL` is accepted anywhere local code previously expected `WADD_DSN`.
The API also honors Railway's `PORT`.

### 4. Add the Web Service

1. New service -> GitHub Repo -> this repository.
2. Service name: `web`.
3. Root directory: `apps/web`.
4. Config path: `/apps/web/railway.json`.
5. Add variables:

```txt
WADD_API_URL=https://<api-domain>
API_BASE_URL=https://<api-domain>
NEXT_PUBLIC_API_URL=https://<api-domain>
NEXT_PUBLIC_SITE_URL=https://<web-domain after generated>
```

6. Deploy.
7. Generate a public domain.
8. Update `NEXT_PUBLIC_SITE_URL` to the final web domain.
9. Redeploy and verify:

```sh
curl -fsS https://<web-domain> >/dev/null
```

### 5. Add the Migration Service

1. New service -> GitHub Repo -> this repository.
2. Service name: `migrate`.
3. Root directory: leave blank, or `/`.
4. Custom config path: `/infra/railway/config/migrate.railway.json`.
5. Public domain: none.
6. Add variables:

```txt
DATABASE_URL=<private PostGIS URL>
```

7. Deploy/run manually.
8. Confirm logs show all migrations applied.

The configured command is:

```sh
goose -dir /app/db/migrations postgres "$DATABASE_URL" up
```

### 6. Add the Daily Cron Service

1. New service -> GitHub Repo -> this repository.
2. Service name: `daily`.
3. Root directory: leave blank, or `/`.
4. Custom config path: `/infra/railway/config/daily.railway.json`.
5. Public domain: none.
6. Configure it as a cron/scheduled service.
7. Keep the schedule disabled/manual during setup if the UI allows it.
8. Add variables:

```txt
APP_ENV=production
LOG_LEVEL=info
DATABASE_URL=<private PostGIS URL>
OBJECT_STORE=s3
S3_ENDPOINT_URL=https://<account-id>.r2.cloudflarestorage.com
S3_REGION=auto
S3_BUCKET_RAW=wa-dd-raw-prod
S3_ACCESS_KEY_ID=<secret>
S3_SECRET_ACCESS_KEY=<secret>
S3_FORCE_PATH_STYLE=false
S3_PREFIX=raw
INVINTUS_EMBEDDER_KEY=<secret>
SOCRATA_APP_TOKEN=<optional>
SOURCE_USER_AGENT=wa-dd/0.0.1 (https://github.com/nolan-mccafferty/wa-digital-democracy; contact: nolan-mccafferty)
BIENNIUM=2025-26
```

9. For first smoke, temporarily add:

```txt
DAILY_EXTRA_ARGS=--session-limit 25 --hearing-limit 5
```

10. Run the service manually.
11. Check logs and R2 for `raw/...` objects.
12. Remove `DAILY_EXTRA_ARGS`.
13. Set cron:

```cron
30 3 * * *
```

`wa-dd daily` holds a Postgres advisory lock named `wa-dd:daily`. If a run
overlaps another run, the second process exits successfully without duplicate
ingestion.

## Variable Placement

Use `infra/railway/variables.example.env` as the source checklist. The usual
split is:

```txt
api
  APP_ENV
  LOG_LEVEL
  PORT
  DATABASE_URL
  SOURCE_USER_AGENT

web
  WADD_API_URL
  API_BASE_URL
  NEXT_PUBLIC_API_URL
  NEXT_PUBLIC_SITE_URL

migrate
  DATABASE_URL

daily
  APP_ENV
  LOG_LEVEL
  DATABASE_URL
  OBJECT_STORE
  S3_*
  INVINTUS_EMBEDDER_KEY
  SOCRATA_APP_TOKEN
  SOURCE_USER_AGENT
  BIENNIUM
  DAILY_EXTRA_ARGS only during smoke tests
```

Do not commit real secrets. If Terraform is later used, treat Terraform state
as secret material because it will contain service variables.

## Config-As-Code Details

Service config files define the build and deploy behavior:

```txt
api     railway.json
web     /apps/web/railway.json
migrate /infra/railway/config/migrate.railway.json
daily   /infra/railway/config/daily.railway.json
```

They define:

- Dockerfile builder.
- Dockerfile path.
- Watch paths.
- Start command.
- Health checks for web/API.
- Restart policy.

They intentionally do not store secrets.

## Terraform Is Optional

`infra/railway/terraform/main.tf.example` is an optional scaffold for later
infrastructure-as-code. It is useful once the manual deployment proves Railway
is worth keeping.

Use it like this only after the simple path works:

```sh
cd infra/railway/terraform
cp main.tf.example main.tf
terraform init
```

Create `terraform.tfvars` locally. Do not commit it:

```hcl
github_repo           = "nolan-mccafferty/wa-digital-democracy"
github_branch         = "main"
postgis_password      = "<secret>"
daily_biennium        = "2025-26"
r2_endpoint_url       = "https://<account-id>.r2.cloudflarestorage.com"
r2_bucket_raw         = "wa-dd-raw-prod"
r2_access_key_id      = "<secret>"
r2_secret_access_key  = "<secret>"
invintus_embedder_key = "<secret>"
socrata_app_token     = ""
```

Authenticate with Railway:

```sh
export RAILWAY_TOKEN=<workspace-or-account-token>
terraform plan
terraform apply
```

What the scaffold attempts to create:

- Railway project.
- Optional `postgis` service from `postgis/postgis:16-3.5-alpine` with volume.
- `api`, `web`, `migrate`, and `daily` services.
- Service variables.
- Cron schedule for `daily`.

Terraform caveats:

- The provider is community-maintained.
- Database templates may still be easier to create in the Railway dashboard.
- Public domains and final URL variables may still need dashboard follow-up.
- State contains secrets unless you deliberately externalize them.
- Use a remote encrypted backend before treating Terraform as production
  source of truth.

## Local Verification

Run these before pushing deployment changes:

```sh
go test ./...
go vet ./...
go build ./cmd/...
cd apps/web && pnpm typecheck
cd apps/web && SKIP_BUILD_STATIC_PARAMS=1 pnpm build
docker build -t wa-dd-railway:local .
docker build -t wa-dd-web:local apps/web
docker run --rm wa-dd-railway:local wa-dd version
docker run --rm wa-dd-railway:local goose -version
```

Validate JSON config:

```sh
jq empty railway.json apps/web/railway.json infra/railway/config/*.json
```

## Hosted Verification

After Railway deploy:

```sh
curl -fsS https://<api-domain>/healthz
curl -fsS https://<api-domain>/api/v1/bills?limit=1
curl -fsS https://<web-domain> >/dev/null
```

Check DB extensions:

```sql
SELECT extname
FROM pg_extension
WHERE extname IN ('postgis', 'pg_trgm', 'unaccent')
ORDER BY extname;
```

Check ingestion smoke:

```sql
SELECT source_system, count(*)
FROM source_record
GROUP BY source_system
ORDER BY source_system;
```

Check R2 for raw objects:

```txt
raw/lws/...
raw/csi/...
raw/tvw/...
raw/invintus/...
raw/pdc_socrata/...
```

## Backup And Restore

Before enabling full cron, prove backup and restore:

```sh
pg_dump --format=custom --no-owner --no-acl "$DATABASE_URL" > railway.dump

createdb wa_dd_restore
pg_restore --clean --if-exists --no-owner --no-acl \
  --dbname "postgres://wadd:wadd@localhost:5432/wa_dd_restore?sslmode=disable" \
  railway.dump
```

Minimum restore checks:

```sql
SELECT count(*) FROM source_record;
SELECT count(*) FROM bill;
SELECT count(*) FROM hearing;
SELECT extname
FROM pg_extension
WHERE extname IN ('postgis', 'pg_trgm', 'unaccent')
ORDER BY extname;
```

## Rollback

For web/API regressions:

1. Disable `daily` cron if writes may be affected.
2. Roll back the Railway deployment for the affected service.
3. If schema migration was involved, roll forward with a fix or restore from
   backup. Do not hand-edit production schema.

For ingestion regressions:

1. Disable `daily`.
2. Inspect the latest logs.
3. Inspect `source_record` rows by `fetched_at`.
4. Patch and run a limited smoke with `DAILY_EXTRA_ARGS`.
5. Re-enable cron.

## Open Follow-Ups

- Pick Railway marketplace PostGIS or the self-hosted PostGIS container after
  the first successful deploy.
- Add custom domains and DNS.
- Add uptime checks for web and API.
- Add recurring database backups.
- Move Terraform state to a remote encrypted backend before using it as source
  of truth.
