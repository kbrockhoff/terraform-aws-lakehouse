# Statements are split across two managed policies, because together they exceed the IAM
# managed policy size quota (6,144 characters; found in the T076 sandbox).

# IAM Policy Document for S3 Backend and Basic Terraform Operations
data "aws_iam_policy_document" "basic" {
  count = var.enabled ? 1 : 0

  # KMS read collections
  statement {
    sid    = "KmsRead"
    effect = "Allow"
    actions = [
      "kms:ListAliases",
      "kms:ListKeys",
    ]
    resources = ["*"] # ListAliases and ListKeys have no resource-level permissions
  }

  # KMS provisioning
  statement {
    sid    = "KmsProvisioning"
    effect = "Allow"
    actions = [
      "kms:EnableKeyRotation",
      "kms:DescribeKey",
      "kms:ListResourceTags",
      "kms:ScheduleKeyDeletion",
      "kms:Decrypt",
      "kms:GetKeyRotationStatus",
      "kms:Encrypt",
      "kms:GenerateDataKey",
      "kms:GetKeyPolicy",
      "kms:PutKeyPolicy",
      "kms:TagResource",
      "kms:UntagResource",
      "kms:DisableKey",
      "kms:EnableKey",
      "kms:CancelKeyDeletion",
      "kms:DisableKeyRotation",
      "kms:ListGrants",
      "kms:CreateGrant",
      "kms:RevokeGrant",
      "kms:RetireGrant",
    ]
    resources = [
      "arn:${var.partition}:kms:${var.region}:${var.account_id}:key/*",
    ]
  }

  # CreateKey has no resource-level permissions: the key does not exist yet
  statement {
    sid       = "KmsCreateKey"
    effect    = "Allow"
    actions   = ["kms:CreateKey"]
    resources = ["*"]
  }

  # KMS read collections
  statement {
    sid    = "KmsAlias"
    effect = "Allow"
    actions = [
      "kms:CreateAlias",
      "kms:DeleteAlias",
    ]
    # CreateAlias and DeleteAlias authorize against both the alias and the key
    resources = [
      "arn:${var.partition}:kms:${var.region}:${var.account_id}:alias/${var.name_prefix}-*",
      "arn:${var.partition}:kms:${var.region}:${var.account_id}:key/*",
    ]
  }

  statement {
    sid    = "AlarmManagement"
    effect = "Allow"
    actions = [
      "cloudwatch:DescribeAlarms",
      "cloudwatch:PutMetricAlarm",
      "cloudwatch:DeleteAlarms",
      "cloudwatch:ListTagsForResource",
      "cloudwatch:TagResource",
      "cloudwatch:UntagResource",
    ]
    resources = [
      "arn:${var.partition}:cloudwatch:${var.region}:${var.account_id}:alarm:${var.name_prefix}*"
    ]
  }

  statement {
    sid    = "TopicManagement"
    effect = "Allow"
    actions = [
      "sns:CreateTopic",
      "sns:DeleteTopic",
      "sns:GetTopicAttributes",
      "sns:SetTopicAttributes",
      "sns:ListTagsForResource",
      "sns:TagResource",
      "sns:UntagResource",
    ]
    resources = [
      "arn:${var.partition}:sns:${var.region}:${var.account_id}:${var.name_prefix}*"
    ]
  }

  # Basic AWS API access for resource management
  statement {
    sid    = "BasicAWSAccess"
    effect = "Allow"
    actions = [
      "sts:GetCallerIdentity",
    ]
    resources = ["*"]
  }

  # Pricing API access (required for cost estimation)
  dynamic "statement" {
    for_each = var.cost_estimation_enabled ? [1] : []
    content {
      sid    = "PricingAPIAccess"
      effect = "Allow"
      actions = [
        "freetier:GetAccountPlanState",
        "pricing:GetAttributeValues",
        "pricing:GetProducts",
      ]
      resources = ["*"] # the Price List API has no resource-level permissions
    }
  }

  # ----
  # Query results buckets
  # ----

  statement {
    sid    = "ResultsBuckets"
    effect = "Allow"
    actions = [
      "s3:CreateBucket",
      "s3:DeleteBucket",
      "s3:ListBucket",
      "s3:ListBucketVersions",
      "s3:DeleteObject",
      "s3:DeleteObjectVersion",
      "s3:GetBucketLocation",
      "s3:GetBucketAcl",
      "s3:GetBucketCORS",
      "s3:GetBucketWebsite",
      "s3:GetBucketLogging",
      "s3:GetBucketVersioning",
      "s3:GetBucketRequestPayment",
      "s3:GetBucketObjectLockConfiguration",
      "s3:GetAccelerateConfiguration",
      "s3:GetReplicationConfiguration",
      "s3:GetBucketPolicy",
      "s3:GetBucketPolicyStatus",
      "s3:PutBucketPolicy",
      "s3:DeleteBucketPolicy",
      "s3:GetBucketPublicAccessBlock",
      "s3:PutBucketPublicAccessBlock",
      "s3:GetBucketOwnershipControls",
      "s3:PutBucketOwnershipControls",
      "s3:GetEncryptionConfiguration",
      "s3:PutEncryptionConfiguration",
      "s3:GetLifecycleConfiguration",
      "s3:PutLifecycleConfiguration",
      "s3:GetBucketTagging",
      "s3:PutBucketTagging",
    ]
    # The platform also creates designated tenants' results buckets (<prefix>-<tenant>-athena-results-*)
    resources = var.scope == "platform" ? [
      "arn:${var.partition}:s3:::${var.name_prefix}-*athena-results-*",
      "arn:${var.partition}:s3:::${var.name_prefix}-*athena-results-*/*",
      ] : [
      "arn:${var.partition}:s3:::${var.name_prefix}-*-athena-results-*",
      "arn:${var.partition}:s3:::${var.name_prefix}-*-athena-results-*/*",
    ]
  }

  # Athena checks access to a workgroup's output location when creating it; tenant workgroups
  # write to a prefix of the shared results bucket (found in the T076 sandbox)
  dynamic "statement" {
    for_each = var.scope == "tenant" ? [1] : []
    content {
      sid    = "SharedResultsLocation"
      effect = "Allow"
      actions = [
        "s3:GetBucketLocation",
        "s3:ListBucket",
      ]
      resources = ["arn:${var.partition}:s3:::${var.name_prefix}-athena-results-*"]
    }
  }

  # aws_iam_session_context resolves the deploying role
  statement {
    sid       = "ResolveSessionRole"
    effect    = "Allow"
    actions   = ["iam:GetRole"]
    resources = ["arn:${var.partition}:iam::${var.account_id}:role/*"]
  }

  # Lake Formation checks Glue read access to the databases and tables it labels and grants on
  # (found in the T076 sandbox)
  statement {
    sid    = "CatalogRead"
    effect = "Allow"
    actions = [
      "glue:GetCatalog",
      "glue:GetCatalogs",
      "glue:GetDatabase",
      "glue:GetDatabases",
      "glue:GetTable",
      "glue:GetTables",
    ]
    resources = [
      "arn:${var.partition}:glue:${var.region}:${var.account_id}:catalog",
      "arn:${var.partition}:glue:${var.region}:${var.account_id}:catalog/*",
      "arn:${var.partition}:glue:${var.region}:${var.account_id}:database/*",
      "arn:${var.partition}:glue:${var.region}:${var.account_id}:table/*",
    ]
  }

  # Tenant pipelines that read the platform contract from SSM (found in the T076 sandbox)
  dynamic "statement" {
    for_each = var.platform_contract_parameter_arn != "" ? [1] : []
    content {
      sid       = "ReadPlatformContract"
      effect    = "Allow"
      actions   = ["ssm:GetParameter"]
      resources = [var.platform_contract_parameter_arn]
    }
  }

}

