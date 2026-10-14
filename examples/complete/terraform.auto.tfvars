enabled     = true
name_prefix = "complete-ex"
tags = {
  Environment = "Production"
  Example     = "complete"
  CostCenter  = "data-platform"
  DataOwner   = "platform-data@example.com"
}
environment_type = "Production"
cost_estimation_config = {
  enabled = true
}
alarms_config = {
  enabled          = true
  create_sns_topic = true
  sns_topic_arn    = ""
}
governance_config = {
  sensitivity_values = ["public", "internal", "confidential", "restricted"]
  regulation_values  = ["gdpr", "hipaa", "pci_dss"]
  domain_values      = ["general", "sales", "finance", "health"]
}
