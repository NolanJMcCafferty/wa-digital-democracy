resource "cloudflare_r2_bucket" "raw" {
  account_id    = var.cloudflare_account_id
  name          = var.r2_bucket_raw
  location      = var.r2_bucket_location
  jurisdiction  = var.r2_bucket_jurisdiction
  storage_class = var.r2_bucket_storage_class
}

resource "railway_project" "wadd" {
  name         = var.project_name
  description  = "Washington Digital Democracy hosted demo"
  private      = true
  workspace_id = var.workspace_id

  default_environment = {
    name = var.environment_name
  }
}

locals {
  environment_id = railway_project.wadd.default_environment.id

  postgis_user_url     = replace(urlencode(var.postgis_user), "+", "%20")
  postgis_password_url = replace(urlencode(var.postgis_password), "+", "%20")
  postgis_db_url       = replace(urlencode(var.postgis_db), "+", "%20")

  database_url = "postgres://${local.postgis_user_url}:${local.postgis_password_url}@$${{postgis.RAILWAY_PRIVATE_DOMAIN}}:5432/${local.postgis_db_url}?sslmode=disable"
  r2_endpoint  = "https://${var.cloudflare_account_id}.r2.cloudflarestorage.com"

  api_domain = (
    var.api_custom_domain != null ? var.api_custom_domain :
    var.api_railway_subdomain != null ? railway_service_domain.api[0].domain :
    null
  )
  web_domain = (
    var.web_custom_domain != null ? var.web_custom_domain :
    var.web_railway_subdomain != null ? railway_service_domain.web[0].domain :
    null
  )

  api_public_url = (
    var.api_public_url_override != null ? var.api_public_url_override :
    local.api_domain != null ? "https://${local.api_domain}" :
    "https://replace-after-api-domain"
  )
  web_public_url = (
    var.web_public_url_override != null ? var.web_public_url_override :
    local.web_domain != null ? "https://${local.web_domain}" :
    "https://replace-after-web-domain"
  )
}

resource "railway_service" "postgis" {
  name               = "postgis"
  project_id         = railway_project.wadd.id
  source_repo        = var.github_repo
  source_repo_branch = var.github_branch
  config_path        = "/infra/railway/config/postgis.railway.json"

  volume = {
    name       = "postgis-data-v2"
    mount_path = "/var/lib/postgresql/data"
  }
}

resource "railway_variable" "postgis" {
  for_each = {
    PGDATA            = "/var/lib/postgresql/data/pgdata"
    POSTGRES_USER     = var.postgis_user
    POSTGRES_DB       = var.postgis_db
    POSTGRES_PASSWORD = var.postgis_password
  }

  name           = each.key
  value          = each.value
  environment_id = local.environment_id
  service_id     = railway_service.postgis.id
}

resource "railway_service" "api" {
  name               = "api"
  project_id         = railway_project.wadd.id
  source_repo        = var.github_repo
  source_repo_branch = var.github_branch
}

resource "railway_service" "web" {
  name               = "web"
  project_id         = railway_project.wadd.id
  source_repo        = var.github_repo
  source_repo_branch = var.github_branch
  root_directory     = "apps/web"
  config_path        = "/apps/web/railway.json"
}

resource "railway_service" "migrate" {
  name               = "migrate"
  project_id         = railway_project.wadd.id
  source_repo        = var.github_repo
  source_repo_branch = var.github_branch
  config_path        = "/infra/railway/config/migrate.railway.json"
}

resource "railway_service" "daily" {
  name               = "daily"
  project_id         = railway_project.wadd.id
  source_repo        = var.github_repo
  source_repo_branch = var.github_branch
  config_path        = "/infra/railway/config/daily.railway.json"
  cron_schedule      = var.daily_cron
}

resource "railway_service_domain" "api" {
  count = var.api_railway_subdomain == null ? 0 : 1

  subdomain      = var.api_railway_subdomain
  environment_id = local.environment_id
  service_id     = railway_service.api.id
}

