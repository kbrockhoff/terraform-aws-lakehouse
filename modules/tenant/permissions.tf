# ----
# Persona grants (FR-008 to FR-014)
# ----

# Every tag-based grant is scoped to one bucket catalog and includes the tenant term, so grants
# never reach another tenant or a zone the persona is not assigned.

locals {
  tenant_expression = [{ key = local.tenant_tag_key, values = ["member"] }]

  steward_database_permissions = ["CREATE_TABLE", "ALTER", "DROP", "DESCRIBE"]
  steward_table_permissions    = ["SELECT", "INSERT", "DELETE", "ALTER", "DROP", "DESCRIBE"]
  writer_table_permissions = concat(
    ["SELECT", "INSERT", "DELETE", "ALTER", "DESCRIBE"],
    local.effective.writer_drop_enabled ? ["DROP"] : [],
  )

  steward_label_permissions = {
    for pair in setproduct(var.tenant.stewards, keys(local.label_scope)) : "${pair[0]}/${pair[1]}" => {
      principal_arn = pair[0]
      key           = pair[1]
    }
  }
  writer_label_permissions = {
    for pair in setproduct(keys(var.tenant.writers), keys(local.label_scope)) : "${pair[0]}/${pair[1]}" => {
      principal_arn = var.tenant.writers[pair[0]].principal_arn
      key           = pair[1]
    }
  }
}

# Stewards: full control of the tenant's databases and tables, grantable within the tenant

resource "aws_lakeformation_permissions" "steward_database" {
  for_each = local.steward_grants

  principal                     = each.value.principal_arn
  permissions                   = local.steward_database_permissions
  permissions_with_grant_option = local.steward_database_permissions

  lf_tag_policy {
    catalog_id    = local.catalog_id[each.value.zone]
    resource_type = "DATABASE"
    dynamic "expression" {
      for_each = local.tenant_expression
      content {
        key    = expression.value.key
        values = expression.value.values
      }
    }
  }

  depends_on = [aws_lakeformation_resource_lf_tags.database, aws_lakeformation_lf_tag.tenant, terraform_data.admin_guard]
}

resource "aws_lakeformation_permissions" "steward_table" {
  for_each = local.steward_grants

  principal                     = each.value.principal_arn
  permissions                   = local.steward_table_permissions
  permissions_with_grant_option = local.steward_table_permissions

  lf_tag_policy {
    catalog_id    = local.catalog_id[each.value.zone]
    resource_type = "TABLE"
    dynamic "expression" {
      for_each = local.tenant_expression
      content {
        key    = expression.value.key
        values = expression.value.values
      }
    }
  }

  depends_on = [aws_lakeformation_resource_lf_tags.database, aws_lakeformation_lf_tag.tenant, terraform_data.admin_guard]
}

# LF-tag permissions. Nobody but platform admins can ASSOCIATE the tenant key, so nobody else can
# retag a resource into or out of a tenant.

resource "aws_lakeformation_permissions" "steward_label" {
  for_each = local.steward_label_permissions

  principal   = each.value.principal_arn
  permissions = ["DESCRIBE", "ASSOCIATE", "GRANT_WITH_LF_TAG_EXPRESSION"]

  lf_tag {
    key    = each.value.key
    values = local.label_scope[each.value.key]
  }

  depends_on = [aws_lakeformation_resource_lf_tags.database, aws_lakeformation_lf_tag.tenant, terraform_data.admin_guard]
}

resource "aws_lakeformation_permissions" "steward_tenant_tag" {
  for_each = toset(var.tenant.stewards)

  principal   = each.value
  permissions = ["DESCRIBE", "GRANT_WITH_LF_TAG_EXPRESSION"]

  lf_tag {
    key    = local.tenant_tag_key
    values = ["member"]
  }

  depends_on = [aws_lakeformation_resource_lf_tags.database, aws_lakeformation_lf_tag.tenant, terraform_data.admin_guard]
}

resource "aws_lakeformation_permissions" "writer_label" {
  for_each = local.writer_label_permissions

  principal   = each.value.principal_arn
  permissions = ["DESCRIBE", "ASSOCIATE"]

  lf_tag {
    key    = each.value.key
    values = local.label_scope[each.value.key]
  }

  depends_on = [aws_lakeformation_resource_lf_tags.database, aws_lakeformation_lf_tag.tenant, terraform_data.admin_guard]
}

# Writers: create and change tables in the tenant; DROP only in Baseline environments

