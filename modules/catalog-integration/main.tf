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

  depends_on = [aws_iam_role_policy.lf_s3tables_access]
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

  depends_on = [aws_lakeformation_resource.s3tables]
}

# Authoritative for ALL data-lake settings in the account and Region: admins not listed here are
# removed. Tenant pipeline roles and secondary deployers must be in admin_principal_arns.
resource "aws_lakeformation_data_lake_settings" "this" {
  admins = var.admin_principal_arns

  # Empty defaults: no IAM_ALLOWED_PRINCIPALS access to new databases or tables
  create_database_default_permissions {
    permissions = []
    principal   = null
  }
  create_table_default_permissions {
    permissions = []
    principal   = null
  }

  lifecycle {
    precondition {
      condition     = contains(var.admin_principal_arns, var.deployer_role_arn)
      error_message = "The deploying role (${var.deployer_role_arn}) must be in admin_principal_arns, or this apply would remove its own data-lake admin rights and fail every later grant."
    }
  }
}
