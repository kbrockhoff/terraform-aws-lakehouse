# This submodule deliberately takes no KMS key ARNs: key policies reference the data-access
# role it creates, so taking keys here would create a dependency cycle.

variable "name_prefix" {
  description = "Organization unique prefix used for resource names."
  type        = string

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{0,22}[a-z0-9]$", var.name_prefix))
    error_message = "name_prefix must be 2 to 24 lowercase alphanumeric or hyphen characters, starting with a letter and ending with an alphanumeric character."
  }
}

variable "account_id" {
  description = "AWS account ID that owns the S3 table buckets."
  type        = string

  validation {
    condition     = can(regex("^[0-9]{12}$", var.account_id))
    error_message = "account_id must be a 12-digit AWS account ID."
  }
}

variable "partition" {
  description = "AWS partition (aws, aws-us-gov, aws-cn)."
  type        = string

  validation {
    condition     = can(regex("^aws[a-z-]*$", var.partition))
    error_message = "partition must be an AWS partition name."
  }
}

variable "region" {
  description = "AWS Region of the S3 Tables catalog integration."
  type        = string

  validation {
    condition     = can(regex("^[a-z]{2}(-[a-z]+)+-[0-9]$", var.region))
    error_message = "region must be an AWS Region name."
  }
}

variable "tags" {
  description = "Tags to apply to all resources."
  type        = map(string)
  default     = {}
}

variable "admin_principal_arns" {
  description = "Lake Formation data-lake admins. Authoritative: every secondary deployment's deploying role and every tenant pipeline role must be listed."
  type        = list(string)

  validation {
    condition     = length(var.admin_principal_arns) > 0
    error_message = "admin_principal_arns must not be empty."
  }

  validation {
    condition     = alltrue([for a in var.admin_principal_arns : can(regex("^arn:aws[a-z-]*:iam::[0-9]{12}:(role|user)/.+$", a))])
    error_message = "Every admin_principal_arns entry must be an IAM role or user ARN."
  }
}

variable "deployer_role_arn" {
  description = "Role ARN of the deploying session. It must be one of admin_principal_arns."
  type        = string

  validation {
    condition     = can(regex("^arn:aws[a-z-]*:iam::[0-9]{12}:(role|user)/.+$", var.deployer_role_arn))
    error_message = "deployer_role_arn must be an IAM role or user ARN."
  }
}

variable "allow_full_table_external_data_access" {
  description = "Allow third-party engines to access full tables in the federated s3tablescatalog."
  type        = bool
  default     = false
}