resource "aws_lakeformation_permissions" "writer_database" {
  for_each = local.writer_grants

  principal   = each.value.principal_arn
  permissions = ["CREATE_TABLE", "DESCRIBE"]

  lf_tag_policy {
    catalog_id    = local.catalog_id[each.value.zone]
    resource_type = "DATABASE"
    dynamic "expression" {
      for_each = local.tenant_expression
      content {
        key    = expression.value.key
        values = expression.value.values
      }
    }
  }

  depends_on = [aws_lakeformation_resource_lf_tags.database, aws_lakeformation_lf_tag.tenant, terraform_data.admin_guard]
}

resource "aws_lakeformation_permissions" "writer_table" {
  for_each = local.writer_grants

  principal   = each.value.principal_arn
  permissions = local.writer_table_permissions

  lf_tag_policy {
    catalog_id    = local.catalog_id[each.value.zone]
    resource_type = "TABLE"
    dynamic "expression" {
      for_each = local.tenant_expression
      content {
        key    = expression.value.key
        values = expression.value.values
      }
    }
  }

  depends_on = [aws_lakeformation_resource_lf_tags.database, aws_lakeformation_lf_tag.tenant, terraform_data.admin_guard]
}

# Readers: SELECT on tables whose labels fall within the reader's sensitivity, regulations, and
# domains. LF-TBAC evaluates column tags, so columns outside the expression are excluded (FR-011).

resource "aws_lakeformation_permissions" "reader_database" {
  for_each = local.reader_grants

  principal   = each.value.principal_arn
  permissions = ["DESCRIBE"]

  lf_tag_policy {
    catalog_id    = local.catalog_id[each.value.zone]
    resource_type = "DATABASE"
    dynamic "expression" {
      for_each = local.tenant_expression
      content {
        key    = expression.value.key
        values = expression.value.values
      }
    }
  }

  depends_on = [aws_lakeformation_resource_lf_tags.database, aws_lakeformation_lf_tag.tenant, terraform_data.admin_guard]
}

resource "aws_lakeformation_permissions" "reader_table" {
  for_each = local.reader_grants

  principal   = each.value.principal_arn
  permissions = ["SELECT", "DESCRIBE"]

  lf_tag_policy {
    catalog_id    = local.catalog_id[each.value.zone]
    resource_type = "TABLE"
    dynamic "expression" {
      for_each = concat(
        local.tenant_expression,
        [{ key = var.platform.lf_tags.sensitivity.key, values = each.value.sensitivity_values }],
        [
          for r, key in var.platform.lf_tags.regulations : {
            key    = key
            values = contains(each.value.regulations, r) ? ["no", "yes"] : ["no"]
          }
        ],
        each.value.domains == null ? [] : [{ key = var.platform.lf_tags.domain.key, values = each.value.domains }],
      )
      content {
        key    = expression.value.key
        values = expression.value.values
      }
    }
  }

  depends_on = [aws_lakeformation_resource_lf_tags.database, aws_lakeformation_lf_tag.tenant, terraform_data.admin_guard]
}

# ----
# Row filters (FR-012)
# ----

# The only named-resource grants: data cells filters cannot be granted through LF-tag
# expressions, and they need the table to exist, so they are applied after writers create it.

resource "aws_lakeformation_data_cells_filter" "this" {
  for_each = local.row_filters

  table_data {
    name             = "${var.name}-${each.key}"
    database_name    = var.name
    table_catalog_id = local.catalog_id[each.value.zone]
    table_name       = each.value.table
    column_names     = each.value.column_names

    row_filter {
      filter_expression = each.value.row_filter_expression
    }

    dynamic "column_wildcard" {
      for_each = each.value.column_names == null ? [1] : []
      content {}
    }
  }

  depends_on = [aws_lakeformation_resource_lf_tags.database, aws_lakeformation_lf_tag.tenant, terraform_data.admin_guard]
}

resource "aws_lakeformation_permissions" "row_filter" {
  for_each = local.row_filters

  principal   = var.tenant.readers[each.value.reader].principal_arn
  permissions = ["SELECT"]

  data_cells_filter {
    database_name    = var.name
    name             = aws_lakeformation_data_cells_filter.this[each.key].table_data[0].name
    table_catalog_id = local.catalog_id[each.value.zone]
    table_name       = each.value.table
  }

  depends_on = [aws_lakeformation_resource_lf_tags.database, aws_lakeformation_lf_tag.tenant, terraform_data.admin_guard]
}
