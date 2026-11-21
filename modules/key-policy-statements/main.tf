# Key policy statements every module-created key shares (platform, catalog metadata, and
# designated-tenant keys), so the three policies cannot drift apart.

# Named key administrators manage the key; the account root only reads it
data "aws_iam_policy_document" "administration" {
  statement {
    sid    = "KeyAdministrators"
    effect = "Allow"
    principals {
      type        = "AWS"
      identifiers = sort(distinct(var.key_administrator_arns))
    }
    # Key management only: no Encrypt, Decrypt, GenerateDataKey, or CreateGrant. PutKeyPolicy still
    # lets an administrator rewrite the policy, including granting itself key use, so list only
    # trusted policy editors.
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

  # Break-glass principals keep their path to retire a protected key, which a deny elsewhere in
  # the policy exempts exactly them from. Delegated to the account with an exact principal ARN
  # condition, not named as principals: KMS rejects a policy naming a role that no longer exists,
  # so a retired break-glass role would otherwise block every policy update. They need the same
  # actions in their IAM policy.
  dynamic "statement" {
    for_each = length(var.key_deletion_principal_arns) > 0 ? [1] : []
    content {
      sid    = "KeyDeletionBreakGlass"
      effect = "Allow"
      principals {
        type        = "AWS"
        identifiers = ["arn:${var.partition}:iam::${var.account_id}:root"]
      }
      actions = [
        "kms:CancelKeyDeletion",
        "kms:DescribeKey",
        "kms:DisableKey",
        "kms:EnableKey",
        "kms:ScheduleKeyDeletion",
      ]
      resources = ["*"]
      condition {
        test     = "ArnEquals"
        variable = "aws:PrincipalArn"
        values   = sort(distinct(var.key_deletion_principal_arns))
      }
      condition {
        test     = "StringEquals"
        variable = "aws:PrincipalAccount"
        values   = [var.account_id]
      }
    }
  }
}

# Principals exempt from the table-bucket direct-access deny read table storage themselves, and
# the account root does not delegate key use to IAM. Scoped to the table storage the exemption
# covers (the S3 Tables encryption context; kms:ViaService alone doesn't identify the bucket), and
# delegated with an exact principal ARN condition so a retired exempt role never invalidates the
# policy. Same-account principals only; they need kms:Decrypt and kms:GenerateDataKey in IAM too.
data "aws_iam_policy_document" "direct_access" {
  count = length(var.direct_access_exempt_principal_arns) > 0 ? 1 : 0

  statement {
    sid    = "AllowDirectAccessExemptions"
    effect = "Allow"
    principals {
      type        = "AWS"
      identifiers = ["arn:${var.partition}:iam::${var.account_id}:root"]
    }
    actions = [
      "kms:Decrypt",
      "kms:GenerateDataKey",
    ]
    resources = ["*"]
    condition {
      test     = "ArnEquals"
      variable = "aws:PrincipalArn"
      values   = sort(distinct(var.direct_access_exempt_principal_arns))
    }
    condition {
      test     = "StringEquals"
      variable = "aws:PrincipalAccount"
      values   = [var.account_id]
    }
    condition {
      test     = "StringEquals"
      variable = "kms:ViaService"
      values = [
        "s3tables.${var.region}.amazonaws.com",
        "s3.${var.region}.amazonaws.com",
      ]
    }
    condition {
      test     = "StringLike"
      variable = "kms:EncryptionContext:aws:s3:arn"
      values   = var.table_storage_arn_patterns
    }
  }
}
