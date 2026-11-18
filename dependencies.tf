# AWS account, partition, and region data sources
data "aws_caller_identity" "current" {}
data "aws_partition" "current" {}
data "aws_region" "current" {}

# Resolves the role behind the deploying session (an assumed-role ARN becomes the role ARN)
data "aws_iam_session_context" "current" {
  arn = data.aws_caller_identity.current.arn
}

# Deployments that do not own the account settings read the live values they depend on
data "aws_lakeformation_data_lake_settings" "current" {
  count = var.enabled && !local.settings_owner ? 1 : 0
}

data "aws_glue_data_catalog_encryption_settings" "current" {
  count = var.enabled && !local.settings_owner ? 1 : 0

  catalog_id = local.account_id
}
