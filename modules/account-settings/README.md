# Account Settings Submodule

Manages the account- and Region-wide Lake Formation and Glue Data Catalog settings. Exactly one deployment per
account and Region calls it (`account_settings_config.owner` in the root module):

- **Data-lake settings**: authoritative admins and read-only admins, no `IAM_ALLOWED_PRINCIPALS` defaults,
  cross-account version (never lowered), `SET_CONTEXT`, and explicit external-engine settings. A precondition
  keeps the deploying role an admin.
- **Catalog encryption**: metadata and connection passwords encrypted with a dedicated catalog key (created here
  unless `catalog_encryption.kms_key_arn` is set), in `service_role` mode (a Glue role with an
  `aws:SourceAccount`-only trust, plus a 20-second propagation delay) or `per_caller` mode. The key policy denies
  deletion and disabling except to break-glass principals.
- **Catalog resource policy** (optional): the RAM `glue:ShareResource` statement plus caller-supplied Deny statements,
  with `EnableHybrid = TRUE`.
- **Federated `s3tablescatalog`** (`catalog.tf`): an apply-time step runs `scripts/ensure-s3tables-catalog.sh`, which
  creates the catalog only when it is missing, never updates or deletes it, and refuses to act in another account.
  It needs AWS CLI v2 and bash where Terraform runs, and uses the environment's credentials (see the root README,
  "S3 Tables Catalog").

Destroying this submodule resets catalog encryption to `DISABLED` and empties the admins, but never deletes
`s3tablescatalog`. To hand over ownership, use a
`removed` block with `destroy = false` (see the root README, "Handing over ownership").

<!-- BEGIN_TF_DOCS -->
## Requirements

