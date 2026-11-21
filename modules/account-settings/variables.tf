# The dedicated catalog key, its policy, and the catalog-encryption role live together here. The
# key policy references the role, the role's inline policy references the key, and the role itself
# depends on neither, so the graph has no cycle.

variable "name_prefix" {
  description = "Organization unique prefix used for resource names."
  type        = string

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{0,22}[a-z0-9]$", var.name_prefix))
    error_message = "name_prefix must be 2 to 24 lowercase alphanumeric or hyphen characters, starting with a letter and ending with an alphanumeric character."
  }
}

variable "account_id" {
  description = "AWS account ID whose Lake Formation and Glue Data Catalog settings this submodule owns."
  type        = string

  validation {
    condition     = can(regex("^[0-9]{12}$", var.account_id))
    error_message = "account_id must be a 12-digit AWS account ID."
  }
}

variable "partition" {
  description = "AWS partition (aws, aws-us-gov, aws-cn)."
  type        = string

  validation {
    condition     = can(regex("^aws[a-z-]*$", var.partition))
    error_message = "partition must be an AWS partition name."
  }
}

variable "region" {
  description = "AWS Region of the settings."
  type        = string

  validation {
    condition     = can(regex("^[a-z]{2}(-[a-z]+)+-[0-9]$", var.region))
    error_message = "region must be an AWS Region name."
  }
}

variable "tags" {
  description = "Tags to apply to all resources. Common tags only: nothing here holds data at rest."
  type        = map(string)
  default     = {}
}

variable "environment_type" {
  description = "Environment type of the owning deployment. Only Ephemeral may change the catalog key after installation."
  type        = string

  validation {
    condition     = contains(["None", "Ephemeral", "Development", "Testing", "UAT", "Production", "MissionCritical"], var.environment_type)
    error_message = "environment_type must be one of None, Ephemeral, Development, Testing, UAT, Production, MissionCritical."
  }
}

variable "admin_principal_arns" {
  description = "Lake Formation data-lake admins, including studio integration principals. Authoritative: admins not listed are removed."
  type        = list(string)

  validation {
    condition     = length(var.admin_principal_arns) > 0
    error_message = "admin_principal_arns must not be empty."
  }

  validation {
    condition     = alltrue([for a in var.admin_principal_arns : can(regex("^arn:aws[a-z-]*:iam::[0-9]{12}:(role|user)/[^*]+$", a))])
    error_message = "Every admin_principal_arns entry must be an IAM role or user ARN without wildcards."
  }
}

variable "read_only_admin_principal_arns" {
  description = "Lake Formation read-only admins. Authoritative, like admin_principal_arns."
  type        = list(string)
  default     = []

  validation {
    condition     = alltrue([for a in var.read_only_admin_principal_arns : can(regex("^arn:aws[a-z-]*:iam::[0-9]{12}:(role|user)/[^*]+$", a))])
    error_message = "Every read_only_admin_principal_arns entry must be an IAM role or user ARN without wildcards."
  }
}

variable "deployer_role_arn" {
  description = "Role ARN of the deploying session. It must be one of admin_principal_arns."
  type        = string

  validation {
    condition     = can(regex("^arn:aws[a-z-]*:iam::[0-9]{12}:(role|user)/.+$", var.deployer_role_arn))
    error_message = "deployer_role_arn must be an IAM role or user ARN."
  }
}

variable "cross_account_version" {
  description = "Lake Formation cross-account version (4 or 5). Versions are never lowered; 5 is irreversible."
  type        = number
  default     = 4

  validation {
    condition     = contains([4, 5], var.cross_account_version)
    error_message = "cross_account_version must be 4 or 5."
  }
}

variable "data_lake" {
  description = "Explicit values for every data-lake settings field. PutDataLakeSettings replaces the whole object, so nothing is left implicit."
  type = object({
    allow_external_data_filtering         = bool
    external_data_filtering_allow_list    = list(string)
    authorized_session_tag_values         = list(string)
    allow_full_table_external_data_access = bool
    trusted_resource_owners               = list(string)
  })

  validation {
    condition     = !var.data_lake.allow_external_data_filtering || length(var.data_lake.external_data_filtering_allow_list) > 0
    error_message = "allow_external_data_filtering requires a non-empty external_data_filtering_allow_list."
  }
}

variable "catalog_encryption" {
  description = "Catalog metadata encryption. An empty kms_key_arn creates the dedicated catalog key."
  type = object({
    key_access_mode             = string
    kms_key_arn                 = string
    additional_key_user_arns    = list(string)
    key_deletion_principal_arns = list(string)
  })

  validation {
    condition     = contains(["service_role", "per_caller"], var.catalog_encryption.key_access_mode)
    error_message = "catalog_encryption.key_access_mode must be service_role or per_caller."
  }

  validation {
    condition = (
      var.catalog_encryption.kms_key_arn == "" ||
      can(regex("^arn:aws[a-z-]*:kms:[a-z0-9-]+:[0-9]{12}:key/[0-9a-f-]+$", var.catalog_encryption.kms_key_arn))
    )
    error_message = "catalog_encryption.kms_key_arn must be empty or a KMS key ARN."
  }
}

variable "kms_key_deletion_window_days" {
  description = "Deletion window for the created catalog key. Deletion itself is denied to everyone but key_deletion_principal_arns."
  type        = number

  validation {
    condition     = var.kms_key_deletion_window_days >= 7 && var.kms_key_deletion_window_days <= 30
    error_message = "kms_key_deletion_window_days must be between 7 and 30."
  }
}

variable "key_administrator_arns" {
  description = "Named administrators of the created catalog key, including the deploying role. The account root gets read-only access only."
  type        = list(string)

  validation {
    condition     = length(var.key_administrator_arns) > 0
    error_message = "key_administrator_arns must name at least one principal."
  }
}

variable "key_user_arns" {
  description = "Tenant persona ARNs granted catalog key use in per_caller mode."
  type        = list(string)
  default     = []
}

variable "storage_key_arns" {
  description = "Keys that encrypt table storage or query results. A supplied catalog key must not be one of them."
  type        = list(string)
  default     = []
}

variable "catalog_ready" {
  description = "ID of the root module's s3tablescatalog check-and-create step. Every resource here waits on it, so the catalog exists before anything else is created or changed (spec 003 FR-029)."
  type        = string
  default     = ""
}

variable "catalog_resource_policy" {
  description = "Optional Glue Data Catalog resource policy: the RAM sharing statement plus caller-supplied Deny statements."
  type = object({
    enabled           = bool
    deny_policy_jsons = list(string)
  })
}

variable "permissions_boundary_arn" {
  description = "Permissions boundary for the catalog-encryption role. Empty means none."
  type        = string
  default     = ""
}
