# ----
# Consumer account (spec 003 FR-020 cases 6 to 10)
# ----

# The producer shares tenant alpha's curated namespace (public and internal labels) with the
# consumer account. In the consumer, the account's own data-lake admin (the consumer_profile role)
# creates a resource link and grants a consumer persona by LF-tag expression. That re-grant works
# only because the share also grants DESCRIBE on the tags (share_tag_describe). Row-filtered tables
# (orders, at stage 2) are never shared.
#
# The fixture never changes the consumer's data-lake settings: PutDataLakeSettings replaces the
# whole object and would remove the account's other admins, such as SageMaker Unified Studio roles.

locals {
  consumer_enabled    = var.consumer_profile != ""
  consumer_account_id = local.consumer_enabled ? data.aws_caller_identity.consumer[0].account_id : ""
  consumer_link       = "${replace(local.prefix, "-", "_")}_alpha_link"
  consumer_share      = local.consumer_enabled ? module.lakehouse.tenants["alpha"].shares["consumer"] : null

  # Accounts full mode may deploy into (verify/sandbox-accounts.txt)
  sandbox_accounts = [
    for l in split("\n", file("${path.module}/../sandbox-accounts.txt")) : split(" ", trimspace(l))[0]
    if trimspace(l) != "" && !startswith(trimspace(l), "#")
  ]
}

data "aws_caller_identity" "consumer" {
  count    = local.consumer_enabled ? 1 : 0
  provider = aws.consumer
}

data "aws_iam_session_context" "consumer" {
  count    = local.consumer_enabled ? 1 : 0
  provider = aws.consumer
  arn      = data.aws_caller_identity.consumer[0].arn
}

data "aws_lakeformation_data_lake_settings" "consumer" {
  count    = local.consumer_enabled ? 1 : 0
  provider = aws.consumer
}

resource "terraform_data" "consumer_guard" {
  count = local.consumer_enabled ? 1 : 0

  lifecycle {
    precondition {
      condition     = contains(local.sandbox_accounts, local.consumer_account_id) && local.consumer_account_id != local.account_id
      error_message = "The consumer account ${local.consumer_account_id} must be another account listed in verify/sandbox-accounts.txt."
    }

    precondition {
      condition     = contains(data.aws_lakeformation_data_lake_settings.consumer[0].admins, data.aws_iam_session_context.consumer[0].issuer_arn)
      error_message = "The consumer role ${data.aws_iam_session_context.consumer[0].issuer_arn} must already be a Lake Formation data-lake admin in ${local.consumer_account_id}. The fixture never edits the consumer's settings."
    }
  }
}

# ----
# Consumer persona
# ----

data "aws_iam_policy_document" "consumer_trust" {
  statement {
    sid     = "RunnerOnly"
    effect  = "Allow"
    actions = ["sts:AssumeRole", "sts:TagSession"]

    principals {
      type        = "AWS"
      identifiers = [local.runner_role_arn]
    }

    condition {
      test     = "StringLike"
      variable = "sts:RoleSessionName"
      values   = ["lfverify-${var.run_id}-*"]
    }
  }
}

resource "aws_iam_role" "consumer" {
  count    = local.consumer_enabled ? 1 : 0
  provider = aws.consumer

  name                 = "${local.prefix}-consumer"
  description          = "lfverify consumer persona for run ${var.run_id}"
  assume_role_policy   = data.aws_iam_policy_document.consumer_trust.json
  max_session_duration = 3600

  depends_on = [terraform_data.consumer_guard]
}

# API reachability only; Lake Formation in both accounts decides every outcome. The direct table
# storage permission makes the storage bypass case meaningful: the producer's bucket must refuse it.
data "aws_iam_policy_document" "consumer" {
  count = local.consumer_enabled ? 1 : 0

  statement {
    sid    = "Query"
    effect = "Allow"
    actions = [
      "athena:StartQueryExecution",
      "athena:GetQueryExecution",
      "athena:GetQueryResults",
      "athena:StopQueryExecution",
      "athena:GetWorkGroup",
    ]
    resources = [aws_athena_workgroup.consumer[0].arn]
  }

  statement {
    sid       = "Catalog"
    effect    = "Allow"
    actions   = ["glue:GetCatalog", "glue:GetCatalogs", "glue:GetDatabase", "glue:GetDatabases", "glue:GetTable", "glue:GetTables", "glue:GetPartitions", "glue:CreateTable", "glue:DeleteTable", "lakeformation:GetDataAccess"]
    resources = ["*"] # Lake Formation decides; the write actions let the write cases reach it
  }

  statement {
    sid       = "Results"
    effect    = "Allow"
    actions   = ["s3:GetObject", "s3:PutObject", "s3:GetBucketLocation", "s3:ListBucket"]
    resources = [aws_s3_bucket.consumer_results[0].arn, "${aws_s3_bucket.consumer_results[0].arn}/*"]
  }

  statement {
    sid       = "DirectTableStorage"
    effect    = "Allow"
    actions   = ["s3tables:GetTableMetadataLocation", "s3tables:GetTable", "s3tables:GetTableData"]
    resources = [module.lakehouse.tenants["alpha"].table_bucket_arns["curated"], "${module.lakehouse.tenants["alpha"].table_bucket_arns["curated"]}/*"]
  }
}