| Name | Version |
| ---- | ------- |
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.11 |
| <a name="requirement_aws"></a> [aws](#requirement\_aws) | >= 6.59.0 |
| <a name="requirement_time"></a> [time](#requirement\_time) | >= 0.12 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_aws"></a> [aws](#provider\_aws) | >= 6.59.0 |
| <a name="provider_terraform"></a> [terraform](#provider\_terraform) | n/a |
| <a name="provider_time"></a> [time](#provider\_time) | >= 0.12 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [aws_glue_data_catalog_encryption_settings.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/glue_data_catalog_encryption_settings) | resource |
| [aws_glue_resource_policy.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/glue_resource_policy) | resource |
| [aws_iam_role.catalog_encryption](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/iam_role) | resource |
| [aws_iam_role_policy.catalog_encryption](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/iam_role_policy) | resource |
| [aws_kms_alias.catalog](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/kms_alias) | resource |
| [aws_kms_key.catalog](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/kms_key) | resource |
| [aws_lakeformation_data_lake_settings.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lakeformation_data_lake_settings) | resource |
| [terraform_data.s3tables_catalog](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |
| [time_sleep.catalog_encryption_role](https://registry.terraform.io/providers/hashicorp/time/latest/docs/resources/sleep) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_account_id"></a> [account\_id](#input\_account\_id) | AWS account ID whose Lake Formation and Glue Data Catalog settings this submodule owns. | `string` | n/a | yes |
| <a name="input_admin_principal_arns"></a> [admin\_principal\_arns](#input\_admin\_principal\_arns) | Lake Formation data-lake admins, including studio integration principals. Authoritative: admins not listed are removed. | `list(string)` | n/a | yes |
| <a name="input_catalog_encryption"></a> [catalog\_encryption](#input\_catalog\_encryption) | Catalog metadata encryption. An empty kms\_key\_arn creates the dedicated catalog key. | <pre>object({<br/>    key_access_mode             = string<br/>    kms_key_arn                 = string<br/>    additional_key_user_arns    = list(string)<br/>    key_deletion_principal_arns = list(string)<br/>  })</pre> | n/a | yes |
| <a name="input_catalog_resource_policy"></a> [catalog\_resource\_policy](#input\_catalog\_resource\_policy) | Optional Glue Data Catalog resource policy: the RAM sharing statement plus caller-supplied Deny statements. | <pre>object({<br/>    enabled           = bool<br/>    deny_policy_jsons = list(string)<br/>  })</pre> | n/a | yes |
| <a name="input_data_lake"></a> [data\_lake](#input\_data\_lake) | Explicit values for every data-lake settings field. PutDataLakeSettings replaces the whole object, so nothing is left implicit. | <pre>object({<br/>    allow_external_data_filtering         = bool<br/>    external_data_filtering_allow_list    = list(string)<br/>    authorized_session_tag_values         = list(string)<br/>    allow_full_table_external_data_access = bool<br/>    trusted_resource_owners               = list(string)<br/>  })</pre> | n/a | yes |
| <a name="input_deployer_role_arn"></a> [deployer\_role\_arn](#input\_deployer\_role\_arn) | Role ARN of the deploying session. It must be one of admin\_principal\_arns. | `string` | n/a | yes |
| <a name="input_environment_type"></a> [environment\_type](#input\_environment\_type) | Environment type of the owning deployment. Only Ephemeral may change the catalog key after installation. | `string` | n/a | yes |
| <a name="input_kms_key_deletion_window_days"></a> [kms\_key\_deletion\_window\_days](#input\_kms\_key\_deletion\_window\_days) | Deletion window for the created catalog key. Deletion itself is denied to everyone but key\_deletion\_principal\_arns. | `number` | n/a | yes |
| <a name="input_name_prefix"></a> [name\_prefix](#input\_name\_prefix) | Organization unique prefix used for resource names. | `string` | n/a | yes |
| <a name="input_partition"></a> [partition](#input\_partition) | AWS partition (aws, aws-us-gov, aws-cn). | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | AWS Region of the settings. | `string` | n/a | yes |
| <a name="input_cross_account_version"></a> [cross\_account\_version](#input\_cross\_account\_version) | Lake Formation cross-account version (4 or 5). Versions are never lowered; 5 is irreversible. | `number` | `4` | no |
| <a name="input_key_user_arns"></a> [key\_user\_arns](#input\_key\_user\_arns) | Tenant persona ARNs granted catalog key use in per\_caller mode. | `list(string)` | `[]` | no |
| <a name="input_permissions_boundary_arn"></a> [permissions\_boundary\_arn](#input\_permissions\_boundary\_arn) | Permissions boundary for the catalog-encryption role. Empty means none. | `string` | `""` | no |
| <a name="input_read_only_admin_principal_arns"></a> [read\_only\_admin\_principal\_arns](#input\_read\_only\_admin\_principal\_arns) | Lake Formation read-only admins. Authoritative, like admin\_principal\_arns. | `list(string)` | `[]` | no |
| <a name="input_s3tables_catalog"></a> [s3tables\_catalog](#input\_s3tables\_catalog) | Settings used only when s3tablescatalog is missing and gets created (FR-029); an existing catalog is never changed. allow\_full\_table\_external\_data\_access lets third-party engines get credentials without session tags when the caller has full table access. | <pre>object({<br/>    allow_full_table_external_data_access = bool<br/>  })</pre> | <pre>{<br/>  "allow_full_table_external_data_access": true<br/>}</pre> | no |
| <a name="input_storage_key_arns"></a> [storage\_key\_arns](#input\_storage\_key\_arns) | Keys that encrypt table storage or query results. A supplied catalog key must not be one of them. | `list(string)` | `[]` | no |
| <a name="input_tags"></a> [tags](#input\_tags) | Tags to apply to all resources. Common tags only: nothing here holds data at rest. | `map(string)` | `{}` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_catalog_encryption_mode"></a> [catalog\_encryption\_mode](#output\_catalog\_encryption\_mode) | Glue Data Catalog encryption mode: SSE-KMS-WITH-SERVICE-ROLE or SSE-KMS. |
| <a name="output_catalog_encryption_role_arn"></a> [catalog\_encryption\_role\_arn](#output\_catalog\_encryption\_role\_arn) | ARN of the Glue catalog-encryption role. Empty in per\_caller mode. |
| <a name="output_catalog_kms_key_arn"></a> [catalog\_kms\_key\_arn](#output\_catalog\_kms\_key\_arn) | ARN of the dedicated catalog-metadata key, created here or supplied. |
| <a name="output_cross_account_version"></a> [cross\_account\_version](#output\_cross\_account\_version) | Lake Formation cross-account version managed here. |
| <a name="output_data_lake_settings_id"></a> [data\_lake\_settings\_id](#output\_data\_lake\_settings\_id) | ID of the data-lake settings. Registration and grants wait on it so the deployer's admin rights exist first. |
| <a name="output_required_catalog_kms_key_policy_json"></a> [required\_catalog\_kms\_key\_policy\_json](#output\_required\_catalog\_kms\_key\_policy\_json) | Key policy statements a consumer-supplied catalog key needs. Account-root access is the key owner's own responsibility. |
| <a name="output_s3tables_catalog_ready"></a> [s3tables\_catalog\_ready](#output\_s3tables\_catalog\_ready) | ID of the s3tablescatalog check-and-create step. Grants and LF-tags on the catalog wait on it. |
<!-- END_TF_DOCS -->