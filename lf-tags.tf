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

# Zone of each tenant database. Lake Formation only accepts account-level LF-tag policy grants
# (catalog_id = account), not grants scoped to one S3 Tables bucket catalog, so zone isolation
# (FR-014) comes from this tag in grant expressions. Only platform admins ASSOCIATE it, so nobody
# can move a table into another zone by relabeling it.
resource "aws_lakeformation_lf_tag" "zone" {
  count = var.enabled ? 1 : 0

  key    = "${local.name_prefix}-zone"
  values = sort(tolist(local.zones))

  depends_on = [module.catalog_integration, terraform_data.secondary_admin_guard]
}

resource "aws_lakeformation_lf_tag" "domain" {
  count = var.enabled ? 1 : 0

  key    = "${local.name_prefix}-domain"
  values = local.domain_values

  depends_on = [module.catalog_integration, terraform_data.secondary_admin_guard]
}
