locals {
  table_buckets_arn = "arn:${var.partition}:s3tables:${var.region}:${var.account_id}:bucket/*"
}

# Carries the root's s3tablescatalog step ID (empty when another deployment owns the account
# settings), so the catalog exists before anything here is created or changed (spec 003 FR-029)
resource "terraform_data" "catalog_ready" {
  input = var.catalog_ready
}

# The data-lake settings live in modules/account-settings, which another deployment may own. This
# carries their ID (or the settings guard's) so registration waits for the deployer's admin rights.
resource "terraform_data" "settings_ready" {
  input = var.settings_ready
}

# ----
# S3 Tables integration with Lake Formation
# ----

# Registers every table bucket in the account and Region with Lake Formation
resource "aws_lakeformation_resource" "s3tables" {
  arn                   = local.table_buckets_arn
  role_arn              = aws_iam_role.lf_s3tables_access.arn
  with_federation       = true
  hybrid_access_enabled = false

  # Registration needs the deploying role to be a data-lake admin first
  depends_on = [aws_iam_role_policy.lf_s3tables_access, terraform_data.settings_ready]
}

# The federated s3tablescatalog moved to modules/account-settings, which creates it only when it
# is missing and never deletes it (spec 003 FR-029 to FR-031). Forget the catalog without deleting
# it: a delete would drop every catalog grant and bucket catalog for all deployments in the account.
removed {
  from = aws_glue_catalog.s3tables

  lifecycle {
    destroy = false
  }
}
