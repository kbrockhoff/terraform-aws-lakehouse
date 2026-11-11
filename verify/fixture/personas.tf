# ----
# Persona roles
# ----

# One role per persona. Only the runner can assume them, and only with a run-scoped session name,
# so CloudTrail attributes every persona call to its run.
data "aws_iam_policy_document" "persona_trust" {
  statement {
    sid     = "RunnerOnly"
    effect  = "Allow"
    actions = ["sts:AssumeRole", "sts:TagSession"]

    principals {
      type        = "AWS"
      identifiers = [local.runner_role_arn]
    }

    condition {
      test     = "StringLike"
      variable = "sts:RoleSessionName"
      values   = ["lfverify-${var.run_id}-*"]
    }
  }

  # A read-only probe runner (full/probe cross-check) uses its own run IDs, but still only
  # lfverify session names
  dynamic "statement" {
    for_each = length(var.read_only_runner_arns) > 0 ? [1] : []
    content {
      sid     = "ProbeRunners"
      effect  = "Allow"
      actions = ["sts:AssumeRole", "sts:TagSession"]

      principals {
        type        = "AWS"
        identifiers = var.read_only_runner_arns
      }

      condition {
        test     = "StringLike"
        variable = "sts:RoleSessionName"
        values   = ["lfverify-*"]
      }
    }
  }
}

resource "aws_iam_role" "persona" {
  for_each = local.personas

  name                 = local.role_name[each.key]
  description          = "lfverify ${each.value.kind} persona ${each.key} for run ${var.run_id}"
  assume_role_policy   = data.aws_iam_policy_document.persona_trust.json
  max_session_duration = 3600
}

# ----
# Identity policies
# ----

# Identity policies grant API reachability only; Lake Formation decides every data and metadata
# outcome. No persona policy may allow s3tables:GetTableData, s3tables:GetTableMetadataLocation,
# s3tables:PutTableData, or s3:GetObject on table storage: the storage-bypass cases (FR-022)
# expect those calls to be denied.

resource "aws_iam_role_policy" "module_doc" {
  for_each = local.persona_module_docs

  name   = "tenant-${each.value.tenant}"
  role   = aws_iam_role.persona[each.value.persona].id
  policy = module.lakehouse.tenants[each.value.tenant].persona_policy_json[each.value.doc_key]
}

# The module keys steward documents by principal ARN
resource "aws_iam_role_policy" "steward" {
  name   = "tenant-alpha"
  role   = aws_iam_role.persona["steward"].id
  policy = module.lakehouse.tenants["alpha"].persona_policy_json["steward/${aws_iam_role.persona["steward"].arn}"]
}

locals {
  # Same account-wide Glue scope as the module's persona documents; Lake Formation narrows it
  glue_arns = [
    "arn:${local.partition}:glue:${local.region}:${local.account_id}:catalog",
    "arn:${local.partition}:glue:${local.region}:${local.account_id}:catalog/*",
    "arn:${local.partition}:glue:${local.region}:${local.account_id}:database/*",
    "arn:${local.partition}:glue:${local.region}:${local.account_id}:table/*",
  ]

  # The module outputs workgroup names only; the ARN format is fixed by Athena
  tenant_workgroup_arns = [
    for t in local.tenant_names :
    "arn:${local.partition}:athena:${local.region}:${local.account_id}:workgroup/${module.lakehouse.tenants[t].workgroup_name}"
  ]
}

# Lake Formation decides whether the metadata writer may create, alter, or drop (FR-012), so IAM
# allows the Glue calls
data "aws_iam_policy_document" "meta_writer" {
  statement {
    sid       = "MetadataWrites"
    effect    = "Allow"
    actions   = ["glue:CreateTable", "glue:UpdateTable", "glue:DeleteTable"]
    resources = local.glue_arns
  }
}

resource "aws_iam_role_policy" "meta_writer" {
  name   = "metadata-writes"
  role   = aws_iam_role.persona["meta_writer"].id
  policy = data.aws_iam_policy_document.meta_writer.json
}

