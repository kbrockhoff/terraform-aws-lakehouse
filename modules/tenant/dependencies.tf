data "aws_caller_identity" "current" {}

# Resolves the role behind the deploying session (an assumed-role ARN becomes the role ARN)
data "aws_iam_session_context" "current" {
  arn = data.aws_caller_identity.current.arn
}

# Standalone pipelines verify their role is a data-lake admin before planning grants, and read the
# live cross-account version for shares (FR-023). Platform-applied tenants (verify_deployer_admin =
# false) skip both: the root guards them, and an owner raises the version in the same apply, so a
# plan-time read would reject the apply that fixes it.
data "aws_lakeformation_data_lake_settings" "current" {
  count = var.verify_deployer_admin ? 1 : 0
}
