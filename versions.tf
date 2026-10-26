terraform {
  required_version = ">= 1.11"

  required_providers {
    aws = {
      source                = "hashicorp/aws"
      version               = ">= 6.59.0, < 7.0.0"
      configuration_aliases = [aws.pricing]
    }
  }
}

locals {
  module_version = "v0.1.0"
}
