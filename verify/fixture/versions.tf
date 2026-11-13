terraform {
  required_version = ">= 1.11"

  # Each run keeps its own state under verify/out/<run-id>; make verify-* passes the path with
  # -backend-config
  backend "local" {}

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 6.59.0, < 7.0.0"
    }
  }
}
