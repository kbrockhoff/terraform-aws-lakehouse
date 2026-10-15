# ----
# Shared zone table buckets (FR-004, FR-005)
# ----

# Created for every zone even before a tenant uses it: an empty bucket has no idle cost, and
# bucket addresses stay stable as tenants come and go.
resource "aws_s3tables_table_bucket" "zone" {
  for_each = var.enabled ? local.zones : toset([])

  name = local.zone_bucket_names[each.key]

  encryption_configuration = {
    sse_algorithm = "aws:kms"
    kms_key_arn   = local.kms_key_arn
  }

  maintenance_configuration = {
    iceberg_unreferenced_file_removal = {
      status = "enabled"
      settings = {
        unreferenced_days = local.effective_config.unreferenced_days
        non_current_days  = local.effective_config.non_current_days
      }
    }
  }

  # The effective table_storage_class is not set here: provider 6.x has no table bucket argument
  # for the default storage class of new tables (plan.md, Complexity Tracking).

  force_destroy = local.effective_config.force_destroy

  tags = merge(local.common_data_tags, {
    (local.data_classification_key) = local.shared_storage_classification
  })

  depends_on = [terraform_data.required_tags_guard]
}

resource "aws_s3tables_table_bucket_policy" "zone" {
  for_each = aws_s3tables_table_bucket.zone

  table_bucket_arn = each.value.arn
  resource_policy  = data.aws_iam_policy_document.zone_bucket[each.key].json
}

# Grants nothing; Lake Formation grants are the only access path. The CreateTable denies use
# IfExists because a CreateTable call without encryption settings inherits the bucket default,
# which is the platform key. That inheritance is also why designated tenants, whose tables must
# use the tenant key, need dedicated buckets instead of a namespace in a shared bucket.
data "aws_iam_policy_document" "zone_bucket" {
  for_each = aws_s3tables_table_bucket.zone

  statement {
    sid    = "DenyInsecureTransport"
    effect = "Deny"
    principals {
      type        = "*"
      identifiers = ["*"]
    }
    actions = ["s3tables:*"]
    resources = [
      each.value.arn,
      "${each.value.arn}/*",
    ]
    condition {
      test     = "Bool"
      variable = "aws:SecureTransport"
      values   = ["false"]
    }
    condition {
      test     = "Bool"
      variable = "aws:PrincipalIsAWSService"
      values   = ["false"]
    }
  }

  statement {
    sid    = "EnforceKMSEncryptionAlgorithm"
    effect = "Deny"
    principals {
      type        = "*"
      identifiers = ["*"]
    }
    actions = ["s3tables:CreateTable"]
    resources = [
      each.value.arn,
      "${each.value.arn}/*",
    ]
    condition {
      test     = "StringNotEqualsIfExists"
      variable = "s3tables:SSEAlgorithm"
      values   = ["aws:kms"]
    }
  }

  statement {
    sid    = "EnforceKMSEncryptionKey"
    effect = "Deny"
    principals {
      type        = "*"
      identifiers = ["*"]
    }
    actions = ["s3tables:CreateTable"]
    resources = [
      each.value.arn,
      "${each.value.arn}/*",
    ]
    condition {
      test     = "StringNotEqualsIfExists"
      variable = "s3tables:KMSKeyArn"
      values   = [local.kms_key_arn]
    }
  }
}
