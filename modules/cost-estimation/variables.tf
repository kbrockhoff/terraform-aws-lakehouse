variable "enabled" {
  description = "Set to false to prevent the module from creating any resources"
  type        = bool
  default     = true
}

variable "region" {
  description = "AWS region"
  type        = string
}

variable "kms_key_count" {
  description = "Number of KMS keys to include in cost estimation"
  type        = number
  default     = 0

  validation {
    condition     = var.kms_key_count >= 0
    error_message = "KMS key count must be zero or positive."
  }
}

variable "cloudwatch_metric_count" {
  description = "Number of CloudWatch custom metrics to include in cost estimation"
  type        = number
  default     = 0

  validation {
    condition     = var.cloudwatch_metric_count >= 0
    error_message = "CloudWatch metric count must be zero or positive."
  }
}

variable "alarm_count" {
  description = "Number of CloudWatch alarms to include in cost estimation"
  type        = number
  default     = 0

  validation {
    condition     = var.alarm_count >= 0
    error_message = "CloudWatch alarm count must be zero or positive."
  }
}

variable "compliance_kms_key_count" {
  description = "Number of KMS keys that exist only because of a compliance designation (FR-024); reported separately as compliance_monthly_cost"
  type        = number
  default     = 0

  validation {
    condition     = var.compliance_kms_key_count >= 0
    error_message = "Compliance KMS key count must be zero or positive."
  }
}

variable "table_bucket_count" {
  description = "Number of S3 table buckets (no idle charge; listed for completeness)"
  type        = number
  default     = 0

  validation {
    condition     = var.table_bucket_count >= 0
    error_message = "Table bucket count must be zero or positive."
  }
}

variable "s3_bucket_count" {
  description = "Number of general purpose S3 buckets (no idle charge; listed for completeness)"
  type        = number
  default     = 0

  validation {
    condition     = var.s3_bucket_count >= 0
    error_message = "S3 bucket count must be zero or positive."
  }
}

variable "athena_workgroup_count" {
  description = "Number of Athena workgroups (no idle charge; listed for completeness)"
  type        = number
  default     = 0

  validation {
    condition     = var.athena_workgroup_count >= 0
    error_message = "Athena workgroup count must be zero or positive."
  }
}
