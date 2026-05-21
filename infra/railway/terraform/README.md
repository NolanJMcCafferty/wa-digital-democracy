# Railway Terraform

This directory is the reproducible Railway setup for Washington Digital
Democracy.

It creates:

- Cloudflare R2 bucket for raw artifacts.
- Railway project and production environment.
- Railway `postgis` service from `postgis/postgis` with a mounted volume and
  `PGDATA=/var/lib/postgresql/data/pgdata`.
- Railway `api`, `web`, `migrate`, and `daily` services from GitHub.
- Railway service variables.
- Railway public service domains, if subdomains are provided.
- Railway custom domain attachments, if custom domains are provided.
- Railway cron schedule for `daily`.

## Prerequisites

Install Terraform and export provider credentials:

```sh
export RAILWAY_TOKEN=<railway-account-or-workspace-token>
export CLOUDFLARE_API_TOKEN=<cloudflare-token-with-r2-edit>
```

Generate R2 S3-compatible access keys in the Cloudflare R2 dashboard. Terraform
can manage the bucket, but the app still needs the R2 Access Key ID and Secret
Access Key as Railway service variables.

## Configure

```sh
cd infra/railway/terraform
cp terraform.tfvars.example terraform.tfvars
```

Edit `terraform.tfvars`:

- Set `github_repo`.
- Set `workspace_id` if the Railway token can access more than one workspace.
- Set a strong `postgis_password`.
- Choose globally unique `api_railway_subdomain` and `web_railway_subdomain`,
  or set URL overrides after creating domains another way.
- Set `cloudflare_account_id`.
- Set R2 access keys.
- Set `invintus_embedder_key`.

## Apply

```sh
terraform init
terraform plan
terraform apply
```

After the first apply:

1. Run the `migrate` service manually in Railway.
2. Verify the API health check.
3. Verify the web service can reach the API.
4. Verify the first scheduled `daily` run writes raw artifacts to R2.

## Custom Domains

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

Then use `terraform output custom_domain_dns` to retrieve DNS records and add
them in your DNS provider.

## State

`backend.tf` declares an S3-compatible backend. Local runs and GitHub Actions
pass the concrete backend settings at `terraform init` time.

GitHub Actions expects an R2 bucket for Terraform state. Create that state
bucket manually, then set:

```txt
TF_STATE_R2_BUCKET
TF_STATE_R2_KEY
TF_STATE_R2_ACCESS_KEY_ID
TF_STATE_R2_SECRET_ACCESS_KEY
```

Set `TF_STATE_R2_KEY` to:

```txt
wa-digital-democracy/railway/terraform.tfstate
```

`terraform.tfvars`, `.terraform/`, generated backend files, plan files, and
local state are ignored by git. The committed `.terraform.lock.hcl` pins
provider versions.

State contains sensitive Railway variables.

## GitHub Actions

`.github/workflows/deploy-railway.yml` validates Terraform on pull requests,
then plans and applies on pushes to `main`.

Set the GitHub repository secrets and variables listed in
`infra/railway/README.md` before merging the workflow to `main`.
