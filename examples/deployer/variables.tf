variable "name_prefix" {
  description = "The lakehouse name_prefix the policies are scoped to"
  type        = string
}

variable "iam_role_arn" {
  description = "Existing role to attach the policies to (a placeholder in this example)"
  type        = string
}

variable "cost_estimation_enabled" {
  description = "Include Price List API permissions"
  type        = bool
  default     = true
}

variable "identity_center_instance_arn" {
  description = "Identity Center instance of the Lake Formation integration; empty grants no Identity Center permissions"
  type        = string
  default     = ""
}
