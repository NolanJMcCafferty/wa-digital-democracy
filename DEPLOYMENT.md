# Deployment

## Railway deployment

Railway is the first hosted demo target. The deploy contract is:

- `infra/railway/terraform/` is the reproducible infra setup for Railway
  services, variables, domains, cron, and the optional R2 bucket.
- `.github/workflows/deploy-railway.yml` runs Terraform plan/apply from
  GitHub Actions; pushes to `main` apply production infrastructure.
- `apps/web/Dockerfile` for the Next.js web service.
- root `Dockerfile` default Railway image for `wa-dd-api`, `wa-dd`, and
  `goose`.
- cron jobs use the same root image, with start command
  `wa-dd daily --biennium 2025-26`.
- migrations use the same root image, with `goose -dir /app/db/migrations ...`.
- Cloudflare R2 via `OBJECT_STORE=s3` and `S3_*` variables for raw source
  artifacts.

See `infra/railway/README.md` and `infra/railway/variables.example.env` for
service layout and required variables. Hosted code accepts `DATABASE_URL` as the
production alias for local `WADD_DSN`; the API also honors Railway's `PORT`.
