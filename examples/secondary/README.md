# Secondary Deployment Example

A second lakehouse platform in an account and Region whose catalog integration (federated
`s3tablescatalog`, Lake Formation registration, and data-lake settings) is owned by another
deployment.

- Set `catalog_integration_config.enabled = false` and pass the owning deployment's
  `data_access_role_arn` output.
- The role that deploys this example **must already be a data-lake admin**: add it to the owning
  deployment's `governance_config.admin_principal_arns` and apply that deployment first. The plan
  fails on `terraform_data.secondary_admin_guard` otherwise.
- Use a different `name_prefix` from the owner, so LF-tag keys and bucket names do not collide.

<!-- BEGIN_TF_DOCS -->
<!-- END_TF_DOCS -->
