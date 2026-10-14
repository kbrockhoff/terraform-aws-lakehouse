output "platform_policy_json" {
  description = "Rendered platform-scope deployer policy"
  value       = module.platform.policy_json
}

output "tenant_policy_json" {
  description = "Rendered tenant-scope deployer policy"
  value       = module.tenant.policy_json
}
