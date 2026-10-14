# Catalog Integration Submodule

Integrates S3 Tables with AWS Lake Formation for one account and Region:

- the Lake Formation data-access role (`<name_prefix>-lf-s3tables-access`);
- the registration of every table bucket (`bucket/*`) with Lake Formation, with federation;
- the federated `s3tablescatalog` Glue catalog;
- the data-lake settings: admins, and empty default permissions.

## One owner per account and Region

These resources are account-level singletons. Exactly one deployment per account and Region uses
this submodule (`catalog_integration_config.enabled = true` in the root module). Every other
deployment sets `enabled = false` and passes this submodule's `data_access_role_arn`.

## Data-lake settings are authoritative

`aws_lakeformation_data_lake_settings` replaces **all** data-lake settings in the account and
Region. Any admin not in `admin_principal_arns` is removed on apply, including admins added in the
console. List in `admin_principal_arns`:

- the role that deploys this module (a plan-time precondition enforces this);
- the deploying role of every secondary deployment;
- every tenant provisioning pipeline role.

## Importing an existing catalog

If `s3tablescatalog` was created in the console, import it before the first apply, or the create
fails because the catalog already exists. Use an `import` block with the ID format from the AWS
provider's `aws_glue_catalog` import documentation:

```hcl
import {
  to = module.main.module.catalog_integration[0].aws_glue_catalog.s3tables
  id = "<see aws_glue_catalog import docs>"
}
```

Import an existing registration (`aws_lakeformation_resource.s3tables`) the same way.

<!-- BEGIN_TF_DOCS -->
<!-- END_TF_DOCS -->
