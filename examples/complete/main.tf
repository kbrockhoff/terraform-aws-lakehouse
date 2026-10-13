# Main AWS provider - uses the current region
provider "aws" {
  # This is the default provider used for VPC resources
}

# Pricing provider - always uses us-east-1 where the AWS Pricing API is available
provider "aws" {
  alias  = "pricing"
  region = "us-east-1"
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
  networktags_name       = var.networktags_name
  encryption_config      = var.encryption_config
  monitoring_config      = var.monitoring_config
  alarms_config          = var.alarms_config
}
