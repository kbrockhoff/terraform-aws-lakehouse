output "monthly_cost_estimate" {
  description = "Estimated idle monthly cost in USD of the platform and its tenants"
  value       = module.main.monthly_cost_estimate
}

output "platform_contract" {
  description = "Platform contract for standalone tenants"
  value       = module.main.platform_contract
}

output "tenants" {
  description = "Platform-applied tenant outputs"
  value       = module.main.tenants
}
