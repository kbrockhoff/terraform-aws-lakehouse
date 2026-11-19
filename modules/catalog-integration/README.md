# Catalog Integration Submodule

Integrates S3 Tables with AWS Lake Formation for one account and Region:

- the Lake Formation data-access role (`<name_prefix>-lf-s3tables-access`);
- the registration of every table bucket (`bucket/*`) with Lake Formation, with federation.

The data-lake settings and the federated `s3tablescatalog` moved to [`modules/account-settings`](../account-settings)
in spec 003. The root module passes `settings_ready` so the registration waits until the deployer's admin rights
exist.

## One owner per account and Region

These resources are account-level singletons. Exactly one deployment per account and Region uses
this submodule (`catalog_integration_config.enabled = true` in the root module). Every other
deployment sets `enabled = false` and passes this submodule's `data_access_role_arn`.

## The catalog moved out

Versions before spec 003 created `aws_glue_catalog.s3tables` here. A `removed` block now forgets it without deleting
it, so the upgrade leaves the catalog in place. `modules/account-settings` creates the catalog only when it is
missing and never deletes it.

## Importing an existing registration

If the `bucket/*` registration already exists, import `aws_lakeformation_resource.s3tables` before the first apply,
or the create fails:

```hcl
import {
  to = module.main.module.catalog_integration[0].aws_lakeformation_resource.s3tables
  id = "arn:aws:s3tables:<region>:<account>:bucket/*"
}
```

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
| [terraform_data.catalog_ready](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |
| [terraform_data.settings_ready](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_account_id"></a> [account\_id](#input\_account\_id) | AWS account ID that owns the S3 table buckets. | `string` | n/a | yes |
| <a name="input_name_prefix"></a> [name\_prefix](#input\_name\_prefix) | Organization unique prefix used for resource names. | `string` | n/a | yes |
| <a name="input_partition"></a> [partition](#input\_partition) | AWS partition (aws, aws-us-gov, aws-cn). | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | AWS Region of the S3 Tables catalog integration. | `string` | n/a | yes |
| <a name="input_settings_ready"></a> [settings\_ready](#input\_settings\_ready) | ID of the data-lake settings or of the settings guard. Orders registration after the deployer's admin rights exist. | `string` | n/a | yes |
| <a name="input_catalog_ready"></a> [catalog\_ready](#input\_catalog\_ready) | ID of the root module's s3tablescatalog check-and-create step, or empty when another deployment owns the account settings. Every resource here waits on it (spec 003 FR-029). | `string` | `""` | no |
| <a name="input_permissions_boundary_arn"></a> [permissions\_boundary\_arn](#input\_permissions\_boundary\_arn) | Permissions boundary for the Lake Formation data-access role. Empty means none. | `string` | `""` | no |
| <a name="input_tags"></a> [tags](#input\_tags) | Tags to apply to all resources. | `map(string)` | `{}` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_data_access_role_arn"></a> [data\_access\_role\_arn](#output\_data\_access\_role\_arn) | ARN of the Lake Formation data-access role. Table bucket KMS key policies must grant it use of the key. |
<!-- END_TF_DOCS -->
