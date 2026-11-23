terraform {
  required_version = ">= 1.11"

  required_providers {
    aws = {
      source                = "hashicorp/aws"
      version               = ">= 6.59.0"
      configuration_aliases = [aws.pricing]
    }
    time = {
      source  = "hashicorp/time"
      version = ">= 0.12"
    }
  }
}

locals {
  module_version = "v0.5.0"
}
