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

  lifecycle {
    precondition {
      condition     = length(local.key_administrator_arns) > 0
      error_message = "Tenant ${var.name}: the platform contract names no key administrators, so the tenant key would have none. Set encryption_config.key_administrator_arns on the platform, apply it, and republish its platform_contract."
    }

    precondition {
      condition     = contains(local.key_administrator_arns, data.aws_iam_session_context.current.issuer_arn)
      error_message = "Tenant ${var.name}: the applying role (${data.aws_iam_session_context.current.issuer_arn}) is not one of the platform's key administrators, so it could not manage the tenant key. Add it to the platform's encryption_config.key_administrator_arns and apply the platform first."
    }
  }
}

resource "aws_kms_alias" "this" {
  count = local.designated ? 1 : 0

  name          = "alias/${local.name_prefix}-${var.name}-cmk"
  target_key_id = aws_kms_key.this[0].key_id
}

module "key_policy_statements" {
  source = "../key-policy-statements"
  count  = local.designated ? 1 : 0

  account_id                          = local.account_id
  partition                           = local.partition
  region                              = local.region
  key_administrator_arns              = local.key_administrator_arns
  direct_access_exempt_principal_arns = var.platform.direct_access.exempt_principal_arns
  # This tenant's dedicated buckets, which its key encrypts
  table_storage_arn_patterns = [
    for name in values(local.dedicated_bucket_names) :
    "arn:${local.partition}:s3tables:${local.region}:${local.account_id}:bucket/${name}/*"
  ]
}

# The same statements as the platform key, with maintenance limited to this tenant's buckets
data "aws_iam_policy_document" "kms" {
  count = local.designated ? 1 : 0

  source_policy_documents = compact([
    module.key_policy_statements[0].administration_json,
    module.key_policy_statements[0].direct_access_json,
  ])

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
