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
