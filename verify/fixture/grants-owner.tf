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

  # Lake Formation reports every grant the runner holds here, including the implicit creator grants
  # (ALTER, CREATE_TABLE, DESCRIBE, DROP with grant option), and the provider merges them into these
  # lists, so they never match ["ALL"] and the grant was replaced on every apply (awslkhs-c921). A
  # grant revoked entirely still reads back empty, leaves state, and is recreated.
  lifecycle {
    ignore_changes = [permissions, permissions_with_grant_option]
  }
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

  # As owner_database: the creator grants on seeded tables (ALTER, DELETE, DESCRIBE, DROP, INSERT,
  # SELECT with grant option) are merged into the read-back lists (awslkhs-c921)
  lifecycle {
    ignore_changes = [permissions, permissions_with_grant_option]
  }
}
