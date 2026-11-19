output "data_access_role_arn" {
  description = "ARN of the Lake Formation data-access role. Table bucket KMS key policies must grant it use of the key."
  value       = aws_iam_role.lf_s3tables_access.arn
}
