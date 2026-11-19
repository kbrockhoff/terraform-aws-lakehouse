data "aws_caller_identity" "current" {}
data "aws_partition" "current" {}
data "aws_region" "current" {}

# The role running Terraform, used when runner_role_arn is empty
data "aws_iam_session_context" "current" {
  arn = data.aws_caller_identity.current.arn
}

locals {
  runner_role_arn = var.runner_role_arn != "" ? var.runner_role_arn : data.aws_iam_session_context.current.issuer_arn

  account_id = data.aws_caller_identity.current.account_id
  partition  = data.aws_partition.current.partition
  region     = data.aws_region.current.region

  # Persona role ARNs are derived from the names this fixture assigns (personas.tf), so they are
  # known at plan time: the module keys steward resources by principal ARN, and for_each keys
  # cannot be unknown. Referencing the role's name (known at plan) orders grants after their
  # principals without a module-level depends_on, which would defer every data source in the module
  # whenever a persona changes and force replacements (sandbox-results.md, T064).
  role_name = { for k in keys(local.personas) : k => "${local.prefix}-${replace(k, "_", "-")}" }
  role_arn  = { for k, r in aws_iam_role.persona : k => "arn:${local.partition}:iam::${local.account_id}:role/${r.name}" }

  all_sensitivity = ["public", "internal", "confidential", "restricted"]

  # Label-style row filters need the seeded orders table, so they exist only at stage 2
  # (plan.md, Two-stage apply). inject.tf can drop them (drop_row_filter).
  alpha_row_filters = { for k, f in {
    rows_eu = {
      reader                = "reader_rows"
      zone                  = "curated"
      table                 = "orders"
      row_filter_expression = "region = 'EU'"
      column_names          = null
    }
    rows_us = {
      reader                = "reader_rows"
      zone                  = "curated"
      table                 = "orders"
      row_filter_expression = "region = 'US'"
      column_names          = null
    }
    cells_eu = {
      reader                = "reader_cells"
      zone                  = "curated"
      table                 = "orders"
      row_filter_expression = "region = 'EU'"
      column_names          = [for c in local.tables.orders.columns : c.name if c.name != local.tables.orders.restricted_column]
    }
  } : k => f if var.stage == 2 && !local.inject_drop_row_filter && f.reader != local.inject_dropped_persona }

  # missing_grant on a reader or writer drops it from the tenants below, which removes the module's
  # LF-tag grants (inject.tf). locals.tf keeps an identity policy attached.
  inject_dropped_persona = local.inject_missing_grant ? var.inject_persona : ""

  reader = { for k in ["reader_single", "reader_rows", "reader_cells"] : k => {
    principal_arn      = local.role_arn[k]
    zones              = ["curated"]
    sensitivity_values = local.all_sensitivity
  } }

  tenants = {
    alpha = {
      owner       = "lfverify@example.com"
      sensitivity = "restricted"
      zones       = ["raw", "curated"]
      stewards    = [local.role_arn["steward"]]
      writers = { for k, w in {
        writer_multi  = { principal_arn = local.role_arn["writer_multi"], zones = ["curated"] }
        writer_single = { principal_arn = local.role_arn["writer_single"], zones = ["curated"] }
      } : k => w if k != local.inject_dropped_persona }
      readers = { for k, r in merge(local.reader, {
        reader_multi = {
          principal_arn      = local.role_arn["reader_multi"]
          zones              = ["curated"]
          sensitivity_values = local.all_sensitivity
        }
        # Columns labeled restricted are hidden (FR-017); drop_column_restriction widens this
        reader_cols = {
          principal_arn      = local.role_arn["reader_cols"]
          zones              = ["curated"]
          sensitivity_values = local.inject_drop_column_restriction ? local.all_sensitivity : ["public", "internal", "confidential"]
        }
      }) : k => r if k != local.inject_dropped_persona }
      # Metadata personas (FR-011, FR-012); labels.tf tags scratch for the metadata writer
      metadata_readers = {
        meta_reader = { principal_arn = local.role_arn["meta_reader"], zones = ["curated"] }
      }
      metadata_writers = {
        meta_writer = { principal_arn = local.role_arn["meta_writer"], zones = ["curated"] }
      }
      row_filters = local.alpha_row_filters
      # Spec 003 sharing cases: curated, public and internal, with the consumer account
      shares = local.consumer_enabled ? {
        consumer = {
          account_id         = local.consumer_account_id
          zones              = ["curated"]
          sensitivity_values = ["public", "internal"]
        }
      } : {}
    }
    beta = {
      owner       = "lfverify@example.com"
      sensitivity = "restricted"
      zones       = ["curated"]
      writers = { for k, w in {
        writer_multi = { principal_arn = local.role_arn["writer_multi"] }
      } : k => w if k != local.inject_dropped_persona }
      readers = { for k, r in {
        reader_multi = {
          principal_arn      = local.role_arn["reader_multi"]
          sensitivity_values = local.all_sensitivity
        }
      } : k => r if k != local.inject_dropped_persona }
      metadata_readers = {
        meta_reader = { principal_arn = local.role_arn["meta_reader"] }
      }
    }
    gamma = {
      owner       = "lfverify@example.com"
      sensitivity = "restricted"
      zones       = ["curated"]
    }
  }
}

# The label-style system under test (FR-002): this repository's module, granting through LF-tag
# expressions only
module "lakehouse" {
  source = "../.."

  providers = {
    aws         = aws
    aws.pricing = aws.pricing
  }

  name_prefix = local.prefix
  # Ephemeral: force_destroy on the results bucket and short retention suit a disposable fixture
  environment_type = "Ephemeral"
  tags = {
    LfverifyRun = var.run_id
  }

  cost_estimation_config = {
    enabled = false
  }

  # Writer cases run INSERT, UPDATE, and DELETE through the tenant DML workgroups (awslkhs-cdq)
  query_config = {
    dml_workgroup_enabled = true
  }

  # The fixture owns the account settings (Ephemeral) and supplies the long-lived catalog key
  account_settings_config = {
    catalog_encryption = {
      key_access_mode = var.catalog_key_access_mode
      kms_key_arn     = var.catalog_kms_key_arn
    }
    # s3tablescatalog outlives every run and is never updated, so the first creator fixes its
    # value. Versions before spec 003 Phase 8 created it with "False"; matching that keeps the
    # sandbox consistent whichever created it (sandbox-results.md, T083).
    s3tables_catalog = {
      allow_full_table_external_data_access = false
    }
  }

  governance_config = {
    # lfverify setup and cleanup call s3tables:DeleteTable directly as the runner, which the
    # table-bucket deny otherwise blocks (sandbox-results.md item 4). Personas stay non-exempt.
    # bypass_exempt (inject.tf) exempts the bypass persona too, which its cases must catch
    direct_access_exempt_principal_arns = concat(
      [local.runner_role_arn],
      local.inject_bypass_exempt ? [local.role_arn["bypass"]] : [],
    )

    # missing_grant on admin or roadmin removes that persona from its list
    admin_principal_arns = concat(
      [local.runner_role_arn],
      local.inject_missing_grant && var.inject_persona == "admin" ? [] : [local.role_arn["admin"]],
    )
    read_only_admin_principal_arns = concat(
      local.inject_missing_grant && var.inject_persona == "roadmin" ? [] : [local.role_arn["roadmin"]],
      var.read_only_runner_arns,
    )
    domain_values = ["general"]
  }

  sharing_config = {
    consumer_accounts = local.consumer_enabled ? {
      (local.consumer_account_id) = { purpose = "lfverify consumer for run ${var.run_id}" }
    } : {}
  }

  tenants = local.tenants
}
