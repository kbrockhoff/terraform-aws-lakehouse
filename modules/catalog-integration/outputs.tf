output "data_access_role_arn" {
  description = "ARN of the Lake Formation data-access role. Table bucket KMS key policies must grant it use of the key."
  value       = aws_iam_role.lf_s3tables_access.arn
}

output "catalog_name" {
  description = "Name of the federated S3 Tables Glue catalog."
  value       = aws_glue_catalog.s3tables.name
}

output "catalog_arn" {
  description = "ARN of the federated S3 Tables Glue catalog."
  value       = aws_glue_catalog.s3tables.arn
}
