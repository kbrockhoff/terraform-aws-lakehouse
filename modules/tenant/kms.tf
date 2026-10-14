# ----
# Tenant key (designated tenants only, FR-028)
# ----

resource "aws_kms_key" "this" {
  count = local.designated ? 1 : 0

  description             = "Tenant key for ${local.name_prefix} tenant ${var.name}"
  deletion_window_in_days = local.effective.kms_key_deletion_window_days
  enable_key_rotation     = true
  policy                  = data.aws_iam_policy_document.kms[0].json

  tags = merge(local.data_tags, {
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
      values   = ["s3tables.${local.region}.amazonaws.com"]
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
