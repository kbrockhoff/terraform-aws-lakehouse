# All tenant validation lives here, so platform-applied and standalone tenants are validated
# identically. Error messages name the tenant because the root applies many instances.

variable "platform" {
  description = "The platform_contract output of the root lakehouse module, from remote state, a decoded SSM parameter, or the root itself."
  type = object({
    name_prefix          = string
    account_id           = string
    partition            = string
    region               = string
    environment_type     = string
    baseline_environment = bool
    effective = object({
      bytes_scanned_cutoff_per_query = number
      query_results_expiration_days  = number
      unreferenced_days              = number
      non_current_days               = number
      force_destroy                  = bool
      writer_drop_enabled            = bool
      monitoring_enabled             = bool
      alarms_enabled                 = bool
      scanned_bytes_alarm_threshold  = number
      kms_key_deletion_window_days   = number
      table_storage_class            = string
    })
    shared_storage_classification = string
    zones = map(object({
      bucket_name = string
      bucket_arn  = string
      catalog_id  = string
    }))
    lf_tags = object({
      sensitivity = object({
        key    = string
        values = list(string)
      })
      regulations = map(string)
      domain = object({
        key    = string
        values = list(string)
      })
      tenant_key_prefix = string
    })
    regulation_designation  = map(string)
    kms_key_arn             = string
    data_access_role_arn    = string
    query_results           = object({ bucket_name = string, bucket_arn = string })
    alarm_sns_topic_arn     = string
    cost_estimation_enabled = bool
    common_tags             = map(string)
    data_tags               = map(string)
    required_tags = object({
      enforced                = bool
      cost_center_key         = string
      data_owner_key          = string
      data_classification_key = string
      tenant_key              = string
    })
  })

  validation {
    condition     = length(var.platform.zones) > 0 && length(var.platform.lf_tags.sensitivity.values) > 0 && length(var.platform.lf_tags.domain.values) > 0
    error_message = "platform must have zones and LF-tag vocabularies; pass the root module's platform_contract output (it is null when the platform has enabled = false)."
  }
}

variable "name" {
  description = "Tenant name. It becomes the S3 Tables namespace and part of bucket, workgroup, and LF-tag names."
  type        = string

  validation {
    condition     = can(regex("^[a-z][a-z0-9]{1,31}$", var.name))
    error_message = "Tenant name must match ^[a-z][a-z0-9]{1,31}$ (valid as both a namespace and part of a bucket name)."
  }
}

