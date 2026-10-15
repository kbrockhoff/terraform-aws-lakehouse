data "aws_caller_identity" "current" {}

# Resolves the role behind the deploying session (an assumed-role ARN becomes the role ARN)
data "aws_iam_session_context" "current" {
  arn = data.aws_caller_identity.current.arn
}

# Standalone pipelines verify their role is a data-lake admin before planning grants
data "aws_lakeformation_data_lake_settings" "current" {
  count = var.verify_deployer_admin ? 1 : 0
}
