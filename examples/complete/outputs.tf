output "monthly_cost_estimate" {
  description = "Estimated idle monthly cost in USD of the platform and its tenants"
  value       = module.main.monthly_cost_estimate
}

output "platform_contract" {
  description = "Platform contract for standalone tenants"
  value       = module.main.platform_contract
}

output "lf_tag_vocabulary" {
  description = "LF-tag keys and values for pipeline labeling"
  value       = module.main.lf_tag_vocabulary
}

output "tenants" {
  description = "Platform-applied tenant outputs"
  value       = module.main.tenants
}

output "required_kms_key_policy_json" {
  description = "Key policy statements the platform key needs"
  value       = module.main.required_kms_key_policy_json
}
