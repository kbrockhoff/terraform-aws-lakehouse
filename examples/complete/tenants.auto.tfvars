# Placeholder principals in account 123456789012. Plans do not resolve them; replace them before
# applying.
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
    row_filters = {
      eu = {
        reader                = "gdpranalyst"
        zone                  = "curated"
        table                 = "orders"
        row_filter_expression = "region = 'EU'"
      }
    }
  }

  finance = {
    owner                          = "finance-data@example.com"
    sensitivity                    = "confidential"
    domain                         = "finance"
    bytes_scanned_cutoff_per_query = 107374182400 # 100 GB
    readers = {
      semantic = {
        principal_arn      = "arn:aws:iam::123456789012:role/semantic-layer"
        zones              = ["consumption"]
        sensitivity_values = ["public", "internal"]
      }
      controller = {
        principal_arn      = "arn:aws:iam::123456789012:role/finance-controller"
        sensitivity_values = ["public", "internal", "confidential"]
        domains            = ["finance"]
      }
    }
  }

  # compliance_frameworks comes from var.claims_compliance_frameworks (default ["HIPAA"])
  claims = {
    owner       = "claims-data@example.com"
    sensitivity = "restricted"
    regulations = ["hipaa", "gdpr"]
    domain      = "health"
    stewards    = ["arn:aws:iam::123456789012:role/claims-steward"]
    writers = {
      ingest = {
        principal_arn = "arn:aws:iam::123456789012:role/claims-ingest"
      }
    }
    readers = {
      adjuster = {
        principal_arn      = "arn:aws:iam::123456789012:role/claims-adjuster"
        sensitivity_values = ["public", "internal", "confidential", "restricted"]
        regulations        = ["hipaa", "gdpr"]
      }
    }
  }
}