# Source of truth: the root var.tenants value type (tasks.md T006). Keep the two identical so one
# tenant definition works in both entry points.
variable "tenant" {
  description = "Tenant definition: owner, sensitivity ceiling, regulations, domain, zones, compliance designation, personas, row filters, and query limits."
  type = object({
    owner                 = string
    sensitivity           = string
    regulations           = optional(set(string), [])
    domain                = optional(string)
    zones                 = optional(list(string))
    compliance_frameworks = optional(set(string), [])
    stewards              = optional(list(string), [])
    writers = optional(map(object({
      principal_arn = string
      zones         = optional(list(string))
    })), {})
    readers = optional(map(object({
      principal_arn      = string
      zones              = optional(list(string))
      sensitivity_values = list(string)
      regulations        = optional(set(string), [])
      domains            = optional(list(string))
    })), {})
    row_filters = optional(map(object({
      reader                = string
      zone                  = string
      table                 = string
      row_filter_expression = string
      column_names          = optional(list(string))
    })), {})
    bytes_scanned_cutoff_per_query = optional(number)
    scanned_bytes_alarm_threshold  = optional(number)
    dedicated_storage              = optional(bool, false)
  })

  validation {
    condition = alltrue([
      for z in concat(
        coalesce(var.tenant.zones, keys(var.platform.zones)),
        flatten([for w in values(var.tenant.writers) : coalesce(w.zones, [])]),
        flatten([for r in values(var.tenant.readers) : coalesce(r.zones, [])]),
      ) : contains(keys(var.platform.zones), z)
    ])
    error_message = "Tenant ${var.name}: every tenant, writer, and reader zone must be a platform zone (${join(", ", keys(var.platform.zones))})."
  }

  validation {
    condition = alltrue([
      for z in concat(
        flatten([for w in values(var.tenant.writers) : coalesce(w.zones, [])]),
        flatten([for r in values(var.tenant.readers) : coalesce(r.zones, [])]),
      ) : contains(coalesce(var.tenant.zones, keys(var.platform.zones)), z)
    ])
    error_message = "Tenant ${var.name}: writer and reader zones must be among the tenant's zones."
  }

  validation {
    condition = alltrue([
      for a in concat(
        var.tenant.stewards,
        [for w in values(var.tenant.writers) : w.principal_arn],
        [for r in values(var.tenant.readers) : r.principal_arn],
      ) : can(regex("^arn:aws[a-z-]*:iam::[0-9]{12}:(role|user)/.+$", a))
    ])
    error_message = "Tenant ${var.name}: every steward, writer, and reader principal must be an IAM role or user ARN."
  }

  validation {
    condition     = contains(var.platform.lf_tags.sensitivity.values, var.tenant.sensitivity)
    error_message = "Tenant ${var.name}: sensitivity must be one of ${join(", ", var.platform.lf_tags.sensitivity.values)}."
  }

  validation {
    condition = alltrue(flatten([
      for r in values(var.tenant.readers) : [
        for s in r.sensitivity_values : (
          contains(var.platform.lf_tags.sensitivity.values, s) &&
          contains(var.platform.lf_tags.sensitivity.values, var.tenant.sensitivity) &&
          try(index(var.platform.lf_tags.sensitivity.values, s) <= index(var.platform.lf_tags.sensitivity.values, var.tenant.sensitivity), false)
        )
      ]
    ]))
    error_message = "Tenant ${var.name}: reader sensitivity_values must be platform sensitivity values at or below the tenant ceiling (${var.tenant.sensitivity})."
  }

  validation {
    condition     = alltrue([for r in var.tenant.regulations : contains(keys(var.platform.lf_tags.regulations), r)])
    error_message = "Tenant ${var.name}: regulations must be platform regulation values (${join(", ", keys(var.platform.lf_tags.regulations))})."
  }

  validation {
    condition = alltrue([
      for r in var.tenant.regulations : contains(var.tenant.compliance_frameworks, var.platform.regulation_designation[r])
      if contains(keys(var.platform.regulation_designation), r)
    ])
    error_message = "Tenant ${var.name}: regulations hipaa and pci_dss require the matching compliance_frameworks designation (HIPAA, PCI_DSS)."
  }

  validation {
    condition     = alltrue([for r in values(var.tenant.readers) : length(setsubtract(r.regulations, var.tenant.regulations)) == 0])
    error_message = "Tenant ${var.name}: reader regulations must be a subset of the tenant's regulations."
  }

  validation {
    condition = alltrue([
      for d in concat(
        var.tenant.domain == null ? [] : [var.tenant.domain],
        flatten([for r in values(var.tenant.readers) : coalesce(r.domains, [])]),
      ) : contains(var.platform.lf_tags.domain.values, d)
    ])
    error_message = "Tenant ${var.name}: domain and reader domains must be platform domain values (${join(", ", var.platform.lf_tags.domain.values)})."
  }

  validation {
    condition = alltrue([
      for f in values(var.tenant.row_filters) :
      contains(keys(var.tenant.readers), f.reader) && contains(coalesce(var.tenant.zones, keys(var.platform.zones)), f.zone)
    ])
    error_message = "Tenant ${var.name}: each row filter's reader must be a declared reader and its zone one of the tenant's zones."
  }

  validation {
    condition     = length(setsubtract(var.tenant.compliance_frameworks, ["SOC2", "HIPAA", "PCI_DSS"])) == 0
    error_message = "Tenant ${var.name}: compliance_frameworks may contain only SOC2, HIPAA, and PCI_DSS."
  }

  validation {
    condition     = var.tenant.bytes_scanned_cutoff_per_query == null || coalesce(var.tenant.bytes_scanned_cutoff_per_query, 0) >= 10485760
    error_message = "Tenant ${var.name}: bytes_scanned_cutoff_per_query must be at least 10485760 (10 MB)."
  }

  validation {
    condition = !(var.tenant.dedicated_storage || length(var.tenant.compliance_frameworks) > 0) || alltrue([
      for z in coalesce(var.tenant.zones, keys(var.platform.zones)) : (
        length("${var.platform.name_prefix}-${z}-${var.name}") <= 63 &&
        !contains([for pz in values(var.platform.zones) : pz.bucket_name], "${var.platform.name_prefix}-${z}-${var.name}")
      )
    ])
    error_message = "Tenant ${var.name}: dedicated bucket names <name_prefix>-<zone>-${var.name} must be at most 63 characters and must not equal a shared zone bucket name."
  }

  validation {
    condition     = length(var.tenant.compliance_frameworks) == 0 || length("${var.platform.name_prefix}-${var.name}-athena-results-${var.platform.account_id}") <= 63
    error_message = "Tenant ${var.name}: the designated results bucket name <name_prefix>-${var.name}-athena-results-<account_id> exceeds 63 characters; shorten the tenant name."
  }

  validation {
    condition = var.tenant.dedicated_storage || length(var.tenant.compliance_frameworks) > 0 || try(
      index(var.platform.lf_tags.sensitivity.values, var.tenant.sensitivity) <= index(var.platform.lf_tags.sensitivity.values, var.platform.shared_storage_classification),
      false
    )
    error_message = "Tenant ${var.name}: sensitivity ${var.tenant.sensitivity} ranks above the shared storage classification (${var.platform.shared_storage_classification}); set dedicated_storage = true (FR-022)."
  }
}

variable "tags" {
  description = "Additional tags for the tenant's resources."
  type        = map(string)
  default     = {}
}

variable "verify_deployer_admin" {
  description = "Fail the plan unless the deploying role is a Lake Formation data-lake admin. The root sets false because it guards this itself."
  type        = bool
  default     = true
}
