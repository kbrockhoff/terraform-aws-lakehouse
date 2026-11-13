# ----
# Misconfiguration overlays for lfverify detect (Detection SLO)
# ----

# Each flag is read where the affected grant is defined:
# - extra_grant: grants-named.tf (n_gamma) and the label grant below (gamma tenant)
# - missing_grant: grants-named.tf filters out the persona's grants; platform.tf drops admin or
#   roadmin from the settings lists
# - drop_column_restriction: platform.tf (reader_cols sensitivity) and grants-named.tf
# - drop_row_filter: platform.tf (row_filters) and grants-named.tf
# - open_default: the IAM_ALLOWED_PRINCIPALS grant below
locals {
  inject_extra_grant             = var.inject == "extra_grant"
  inject_missing_grant           = var.inject == "missing_grant"
  inject_drop_column_restriction = var.inject == "drop_column_restriction"
  inject_drop_row_filter         = var.inject == "drop_row_filter"
  inject_open_default            = var.inject == "open_default"
}

resource "aws_lakeformation_permissions" "inject_extra_label" {
  count = local.inject_extra_grant ? 1 : 0

  principal   = aws_iam_role.persona[var.inject_persona].arn
  permissions = local.reader_perms

  lf_tag_policy {
    catalog_id    = local.account_id
    resource_type = "TABLE"
    dynamic "expression" {
      for_each = local.tenant_curated_expression["gamma"]
      content {
        key    = expression.value.key
        values = expression.value.values
      }
    }
  }

  # The runner becomes a data-lake admin inside the module; nothing here may run before that
  depends_on = [module.lakehouse]
}

# Restores the open default that the catalog integration removes (FR-008 must fail the run)
resource "aws_lakeformation_permissions" "inject_open_default" {
  count = local.inject_open_default ? 1 : 0

  principal   = "IAM_ALLOWED_PRINCIPALS"
  permissions = ["ALL"]

  database {
    catalog_id = local.named_catalog_id
    name       = "n_alpha"
  }

  depends_on = [module.lakehouse, aws_s3tables_namespace.named]
}
