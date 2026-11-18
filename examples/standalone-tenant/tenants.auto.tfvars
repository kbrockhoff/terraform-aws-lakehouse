# Copied from examples/complete/tenants.auto.tfvars; keep the sales definition identical.
tenants = {
  sales = {
    owner       = "sales-data@example.com"
    sensitivity = "confidential"
    regulations = ["gdpr"]
    domain      = "sales"
    stewards    = ["arn:aws:iam::123456789012:role/sales-steward"]
    writers = {
      ingest = {
        principal_arn = "arn:aws:iam::123456789012:role/sales-ingest"
        zones         = ["raw", "curated"]
      }
    }
    readers = {
      semantic = {
        principal_arn      = "arn:aws:iam::123456789012:role/semantic-layer"
        zones              = ["consumption"]
        sensitivity_values = ["public", "internal"]
      }
      gdpranalyst = {
        principal_arn      = "arn:aws:iam::123456789012:role/sales-gdpr-analyst"
        sensitivity_values = ["public", "internal", "confidential"]
        regulations        = ["gdpr"]
      }
    }
    # Schema-only personas: a catalog browser, and a migration role that creates tables and alters
    # the ones stewards tag with the tenant's metadata-writer key
    metadata_readers = {
      catalog = {
        principal_arn = "arn:aws:iam::123456789012:role/data-catalog-browser"
      }
    }
    metadata_writers = {
      schema = {
        principal_arn = "arn:aws:iam::123456789012:role/sales-schema-migrator"
        zones         = ["curated"]
      }
    }
    row_filters = {
      eu = {
        reader                = "gdpranalyst"
        zone                  = "curated"
        table                 = "orders"
        row_filter_expression = "region = 'EU'"
      }
    }
    # Read-only, label-scoped share of the consumption zone with an approved consumer account
    shares = {
      partner = {
        account_id         = "111122223333"
        sensitivity_values = ["public", "internal"]
      }
    }
  }
}
