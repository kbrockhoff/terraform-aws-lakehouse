# ----
# Named-resource grants (FR-007): the same personas as the label style, with no LF-tags
# ----

# Database grants and all-tables (wildcard) grants exist from stage 1; grants on a named table
# and data cells filters need the seeded tables, so they exist only at stage 2.

locals {
  reader_perms = ["SELECT", "DESCRIBE"]
  writer_perms = ["SELECT", "INSERT", "DELETE", "ALTER", "DESCRIBE"]

  # One entry per grant. kind is database, tables (wildcard), table, or columns.
  named_grant_list = concat(
    flatten([for db in ["n_alpha", "n_beta"] : [
      { persona = "meta_reader", kind = "database", database = db, table = null, permissions = ["DESCRIBE"] },
      { persona = "meta_reader", kind = "tables", database = db, table = null, permissions = ["DESCRIBE"] },
      { persona = "reader_multi", kind = "database", database = db, table = null, permissions = ["DESCRIBE"] },
      { persona = "reader_multi", kind = "tables", database = db, table = null, permissions = local.reader_perms },
      { persona = "writer_multi", kind = "database", database = db, table = null, permissions = ["DESCRIBE"] },
      { persona = "writer_multi", kind = "tables", database = db, table = null, permissions = local.writer_perms },
    ]]),
    [
      { persona = "meta_writer", kind = "database", database = "n_alpha", table = null, permissions = ["CREATE_TABLE"] },
      { persona = "meta_writer", kind = "table", database = "n_alpha", table = "scratch", permissions = ["ALTER", "DROP"] },
      { persona = "reader_single", kind = "database", database = "n_alpha", table = null, permissions = ["DESCRIBE"] },
      { persona = "reader_single", kind = "tables", database = "n_alpha", table = null, permissions = local.reader_perms },
      { persona = "writer_single", kind = "database", database = "n_alpha", table = null, permissions = ["DESCRIBE"] },
      { persona = "writer_single", kind = "tables", database = "n_alpha", table = null, permissions = local.writer_perms },
      { persona = "reader_cols", kind = "database", database = "n_alpha", table = null, permissions = ["DESCRIBE"] },
      # drop_column_restriction widens the column grant to every column
      { persona = "reader_cols", kind = local.inject_drop_column_restriction ? "table" : "columns", database = "n_alpha", table = "orders", permissions = ["SELECT"] },
      { persona = "reader_cols", kind = "table", database = "n_alpha", table = "customers", permissions = local.reader_perms },
      { persona = "reader_rows", kind = "database", database = "n_alpha", table = null, permissions = ["DESCRIBE"] },
      { persona = "reader_cells", kind = "database", database = "n_alpha", table = null, permissions = ["DESCRIBE"] },
      { persona = "steward", kind = "database", database = "n_alpha", table = null, permissions = ["ALL"] },
      { persona = "steward", kind = "tables", database = "n_alpha", table = null, permissions = ["ALL"] },
    ],
    # drop_row_filter replaces the row and cell readers' filters with whole-table SELECT
    local.inject_drop_row_filter ? [
      { persona = "reader_rows", kind = "table", database = "n_alpha", table = "orders", permissions = ["SELECT"] },
      { persona = "reader_cells", kind = "table", database = "n_alpha", table = "orders", permissions = ["SELECT"] },
    ] : [],
    # extra_grant gives the persona read access to the control namespace
    local.inject_extra_grant ? [
      { persona = var.inject_persona, kind = "tables", database = "n_gamma", table = null, permissions = local.reader_perms },
    ] : [],
  )

  named_grants = {
    for g in local.named_grant_list :
    "${g.persona}/${g.kind}/${g.database}${g.table == null ? "" : "/${g.table}"}" => g
    if(var.stage == 2 || contains(["database", "tables"], g.kind)) &&
    !(local.inject_missing_grant && g.persona == var.inject_persona)
  }

  named_filters = var.stage == 2 && !local.inject_drop_row_filter ? {
    n_rows_eu  = { persona = "reader_rows", expression = "region = 'EU'", exclude_columns = [] }
    n_rows_us  = { persona = "reader_rows", expression = "region = 'US'", exclude_columns = [] }
    n_cells_eu = { persona = "reader_cells", expression = "region = 'EU'", exclude_columns = [local.tables.orders.restricted_column] }
  } : {}

  named_filter_grants = {
    for k, f in local.named_filters : k => f
    if !(local.inject_missing_grant && f.persona == var.inject_persona)
  }
}

resource "aws_lakeformation_permissions" "named_database" {
  for_each = { for k, g in local.named_grants : k => g if g.kind == "database" }

  principal                     = aws_iam_role.persona[each.value.persona].arn
  permissions                   = each.value.permissions
  permissions_with_grant_option = each.value.persona == "steward" ? each.value.permissions : null

  database {
    catalog_id = local.named_catalog_id
    name       = each.value.database
  }

  depends_on = [module.lakehouse, aws_s3tables_namespace.named]
}

resource "aws_lakeformation_permissions" "named_tables" {
  for_each = { for k, g in local.named_grants : k => g if g.kind == "tables" }

  principal                     = aws_iam_role.persona[each.value.persona].arn
  permissions                   = each.value.permissions
  permissions_with_grant_option = each.value.persona == "steward" ? each.value.permissions : null

  table {
    catalog_id    = local.named_catalog_id
    database_name = each.value.database
    wildcard      = true
  }

  depends_on = [module.lakehouse, aws_s3tables_namespace.named]
}

resource "aws_lakeformation_permissions" "named_table" {
  for_each = { for k, g in local.named_grants : k => g if g.kind == "table" }

  principal   = aws_iam_role.persona[each.value.persona].arn
  permissions = each.value.permissions

  table {
    catalog_id    = local.named_catalog_id
    database_name = each.value.database
    name          = each.value.table
  }

  # The runner becomes a data-lake admin inside the module; nothing here may run before that
  depends_on = [module.lakehouse]
}

resource "aws_lakeformation_permissions" "named_columns" {
  for_each = { for k, g in local.named_grants : k => g if g.kind == "columns" }

  principal   = aws_iam_role.persona[each.value.persona].arn
  permissions = each.value.permissions

  table_with_columns {
    catalog_id            = local.named_catalog_id
    database_name         = each.value.database
    name                  = each.value.table
    wildcard              = true
    excluded_column_names = [local.tables.orders.restricted_column]
  }

  # The runner becomes a data-lake admin inside the module; nothing here may run before that
  depends_on = [module.lakehouse]
}

resource "aws_lakeformation_data_cells_filter" "named" {
  for_each = local.named_filters

  table_data {
    name             = "${local.prefix}-${each.key}"
    database_name    = "n_alpha"
    table_catalog_id = local.named_catalog_id
    table_name       = "orders"

    row_filter {
      filter_expression = each.value.expression
    }

    column_wildcard {
      excluded_column_names = each.value.exclude_columns
    }
  }

  # The runner becomes a data-lake admin inside the module; nothing here may run before that
  depends_on = [module.lakehouse]
}

resource "aws_lakeformation_permissions" "named_filter" {
  for_each = local.named_filter_grants

  principal   = aws_iam_role.persona[each.value.persona].arn
  permissions = ["SELECT"]

  data_cells_filter {
    database_name    = "n_alpha"
    name             = aws_lakeformation_data_cells_filter.named[each.key].table_data[0].name
    table_catalog_id = local.named_catalog_id
    table_name       = "orders"
  }

  # The runner becomes a data-lake admin inside the module; nothing here may run before that
  depends_on = [module.lakehouse]
}
