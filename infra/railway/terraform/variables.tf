variable "project_name" {
  description = "Railway project name."
  type        = string
  default     = "wa-digital-democracy"
}

variable "environment_name" {
  description = "Railway default environment name."
  type        = string
  default     = "production"
}

variable "workspace_id" {
  description = "Railway workspace ID. Required when the Railway token can access multiple workspaces."
  type        = string
  default     = null
}

variable "github_repo" {
  description = "GitHub repository in owner/name form, for example nolan-mccafferty/wa-digital-democracy."
  type        = string
}

variable "github_branch" {
  description = "Git branch Railway should deploy."
  type        = string
  default     = "main"
}

variable "postgis_user" {
  description = "Postgres user for the PostGIS service."
  type        = string
  default     = "wadd"
}

variable "postgis_db" {
  description = "Postgres database name for the app."
  type        = string
  default     = "wa_dd"
}

variable "postgis_password" {
  description = "Postgres password for the self-hosted PostGIS service."
  type        = string
  sensitive   = true
}

variable "daily_cron" {
  description = "Railway cron schedule for the ingestion service."
  type        = string
  default     = "30 3 * * *"
}

variable "daily_biennium" {
  description = "Default biennium passed to wa-dd daily."
  type        = string
  default     = "2025-26"
}

variable "cloudflare_account_id" {
  description = "Cloudflare account ID used for the R2 endpoint and optional bucket management."
  type        = string
}

variable "r2_bucket_raw" {
  description = "Cloudflare R2 bucket for immutable raw upstream responses."
  type        = string
  default     = "wa-dd-raw-prod"
}

variable "r2_bucket_location" {
  description = "Optional Cloudflare R2 bucket location. Null lets Cloudflare choose."
  type        = string
  default     = null
}

variable "r2_bucket_jurisdiction" {
  description = "Optional Cloudflare R2 jurisdiction: default, eu, or fedramp."
  type        = string
  default     = null
}

variable "r2_bucket_storage_class" {
  description = "R2 storage class for new objects."
  type        = string
  default     = "Standard"
}

variable "r2_access_key_id" {
  description = "R2 S3-compatible Access Key ID. Generate in Cloudflare R2."
  type        = string
  sensitive   = true
}

variable "r2_secret_access_key" {
  description = "R2 S3-compatible Secret Access Key. Generate in Cloudflare R2."
  type        = string
  sensitive   = true
}

variable "invintus_embedder_key" {
  description = "TVW/Invintus caption ingestion key."
  type        = string
  sensitive   = true
}

variable "socrata_app_token" {
  description = "Optional Socrata app token for data.wa.gov/PDC reads."
  type        = string
  sensitive   = true
  default     = ""
}

variable "source_user_agent" {
  description = "User-Agent for upstream public data requests."
  type        = string
  default     = "wa-dd/0.0.1 (https://github.com/nolan-mccafferty/wa-digital-democracy; contact: nolan-mccafferty)"
}

variable "api_railway_subdomain" {
  description = "Optional Railway-provided API subdomain. Leave null to skip Railway service-domain creation."
  type        = string
  default     = null
}

variable "web_railway_subdomain" {
  description = "Optional Railway-provided web subdomain. Leave null to skip Railway service-domain creation."
  type        = string
  default     = null
}

variable "api_custom_domain" {
  description = "Optional custom API domain, for example api.example.org."
  type        = string
  default     = null
}

variable "web_custom_domain" {
  description = "Optional custom web domain, for example example.org."
  type        = string
  default     = null
}

variable "api_public_url_override" {
  description = "Optional API URL override used in web environment variables."
  type        = string
  default     = null
}

variable "web_public_url_override" {
  description = "Optional web URL override used in NEXT_PUBLIC_SITE_URL."
  type        = string
  default     = null
}
