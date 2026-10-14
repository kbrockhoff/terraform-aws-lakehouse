provider "aws" {}

data "aws_caller_identity" "current" {}
data "aws_partition" "current" {}
data "aws_region" "current" {}

# Policy for the role that deploys the platform (root module)
module "platform" {
  source = "../../modules/deployer"

  name_prefix             = var.name_prefix
  scope                   = "platform"
  iam_role_arn            = var.iam_role_arn
  cost_estimation_enabled = var.cost_estimation_enabled
  partition               = data.aws_partition.current.partition
  region                  = data.aws_region.current.region
  account_id              = data.aws_caller_identity.current.account_id
}

# Policy for a standalone tenant provisioning pipeline role
module "tenant" {
  source = "../../modules/deployer"

  name_prefix             = var.name_prefix
  scope                   = "tenant"
  iam_role_arn            = var.iam_role_arn
  cost_estimation_enabled = var.cost_estimation_enabled
  partition               = data.aws_partition.current.partition
  region                  = data.aws_region.current.region
  account_id              = data.aws_caller_identity.current.account_id
}
