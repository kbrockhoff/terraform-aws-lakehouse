enabled     = true
name_prefix = "complete-ex"
tags = {
  Environment = "Production"
  Example     = "complete"
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
# SageMaker Unified Studio in the same account: a deployed domain makes its blueprint provisioning
# role and its manage-access role data-lake admins. Placeholder ARNs; replace before applying.
studio_config = {
  enabled = true
  integration_principal_arns = [
    "arn:aws:iam::123456789012:role/service-role/AmazonSageMakerProvisioning-123456789012",
    "arn:aws:iam::123456789012:role/service-role/AmazonSageMakerManageAccess-us-east-1-dzd_example",
  ]
}

# Approved consumer accounts for tenant shares. 111122223333 is a placeholder for plans only: a
# share to an account that does not exist fails at apply.
sharing_config = {
  consumer_accounts = {
    "111122223333" = {
      purpose = "Partner analytics (example, plan only)"
    }
  }
}

governance_config = {
  sensitivity_values = ["public", "internal", "confidential", "restricted"]
  regulation_values  = ["gdpr", "hipaa", "pci_dss"]
  domain_values      = ["general", "sales", "finance", "health"]

  # Auditors view all catalog resources and permissions but change nothing
  read_only_admin_principal_arns = ["arn:aws:iam::123456789012:role/lakehouse-auditor"]
}
