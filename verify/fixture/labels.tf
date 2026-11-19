# ----
# Label-style table and column labels (stage 2, after lfverify seed creates the tables)
# ----

# Every label-style namespace gets the same labels: orders columns are internal except the
# restricted ssn column, and the table rolls up to restricted (001 FR-011a). The customers and
# scratch tables are labeled internal, as a writer pipeline would label them.

locals {
  label_namespaces = merge([
    for t in local.tenant_names : {
      for z, ns in module.lakehouse.tenants[t].namespaces : "${t}/${z}" => {
        catalog_id = module.lakehouse.tenants[t].catalog_ids[z]
        database   = ns
      }
    }
  ]...)

  stage2_label_namespaces = var.stage == 2 ? local.label_namespaces : {}

  sensitivity_tag_key = module.lakehouse.lf_tag_vocabulary.sensitivity.key
  orders_columns      = [for c in local.tables.orders.columns : c.name]
  orders_internal     = [for c in local.orders_columns : c if !contains(keys(local.column_sensitivity.orders), c)]
}

resource "aws_lakeformation_resource_lf_tag" "orders_table" {
  for_each = local.stage2_label_namespaces

  table {
    catalog_id    = each.value.catalog_id
    database_name = each.value.database
    name          = "orders"
  }

  lf_tag {
    key   = local.sensitivity_tag_key
    value = "restricted"
  }

  # The runner becomes a data-lake admin inside the module; nothing here may run before that
  depends_on = [module.lakehouse]
}

resource "aws_lakeformation_resource_lf_tag" "orders_internal_columns" {
  for_each = local.stage2_label_namespaces

  table_with_columns {
    catalog_id    = each.value.catalog_id
    database_name = each.value.database
    name          = "orders"
    column_names  = local.orders_internal
  }

  lf_tag {
    key   = local.sensitivity_tag_key
    value = "internal"
  }

  depends_on = [module.lakehouse, aws_lakeformation_resource_lf_tag.orders_table]
}

resource "aws_lakeformation_resource_lf_tag" "orders_restricted_columns" {
  for_each = local.stage2_label_namespaces

  table_with_columns {
    catalog_id    = each.value.catalog_id
    database_name = each.value.database
    name          = "orders"
    column_names  = [for c, s in local.column_sensitivity.orders : c if s == "restricted"]
  }

  lf_tag {
    key   = local.sensitivity_tag_key
    value = "restricted"
  }

  depends_on = [module.lakehouse, aws_lakeformation_resource_lf_tag.orders_table]
}

resource "aws_lakeformation_resource_lf_tag" "scratch_mw" {
  count = var.stage == 2 ? 1 : 0

  table {
    catalog_id    = local.label_namespaces["alpha/curated"].catalog_id
    database_name = local.label_namespaces["alpha/curated"].database
    name          = "scratch"
  }

  lf_tag {
    key   = module.lakehouse.tenants["alpha"].metadata_writer_tag_key
    value = "yes"
  }

  # The runner becomes a data-lake admin inside the module; nothing here may run before that
  depends_on = [module.lakehouse]
}

# Writer pipelines label the tables they create (001 FR-011a); without this, customers and scratch
# would inherit the tenant ceiling (restricted) and hide from column-limited readers
resource "aws_lakeformation_resource_lf_tag" "internal_tables" {
  for_each = {
    for pair in setproduct(keys(local.stage2_label_namespaces), ["customers", "scratch"]) : "${pair[0]}/${pair[1]}" => {
      namespace = local.stage2_label_namespaces[pair[0]]
      table     = pair[1]
    }
  }

  table {
    catalog_id    = each.value.namespace.catalog_id
    database_name = each.value.namespace.database
    name          = each.value.table
  }

  lf_tag {
    key   = local.sensitivity_tag_key
    value = "internal"
  }

  # The runner becomes a data-lake admin inside the module; nothing here may run before that
  depends_on = [module.lakehouse]
}

# ----
# Migration from aws_lakeformation_resource_lf_tags (awslkhs-8oof)
# ----

# The plural resource reads every tag on the table, including the module's row-filter exclusion
# keys, so it was replaced on every apply, and its delete removed all of them. Forget it without
# calling delete; the singular resources above adopt the existing tags.

removed {
  from = aws_lakeformation_resource_lf_tags.orders_table

  lifecycle {
    destroy = false
  }
}

removed {
  from = aws_lakeformation_resource_lf_tags.orders_internal_columns

  lifecycle {
    destroy = false
  }
}

removed {
  from = aws_lakeformation_resource_lf_tags.orders_restricted_columns

  lifecycle {
    destroy = false
  }
}

removed {
  from = aws_lakeformation_resource_lf_tags.scratch_mw

  lifecycle {
    destroy = false
  }
}

removed {
  from = aws_lakeformation_resource_lf_tags.internal_tables

  lifecycle {
    destroy = false
  }
}
