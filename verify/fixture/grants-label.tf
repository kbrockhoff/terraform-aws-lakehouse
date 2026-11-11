# ----
# Label-style grants for personas the module does not model (FR-011, FR-012)
# ----

# LF-tag policy grants are accepted only at account level (catalog_id = account), and each tenant
# has its own tag key, so a persona spanning tenants needs one grant per tenant.

locals {
  zone_tag_key = module.lakehouse.lf_tag_vocabulary.zone.key

  tenant_curated_expression = {
    for t in local.tenant_names : t => [
      { key = module.lakehouse.tenants[t].tenant_tag_key, values = ["member"] },
      { key = local.zone_tag_key, values = ["curated"] },
    ]
  }

  meta_reader_grants = {
    for pair in setproduct(["alpha", "beta"], ["DATABASE", "TABLE"]) : "${pair[0]}/${pair[1]}" => {
      tenant        = pair[0]
      resource_type = pair[1]
    }
  }
}

# Marks the one table the metadata writer may alter and drop (labels.tf assigns it)
resource "aws_lakeformation_lf_tag" "mw" {
  key    = "${local.prefix}-mw"
  values = ["yes"]

  # The runner becomes a data-lake admin inside the module; nothing here may run before that
  depends_on = [module.lakehouse]
}

resource "aws_lakeformation_permissions" "meta_reader" {
  for_each = local.meta_reader_grants

  principal   = aws_iam_role.persona["meta_reader"].arn
  permissions = ["DESCRIBE"]

  lf_tag_policy {
    catalog_id    = local.account_id
    resource_type = each.value.resource_type
    dynamic "expression" {
      for_each = local.tenant_curated_expression[each.value.tenant]
      content {
        key    = expression.value.key
        values = expression.value.values
      }
    }
  }

  # The runner becomes a data-lake admin inside the module; nothing here may run before that
  depends_on = [module.lakehouse]
}

resource "aws_lakeformation_permissions" "meta_writer_database" {
  principal   = aws_iam_role.persona["meta_writer"].arn
  permissions = ["CREATE_TABLE"]

  lf_tag_policy {
    catalog_id    = local.account_id
    resource_type = "DATABASE"
    dynamic "expression" {
      for_each = local.tenant_curated_expression["alpha"]
      content {
        key    = expression.value.key
        values = expression.value.values
      }
    }
  }

  # The runner becomes a data-lake admin inside the module; nothing here may run before that
  depends_on = [module.lakehouse]
}

resource "aws_lakeformation_permissions" "meta_writer_table" {
  principal   = aws_iam_role.persona["meta_writer"].arn
  permissions = ["ALTER", "DROP"]

  lf_tag_policy {
    catalog_id    = local.account_id
    resource_type = "TABLE"
    expression {
      key    = aws_lakeformation_lf_tag.mw.key
      values = ["yes"]
    }
  }

  # The runner becomes a data-lake admin inside the module; nothing here may run before that
  depends_on = [module.lakehouse]
}
