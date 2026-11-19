# ----
# Federated S3 Tables catalog (spec 003 FR-029 to FR-031)
# ----

locals {
  # Matches the catalog the catalog-integration submodule created before spec 003 (sandbox result 8).
  # The CLI takes AllowFullTableExternalDataAccess as the string "True" or "False".
  s3tables_catalog_input = jsonencode({
    FederatedCatalog = {
      Identifier     = "arn:${local.partition}:s3tables:${local.region}:${local.account_id}:bucket/*"
      ConnectionName = "aws:s3tables"
    }
    CreateDatabaseDefaultPermissions = []
    CreateTableDefaultPermissions    = []
    AllowFullTableExternalDataAccess = var.account_settings_config.s3tables_catalog.allow_full_table_external_data_access ? "True" : "False"
  })

  # Every other resource this deployment creates or changes waits on this ID (FR-029 ordering):
  # root resources through depends_on, the submodules through their catalog_ready input, and
  # tenants through the platform contract. Empty on non-owners, which never create the catalog.
  s3tables_catalog_ready = length(terraform_data.s3tables_catalog) > 0 ? terraform_data.s3tables_catalog[0].id : ""
}

# The account-settings owner creates s3tablescatalog only when it is missing, before any other
# resource, and never updates or deletes it. Terraform cannot test for the catalog at plan time (the
# data source fails on a missing catalog, plan R13), so the check runs at apply time through the AWS
# CLI. Glue IAM permissions suffice: the deployer need not be a data-lake admin yet (sandbox result
# 8). The catalog is not in state; lfverify reports its attributes instead of plan drift.
#
# Nothing here can delete the catalog: terraform_data has no remote object and there is no
# destroy-time provisioner, so destroy, owner hand-over, and Ephemeral teardown only forget it
# (FR-030). A changed input replaces this step and re-runs the check, which still leaves an existing
# catalog unchanged. Everything waits on it through resource-level depends_on only, never a
# module-level depends_on or a data source, so a replacement only orders the apply.
resource "terraform_data" "s3tables_catalog" {
  count = var.enabled && local.settings_owner ? 1 : 0

  input = {
    account_id    = local.account_id
    region        = local.region
    name          = "s3tablescatalog"
    catalog_input = local.s3tables_catalog_input
    tags          = local.common_tags
  }

  provisioner "local-exec" {
    command     = "${path.module}/scripts/ensure-s3tables-catalog.sh"
    interpreter = ["bash"]

    environment = {
      ACCOUNT_ID         = local.account_id
      REGION             = local.region
      CATALOG_NAME       = "s3tablescatalog"
      CATALOG_INPUT_JSON = local.s3tables_catalog_input
      TAGS_JSON          = jsonencode(local.common_tags)
    }
  }
}

moved {
  from = module.account_settings[0].terraform_data.s3tables_catalog
  to   = terraform_data.s3tables_catalog[0]
}
