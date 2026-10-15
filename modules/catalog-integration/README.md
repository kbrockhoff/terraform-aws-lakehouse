# Catalog Integration Submodule

Integrates S3 Tables with AWS Lake Formation for one account and Region:

- the Lake Formation data-access role (`<name_prefix>-lf-s3tables-access`);
- the registration of every table bucket (`bucket/*`) with Lake Formation, with federation;
- the federated `s3tablescatalog` Glue catalog;
- the data-lake settings: admins, and empty default permissions.

## One owner per account and Region

These resources are account-level singletons. Exactly one deployment per account and Region uses
this submodule (`catalog_integration_config.enabled = true` in the root module). Every other
deployment sets `enabled = false` and passes this submodule's `data_access_role_arn`.

## Data-lake settings are authoritative

`aws_lakeformation_data_lake_settings` replaces **all** data-lake settings in the account and
Region. Any admin not in `admin_principal_arns` is removed on apply, including admins added in the
console. List in `admin_principal_arns`:

- the role that deploys this module (a plan-time precondition enforces this);
- the deploying role of every secondary deployment;
- every tenant provisioning pipeline role.

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
| [aws_glue_catalog.s3tables](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/glue_catalog) | resource |
| [aws_iam_role.lf_s3tables_access](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/iam_role) | resource |
| [aws_iam_role_policy.lf_s3tables_access](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/iam_role_policy) | resource |
| [aws_lakeformation_data_lake_settings.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lakeformation_data_lake_settings) | resource |
| [aws_lakeformation_resource.s3tables](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lakeformation_resource) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_account_id"></a> [account\_id](#input\_account\_id) | AWS account ID that owns the S3 table buckets. | `string` | n/a | yes |
| <a name="input_admin_principal_arns"></a> [admin\_principal\_arns](#input\_admin\_principal\_arns) | Lake Formation data-lake admins. Authoritative: every secondary deployment's deploying role and every tenant pipeline role must be listed. | `list(string)` | n/a | yes |
| <a name="input_deployer_role_arn"></a> [deployer\_role\_arn](#input\_deployer\_role\_arn) | Role ARN of the deploying session. It must be one of admin\_principal\_arns. | `string` | n/a | yes |
| <a name="input_name_prefix"></a> [name\_prefix](#input\_name\_prefix) | Organization unique prefix used for resource names. | `string` | n/a | yes |
| <a name="input_partition"></a> [partition](#input\_partition) | AWS partition (aws, aws-us-gov, aws-cn). | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | AWS Region of the S3 Tables catalog integration. | `string` | n/a | yes |
| <a name="input_allow_full_table_external_data_access"></a> [allow\_full\_table\_external\_data\_access](#input\_allow\_full\_table\_external\_data\_access) | Allow third-party engines to access full tables in the federated s3tablescatalog. | `bool` | `false` | no |
| <a name="input_tags"></a> [tags](#input\_tags) | Tags to apply to all resources. | `map(string)` | `{}` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_catalog_arn"></a> [catalog\_arn](#output\_catalog\_arn) | ARN of the federated S3 Tables Glue catalog. |
| <a name="output_catalog_name"></a> [catalog\_name](#output\_catalog\_name) | Name of the federated S3 Tables Glue catalog. |
| <a name="output_data_access_role_arn"></a> [data\_access\_role\_arn](#output\_data\_access\_role\_arn) | ARN of the Lake Formation data-access role. Table bucket KMS key policies must grant it use of the key. |
<!-- END_TF_DOCS -->
