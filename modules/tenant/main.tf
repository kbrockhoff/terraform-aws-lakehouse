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
  athena_workgroup_count   = 1
  cloudwatch_metric_count  = local.effective.monitoring_enabled ? 2 : 0
  alarm_count              = local.effective.alarms_enabled ? 2 : 0
}
