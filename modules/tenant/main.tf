# ----
# Guards
# ----

# Standalone pipelines must run as a data-lake admin; the root guards this itself
resource "terraform_data" "admin_guard" {
  count = var.verify_deployer_admin ? 1 : 0

  lifecycle {
    precondition {
      condition     = contains(data.aws_lakeformation_data_lake_settings.current[0].admins, data.aws_iam_session_context.current.issuer_arn)
      error_message = "Tenant ${var.name}: the pipeline role ${data.aws_iam_session_context.current.issuer_arn} is not a Lake Formation data-lake admin. Add it to the platform's governance_config.admin_principal_arns and apply the platform first."
    }
  }
}

# Shares go only to approved accounts in this Region, never to this account, and a designated
# tenant must acknowledge that each consumer is in scope for its frameworks (FR-023, FR-025, FR-028)
resource "terraform_data" "share_guard" {
  count = length(var.tenant.shares) > 0 ? 1 : 0

  lifecycle {
    precondition {
      condition     = alltrue([for s in values(var.tenant.shares) : contains(keys(var.platform.sharing.consumer_accounts), s.account_id)])
      error_message = "Tenant ${var.name}: shares ${join(", ", sort([for k, s in var.tenant.shares : k if !contains(keys(var.platform.sharing.consumer_accounts), s.account_id)]))} target accounts that are not approved. Add them to the platform's sharing_config.consumer_accounts first."
    }

    precondition {
      condition     = alltrue([for s in values(var.tenant.shares) : s.account_id != var.platform.account_id])
      error_message = "Tenant ${var.name}: a share cannot target the platform's own account (${var.platform.account_id})."
    }

    precondition {
      condition     = !local.designated || alltrue([for s in values(var.tenant.shares) : s.compliance_acknowledged])
      error_message = "Tenant ${var.name} is designated (${join(", ", sort(tolist(var.tenant.compliance_frameworks)))}). Set compliance_acknowledged = true on each share to confirm the consumer is in scope for those frameworks."
    }

    # Standalone tenants read live, not from the contract: a copy taken at platform-apply time can
    # be stale. Platform-applied tenants rely on the root (owner: validated version >= 4, applied
    # before tenant grants; non-owner: settings_guard checks the live version).
    precondition {
      condition     = !var.verify_deployer_admin || try(tonumber(data.aws_lakeformation_data_lake_settings.current[0].parameters["CROSS_ACCOUNT_VERSION"]), 0) >= 4
      error_message = "Tenant ${var.name}: sharing S3 Tables needs Lake Formation cross-account version 4 or later, but the live version is ${try(data.aws_lakeformation_data_lake_settings.current[0].parameters["CROSS_ACCOUNT_VERSION"], "unset")}. Apply the deployment that owns the account settings first."
    }
  }
}

# FR-030: designated (regulated) tenants belong in Enhanced environments
check "regulated_data_in_baseline" {
  assert {
    condition     = !(local.designated && var.platform.baseline_environment)
    error_message = "Tenant ${var.name} is designated (${join(", ", sort(tolist(var.tenant.compliance_frameworks)))}) but the platform environment type ${var.platform.environment_type} is Baseline. Regulated data should run in UAT, Production, or MissionCritical."
  }
}

# ----
# Cost estimation (FR-024)
# ----

module "cost_estimation" {
  source = "../cost-estimation"

  providers = {
    aws = aws.pricing
  }

  enabled                  = var.platform.cost_estimation_enabled
  region                   = local.region
  compliance_kms_key_count = local.designated ? 1 : 0 # the tenant key exists only for the designation (FR-024)
  table_bucket_count       = local.dedicated ? length(local.zones) : 0
  s3_bucket_count          = local.designated ? 1 : 0
  athena_workgroup_count   = local.workgroup_count
  cloudwatch_metric_count  = local.effective.monitoring_enabled ? 2 * local.workgroup_count : 0
  # Alarms are billed per metric: the failure alarm has one per query type, scanned bytes one
  alarm_count = local.effective.alarms_enabled ? (length(local.alarmed_query_types) + 1) * local.workgroup_count : 0
}
