# ----
# Common
# ----

variable "enabled" {
  description = "Set to false to prevent the module from creating any resources"
  type        = bool
  default     = true
}

variable "name_prefix" {
  description = "Organization unique prefix to use for resource names. Recommend including environment and region. e.g. 'prod-usw2'"
  type        = string

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{0,22}[a-z0-9]$", var.name_prefix))
    error_message = "The name_prefix value must start with a lowercase letter, followed by 0 to 22 alphanumeric or hyphen characters, ending with alphanumeric, for a total length of 2 to 24 characters."
  }
}

variable "tags" {
  description = "Tags/labels to apply to all resources"
  type        = map(string)
  default     = {}
}

variable "data_tags" {
  description = "Tags/labels to apply to all resources with data-at-rest"
  type        = map(string)
  default     = {}
}

variable "environment_type" {
  description = "Environment type for resource configuration defaults. Select 'None' to use individual config values."
  type        = string
  default     = "Production"
}

variable "cost_estimation_config" {
  description = "Configuration object for monthly cost estimation"
  type = object({
    enabled = bool
  })
  default = {
    enabled = true
  }
}

# ----
# Encryption and monitoring
# ----

variable "encryption_config" {
  description = "Configuration object for encryption settings and KMS key management"
  type = object({
    create_kms_key               = bool
    kms_key_id                   = string
    kms_key_deletion_window_days = number
  })
  default = {
    create_kms_key               = true
    kms_key_id                   = ""
    kms_key_deletion_window_days = 30
  }
}

variable "monitoring_config" {
  description = "Configuration object for CloudWatch monitoring"
  type = object({
    enabled = bool
  })
  default = {
    enabled = true
  }
}

variable "alarms_config" {
  description = "Configuration object for CloudWatch alarms and SNS notifications"
  type = object({
    enabled                 = bool
    create_sns_topic        = bool
    sns_topic_arn           = string
    scanned_bytes_threshold = optional(number)
  })
  default = {
    enabled          = true
    create_sns_topic = true
    sns_topic_arn    = ""
  }
}

# ----
# Lakehouse
# ----

# The object inputs below are typed `any` here and validated by the module's typed variables, so
# the example does not repeat every type.

variable "zones_config" {
  description = "Data zones"
  type        = any
  default = {
    zones = ["raw", "curated", "consumption"]
  }
}

variable "storage_config" {
  description = "Table storage settings"
  type        = any
  default     = {}
}

variable "query_config" {
  description = "Platform query defaults"
  type        = any
  default     = {}
}

variable "required_tags_config" {
  description = "Required tag keys"
  type        = any
  default     = {}
}

variable "catalog_integration_config" {
  description = "Account-level catalog integration"
  type        = any
  default = {
    enabled                               = true
    data_access_role_arn                  = ""
    allow_full_table_external_data_access = false
  }
}

variable "governance_config" {
  description = "LF-tag vocabularies and settings, without admin_principal_arns (see admin_principal_arns)"
  type        = any
  default     = {}
}

variable "admin_principal_arns" {
  description = "Lake Formation data-lake admins. Empty means the role running Terraform."
  type        = list(string)
  default     = []
}

variable "tenants" {
  description = "Tenant definitions (tenants.auto.tfvars); the module's tenants variable enforces the type"
  type        = any
  default     = {}
}

variable "claims_compliance_frameworks" {
  description = "Compliance designation of the claims tenant"
  type        = list(string)
  default     = ["HIPAA"]
}

variable "tenant_overrides" {
  description = "Attributes merged into individual tenant definitions, keyed by tenant name (used by tests)"
  type        = any
  default     = {}
}

variable "extra_tenants" {
  description = "Additional tenants merged into the tenant map (used by tests)"
  type        = any
  default     = {}
}
