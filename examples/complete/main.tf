# Main AWS provider - uses the current region
provider "aws" {}

# Pricing provider - always uses us-east-1 where the AWS Pricing API is available
provider "aws" {
  alias  = "pricing"
  region = "us-east-1"
}

data "aws_caller_identity" "current" {}

# The role running the plan, so the data-lake admin precondition passes for whoever runs it
data "aws_iam_session_context" "current" {
  arn = data.aws_caller_identity.current.arn
}

locals {
  # Tenant definitions come from tenants.auto.tfvars. Tests adjust them through
  # claims_compliance_frameworks, tenant_overrides, and extra_tenants.
  tenants = merge(
    {
      for name, t in var.tenants : name => merge(
        t,
        name == "claims" ? { compliance_frameworks = var.claims_compliance_frameworks } : {},
        lookup(var.tenant_overrides, name, {}),
      )
    },
    var.extra_tenants,
  )

  studio_config = merge(var.studio_config, {
    integration_principal_arns = [
      for a in lookup(var.studio_config, "integration_principal_arns", []) :
      replace(a, "ACCOUNT_ID", data.aws_caller_identity.current.account_id)
    ]
  })
}

module "main" {
  source = "../../"

  providers = {
    aws         = aws
    aws.pricing = aws.pricing
  }

  enabled                = var.enabled
  name_prefix            = var.name_prefix
  tags                   = var.tags
  data_tags              = var.data_tags
  environment_type       = var.environment_type
  cost_estimation_config = var.cost_estimation_config
  # Key administrators default to the deploying role; production lists every applying role
  encryption_config = merge(var.encryption_config, {
    key_administrator_arns = length(var.encryption_config.key_administrator_arns) > 0 ? var.encryption_config.key_administrator_arns : [data.aws_iam_session_context.current.issuer_arn]
  })
  monitoring_config = var.monitoring_config
  alarms_config     = var.alarms_config
  zones_config      = var.zones_config
  storage_config    = var.storage_config
  query_config      = var.query_config

  catalog_integration_config = var.catalog_integration_config
  account_settings_config    = var.account_settings_config
  studio_config              = local.studio_config
  sharing_config             = var.sharing_config

  governance_config = merge(var.governance_config, {
    admin_principal_arns = length(var.admin_principal_arns) > 0 ? var.admin_principal_arns : [data.aws_iam_session_context.current.issuer_arn]
  })

  tenants = local.tenants
}
