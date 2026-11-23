variable "account_id" {
  description = "Account that owns the key. Administrators and the read-only root statement are limited to it."
  type        = string
}

variable "partition" {
  description = "AWS partition."
  type        = string
}

variable "region" {
  description = "Region of the key, for the kms:ViaService conditions."
  type        = string
}

variable "key_administrator_arns" {
  description = "Named key administrators: key management without key use. administration_json is only valid with at least one; callers check that, and that the deploying role is listed, before creating a key."
  type        = list(string)
  default     = []
}

variable "key_deletion_principal_arns" {
  description = "Break-glass principals that may schedule deletion of, or disable, a protected key. Empty adds no statement."
  type        = list(string)
  default     = []
}

variable "table_storage_arn_patterns" {
  description = "S3 Tables encryption contexts (kms:EncryptionContext:aws:s3:arn patterns, such as arn:aws:s3tables:<region>:<account>:bucket/<name>/*) the direct-access exemption covers. Required with direct_access_exempt_principal_arns."
  type        = list(string)
  default     = []

  validation {
    condition     = length(var.direct_access_exempt_principal_arns) == 0 || length(var.table_storage_arn_patterns) > 0
    error_message = "table_storage_arn_patterns must name the table storage a direct-access exemption covers."
  }
}

variable "direct_access_exempt_principal_arns" {
  description = "Same-account principals exempt from the table-bucket direct-access deny, granted key use through S3 Tables and S3 on table_storage_arn_patterns only. Empty adds no statement."
  type        = list(string)
  default     = []
}
