output "policy_arn" {
  description = "ARN of the IAM policy created for deployment operations"
  value       = var.enabled ? aws_iam_policy.basic[0].arn : null
}

output "policy_name" {
  description = "Name of the IAM policy created for deployment operations"
  value       = var.enabled ? aws_iam_policy.basic[0].name : null
}
output "policy_arns" {
  description = "ARNs of both managed policies attached to the role"
  value       = var.enabled ? [aws_iam_policy.basic[0].arn, aws_iam_policy.lakehouse[0].arn] : []
}

output "policy_json" {
  description = "Both rendered policy documents merged, for review (the role gets them as two managed policies)"
  value       = var.enabled ? data.aws_iam_policy_document.combined[0].json : null
}
