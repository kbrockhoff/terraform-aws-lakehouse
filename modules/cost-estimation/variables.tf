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

variable "cloudwatch_alarm_count" {
  description = "Number of CloudWatch alarms to include in cost estimation"
  type        = number
  default     = 0

  validation {
    condition     = var.cloudwatch_alarm_count >= 0
    error_message = "CloudWatch alarm count must be zero or positive."
  }
}
