# ----
# Lake Formation data-lake settings
# ----

# Live settings: the cross-account version is never lowered (FR-005)
data "aws_lakeformation_data_lake_settings" "current" {}

# Authoritative for ALL data-lake settings in the account and Region: admins not listed here are
# removed. Tenant pipeline roles, secondary deployers, and studio integration principals must be
# in admin_principal_arns. PutDataLakeSettings replaces the whole settings object, so every field
# is declared here and no apply resets a value implicitly (FR-003).
resource "aws_lakeformation_data_lake_settings" "this" {
  admins = var.admin_principal_arns

  # Omitting this would erase any read-only admins configured outside Terraform on every apply
  read_only_admins = var.read_only_admin_principal_arns

  # No create_*_default_permissions blocks: omitting them sends empty lists, which removes the
  # IAM_ALLOWED_PRINCIPALS defaults, so Lake Formation grants are the only access to new
  # databases and tables. (An empty block would send an empty principal, which the API rejects.)

  allow_external_data_filtering         = var.data_lake.allow_external_data_filtering
  external_data_filtering_allow_list    = var.data_lake.external_data_filtering_allow_list
  authorized_session_tag_value_list     = var.data_lake.authorized_session_tag_values
  allow_full_table_external_data_access = var.data_lake.allow_full_table_external_data_access
  trusted_resource_owners               = var.data_lake.trusted_resource_owners

  # SET_CONTEXT keeps the caller's identity in vended-credential sessions; the live value is "TRUE"
  # (sandbox-results.md item 1)
  parameters = {
    CROSS_ACCOUNT_VERSION = tostring(var.cross_account_version)
    SET_CONTEXT           = "TRUE"
  }

  lifecycle {
    precondition {
      condition     = contains(var.admin_principal_arns, var.deployer_role_arn)
      error_message = "The deploying role (${var.deployer_role_arn}) must be in admin_principal_arns, or this apply would remove its own data-lake admin rights and fail every later grant."
    }

    precondition {
      condition     = try(tonumber(data.aws_lakeformation_data_lake_settings.current.parameters["CROSS_ACCOUNT_VERSION"]), 0) <= var.cross_account_version
      error_message = "The live Lake Formation cross-account version (${try(data.aws_lakeformation_data_lake_settings.current.parameters["CROSS_ACCOUNT_VERSION"], "unset")}) is above cross_account_version (${var.cross_account_version}). Versions are never lowered; set cross_account_version to the live value."
    }
  }
}

# ----
# Glue Data Catalog encryption (FR-009 to FR-014)
# ----

# Live settings guard the migration and key-change rules below
data "aws_glue_data_catalog_encryption_settings" "current" {
  catalog_id = var.account_id
}

locals {
  live_encryption    = one(data.aws_glue_data_catalog_encryption_settings.current.data_catalog_encryption_settings[0].encryption_at_rest)
  live_mode          = try(local.live_encryption.catalog_encryption_mode, "DISABLED")
  live_catalog_key   = try(local.live_encryption.sse_aws_kms_key_id, "")
  live_key_inspected = local.live_mode != "DISABLED" && local.live_catalog_key != ""
}

# Resolves an alias or key ID in the live settings to a key ARN and its manager. No count: the live
# settings can be unknown until apply (for example under a module-level depends_on), and a count
# cannot depend on them. Without a live key it reads the catalog key, and the checks ignore it.
data "aws_kms_key" "live_catalog" {
  key_id = local.live_key_inspected ? local.live_catalog_key : local.catalog_key_arn
}

# Enabling encryption covers only objects written afterwards (FR-014). Disabling leaves existing
# objects encrypted, which is why the catalog key is protected (FR-013). Destroying this resource
# resets the catalog to DISABLED (sandbox-results.md item 2).
resource "aws_glue_data_catalog_encryption_settings" "this" {
  catalog_id = var.account_id

  data_catalog_encryption_settings {
    encryption_at_rest {
      catalog_encryption_mode         = local.service_role_mode ? "SSE-KMS-WITH-SERVICE-ROLE" : "SSE-KMS"
      sse_aws_kms_key_id              = local.catalog_key_arn
      catalog_encryption_service_role = local.service_role_mode ? aws_iam_role.catalog_encryption[0].arn : null
    }

    connection_password_encryption {
      return_connection_password_encrypted = true
      aws_kms_key_id                       = local.catalog_key_arn
    }
  }

  depends_on = [aws_iam_role_policy.catalog_encryption, time_sleep.catalog_encryption_role]

  lifecycle {
    # The catalog key must never encrypt table storage or query results (FR-009)
    precondition {
      condition     = !contains(var.storage_key_arns, local.catalog_key_arn)
      error_message = "catalog_encryption.kms_key_arn (${local.catalog_key_arn}) is a table-storage or query-results key. The catalog needs a dedicated key; leave kms_key_arn empty to create one."
    }

    # Objects encrypted with the AWS managed aws/glue key become inaccessible under the service role
    precondition {
      condition = !(
        local.service_role_mode && local.live_mode == "SSE-KMS" &&
        data.aws_kms_key.live_catalog.key_manager == "AWS"
      )
      error_message = "The catalog is encrypted with the AWS managed aws/glue key. Switching to SSE-KMS-WITH-SERVICE-ROLE would make existing catalog objects inaccessible. See the README section \"Migrating catalog encryption\"."
    }

    # The catalog key is chosen at installation; no prior installations exist to migrate (FR-013)
    precondition {
      condition = (
        var.environment_type == "Ephemeral" || !local.live_key_inspected ||
        data.aws_kms_key.live_catalog.arn == local.catalog_key_arn
      )
      error_message = "The catalog is already encrypted with ${data.aws_kms_key.live_catalog.arn}. Changing the catalog key after installation is not supported outside Ephemeral environments."
    }
  }
}

# Ephemeral owners are torn down, but a key they created cannot be deleted (FR-013)
check "ephemeral_catalog_key" {
  assert {
    condition     = !(var.environment_type == "Ephemeral" && var.catalog_encryption.kms_key_arn == "")
    error_message = "An Ephemeral deployment owns the account settings and creates its own catalog key. Teardown cannot delete that key, because it protects account-wide catalog metadata. Set account_settings_config.catalog_encryption.kms_key_arn to a long-lived key instead."
  }
}

# ----
# Glue Data Catalog resource policy (FR-006)
# ----

# None by default: LF-tag sharing on cross-account version 4 needs no catalog resource policy
# (plan R3). An unmanaged policy without the RAM statement breaks every share; lfverify reports it.
resource "aws_glue_resource_policy" "this" {
  count = var.catalog_resource_policy.enabled ? 1 : 0

  policy = data.aws_iam_policy_document.catalog_resource_policy[0].json

  # Required once RAM-based grants exist alongside the policy
  enable_hybrid = "TRUE"
}

# source_policy_documents rather than override: a caller statement cannot replace RamShareResource
data "aws_iam_policy_document" "catalog_resource_policy" {
  count = var.catalog_resource_policy.enabled ? 1 : 0

  source_policy_documents = var.catalog_resource_policy.deny_policy_jsons

  statement {
    sid     = "RamShareResource"
    effect  = "Allow"
    actions = ["glue:ShareResource"]
    principals {
      type        = "Service"
      identifiers = ["ram.amazonaws.com"]
    }
    resources = [
      "arn:${var.partition}:glue:${var.region}:${var.account_id}:catalog",
      "arn:${var.partition}:glue:${var.region}:${var.account_id}:database/*",
      "arn:${var.partition}:glue:${var.region}:${var.account_id}:table/*/*",
    ]
  }
}
