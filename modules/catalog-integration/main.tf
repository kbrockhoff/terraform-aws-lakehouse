locals {
  table_buckets_arn = "arn:${var.partition}:s3tables:${var.region}:${var.account_id}:bucket/*"
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
  depends_on = [aws_iam_role_policy.lf_s3tables_access, aws_lakeformation_data_lake_settings.this]
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

  depends_on = [aws_lakeformation_resource.s3tables, aws_lakeformation_data_lake_settings.this]
}

# Authoritative for ALL data-lake settings in the account and Region: admins not listed here are
# removed. Tenant pipeline roles and secondary deployers must be in admin_principal_arns.
resource "aws_lakeformation_data_lake_settings" "this" {
  admins = var.admin_principal_arns

  # No create_*_default_permissions blocks: omitting them sends empty lists, which removes the
  # IAM_ALLOWED_PRINCIPALS defaults, so Lake Formation grants are the only access to new
  # databases and tables. (An empty block would send an empty principal, which the API rejects.)

  lifecycle {
    precondition {
      condition     = contains(var.admin_principal_arns, var.deployer_role_arn)
      error_message = "The deploying role (${var.deployer_role_arn}) must be in admin_principal_arns, or this apply would remove its own data-lake admin rights and fail every later grant."
    }
  }
}
