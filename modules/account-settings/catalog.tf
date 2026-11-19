# ----
# Federated S3 Tables catalog (FR-029 to FR-031)
# ----

locals {
  # Matches the catalog the catalog-integration submodule created before spec 003 (sandbox result 8).
  # The CLI takes AllowFullTableExternalDataAccess as the string "True" or "False".
  s3tables_catalog_input = jsonencode({
    FederatedCatalog = {
      Identifier     = "arn:${var.partition}:s3tables:${var.region}:${var.account_id}:bucket/*"
      ConnectionName = "aws:s3tables"
    }
    CreateDatabaseDefaultPermissions = []
    CreateTableDefaultPermissions    = []
    AllowFullTableExternalDataAccess = var.s3tables_catalog.allow_full_table_external_data_access ? "True" : "False"
  })
}

# Creates s3tablescatalog only when it is missing, and never updates or deletes it. Terraform
# cannot test for the catalog at plan time (the data source fails on a missing catalog, plan R13),
# so the check runs at apply time through the AWS CLI. The catalog is not in state: lfverify
# reports its attributes instead of plan drift (FR-020 case 11).
#
# Nothing here can delete the catalog: terraform_data has no remote object, and there is no
# destroy-time provisioner, so destroy, owner hand-over, and Ephemeral teardown only forget this
# resource (FR-030). A changed input re-runs the check, which still leaves an existing catalog as is.
resource "terraform_data" "s3tables_catalog" {
  input = {
    account_id    = var.account_id
    region        = var.region
    name          = "s3tablescatalog"
    catalog_input = local.s3tables_catalog_input
    tags          = var.tags
  }

  provisioner "local-exec" {
    command     = "${path.module}/scripts/ensure-s3tables-catalog.sh"
    interpreter = ["bash"]

    environment = {
      ACCOUNT_ID         = var.account_id
      REGION             = var.region
      CATALOG_NAME       = "s3tablescatalog"
      CATALOG_INPUT_JSON = local.s3tables_catalog_input
      TAGS_JSON          = jsonencode(var.tags)
    }
  }

  # The deployer must be a data-lake admin. The bucket/* registration is not needed first
  # (sandbox result 8).
  depends_on = [aws_lakeformation_data_lake_settings.this]
}
