locals {
  prefix = "lfv-${var.run_id}"

  # Label-style tenants: alpha and beta are granted, gamma is the control namespace that only
  # admins reach (FR-005). Each tenant has one namespace per zone.
  tenant_names = ["alpha", "beta", "gamma"]

  # Named-resource baseline namespaces mirror the tenants' curated namespaces
  named_namespaces = ["n_alpha", "n_beta", "n_gamma"]

  # Persona keys are shared by Terraform, the manifest, matrix files, and session names.
  # module_docs lists the tenant persona_policy_json documents attached as identity policies;
  # personas the module does not model reuse a reader's document, so Lake Formation alone
  # decides their access (FR-020).
  personas = {
    admin         = { kind = "admin", module_docs = [] }
    roadmin       = { kind = "roadmin", module_docs = [] }
    meta_reader   = { kind = "meta", module_docs = [["alpha", "reader/reader_multi"], ["beta", "reader/reader_multi"]] }
    meta_writer   = { kind = "meta", module_docs = [["alpha", "reader/reader_single"]] }
    reader_multi  = { kind = "reader", module_docs = [["alpha", "reader/reader_multi"], ["beta", "reader/reader_multi"]] }
    reader_single = { kind = "reader", module_docs = [["alpha", "reader/reader_single"]] }
    writer_multi  = { kind = "writer", module_docs = [["alpha", "writer/writer_multi"], ["beta", "writer/writer_multi"]] }
    writer_single = { kind = "writer", module_docs = [["alpha", "writer/writer_single"]] }
    reader_cols   = { kind = "reader", module_docs = [["alpha", "reader/reader_cols"]] }
    reader_rows   = { kind = "reader", module_docs = [["alpha", "reader/reader_rows"]] }
    reader_cells  = { kind = "reader", module_docs = [["alpha", "reader/reader_cells"]] }
    control       = { kind = "control", module_docs = [["alpha", "reader/reader_single"]] }
    steward       = { kind = "steward", module_docs = [] } # keyed by role ARN in the module; see personas.tf
  }

  # One inline policy per persona and module document, because the documents share statement IDs
  # and cannot be merged into one policy
  persona_module_docs = merge([
    for p, v in local.personas : {
      for d in v.module_docs : "${p}/${d[0]}" => { persona = p, tenant = d[0], doc_key = d[1] }
    }
  ]...)

  # Fixture tables, created by lfverify seed in every namespace of both styles. ssn is the
  # restricted column, order_date the partition column, and attrs the map column that row
  # filter expressions cannot reference (FR-021).
  tables = {
    orders = {
      columns = [
        { name = "order_id", type = "bigint" },
        { name = "region", type = "string" },
        { name = "amount", type = "decimal(12,2)" },
        { name = "ssn", type = "string" },
        { name = "attrs", type = "map<string,string>" },
        { name = "order_date", type = "date" },
      ]
      partition_column  = "order_date"
      restricted_column = "ssn"
      map_column        = "attrs"
      rows              = 200
    }
    customers = {
      columns = [
        { name = "customer_id", type = "bigint" },
        { name = "name", type = "string" },
        { name = "tier", type = "string" },
      ]
      partition_column  = null
      restricted_column = null
      map_column        = null
      rows              = 50
    }
    scratch = {
      columns = [
        { name = "id", type = "bigint" },
        { name = "note", type = "string" },
      ]
      partition_column  = null
      restricted_column = null
      map_column        = null
      rows              = 5
    }
  }

  # Column sensitivity labels for the label style (stage 2). Columns not listed are internal.
  column_sensitivity = {
    orders = { ssn = "restricted" }
  }
}
