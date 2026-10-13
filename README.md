# Terraform Module Template

> **📋 Template Setup Instructions**
>
> After cloning this template repository, complete these setup steps:
>
> 1. **Configure GitHub repository settings:**
>    - Under **Settings → Actions → General**
>      - Enable **Allow all actions and reusable workflows**
>      - Set **Workflow permissions** to "Read and write permissions"
>      - Enable **Workflow permissions** "Allow GitHub Actions to create and approve pull requests"
>
> 2. **Configure repository secrets:**
>    - Under **Settings → Environments**
>      - Create `development` environment
>      - Add `AWS_ROLE_ARN` secret for GitHub Actions AWS access to your AWS development account
>    - Under **Settings → Secrets and variables → Actions → Repository secrets**
>      - Add `RELEASE_PLEASE_TOKEN` secret (Personal Access Token with repo permissions)
>    - For AWS authentication, ensure your IAM role has cross-account trust with GitHub OIDC
>
> 3. **Configure main branch protection:**
>    - Under **Settings → Rules → Rulesets**
>      - Create new ruleset which applies to default branch and is set to Active
>      - Enable **Require a pull request before merging**
>      - Enable other rules as needed for your workflow
>
> 4. **Replace placeholder text:**
>    - Find and replace all instances of `replace-me` with your actual module name
>    - Find and replace all instances of `replace` with appropriate values
>    - Update `CLOUD` and `XXX` placeholders with your target cloud provider and resources
>
> 5. **Update module metadata:**
>    - Modify `locals.tf` → `ModuleName` to match your module using Terraform registry naming conventions
>    - Update repository URLs and documentation
>    - Customize examples and tests for your specific resources

This is a standardized multi-cloud Terraform module template that follows Brockhoff Cloud standards and integrates with the
`terraform-external-context` module for consistent naming and tagging across AWS, Azure, and GCP.

## Features

- Feature 1
- Feature 2
- Feature 3
- **Multi-Cloud Support**: Works across AWS, Azure, and GCP with consistent interfaces
- **Context Integration**: Leverages terraform-external-context for standardized naming and tagging
- **Environment-Aware**: Automatically adjusts configuration based on environment type
- **Security-First**: Built-in encryption, monitoring, and compliance features
- **Cost-Optimized**: Environment-specific instance sizing and cost optimization recommendations
- **High Availability**: Optional HA configuration for production workloads
- **AI-Friendly**: Includes metadata and structured outputs for AI agent consumption

## Usage

### Basic Example

```hcl
module "example" {
  source = "path/to/terraform-module"

  # ... other required arguments ...
}
```

### Complete Example

```hcl
module "example" {
  source = "path/to/terraform-module"

  # ... all available arguments ...
}
```

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
module "dev_resources" {
  source = "path/to/terraform-module"
  
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
module "prod_resources" {
  source = "path/to/terraform-module"
  
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
module "custom_resources" {
  source = "path/to/terraform-module"
  
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
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.11 |
| <a name="requirement_aws"></a> [aws](#requirement\_aws) | >= 5.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_aws"></a> [aws](#provider\_aws) | 6.6.0 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_pricing"></a> [pricing](#module\_pricing) | ./modules/cost-estimation | n/a |

## Resources

| Name | Type |
|------|------|
| [aws_kms_alias.main](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/kms_alias) | resource |
| [aws_kms_key.main](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/kms_key) | resource |
| [aws_sns_topic.alarms](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/sns_topic) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_name_prefix"></a> [name\_prefix](#input\_name\_prefix) | Organization unique prefix to use for resource names. Designed to be generated by terraform-external-context module. | `string` | n/a | yes |
| <a name="input_alarms_config"></a> [alarms\_config](#input\_alarms\_config) | Configuration object for metric alarms and notifications | <pre>object({<br/>    enabled          = bool<br/>    create_sns_topic = bool<br/>    sns_topic_arn    = string<br/>  })</pre> | <pre>{<br/>  "create_sns_topic": true,<br/>  "enabled": false,<br/>  "sns_topic_arn": ""<br/>}</pre> | no |
| <a name="input_cost_estimation_config"></a> [cost\_estimation\_config](#input\_cost\_estimation\_config) | Configuration object for monthly cost estimation | <pre>object({<br/>    enabled = bool<br/>  })</pre> | <pre>{<br/>  "enabled": true<br/>}</pre> | no |
| <a name="input_data_tags"></a> [data\_tags](#input\_data\_tags) | Additional tags to apply specifically to data storage resources beyond the common tags (usually generated by terraform-external-context). | `map(string)` | `{}` | no |
| <a name="input_enabled"></a> [enabled](#input\_enabled) | Set to false to prevent the module from creating any resources | `bool` | `true` | no |
| <a name="input_encryption_config"></a> [encryption\_config](#input\_encryption\_config) | Configuration object for encryption settings and KMS key management | <pre>object({<br/>    create_kms_key               = bool<br/>    kms_key_id                   = string<br/>    kms_key_deletion_window_days = number<br/>  })</pre> | <pre>{<br/>  "create_kms_key": true,<br/>  "kms_key_deletion_window_days": 14,<br/>  "kms_key_id": ""<br/>}</pre> | no |
| <a name="input_environment_type"></a> [environment\_type](#input\_environment\_type) | Environment type for resource configuration defaults. Select 'None' to use individual config values. | `string` | `"Development"` | no |
| <a name="input_monitoring_config"></a> [monitoring\_config](#input\_monitoring\_config) | Configuration object for optional monitoring | <pre>object({<br/>    enabled = bool<br/>  })</pre> | <pre>{<br/>  "enabled": false<br/>}</pre> | no |
| <a name="input_networktags_name"></a> [networktags\_name](#input\_networktags\_name) | Name of the network tags key used for subnet classification | `string` | `"NetworkTags"` | no |
| <a name="input_tags"></a> [tags](#input\_tags) | Tags/labels to apply to all resources (usually generated by terraform-external-context). | `map(string)` | `{}` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_alarm_sns_topic_arn"></a> [alarm\_sns\_topic\_arn](#output\_alarm\_sns\_topic\_arn) | ARN of the SNS topic used for alarm notifications |
| <a name="output_alarm_sns_topic_name"></a> [alarm\_sns\_topic\_name](#output\_alarm\_sns\_topic\_name) | Name of the SNS topic used for alarm notifications |
| <a name="output_cost_breakdown"></a> [cost\_breakdown](#output\_cost\_breakdown) | Detailed breakdown of monthly costs by service |
| <a name="output_kms_alias_name"></a> [kms\_alias\_name](#output\_kms\_alias\_name) | Name of the KMS key alias |
| <a name="output_kms_key_arn"></a> [kms\_key\_arn](#output\_kms\_key\_arn) | ARN of the KMS key used for encryption |
| <a name="output_kms_key_id"></a> [kms\_key\_id](#output\_kms\_key\_id) | ID of the KMS key used for encryption |
| <a name="output_monthly_cost_estimate"></a> [monthly\_cost\_estimate](#output\_monthly\_cost\_estimate) | Estimated monthly cost in USD for module resources |
<!-- END_TF_DOCS -->

## License

This project is licensed under the Apache License, Version 2.0 - see the [LICENSE](LICENSE) file for details.
