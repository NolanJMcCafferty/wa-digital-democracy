# Railway Deployment

Railway is the hosted demo target. The deploy setup is intentionally simple and
repeatable: a checked-in bootstrap script reconciles Railway's native project,
environments, services, variables, domains, and deploy triggers through the
Railway GraphQL API.

## Target Topology

```txt
Railway project: wa-digital-democracy

production environment
  branch: main
  services: postgis, api, web, migrate, daily

staging environment
  branch: staging
  services: postgis, api, web, migrate, daily
```

The two environments are isolated. Each environment gets its own PostGIS
variables, domains, and object-storage bucket/prefix values. The app services
use Railway private networking for web -> API and API/worker -> PostGIS calls.

## Source Of Truth

```txt
scripts/bootstrap-railway.mjs
  Idempotent Railway API bootstrap/reconcile script. Creates or finds the
  project, production/staging environments, services, variables, domains,
  deployment triggers, and initial deployments.

railway.json
  API build/deploy config for the root service.

apps/web/railway.json
  Web build/deploy config for the apps/web service.

infra/railway/config/postgis.railway.json
  PostGIS service build/deploy config.

infra/railway/config/migrate.railway.json
  One-shot migration service config.

infra/railway/config/daily.railway.json
  Scheduled ingestion service config.
```

## One-Time Prerequisites

Create a Railway account/workspace token and expose it as one of:

```sh
export RAILWAY_API_TOKEN=...
# or
export RAILWAY_TOKEN=...
```

If the token can access multiple workspaces, also set:

```sh
export RAILWAY_WORKSPACE_ID=...
```

Required secrets may be shared across both environments or provided with
`_PRODUCTION` / `_STAGING` suffixes:

```txt
POSTGIS_PASSWORD
WADD_INTERNAL_API_TOKEN
INVINTUS_EMBEDDER_KEY
CLERK_SECRET_KEY
NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY
CLERK_JWT_ISSUER
S3_ACCESS_KEY_ID or R2_ACCESS_KEY_ID
S3_SECRET_ACCESS_KEY or R2_SECRET_ACCESS_KEY
```

Environment-specific examples:

```txt
POSTGIS_PASSWORD_PRODUCTION
POSTGIS_PASSWORD_STAGING
S3_ACCESS_KEY_ID_PRODUCTION
S3_ACCESS_KEY_ID_STAGING
S3_SECRET_ACCESS_KEY_PRODUCTION
S3_SECRET_ACCESS_KEY_STAGING
```

Optional variables:

```txt
CLOUDFLARE_ACCOUNT_ID
SOCRATA_APP_TOKEN
CLERK_JWKS_URL
WADD_ADMIN_JWT_AUDIENCE
DAILY_BIENNIUM
DAILY_CRON
PRODUCTION_WEB_PUBLIC_URL
STAGING_WEB_PUBLIC_URL
PRODUCTION_API_PUBLIC_URL
STAGING_API_PUBLIC_URL
PRODUCTION_WEB_CUSTOM_DOMAIN
STAGING_WEB_CUSTOM_DOMAIN
PRODUCTION_API_CUSTOM_DOMAIN
STAGING_API_CUSTOM_DOMAIN
```

## Bootstrap / Deploy

Dry-run the desired topology:

```sh
node scripts/bootstrap-railway.mjs apply --dry-run
```

Reconcile Railway without triggering deployments:

```sh
make railway-bootstrap
```

Reconcile Railway and trigger deployments:

```sh
make railway-deploy
```

The deploy command creates or updates:

- Railway project `wa-digital-democracy`
- `production` environment from `main`
- `staging` environment from `staging`
- `postgis`, `api`, `web`, `migrate`, and `daily` services
- GitHub deployment triggers for each environment/service
- Railway-provided service domains for `api` and `web`
- optional custom domains when configured
- service variables in one batched write per service with deploys skipped until
  the final explicit deployment step

## GitHub Actions

`.github/workflows/deploy-railway.yml` validates the bootstrap script and JSON
config on pull requests. On pushes to `main` or `staging`, and on manual
`workflow_dispatch`, it runs the bootstrap script and triggers Railway
deployments.

The workflow expects the same secrets/variables listed above in GitHub Actions.
Prefer environment-specific secrets for production/staging isolation.

## Operational Notes

- `production` deploys from `main`; `staging` deploys from `staging`.
- `migrate` is configured as a `NEVER` restart one-shot service. It runs the
  project-pinned Goose binary against `db/migrations` and exits, rather than
  staying up as an app service.
- In deploy mode, the bootstrap script explicitly triggers deployments for
  `postgis`, `migrate`, `api`, `web`, and `daily`. Because `migrate` is a
  one-shot service, each deploy applies pending migrations and then stops.
  Trigger it intentionally before relying on services that require new schema.
- The bootstrap script intentionally uses Railway native environments rather
  than separate Railway projects.
