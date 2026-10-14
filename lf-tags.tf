# ----
# Platform-owned LF-tags
# ----

# There is deliberately no shared tenant tag: each tenant submodule owns its own
# ${prefix}-tenant-<name> key, so standalone tenants never modify a platform resource (FR-001a).

resource "aws_lakeformation_lf_tag" "sensitivity" {
  count = var.enabled ? 1 : 0

  key    = "${local.name_prefix}-sensitivity"
  values = local.sensitivity_values

  depends_on = [module.catalog_integration, terraform_data.secondary_admin_guard]
}

# One key per regulation, because a resource holds only one value per LF-tag key and a column can
# fall under several regulations
resource "aws_lakeformation_lf_tag" "regulation" {
  for_each = var.enabled ? toset(local.regulation_values) : toset([])

  key    = "${local.name_prefix}-reg-${each.key}"
  values = ["no", "yes"]

  depends_on = [module.catalog_integration, terraform_data.secondary_admin_guard]
}

resource "aws_lakeformation_lf_tag" "domain" {
  count = var.enabled ? 1 : 0

  key    = "${local.name_prefix}-domain"
  values = local.domain_values

  depends_on = [module.catalog_integration, terraform_data.secondary_admin_guard]
}
