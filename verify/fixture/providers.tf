# Main AWS provider - uses the current region. Every fixture resource carries the run tags,
# so lfverify cleanup can sweep a run whose state was lost (FR-004).
provider "aws" {
  default_tags {
    tags = {
      LfverifyRun = var.run_id
      Purpose     = "access-verification"
    }
  }
}

# Pricing provider - always uses us-east-1 where the AWS Pricing API is available.
# The module requires the alias even though the fixture disables cost estimation.
provider "aws" {
  alias  = "pricing"
  region = "us-east-1"

  default_tags {
    tags = {
      LfverifyRun = var.run_id
      Purpose     = "access-verification"
    }
  }
}
