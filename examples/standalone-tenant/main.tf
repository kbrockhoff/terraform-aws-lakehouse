# Main AWS provider - uses the current region
provider "aws" {}

# Pricing provider - always uses us-east-1 where the AWS Pricing API is available
provider "aws" {
  alias  = "pricing"
  region = "us-east-1"
}

# A tenant provisioned by its own pipeline, with its own state. In production, read the contract
# from the platform's remote state or SSM parameter (see README); here it comes from the
# platform_contract.auto.tfvars.json fixture.
module "tenant" {
  source = "../../modules/tenant"

  providers = {
    aws         = aws
    aws.pricing = aws.pricing
  }

  platform              = var.platform_contract
  name                  = "sales"
  tenant                = merge(var.tenants["sales"], var.tenant_overrides)
  verify_deployer_admin = var.verify_deployer_admin
}
