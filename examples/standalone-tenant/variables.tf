# Typed by modules/tenant; `any` here avoids repeating the contract and tenant types
variable "platform_contract" {
  description = "The platform's platform_contract output"
  type        = any
}

variable "tenants" {
  description = "Tenant definitions; this example provisions the sales entry"
  type        = any
}

variable "tenant_overrides" {
  description = "Attributes merged into the sales definition (used by tests)"
  type        = any
  default     = {}
}

variable "verify_deployer_admin" {
  description = "Fail the plan unless the deploying role is a Lake Formation data-lake admin. Keep true in pipelines; tests turn it off in accounts where the test role is not an admin."
  type        = bool
  default     = true
}
