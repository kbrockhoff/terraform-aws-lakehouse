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
  # cannot be unknown. The module depends on the roles, so grants never precede their principals.
  role_name = { for k in keys(local.personas) : k => "${local.prefix}-${replace(k, "_", "-")}" }
  role_arn  = { for k, n in local.role_name : k => "arn:${local.partition}:iam::${local.account_id}:role/${n}" }

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
  } : k => f if var.stage == 2 && !local.inject_drop_row_filter }

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
      writers = {
        writer_multi  = { principal_arn = local.role_arn["writer_multi"], zones = ["curated"] }
        writer_single = { principal_arn = local.role_arn["writer_single"], zones = ["curated"] }
      }
      readers = merge(local.reader, {
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
      })
      row_filters = local.alpha_row_filters
    }
    beta = {
      owner       = "lfverify@example.com"
      sensitivity = "restricted"
      zones       = ["curated"]
      writers = {
        writer_multi = { principal_arn = local.role_arn["writer_multi"] }
      }
      readers = {
        reader_multi = {
          principal_arn      = local.role_arn["reader_multi"]
          sensitivity_values = local.all_sensitivity
        }
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

  governance_config = {
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

  tenants = local.tenants

  depends_on = [aws_iam_role.persona]
}
