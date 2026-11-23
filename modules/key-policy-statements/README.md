# Key Policy Statements Submodule

Internal submodule. Renders the key policy statements that every module-created KMS key shares (the platform
key, the catalog-metadata key, and designated-tenant keys), so the three policies stay identical:

- **KeyAdministrators**: key management without key use (no `Encrypt`, `Decrypt`, `GenerateDataKey`, or
  `CreateGrant`) for the named administrators, limited to the key's account. `kms:PutKeyPolicy` lets an
  administrator rewrite the policy, including granting itself key use, so administrators are trusted policy
  editors.
- **AccountReadOnly**: the account root may only `Describe*`, `Get*`, and `List*`. IAM policies alone can neither
  administer nor use the keys.
- **KeyDeletionBreakGlass** (optional): `ScheduleKeyDeletion`, `CancelKeyDeletion`, `DisableKey`, and `EnableKey`
  for break-glass principals of a protected key.
- **AllowDirectAccessExemptions** (separate output): `Decrypt` and `GenerateDataKey` through S3 Tables and S3, on
  the given table storage encryption contexts only, for principals exempt from the table-bucket direct-access deny.

Break-glass and exempt principals are delegated to the account with an exact `aws:PrincipalArn` condition rather
than named as principals. KMS rejects a policy that names a role that no longer exists, so a retired role would
otherwise block every later policy update. They need the same actions in their IAM policies, and must be in the
key's account. Administrators are named directly, so a deleted administrator must be removed from the list first.

Callers merge the JSON outputs into their key policy with `source_policy_documents`.

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

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_account_id"></a> [account\_id](#input\_account\_id) | Account that owns the key. Administrators and the read-only root statement are limited to it. | `string` | n/a | yes |
| <a name="input_partition"></a> [partition](#input\_partition) | AWS partition. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Region of the key, for the kms:ViaService conditions. | `string` | n/a | yes |
| <a name="input_direct_access_exempt_principal_arns"></a> [direct\_access\_exempt\_principal\_arns](#input\_direct\_access\_exempt\_principal\_arns) | Same-account principals exempt from the table-bucket direct-access deny, granted key use through S3 Tables and S3 on table\_storage\_arn\_patterns only. Empty adds no statement. | `list(string)` | `[]` | no |
| <a name="input_key_administrator_arns"></a> [key\_administrator\_arns](#input\_key\_administrator\_arns) | Named key administrators: key management without key use. administration\_json is only valid with at least one; callers check that, and that the deploying role is listed, before creating a key. | `list(string)` | `[]` | no |
| <a name="input_key_deletion_principal_arns"></a> [key\_deletion\_principal\_arns](#input\_key\_deletion\_principal\_arns) | Break-glass principals that may schedule deletion of, or disable, a protected key. Empty adds no statement. | `list(string)` | `[]` | no |
| <a name="input_table_storage_arn_patterns"></a> [table\_storage\_arn\_patterns](#input\_table\_storage\_arn\_patterns) | S3 Tables encryption contexts (kms:EncryptionContext:aws:s3:arn patterns, such as arn:aws:s3tables:<region>:<account>:bucket/<name>/*) the direct-access exemption covers. Required with direct\_access\_exempt\_principal\_arns. | `list(string)` | `[]` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_administration_json"></a> [administration\_json](#output\_administration\_json) | KeyAdministrators, AccountReadOnly, and, with break-glass principals, KeyDeletionBreakGlass. |
| <a name="output_direct_access_json"></a> [direct\_access\_json](#output\_direct\_access\_json) | AllowDirectAccessExemptions, or empty when no principal is exempt. Pass through compact() into source\_policy\_documents. |
<!-- END_TF_DOCS -->
