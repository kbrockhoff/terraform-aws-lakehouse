# ----
# Account settings (account and Region singleton, FR-001)
# ----

# Lake Formation data-lake settings, Glue catalog encryption with a dedicated key, and the optional
# catalog resource policy. Exactly one deployment per account and Region owns them.
module "account_settings" {
  source = "./modules/account-settings"
  count  = var.enabled && local.settings_owner ? 1 : 0

  name_prefix      = local.name_prefix
  account_id       = local.account_id
  partition        = local.partition
  region           = local.region
  tags             = local.common_tags
  environment_type = var.environment_type

  admin_principal_arns = concat(
    var.governance_config.admin_principal_arns,
    var.studio_config.enabled ? var.studio_config.integration_principal_arns : [],
  )
  read_only_admin_principal_arns = var.governance_config.read_only_admin_principal_arns
  deployer_role_arn              = local.deployer_role_arn

  cross_account_version        = var.account_settings_config.cross_account_version
  data_lake                    = var.account_settings_config.data_lake
  catalog_encryption           = var.account_settings_config.catalog_encryption
  catalog_resource_policy      = var.account_settings_config.catalog_resource_policy
  permissions_boundary_arn     = var.account_settings_config.permissions_boundary_arn
  identity_center              = var.account_settings_config.identity_center
  catalog_ready                = local.s3tables_catalog_ready
  kms_key_deletion_window_days = local.effective_config.kms_key_deletion_window_days
  key_administrator_arns       = local.key_administrator_arns

  key_user_arns = local.tenant_persona_arns
  # Tenant keys are created inside the tenant modules after this call; catalog_key_guard checks them
  storage_key_arns = compact([local.kms_key_arn])
}

# ----
# Catalog integration (account and Region singleton)
# ----

# The dependency is one-way: the submodule's data-access role is referenced by key policies, so
# the submodule takes no key ARNs.
module "catalog_integration" {
  source = "./modules/catalog-integration"
  count  = var.enabled && var.catalog_integration_config.enabled ? 1 : 0

  name_prefix              = local.name_prefix
  account_id               = local.account_id
  partition                = local.partition
  region                   = local.region
  tags                     = local.common_tags
  settings_ready           = local.settings_ready
  catalog_ready            = local.s3tables_catalog_ready
  permissions_boundary_arn = var.account_settings_config.permissions_boundary_arn
}

moved {
  from = module.catalog_integration[0].aws_lakeformation_data_lake_settings.this
  to   = module.account_settings[0].aws_lakeformation_data_lake_settings.this
}

# ----
# Guards
# ----

moved {
  from = terraform_data.secondary_admin_guard
  to   = terraform_data.settings_guard
}

# Deployments that do not own the account settings check the live values they depend on (FR-001,
# FR-007) instead of changing them
resource "terraform_data" "settings_guard" {
  count = var.enabled && !local.settings_owner ? 1 : 0

  lifecycle {
    precondition {
      condition     = contains(data.aws_lakeformation_data_lake_settings.current[0].admins, local.deployer_role_arn)
      error_message = "The deploying role (${local.deployer_role_arn}) is not a Lake Formation data-lake admin. Add it to admin_principal_arns in the deployment that owns the account settings, apply that deployment, then retry."
    }

    precondition {
      condition     = local.owner_only_inputs_default
      error_message = "This deployment does not own the account settings (account_settings_config.owner is false), so cross_account_version, data_lake, catalog_encryption, catalog_resource_policy, permissions_boundary_arn, s3tables_catalog, and identity_center must keep their defaults. Set them in the owner deployment."
    }

    precondition {
      condition     = local.live_catalog_encryption_mode != "DISABLED"
      error_message = "Glue Data Catalog encryption is DISABLED in this account and Region. Apply the deployment that owns the account settings first."
    }

    precondition {
      condition = alltrue([for t in values(var.tenants) : length(t.shares) == 0]) || (
        try(tonumber(data.aws_lakeformation_data_lake_settings.current[0].parameters["CROSS_ACCOUNT_VERSION"]), 0) >= 4
      )
      error_message = "Tenants declare cross-account shares, but the live Lake Formation cross-account version is below 4. Apply the deployment that owns the account settings first."
    }

    precondition {
      condition = !var.studio_config.enabled || length(setsubtract(
        var.studio_config.integration_principal_arns,
        data.aws_lakeformation_data_lake_settings.current[0].admins,
      )) == 0
      error_message = "studio_config.integration_principal_arns must already be data-lake admins. Add them to studio_config in the deployment that owns the account settings, apply it, then retry."
    }
  }
}

# The approved consumer list never includes this account (FR-022)
resource "terraform_data" "sharing_guard" {
  count = var.enabled && length(var.sharing_config.consumer_accounts) > 0 ? 1 : 0

  lifecycle {
    precondition {
      condition     = !contains(keys(var.sharing_config.consumer_accounts), local.account_id)
      error_message = "sharing_config.consumer_accounts must not include this account (${local.account_id}); shares go only to other accounts."
    }
  }
}

# Studio integration principals become authoritative data-lake admins on the owner, so they must be
# roles in this account and partition (FR-007)
resource "terraform_data" "studio_guard" {
  count = var.enabled && var.studio_config.enabled ? 1 : 0

  lifecycle {
    precondition {
      condition = alltrue([
        for a in var.studio_config.integration_principal_arns :
        startswith(a, "arn:${local.partition}:iam::${local.account_id}:role/")
      ])
      error_message = "studio_config.integration_principal_arns must be roles in this account (${local.account_id}) and partition: ${join(", ", [for a in var.studio_config.integration_principal_arns : a if !startswith(a, "arn:${local.partition}:iam::${local.account_id}:role/")])}."
    }
  }
}

# A supplied catalog key must not be a designated tenant's key (FR-009). The platform key is
# checked inside modules/account-settings.
resource "terraform_data" "catalog_key_guard" {
  count = var.enabled && local.settings_owner && local.supplied_catalog_kms_key_arn != "" ? 1 : 0

  lifecycle {
    precondition {
      condition     = !contains([for t in values(module.tenant) : t.kms_key_arn], local.supplied_catalog_kms_key_arn)
      error_message = "account_settings_config.catalog_encryption.kms_key_arn is a tenant key. The catalog needs a dedicated key."
    }
  }
}

# ----
# Platform-applied tenants
# ----

# No module-level depends_on: it would defer every data source in the tenant (cost lookups,
# session context) to apply time. Ordering comes from the contract instead: it references the
# platform LF-tags, which depend on the catalog integration and the admin guard, so every tenant
# resource that reads var.platform waits for them.
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
}
