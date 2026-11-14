# ----
# Tenant query alarms (FR-023)
# ----

# Athena publishes per-query metrics with QueryState, QueryType, and WorkGroup dimensions; an alarm
# matches only when all three are given.

resource "aws_cloudwatch_metric_alarm" "query_failures" {
  count = local.effective.alarms_enabled ? 1 : 0

  alarm_name          = "${local.name_prefix}-${var.name}-query-failures"
  alarm_description   = "Failed queries in the ${var.name} tenant workgroup"
  namespace           = "AWS/Athena"
  metric_name         = "TotalExecutionTime"
  statistic           = "SampleCount"
  period              = 300
  evaluation_periods  = 1
  threshold           = 0
  comparison_operator = "GreaterThanThreshold"
  treat_missing_data  = "notBreaching"

  dimensions = {
    WorkGroup  = aws_athena_workgroup.this.name
    QueryState = "FAILED"
    QueryType  = "DML"
  }

  alarm_actions = [var.platform.alarm_sns_topic_arn]
  ok_actions    = [var.platform.alarm_sns_topic_arn]

  tags = local.tags
}

resource "aws_cloudwatch_metric_alarm" "scanned_bytes" {
  count = local.effective.alarms_enabled ? 1 : 0

  alarm_name          = "${local.name_prefix}-${var.name}-scanned-bytes"
  alarm_description   = "Bytes scanned per day by the ${var.name} tenant workgroup above the threshold"
  namespace           = "AWS/Athena"
  metric_name         = "ProcessedBytes"
  statistic           = "Sum"
  period              = 86400
  evaluation_periods  = 1
  threshold           = local.scanned_bytes_alarm_threshold
  comparison_operator = "GreaterThanThreshold"
  treat_missing_data  = "notBreaching"

  dimensions = {
    WorkGroup  = aws_athena_workgroup.this.name
    QueryState = "SUCCEEDED"
    QueryType  = "DML"
  }

  alarm_actions = [var.platform.alarm_sns_topic_arn]
  ok_actions    = [var.platform.alarm_sns_topic_arn]

  tags = local.tags
}

# The DML workgroup gets its own pair, so each workgroup is held to the tenant threshold
resource "aws_cloudwatch_metric_alarm" "dml_query_failures" {
  count = local.effective.alarms_enabled && local.dml_workgroup_enabled ? 1 : 0

  alarm_name          = "${local.name_prefix}-${var.name}-dml-query-failures"
  alarm_description   = "Failed queries in the ${var.name} tenant DML workgroup"
  namespace           = "AWS/Athena"
  metric_name         = "TotalExecutionTime"
  statistic           = "SampleCount"
  period              = 300
  evaluation_periods  = 1
  threshold           = 0
  comparison_operator = "GreaterThanThreshold"
  treat_missing_data  = "notBreaching"

  dimensions = {
    WorkGroup  = aws_athena_workgroup.dml[0].name
    QueryState = "FAILED"
    QueryType  = "DML"
  }

  alarm_actions = [var.platform.alarm_sns_topic_arn]
  ok_actions    = [var.platform.alarm_sns_topic_arn]

  tags = local.tags
}

resource "aws_cloudwatch_metric_alarm" "dml_scanned_bytes" {
  count = local.effective.alarms_enabled && local.dml_workgroup_enabled ? 1 : 0

  alarm_name          = "${local.name_prefix}-${var.name}-dml-scanned-bytes"
  alarm_description   = "Bytes scanned per day by the ${var.name} tenant DML workgroup above the threshold"
  namespace           = "AWS/Athena"
  metric_name         = "ProcessedBytes"
  statistic           = "Sum"
  period              = 86400
  evaluation_periods  = 1
  threshold           = local.scanned_bytes_alarm_threshold
  comparison_operator = "GreaterThanThreshold"
  treat_missing_data  = "notBreaching"

  dimensions = {
    WorkGroup  = aws_athena_workgroup.dml[0].name
    QueryState = "SUCCEEDED"
    QueryType  = "DML"
  }

  alarm_actions = [var.platform.alarm_sns_topic_arn]
  ok_actions    = [var.platform.alarm_sns_topic_arn]

  tags = local.tags
}
