# Catalog Integration Submodule

Integrates S3 Tables with AWS Lake Formation for one account and Region:

- the Lake Formation data-access role (`<name_prefix>-lf-s3tables-access`);
- the registration of every table bucket (`bucket/*`) with Lake Formation, with federation;
- the federated `s3tablescatalog` Glue catalog.

The data-lake settings moved to [`modules/account-settings`](../account-settings) in spec 003. The root module
passes `settings_ready` so the registration waits until the deployer's admin rights exist.

## One owner per account and Region

These resources are account-level singletons. Exactly one deployment per account and Region uses
this submodule (`catalog_integration_config.enabled = true` in the root module). Every other
deployment sets `enabled = false` and passes this submodule's `data_access_role_arn`.

## Importing an existing catalog

If `s3tablescatalog` was created in the console, import it before the first apply, or the create
fails because the catalog already exists. Use an `import` block with the ID format from the AWS
provider's `aws_glue_catalog` import documentation:

```hcl
import {
  to = module.main.module.catalog_integration[0].aws_glue_catalog.s3tables
  id = "<see aws_glue_catalog import docs>"
}
```

Import an existing registration (`aws_lakeformation_resource.s3tables`) the same way.

<!-- BEGIN_TF_DOCS -->
## Requirements

| Name | Version |
| ---- | ------- |
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.11 |
| <a name="requirement_aws"></a> [aws](#requirement\_aws) | >= 6.59.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_aws"></a> [aws](#provider\_aws) | >= 6.59.0 |
| <a name="provider_terraform"></a> [terraform](#provider\_terraform) | n/a |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [aws_iam_role.lf_s3tables_access](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/iam_role) | resource |
| [aws_iam_role_policy.lf_s3tables_access](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/iam_role_policy) | resource |
| [aws_lakeformation_resource.s3tables](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lakeformation_resource) | resource |
| [terraform_data.settings_ready](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_account_id"></a> [account\_id](#input\_account\_id) | AWS account ID that owns the S3 table buckets. | `string` | n/a | yes |
| <a name="input_name_prefix"></a> [name\_prefix](#input\_name\_prefix) | Organization unique prefix used for resource names. | `string` | n/a | yes |
| <a name="input_partition"></a> [partition](#input\_partition) | AWS partition (aws, aws-us-gov, aws-cn). | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | AWS Region of the S3 Tables catalog integration. | `string` | n/a | yes |
| <a name="input_settings_ready"></a> [settings\_ready](#input\_settings\_ready) | ID of the data-lake settings or of the settings guard. Orders registration after the deployer's admin rights exist. | `string` | n/a | yes |
| <a name="input_permissions_boundary_arn"></a> [permissions\_boundary\_arn](#input\_permissions\_boundary\_arn) | Permissions boundary for the Lake Formation data-access role. Empty means none. | `string` | `""` | no |
| <a name="input_tags"></a> [tags](#input\_tags) | Tags to apply to all resources. | `map(string)` | `{}` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_data_access_role_arn"></a> [data\_access\_role\_arn](#output\_data\_access\_role\_arn) | ARN of the Lake Formation data-access role. Table bucket KMS key policies must grant it use of the key. |
<!-- END_TF_DOCS -->
