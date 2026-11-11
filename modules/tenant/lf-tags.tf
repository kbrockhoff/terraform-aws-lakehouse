# ----
# Tenant LF-tag and database labels (FR-003, FR-009)
# ----

# Owned by this tenant, so adding or removing a tenant never modifies a platform LF-tag
resource "aws_lakeformation_lf_tag" "tenant" {
  key    = local.tenant_tag_key
  values = ["member"]

  depends_on = [terraform_data.admin_guard]
}

# Database labels are the tenant's most restrictive values, so a table nobody has labeled yet
# fails closed. Every key is set because a resource missing a key in a grant expression never
# matches. This submodule never labels tables or columns; writer pipelines do (FR-011a).
resource "aws_lakeformation_resource_lf_tags" "database" {
  for_each = local.zones

  database {
    catalog_id = local.catalog_id[each.key]
    name       = var.name
  }

  dynamic "lf_tag" {
    for_each = merge(local.database_labels, { (var.platform.lf_tags.zone.key) = each.key })
    content {
      key   = lf_tag.key
      value = lf_tag.value
    }
  }

  depends_on = [aws_s3tables_namespace.this, aws_lakeformation_lf_tag.tenant, aws_lakeformation_lf_tag.row_filter]
}

# Row-filter exclusion keys (see local.row_filter_tag_key). Owned by this tenant like its tenant
# key; no persona can associate them, so only this submodule decides which tables are filtered.
resource "aws_lakeformation_lf_tag" "row_filter" {
  for_each = local.row_filter_tag_key

  key    = each.value
  values = ["no", "yes"]

  lifecycle {
    precondition {
      condition     = length(each.value) <= 128
      error_message = "Tenant ${var.name}: LF-tag key ${each.value} exceeds 128 characters; shorten the reader key ${each.key}."
    }
  }

  depends_on = [terraform_data.admin_guard]
}

# Marks each row-filtered table for its reader, so the reader's tag grant skips it and the data
# cells filter is the reader's only access. Like row filters, it needs the table to exist.
resource "aws_lakeformation_resource_lf_tags" "row_filtered_table" {
  for_each = local.row_filtered_tables

  table {
    catalog_id    = local.catalog_id[each.value.zone]
    database_name = var.name
    name          = each.value.table
  }

  lf_tag {
    key   = aws_lakeformation_lf_tag.row_filter[each.value.reader].key
    value = "yes"
  }

  depends_on = [aws_lakeformation_resource_lf_tags.database, terraform_data.admin_guard]
}
