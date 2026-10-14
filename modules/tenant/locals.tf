locals {
  name_prefix = var.platform.name_prefix
  account_id  = var.platform.account_id
  partition   = var.platform.partition
  region      = var.platform.region
  effective   = var.platform.effective

  zones      = toset(coalesce(var.tenant.zones, keys(var.platform.zones)))
  designated = length(var.tenant.compliance_frameworks) > 0
  dedicated  = var.tenant.dedicated_storage || local.designated
  domain     = coalesce(var.tenant.domain, var.platform.lf_tags.domain.values[0])

  tenant_tag_key = "${var.platform.lf_tags.tenant_key_prefix}${var.name}"

  # Storage per zone: dedicated buckets, or the tenant namespace in the shared zone bucket
  dedicated_bucket_names = local.dedicated ? { for z in local.zones : z => "${local.name_prefix}-${z}-${var.name}" } : {}
  bucket_name = {
    for z in local.zones : z => local.dedicated ? local.dedicated_bucket_names[z] : var.platform.zones[z].bucket_name
  }
  bucket_arn = {
    for z in local.zones : z => local.dedicated ? aws_s3tables_table_bucket.dedicated[z].arn : var.platform.zones[z].bucket_arn
  }
  catalog_id = {
    for z in local.zones : z => "${local.account_id}:s3tablescatalog/${local.bucket_name[z]}"
  }

  kms_key_arn = local.designated ? aws_kms_key.this[0].arn : var.platform.kms_key_arn

  results = local.designated ? {
    bucket_arn  = aws_s3_bucket.results[0].arn
    bucket_name = aws_s3_bucket.results[0].id
    prefix      = ""
    } : {
    bucket_arn  = var.platform.query_results.bucket_arn
    bucket_name = var.platform.query_results.bucket_name
    prefix      = "${var.name}/"
  }

  # Database labels: the tenant's most restrictive values, so unlabeled tables fail closed
  database_labels = merge(
    { (local.tenant_tag_key) = "member" },
    { (var.platform.lf_tags.sensitivity.key) = var.tenant.sensitivity },
    { for r, key in var.platform.lf_tags.regulations : key => contains(var.tenant.regulations, r) ? "yes" : "no" },
    { (var.platform.lf_tags.domain.key) = local.domain },
  )

  # L(T): the label values writers and stewards may associate within this tenant
  sensitivity_rank = index(var.platform.lf_tags.sensitivity.values, var.tenant.sensitivity)
  label_scope = merge(
    { (var.platform.lf_tags.sensitivity.key) = slice(var.platform.lf_tags.sensitivity.values, 0, local.sensitivity_rank + 1) },
    { for r, key in var.platform.lf_tags.regulations : key => contains(var.tenant.regulations, r) ? ["no", "yes"] : ["no"] },
    { (var.platform.lf_tags.domain.key) = var.platform.lf_tags.domain.values },
  )

  # Persona grants, one per persona and zone
  steward_grants = {
    for pair in setproduct(local.zones, var.tenant.stewards) : "${pair[0]}/${pair[1]}" => {
      zone          = pair[0]
      principal_arn = pair[1]
    }
  }
  writer_grants = merge([
    for k, w in var.tenant.writers : {
      for z in coalesce(w.zones, tolist(local.zones)) : "${k}/${z}" => {
        key           = k
        zone          = z
        principal_arn = w.principal_arn
      }
    }
  ]...)
  reader_grants = merge([
    for k, r in var.tenant.readers : {
      for z in coalesce(r.zones, tolist(local.zones)) : "${k}/${z}" => {
        key                = k
        zone               = z
        principal_arn      = r.principal_arn
        sensitivity_values = r.sensitivity_values
        regulations        = r.regulations
        domains            = r.domains
      }
    }
  ]...)
  row_filters = var.tenant.row_filters

  bytes_scanned_cutoff_per_query = coalesce(var.tenant.bytes_scanned_cutoff_per_query, local.effective.bytes_scanned_cutoff_per_query)
  scanned_bytes_alarm_threshold  = coalesce(var.tenant.scanned_bytes_alarm_threshold, local.effective.scanned_bytes_alarm_threshold)

  tags = merge(var.platform.common_tags, var.tags, {
    (var.platform.required_tags.tenant_key)     = var.name
    (var.platform.required_tags.data_owner_key) = var.tenant.owner
  })
  data_tags = merge(local.tags, var.platform.data_tags, {
    (var.platform.required_tags.data_classification_key) = var.tenant.sensitivity
    }, local.designated ? {
    ComplianceFrameworks = join(" ", sort(tolist(var.tenant.compliance_frameworks)))
  } : {})
}
