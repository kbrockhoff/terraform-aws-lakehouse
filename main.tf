# ----
# Catalog integration (account and Region singleton)
# ----

# The dependency is one-way: the submodule's data-access role is referenced by key policies, so
# the submodule takes no key ARNs.
module "catalog_integration" {
  source = "./modules/catalog-integration"
  count  = var.enabled && var.catalog_integration_config.enabled ? 1 : 0

  name_prefix                           = local.name_prefix
  account_id                            = local.account_id
  partition                             = local.partition
  region                                = local.region
  tags                                  = local.common_tags
  admin_principal_arns                  = var.governance_config.admin_principal_arns
  deployer_role_arn                     = local.deployer_role_arn
  allow_full_table_external_data_access = var.catalog_integration_config.allow_full_table_external_data_access
}

# ----
# Guards
# ----

# Secondary deployments do not own the data-lake settings, so they check the live admin list
resource "terraform_data" "secondary_admin_guard" {
  count = var.enabled && !var.catalog_integration_config.enabled ? 1 : 0

  lifecycle {
    precondition {
      condition     = contains(data.aws_lakeformation_data_lake_settings.current[0].admins, local.deployer_role_arn)
      error_message = "The deploying role (${local.deployer_role_arn}) is not a Lake Formation data-lake admin. Add it to admin_principal_arns in the deployment that owns the catalog integration, apply that deployment, then retry."
    }
  }
}

# Every storage resource depends on this guard, so missing required tags fail before anything
# is created
resource "terraform_data" "required_tags_guard" {
  count = var.enabled && local.effective_config.required_tags_enforced ? 1 : 0

  lifecycle {
    precondition {
      condition     = try(var.tags[var.required_tags_config.cost_center_key], "") != ""
      error_message = "tags must include a non-empty \"${var.required_tags_config.cost_center_key}\" tag (required_tags_config.cost_center_key)."
    }
    precondition {
      condition     = try(var.tags[var.required_tags_config.data_owner_key], "") != ""
      error_message = "tags must include a non-empty \"${var.required_tags_config.data_owner_key}\" tag (required_tags_config.data_owner_key)."
    }
  }
}

# ----
# Platform-applied tenants
# ----

# The contract carries values only, so explicit dependencies on the platform's governance
# resources are needed for grants to wait for the LF-tags and admin settings.
module "tenant" {
  source   = "./modules/tenant"
  for_each = var.enabled ? var.tenants : {}

  providers = {
    aws         = aws
    aws.pricing = aws.pricing
  }

  platform              = local.platform_contract
  name                  = each.key
  tenant                = each.value
  verify_deployer_admin = false # the root already guards (FR-013)

  depends_on = [
    module.catalog_integration,
    terraform_data.secondary_admin_guard,
    aws_lakeformation_lf_tag.sensitivity,
    aws_lakeformation_lf_tag.regulation,
    aws_lakeformation_lf_tag.domain,
  ]
}
