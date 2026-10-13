output "policy_arn" {
  description = "ARN of the IAM policy created for deployment operations"
  value       = var.enabled ? aws_iam_policy.basic[0].arn : null
}

output "policy_name" {
  description = "Name of the IAM policy created for deployment operations"
  value       = var.enabled ? aws_iam_policy.basic[0].name : null
}