data "aws_iam_policy_document" "lakehouse" {
  count = var.enabled ? 1 : 0

  # ----
  # S3 Tables: zone, dedicated, and tenant buckets and namespaces
  # ----

  statement {
    sid    = "TableBuckets"
    effect = "Allow"
    actions = [
      "s3tables:CreateTableBucket",
      "s3tables:DeleteTableBucket",
      "s3tables:GetTableBucket",
      "s3tables:PutTableBucketEncryption",
      "s3tables:GetTableBucketEncryption",
      "s3tables:PutTableBucketMaintenanceConfiguration",
      "s3tables:GetTableBucketMaintenanceConfiguration",
      "s3tables:PutTableBucketStorageClass",
      "s3tables:GetTableBucketStorageClass",
      "s3tables:PutTableBucketPolicy",
      "s3tables:GetTableBucketPolicy",
      "s3tables:DeleteTableBucketPolicy",
      "s3tables:CreateNamespace",
      "s3tables:DeleteNamespace",
      "s3tables:GetNamespace",
      "s3tables:ListNamespaces",
      "s3tables:TagResource",
      "s3tables:UntagResource",
      "s3tables:ListTagsForResource",
    ]
    resources = [
      "arn:${var.partition}:s3tables:${var.region}:${var.account_id}:bucket/${var.name_prefix}-*",
    ]
  }

  statement {
    sid       = "ListTableBuckets"
    effect    = "Allow"
    actions   = ["s3tables:ListTableBuckets"]
    resources = ["*"] # ListTableBuckets has no resource-level permissions
  }

  # ----
  # Lake Formation
  # ----

  statement {
    sid    = "LakeFormationGovernance"
    effect = "Allow"
    actions = [
      "lakeformation:GetDataLakeSettings",
      "lakeformation:GetDataAccess", # Glue resolves federated S3 Tables databases through it
      "lakeformation:GrantPermissions",
      "lakeformation:RevokePermissions",
      "lakeformation:ListPermissions",
      "lakeformation:CreateLFTag",
      "lakeformation:DeleteLFTag",
      "lakeformation:GetLFTag",
      "lakeformation:UpdateLFTag",
      "lakeformation:AddLFTagsToResource",
      "lakeformation:RemoveLFTagsFromResource",
      "lakeformation:GetResourceLFTags",
      "lakeformation:CreateDataCellsFilter",
      "lakeformation:DeleteDataCellsFilter",
      "lakeformation:GetDataCellsFilter",
    ]
    resources = ["*"] # Lake Formation has no resource-level permissions; it authorizes by its own grants
  }

  # ----
  # Athena workgroups
  # ----

  statement {
    sid    = "AthenaWorkgroups"
    effect = "Allow"
    actions = [
      "athena:CreateWorkGroup",
      "athena:UpdateWorkGroup",
      "athena:DeleteWorkGroup",
      "athena:GetWorkGroup",
      "athena:TagResource",
      "athena:UntagResource",
      "athena:ListTagsForResource",
    ]
    resources = [
      "arn:${var.partition}:athena:${var.region}:${var.account_id}:workgroup/${var.name_prefix}-*",
    ]
  }

  # ----
  # Platform scope only: account-level catalog integration
  # ----

  dynamic "statement" {
    for_each = var.scope == "platform" ? [1] : []
    content {
      sid    = "FederatedCatalog"
      effect = "Allow"
      actions = [
        "glue:CreateCatalog",
        "glue:GetCatalog",
        "glue:UpdateCatalog",
        "glue:DeleteCatalog",
        "glue:TagResource",
        "glue:UntagResource",
        "glue:GetTags",
      ]
      resources = [
        "arn:${var.partition}:glue:${var.region}:${var.account_id}:catalog",
        "arn:${var.partition}:glue:${var.region}:${var.account_id}:catalog/s3tablescatalog",
        "arn:${var.partition}:glue:${var.region}:${var.account_id}:catalog/s3tablescatalog/*",
      ]
    }
  }

  # The federated catalog is created through the AWS-managed aws:s3tables connection
  dynamic "statement" {
    for_each = var.scope == "platform" ? [1] : []
    content {
      sid       = "PassS3TablesConnection"
      effect    = "Allow"
      actions   = ["glue:PassConnection"]
      resources = ["arn:${var.partition}:glue:${var.region}:${var.account_id}:connection/aws:s3tables"]
    }
  }

  dynamic "statement" {
    for_each = var.scope == "platform" ? [1] : []
    content {
      sid    = "LakeFormationIntegration"
      effect = "Allow"
      actions = [
        "lakeformation:RegisterResource",
        "lakeformation:DeregisterResource",
        "lakeformation:DescribeResource",
        "lakeformation:PutDataLakeSettings",
      ]
      resources = ["*"] # Lake Formation has no resource-level permissions
    }
  }

  dynamic "statement" {
    for_each = var.scope == "platform" ? [1] : []
    content {
      sid    = "DataAccessRole"
      effect = "Allow"
      actions = [
        "iam:CreateRole",
        "iam:DeleteRole",
        "iam:PutRolePolicy",
        "iam:DeleteRolePolicy",
        "iam:GetRolePolicy",
        "iam:ListRolePolicies",
        "iam:ListAttachedRolePolicies",
        "iam:ListInstanceProfilesForRole",
        "iam:TagRole",
        "iam:UntagRole",
      ]
      resources = ["arn:${var.partition}:iam::${var.account_id}:role/${var.name_prefix}-lf-s3tables-access"]
    }
  }

  dynamic "statement" {
    for_each = var.scope == "platform" ? [1] : []
    content {
      sid       = "PassDataAccessRole"
      effect    = "Allow"
      actions   = ["iam:PassRole"]
      resources = ["arn:${var.partition}:iam::${var.account_id}:role/${var.name_prefix}-lf-s3tables-access"]
      condition {
        test     = "StringEquals"
        variable = "iam:PassedToService"
        values   = ["lakeformation.amazonaws.com"]
      }
    }
  }
}

