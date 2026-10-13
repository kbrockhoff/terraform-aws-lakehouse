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
    resources = [
      "arn:${var.partition}:kms:${var.region}:${var.account_id}:key/*",
      "arn:${var.partition}:kms:${var.region}:${var.account_id}:alias/*",
    ]
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
      "kms:CreateKey",
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

  # KMS read collections
  statement {
    sid    = "KmsAlias"
    effect = "Allow"
    actions = [
      "kms:CreateAlias",
      "kms:DeleteAlias",
    ]
    resources = [
      "arn:${var.partition}:kms:${var.region}:${var.account_id}:alias/${var.name_prefix}-*",
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
  statement {
    sid    = "PricingAPIAccess"
    effect = "Allow"
    actions = [
      "freetier:GetAccountPlanState",
      "pricing:GetAttributeValues",
      "pricing:GetProducts",
    ]
    resources = ["*"]
  }

}

# IAM Policy (standalone)
resource "aws_iam_policy" "basic" {
  count       = var.enabled ? 1 : 0
  name        = "${var.name_prefix}-github-actions-basic"
  description = "Basic permissions for GitHub Actions CI/CD operations"
  policy      = data.aws_iam_policy_document.basic[0].json

  tags = merge(var.tags, {
    Name = "${var.name_prefix}-github-actions-basic"
  })
}

# IAM Policy Attachment
resource "aws_iam_role_policy_attachment" "basic" {
  count      = var.enabled ? 1 : 0
  role       = split("/", var.iam_role_arn)[1]
  policy_arn = aws_iam_policy.basic[0].arn
}
