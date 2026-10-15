# ----
# Storage and catalog
# ----

output "zone_table_bucket_arns" {
  description = "ARN of the shared table bucket per zone."
  value       = { for z, b in aws_s3tables_table_bucket.zone : z => b.arn }
}

output "catalog_name" {
  description = "Name of the federated S3 Tables Glue catalog. Empty when this deployment does not own the catalog integration or enabled = false."
  value       = var.enabled && var.catalog_integration_config.enabled ? module.catalog_integration[0].catalog_name : ""
}

output "data_access_role_arn" {
  description = "Lake Formation data-access role for S3 Tables. Secondary deployments pass this as catalog_integration_config.data_access_role_arn."
  value       = var.enabled ? local.data_access_role_arn : ""
}

output "query_results_bucket_arn" {
  description = "ARN of the shared Athena query results bucket."
  value       = var.enabled ? aws_s3_bucket.query_results[0].arn : ""
}

# ----
# Governance
# ----

output "platform_contract" {
  description = "Versioned public interface consumed by modules/tenant, both platform-applied and standalone: identifiers and effective settings only. Adding a field is a minor change; removing or renaming one is a major change. Null when enabled = false."
  value       = local.platform_contract
}

output "lf_tag_vocabulary" {
  description = "LF-tag keys and allowed values for pipeline labeling: sensitivity (ordered least to most sensitive), one yes/no key per regulation, domain, and the tenant key prefix. Null when enabled = false."
  value = var.enabled ? {
    sensitivity = {
      key    = aws_lakeformation_lf_tag.sensitivity[0].key
      values = local.sensitivity_values
      order  = "least to most sensitive"
    }
    regulations = {
      for r, t in aws_lakeformation_lf_tag.regulation : r => {
        key    = t.key
        values = ["no", "yes"]
      }
    }
    domain = {
      key    = aws_lakeformation_lf_tag.domain[0].key
      values = local.domain_values
    }
    zone = {
      key    = aws_lakeformation_lf_tag.zone[0].key
      values = aws_lakeformation_lf_tag.zone[0].values
      note   = "set by the platform on tenant databases; pipelines must not relabel it"
    }
    tenant_key_prefix = "${local.name_prefix}-tenant-"
  } : null
}

output "tenants" {
  description = "Outputs of each platform-applied tenant, keyed by tenant name."
  value = {
    for name, t in module.tenant : name => {
      namespaces              = t.namespaces
      catalog_ids             = t.catalog_ids
      table_bucket_arns       = t.table_bucket_arns
      tenant_tag_key          = t.tenant_tag_key
      workgroup_name          = t.workgroup_name
      query_results_location  = t.query_results_location
      kms_key_arn             = t.kms_key_arn
      persona_policy_json     = t.persona_policy_json
      monthly_cost_estimate   = t.monthly_cost_estimate
      compliance_monthly_cost = t.compliance_monthly_cost
    }
  }
}

# ----
# Encryption
# ----

output "kms_key_id" {
  description = "ID of the KMS key used for encryption"
  value       = var.enabled && var.encryption_config.create_kms_key ? aws_kms_key.main[0].key_id : ""
}

output "kms_key_arn" {
  description = "ARN of the platform KMS key"
  value       = local.kms_key_arn
}

output "kms_alias_name" {
  description = "Name of the KMS key alias"
  value       = var.enabled && var.encryption_config.create_kms_key ? aws_kms_alias.main[0].name : ""
}

output "required_kms_key_policy_json" {
  description = "Key policy statements the platform key needs (S3 Tables maintenance, Lake Formation data access, Athena results). Merge them into a consumer-supplied key's policy. Empty when enabled = false."
  value       = var.enabled ? data.aws_iam_policy_document.required_kms_statements[0].json : ""
}

# ----
# Monitoring
# ----

output "alarm_sns_topic_arn" {
  description = "ARN of the SNS topic used for alarm notifications"
  value       = local.alarm_sns_topic_arn
}

output "alarm_sns_topic_name" {
  description = "Name of the SNS topic used for alarm notifications"
  value       = var.enabled && local.effective_config.alarms_enabled && var.alarms_config.create_sns_topic ? aws_sns_topic.alarms[0].name : ""
}

# ----
# Pricing
# ----

output "monthly_cost_estimate" {
  description = "Estimated idle monthly cost in USD of the platform plus every platform-applied tenant. Standalone tenants report their own estimate."
  value       = local.total_monthly_cost_estimate
}

output "cost_breakdown" {
  description = "Breakdown of the platform's own monthly cost by service; tenant estimates are in the tenants output"
  value       = module.pricing.cost_breakdown
}

output "usage_rates" {
  description = "Usage-based list prices in USD (S3 Tables storage and maintenance, Athena per TB scanned, S3 Standard storage) for estimating data-dependent costs"
  value       = module.pricing.usage_rates
}
