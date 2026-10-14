# Deployer IAM Terraform Module

Creates a least-privilege IAM policy for deploying the lakehouse with Terraform and attaches it
to an existing role (`iam_role_arn`). It creates no roles, and the root module never calls it;
call it from your bootstrap or CI configuration.

## Scopes

- `scope = "platform"` (default): the root module, including the account-level catalog
  integration (federated `s3tablescatalog`, Lake Formation registration and data-lake settings,
  and the Lake Formation data-access role).
- `scope = "tenant"`: a standalone tenant provisioning pipeline running `modules/tenant`. It omits
  the account-level permissions and limits results buckets to `<name_prefix>-*-athena-results-*`.

Every action is listed explicitly. Resources are limited to `<name_prefix>` names wherever the
service supports resource-level permissions; each `"*"` resource is commented with why.

## Lake Formation admin membership is separate

IAM permissions are not enough to grant Lake Formation permissions or create LF-tags. The role
must also be a Lake Formation data-lake admin: list it in the platform's
`governance_config.admin_principal_arns` (FR-013). This module does not do that.

<!-- BEGIN_TF_DOCS -->
## Requirements

| Name | Version |
| ---- | ------- |
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.11 |
| <a name="requirement_aws"></a> [aws](#requirement\_aws) | >= 6.59.0, < 7.0.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_aws"></a> [aws](#provider\_aws) | 6.69.0 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [aws_iam_policy.basic](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/iam_policy) | resource |
| [aws_iam_role_policy_attachment.basic](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/iam_role_policy_attachment) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_account_id"></a> [account\_id](#input\_account\_id) | AWS account ID | `string` | n/a | yes |
| <a name="input_iam_role_arn"></a> [iam\_role\_arn](#input\_iam\_role\_arn) | ARN of the IAM role to attach the policy to | `string` | n/a | yes |
| <a name="input_name_prefix"></a> [name\_prefix](#input\_name\_prefix) | Organization unique prefix to use for resource names. Recommend including environment and region. e.g. 'prod-usw2'. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | AWS region | `string` | n/a | yes |
| <a name="input_cost_estimation_enabled"></a> [cost\_estimation\_enabled](#input\_cost\_estimation\_enabled) | Allow the AWS Price List API calls used by cost estimation. | `bool` | `true` | no |
| <a name="input_dns_suffix"></a> [dns\_suffix](#input\_dns\_suffix) | AWS services DNS suffix | `string` | `"amazonaws.com"` | no |
| <a name="input_enabled"></a> [enabled](#input\_enabled) | Set to false to prevent the module from creating any resources. | `bool` | `true` | no |
| <a name="input_partition"></a> [partition](#input\_partition) | AWS partition | `string` | `"aws"` | no |
| <a name="input_scope"></a> [scope](#input\_scope) | Which deployment the policy is for: "platform" (the root module, including the account-level catalog integration) or "tenant" (a standalone tenant provisioning pipeline). | `string` | `"platform"` | no |
| <a name="input_tags"></a> [tags](#input\_tags) | Tags/labels to apply to all resources. | `map(string)` | `{}` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_policy_arn"></a> [policy\_arn](#output\_policy\_arn) | ARN of the IAM policy created for deployment operations |
| <a name="output_policy_name"></a> [policy\_name](#output\_policy\_name) | Name of the IAM policy created for deployment operations |
<!-- END_TF_DOCS -->    