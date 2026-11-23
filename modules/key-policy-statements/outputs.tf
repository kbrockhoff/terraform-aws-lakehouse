output "administration_json" {
  description = "KeyAdministrators, AccountReadOnly, and, with break-glass principals, KeyDeletionBreakGlass."
  value       = data.aws_iam_policy_document.administration.json
}

output "direct_access_json" {
  description = "AllowDirectAccessExemptions, or empty when no principal is exempt. Pass through compact() into source_policy_documents."
  value       = length(data.aws_iam_policy_document.direct_access) > 0 ? data.aws_iam_policy_document.direct_access[0].json : ""
}
