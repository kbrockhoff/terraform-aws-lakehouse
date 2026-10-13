# ----
# KMS Pricing Data
# ----

# KMS pricing
data "aws_pricing_product" "kms" {
  count = local.pricing_enabled ? 1 : 0

  service_code = "AWSKMS"

  filters {
    field = "productFamily"
    value = "Encryption Key"
  }

  filters {
    field = "usagetype"
    value = "${var.region}-KMS-Keys"
  }
}

# ----
# CloudWatch Pricing Data
# ----

# CloudWatch standard metrics
data "aws_pricing_product" "cloudwatch_metrics" {
  count = local.pricing_enabled ? 1 : 0

  service_code = "AmazonCloudWatch"

  filters {
    field = "productFamily"
    value = "Metric"
  }

  filters {
    field = "usagetype"
    value = "${local.usagetype_region}-CW:MetricsUsage"
  }

  filters {
    field = "location"
    value = local.pricing_location
  }
}

# CloudWatch alarms
data "aws_pricing_product" "cloudwatch_alarms" {
  count = local.pricing_enabled ? 1 : 0

  service_code = "AmazonCloudWatch"

  filters {
    field = "productFamily"
    value = "Alarm"
  }

  filters {
    field = "usagetype"
    value = "CW:AlarmMonitorUsage"
  }

  filters {
    field = "location"
    value = local.pricing_location
  }
}

# ----
# SNS Pricing Data
# ----

# SNS topics pricing
data "aws_pricing_product" "sns_requests" {
  count = local.pricing_enabled ? 1 : 0

  service_code = "AmazonSNS"

  filters {
    field = "productFamily"
    value = "API Request"
  }

  filters {
    field = "location"
    value = local.pricing_location
  }
}
