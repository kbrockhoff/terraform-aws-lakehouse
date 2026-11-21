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

# KMS Key Policy - allows account root and SNS service access, plus the statements S3 Tables,
# Lake Formation, and Athena need
data "aws_iam_policy_document" "kms_key_policy" {
  count = var.enabled && var.encryption_config.create_kms_key ? 1 : 0

  source_policy_documents = [data.aws_iam_policy_document.required_kms_statements[0].json]

  # Named key administrators manage the key; the account root only reads it
  statement {
    sid    = "KeyAdministrators"
    effect = "Allow"
    principals {
      type        = "AWS"
      identifiers = local.key_administrator_arns
    }
    # Key management only: no Encrypt, Decrypt, GenerateDataKey, or CreateGrant, so administrators
    # cannot read data or grant themselves use
    actions = [
      "kms:CancelKeyDeletion",
      "kms:CreateAlias",
      "kms:DeleteAlias",
      "kms:DescribeKey",
      "kms:DisableKey",
      "kms:DisableKeyRotation",
      "kms:EnableKey",
      "kms:EnableKeyRotation",
      "kms:GetKeyPolicy",
      "kms:GetKeyRotationStatus",
      "kms:ListGrants",
      "kms:ListKeyPolicies",
      "kms:ListKeyRotations",
      "kms:ListResourceTags",
      "kms:PutKeyPolicy",
      "kms:RetireGrant",
      "kms:RevokeGrant",
      "kms:RotateKeyOnDemand",
      "kms:ScheduleKeyDeletion",
      "kms:TagResource",
      "kms:UntagResource",
      "kms:UpdateAlias",
      "kms:UpdateKeyDescription",
    ]
    resources = ["*"]
    # Administrators must belong to this account, even if a listed ARN names another
    condition {
      test     = "StringEquals"
      variable = "aws:PrincipalAccount"
      values   = [local.account_id]
    }
  }

  # The rest of the account may only inspect the key
  statement {
    sid    = "AccountReadOnly"
    effect = "Allow"
    principals {
      type        = "AWS"
      identifiers = ["arn:${local.partition}:iam::${local.account_id}:root"]
    }
    actions   = ["kms:Describe*", "kms:Get*", "kms:List*"]
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

  # Principals exempt from the table-bucket direct-access deny read table storage themselves, and
  # the account root no longer delegates key use to IAM
  dynamic "statement" {
    for_each = length(var.governance_config.direct_access_exempt_principal_arns) > 0 ? [1] : []
    content {
      sid    = "AllowDirectAccessExemptions"
      effect = "Allow"
      principals {
        type        = "AWS"
        identifiers = var.governance_config.direct_access_exempt_principal_arns
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
          "s3.${local.region}.amazonaws.com",
        ]
      }
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
