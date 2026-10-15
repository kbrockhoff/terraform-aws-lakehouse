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
    # The parameter value is marked sensitive, but the contract holds no secrets, and
    # Terraform cannot use sensitive values in for_each
    platform = jsondecode(nonsensitive(data.aws_ssm_parameter.platform_contract.value))
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
## Requirements

| Name | Version |
| ---- | ------- |
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.11 |
| <a name="requirement_aws"></a> [aws](#requirement\_aws) | >= 6.59.0, < 7.0.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_aws"></a> [aws](#provider\_aws) | >= 6.59.0, < 7.0.0 |
| <a name="provider_terraform"></a> [terraform](#provider\_terraform) | n/a |

## Modules

| Name | Source | Version |
| ---- | ------ | ------- |
| <a name="module_cost_estimation"></a> [cost\_estimation](#module\_cost\_estimation) | ../cost-estimation | n/a |

## Resources

| Name | Type |
| ---- | ---- |
| [aws_athena_workgroup.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/athena_workgroup) | resource |
| [aws_cloudwatch_metric_alarm.query_failures](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/cloudwatch_metric_alarm) | resource |
| [aws_cloudwatch_metric_alarm.scanned_bytes](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/cloudwatch_metric_alarm) | resource |
| [aws_kms_alias.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/kms_alias) | resource |
| [aws_kms_key.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/kms_key) | resource |
| [aws_lakeformation_data_cells_filter.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lakeformation_data_cells_filter) | resource |
| [aws_lakeformation_lf_tag.tenant](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lakeformation_lf_tag) | resource |
| [aws_lakeformation_permissions.reader_database](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lakeformation_permissions) | resource |
| [aws_lakeformation_permissions.reader_table](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lakeformation_permissions) | resource |
| [aws_lakeformation_permissions.row_filter](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lakeformation_permissions) | resource |
| [aws_lakeformation_permissions.steward_database](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lakeformation_permissions) | resource |
| [aws_lakeformation_permissions.steward_label](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lakeformation_permissions) | resource |
| [aws_lakeformation_permissions.steward_table](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lakeformation_permissions) | resource |
| [aws_lakeformation_permissions.steward_tenant_tag](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lakeformation_permissions) | resource |
| [aws_lakeformation_permissions.steward_zone_tag](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lakeformation_permissions) | resource |
| [aws_lakeformation_permissions.writer_database](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lakeformation_permissions) | resource |
| [aws_lakeformation_permissions.writer_label](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lakeformation_permissions) | resource |
| [aws_lakeformation_permissions.writer_table](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lakeformation_permissions) | resource |
| [aws_lakeformation_resource_lf_tags.database](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lakeformation_resource_lf_tags) | resource |
| [aws_s3_bucket.results](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket) | resource |
| [aws_s3_bucket_lifecycle_configuration.results](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket_lifecycle_configuration) | resource |
| [aws_s3_bucket_ownership_controls.results](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket_ownership_controls) | resource |
| [aws_s3_bucket_policy.results](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket_policy) | resource |
| [aws_s3_bucket_public_access_block.results](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket_public_access_block) | resource |
| [aws_s3_bucket_server_side_encryption_configuration.results](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket_server_side_encryption_configuration) | resource |
| [aws_s3tables_namespace.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3tables_namespace) | resource |
| [aws_s3tables_table_bucket.dedicated](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3tables_table_bucket) | resource |
| [aws_s3tables_table_bucket_policy.dedicated](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3tables_table_bucket_policy) | resource |
| [terraform_data.admin_guard](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_name"></a> [name](#input\_name) | Tenant name. It becomes the S3 Tables namespace and part of bucket, workgroup, and LF-tag names. | `string` | n/a | yes |
| <a name="input_platform"></a> [platform](#input\_platform) | The platform\_contract output of the root lakehouse module, from remote state, a decoded SSM parameter, or the root itself. | <pre>object({<br/>    name_prefix          = string<br/>    account_id           = string<br/>    partition            = string<br/>    region               = string<br/>    environment_type     = string<br/>    baseline_environment = bool<br/>    effective = object({<br/>      bytes_scanned_cutoff_per_query = number<br/>      query_results_expiration_days  = number<br/>      unreferenced_days              = number<br/>      non_current_days               = number<br/>      force_destroy                  = bool<br/>      writer_drop_enabled            = bool<br/>      monitoring_enabled             = bool<br/>      alarms_enabled                 = bool<br/>      scanned_bytes_alarm_threshold  = number<br/>      kms_key_deletion_window_days   = number<br/>      table_storage_class            = string<br/>    })<br/>    shared_storage_classification = string<br/>    zones = map(object({<br/>      bucket_name = string<br/>      bucket_arn  = string<br/>      catalog_id  = string<br/>    }))<br/>    lf_tags = object({<br/>      sensitivity = object({<br/>        key    = string<br/>        values = list(string)<br/>      })<br/>      regulations = map(string)<br/>      domain = object({<br/>        key    = string<br/>        values = list(string)<br/>      })<br/>      zone = object({<br/>        key    = string<br/>        values = list(string)<br/>      })<br/>      tenant_key_prefix = string<br/>    })<br/>    regulation_designation  = map(string)<br/>    kms_key_arn             = string<br/>    data_access_role_arn    = string<br/>    query_results           = object({ bucket_name = string, bucket_arn = string })<br/>    alarm_sns_topic_arn     = string<br/>    cost_estimation_enabled = bool<br/>    common_tags             = map(string)<br/>    data_tags               = map(string)<br/>    required_tags = object({<br/>      enforced                = bool<br/>      cost_center_key         = string<br/>      data_owner_key          = string<br/>      data_classification_key = string<br/>      tenant_key              = string<br/>    })<br/>  })</pre> | n/a | yes |
| <a name="input_tenant"></a> [tenant](#input\_tenant) | Tenant definition: owner, sensitivity ceiling, regulations, domain, zones, compliance designation, personas, row filters, and query limits. | <pre>object({<br/>    owner                 = string<br/>    sensitivity           = string<br/>    regulations           = optional(set(string), [])<br/>    domain                = optional(string)<br/>    zones                 = optional(list(string))<br/>    compliance_frameworks = optional(set(string), [])<br/>    stewards              = optional(list(string), [])<br/>    writers = optional(map(object({<br/>      principal_arn = string<br/>      zones         = optional(list(string))<br/>    })), {})<br/>    readers = optional(map(object({<br/>      principal_arn      = string<br/>      zones              = optional(list(string))<br/>      sensitivity_values = list(string)<br/>      regulations        = optional(set(string), [])<br/>      domains            = optional(list(string))<br/>    })), {})<br/>    row_filters = optional(map(object({<br/>      reader                = string<br/>      zone                  = string<br/>      table                 = string<br/>      row_filter_expression = string<br/>      column_names          = optional(list(string))<br/>    })), {})<br/>    bytes_scanned_cutoff_per_query = optional(number)<br/>    scanned_bytes_alarm_threshold  = optional(number)<br/>    dedicated_storage              = optional(bool, false)<br/>  })</pre> | n/a | yes |
| <a name="input_tags"></a> [tags](#input\_tags) | Additional tags for the tenant's resources. | `map(string)` | `{}` | no |
| <a name="input_verify_deployer_admin"></a> [verify\_deployer\_admin](#input\_verify\_deployer\_admin) | Fail the plan unless the deploying role is a Lake Formation data-lake admin. The root sets false because it guards this itself. | `bool` | `true` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_catalog_ids"></a> [catalog\_ids](#output\_catalog\_ids) | Lake Formation catalog ID (<account>:s3tablescatalog/<bucket>) holding the tenant database, per zone. |
| <a name="output_compliance_monthly_cost"></a> [compliance\_monthly\_cost](#output\_compliance\_monthly\_cost) | Portion of the monthly estimate caused by the compliance designation (the tenant key). |
| <a name="output_kms_key_arn"></a> [kms\_key\_arn](#output\_kms\_key\_arn) | Key encrypting the tenant's tables and results: the tenant key when designated, otherwise the platform key. |
| <a name="output_monthly_cost_estimate"></a> [monthly\_cost\_estimate](#output\_monthly\_cost\_estimate) | Estimated idle monthly cost in USD of this tenant's resources. |
| <a name="output_namespaces"></a> [namespaces](#output\_namespaces) | Tenant namespace (Lake Formation database name) per zone. |
| <a name="output_persona_policy_json"></a> [persona\_policy\_json](#output\_persona\_policy\_json) | Least-privilege IAM policy JSON per persona, keyed <steward\|writer\|reader>/<key>. Attach to the persona roles; the module does not. |
| <a name="output_query_results_location"></a> [query\_results\_location](#output\_query\_results\_location) | S3 location of the tenant's query results. |
| <a name="output_table_bucket_arns"></a> [table\_bucket\_arns](#output\_table\_bucket\_arns) | Table bucket ARN holding the tenant namespace, per zone: dedicated buckets, or the shared zone buckets. |
| <a name="output_tenant_tag_key"></a> [tenant\_tag\_key](#output\_tenant\_tag\_key) | The tenant-owned LF-tag key; its single value is member. |
| <a name="output_workgroup_name"></a> [workgroup\_name](#output\_workgroup\_name) | Name of the tenant Athena workgroup. |
<!-- END_TF_DOCS -->
