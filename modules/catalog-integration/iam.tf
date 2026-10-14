# ----
# Lake Formation data-access role for S3 Tables
# ----

resource "aws_iam_role" "lf_s3tables_access" {
  name               = "${var.name_prefix}-lf-s3tables-access"
  description        = "Lake Formation data access to S3 table buckets in ${var.region}"
  assume_role_policy = data.aws_iam_policy_document.lf_s3tables_trust.json

  tags = var.tags
}

data "aws_iam_policy_document" "lf_s3tables_trust" {
  statement {
    sid    = "LakeFormationAssume"
    effect = "Allow"
    principals {
      type        = "Service"
      identifiers = ["lakeformation.amazonaws.com"]
    }
    actions = [
      "sts:AssumeRole",
      "sts:SetContext",
      "sts:SetSourceIdentity",
    ]
    condition {
      test     = "StringEquals"
      variable = "aws:SourceAccount"
      values   = [var.account_id]
    }
  }
}

resource "aws_iam_role_policy" "lf_s3tables_access" {
  name   = "s3tables-data-access"
  role   = aws_iam_role.lf_s3tables_access.id
  policy = data.aws_iam_policy_document.lf_s3tables_access.json
}

data "aws_iam_policy_document" "lf_s3tables_access" {
  statement {
    sid    = "S3TablesDataAccess"
    effect = "Allow"
    actions = [
      "s3tables:ListTableBuckets",
      "s3tables:GetTableBucket",
      "s3tables:ListNamespaces",
      "s3tables:GetNamespace",
      "s3tables:CreateNamespace",
      "s3tables:DeleteNamespace",
      "s3tables:ListTables",
      "s3tables:CreateTable",
      "s3tables:DeleteTable",
      "s3tables:RenameTable",
      "s3tables:GetTable",
      "s3tables:GetTableMetadataLocation",
      "s3tables:UpdateTableMetadataLocation",
      "s3tables:GetTableData",
      "s3tables:PutTableData",
    ]
    resources = ["arn:${var.partition}:s3tables:${var.region}:${var.account_id}:bucket/*"]
  }

  # Platform and tenant key policies scope this to specific keys
  statement {
    sid    = "KmsViaS3Tables"
    effect = "Allow"
    actions = [
      "kms:Decrypt",
      "kms:GenerateDataKey",
    ]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "kms:ViaService"
      values   = ["s3tables.${var.region}.amazonaws.com"]
    }
    condition {
      test     = "StringEquals"
      variable = "aws:ResourceAccount"
      values   = [var.account_id]
    }
  }
}
