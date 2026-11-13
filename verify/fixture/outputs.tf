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

    personas = local.role_arn

    styles = {
      label = {
        namespaces = {
          for k, ns in local.label_namespaces : k => {
            catalog_id       = ns.catalog_id
            database         = ns.database
            table_bucket_arn = module.lakehouse.tenants[split("/", k)[0]].table_bucket_arns[split("/", k)[1]]
            workgroup        = module.lakehouse.tenants[split("/", k)[0]].workgroup_name
            results_location = module.lakehouse.tenants[split("/", k)[0]].query_results_location
          }
        }
        tag_keys = {
          tenant      = { for t in local.tenant_names : t => module.lakehouse.tenants[t].tenant_tag_key }
          zone        = local.zone_tag_key
          sensitivity = local.sensitivity_tag_key
          domain      = module.lakehouse.lf_tag_vocabulary.domain.key
          regulations = { for r, v in module.lakehouse.lf_tag_vocabulary.regulations : r => v.key }
          mw          = aws_lakeformation_lf_tag.mw.key
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
            results_location = "s3://${aws_s3_bucket.named_results.bucket}/"
          }
        }
        filters = { for k, f in local.named_filters : k => "${local.prefix}-${k}" }
      }
    }

    tables      = local.tables
    kms_key_arn = module.lakehouse.kms_key_arn
  }
}
