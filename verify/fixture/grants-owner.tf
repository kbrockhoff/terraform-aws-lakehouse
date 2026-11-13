# ----
# Fixture owner grants
# ----

# The runner seeds every fixture table (FR-006). Data-lake admins get no implicit permissions on
# databases they did not create, and the namespaces are created through the S3 Tables API, so
# the owner needs explicit grants. The owner is never a persona under test.

locals {
  owner_databases = merge(
    { for k, ns in local.label_namespaces : "label/${k}" => ns },
    { for ns in local.named_namespaces : "named/${ns}" => { catalog_id = local.named_catalog_id, database = ns } },
  )
}

resource "aws_lakeformation_permissions" "owner_database" {
  for_each = local.owner_databases

  principal                     = local.runner_role_arn
  permissions                   = ["ALL"]
  permissions_with_grant_option = ["ALL"]

  database {
    catalog_id = each.value.catalog_id
    name       = each.value.database
  }

  depends_on = [module.lakehouse, aws_s3tables_namespace.named]
}

resource "aws_lakeformation_permissions" "owner_tables" {
  for_each = local.owner_databases

  principal                     = local.runner_role_arn
  permissions                   = ["ALL"]
  permissions_with_grant_option = ["ALL"]

  table {
    catalog_id    = each.value.catalog_id
    database_name = each.value.database
    wildcard      = true
  }

  depends_on = [module.lakehouse, aws_s3tables_namespace.named]
}
