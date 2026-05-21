# Railway Deployment

Railway is the hosted-demo target. The deployment should be reproducible from
this repository: Terraform creates the infrastructure, and Railway builds the
app services from GitHub using the checked-in `railway.json` files.

The dashboard should be used for inspection, logs, manual one-shot runs, and
emergency fixes. It should not be the source of truth for normal setup.

## Source Of Truth

```txt
infra/railway/terraform/
  Terraform for Railway services, variables, domains, cron, and optional R2
  bucket creation.

railway.json
  API build/deploy config for the root service.

apps/web/railway.json
  Web build/deploy config for the apps/web service.

infra/railway/config/migrate.railway.json
  One-shot migration service config.

infra/railway/config/daily.railway.json
  Scheduled ingestion service config.
```

## What Terraform Creates

Terraform creates:

- Railway project and production environment.
- Railway `postgis` service from `postgis/postgis:16-3.5-alpine`.
- Railway `api` service from the GitHub repo root.
- Railway `web` service from GitHub root directory `apps/web`.
- Railway `migrate` service from the GitHub repo root.
- Railway `daily` cron service from the GitHub repo root.
- Railway service variables for database, API URLs, R2, and ingestion secrets.
- Railway-provided public service domains when subdomains are configured.
- Railway custom domain attachments when custom domains are configured.
- Cloudflare R2 raw archive bucket.

Terraform does not generate the R2 S3-compatible Access Key ID and Secret
Access Key. Cloudflare requires those to be generated in the R2 dashboard, then
passed into Terraform as sensitive variables.

Terraform also does not run database migrations. It creates the `migrate`
service; run that service manually in Railway after `terraform apply`.

## Target Topology

```txt
Railway project: wa-digital-democracy
Environment: production

  postgis
    source image: postgis/postgis:16-3.5-alpine
    volume: postgis-data mounted at /var/lib/postgresql/data
    PGDATA: /var/lib/postgresql/data/pgdata
    public domain: no

  api
    source: GitHub repo
    root directory: /
    config file: railway.json
    public domain: yes
    health check: /healthz
    command: wa-dd-api

  web
    source: GitHub repo
    root directory: apps/web
    config path: /apps/web/railway.json
    public domain: yes
    health check: /healthz
    command: pnpm start

  migrate
    source: GitHub repo
    root directory: /
    config path: /infra/railway/config/migrate.railway.json
    public domain: no
    command: goose -dir /app/db/migrations postgres "$DATABASE_URL" up
    run mode: manual

  daily
    source: GitHub repo
    root directory: /
    config path: /infra/railway/config/daily.railway.json
    public domain: no
    command: wa-dd daily --biennium "${BIENNIUM:-2025-26}"
    schedule: 30 3 * * *

Cloudflare R2
  bucket: wa-dd-raw-prod
  purpose: immutable raw upstream responses
```

## One-Time Prerequisites

Install Terraform and export provider credentials:

```sh
export RAILWAY_TOKEN=<railway-account-or-workspace-token>
export CLOUDFLARE_API_TOKEN=<cloudflare-token-with-r2-edit>
```

`CLOUDFLARE_API_TOKEN` is required because Terraform creates and manages the
raw archive R2 bucket.

Generate an R2 S3-compatible token in Cloudflare:

1. Cloudflare dashboard -> R2.
2. Manage R2 API tokens.
3. Create a token scoped to the raw bucket with object read/write access.
4. Record the Access Key ID and Secret Access Key.

The app needs those values as `S3_ACCESS_KEY_ID` and
`S3_SECRET_ACCESS_KEY`.

## Terraform Setup

```sh
cd infra/railway/terraform
cp terraform.tfvars.example terraform.tfvars
```

Edit `terraform.tfvars`:

```hcl
github_repo   = "nolan-mccafferty/wa-digital-democracy"
github_branch = "main"

# Required if the Railway token can access multiple workspaces.
workspace_id = null

postgis_password = "<long random password>"

# Pick globally unique Railway subdomains, or leave null and use URL overrides.
api_railway_subdomain = "wa-dd-api"
web_railway_subdomain = "wa-dd-web"

cloudflare_account_id = "<account id>"
r2_bucket_raw         = "wa-dd-raw-prod"
r2_access_key_id      = "<r2 access key id>"
r2_secret_access_key  = "<r2 secret access key>"

invintus_embedder_key = "<invintus key>"
socrata_app_token     = ""
```

Apply:

```sh
terraform init
terraform plan
terraform apply
```

After the first apply:

1. Open Railway.
2. Run the `migrate` service manually.
3. Verify API health:

```sh
curl -fsS "$(terraform output -raw api_public_url)/healthz"
curl -fsS "$(terraform output -raw api_public_url)/readyz"
```

4. Verify the web service:

```sh
curl -fsS "$(terraform output -raw web_public_url)" >/dev/null
```

5. Confirm logs are clean and R2 has `raw/...` objects after the first
   scheduled `daily` run.

## GitHub Actions Deployment

`.github/workflows/deploy-railway.yml` runs Terraform automatically:

- `pull_request`: backend-free init and validate only.
- `push` to `main`: init, validate, plan, and apply.
- `workflow_dispatch` on `main`: manual deploy.

