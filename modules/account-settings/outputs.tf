output "data_lake_settings_id" {
  description = "ID of the data-lake settings. Registration and grants wait on it so the deployer's admin rights exist first."
  value       = aws_lakeformation_data_lake_settings.this.id
}

output "catalog_kms_key_arn" {
  description = "ARN of the dedicated catalog-metadata key, created here or supplied."
  value       = local.catalog_key_arn
}

output "catalog_encryption_role_arn" {
  description = "ARN of the Glue catalog-encryption role. Empty in per_caller mode."
  value       = local.service_role_mode ? aws_iam_role.catalog_encryption[0].arn : ""
}

output "catalog_encryption_mode" {
  description = "Glue Data Catalog encryption mode: SSE-KMS-WITH-SERVICE-ROLE or SSE-KMS."
  value       = aws_glue_data_catalog_encryption_settings.this.data_catalog_encryption_settings[0].encryption_at_rest[0].catalog_encryption_mode
}

output "cross_account_version" {
  description = "Lake Formation cross-account version managed here."
  value       = var.cross_account_version
}

output "required_catalog_kms_key_policy_json" {
  description = "Key policy statements a consumer-supplied catalog key needs. Account-root access is the key owner's own responsibility."
  value       = data.aws_iam_policy_document.required_catalog_key_statements.json
}

output "s3tables_catalog_ready" {
  description = "ID of the s3tablescatalog check-and-create step. Grants and LF-tags on the catalog wait on it."
  value       = terraform_data.s3tables_catalog.id
}
