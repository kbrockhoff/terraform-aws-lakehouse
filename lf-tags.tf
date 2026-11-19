# ----
# Platform-owned LF-tags
# ----

# There is deliberately no shared tenant tag: each tenant submodule owns its own
# ${prefix}-tenant-<name> key, so standalone tenants never modify a platform resource (FR-001a).

resource "aws_lakeformation_lf_tag" "sensitivity" {
  count = var.enabled ? 1 : 0

  key    = "${local.name_prefix}-sensitivity"
  values = local.sensitivity_values

  depends_on = [terraform_data.s3tables_catalog, module.account_settings, module.catalog_integration, terraform_data.settings_guard]
}

# One key per regulation, because a resource holds only one value per LF-tag key and a column can
# fall under several regulations
resource "aws_lakeformation_lf_tag" "regulation" {
  for_each = var.enabled ? toset(local.regulation_values) : toset([])

  key    = "${local.name_prefix}-reg-${each.key}"
  values = ["no", "yes"]

  depends_on = [terraform_data.s3tables_catalog, module.account_settings, module.catalog_integration, terraform_data.settings_guard]
}

# Zone of each tenant database. Lake Formation only accepts account-level LF-tag policy grants
# (catalog_id = account), not grants scoped to one S3 Tables bucket catalog, so zone isolation
# (FR-014) comes from this tag in grant expressions. Only platform admins ASSOCIATE it, so nobody
# can move a table into another zone by relabeling it.
resource "aws_lakeformation_lf_tag" "zone" {
  count = var.enabled ? 1 : 0

  key    = "${local.name_prefix}-zone"
  values = sort(tolist(local.zones))

  depends_on = [terraform_data.s3tables_catalog, module.account_settings, module.catalog_integration, terraform_data.settings_guard]
}

resource "aws_lakeformation_lf_tag" "domain" {
  count = var.enabled ? 1 : 0

  key    = "${local.name_prefix}-domain"
  values = local.domain_values

  depends_on = [terraform_data.s3tables_catalog, module.account_settings, module.catalog_integration, terraform_data.settings_guard]
}

# Lake Formation admins do not implicitly hold ASSOCIATE on LF-tags another principal created, so
# tenant pipeline roles need the platform tags' permissions granted once (README, "Required
# one-time grants"). They are not managed here: AWS provider 6.69 reads the tag creator's
# grantable permissions back onto grants to other admins, which forces replacement and an
# invalid revoke on every apply (found in the T076 sandbox).

# ----
# Platform tag visibility for approved consumer accounts (FR-024)
# ----

# Consumer administrators re-grant shared tables by tag only with DESCRIBE on every key in the share
# expression (sandbox-results.md item 5). Lake Formation keeps one grant per principal, key, and
# value, so tenants granting the same platform tag values to one account would revoke each other's
# on removal. The platform therefore owns these grants, one per approved account and platform key,
# with every value: the values are vocabulary, not data. Tenants grant only their own keys.
locals {
  platform_lf_tags = var.enabled ? merge(
    {
      sensitivity = { key = aws_lakeformation_lf_tag.sensitivity[0].key, values = local.sensitivity_values }
      zone        = { key = aws_lakeformation_lf_tag.zone[0].key, values = sort(tolist(local.zones)) }
      domain      = { key = aws_lakeformation_lf_tag.domain[0].key, values = local.domain_values }
    },
    { for r, t in aws_lakeformation_lf_tag.regulation : "reg-${r}" => { key = t.key, values = ["no", "yes"] } },
  ) : {}

  consumer_tag_grants = {
    for pair in setproduct(keys(var.sharing_config.consumer_accounts), keys(local.platform_lf_tags)) :
    "${pair[0]}/${pair[1]}" => { account_id = pair[0], tag = local.platform_lf_tags[pair[1]] }
  }
}

resource "aws_lakeformation_permissions" "consumer_tag_describe" {
  for_each = local.consumer_tag_grants

  principal                     = each.value.account_id
  permissions                   = ["DESCRIBE"]
  permissions_with_grant_option = ["DESCRIBE"]

  lf_tag {
    key    = each.value.tag.key
    values = each.value.tag.values
  }

  depends_on = [aws_lakeformation_lf_tag.sensitivity, aws_lakeformation_lf_tag.regulation, aws_lakeformation_lf_tag.zone, aws_lakeformation_lf_tag.domain, terraform_data.sharing_guard, terraform_data.settings_guard]
}
