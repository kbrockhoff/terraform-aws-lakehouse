# ----
# Pricing Calculator
# ----

module "pricing" {
  source = "./modules/cost-estimation"

  providers = {
    aws = aws.pricing
  }

  enabled                 = var.enabled && var.cost_estimation_config.enabled
  region                  = local.region
  kms_key_count           = var.enabled && var.encryption_config.create_kms_key ? 1 : 0
  cloudwatch_metric_count = var.enabled && var.alarms_config.enabled ? 5 : 0 # Estimate 5 metrics when monitoring enabled
  cloudwatch_alarm_count  = var.enabled && var.alarms_config.enabled ? 3 : 0 # Estimate 3 alarms when monitoring enabled
}
