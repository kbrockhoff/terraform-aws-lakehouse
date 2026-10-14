# AWS Lakehouse Terraform Module

Terraform module that provisions a multi-tenant, Apache Iceberg-based lakehouse platform on AWS: layered data storage,
an open table catalog, tag-based governed data access, and per-tenant query engine configuration. It follows Brockhoff Cloud standards and
integrates with the `terraform-external-context` module for consistent naming and tagging.

Design principles for this module are recorded in [`.specify/memory/principles.md`](.specify/memory/principles.md).

## Features

- **Multi-Tenant Platform**: Each tenant is an S3 Tables namespace (Glue database) in every zone it uses, provisioned by the `modules/tenant` submodule, either from the platform's `tenants` map or standalone by a tenant provisioning pipeline using the published platform contract *(in progress)*
- **Layered Storage**: Raw, curated, and consumption (gold) data zones with automatic table maintenance and expiration *(in progress)*
- **Semantic Layer Ready**: Read-only grants on tenants' consumption namespaces let an external semantic layer serve published data products
- **Open Table Catalog**: Apache Iceberg tables registered in a central data catalog *(in progress)*
- **Governed Access**: Lake Formation LF-tag access control by per-tenant tag, sensitivity, compliance regulation, and data domain on databases, tables, and columns, with the most restrictive column labels rolled up to the table and delegated tenant stewards *(in progress)*
- **Query Engine Setup**: A query workgroup per tenant with encrypted, isolated results, per-query scan limits, and cost attribution *(in progress)*
- **Context Integration**: Leverages terraform-external-context for standardized naming and tagging
- **Environment-Aware**: Derives security, durability, monitoring, and cost defaults from `environment_type`
- **Encryption by Default**: Customer-managed KMS key created by default, or bring your own
- **Compliance-Ready**: Supports SOC 2, HIPAA, PCI DSS, and GDPR workloads; cost-incurring controls (dedicated keys and storage) activate only for tenants designated under a framework
- **Cost Visibility**: Monthly cost estimate output for module resources
- **Alarm Notifications**: Optional SNS topic for metric alarms, enabled by default in production environment types

## Usage

The module is not yet published to the Terraform Registry. Reference it by Git source pinned to a release tag.

### Basic Example

```hcl
module "lakehouse" {
  source = "git::https://github.com/kbrockhoff/terraform-aws-lakehouse.git?ref=vX.Y.Z"

  name_prefix = "dev-usw2"
}
```

### Complete Example

See [`examples/complete`](examples/complete) for a configuration that sets all available arguments.

## Environment Type Configuration

The `environment_type` variable provides a standardized way to configure resource defaults based on environment
characteristics. This follows cloud well-architected framework recommendations for different deployment stages.
Resiliency settings comply with the recovery point objective (RPO) and recovery time objective (RTO) values in
the table below. Cost optimization settings focus on shutting down resources during off-hours.

### Available Environment Types

| Type | Use Case | Configuration Focus | RPO | RTO |
|------|----------|---------------------|-----|-----|
| `None` | Custom configuration | No defaults applied, use individual config values | N/A | N/A |
| `Ephemeral` | Temporary environments | Cost-optimized, minimal durability requirements | N/A | 48h |
| `Development` | Developer workspaces | Balanced cost and functionality for active development | 24h | 48h |
| `Testing` | Automated testing | Consistent, repeatable configurations | 24h | 48h |
| `UAT` | User acceptance testing | Production-like settings with some cost optimization | 12h | 24h |
| `Production` | Live systems | High availability, durability, and performance | 1h  | 4h  |
| `MissionCritical` | Critical production | Maximum reliability, redundancy, and monitoring | 5m  | 1h  |

### Usage Examples

#### Development Environment

```hcl
module "dev_lakehouse" {
  source = "git::https://github.com/kbrockhoff/terraform-aws-lakehouse.git?ref=vX.Y.Z"
  
  name_prefix      = "dev-usw2"
  environment_type = "Development"
  
  tags = {
    Environment = "development"
    Team        = "platform"
  }
}
```

#### Production Environment

