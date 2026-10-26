variable "name_prefix" {
  description = "Organization unique prefix to use for resource names. Recommend including environment and region. e.g. 'prod-usw2'."
  type        = string

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{0,22}[a-z0-9]$", var.name_prefix))
    error_message = "The name_prefix value must be between 2 and 24 characters."
  }
}

variable "admin_principal_arns" {
  description = "Lake Formation data-lake admins. Empty means the role running Terraform."
  type        = list(string)
  default     = []
}

variable "zones_config" {
  description = "Data zones; the module default is raw, curated, and consumption."
  type = object({
    zones = list(string)
  })
  default = {
    zones = ["raw", "curated", "consumption"]
  }
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
