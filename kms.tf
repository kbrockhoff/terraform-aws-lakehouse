# ----
# KMS Key for encryption
# ----

# KMS Key for encrypting SNS topics and other resources
resource "aws_kms_key" "main" {
  count = var.enabled && var.encryption_config.create_kms_key ? 1 : 0

  description             = "Customer-managed key for ${local.name_prefix} module encryption"
  deletion_window_in_days = local.effective_config.kms_key_deletion_window_days
  enable_key_rotation     = true

  policy = data.aws_iam_policy_document.kms_key_policy[0].json

  tags = merge(local.common_data_tags, {
    Name = "${local.name_prefix}-cmk"
  })
}

# KMS Key Alias for easier reference
resource "aws_kms_alias" "main" {
  count = var.enabled && var.encryption_config.create_kms_key ? 1 : 0

  name          = "alias/${local.name_prefix}-cmk"
  target_key_id = aws_kms_key.main[0].key_id
}

# KMS Key Policy - allows account root and SNS service access, plus the statements S3 Tables,
# Lake Formation, and Athena need
data "aws_iam_policy_document" "kms_key_policy" {
  count = var.enabled && var.encryption_config.create_kms_key ? 1 : 0

  source_policy_documents = [data.aws_iam_policy_document.required_kms_statements[0].json]

  # Allow account root full access to the key
  statement {
    sid    = "EnableRootAccess"
    effect = "Allow"
    principals {
      type        = "AWS"
      identifiers = ["arn:${local.partition}:iam::${local.account_id}:root"]
    }
    actions   = ["kms:*"]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "aws:PrincipalAccount"
      values   = [local.account_id]
    }
  }

  # Allow SNS service to use the key
  statement {
    sid    = "AllowSNSAccess"
    effect = "Allow"
    principals {
      type        = "Service"
      identifiers = ["sns.${local.dns_suffix}"]
    }
    actions = [
      "kms:Decrypt",
      "kms:DescribeKey",
      "kms:Encrypt",
      "kms:GenerateDataKey*",
      "kms:ReEncrypt*"
    ]
    resources = ["*"]
    condition {
      test     = "Bool"
      variable = "kms:GrantIsForAWSResource"
      values   = [true]
    }
  }
}

# Statements any platform key needs, kept separate so they can be output for consumer-supplied
# keys
data "aws_iam_policy_document" "required_kms_statements" {
  count = var.enabled ? 1 : 0

  # S3 Tables maintenance (compaction, snapshot and unreferenced-file cleanup) on platform buckets
  statement {
    sid    = "AllowS3TablesMaintenance"
    effect = "Allow"
    principals {
      type        = "Service"
      identifiers = ["maintenance.s3tables.amazonaws.com"]
    }
    actions = [
      "kms:GenerateDataKey",
      "kms:Decrypt",
    ]
    resources = ["*"]
    condition {
      test     = "StringLike"
      variable = "kms:EncryptionContext:aws:s3:arn"
      values   = ["arn:${local.partition}:s3tables:${local.region}:${local.account_id}:bucket/${local.name_prefix}-*/*"]
    }
  }

  # Lake Formation vends table data through its data-access role
  statement {
    sid    = "AllowLakeFormationDataAccess"
    effect = "Allow"
    principals {
      type        = "AWS"
      identifiers = [local.data_access_role_arn]
    }
    actions = [
      "kms:Decrypt",
      "kms:GenerateDataKey",
    ]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "kms:ViaService"
      values = [
        "s3tables.${local.region}.amazonaws.com",
        "s3.${local.region}.amazonaws.com", # table metadata and data files are read through S3
      ]
    }
  }

  # Athena query results in the shared results bucket
  statement {
    sid    = "AllowAthenaResults"
    effect = "Allow"
    principals {
      type        = "AWS"
      identifiers = ["arn:${local.partition}:iam::${local.account_id}:root"]
    }
    actions = [
      "kms:Decrypt",
      "kms:GenerateDataKey",
    ]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "kms:ViaService"
      values = [
        "s3.${local.region}.amazonaws.com",
        "athena.${local.region}.amazonaws.com",
      ]
    }
  }
}
