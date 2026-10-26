# Cost Estimation Terraform Module

Estimates the monthly cost of resources provisioned by the parent module.

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

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_region"></a> [region](#input\_region) | AWS region | `string` | n/a | yes |
| <a name="input_alarm_count"></a> [alarm\_count](#input\_alarm\_count) | Number of CloudWatch alarms to include in cost estimation | `number` | `0` | no |
| <a name="input_athena_workgroup_count"></a> [athena\_workgroup\_count](#input\_athena\_workgroup\_count) | Number of Athena workgroups (no idle charge; listed for completeness) | `number` | `0` | no |
| <a name="input_cloudwatch_metric_count"></a> [cloudwatch\_metric\_count](#input\_cloudwatch\_metric\_count) | Number of CloudWatch custom metrics to include in cost estimation | `number` | `0` | no |
| <a name="input_compliance_kms_key_count"></a> [compliance\_kms\_key\_count](#input\_compliance\_kms\_key\_count) | Number of KMS keys that exist only because of a compliance designation (FR-024); reported separately as compliance\_monthly\_cost | `number` | `0` | no |
| <a name="input_enabled"></a> [enabled](#input\_enabled) | Set to false to prevent the module from creating any resources | `bool` | `true` | no |
| <a name="input_kms_key_count"></a> [kms\_key\_count](#input\_kms\_key\_count) | Number of KMS keys to include in cost estimation | `number` | `0` | no |
| <a name="input_s3_bucket_count"></a> [s3\_bucket\_count](#input\_s3\_bucket\_count) | Number of general purpose S3 buckets (no idle charge; listed for completeness) | `number` | `0` | no |
| <a name="input_table_bucket_count"></a> [table\_bucket\_count](#input\_table\_bucket\_count) | Number of S3 table buckets (no idle charge; listed for completeness) | `number` | `0` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_compliance_monthly_cost"></a> [compliance\_monthly\_cost](#output\_compliance\_monthly\_cost) | Portion of monthly\_cost\_estimate caused by compliance designations, in USD |
| <a name="output_cost_breakdown"></a> [cost\_breakdown](#output\_cost\_breakdown) | Detailed breakdown of monthly costs by service |
| <a name="output_monthly_cost_estimate"></a> [monthly\_cost\_estimate](#output\_monthly\_cost\_estimate) | Idle fixed monthly cost estimate in USD, including compliance costs |
| <a name="output_usage_rates"></a> [usage\_rates](#output\_usage\_rates) | Usage-based list prices in USD for estimating data-dependent costs: S3 Tables storage per GB-month (Standard and Intelligent-Tiering tiers), monitoring and maintenance per object, Athena per TB scanned, and S3 Standard per GB-month |
<!-- END_TF_DOCS -->    