# Both documents merged, for review and tests (not attached)
data "aws_iam_policy_document" "combined" {
  count = var.enabled ? 1 : 0

  source_policy_documents = [
    data.aws_iam_policy_document.basic[0].json,
    data.aws_iam_policy_document.lakehouse[0].json,
  ]
}
# IAM Policy (standalone)
resource "aws_iam_policy" "basic" {
  count       = var.enabled ? 1 : 0
  name        = "${var.name_prefix}-${var.scope}-deployer"
  description = "Least-privilege permissions to deploy the lakehouse ${var.scope} with Terraform"
  policy      = data.aws_iam_policy_document.basic[0].json

  tags = merge(var.tags, {
    Name = "${var.name_prefix}-${var.scope}-deployer"
  })
}

resource "aws_iam_policy" "lakehouse" {
  count       = var.enabled ? 1 : 0
  name        = "${var.name_prefix}-${var.scope}-deployer-lakehouse"
  description = "Least-privilege S3 Tables, Lake Formation, and Athena permissions to deploy the lakehouse ${var.scope} with Terraform"
  policy      = data.aws_iam_policy_document.lakehouse[0].json

  tags = merge(var.tags, {
    Name = "${var.name_prefix}-${var.scope}-deployer-lakehouse"
  })
}

# IAM Policy Attachments
resource "aws_iam_role_policy_attachment" "basic" {
  count      = var.enabled ? 1 : 0
  role       = split("/", var.iam_role_arn)[1]
  policy_arn = aws_iam_policy.basic[0].arn
}

resource "aws_iam_role_policy_attachment" "lakehouse" {
  count      = var.enabled ? 1 : 0
  role       = split("/", var.iam_role_arn)[1]
  policy_arn = aws_iam_policy.lakehouse[0].arn
}