resource "railway_service_domain" "web" {
  count = var.web_railway_subdomain == null ? 0 : 1

  subdomain      = var.web_railway_subdomain
  environment_id = local.environment_id
  service_id     = railway_service.web.id
}

resource "railway_custom_domain" "api" {
  count = var.api_custom_domain == null ? 0 : 1

  domain         = var.api_custom_domain
  target_port    = var.api_port
  environment_id = local.environment_id
  service_id     = railway_service.api.id
}

resource "railway_custom_domain" "web" {
  count = var.web_custom_domain == null ? 0 : 1

  domain         = var.web_custom_domain
  target_port    = var.web_port
  environment_id = local.environment_id
  service_id     = railway_service.web.id
}

locals {
  api_vars = {
    APP_ENV                 = "production"
    LOG_LEVEL               = "info"
    DATABASE_URL            = local.database_url
    SOURCE_USER_AGENT       = var.source_user_agent
    WADD_INTERNAL_API_TOKEN = var.internal_api_token
    CLERK_JWT_ISSUER       = var.clerk_jwt_issuer
    WADD_ADMIN_JWT_AUDIENCE = var.admin_jwt_audience
  }

  optional_api_vars = {
    CLERK_JWKS_URL = var.clerk_jwks_url
  }

  effective_api_vars = merge(
    local.api_vars,
    { for key, value in local.optional_api_vars : key => value if trimspace(value) != "" },
  )

  web_vars = {
    # Use Railway private networking for server-side web→API calls. The API may
    # still have a public domain for health checks and manual diagnostics, but
    # /api/v1 requires WADD_INTERNAL_API_TOKEN either way.
    WADD_API_URL                       = "http://$${{api.RAILWAY_PRIVATE_DOMAIN}}:${var.api_port}"
    API_BASE_URL                       = "http://$${{api.RAILWAY_PRIVATE_DOMAIN}}:${var.api_port}"
    NEXT_PUBLIC_SITE_URL               = local.web_public_url
    WADD_INTERNAL_API_TOKEN            = var.internal_api_token
    NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY  = var.clerk_publishable_key
    CLERK_SECRET_KEY                   = var.clerk_secret_key
  }

  migrate_vars = {
    DATABASE_URL = local.database_url
  }

  daily_vars = {
    APP_ENV               = "production"
    LOG_LEVEL             = "info"
    DATABASE_URL          = local.database_url
    OBJECT_STORE          = "s3"
    S3_ENDPOINT_URL       = local.r2_endpoint
    S3_REGION             = "auto"
    S3_BUCKET_RAW         = var.r2_bucket_raw
    S3_ACCESS_KEY_ID      = var.r2_access_key_id
    S3_SECRET_ACCESS_KEY  = var.r2_secret_access_key
    S3_FORCE_PATH_STYLE   = "false"
    S3_PREFIX             = "raw"
    INVINTUS_EMBEDDER_KEY = var.invintus_embedder_key
    SOCRATA_APP_TOKEN     = var.socrata_app_token
    SOURCE_USER_AGENT     = var.source_user_agent
    BIENNIUM              = var.daily_biennium
  }
}

resource "railway_variable" "api" {
  for_each = local.effective_api_vars

  name           = each.key
  value          = each.value
  environment_id = local.environment_id
  service_id     = railway_service.api.id
}

resource "railway_variable" "web" {
  for_each = local.web_vars

  name           = each.key
  value          = each.value
  environment_id = local.environment_id
  service_id     = railway_service.web.id
}

resource "railway_variable" "migrate" {
  for_each = local.migrate_vars

  name           = each.key
  value          = each.value
  environment_id = local.environment_id
  service_id     = railway_service.migrate.id
}

resource "railway_variable" "daily" {
  for_each = local.daily_vars

  name           = each.key
  value          = each.value
  environment_id = local.environment_id
  service_id     = railway_service.daily.id
}
