# AWS account, partition, and region data sources
data "aws_caller_identity" "current" {}
data "aws_partition" "current" {}
data "aws_region" "current" {}

# Resolves the role behind the deploying session (an assumed-role ARN becomes the role ARN)
data "aws_iam_session_context" "current" {
  arn = data.aws_caller_identity.current.arn
}

# Secondary deployments (catalog integration owned elsewhere) read the data-lake admins to
# verify the deploying role is one of them
data "aws_lakeformation_data_lake_settings" "current" {
  count = var.enabled && !var.catalog_integration_config.enabled ? 1 : 0
}
