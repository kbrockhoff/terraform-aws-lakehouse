# ----
# Pricing Calculator
# ----

module "pricing" {
  source = "./modules/cost-estimation"

  providers = {
    aws = aws.pricing
  }

  enabled = var.enabled && var.cost_estimation_config.enabled
  region  = local.region
  # Platform-only counts; tenants report through their own module.tenant estimates. The owner of
  # the account settings adds the dedicated catalog key unless one is supplied, and its requests.
  kms_key_count = (var.enabled && var.encryption_config.create_kms_key ? 1 : 0) + (
    var.enabled && local.settings_owner && local.supplied_catalog_kms_key_arn == "" ? 1 : 0
  )
  kms_request_count_per_month = var.enabled && local.settings_owner ? var.cost_estimation_config.catalog_kms_requests_per_month : 0
  table_bucket_count          = var.enabled ? length(local.zones) : 0
  s3_bucket_count             = var.enabled ? 1 : 0 # shared query results
  alarm_count                 = 0                   # alarms are per tenant
}

locals {
  # Platform plus every platform-applied tenant. Standalone tenants report through their own
  # module instance (FR-024).
  total_monthly_cost_estimate = tonumber(format("%.2f", module.pricing.monthly_cost_estimate + sum(concat(
    [0],
    [for t in module.tenant : t.monthly_cost_estimate],
  ))))
}
