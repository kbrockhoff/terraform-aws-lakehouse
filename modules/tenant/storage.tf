# ----
# Dedicated table buckets (dedicated or designated tenants, FR-002, FR-028)
# ----

resource "aws_s3tables_table_bucket" "dedicated" {
  for_each = local.dedicated ? local.zones : toset([])

  name = local.dedicated_bucket_names[each.key]

  encryption_configuration = {
    sse_algorithm = "aws:kms"
    kms_key_arn   = local.kms_key_arn
  }

  maintenance_configuration = {
    iceberg_unreferenced_file_removal = {
      status = "enabled"
      settings = {
        unreferenced_days = local.effective.unreferenced_days
        non_current_days  = local.effective.non_current_days
      }
    }
  }

  # The effective table_storage_class is not set: provider 6.x has no table bucket argument for
  # the default storage class of new tables (plan.md, Complexity Tracking).

  force_destroy = local.effective.force_destroy

  tags = local.data_tags
}

resource "aws_s3tables_table_bucket_policy" "dedicated" {
  for_each = aws_s3tables_table_bucket.dedicated

  table_bucket_arn = each.value.arn
  resource_policy  = data.aws_iam_policy_document.dedicated_bucket[each.key].json
}

# Same deny-only policy as the shared zone buckets, keyed to this tenant's key. IfExists lets
# CreateTable calls that state no encryption inherit the bucket default key.
data "aws_iam_policy_document" "dedicated_bucket" {
  for_each = aws_s3tables_table_bucket.dedicated

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

# ----
# Tenant namespaces: the Lake Formation database <tenant> in each zone's bucket catalog
# ----

resource "aws_s3tables_namespace" "this" {
  for_each = local.zones

  namespace        = var.name
  table_bucket_arn = local.bucket_arn[each.key]
}