resource "aws_iam_role_policy_attachment" "admin" {
  role       = aws_iam_role.persona["admin"].name
  policy_arn = "arn:${local.partition}:iam::aws:policy/AWSLakeFormationDataAdmin"
}

# Admins query every tenant workgroup to record whether they hold implicit data access (FR-009)
data "aws_iam_policy_document" "admin" {
  statement {
    sid    = "Workgroups"
    effect = "Allow"
    actions = [
      "athena:StartQueryExecution",
      "athena:GetQueryExecution",
      "athena:GetQueryResults",
      "athena:StopQueryExecution",
      "athena:GetWorkGroup",
    ]
    resources = local.tenant_workgroup_arns
  }

  statement {
    sid       = "DataAccess"
    effect    = "Allow"
    actions   = ["lakeformation:GetDataAccess"]
    resources = ["*"] # Lake Formation has no resource-level permissions
  }

  statement {
    sid       = "Results"
    effect    = "Allow"
    actions   = ["s3:GetObject", "s3:PutObject", "s3:GetBucketLocation"]
    resources = [module.lakehouse.query_results_bucket_arn, "${module.lakehouse.query_results_bucket_arn}/*"]
  }

  statement {
    sid       = "ResultsKey"
    effect    = "Allow"
    actions   = ["kms:Decrypt", "kms:GenerateDataKey"]
    resources = [module.lakehouse.kms_key_arn]
  }
}

resource "aws_iam_role_policy" "admin" {
  name   = "lfverify-admin"
  role   = aws_iam_role.persona["admin"].id
  policy = data.aws_iam_policy_document.admin.json
}

# The read-only administrator persona from the Lake Formation personas reference, with its
# explicit settings deny
data "aws_iam_policy_document" "roadmin" {
  statement {
    sid    = "ReadOnlyAdmin"
    effect = "Allow"
    actions = [
      "lakeformation:GetDataLakeSettings",
      "lakeformation:GetEffectivePermissionsForPath",
      "lakeformation:ListPermissions",
      "lakeformation:ListDataCellsFilter",
      "lakeformation:GetDataCellsFilter",
      "lakeformation:SearchDatabasesByLFTags",
      "lakeformation:SearchTablesByLFTags",
      "lakeformation:GetLFTag",
      "lakeformation:ListLFTags",
      "lakeformation:GetResourceLFTags",
      "lakeformation:ListLakeFormationOptins",
      "glue:GetCatalog",
      "glue:GetCatalogs",
      "glue:GetDatabase",
      "glue:GetDatabases",
      "glue:SearchTables",
      "glue:GetTable",
      "glue:GetTableVersions",
      "glue:GetPartitions",
      "glue:GetTables",
      "lakeformation:GetDataAccess",
    ]
    resources = ["*"] # Lake Formation and the federated catalogs are what this persona inspects
  }

  # Reaching Athena lets the matrix confirm that read-only admins cannot read table data
  statement {
    sid    = "Workgroups"
    effect = "Allow"
    actions = [
      "athena:StartQueryExecution",
      "athena:GetQueryExecution",
      "athena:GetQueryResults",
      "athena:GetWorkGroup",
    ]
    resources = local.tenant_workgroup_arns
  }

  statement {
    sid       = "Results"
    effect    = "Allow"
    actions   = ["s3:GetObject", "s3:PutObject", "s3:GetBucketLocation"]
    resources = [module.lakehouse.query_results_bucket_arn, "${module.lakehouse.query_results_bucket_arn}/*"]
  }

  statement {
    sid       = "ResultsKey"
    effect    = "Allow"
    actions   = ["kms:Decrypt", "kms:GenerateDataKey"]
    resources = [module.lakehouse.kms_key_arn]
  }

  statement {
    sid       = "NoSettingsChanges"
    effect    = "Deny"
    actions   = ["lakeformation:PutDataLakeSettings"]
    resources = ["*"]
  }
}

resource "aws_iam_role_policy" "roadmin" {
  name   = "lfverify-roadmin"
  role   = aws_iam_role.persona["roadmin"].id
  policy = data.aws_iam_policy_document.roadmin.json
}
