# SNS Topic for alarm notifications (only created if no external topic provided)
resource "aws_sns_topic" "alarms" {
  count = local.create_sns_topic ? 1 : 0

  name              = "${local.name_prefix}-alarms"
  kms_master_key_id = local.kms_key_id

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-alarms"
  })

  # s3tablescatalog exists before anything else is created or changed (FR-029)
  depends_on = [terraform_data.s3tables_catalog]
}

