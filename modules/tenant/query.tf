# ----
# Designated tenant results bucket (FR-028)
# ----

resource "aws_s3_bucket" "results" {
  # checkov:skip=CKV_AWS_21:Query results are transient and must actually be deleted by the lifecycle rule; versioning would retain them (plan.md, Data Storage)
  # checkov:skip=CKV_AWS_18:Access is audited through CloudTrail (FR-020); the module has no log bucket to deliver to
  # checkov:skip=CKV2_AWS_62:Nothing consumes notifications for transient query results
  count = local.designated ? 1 : 0

  bucket        = local.results_bucket_name
  force_destroy = local.effective.force_destroy

  tags = local.data_tags
}

resource "aws_s3_bucket_public_access_block" "results" {
  count = local.designated ? 1 : 0

  bucket                  = aws_s3_bucket.results[0].id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_ownership_controls" "results" {
  count = local.designated ? 1 : 0

  bucket = aws_s3_bucket.results[0].id
  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "results" {
  count = local.designated ? 1 : 0

  bucket = aws_s3_bucket.results[0].id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm     = "aws:kms"
      kms_master_key_id = aws_kms_key.this[0].arn
    }
    bucket_key_enabled = true
  }
}

# Results expire instead of tiering (FR-005); versioning stays off so they are actually deleted
resource "aws_s3_bucket_lifecycle_configuration" "results" {
  count = local.designated ? 1 : 0

  bucket = aws_s3_bucket.results[0].id

  rule {
    id     = "expire-query-results"
    status = "Enabled"
    filter {}
    expiration {
      days = local.effective.query_results_expiration_days
    }
    abort_incomplete_multipart_upload {
      days_after_initiation = 1
    }
  }
}

resource "aws_s3_bucket_policy" "results" {
  count = local.designated ? 1 : 0

  bucket = aws_s3_bucket.results[0].id
  policy = data.aws_iam_policy_document.results[0].json

  depends_on = [aws_s3_bucket_public_access_block.results]
}

data "aws_iam_policy_document" "results" {
  count = local.designated ? 1 : 0

  statement {
    sid    = "DenyInsecureTransport"
    effect = "Deny"
    principals {
      type        = "*"
      identifiers = ["*"]
    }
    actions = ["s3:*"]
    resources = [
      aws_s3_bucket.results[0].arn,
      "${aws_s3_bucket.results[0].arn}/*",
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

  # Uploads without encryption headers get the bucket default key, which a workgroup without a
  # result encryption setting needs (awslkhs-cdq). These statements deny only explicit headers that
  # name something other than the key. "aws:kms" without a key ID would use the AWS managed
  # aws/s3 key, not the bucket default, so it is denied as well. SSE-C uploads send their own
  # headers instead and would skip the key, so any SSE-C header is denied.
  statement {
    sid    = "DenySseC"
    effect = "Deny"
    principals {
      type        = "*"
      identifiers = ["*"]
    }
    actions   = ["s3:PutObject"]
    resources = ["${aws_s3_bucket.results[0].arn}/*"]
    condition {
      test     = "Null"
      variable = "s3:x-amz-server-side-encryption-customer-algorithm"
      values   = ["false"]
    }
  }

  statement {
    sid    = "DenyNonKmsEncryption"
    effect = "Deny"
    principals {
      type        = "*"
      identifiers = ["*"]
    }
    actions   = ["s3:PutObject"]
    resources = ["${aws_s3_bucket.results[0].arn}/*"]
    condition {
      test     = "Null"
      variable = "s3:x-amz-server-side-encryption"
      values   = ["false"]
    }
    condition {
      test     = "StringNotEquals"
      variable = "s3:x-amz-server-side-encryption"
      values   = ["aws:kms"]
    }
  }

  statement {
    sid    = "DenyAwsManagedKey"
    effect = "Deny"
    principals {
      type        = "*"
      identifiers = ["*"]
    }
    actions   = ["s3:PutObject"]
    resources = ["${aws_s3_bucket.results[0].arn}/*"]
    condition {
      test     = "StringEquals"
      variable = "s3:x-amz-server-side-encryption"
      values   = ["aws:kms"]
    }
    condition {
      test     = "Null"
      variable = "s3:x-amz-server-side-encryption-aws-kms-key-id"
      values   = ["true"]
    }
  }

  statement {
    sid    = "DenyOtherKmsKeys"
    effect = "Deny"
    principals {
      type        = "*"
      identifiers = ["*"]
    }
    actions   = ["s3:PutObject"]
    resources = ["${aws_s3_bucket.results[0].arn}/*"]
    condition {
      test     = "Null"
      variable = "s3:x-amz-server-side-encryption-aws-kms-key-id"
      values   = ["false"]
    }
    condition {
      test     = "StringNotEquals"
      variable = "s3:x-amz-server-side-encryption-aws-kms-key-id"
      values   = [aws_kms_key.this[0].arn]
    }
  }
}

# ----
# Tenant workgroup (FR-015 to FR-017)
# ----

resource "aws_athena_workgroup" "this" {
  name        = "${local.name_prefix}-${var.name}"
  description = "Query workgroup for tenant ${var.name}"
  # Always true: Athena refuses to delete a workgroup that has run queries otherwise, which blocks
  # tenant offboarding. The workgroup holds only query history; results live in the results bucket,
  # which keeps the environment's force_destroy (found in the T076 sandbox).
  force_destroy = true

  configuration {
    enforce_workgroup_configuration    = true
    bytes_scanned_cutoff_per_query     = local.bytes_scanned_cutoff_per_query
    publish_cloudwatch_metrics_enabled = local.effective.monitoring_enabled

    engine_version {
      selected_engine_version = "Athena engine version 3"
    }

    result_configuration {
      output_location       = "s3://${local.results.bucket_name}/${local.results.prefix}"
      expected_bucket_owner = local.account_id

      encryption_configuration {
        encryption_option = "SSE_KMS"
        kms_key_arn       = local.kms_key_arn
      }
    }
  }

  tags = local.tags
}

# Athena applies a workgroup result encryption setting to S3 Tables writes as well, and every
# INSERT, UPDATE, DELETE, and MERGE then fails with INTERNAL_ERROR_QUERY_ENGINE (awslkhs-cdq).
# Writers that run DML through Athena use this opt-in workgroup, which has no result encryption
# setting. Its results still use the tenant's KMS key: the results bucket encrypts header-less
# uploads with it by default, and its policy denies any other key.
resource "aws_athena_workgroup" "dml" {
  # checkov:skip=CKV_AWS_159:A result encryption setting breaks Athena DML into S3 Tables (awslkhs-cdq); the results bucket encrypts with the tenant's KMS key by default and denies other keys
  count = local.dml_workgroup_enabled ? 1 : 0

  name          = "${local.name_prefix}-${var.name}-dml"
  description   = "DML workgroup for tenant ${var.name}"
  force_destroy = true # same reason as the tenant workgroup

  configuration {
    enforce_workgroup_configuration    = true
    bytes_scanned_cutoff_per_query     = local.bytes_scanned_cutoff_per_query
    publish_cloudwatch_metrics_enabled = local.effective.monitoring_enabled

    engine_version {
      selected_engine_version = "Athena engine version 3"
    }

    result_configuration {
      output_location       = "s3://${local.results.bucket_name}/${local.results.prefix}dml/"
      expected_bucket_owner = local.account_id
    }
  }

  tags = local.tags
}
