# ----
# Run identity
# ----

variable "run_id" {
  description = "Six lowercase hex characters that identify this run. Every fixture name starts with lfv-<run_id>."
  type        = string

  validation {
    condition     = can(regex("^[0-9a-f]{6}$", var.run_id))
    error_message = "run_id must be exactly six lowercase hex characters."
  }
}

variable "runner_role_arn" {
  description = "IAM role ARN that runs lfverify. It is the fixture owner, a data-lake admin, and the only principal persona roles trust. Empty means the role running Terraform."
  type        = string
  default     = ""

  validation {
    condition     = var.runner_role_arn == "" || can(regex("^arn:aws[a-z-]*:iam::[0-9]{12}:role/.+$", var.runner_role_arn))
    error_message = "runner_role_arn must be empty or an IAM role ARN."
  }
}

variable "read_only_runner_arns" {
  description = "Extra read-only Lake Formation admins, such as the probe-mode runner used for the full/probe cross-check."
  type        = list(string)
  default     = []

  validation {
    condition     = alltrue([for a in var.read_only_runner_arns : can(regex("^arn:aws[a-z-]*:iam::[0-9]{12}:role/.+$", a))])
    error_message = "Every read_only_runner_arns entry must be an IAM role ARN."
  }
}

# ----
# Apply stages and misconfiguration overlays
# ----

variable "stage" {
  description = "1 creates the platform, buckets, namespaces, roles, and database grants. 2 adds table-level labels, grants, and filters after lfverify seed creates the tables."
  type        = number
  default     = 1

  validation {
    condition     = contains([1, 2], var.stage)
    error_message = "stage must be 1 or 2."
  }
}

variable "inject" {
  description = "Misconfiguration overlay for lfverify detect. Empty means none."
  type        = string
  default     = ""

  validation {
    condition     = contains(["", "extra_grant", "missing_grant", "drop_column_restriction", "drop_row_filter", "open_default"], var.inject)
    error_message = "inject must be empty or one of extra_grant, missing_grant, drop_column_restriction, drop_row_filter, open_default."
  }

  validation {
    condition     = var.inject == "" || var.stage == 2
    error_message = "inject overlays require stage = 2."
  }
}

variable "inject_persona" {
  description = "Persona key that extra_grant or missing_grant applies to."
  type        = string
  default     = ""

  validation {
    condition = (
      !contains(["extra_grant", "missing_grant"], var.inject) ||
      contains([
        "admin", "roadmin", "meta_reader", "meta_writer", "reader_multi", "reader_single", "writer_multi",
        "writer_single", "reader_cols", "reader_rows", "reader_cells", "control", "steward",
      ], var.inject_persona)
    )
    error_message = "inject_persona must be a persona key when inject is extra_grant or missing_grant."
  }
}