resource "aws_iam_role_policy" "consumer" {
  count    = local.consumer_enabled ? 1 : 0
  provider = aws.consumer

  name   = "consumer"
  role   = aws_iam_role.consumer[0].id
  policy = data.aws_iam_policy_document.consumer[0].json
}

# ----
# Consumer query results
# ----

resource "aws_s3_bucket" "consumer_results" {
  count    = local.consumer_enabled ? 1 : 0
  provider = aws.consumer

  bucket        = "${local.prefix}-consumer-results-${local.consumer_account_id}"
  force_destroy = true

  depends_on = [terraform_data.consumer_guard]
}

resource "aws_s3_bucket_public_access_block" "consumer_results" {
  count    = local.consumer_enabled ? 1 : 0
  provider = aws.consumer

  bucket                  = aws_s3_bucket.consumer_results[0].id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "consumer_results" {
  count    = local.consumer_enabled ? 1 : 0
  provider = aws.consumer

  bucket = aws_s3_bucket.consumer_results[0].id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_athena_workgroup" "consumer" {
  count    = local.consumer_enabled ? 1 : 0
  provider = aws.consumer

  name          = "${local.prefix}-consumer"
  force_destroy = true

  configuration {
    enforce_workgroup_configuration = true
    result_configuration {
      output_location = "s3://${aws_s3_bucket.consumer_results[0].bucket}/"
      encryption_configuration {
        encryption_option = "SSE_S3"
      }
    }
  }
}

# ----
# Resource link and consumer grants (made by the consumer's data-lake admin)
# ----

resource "aws_glue_catalog_database" "consumer_link" {
  count    = local.consumer_enabled ? 1 : 0
  provider = aws.consumer

  name = local.consumer_link
  target_database {
    catalog_id    = local.consumer_share.catalog_ids["curated"]
    database_name = local.consumer_share.databases["curated"]
  }

  # A resource link reads back its target's parameters and federation, which are not the link's
  # own. Updating them needs ALTER on the producer's database, so they are ignored.
  lifecycle {
    ignore_changes = [parameters, federated_database]
  }

  depends_on = [terraform_data.consumer_guard]
}

resource "aws_lakeformation_permissions" "consumer_link" {
  count    = local.consumer_enabled ? 1 : 0
  provider = aws.consumer

  principal   = aws_iam_role.consumer[0].arn
  permissions = ["DESCRIBE"]
  database {
    name = aws_glue_catalog_database.consumer_link[0].name
  }
}

# Narrower than the share: tenant and public or internal only, in the producer's tags
resource "aws_lakeformation_permissions" "consumer_database" {
  count    = local.consumer_enabled ? 1 : 0
  provider = aws.consumer

  principal   = aws_iam_role.consumer[0].arn
  permissions = ["DESCRIBE"]
  lf_tag_policy {
    catalog_id    = local.account_id
    resource_type = "DATABASE"
    expression {
      key    = module.lakehouse.tenants["alpha"].tenant_tag_key
      values = ["member"]
    }
  }

  depends_on = [aws_glue_catalog_database.consumer_link]
}

resource "aws_lakeformation_permissions" "consumer_tables" {
  count    = local.consumer_enabled ? 1 : 0
  provider = aws.consumer

  principal   = aws_iam_role.consumer[0].arn
  permissions = ["SELECT", "DESCRIBE"]
  lf_tag_policy {
    catalog_id    = local.account_id
    resource_type = "TABLE"
    expression {
      key    = module.lakehouse.tenants["alpha"].tenant_tag_key
      values = ["member"]
    }
    expression {
      key    = local.sensitivity_tag_key
      values = ["public", "internal"]
    }
  }

  depends_on = [aws_glue_catalog_database.consumer_link]
}
