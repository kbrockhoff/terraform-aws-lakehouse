# ----
# Lake Formation Identity Center integration (FR-032)
# ----

# Connects Lake Formation to an existing IAM Identity Center instance, so data permissions can be
# granted to Identity Center users and groups and trusted identity propagation reaches Lake
# Formation. One per catalog: an integration created outside the module must be imported first.
# Removing it stops every grant to Identity Center users and groups from working.
resource "aws_lakeformation_identity_center_configuration" "this" {
  count = var.identity_center.enabled ? 1 : 0

  catalog_id   = var.account_id
  instance_arn = var.identity_center.instance_arn

  # The provider resource takes no tags. The deployer must already be a data-lake admin.
  depends_on = [aws_lakeformation_data_lake_settings.this]
}
