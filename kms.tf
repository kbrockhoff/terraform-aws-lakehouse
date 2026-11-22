# ----
# KMS Key for encryption
# ----

# KMS Key for encrypting SNS topics and other resources. Encrypts table buckets and query results
# only; catalog metadata uses the dedicated key in modules/account-settings (FR-009).
resource "aws_kms_key" "main" {
  count = var.enabled && var.encryption_config.create_kms_key ? 1 : 0

  description             = "Customer-managed key for ${local.name_prefix} module encryption"
  deletion_window_in_days = local.effective_config.kms_key_deletion_window_days
  enable_key_rotation     = true

  policy = data.aws_iam_policy_document.kms_key_policy[0].json

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-cmk"
  })

  # s3tablescatalog exists before anything else is created or changed (FR-029)
  depends_on = [terraform_data.s3tables_catalog]
}

# KMS Key Alias for easier reference
resource "aws_kms_alias" "main" {
  count = var.enabled && var.encryption_config.create_kms_key ? 1 : 0

  name          = "alias/${local.name_prefix}-cmk"
  target_key_id = aws_kms_key.main[0].key_id
}

# Statements every module-created key shares: named administrators, a read-only account root,
# and key use for direct-access exemptions
module "key_policy_statements" {
  source = "./modules/key-policy-statements"
  count  = var.enabled ? 1 : 0

  account_id                          = local.account_id
  partition                           = local.partition
  region                              = local.region
  key_administrator_arns              = local.key_administrator_arns
  direct_access_exempt_principal_arns = var.governance_config.direct_access_exempt_principal_arns
  # The platform's table buckets, the encryption context the maintenance statement also uses
  table_storage_arn_patterns = ["arn:${local.partition}:s3tables:${local.region}:${local.account_id}:bucket/${local.name_prefix}-*/*"]
}

# KMS Key Policy - named administrators, a read-only account root, and SNS service access, plus
# the statements S3 Tables, Lake Formation, and Athena need
data "aws_iam_policy_document" "kms_key_policy" {
  count = var.enabled && var.encryption_config.create_kms_key ? 1 : 0

  source_policy_documents = [
    data.aws_iam_policy_document.required_kms_statements[0].json,
    module.key_policy_statements[0].administration_json,
  ]

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

  source_policy_documents = compact([module.key_policy_statements[0].direct_access_json])

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

  # CloudWatch alarms publish to the alarm topic, which this key encrypts
  statement {
    sid    = "AllowCloudWatchAlarms"
    effect = "Allow"
    principals {
      type        = "Service"
      identifiers = ["cloudwatch.amazonaws.com"]
    }
    actions = [
      "kms:Decrypt",
      "kms:GenerateDataKey*",
    ]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "aws:SourceAccount"
      values   = [local.account_id]
    }
  }

  # Athena query results through S3, in the platform's S3 buckets only: kms:ViaService alone would
  # also cover S3-path decrypts of table storage for any principal with IAM key permissions. Table
  # storage carries an s3tables encryption context, so it never matches. Buckets named with the
  # platform prefix count as platform buckets, as in the maintenance statement, which keeps
  # workgroups the consumer builds on the platform key working (sandbox run b4e4b9).
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
      values   = ["s3.${local.region}.amazonaws.com"]
    }
    # The bucket ARN with S3 Bucket Keys (enabled), the object ARN without
    condition {
      test     = "StringLike"
      variable = "kms:EncryptionContext:aws:s3:arn"
      values   = ["arn:${local.partition}:s3:::${local.name_prefix}-*"]
    }
  }

  # Athena's own key use for query results, which carries no S3 encryption context. Table data
  # is never decrypted through Athena directly, only through S3 with Lake Formation credentials.
  statement {
    sid    = "AllowAthenaService"
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
      values   = ["athena.${local.region}.amazonaws.com"]
    }
  }
}
