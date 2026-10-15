# Standalone Tenant Example

Provisions one tenant with `modules/tenant` directly, in its own state, the way a tenant
provisioning pipeline does. It touches no platform resource.

Here the platform contract is a fixture (`platform_contract.auto.tfvars.json`) shaped like the
`examples/complete` platform. In production, read the real contract:

```hcl
data "terraform_remote_state" "platform" {
  backend = "s3"
  config = {
    bucket = "my-terraform-state"
    key    = "lakehouse/platform.tfstate"
    region = "us-east-1"
  }
}

module "tenant" {
  source   = "git::https://github.com/kbrockhoff/terraform-aws-lakehouse.git//modules/tenant?ref=vX.Y.Z"
  platform = data.terraform_remote_state.platform.outputs.platform_contract
  # or: jsondecode(nonsensitive(data.aws_ssm_parameter.platform_contract.value))
  name   = "sales"
  tenant = { owner = "sales-data@example.com", sensitivity = "confidential" }

  providers = { aws = aws, aws.pricing = aws.pricing }
}
```

The pipeline role must be a Lake Formation data-lake admin (`verify_deployer_admin = true`
checks this at plan time) and should use the `scope = "tenant"` deployer policy.

<!-- BEGIN_TF_DOCS -->
<!-- END_TF_DOCS -->
