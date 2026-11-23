# Main AWS provider - uses the current region
provider "aws" {}

# Pricing provider - always uses us-east-1 where the AWS Pricing API is available
provider "aws" {
  alias  = "pricing"
  region = "us-east-1"
}

data "aws_caller_identity" "current" {}

data "aws_iam_session_context" "current" {
  arn = data.aws_caller_identity.current.arn
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
    enabled              = false
    data_access_role_arn = var.data_access_role_arn
  }

  # Another deployment owns the account-wide Lake Formation and Glue settings. This deployment
  # only checks the live values it depends on (data-lake admin, catalog encryption on). The owner
  # flag defaults to catalog_integration_config.enabled; it is set here for clarity.
  account_settings_config = {
    owner = false
  }

  # Admins are managed by the owning deployment; this list is not applied here
  governance_config = {
    admin_principal_arns = []
  }

  # This deployment creates its own platform key, administered by the roles that apply it
  encryption_config = {
    create_kms_key               = true
    kms_key_id                   = ""
    kms_key_deletion_window_days = 14
    key_administrator_arns       = [data.aws_iam_session_context.current.issuer_arn]
  }

  tenants = {
    research = {
      owner       = "research-data@example.com"
      sensitivity = "internal"
    }
  }
}
