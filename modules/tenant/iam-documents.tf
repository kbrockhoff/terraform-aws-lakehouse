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
      "lakeformation:GetDataAccess",
    ]
    resources = ["*"]
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
      resources = ["*"]
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
      resources = ["*"]
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
      resources = ["*"]
    }
  }
}
