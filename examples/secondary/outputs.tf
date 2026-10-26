output "platform_contract" {
  description = "Platform contract for standalone tenants"
  value       = module.main.platform_contract
}

output "monthly_cost_estimate" {
  description = "Estimated idle monthly cost in USD"
  value       = module.main.monthly_cost_estimate
}
