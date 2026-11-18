# ----
# Glue catalog-encryption role (SSE-KMS-WITH-SERVICE-ROLE)
# ----

locals {
  service_role_mode = var.catalog_encryption.key_access_mode == "service_role"
}

# Glue assumes this role for every catalog encrypt and decrypt, so callers need no key access
resource "aws_iam_role" "catalog_encryption" {
  count = local.service_role_mode ? 1 : 0

  name                 = "${var.name_prefix}-glue-catalog-encryption"
  description          = "Glue Data Catalog metadata encryption in ${var.region}"
  assume_role_policy   = data.aws_iam_policy_document.catalog_encryption_trust.json
  permissions_boundary = var.permissions_boundary_arn != "" ? var.permissions_boundary_arn : null

  tags = var.tags
}

# aws:SourceAccount only (FR-011). Glue sends no source ARN matching the catalog when it assumes the
# role, and an aws:SourceArn condition breaks all catalog access (sandbox-results.md item 3).
data "aws_iam_policy_document" "catalog_encryption_trust" {
  statement {
    sid     = "GlueAssume"
    effect  = "Allow"
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["glue.amazonaws.com"]
    }
    condition {
      test     = "StringEquals"
      variable = "aws:SourceAccount"
      values   = [var.account_id]
    }
  }
}

resource "aws_iam_role_policy" "catalog_encryption" {
  count = local.service_role_mode ? 1 : 0

  name   = "catalog-key"
  role   = aws_iam_role.catalog_encryption[0].id
  policy = data.aws_iam_policy_document.catalog_encryption_key.json
}

# Glue itself narrows each session to one action; Encrypt is listed because the Glue documentation
# requires it
data "aws_iam_policy_document" "catalog_encryption_key" {
  statement {
    sid    = "CatalogKey"
    effect = "Allow"
    actions = [
      "kms:Decrypt",
      "kms:Encrypt",
      "kms:GenerateDataKey",
    ]
    resources = [local.catalog_key_arn]
  }
}

# Glue rejects a new role as not assumable for about 10 to 15 seconds after it is created
# (sandbox-results.md item 2)
resource "time_sleep" "catalog_encryption_role" {
  count = local.service_role_mode ? 1 : 0

  create_duration = "20s"

  triggers = {
    role_arn  = aws_iam_role.catalog_encryption[0].arn
    policy_id = aws_iam_role_policy.catalog_encryption[0].id
  }
}
