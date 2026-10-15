# Main AWS provider - uses the current region
provider "aws" {}

# Pricing provider - always uses us-east-1 where the AWS Pricing API is available
provider "aws" {
  alias  = "pricing"
  region = "us-east-1"
}

# A second platform in an account and Region whose catalog integration another deployment owns
module "main" {
  source = "../../"

  providers = {
    aws         = aws
    aws.pricing = aws.pricing
  }

  name_prefix      = var.name_prefix
  environment_type = "Development"
  tags = {
    Example = "secondary"
  }

  catalog_integration_config = {
    enabled                               = false
    data_access_role_arn                  = var.data_access_role_arn
    allow_full_table_external_data_access = false
  }

  # Admins are managed by the owning deployment; this list is not applied here
  governance_config = {
    admin_principal_arns = []
  }

  tenants = {
    research = {
      owner       = "research-data@example.com"
      sensitivity = "internal"
    }
  }
}
