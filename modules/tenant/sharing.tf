# ----
# Cross-account shares (FR-021 to FR-028)
# ----

# Read-only LF-tag expression grants to approved consumer accounts in the same Region. Lake
# Formation creates and reuses the RAM shares itself on cross-account version 4, so the module
# never manages aws_ram_* resources. Every expression includes the tenant term, so a share never
# reaches another tenant's data.

locals {
  share_grants = {
    for k, s in var.tenant.shares : k => {
      account_id         = s.account_id
      zones              = sort(s.zones)
      sensitivity_values = s.sensitivity_values
      regulations        = s.regulations
      domains            = s.domains
    }
  }

  # Same terms as reader expressions. Row filters are not shared (FR-024), so a table filtered for
  # any reader is excluded: every row-filter key must be "no", or the consumer would see all rows.
  share_expressions = {
    for k, s in local.share_grants : k => concat(
      [local.tenant_term, { key = local.zone_key, values = s.zones }],
      [{ key = var.platform.lf_tags.sensitivity.key, values = s.sensitivity_values }],
      [
        for reg, key in var.platform.lf_tags.regulations : {
          key    = key
          values = contains(s.regulations, reg) ? ["no", "yes"] : ["no"]
        }
      ],
      s.domains == null ? [] : [{ key = var.platform.lf_tags.domain.key, values = s.domains }],
      [for key in values(local.row_filter_tag_key) : { key = key, values = ["no"] }],
    )
  }

  # Consumer administrators re-grant by tag only with DESCRIBE on the tag keys and values in the
  # expression (sandbox-results.md item 5). The platform grants its own keys (zone, sensitivity,
  # regulations, domain) to every approved account, because several tenants would share those
  # grants. The tenant grants only the keys it owns: its tenant key and its row-filter exclusions.
  # One share per account (validated), so these grants are never shared between shares.
  tenant_owned_keys = concat([local.tenant_term.key], values(local.row_filter_tag_key))
  share_tag_values = merge([
    for k, s in local.share_grants : {
      for term in local.share_expressions[k] : "${s.account_id}/${term.key}" => {
        account_id = s.account_id
        key        = term.key
        values     = term.values
      } if contains(local.tenant_owned_keys, term.key)
    }
  ]...)
}

resource "aws_lakeformation_permissions" "share_database" {
  for_each = local.share_grants

  principal                     = each.value.account_id
  permissions                   = ["DESCRIBE"]
  permissions_with_grant_option = ["DESCRIBE"]

  lf_tag_policy {
    catalog_id    = local.account_id
    resource_type = "DATABASE"
    dynamic "expression" {
      for_each = [local.tenant_term, { key = local.zone_key, values = each.value.zones }]
      content {
        key    = expression.value.key
        values = expression.value.values
      }
    }
  }

  # Grants are revoked before tags and namespaces are destroyed, or RAM shares are orphaned
  depends_on = [aws_lakeformation_resource_lf_tags.database, aws_lakeformation_lf_tag.tenant, terraform_data.share_guard, terraform_data.admin_guard]
}

# Read-only, with the grant option limited to the same permissions, so the consumer administrator
# can delegate within that account but cannot widen access (FR-024)
resource "aws_lakeformation_permissions" "share_table" {
  for_each = local.share_grants

  principal                     = each.value.account_id
  permissions                   = ["SELECT", "DESCRIBE"]
  permissions_with_grant_option = ["SELECT", "DESCRIBE"]

  lf_tag_policy {
    catalog_id    = local.account_id
    resource_type = "TABLE"
    dynamic "expression" {
      for_each = local.share_expressions[each.key]
      content {
        key    = expression.value.key
        values = expression.value.values
      }
    }
  }

  # The row-filtered table associations set their exclusion keys to "yes"; until they exist those
  # tables inherit "no" from the database and would match. Ordering also revokes before they go.
  depends_on = [aws_lakeformation_resource_lf_tags.database, aws_lakeformation_resource_lf_tags.row_filtered_table, aws_lakeformation_lf_tag.tenant, aws_lakeformation_lf_tag.row_filter, terraform_data.share_guard, terraform_data.admin_guard]
}

# DESCRIBE only: GRANT_WITH_LF_TAG_EXPRESSION cannot be granted across accounts
# ("MANAGE_TAG_POLICY is not supported cross account")
resource "aws_lakeformation_permissions" "share_tag_describe" {
  for_each = local.share_tag_values

  principal                     = each.value.account_id
  permissions                   = ["DESCRIBE"]
  permissions_with_grant_option = ["DESCRIBE"]

  lf_tag {
    key    = each.value.key
    values = each.value.values
  }

  depends_on = [aws_lakeformation_lf_tag.tenant, aws_lakeformation_lf_tag.row_filter, terraform_data.share_guard, terraform_data.admin_guard]
}