```hcl
module "prod_lakehouse" {
  source = "git::https://github.com/kbrockhoff/terraform-aws-lakehouse.git?ref=vX.Y.Z"
  
  name_prefix      = "prod-usw2"
  environment_type = "Production"
  
  tags = {
    Environment = "production"
    Team        = "platform"
    Backup      = "required"
  }
}
```

#### Custom Configuration (None)

```hcl
module "custom_lakehouse" {
  source = "git::https://github.com/kbrockhoff/terraform-aws-lakehouse.git?ref=vX.Y.Z"
  
  name_prefix      = "custom-usw2"
  environment_type = "None"
  
  # Specify all individual configuration values
  # when environment_type is "None"
}
```

## Network Tags Configuration

Resources deployed to subnets use lookup by `NetworkTags` values to determine which subnets to deploy to.
This eliminates the need to manage different subnet IDs variable values for each environment.

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
| <a name="module_catalog_integration"></a> [catalog\_integration](#module\_catalog\_integration) | ./modules/catalog-integration | n/a |
| <a name="module_pricing"></a> [pricing](#module\_pricing) | ./modules/cost-estimation | n/a |
| <a name="module_tenant"></a> [tenant](#module\_tenant) | ./modules/tenant | n/a |

## Resources

| Name | Type |
| ---- | ---- |
| [aws_kms_alias.main](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/kms_alias) | resource |
| [aws_kms_key.main](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/kms_key) | resource |
| [aws_lakeformation_lf_tag.domain](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lakeformation_lf_tag) | resource |
| [aws_lakeformation_lf_tag.regulation](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lakeformation_lf_tag) | resource |
| [aws_lakeformation_lf_tag.sensitivity](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lakeformation_lf_tag) | resource |
| [aws_s3_bucket.query_results](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket) | resource |
| [aws_s3_bucket_lifecycle_configuration.query_results](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket_lifecycle_configuration) | resource |
| [aws_s3_bucket_ownership_controls.query_results](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket_ownership_controls) | resource |
| [aws_s3_bucket_policy.query_results](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket_policy) | resource |
| [aws_s3_bucket_public_access_block.query_results](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket_public_access_block) | resource |
| [aws_s3_bucket_server_side_encryption_configuration.query_results](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket_server_side_encryption_configuration) | resource |
| [aws_s3tables_table_bucket.zone](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3tables_table_bucket) | resource |
| [aws_s3tables_table_bucket_policy.zone](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3tables_table_bucket_policy) | resource |
| [aws_sns_topic.alarms](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/sns_topic) | resource |
| [terraform_data.required_tags_guard](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |
| [terraform_data.secondary_admin_guard](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_name_prefix"></a> [name\_prefix](#input\_name\_prefix) | Organization unique prefix to use for resource names. Designed to be generated by terraform-external-context module. | `string` | n/a | yes |
| <a name="input_alarms_config"></a> [alarms\_config](#input\_alarms\_config) | Configuration object for metric alarms and notifications | <pre>object({<br/>    enabled                 = bool<br/>    create_sns_topic        = bool<br/>    sns_topic_arn           = string<br/>    scanned_bytes_threshold = optional(number)<br/>  })</pre> | <pre>{<br/>  "create_sns_topic": true,<br/>  "enabled": false,<br/>  "sns_topic_arn": ""<br/>}</pre> | no |
| <a name="input_catalog_integration_config"></a> [catalog\_integration\_config](#input\_catalog\_integration\_config) | Account-level Lake Formation integration for S3 Tables. Exactly one deployment per account and Region sets enabled = true; secondary deployments pass the owner's data access role ARN. | <pre>object({<br/>    enabled                               = bool<br/>    data_access_role_arn                  = string<br/>    allow_full_table_external_data_access = bool<br/>  })</pre> | <pre>{<br/>  "allow_full_table_external_data_access": false,<br/>  "data_access_role_arn": "",<br/>  "enabled": true<br/>}</pre> | no |
| <a name="input_cost_estimation_config"></a> [cost\_estimation\_config](#input\_cost\_estimation\_config) | Configuration object for monthly cost estimation | <pre>object({<br/>    enabled = bool<br/>  })</pre> | <pre>{<br/>  "enabled": true<br/>}</pre> | no |
| <a name="input_data_tags"></a> [data\_tags](#input\_data\_tags) | Additional tags to apply specifically to data storage resources beyond the common tags (usually generated by terraform-external-context). | `map(string)` | `{}` | no |
| <a name="input_enabled"></a> [enabled](#input\_enabled) | Set to false to prevent the module from creating any resources | `bool` | `true` | no |
| <a name="input_encryption_config"></a> [encryption\_config](#input\_encryption\_config) | Configuration object for encryption settings and KMS key management | <pre>object({<br/>    create_kms_key               = bool<br/>    kms_key_id                   = string<br/>    kms_key_deletion_window_days = number<br/>  })</pre> | <pre>{<br/>  "create_kms_key": true,<br/>  "kms_key_deletion_window_days": 14,<br/>  "kms_key_id": ""<br/>}</pre> | no |
| <a name="input_environment_type"></a> [environment\_type](#input\_environment\_type) | Environment type for resource configuration defaults. Select 'None' to use individual config values. | `string` | `"Development"` | no |
| <a name="input_governance_config"></a> [governance\_config](#input\_governance\_config) | Lake Formation governance: data-lake admins and the platform LF-tag vocabularies for sensitivity, regulations, and domains. | <pre>object({<br/>    admin_principal_arns          = list(string)<br/>    sensitivity_values            = optional(list(string), ["public", "internal", "confidential", "restricted"])<br/>    regulation_values             = optional(list(string), ["gdpr", "hipaa", "pci_dss"])<br/>    domain_values                 = optional(list(string), ["general"])<br/>    shared_storage_classification = optional(string)<br/>    writer_drop_enabled           = optional(bool)<br/>  })</pre> | <pre>{<br/>  "admin_principal_arns": []<br/>}</pre> | no |
| <a name="input_monitoring_config"></a> [monitoring\_config](#input\_monitoring\_config) | Configuration object for optional monitoring | <pre>object({<br/>    enabled = bool<br/>  })</pre> | <pre>{<br/>  "enabled": false<br/>}</pre> | no |
| <a name="input_query_config"></a> [query\_config](#input\_query\_config) | Platform query defaults that tenants may override. Null values use the environment default. | <pre>object({<br/>    bytes_scanned_cutoff_per_query = optional(number)<br/>    results_expiration_days        = optional(number)<br/>  })</pre> | `{}` | no |
| <a name="input_required_tags_config"></a> [required\_tags\_config](#input\_required\_tags\_config) | Tag keys the module requires or writes. When enforced (null uses the environment default), tags must include the cost-center and data-owner keys. | <pre>object({<br/>    enforced                = optional(bool)<br/>    cost_center_key         = optional(string, "CostCenter")<br/>    data_owner_key          = optional(string, "DataOwner")<br/>    data_classification_key = optional(string, "DataClassification")<br/>    tenant_key              = optional(string, "Tenant")<br/>  })</pre> | `{}` | no |
| <a name="input_storage_config"></a> [storage\_config](#input\_storage\_config) | Table storage settings. Null values use the environment default. | <pre>object({<br/>    table_storage_class = optional(string)<br/>  })</pre> | `{}` | no |
| <a name="input_tags"></a> [tags](#input\_tags) | Tags/labels to apply to all resources (usually generated by terraform-external-context). | `map(string)` | `{}` | no |
| <a name="input_tenants"></a> [tenants](#input\_tenants) | Tenants to provision with the platform, keyed by tenant name. Each entry is applied through modules/tenant. | <pre>map(object({<br/>    owner                 = string<br/>    sensitivity           = string<br/>    regulations           = optional(set(string), [])<br/>    domain                = optional(string)<br/>    zones                 = optional(list(string))<br/>    compliance_frameworks = optional(set(string), [])<br/>    stewards              = optional(list(string), [])<br/>    writers = optional(map(object({<br/>      principal_arn = string<br/>      zones         = optional(list(string))<br/>    })), {})<br/>    readers = optional(map(object({<br/>      principal_arn      = string<br/>      zones              = optional(list(string))<br/>      sensitivity_values = list(string)<br/>      regulations        = optional(set(string), [])<br/>      domains            = optional(list(string))<br/>    })), {})<br/>    row_filters = optional(map(object({<br/>      reader                = string<br/>      zone                  = string<br/>      table                 = string<br/>      row_filter_expression = string<br/>      column_names          = optional(list(string))<br/>    })), {})<br/>    bytes_scanned_cutoff_per_query = optional(number)<br/>    scanned_bytes_alarm_threshold  = optional(number)<br/>    dedicated_storage              = optional(bool, false)<br/>  }))</pre> | `{}` | no |
| <a name="input_zones_config"></a> [zones\_config](#input\_zones\_config) | Data zones shared by the platform. Each zone becomes an S3 table bucket named <name\_prefix>-<zone>. | <pre>object({<br/>    zones = list(string)<br/>  })</pre> | <pre>{<br/>  "zones": [<br/>    "raw",<br/>    "curated",<br/>    "consumption"<br/>  ]<br/>}</pre> | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_alarm_sns_topic_arn"></a> [alarm\_sns\_topic\_arn](#output\_alarm\_sns\_topic\_arn) | ARN of the SNS topic used for alarm notifications |
| <a name="output_alarm_sns_topic_name"></a> [alarm\_sns\_topic\_name](#output\_alarm\_sns\_topic\_name) | Name of the SNS topic used for alarm notifications |
| <a name="output_catalog_name"></a> [catalog\_name](#output\_catalog\_name) | Name of the federated S3 Tables Glue catalog. Empty when this deployment does not own the catalog integration or enabled = false. |
| <a name="output_cost_breakdown"></a> [cost\_breakdown](#output\_cost\_breakdown) | Breakdown of the platform's own monthly cost by service; tenant estimates are in the tenants output |
| <a name="output_data_access_role_arn"></a> [data\_access\_role\_arn](#output\_data\_access\_role\_arn) | Lake Formation data-access role for S3 Tables. Secondary deployments pass this as catalog\_integration\_config.data\_access\_role\_arn. |
| <a name="output_kms_alias_name"></a> [kms\_alias\_name](#output\_kms\_alias\_name) | Name of the KMS key alias |
| <a name="output_kms_key_arn"></a> [kms\_key\_arn](#output\_kms\_key\_arn) | ARN of the platform KMS key |
| <a name="output_kms_key_id"></a> [kms\_key\_id](#output\_kms\_key\_id) | ID of the KMS key used for encryption |
| <a name="output_lf_tag_vocabulary"></a> [lf\_tag\_vocabulary](#output\_lf\_tag\_vocabulary) | LF-tag keys and allowed values for pipeline labeling: sensitivity (ordered least to most sensitive), one yes/no key per regulation, domain, and the tenant key prefix. Null when enabled = false. |
| <a name="output_monthly_cost_estimate"></a> [monthly\_cost\_estimate](#output\_monthly\_cost\_estimate) | Estimated idle monthly cost in USD of the platform plus every platform-applied tenant. Standalone tenants report their own estimate. |
| <a name="output_platform_contract"></a> [platform\_contract](#output\_platform\_contract) | Versioned public interface consumed by modules/tenant, both platform-applied and standalone: identifiers and effective settings only. Adding a field is a minor change; removing or renaming one is a major change. Null when enabled = false. |
| <a name="output_query_results_bucket_arn"></a> [query\_results\_bucket\_arn](#output\_query\_results\_bucket\_arn) | ARN of the shared Athena query results bucket. |
| <a name="output_required_kms_key_policy_json"></a> [required\_kms\_key\_policy\_json](#output\_required\_kms\_key\_policy\_json) | Key policy statements the platform key needs (S3 Tables maintenance, Lake Formation data access, Athena results). Merge them into a consumer-supplied key's policy. Empty when enabled = false. |
| <a name="output_tenants"></a> [tenants](#output\_tenants) | Outputs of each platform-applied tenant, keyed by tenant name. |
| <a name="output_usage_rates"></a> [usage\_rates](#output\_usage\_rates) | Usage-based list prices in USD (S3 Tables storage and maintenance, Athena per TB scanned, S3 Standard storage) for estimating data-dependent costs |
| <a name="output_zone_table_bucket_arns"></a> [zone\_table\_bucket\_arns](#output\_zone\_table\_bucket\_arns) | ARN of the shared table bucket per zone. |
<!-- END_TF_DOCS -->

## License

This project is licensed under the Apache License, Version 2.0 - see the [LICENSE](LICENSE) file for details.
