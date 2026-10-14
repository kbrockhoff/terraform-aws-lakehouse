output "namespaces" {
  description = "Tenant namespace (Lake Formation database name) per zone."
  value       = { for z, ns in aws_s3tables_namespace.this : z => ns.namespace }
}

output "catalog_ids" {
  description = "Lake Formation catalog ID (<account>:s3tablescatalog/<bucket>) holding the tenant database, per zone."
  value       = local.catalog_id
}

output "table_bucket_arns" {
  description = "Table bucket ARN holding the tenant namespace, per zone: dedicated buckets, or the shared zone buckets."
  value       = local.bucket_arn
}

output "tenant_tag_key" {
  description = "The tenant-owned LF-tag key; its single value is member."
  value       = aws_lakeformation_lf_tag.tenant.key
}

output "workgroup_name" {
  description = "Name of the tenant Athena workgroup."
  value       = aws_athena_workgroup.this.name
}

output "query_results_location" {
  description = "S3 location of the tenant's query results."
  value       = "s3://${local.results.bucket_name}/${local.results.prefix}"
}

output "kms_key_arn" {
  description = "Key encrypting the tenant's tables and results: the tenant key when designated, otherwise the platform key."
  value       = local.kms_key_arn
}

output "persona_policy_json" {
  description = "Least-privilege IAM policy JSON per persona, keyed <steward|writer|reader>/<key>. Attach to the persona roles; the module does not."
  value       = { for k, d in data.aws_iam_policy_document.persona : k => d.json }
}

output "monthly_cost_estimate" {
  description = "Estimated idle monthly cost in USD of this tenant's resources."
  value       = module.cost_estimation.monthly_cost_estimate
}

output "compliance_monthly_cost" {
  description = "Portion of the monthly estimate caused by the compliance designation (the tenant key)."
  value       = module.cost_estimation.compliance_monthly_cost
}
