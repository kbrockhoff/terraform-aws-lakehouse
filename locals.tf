locals {
  # Environment type configuration maps
  environment_defaults = {
    None = {
      rpo_hours                      = null
      rto_hours                      = null
      monitoring_enabled             = var.monitoring_config.enabled
      alarms_enabled                 = var.alarms_config.enabled
      kms_key_deletion_window_days   = var.encryption_config.kms_key_deletion_window_days
      query_results_expiration_days  = var.query_config.results_expiration_days
      bytes_scanned_cutoff_per_query = var.query_config.bytes_scanned_cutoff_per_query
      # No *_config input carries the S3 Tables retention or force_destroy, so None uses the
      # Development values.
      unreferenced_days             = 3
      non_current_days              = 7
      force_destroy                 = false
      table_storage_class           = var.storage_config.table_storage_class
      writer_drop_enabled           = var.governance_config.writer_drop_enabled
      required_tags_enforced        = var.required_tags_config.enforced
      scanned_bytes_alarm_threshold = var.alarms_config.scanned_bytes_threshold
    }
    Ephemeral = {
      rpo_hours                      = null
      rto_hours                      = 48
      monitoring_enabled             = false
      alarms_enabled                 = false
      kms_key_deletion_window_days   = 7 # Minimal window for ephemeral environments
      query_results_expiration_days  = 1
      bytes_scanned_cutoff_per_query = 10 * 1073741824 # 10 GB
      unreferenced_days              = 1
      non_current_days               = 1
      force_destroy                  = true
      table_storage_class            = "STANDARD"
      writer_drop_enabled            = true
      required_tags_enforced         = false
      scanned_bytes_alarm_threshold  = 100 * 1073741824 # 100 GB
    }
    Development = {
      rpo_hours                      = 24
      rto_hours                      = 48
      monitoring_enabled             = false
      alarms_enabled                 = false
      kms_key_deletion_window_days   = 7 # Short window for development
      query_results_expiration_days  = 7
      bytes_scanned_cutoff_per_query = 10 * 1073741824 # 10 GB
      unreferenced_days              = 3
      non_current_days               = 7
      force_destroy                  = false
      table_storage_class            = "STANDARD"
      writer_drop_enabled            = true
      required_tags_enforced         = false
      scanned_bytes_alarm_threshold  = 100 * 1073741824 # 100 GB
    }
    Testing = {
      rpo_hours                      = 24
      rto_hours                      = 48
      monitoring_enabled             = false
      alarms_enabled                 = false
      kms_key_deletion_window_days   = 14 # Medium window for testing
      query_results_expiration_days  = 7
      bytes_scanned_cutoff_per_query = 10 * 1073741824 # 10 GB
      unreferenced_days              = 3
      non_current_days               = 7
      force_destroy                  = false
      table_storage_class            = "STANDARD"
      writer_drop_enabled            = true
      required_tags_enforced         = false
      scanned_bytes_alarm_threshold  = 100 * 1073741824 # 100 GB
    }
    UAT = {
      rpo_hours                      = 12
      rto_hours                      = 24
      monitoring_enabled             = false
      alarms_enabled                 = false
      kms_key_deletion_window_days   = 14 # Medium window for UAT
      query_results_expiration_days  = 30
      bytes_scanned_cutoff_per_query = 1099511627776 # 1 TB
      unreferenced_days              = 3
      non_current_days               = 7
      force_destroy                  = false
      table_storage_class            = "INTELLIGENT_TIERING"
      writer_drop_enabled            = false
      required_tags_enforced         = true
      scanned_bytes_alarm_threshold  = 10 * 1099511627776 # 10 TB
    }
    Production = {
      rpo_hours                      = 1
      rto_hours                      = 4
      monitoring_enabled             = true
      alarms_enabled                 = true
      kms_key_deletion_window_days   = 30 # Maximum window for production
      query_results_expiration_days  = 30
      bytes_scanned_cutoff_per_query = 1099511627776 # 1 TB
      unreferenced_days              = 3
      non_current_days               = 7
      force_destroy                  = false
      table_storage_class            = "INTELLIGENT_TIERING"
      writer_drop_enabled            = false
      required_tags_enforced         = true
      scanned_bytes_alarm_threshold  = 10 * 1099511627776 # 10 TB
    }
    MissionCritical = {
      rpo_hours                      = 0.083 # 5 minutes
      rto_hours                      = 1
      monitoring_enabled             = true
      alarms_enabled                 = true
      kms_key_deletion_window_days   = 30 # Maximum window for mission critical
      query_results_expiration_days  = 30
      bytes_scanned_cutoff_per_query = 1099511627776 # 1 TB
      unreferenced_days              = 3
      non_current_days               = 7
      force_destroy                  = false
      table_storage_class            = "INTELLIGENT_TIERING"
      writer_drop_enabled            = false
      required_tags_enforced         = true
      scanned_bytes_alarm_threshold  = 10 * 1099511627776 # 10 TB
    }
  }

  # Apply environment defaults when environment_type is not "None". For other environment
  # types, a non-null *_config input overrides the default. Null checks instead of coalesce():
  # under None both sides can be null, and the variable validations report that clearly.
  environment_config = local.environment_defaults[var.environment_type]
  effective_config = merge(local.environment_config, {
    query_results_expiration_days  = var.query_config.results_expiration_days != null ? var.query_config.results_expiration_days : local.environment_config.query_results_expiration_days
    bytes_scanned_cutoff_per_query = var.query_config.bytes_scanned_cutoff_per_query != null ? var.query_config.bytes_scanned_cutoff_per_query : local.environment_config.bytes_scanned_cutoff_per_query
    table_storage_class            = var.storage_config.table_storage_class != null ? var.storage_config.table_storage_class : local.environment_config.table_storage_class
    writer_drop_enabled            = var.governance_config.writer_drop_enabled != null ? var.governance_config.writer_drop_enabled : local.environment_config.writer_drop_enabled
    required_tags_enforced         = var.required_tags_config.enforced != null ? var.required_tags_config.enforced : local.environment_config.required_tags_enforced
    scanned_bytes_alarm_threshold  = var.alarms_config.scanned_bytes_threshold != null ? var.alarms_config.scanned_bytes_threshold : local.environment_config.scanned_bytes_alarm_threshold
  })

  # AWS account, partition, and region info
  account_id         = data.aws_caller_identity.current.account_id
  partition          = data.aws_partition.current.partition
  region             = data.aws_region.current.region
  dns_suffix         = data.aws_partition.current.dns_suffix
  reverse_dns_prefix = data.aws_partition.current.reverse_dns_prefix

  # Common tags for all resources including module metadata
  common_tags = merge(var.tags, {
    ModuleName    = "kbrockhoff/lakehouse/aws"
    ModuleVersion = local.module_version
    ModuleEnvType = var.environment_type
  })
  # Data tags take precedence over common tags
  common_data_tags = merge(local.common_tags, var.data_tags)

  name_prefix = var.name_prefix

  # KMS key logic - use provided key ID or create new one
  kms_key_id = var.enabled ? (
    var.encryption_config.create_kms_key ? aws_kms_key.main[0].arn : var.encryption_config.kms_key_id
  ) : ""
  kms_key_arn = local.kms_key_id

  # SNS topic logic - use provided topic ARN or create new one
  create_sns_topic = var.enabled && local.effective_config.alarms_enabled && var.alarms_config.create_sns_topic
  alarm_sns_topic_arn = var.enabled && local.effective_config.alarms_enabled ? (
    var.alarms_config.create_sns_topic ? aws_sns_topic.alarms[0].arn : var.alarms_config.sns_topic_arn
  ) : ""

  # Platform zones and their shared table bucket names
  zones             = toset(var.zones_config.zones)
  zone_bucket_names = { for z in local.zones : z => "${local.name_prefix}-${z}" }

  # Platform LF-tag vocabularies
  sensitivity_values = var.governance_config.sensitivity_values
  regulation_values  = var.governance_config.regulation_values
  domain_values      = var.governance_config.domain_values

  # Regulations that designate a tenant under a compliance framework
  regulation_designation = {
    hipaa   = "HIPAA"
    pci_dss = "PCI_DSS"
  }

  baseline_environment = contains(["Ephemeral", "Development", "Testing"], var.environment_type)

  # Role ARN of the deploying session (issuer of an assumed-role session)
  deployer_role_arn = data.aws_iam_session_context.current.issuer_arn

  # Lake Formation data-access role: created by the catalog-integration submodule, or passed in
  # by secondary deployments
  data_access_role_arn = var.catalog_integration_config.enabled ? (
    var.enabled ? module.catalog_integration[0].data_access_role_arn : ""
  ) : var.catalog_integration_config.data_access_role_arn

  # A platform setting, never computed from tenants: a tenant change must not modify platform
  # resources (FR-001), and standalone tenants are invisible to the platform (FR-022).
  shared_storage_classification = coalesce(
    var.governance_config.shared_storage_classification,
    local.sensitivity_values[length(local.sensitivity_values) - 1],
  )

  data_classification_key = var.required_tags_config.data_classification_key

  # Versioned public interface consumed by modules/tenant (plan.md: Tenant Submodule and Platform
  # Contract). Identifiers and effective settings only, never resource objects. Adding a field is
  # a minor change; removing or renaming one is a major change.
  platform_contract = var.enabled ? {
    name_prefix          = local.name_prefix
    account_id           = local.account_id
    partition            = local.partition
    region               = local.region
    environment_type     = var.environment_type
    baseline_environment = local.baseline_environment
    effective = {
      bytes_scanned_cutoff_per_query = local.effective_config.bytes_scanned_cutoff_per_query
      query_results_expiration_days  = local.effective_config.query_results_expiration_days
      unreferenced_days              = local.effective_config.unreferenced_days
      non_current_days               = local.effective_config.non_current_days
      force_destroy                  = local.effective_config.force_destroy
      writer_drop_enabled            = local.effective_config.writer_drop_enabled
      monitoring_enabled             = local.effective_config.monitoring_enabled
      alarms_enabled                 = local.effective_config.alarms_enabled
      scanned_bytes_alarm_threshold  = local.effective_config.scanned_bytes_alarm_threshold
      kms_key_deletion_window_days   = local.effective_config.kms_key_deletion_window_days
      table_storage_class            = local.effective_config.table_storage_class
    }
    shared_storage_classification = local.shared_storage_classification
    zones = {
      for z in local.zones : z => {
        bucket_name = aws_s3tables_table_bucket.zone[z].name
        bucket_arn  = aws_s3tables_table_bucket.zone[z].arn
        catalog_id  = "${local.account_id}:s3tablescatalog/${aws_s3tables_table_bucket.zone[z].name}"
      }
    }
    lf_tags = {
      sensitivity = {
        key    = aws_lakeformation_lf_tag.sensitivity[0].key
        values = local.sensitivity_values
      }
      regulations = { for r, t in aws_lakeformation_lf_tag.regulation : r => t.key }
      domain = {
        key    = aws_lakeformation_lf_tag.domain[0].key
        values = local.domain_values
      }
      tenant_key_prefix = "${local.name_prefix}-tenant-"
    }
    regulation_designation = local.regulation_designation
    kms_key_arn            = local.kms_key_arn
    data_access_role_arn   = local.data_access_role_arn
    query_results = {
      bucket_name = aws_s3_bucket.query_results[0].bucket
      bucket_arn  = aws_s3_bucket.query_results[0].arn
    }
    alarm_sns_topic_arn     = local.alarm_sns_topic_arn
    cost_estimation_enabled = var.cost_estimation_config.enabled
    common_tags             = local.common_tags
    data_tags               = var.data_tags
    required_tags = {
      enforced                = local.effective_config.required_tags_enforced
      cost_center_key         = var.required_tags_config.cost_center_key
      data_owner_key          = var.required_tags_config.data_owner_key
      data_classification_key = var.required_tags_config.data_classification_key
      tenant_key              = var.required_tags_config.tenant_key
    }
  } : null
}
