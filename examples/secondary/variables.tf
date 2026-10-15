variable "name_prefix" {
  description = "Organization unique prefix to use for resource names. Must differ from the owning deployment's prefix."
  type        = string

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{0,22}[a-z0-9]$", var.name_prefix))
    error_message = "The name_prefix value must be between 2 and 24 characters."
  }
}

variable "data_access_role_arn" {
  description = "The owning deployment's data_access_role_arn output."
  type        = string
}
