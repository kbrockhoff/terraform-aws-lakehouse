# ----
# Shared Athena query results (FR-016)
# ----

# Results expire instead of tiering to a cheaper storage class (FR-005): they are transient and
# must actually be deleted, so versioning is also off.
resource "aws_s3_bucket" "query_results" {
  count = var.enabled ? 1 : 0

  bucket        = "${local.name_prefix}-athena-results-${local.account_id}"
  force_destroy = local.effective_config.force_destroy

  tags = merge(local.common_data_tags, {
    (local.data_classification_key) = local.shared_storage_classification
  })

  depends_on = [terraform_data.required_tags_guard]
}

resource "aws_s3_bucket_public_access_block" "query_results" {
  count = var.enabled ? 1 : 0

  bucket                  = aws_s3_bucket.query_results[0].id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_ownership_controls" "query_results" {
  count = var.enabled ? 1 : 0

  bucket = aws_s3_bucket.query_results[0].id
  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "query_results" {
  count = var.enabled ? 1 : 0

  bucket = aws_s3_bucket.query_results[0].id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm     = "aws:kms"
      kms_master_key_id = local.kms_key_arn
    }
    bucket_key_enabled = true
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "query_results" {
  count = var.enabled ? 1 : 0

  bucket = aws_s3_bucket.query_results[0].id

  rule {
    id     = "expire-query-results"
    status = "Enabled"
    filter {}
    expiration {
      days = local.effective_config.query_results_expiration_days
    }
    abort_incomplete_multipart_upload {
      days_after_initiation = 1
    }
  }
}

resource "aws_s3_bucket_policy" "query_results" {
  count = var.enabled ? 1 : 0

  bucket = aws_s3_bucket.query_results[0].id
  policy = data.aws_iam_policy_document.query_results[0].json

  depends_on = [aws_s3_bucket_public_access_block.query_results]
}

# No per-tenant statements: tenants are isolated by workgroup output prefix and IAM, so a
# standalone tenant never needs to edit this platform-owned policy.
data "aws_iam_policy_document" "query_results" {
  count = var.enabled ? 1 : 0

  statement {
    sid    = "DenyInsecureTransport"
    effect = "Deny"
    principals {
      type        = "*"
      identifiers = ["*"]
    }
    actions = ["s3:*"]
    resources = [
      aws_s3_bucket.query_results[0].arn,
      "${aws_s3_bucket.query_results[0].arn}/*",
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
    sid    = "DenyUnencryptedUploads"
    effect = "Deny"
    principals {
      type        = "*"
      identifiers = ["*"]
    }
    actions   = ["s3:PutObject"]
    resources = ["${aws_s3_bucket.query_results[0].arn}/*"]
    condition {
      test     = "StringNotEqualsIfExists"
      variable = "s3:x-amz-server-side-encryption"
      values   = ["aws:kms"]
    }
  }
}
