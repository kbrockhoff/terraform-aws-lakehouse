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

  depends_on = [aws_s3tables_namespace.this, aws_lakeformation_lf_tag.tenant]
}
