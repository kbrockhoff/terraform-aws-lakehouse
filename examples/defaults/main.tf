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

module "main" {
  source = "../../"

  providers = {
    aws         = aws
    aws.pricing = aws.pricing
  }

  name_prefix      = var.name_prefix
  environment_type = "Development"
  tags = {
    Example = "defaults"
  }

  governance_config = {
    admin_principal_arns = length(var.admin_principal_arns) > 0 ? var.admin_principal_arns : [data.aws_iam_session_context.current.issuer_arn]
  }

  zones_config           = var.zones_config
  cost_estimation_config = var.cost_estimation_config

  tenants = {
    analytics = {
      owner       = "analytics-data@example.com"
      sensitivity = "internal"
    }
  }
}
