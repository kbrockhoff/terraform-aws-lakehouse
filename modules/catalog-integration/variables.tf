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

variable "settings_ready" {
  description = "ID of the data-lake settings or of the settings guard. Orders registration after the deployer's admin rights exist."
  type        = string
}

variable "permissions_boundary_arn" {
  description = "Permissions boundary for the Lake Formation data-access role. Empty means none."
  type        = string
  default     = ""

  validation {
    condition     = var.permissions_boundary_arn == "" || can(regex("^arn:aws[a-z-]*:iam::([0-9]{12}|aws):policy/.+$", var.permissions_boundary_arn))
    error_message = "permissions_boundary_arn must be empty or an IAM policy ARN."
  }
}
