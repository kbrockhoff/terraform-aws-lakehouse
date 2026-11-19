# The manifest is the only interface lfverify reads (plan.md, Make Dependencies Explicit). Symbolic
# names in matrix files (tenant/zone, namespace, table) resolve through it.
output "manifest" {
  description = "Fixture identifiers for lfverify: personas, both grant styles, tables, and keys."
  value = {
    run_id     = var.run_id
    account_id = local.account_id
    region     = local.region
    prefix     = local.prefix
    stage      = var.stage
    inject     = var.inject
    runner     = local.runner_role_arn

    personas = merge(local.role_arn, local.consumer_enabled ? { consumer = aws_iam_role.consumer[0].arn } : {})

    styles = {
      label = {
        namespaces = {
          for k, ns in local.label_namespaces : k => {
            catalog_id       = ns.catalog_id
            database         = ns.database
            table_bucket_arn = module.lakehouse.tenants[split("/", k)[0]].table_bucket_arns[split("/", k)[1]]
            workgroup        = module.lakehouse.tenants[split("/", k)[0]].workgroup_name
            dml_workgroup    = module.lakehouse.tenants[split("/", k)[0]].dml_workgroup_name
            results_location = module.lakehouse.tenants[split("/", k)[0]].query_results_location
          }
        }
        tag_keys = {
          tenant      = { for t in local.tenant_names : t => module.lakehouse.tenants[t].tenant_tag_key }
          zone        = local.zone_tag_key
          sensitivity = local.sensitivity_tag_key
          domain      = module.lakehouse.lf_tag_vocabulary.domain.key
          regulations = { for r, v in module.lakehouse.lf_tag_vocabulary.regulations : r => v.key }
          mw          = module.lakehouse.tenants["alpha"].metadata_writer_tag_key
        }
        filters = { for k, f in local.alpha_row_filters : k => "alpha-${k}" }
      }
      named = {
        namespaces = {
          for ns in local.named_namespaces : ns => {
            catalog_id       = local.named_catalog_id
            database         = ns
            table_bucket_arn = aws_s3tables_table_bucket.named.arn
            workgroup        = aws_athena_workgroup.named.name
            dml_workgroup    = aws_athena_workgroup.named_dml.name
            results_location = "s3://${aws_s3_bucket.named_results.bucket}/"
          }
        }
        filters = { for k, f in local.named_filters : k => "${local.prefix}-${k}" }
      }
    }

    tables      = local.tables
    kms_key_arn = module.lakehouse.kms_key_arn

    # Spec 003 sharing cases: the consumer's resource link, and the share the revoke_share setup
    # revokes and restores
    consumer = local.consumer_enabled ? {
      catalog_id       = local.consumer_account_id
      database         = aws_glue_catalog_database.consumer_link[0].name
      table_bucket_arn = module.lakehouse.tenants["alpha"].table_bucket_arns["curated"]
      workgroup        = aws_athena_workgroup.consumer[0].name
      dml_workgroup    = aws_athena_workgroup.consumer[0].name
      results_location = "s3://${aws_s3_bucket.consumer_results[0].bucket}/"
    } : null
    share = local.consumer_enabled ? {
      account_id = local.consumer_share.account_id
      expression = local.consumer_share.expression
    } : null

    # Expected account settings for the spec 003 check cases
    account_settings = {
      cross_account_version   = "4"
      catalog_encryption_mode = module.lakehouse.catalog_encryption_mode
      catalog_kms_key_arn     = module.lakehouse.catalog_kms_key_arn
      # Set when the module creates s3tablescatalog; an existing catalog is never changed (FR-029)
      s3tables_catalog_identifier = "arn:${local.partition}:s3tables:${local.region}:${local.account_id}:bucket/*"
    }
  }
}
