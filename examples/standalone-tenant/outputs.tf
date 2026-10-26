output "monthly_cost_estimate" {
  description = "Estimated idle monthly cost in USD of this tenant"
  value       = module.tenant.monthly_cost_estimate
}

output "namespaces" {
  description = "Tenant namespace per zone"
  value       = module.tenant.namespaces
}

output "workgroup_name" {
  description = "Tenant Athena workgroup"
  value       = module.tenant.workgroup_name
}
