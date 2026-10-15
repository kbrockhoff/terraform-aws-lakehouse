# ----
# Persona IAM policy documents (outputs only; the module never attaches them)
# ----

locals {
  personas = merge(
    { for arn in var.tenant.stewards : "steward/${arn}" => { persona = "steward" } },
    { for k in keys(var.tenant.writers) : "writer/${k}" => { persona = "writer" } },
    { for k in keys(var.tenant.readers) : "reader/${k}" => { persona = "reader" } },
  )
  results_object_arn = "${local.results.bucket_arn}/${local.results.prefix}*"

  # Lake Formation authorizes the tenant scope; IAM limits Glue to this account's Data Catalog,
  # including the federated S3 Tables catalogs
  glue_arns = [
    "arn:${local.partition}:glue:${local.region}:${local.account_id}:catalog",
    "arn:${local.partition}:glue:${local.region}:${local.account_id}:catalog/*",
    "arn:${local.partition}:glue:${local.region}:${local.account_id}:database/*",
    "arn:${local.partition}:glue:${local.region}:${local.account_id}:table/*",
  ]
}

data "aws_iam_policy_document" "persona" {
  for_each = local.personas

  statement {
    sid    = "CatalogRead"
    effect = "Allow"
    actions = [
      "glue:GetCatalog",
      "glue:GetCatalogs",
      "glue:GetDatabase",
      "glue:GetDatabases",
      "glue:GetTable",
      "glue:GetTables",
      "glue:GetTableVersion",
      "glue:GetTableVersions",
    ]
    resources = local.glue_arns
  }

  statement {
    sid       = "LakeFormationDataAccess"
    effect    = "Allow"
    actions   = ["lakeformation:GetDataAccess"]
    resources = ["*"] # Lake Formation has no resource-level permissions
  }

  statement {
    sid    = "TenantWorkgroup"
    effect = "Allow"
    actions = [
      "athena:StartQueryExecution",
      "athena:GetQueryExecution",
      "athena:GetQueryResults",
      "athena:GetQueryResultsStream",
      "athena:GetQueryRuntimeStatistics",
      "athena:StopQueryExecution",
      "athena:GetWorkGroup",
    ]
    resources = [aws_athena_workgroup.this.arn]
  }

  statement {
    sid    = "TenantResults"
    effect = "Allow"
    actions = [
      "s3:GetObject",
      "s3:PutObject",
    ]
    resources = [local.results_object_arn]
  }

  statement {
    sid       = "ResultsBucketLocation"
    effect    = "Allow"
    actions   = ["s3:GetBucketLocation"]
    resources = [local.results.bucket_arn]
  }

  statement {
    sid    = "ResultsKey"
    effect = "Allow"
    actions = [
      "kms:Decrypt",
      "kms:GenerateDataKey",
    ]
    resources = [local.kms_key_arn]
    condition {
      test     = "StringEquals"
      variable = "kms:ViaService"
      values = [
        "s3.${local.region}.amazonaws.com",
        "athena.${local.region}.amazonaws.com",
      ]
    }
  }

  dynamic "statement" {
    for_each = each.value.persona == "writer" ? [1] : []
    content {
      sid    = "WriterTables"
      effect = "Allow"
      actions = concat(
        ["glue:CreateTable", "glue:UpdateTable"],
        local.effective.writer_drop_enabled ? ["glue:DeleteTable"] : [],
      )
      resources = local.glue_arns
    }
  }

  # Stewards hold CREATE_TABLE, ALTER, and DROP in Lake Formation, so IAM must allow the Glue calls
  dynamic "statement" {
    for_each = each.value.persona == "steward" ? [1] : []
    content {
      sid    = "StewardTables"
      effect = "Allow"
      actions = [
        "glue:CreateTable",
        "glue:UpdateTable",
        "glue:DeleteTable",
      ]
      resources = local.glue_arns
    }
  }

  dynamic "statement" {
    for_each = contains(["writer", "steward"], each.value.persona) ? [1] : []
    content {
      sid    = "Labeling"
      effect = "Allow"
      actions = [
        "lakeformation:AddLFTagsToResource",
        "lakeformation:RemoveLFTagsFromResource",
        "lakeformation:GetResourceLFTags",
        "lakeformation:GetLFTag",
        "lakeformation:ListLFTags",
      ]
      resources = ["*"] # Lake Formation has no resource-level permissions
    }
  }

  dynamic "statement" {
    for_each = each.value.persona == "steward" ? [1] : []
    content {
      sid    = "StewardGovernance"
      effect = "Allow"
      actions = [
        "lakeformation:GrantPermissions",
        "lakeformation:RevokePermissions",
        "lakeformation:ListPermissions",
        "lakeformation:SearchTablesByLFTags",
        "lakeformation:SearchDatabasesByLFTags",
        "lakeformation:CreateDataCellsFilter",
        "lakeformation:GetDataCellsFilter",
      ]
      resources = ["*"] # Lake Formation has no resource-level permissions
    }
  }
}
