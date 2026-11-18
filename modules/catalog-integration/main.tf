locals {
  table_buckets_arn = "arn:${var.partition}:s3tables:${var.region}:${var.account_id}:bucket/*"
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

  # Registration and the catalog need the deploying role to be a data-lake admin first
  depends_on = [aws_iam_role_policy.lf_s3tables_access, terraform_data.settings_ready]
}

# Federated catalog exposing S3 table buckets to Glue, Athena, and Lake Formation
resource "aws_glue_catalog" "s3tables" {
  name = "s3tablescatalog"

  federated_catalog {
    identifier      = local.table_buckets_arn
    connection_name = "aws:s3tables"
  }

  allow_full_table_external_data_access = var.allow_full_table_external_data_access ? "True" : "False"

  tags = var.tags

  depends_on = [aws_lakeformation_resource.s3tables, terraform_data.settings_ready]
}
