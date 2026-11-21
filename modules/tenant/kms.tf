# ----
# Tenant key (designated tenants only, FR-028)
# ----

resource "aws_kms_key" "this" {
  count = local.designated ? 1 : 0

  description             = "Tenant key for ${local.name_prefix} tenant ${var.name}"
  deletion_window_in_days = local.effective.kms_key_deletion_window_days
  enable_key_rotation     = true
  policy                  = data.aws_iam_policy_document.kms[0].json

  # Common tags only, never data tags
  tags = merge(local.tags, {
    Name = "${local.name_prefix}-${var.name}-cmk"
  })
}

resource "aws_kms_alias" "this" {
  count = local.designated ? 1 : 0

  name          = "alias/${local.name_prefix}-${var.name}-cmk"
  target_key_id = aws_kms_key.this[0].key_id
}

# The same statements as the platform key, with maintenance limited to this tenant's buckets
data "aws_iam_policy_document" "kms" {
  count = local.designated ? 1 : 0

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
      values = [
        for name in values(local.dedicated_bucket_names) :
        "arn:${local.partition}:s3tables:${local.region}:${local.account_id}:bucket/${name}/*"
      ]
    }
  }

  statement {
    sid    = "AllowLakeFormationDataAccess"
    effect = "Allow"
    principals {
      type        = "AWS"
      identifiers = [var.platform.data_access_role_arn]
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
    for_each = length(var.platform.direct_access.exempt_principal_arns) > 0 ? [1] : []
    content {
      sid    = "AllowDirectAccessExemptions"
      effect = "Allow"
      principals {
        type        = "AWS"
        identifiers = var.platform.direct_access.exempt_principal_arns
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
