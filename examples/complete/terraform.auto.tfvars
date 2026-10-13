enabled     = true
name_prefix = "complete-ex"
tags = {
  "Environment" = "Production"
  "Example"     = "complete"
}
data_tags = {
  "DataClassification" = "Confidential"
}
environment_type = "None"
cost_estimation_config = {
  enabled = true
}
networktags_name = "NetworkTags"
alarms_config = {
  enabled          = true
  create_sns_topic = true
  sns_topic_arn    = ""
}
