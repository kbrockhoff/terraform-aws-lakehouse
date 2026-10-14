# Tenant Submodule

The single implementation of a lakehouse tenant: an S3 Tables namespace per zone, the
tenant-owned LF-tag, database labels, persona grants, a query workgroup, and alarms. Designated
(compliance) tenants also get dedicated table buckets, a tenant KMS key, and a dedicated results
bucket.

## Entry points

The same submodule runs in two places, and resource addresses inside it are identical in both.

Platform-applied, from the root module's `tenants` input:

```hcl
module "tenant" {
  source    = "./modules/tenant"
  for_each  = var.enabled ? var.tenants : {}
  providers = { aws = aws, aws.pricing = aws.pricing }

  platform              = local.platform_contract
  name                  = each.key
  tenant                = each.value
  verify_deployer_admin = false # the root already guards (FR-013)
}
```

Standalone, from a tenant provisioning pipeline with its own state:

```hcl
module "tenant_sales" {
  source   = "git::https://github.com/kbrockhoff/terraform-aws-lakehouse.git//modules/tenant?ref=vX.Y.Z"
  platform = data.terraform_remote_state.platform.outputs.platform_contract
  name     = "sales"
  tenant   = { owner = "sales-data@example.com", sensitivity = "confidential" }
  tags     = module.context.tags

  providers = { aws = aws, aws.pricing = aws.pricing }
}
```

## Sharing the platform contract

The `platform_contract` output contains identifiers and settings only, no secrets. Pipelines can
read it in two ways:

- **Remote state**: `data.terraform_remote_state.platform.outputs.platform_contract`.
- **SSM parameter**: the platform's root configuration publishes it, and pipelines decode it.

  ```hcl
  # platform root
  resource "aws_ssm_parameter" "platform_contract" {
    name  = "/lakehouse/platform-contract"
    type  = "String"
    value = jsonencode(module.lakehouse.platform_contract)
  }

  # tenant pipeline
  data "aws_ssm_parameter" "platform_contract" {
    name = "/lakehouse/platform-contract"
  }
  module "tenant_sales" {
    # ...
    platform = jsondecode(data.aws_ssm_parameter.platform_contract.value)
  }
  ```

The lakehouse module does not create that parameter itself.

## Pipelines are platform-operated

Creating a tenant's LF-tag and grants needs Lake Formation admin authority. Tenant provisioning
pipelines are therefore operated by the platform team:

- the pipeline role is listed in the platform's `governance_config.admin_principal_arns`
  (this submodule checks that unless `verify_deployer_admin = false`);
- it uses the `scope = "tenant"` deployer policy from `modules/deployer`;
- it is never handed to tenant-operated identities (FR-001a).

## Names are unique across entry points

There is no registry. A tenant name used in both the platform and a pipeline fails at apply,
on the existing namespace or LF-tag key.

## Moving a tenant between entry points

Move state between `module.tenant["<name>"]` in the platform state and the pipeline's module
address, for example with `terraform state mv`, or with `removed` blocks in the source
configuration and `import` blocks in the target. Every resource address inside the submodule is
the same in both entry points, so no resource is replaced.

## Labeling and roll-up contract

The submodule labels each tenant database with the tenant's most restrictive values, so a
table nobody has labeled yet fails closed. It never labels tables or columns. Writer pipelines
must:

1. label every column with `sensitivity`, every `reg-*` key, and optionally `domain`;
2. set the table's `sensitivity` to the highest column value and each `reg-*` to `yes` if any
   column is `yes`;
3. apply the table labels;
4. on schema change, label new columns and recompute the table labels.

Lake Formation does not enforce the roll-up, and the module does not detect drift.

<!-- BEGIN_TF_DOCS -->
<!-- END_TF_DOCS -->
