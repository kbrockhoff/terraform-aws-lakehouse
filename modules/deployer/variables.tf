variable "enabled" {
  description = "Set to false to prevent the module from creating any resources."
  type        = bool
  default     = true
}

variable "name_prefix" {
  description = "Organization unique prefix to use for resource names. Recommend including environment and region. e.g. 'prod-usw2'."
  type        = string

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{0,22}[a-z0-9]$", var.name_prefix))
    error_message = "The name_prefix value must be between 2 and 24 characters."
  }
}

variable "iam_role_arn" {
  description = "ARN of the IAM role to attach the policy to"
  type        = string
}

variable "tags" {
  description = "Tags/labels to apply to all resources."
  type        = map(string)
  default     = {}
}

variable "partition" {
  description = "AWS partition"
  type        = string
  default     = "aws"
}

variable "dns_suffix" {
  description = "AWS services DNS suffix"
  type        = string
  default     = "amazonaws.com"
}

variable "region" {
  description = "AWS region"
  type        = string
}

variable "account_id" {
  description = "AWS account ID"
  type        = string
}

variable "scope" {
  description = "Which deployment the policy is for: \"platform\" (the root module, including the account-level catalog integration) or \"tenant\" (a standalone tenant provisioning pipeline)."
  type        = string
  default     = "platform"

  validation {
    condition     = contains(["platform", "tenant"], var.scope)
    error_message = "scope must be \"platform\" or \"tenant\"."
  }
}

variable "cost_estimation_enabled" {
  description = "Allow the AWS Price List API calls used by cost estimation."
  type        = bool
  default     = true
}

variable "platform_contract_parameter_arn" {
  description = "ARN of the SSM parameter holding the platform contract, when tenant pipelines read it from SSM. Adds ssm:GetParameter on exactly that parameter. Empty adds nothing."
  type        = string
  default     = ""

  validation {
    condition     = var.platform_contract_parameter_arn == "" || can(regex("^arn:aws[a-z-]*:ssm:[a-z0-9-]+:[0-9]{12}:parameter/.+$", var.platform_contract_parameter_arn))
    error_message = "platform_contract_parameter_arn must be empty or an SSM parameter ARN."
  }
}
