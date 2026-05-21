output "railway_project_id" {
  value = railway_project.wadd.id
}

output "railway_environment_id" {
  value = local.environment_id
}

output "service_ids" {
  value = {
    postgis = railway_service.postgis.id
    api     = railway_service.api.id
    web     = railway_service.web.id
    migrate = railway_service.migrate.id
    daily   = railway_service.daily.id
  }
}

output "api_public_url" {
  value = local.api_public_url
}

output "web_public_url" {
  value = local.web_public_url
}

output "railway_service_domains" {
  value = {
    api = var.api_railway_subdomain == null ? null : railway_service_domain.api[0].domain
    web = var.web_railway_subdomain == null ? null : railway_service_domain.web[0].domain
  }
}

output "custom_domain_dns" {
  value = {
    api = var.api_custom_domain == null ? null : {
      domain                    = railway_custom_domain.api[0].domain
      dns_record_value          = railway_custom_domain.api[0].dns_record_value
      verification_host_label   = railway_custom_domain.api[0].verification_host_label
      verification_record_value = railway_custom_domain.api[0].verification_record_value
    }
    web = var.web_custom_domain == null ? null : {
      domain                    = railway_custom_domain.web[0].domain
      dns_record_value          = railway_custom_domain.web[0].dns_record_value
      verification_host_label   = railway_custom_domain.web[0].verification_host_label
      verification_record_value = railway_custom_domain.web[0].verification_record_value
    }
  }
}

output "r2_bucket_raw" {
  value = var.r2_bucket_raw
}

output "r2_endpoint_url" {
  value = local.r2_endpoint
}

output "database_url_reference" {
  value     = local.database_url
  sensitive = true
}
