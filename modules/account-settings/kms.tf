# ----
# Dedicated catalog-metadata key (FR-009)
# ----

# Separate from the platform and tenant keys, which encrypt table storage and query results. Glue
# catalog encryption covers objects stored in the catalog itself (databases, tables, resource
# links, connections), not the S3 Tables metadata behind s3tablescatalog.
resource "aws_kms_key" "catalog" {
  count = var.catalog_encryption.kms_key_arn == "" ? 1 : 0

  description             = "Catalog metadata key for ${var.name_prefix}"
  deletion_window_in_days = var.kms_key_deletion_window_days
  enable_key_rotation     = true

  policy = data.aws_iam_policy_document.catalog_key_policy.json

  tags = merge(var.tags, {
    Name = "${var.name_prefix}-catalog-cmk"
  })

  lifecycle {
    precondition {
      condition     = length(local.catalog_key_users) <= 200
      error_message = "Per-caller catalog encryption names ${length(local.catalog_key_users)} principals in the key policy; at most 200 fit within the 32 KB key-policy limit. Use key_access_mode = \"service_role\"."
    }
  }

  depends_on = [terraform_data.catalog_ready]
}

resource "aws_kms_alias" "catalog" {
  count = var.catalog_encryption.kms_key_arn == "" ? 1 : 0

  name          = "alias/${var.name_prefix}-catalog-cmk"
  target_key_id = aws_kms_key.catalog[0].key_id
}

locals {
  catalog_key_arn = var.catalog_encryption.kms_key_arn != "" ? var.catalog_encryption.kms_key_arn : aws_kms_key.catalog[0].arn

  # Per-caller mode: every principal the module knows reads or writes the catalog
  catalog_key_users = local.service_role_mode ? [] : sort(distinct(concat(
    var.admin_principal_arns,
    var.read_only_admin_principal_arns,
    var.key_user_arns,
    var.catalog_encryption.additional_key_user_arns,
  )))
}

data "aws_iam_policy_document" "catalog_key_policy" {
  source_policy_documents = [data.aws_iam_policy_document.required_catalog_key_statements.json]

  # Named key administrators manage the key and keep it out of lockout; the account root only
  # reads it
  statement {
    sid    = "KeyAdministrators"
    effect = "Allow"
    principals {
      type        = "AWS"
      identifiers = var.key_administrator_arns
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
      values   = [var.account_id]
    }
  }

  # The rest of the account may only inspect the key
  statement {
    sid    = "AccountReadOnly"
    effect = "Allow"
    principals {
      type        = "AWS"
      identifiers = ["arn:${var.partition}:iam::${var.account_id}:root"]
    }
    actions   = ["kms:Describe*", "kms:Get*", "kms:List*"]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "aws:PrincipalAccount"
      values   = [var.account_id]
    }
  }
}

# Statements a consumer-supplied catalog key also needs (required_catalog_kms_key_policy_json)
data "aws_iam_policy_document" "required_catalog_key_statements" {
  # Glue creates a grant scoped to this account's catalog to encrypt and re-encrypt metadata
  statement {
    sid    = "AllowGlueCatalogEncryption"
    effect = "Allow"
    principals {
      type        = "AWS"
      identifiers = ["arn:${var.partition}:iam::${var.account_id}:root"]
    }
    actions   = ["kms:CreateGrant"]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "kms:CallerAccount"
      values   = [var.account_id]
    }
    condition {
      test     = "Bool"
      variable = "kms:GrantIsForAWSResource"
      values   = ["true"]
    }
    condition {
      test     = "StringEquals"
      variable = "kms:EncryptionContext:glue_catalog_id"
      values   = [var.account_id]
    }
  }

  dynamic "statement" {
    for_each = local.service_role_mode ? [1] : []
    content {
      sid    = "AllowCatalogEncryptionRole"
      effect = "Allow"
      principals {
        type        = "AWS"
        identifiers = [aws_iam_role.catalog_encryption[0].arn]
      }
      actions = [
        "kms:Decrypt",
        "kms:DescribeKey",
        "kms:Encrypt",
        "kms:GenerateDataKey",
      ]
      resources = ["*"]
    }
  }

  dynamic "statement" {
    for_each = length(local.catalog_key_users) > 0 ? [1] : []
    content {
      sid    = "AllowCatalogKeyUsers"
      effect = "Allow"
      principals {
        type        = "AWS"
        identifiers = local.catalog_key_users
      }
      actions = [
        "kms:CreateGrant",
        "kms:Decrypt",
        "kms:DescribeKey",
        "kms:Encrypt",
        "kms:GenerateDataKey",
      ]
      resources = ["*"]
      condition {
        test     = "StringEquals"
        variable = "kms:EncryptionContext:glue_catalog_id"
        values   = [var.account_id]
      }
    }
  }

  # Catalog objects stay encrypted with this key after encryption is disabled, so the key must
  # outlive every deployment (FR-013). PutKeyPolicy stays allowed for recovery. Ephemeral owners
  # supply a long-lived key instead of creating this one.
  statement {
    sid       = "ProtectCatalogKey"
    effect    = "Deny"
    actions   = ["kms:ScheduleKeyDeletion", "kms:DisableKey"]
    resources = ["*"]
    principals {
      type        = "AWS"
      identifiers = ["*"]
    }
    dynamic "condition" {
      for_each = length(var.catalog_encryption.key_deletion_principal_arns) > 0 ? [1] : []
      content {
        test     = "ArnNotEquals"
        variable = "aws:PrincipalArn"
        values   = var.catalog_encryption.key_deletion_principal_arns
      }
    }
  }
}