The workflow uses Cloudflare R2 as the Terraform remote state backend. Create
the Terraform state bucket manually before enabling the workflow. Terraform
creates the app raw archive bucket. The workflow runs Terraform with
`-parallelism=1` because the Railway provider redeploys services after variable
writes, and Railway can rate-limit bursts of variable-triggered redeploys.

Required GitHub repository secrets:

```txt
RAILWAY_TOKEN
CLOUDFLARE_API_TOKEN
CLOUDFLARE_ACCOUNT_ID
TF_STATE_R2_BUCKET
TF_STATE_R2_KEY
TF_STATE_R2_ACCESS_KEY_ID
TF_STATE_R2_SECRET_ACCESS_KEY
POSTGIS_PASSWORD
R2_ACCESS_KEY_ID
R2_SECRET_ACCESS_KEY
INVINTUS_EMBEDDER_KEY
```

Optional GitHub repository secret:

```txt
SOCRATA_APP_TOKEN
```

Required GitHub repository variables:

```txt
API_RAILWAY_SUBDOMAIN, API_CUSTOM_DOMAIN, or API_PUBLIC_URL_OVERRIDE
WEB_RAILWAY_SUBDOMAIN, WEB_CUSTOM_DOMAIN, or WEB_PUBLIC_URL_OVERRIDE
```

Recommended GitHub repository variables:

```txt
RAILWAY_WORKSPACE_ID=<set when Railway token can access multiple workspaces>
R2_BUCKET_RAW=wa-dd-raw-prod
API_RAILWAY_SUBDOMAIN=<globally unique Railway subdomain>
WEB_RAILWAY_SUBDOMAIN=<globally unique Railway subdomain>
DAILY_BIENNIUM=2025-26
DAILY_CRON=30 3 * * *
```

## Domains

For Railway-provided domains, set:

```hcl
api_railway_subdomain = "wa-dd-api"
web_railway_subdomain = "wa-dd-web"
```

For custom domains, set:

```hcl
api_custom_domain = "api.example.org"
web_custom_domain = "example.org"
```

Then run:

```sh
terraform output custom_domain_dns
```

Add the displayed DNS records in the DNS provider. Railway may require domain
verification before the custom domains serve traffic.

## Service Variables

Terraform assigns variables by service.

```txt
api
  APP_ENV
  LOG_LEVEL
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
  S3_ENDPOINT_URL
  S3_REGION
  S3_BUCKET_RAW
  S3_ACCESS_KEY_ID
  S3_SECRET_ACCESS_KEY
  S3_FORCE_PATH_STYLE
  S3_PREFIX
  INVINTUS_EMBEDDER_KEY
  SOCRATA_APP_TOKEN
  SOURCE_USER_AGENT
  BIENNIUM
```

`DATABASE_URL` uses Railway variable references to the `postgis` service:

```txt
postgres://${{postgis.POSTGRES_USER}}:${{postgis.POSTGRES_PASSWORD}}@${{postgis.RAILWAY_PRIVATE_DOMAIN}}:5432/${{postgis.POSTGRES_DB}}?sslmode=disable
```

## PostGIS

The migration `db/migrations/0022_enable_postgis_and_job_locks.sql` runs:

```sql
CREATE EXTENSION IF NOT EXISTS postgis;
```

The Terraform-managed database uses `postgis/postgis:16-3.5-alpine` so the
extension is available. The volume is mounted at `/var/lib/postgresql/data`,
but `PGDATA` is set to `/var/lib/postgresql/data/pgdata`; mounting directly at
`PGDATA` can leave filesystem metadata such as `lost+found` in the data
directory and make `initdb` fail. Migration `0001_initial.sql` also enables
`pg_trgm` and `unaccent`.

After running `migrate`, verify:

```sql
SELECT extname
FROM pg_extension
WHERE extname IN ('postgis', 'pg_trgm', 'unaccent')
ORDER BY extname;
```

## Local Verification

Run these before pushing deployment changes:

```sh
jq empty railway.json apps/web/railway.json infra/railway/config/*.json
terraform -chdir=infra/railway/terraform init -backend=false
terraform -chdir=infra/railway/terraform validate
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

## Hosted Verification

After Railway deploy:

```sh
curl -fsS "$(terraform -chdir=infra/railway/terraform output -raw api_public_url)/healthz"
curl -fsS "$(terraform -chdir=infra/railway/terraform output -raw api_public_url)/readyz"
curl -fsS "$(terraform -chdir=infra/railway/terraform output -raw api_public_url)/api/v1/bills?limit=1"
curl -fsS "$(terraform -chdir=infra/railway/terraform output -raw web_public_url)" >/dev/null
```

Check ingestion:

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

## State And Secrets

`terraform.tfvars`, `.terraform/`, generated backend files, plan files, and
local state are ignored by git. The provider lock file is committed for
reproducible provider versions.

GitHub Actions uses the R2-backed S3 Terraform backend. The state object key is
set by the `TF_STATE_R2_KEY` secret, currently
`wa-digital-democracy/railway/terraform.tfstate`, inside `TF_STATE_R2_BUCKET`.
State contains sensitive Railway variables, including database and R2
credentials.

## Dashboard Fallback

If Terraform is blocked by a provider issue, create the same services manually
in Railway using the topology above. Keep the config paths identical, and
import the resources back into Terraform before treating the environment as
stable.

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
4. Patch and run `daily` manually after the fix.
5. Re-enable cron.
