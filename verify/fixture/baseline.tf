# ----
# Named-resource baseline (FR-007): its own table bucket, namespaces, workgroup, and results
# ----

# Nothing in this catalog carries LF-tags, so only the named grants in grants-named.tf decide
# access. The platform key's S3 Tables maintenance statement covers bucket names starting with
# the fixture prefix.
resource "aws_s3tables_table_bucket" "named" {
  name = "${local.prefix}-named"

  encryption_configuration = {
    sse_algorithm = "aws:kms"
    kms_key_arn   = module.lakehouse.kms_key_arn
  }

  force_destroy = true
}

data "aws_iam_policy_document" "named_bucket" {
  statement {
    sid    = "DenyInsecureTransport"
    effect = "Deny"
    principals {
      type        = "*"
      identifiers = ["*"]
    }
    actions = ["s3tables:*"]
    resources = [
      aws_s3tables_table_bucket.named.arn,
      "${aws_s3tables_table_bucket.named.arn}/*",
    ]
    condition {
      test     = "Bool"
      variable = "aws:SecureTransport"
      values   = ["false"]
    }
  }
}

resource "aws_s3tables_table_bucket_policy" "named" {
  table_bucket_arn = aws_s3tables_table_bucket.named.arn
  resource_policy  = data.aws_iam_policy_document.named_bucket.json
}

resource "aws_s3tables_namespace" "named" {
  for_each = toset(local.named_namespaces)

  namespace        = each.key
  table_bucket_arn = aws_s3tables_table_bucket.named.arn
}

locals {
  named_catalog_id = "${local.account_id}:s3tablescatalog/${aws_s3tables_table_bucket.named.name}"
}

# ----
# Baseline query results
# ----

resource "aws_s3_bucket" "named_results" {
  #checkov:skip=CKV_AWS_18:Disposable fixture bucket; results expire after a day and access is audited by CloudTrail
  #checkov:skip=CKV_AWS_144:Disposable fixture bucket; cross-region replication adds nothing
  #checkov:skip=CKV2_AWS_62:No consumer of event notifications for fixture results
  #checkov:skip=CKV_AWS_21:Results are transient and expire after a day; versioning would only retain them
  bucket        = "${local.prefix}-named-results-${local.account_id}"
  force_destroy = true
}

resource "aws_s3_bucket_public_access_block" "named_results" {
  bucket = aws_s3_bucket.named_results.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_ownership_controls" "named_results" {
  bucket = aws_s3_bucket.named_results.id

  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "named_results" {
  bucket = aws_s3_bucket.named_results.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm     = "aws:kms"
      kms_master_key_id = module.lakehouse.kms_key_arn
    }
    bucket_key_enabled = true
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "named_results" {
  bucket = aws_s3_bucket.named_results.id

  rule {
    id     = "expire-results"
    status = "Enabled"

    filter {}

    expiration {
      days = 1
    }

    abort_incomplete_multipart_upload {
      days_after_initiation = 1
    }
  }
}

data "aws_iam_policy_document" "named_results" {
  statement {
    sid    = "DenyInsecureTransport"
    effect = "Deny"
    principals {
      type        = "*"
      identifiers = ["*"]
    }
    actions = ["s3:*"]
    resources = [
      aws_s3_bucket.named_results.arn,
      "${aws_s3_bucket.named_results.arn}/*",
    ]
    condition {
      test     = "Bool"
      variable = "aws:SecureTransport"
      values   = ["false"]
    }
  }
}

resource "aws_s3_bucket_policy" "named_results" {
  bucket = aws_s3_bucket.named_results.id
  policy = data.aws_iam_policy_document.named_results.json

  depends_on = [aws_s3_bucket_public_access_block.named_results]
}

resource "aws_athena_workgroup" "named" {
  name          = "${local.prefix}-named"
  force_destroy = true

  configuration {
    enforce_workgroup_configuration    = true
    publish_cloudwatch_metrics_enabled = false
    bytes_scanned_cutoff_per_query     = 1073741824

    engine_version {
      selected_engine_version = "Athena engine version 3"
    }

    result_configuration {
      output_location = "s3://${aws_s3_bucket.named_results.bucket}/"

      encryption_configuration {
        encryption_option = "SSE_KMS"
        kms_key_arn       = module.lakehouse.kms_key_arn
      }
    }
  }
}

# Every persona reaches the baseline workgroup and its own results prefix (T020); Lake Formation
# decides what the queries return
data "aws_iam_policy_document" "named_access" {
  for_each = local.personas

  statement {
    sid    = "BaselineWorkgroup"
    effect = "Allow"
    actions = [
      "athena:StartQueryExecution",
      "athena:GetQueryExecution",
      "athena:GetQueryResults",
      "athena:StopQueryExecution",
      "athena:GetWorkGroup",
    ]
    resources = [aws_athena_workgroup.named.arn]
  }

  statement {
    sid       = "BaselineResults"
    effect    = "Allow"
    actions   = ["s3:GetObject", "s3:PutObject"]
    resources = ["${aws_s3_bucket.named_results.arn}/*"]
  }

  statement {
    sid       = "BaselineResultsBucket"
    effect    = "Allow"
    actions   = ["s3:ListBucket", "s3:GetBucketLocation"]
    resources = [aws_s3_bucket.named_results.arn]
  }

  statement {
    sid       = "BaselineResultsKey"
    effect    = "Allow"
    actions   = ["kms:Decrypt", "kms:GenerateDataKey"]
    resources = [module.lakehouse.kms_key_arn]
  }

  statement {
    sid       = "BaselineCatalogRead"
    effect    = "Allow"
    actions   = ["glue:GetCatalog", "glue:GetCatalogs", "glue:GetDatabase", "glue:GetDatabases", "glue:GetTable", "glue:GetTables"]
    resources = local.glue_arns
  }

  statement {
    sid       = "BaselineDataAccess"
    effect    = "Allow"
    actions   = ["lakeformation:GetDataAccess"]
    resources = ["*"] # Lake Formation has no resource-level permissions
  }
}

resource "aws_iam_role_policy" "named_access" {
  for_each = local.personas

  name   = "named-baseline"
  role   = aws_iam_role.persona[each.key].id
  policy = data.aws_iam_policy_document.named_access[each.key].json
}
