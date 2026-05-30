# Deployment

## Railway deployment

Railway deploys are managed by `scripts/bootstrap-railway.mjs`, not by Terraform.
The script reconciles one Railway project with native `production` and `staging`
environments, creates the `postgis`, `api`, `web`, `migrate`, and `daily`
services, writes variables in batches, creates domains/triggers, and can trigger
deployments.

```sh
make railway-bootstrap # reconcile only
make railway-deploy    # reconcile and deploy
```

`production` deploys from `main`; `staging` deploys from `staging`. See
`infra/railway/README.md` for required secrets and optional custom domains.
