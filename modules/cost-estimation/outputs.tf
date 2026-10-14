output "monthly_cost_estimate" {
  description = "Idle fixed monthly cost estimate in USD, including compliance costs"
  value       = local.pricing_enabled ? local.total_monthly_cost : 0
}

output "cost_breakdown" {
  description = "Detailed breakdown of monthly costs by service"
  value       = local.pricing_enabled ? local.costs : {}
}

output "compliance_monthly_cost" {
  description = "Portion of monthly_cost_estimate caused by compliance designations, in USD"
  value       = local.pricing_enabled ? local.compliance_monthly_cost : 0
}

output "usage_rates" {
  description = "Usage-based list prices in USD for estimating data-dependent costs: S3 Tables storage per GB-month (Standard and Intelligent-Tiering tiers), monitoring and maintenance per object, Athena per TB scanned, and S3 Standard per GB-month"
  value       = local.pricing_enabled ? local.usage_rates : {}
}
