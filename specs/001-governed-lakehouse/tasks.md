---

description: "Implementation tasks for the Governed Lakehouse Foundation module"
---

# Tasks: Governed Lakehouse Foundation

**Input**: Design documents from `/specs/001-governed-lakehouse/`
**Prerequisites**: plan.md, spec.md (no research.md, architecture.md, modules.md, or quickstart.md)

**Organization**: Tasks are grouped by tier, following the dependency order Foundation →
Data & Query → Operations. This module has **no network tier**: every service is a Regional AWS
public endpoint (plan.md, Networking). The phase that would hold networking instead holds the
encryption, identity, and account-level catalog foundation that every data resource depends on.

**Revision**: Regenerated 2026-10-13 after `/iac.analyze` remediation (principles v1.2.1).

## Format: `[ID] [P?] Description`

- **[ID]**: Sequential task number (T001, T002, T003...)
- **[P]**: Can run in parallel (different files, no dependencies) - optional
- **Description**: Clear action with exact file path included

## Path Conventions

- The module lives at the **repository root** (Registry layout; plan.md, Structure Decision). Do
  not use `iac/`.
- Submodules go in `modules/<name>/`, examples in `examples/<name>/`, tests in `test/`.
- **All validation runs through `make` targets** (principles v1.2.1, Validate Before Merge).
  Never invoke `terraform`, `tflint`, `terraform-docs`, or `go test` directly.
- Every policy uses `aws_iam_policy_document`, never inline JSON. Never build an ARN from a naming
  convention to reference another resource; use resource attributes, module outputs, or inputs.
  Follow the comment and section header style in `AGENTS.md`.

---

## Phase 1: Setup

**Purpose**: Version constraints, inputs, environment defaults, and shared data sources that every
later tier reads.

- [ ] T001 Update `versions.tf`: set the `hashicorp/aws` constraint to `>= 6.59.0, < 7.0.0` and keep `configuration_aliases = [aws.pricing]` and `required_version = ">= 1.11"`. Apply the same AWS constraint in `examples/defaults/versions.tf`, `examples/complete/versions.tf`, `modules/cost-estimation/versions.tf`, and `modules/deployer/versions.tf`.
- [ ] T002 [P] Add two data sources to `dependencies.tf`:
  - `data "aws_iam_session_context" "current"` with `arn = data.aws_caller_identity.current.arn`, which resolves the deploying role ARN;
  - `data "aws_lakeformation_data_lake_settings" "current"`, with `count = var.enabled && !var.catalog_integration_config.enabled ? 1 : 0`, which secondary deployments use for their admin check.
- [ ] T003 Edit `variables.tf`:
  - Remove `networktags_name`.
  - Add `zones_config` as `object({ zones = list(string), namespaces = map(list(string)) })`, defaulting to `zones = ["raw", "curated"]` and `namespaces = {}`.
  - Validate `zones_config`:
    - zone names match `^[a-z0-9]([a-z0-9-]*[a-z0-9])?$` (no underscores; they become table bucket names);
    - `length(var.name_prefix) + 1 + length(zone) <= 63` for every zone (cross-variable check);
    - the zones list is unique and non-empty;
    - namespace names match `^[a-z0-9_]{1,255}$`, and every key in `namespaces` is a declared zone.
- [ ] T004 Add `catalog_integration_config` to `variables.tf` (FR-005):
  - Type `object({ enabled = bool, data_access_role_arn = string, allow_full_table_external_data_access = bool })`, default `{ enabled = true, data_access_role_arn = "", allow_full_table_external_data_access = false }`.
  - Validation: `data_access_role_arn` must be a valid IAM role ARN when `enabled = false`, and empty when `enabled = true`.
- [ ] T005 Add `governance_config` to `variables.tf` (FR-007, FR-008, FR-009, FR-012a):
  - `admin_principal_arns = list(string)`.
  - `writers`: map of objects with `principal_arn`, `zones`, and `workgroups`.
  - `readers`: map of objects with `principal_arn`, `zones`, `namespaces`, `workgroups`, and `classification_values`.
  - `classification_values = list(string)`, defaulting to `["public", "internal", "confidential", "restricted"]`.
  - `namespace_classifications`: map of `"<zone>/<namespace>"` to a classification value.
  - `reader_column_grants`: map of objects with `principal_arn`, `zone`, `namespace`, `table`, `column_names`, `excluded_column_names`.
  - `reader_row_filters`: map of objects with `principal_arn`, `zone`, `namespace`, `table`, `row_filter_expression`, `column_names`.
  - `writer_drop_enabled`: nullable bool, default `null` so the environment default applies.
  - All other fields default to empty.
  - Validation: every ARN matches `^arn:aws[a-z-]*:iam::[0-9]{12}:(role|user)/.+$`; every referenced zone, namespace, and workgroup is declared in `zones_config` or `query_config`; every classification value is in `classification_values`, which is ordered least to most sensitive.
- [ ] T006 Add `query_config` to `variables.tf` (FR-010 to FR-012):
  - Type `object({ workgroups = list(string), bytes_scanned_cutoff_per_query = number, results_expiration_days = number })`, default `workgroups = ["primary"]`, with the number fields `null` so environment defaults apply.
  - Validation: cutoff `>= 10485760` when not null; expiration between 1 and 3650 days when not null; workgroup names match `^[a-zA-Z0-9._-]{1,128}$`.
- [ ] T007 Add `compliance_frameworks` to `variables.tf` (FR-021): `set(string)`, default `[]`, validated as a subset of `["SOC2", "HIPAA", "PCI_DSS"]`. Add `scanned_bytes_threshold` to the `alarms_config` object (number, default `null`, so the environment default applies).
- [ ] T008 Add `required_tags_config` to `variables.tf` (FR-016):
  - Type `object({ enforced = bool, cost_center_key = string, data_owner_key = string, data_classification_key = string })`.
  - Default `{ enforced = null, cost_center_key = "CostCenter", data_owner_key = "DataOwner", data_classification_key = "DataClassification" }`.
  - Validate only that the key names are non-empty. Enforcement itself is the T024 precondition, because variable validation can't read `local.effective_config`.
- [ ] T009 Extend `locals.environment_defaults` in `locals.tf` with the keys and values from the plan.md Environment Configuration table:
  - `query_results_expiration_days`, `bytes_scanned_cutoff_per_query`, `unreferenced_days`, `non_current_days`, `force_destroy`;
  - `classification_grants_enabled`, `writer_drop_enabled`, `required_tags_enforced`, `scanned_bytes_alarm_threshold`.
  - The `None` entry reads every key from its `*_config` input. `writer_drop_enabled` comes from `governance_config.writer_drop_enabled` and `required_tags_enforced` from `required_tags_config.enforced`; a validation requires both to be non-null when `environment_type = "None"`.
  - For other environment types, a non-null input overrides the default (`coalesce(input, default)`).
- [ ] T010 Add derived locals to `locals.tf`:
  - `zones` (set) and `zone_namespaces` (flattened map keyed `"<zone>/<namespace>"`).
  - `table_bucket_names` (`"${local.name_prefix}-${zone}"`).
  - `compliance_designated = length(var.compliance_frameworks) > 0`.
  - `baseline_environment = contains(["Ephemeral", "Development", "Testing"], var.environment_type)`.
  - `s3tables_catalog_id = { for z in zones : z => "${account_id}:s3tablescatalog/${table_bucket_names[z]}" }`.
  - `deployer_role_arn = data.aws_iam_session_context.current.issuer_arn`.
  - `data_access_role_arn`: `module.catalog_integration[0].data_access_role_arn` when integration is enabled, otherwise `var.catalog_integration_config.data_access_role_arn`.
  - `scanned_bytes_alarm_threshold`: `coalesce(var.alarms_config.scanned_bytes_threshold, local.effective_config.scanned_bytes_alarm_threshold)`.
- [ ] T011 [P] Remove the `networktags_name` argument from `examples/complete/main.tf`, `examples/complete/variables.tf`, `examples/defaults/main.tf`, `examples/defaults/variables.tf`, and the `terraform.auto.tfvars` files that set it.
- [ ] T012 **Checkpoint**: run `make check`. Formatting and `validate` must pass for all submodules and examples before Phase 2.

---

## Phase 2: Foundation Tier (Encryption, Identity, Account-Level Catalog Integration)

**Purpose**: Keys, the Lake Formation data-access role, the federated `s3tablescatalog`, the
data-lake settings, and the admin guards. Every table bucket, grant, and workgroup depends on these.

**⚠️ CRITICAL**: Phase 3 cannot start until this phase passes its checkpoint.

### Catalog-integration submodule (FR-004, FR-005, FR-009)

- [ ] T013 [P] Create `modules/catalog-integration/versions.tf` with `required_version = ">= 1.11"` and `hashicorp/aws >= 6.59.0, < 7.0.0`.
- [ ] T014 [P] Create `modules/catalog-integration/variables.tf` with these inputs, each with a description and validation. It takes no KMS key ARNs, which avoids a dependency cycle with the root module's key policies.
  - `name_prefix`, `account_id`, `partition`, `region`, `tags`;
  - `admin_principal_arns` (list, non-empty);
  - `deployer_role_arn` (string);
  - `allow_full_table_external_data_access` (bool);
  - `classification_values` (list).
- [ ] T015 Create `modules/catalog-integration/iam.tf` with the Lake Formation data-access role `aws_iam_role.lf_s3tables_access`, named `${name_prefix}-lf-s3tables-access`:
  - Trust: `lakeformation.amazonaws.com` for `sts:AssumeRole`, `sts:SetContext`, `sts:SetSourceIdentity`, conditioned on `aws:SourceAccount`.
  - Inline policy, via `aws_iam_policy_document`:
    - S3 Tables data and metadata actions on `arn:${partition}:s3tables:${region}:${account_id}:bucket/*`;
    - `kms:Decrypt` and `kms:GenerateDataKey` on `*`, conditioned on `kms:ViaService = s3tables.${region}.amazonaws.com` and `aws:ResourceAccount = ${account_id}`. Key policies do the per-key scoping, which lets the role serve secondary deployments' buckets.
- [ ] T016 Create `modules/catalog-integration/main.tf` with:
  - `aws_lakeformation_resource.s3tables`: `arn = "arn:${partition}:s3tables:${region}:${account_id}:bucket/*"`, the role ARN from T015, `with_federation = true`, `hybrid_access_enabled = false`.
  - `aws_glue_catalog.s3tables`: `name = "s3tablescatalog"`, `federated_catalog { identifier = <same ARN>, connection_name = "aws:s3tables" }`, no `create_database_default_permissions` or `create_table_default_permissions` blocks, `allow_full_table_external_data_access` from input, `depends_on` the resource registration.
- [ ] T017 Add `aws_lakeformation_data_lake_settings.this` to `modules/catalog-integration/main.tf`:
  - `admins = var.admin_principal_arns`, with empty default-permission blocks.
  - A `lifecycle.precondition` that `contains(var.admin_principal_arns, var.deployer_role_arn)`, with an error message explaining that the deploying role must be a data-lake admin.
  - A comment that this resource is authoritative for all data-lake settings.
- [ ] T018 Add `aws_lakeformation_lf_tag.classification` (key `classification`, values from `classification_values`) to `modules/catalog-integration/main.tf`. It `depends_on` the data-lake settings so the deployer is already an admin.
- [ ] T019 [P] Create `modules/catalog-integration/outputs.tf` with `data_access_role_arn`, `catalog_name`, `catalog_arn`, and `classification_tag_key`.
- [ ] T020 [P] Create `modules/catalog-integration/README.md`:
  - purpose and the one-owner-per-account-and-Region rule;
  - the authoritative data-lake settings warning;
  - the requirement that every secondary deployment's deploying role appears in `admin_principal_arns`;
  - how to `terraform import` an existing console-created `s3tablescatalog`;
  - a terraform-docs injection block.

### Encryption (FR-013, FR-022, FR-023)

- [ ] T021 Extend `data.aws_iam_policy_document.kms_key_policy` in `kms.tf` with:
  - `AllowS3TablesMaintenance`: service `maintenance.s3tables.amazonaws.com`, actions `kms:GenerateDataKey` and `kms:Decrypt`, condition `StringLike kms:EncryptionContext:aws:s3:arn = "arn:${partition}:s3tables:${region}:${account_id}:bucket/${name_prefix}-*/*"`. This is the documented S3 Tables pattern; without it `CreateTable` fails.
  - `AllowLakeFormationDataAccess`: principal `local.data_access_role_arn`, actions `kms:Decrypt` and `kms:GenerateDataKey`, conditioned on `kms:ViaService = s3tables.${region}.amazonaws.com`.
  - `AllowAthenaResults`: account principals via `kms:ViaService` for `s3.${region}.amazonaws.com` and `athena.${region}.amazonaws.com`.
  - Build these three statements in a separate `data "aws_iam_policy_document" "required_kms_statements"` and merge it into the key policy with `source_policy_documents`, so the same document can be output for consumer-supplied keys (T046).
- [ ] T022 Add compliance-only keys to `kms.tf`, created only when `local.compliance_designated`:
  - `aws_kms_key.zone` (`for_each = local.zones`) and `aws_kms_key.query_results`;
  - a matching `aws_kms_alias` for each;
  - the same statements as T021, with the maintenance condition scoped to that zone's bucket ARN;
  - a `ComplianceFrameworks = join(",", sort(var.compliance_frameworks))` tag on each.
  Expose `local.zone_kms_key_arn[zone]` and `local.query_results_kms_key_arn`, which fall back to `local.kms_key_id`.
- [ ] T023 Wire the submodule in `main.tf`: `module "catalog_integration"` with `count = var.enabled && var.catalog_integration_config.enabled ? 1 : 0` and `deployer_role_arn = local.deployer_role_arn`. Pass no key ARNs. Add a comment that the dependency is one-way: submodule role → root key policies.
- [ ] T024 Add `resource "terraform_data" "secondary_admin_guard"` to `main.tf`, with `count` matching the T002 data source and a `lifecycle.precondition` that `contains(data.aws_lakeformation_data_lake_settings.current[0].admins, local.deployer_role_arn)`. The error message tells the operator to add the role to the owning deployment's `admin_principal_arns`. Every grant in `permissions.tf` `depends_on` this resource.
  In the same file, add `resource "terraform_data" "required_tags_guard"` with `count = var.enabled && local.effective_config.required_tags_enforced ? 1 : 0` and three preconditions:
  - the cost-center and data-owner keys from `required_tags_config` are present and non-empty in `var.tags`;
  - the data-classification key is present in `var.data_tags`, with a value in `governance_config.classification_values`;
  - that value's index in `classification_values` is at least the index of every `namespace_classifications` value.
  Error messages name the missing or invalid key. Every storage resource in Phase 3 `depends_on` this guard.
- [ ] T025 Add a `check "regulated_data_in_baseline"` block to `main.tf` (FR-024): `assert { condition = !(local.compliance_designated && local.baseline_environment) }`, with an error message saying regulated data must not be loaded into Ephemeral, Development, or Testing.
- [ ] T026 **Checkpoint**: run `make check`, then `make lint`. Confirm TFLint actually ran; a skip does not count.

---

## Phase 3: Data & Query Tier (Table Storage, Grants, Query Engine)

**Purpose**: Zone table buckets and namespaces, Lake Formation grants, the results bucket,
workgroups, and persona policy documents. These depend on the Phase 2 keys, role, catalog, and
guards.

### Table storage (FR-001 to FR-003, FR-013)

- [ ] T027 Create `table-buckets.tf` with `aws_s3tables_table_bucket.zone` (`for_each = var.enabled ? local.zones : []`):
  - `name = local.table_bucket_names[each.key]`;
  - `encryption_configuration { sse_algorithm = "aws:kms", kms_key_arn = local.zone_kms_key_arn[each.key] }`;
  - `maintenance_configuration.iceberg_unreferenced_file_removal` enabled, with `unreferenced_days` and `non_current_days` from `local.effective_config`;
  - `force_destroy = local.effective_config.force_destroy`;
  - `tags = local.common_data_tags`.
- [ ] T028 Add `aws_s3tables_namespace.this` (`for_each = local.zone_namespaces`) to `table-buckets.tf`, with `table_bucket_arn` from T027.
- [ ] T029 Add `aws_s3tables_table_bucket_policy.zone` to `table-buckets.tf`, built with `aws_iam_policy_document`. It grants nothing and contains three deny statements:
  - `DenyInsecureTransport`: `s3tables:*` on the bucket and `bucket/*`, conditions `Bool aws:SecureTransport = false` and `Bool aws:PrincipalIsAWSService = false`;
  - `EnforceKMSEncryptionAlgorithm`: `s3tables:CreateTable` when `StringNotEqualsIfExists s3tables:SSEAlgorithm = aws:kms`;
  - `EnforceKMSEncryptionKey`: `s3tables:CreateTable` when `StringNotEqualsIfExists s3tables:KMSKeyArn = local.zone_kms_key_arn[zone]`.
  Use `IfExists` because tables created without an explicit encryption setting inherit the bucket default; a plain `StringNotEquals` would deny them. Comment this in the code.

### Governed access (FR-006 to FR-008)

- [ ] T030 Create `permissions.tf` with writer grants: for each writer and assigned zone namespace, an `aws_lakeformation_permissions` resource with `database { catalog_id = local.s3tables_catalog_id[zone], name = namespace }` and permissions `["CREATE_TABLE", "DESCRIBE", "ALTER"]`.
- [ ] T031 Add writer table-wildcard grants to `permissions.tf`: `table { catalog_id, database_name = namespace, wildcard = true }` with `concat(["SELECT", "INSERT", "DELETE", "ALTER", "DESCRIBE"], local.effective_config.writer_drop_enabled ? ["DROP"] : [])`. Add a comment explaining that Enhanced withholds `DROP` because dropped S3 Tables tables and their snapshots are unrecoverable.
- [ ] T032 Add reader table-wildcard grants to `permissions.tf`: `["SELECT", "DESCRIBE"]` for each reader's assigned zones and namespaces.
- [ ] T033 Add column-restricted reader grants to `permissions.tf`: `table_with_columns` grants from `governance_config.reader_column_grants`, with `SELECT`.
- [ ] T034 Add row-filtered reader grants to `permissions.tf`: an `aws_lakeformation_data_cells_filter` and a matching `data_cells_filter` grant (`SELECT`) for each `governance_config.reader_row_filters` entry.
- [ ] T035 Add classification-based grants to `permissions.tf`, enabled when `local.effective_config.classification_grants_enabled`:
  - attach the `classification` LF-tag to each namespace in `governance_config.namespace_classifications` with `aws_lakeformation_resource_lf_tags`;
  - grant each reader `lf_tag_policy` access (`resource_type = "TABLE"`, key `classification`, values from that reader's `classification_values`) with `["SELECT", "DESCRIBE"]`.
  Every grant in this file `depends_on` the catalog-integration module, `terraform_data.secondary_admin_guard`, and the namespaces. LF-tags can only be attached to existing resources, and Lake Formation supports tag-based access control on S3 Tables federated catalogs (verified in the AWS docs).

### Query engine (FR-010 to FR-012a)

- [ ] T036 Create `query.tf` with the results bucket `aws_s3_bucket.query_results`, named `${name_prefix}-athena-results-${account_id}`:
  - `force_destroy` from effective config;
  - `aws_s3_bucket_public_access_block` with all four settings true;
  - `aws_s3_bucket_ownership_controls` set to `BucketOwnerEnforced`;
  - `aws_s3_bucket_server_side_encryption_configuration` with `aws:kms`, `local.query_results_kms_key_arn`, and `bucket_key_enabled = true`;
  - `tags = local.common_data_tags`.
- [ ] T037 Add `aws_s3_bucket_lifecycle_configuration.query_results` to `query.tf`: expire after `local.effective_config.query_results_expiration_days` days, and abort incomplete multipart uploads after 1 day.
- [ ] T038 Add `aws_s3_bucket_policy.query_results` to `query.tf`, built with `aws_iam_policy_document`, with two deny statements:
  - `DenyInsecureTransport`: conditions `aws:SecureTransport = false` and `aws:PrincipalIsAWSService = false`;
  - `DenyUnencryptedUploads`.
- [ ] T039 Add `aws_athena_workgroup.this` (`for_each = toset(var.query_config.workgroups)`) to `query.tf`:
  - name `${name_prefix}-${each.key}`;
  - `configuration`: `enforce_workgroup_configuration = true`, `bytes_scanned_cutoff_per_query` from effective config, `publish_cloudwatch_metrics_enabled = local.effective_config.monitoring_enabled`, `engine_version { selected_engine_version = "Athena engine version 3" }`;
  - `result_configuration`: `output_location = "s3://${aws_s3_bucket.query_results[0].bucket}/${each.key}/"` (one prefix per workgroup, FR-012a), `encryption_configuration` with `SSE_KMS` and the results key ARN, and `expected_bucket_owner`;
  - `force_destroy` from effective config; `tags = local.common_tags`.
- [ ] T040 Create `iam-documents.tf` with persona IAM policy documents. They are outputs only, never attached. Create one `data "aws_iam_policy_document"` per writer and per reader (`for_each` over `governance_config.writers` / `readers`), each scoped to that entry's `workgroups`:
  - `glue:GetCatalog*`, `glue:GetDatabase*`, `glue:GetTable*`, `lakeformation:GetDataAccess`;
  - Athena `StartQueryExecution`, `GetQueryExecution`, `GetQueryResults`, `StopQueryExecution` on the bound `aws_athena_workgroup.this[wg].arn` only;
  - `s3:GetObject` and `s3:PutObject` on `${results_bucket_arn}/${wg}/*` for bound workgroups only, plus `s3:GetBucketLocation` on the bucket;
  - `kms:Decrypt` and `kms:GenerateDataKey` on the results key, via `kms:ViaService`.
  - Writers additionally get `glue:CreateTable` and `glue:UpdateTable`, plus `glue:DeleteTable` only when `local.effective_config.writer_drop_enabled`.
- [ ] T041 **Checkpoint**: run `make check`, then `make lint`. Then run `make plan-examples` against a sandbox account and confirm the plan contains the table buckets, namespaces, grants, and workgroups, with no errors.

---

## Phase 4: Operations Tier (Monitoring, Cost, Deployer, Outputs)

**Purpose**: Alarms, cost estimate, the least-privilege deployer policy, and the module contract
outputs. These depend on the Phase 3 resources.

- [ ] T042 [P] Extend `alarms.tf` (FR-017). When `local.effective_config.alarms_enabled`, add these `aws_cloudwatch_metric_alarm` resources per workgroup, routed to `local.alarm_sns_topic_arn`:
  - `query_failures`: namespace `AWS/Athena`, metric `TotalExecutionTime`, statistic `SampleCount`, dimensions `WorkGroup` and `QueryState = FAILED`, threshold `> 0` over 5 minutes;
  - `scanned_bytes`: metric `ProcessedBytes`, statistic `Sum`, period `86400`, threshold `local.scanned_bytes_alarm_threshold`.
- [ ] T043 [P] Extend `modules/cost-estimation/variables.tf`, `modules/cost-estimation/main.tf`, `modules/cost-estimation/locals.tf`, and `modules/cost-estimation/outputs.tf` (FR-018):
  - new inputs `table_bucket_count`, `s3_bucket_count`, `athena_workgroup_count`, and `compliance_kms_key_count`;
  - Price List lookups for the S3 Tables storage and maintenance rates, the Athena per-TB rate, and S3 Standard storage;
  - outputs: idle fixed monthly cost, a separate `compliance_designation_monthly_cost` line, and per-unit usage rates.
- [ ] T044 Update `cost-estimation.tf` to pass the new counts. `kms_key_count` = shared key + `length(zones) + 1` when designated; alarm count = 2 × workgroups when alarms are enabled.
- [ ] T045 [P] Extend `modules/deployer/main.tf` and `modules/deployer/variables.tf` (FR-019) with least-privilege statements that list every action explicitly (no wildcards inside action names):
  - **s3tables**: `CreateTableBucket`, `DeleteTableBucket`, `GetTableBucket`, `ListTableBuckets`, `PutTableBucketEncryption`, `GetTableBucketEncryption`, `PutTableBucketMaintenanceConfiguration`, `GetTableBucketMaintenanceConfiguration`, `PutTableBucketPolicy`, `GetTableBucketPolicy`, `DeleteTableBucketPolicy`, `CreateNamespace`, `DeleteNamespace`, `GetNamespace`, `ListNamespaces`, `TagResource`, `UntagResource`, `ListTagsForResource`.
  - **glue**: `CreateCatalog`, `GetCatalog`, `UpdateCatalog`, `DeleteCatalog`.
  - **lakeformation**: `RegisterResource`, `DeregisterResource`, `DescribeResource`, `PutDataLakeSettings`, `GetDataLakeSettings`, `GrantPermissions`, `RevokePermissions`, `ListPermissions`, `CreateLFTag`, `DeleteLFTag`, `GetLFTag`, `UpdateLFTag`, `AddLFTagsToResource`, `RemoveLFTagsFromResource`, `GetResourceLFTags`, `CreateDataCellsFilter`, `DeleteDataCellsFilter`, `GetDataCellsFilter`.
  - **athena**: `CreateWorkGroup`, `UpdateWorkGroup`, `DeleteWorkGroup`, `GetWorkGroup`, `TagResource`, `UntagResource`, `ListTagsForResource`.
  - **s3**: bucket configuration actions on `${name_prefix}-athena-results-*`.
  - **kms**: key management.
  - **iam**: `CreateRole`, `DeleteRole`, `GetRole`, `PutRolePolicy`, `DeleteRolePolicy`, `GetRolePolicy`, `TagRole`, and `PassRole` on `${name_prefix}-lf-s3tables-access`, with `iam:PassedToService = lakeformation.amazonaws.com`.
  - **cloudwatch**: `PutMetricAlarm`, `DeleteAlarms`, `DescribeAlarms`.
- [ ] T046 Rewrite `outputs.tf` (FR-020):
  - `table_bucket_arns` (map by zone), `namespaces` (map), `s3tables_catalog_ids` (map by zone), `catalog_name`;
  - `data_access_role_arn`, `workgroup_names` (map), `query_results_bucket_arn`, `query_results_locations` (map by workgroup);
  - `kms_key_arn`, `zone_kms_key_arns`;
  - `writer_policy_json` and `reader_policy_json`, each a map keyed by the `governance_config` entry;
  - `compliance_frameworks`;
  - `required_kms_key_policy_json`: the T021 `required_kms_statements` document, for consumers who supply their own key (FR-013).
  Keep the existing alarm and cost outputs. Every output returns empty or null values when `enabled = false`.
- [ ] T047 **Checkpoint**: run `make check`, then `make lint`, then `make docs`. Confirm terraform-docs ran and updated `README.md` and `modules/*/README.md`.

---

## Phase 5: Examples & Contract Tests

**Purpose**: Plan-only coverage of every module promise in spec.md Success Criteria (Code
Validation).

- [ ] T048 [P] Update `examples/defaults/main.tf`, `variables.tf`, `terraform.auto.tfvars`, and `outputs.tf`: required inputs only, with catalog integration enabled and `environment_type = "Development"`.
  - `name_prefix`.
  - `admin_principal_arns`: a variable defaulting to `[]`. When it's empty, `main.tf` passes `[data.aws_iam_session_context.current.issuer_arn]`, which means the role running the plan. The data sources `aws_caller_identity` and `aws_iam_session_context` are declared in the example.
  This makes the FR-009 precondition pass for whoever runs `make test`.
- [ ] T049 [P] Update `examples/complete/main.tf`, `variables.tf`, `terraform.auto.tfvars`, and `outputs.tf`. Set every input:
  - two zones with namespaces and `namespace_classifications`;
  - writers bound to one workgroup and readers bound to the other, plus column and row grants and reader `classification_values`;
  - two workgroups;
  - `compliance_frameworks` set through a variable, default `["HIPAA"]`;
  - `environment_type = "Production"`;
  - alarms enabled;
  - `tags` with `CostCenter` and `DataOwner`, and `data_tags` with `DataClassification = "restricted"`;
  - the same caller-default admin list pattern as T048.
- [ ] T050 [P] Create `examples/secondary/` (`main.tf`, `variables.tf`, `outputs.tf`, `versions.tf`, `terraform.auto.tfvars`, `README.md`) with `catalog_integration_config = { enabled = false, data_access_role_arn = <variable> }`. The README states that the deploying role must be in the owning deployment's admin list.
- [ ] T051 Update `test/defaults_test.go` to assert:
  - present: `module.main.aws_s3tables_table_bucket.zone["raw"]` and `["curated"]`, `module.main.module.catalog_integration[0].aws_glue_catalog.s3tables`, `module.main.module.catalog_integration[0].aws_lakeformation_data_lake_settings.this`, and `module.main.aws_athena_workgroup.this["primary"]`;
  - absent: any `aws_kms_key.zone` and any `aws_cloudwatch_metric_alarm`, because monitoring and alarms are off in Development.
- [ ] T052 Update `test/complete_test.go`:
  - `TestTerraformCompleteExample` asserts the data cells filter, LF-tag attachments and grants, and both alarms per workgroup are present.
  - It also asserts that writer table grants do not contain `"DROP"` (Production), and that each workgroup's `output_location` ends in `/<workgroup>/`.
  - It runs `terraform.OutputJson` on `reader_policy_json` and asserts each reader's S3 resources cover only its bound workgroups' prefixes, and its Athena resources only its bound workgroup ARNs.
  - Keep `TestEnabledFalse`, asserting no resources to add.
- [ ] T053 [P] Create `test/secondary_test.go` with `TestSecondaryExampleSkipsAccountIntegration`. It asserts the plan has no `module.catalog_integration` resources but contains the table buckets, grants, and `terraform_data.secondary_admin_guard[0]`.
  Document in `test/README.md` that the role running `make test-secondary` must already be a Lake Formation data-lake admin in the sandbox account, because the guard reads the live settings during plan.
- [ ] T054 [P] Create `test/validation_test.go` with table-driven, plan-expected-failure cases, each asserting the error message:
  - an unknown zone in `governance_config`;
  - an empty principal ARN;
  - `bytes_scanned_cutoff_per_query = 1000`;
  - an unknown compliance framework;
  - integration disabled without a role ARN;
  - a reader bound to an undeclared workgroup;
  - a classification value not in `classification_values`;
  - `environment_type = "Production"` without the `DataOwner` tag;
  - a zone named `raw_v2` (underscore);
  - a zone long enough that the bucket name exceeds 63 characters;
  - a `DataClassification` less sensitive than a namespace classification;
  - `environment_type = "None"` with `required_tags_config.enforced = null`.
  Add `TestRegulatedDataInBaselineWarns`, which plans `examples/complete` with `environment_type = "Development"` and asserts the plan succeeds and its output contains the `regulated_data_in_baseline` check warning (FR-024).
- [ ] T055 Add `TestComplianceDesignationGatesKeys` to `test/complete_test.go`. It is table-driven over `[]`, `["SOC2"]`, `["HIPAA"]`, and `["PCI_DSS"]`, planning `examples/complete` (Production) for each:
  - with an empty designation, assert there is no `aws_kms_key.zone` or `aws_kms_key.query_results`;
  - with each framework, assert exactly `aws_kms_key.zone["raw"]`, `aws_kms_key.zone["curated"]`, and `aws_kms_key.query_results`.
- [ ] T056 Add `test-secondary` and `test-validation` targets to `Makefile`, following the existing `test-complete` pattern. Add both to `make help` output via `##` comments.
- [ ] T057 **Checkpoint**: run `make test`. All plan-only tests must pass. Fix failures and rerun the single failing target (`make test-defaults`, `make test-complete`, `make test-enabled-false`, `make test-secondary`, or `make test-validation`).

---

## Phase N: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, security scan, CI alignment, cost check, full gates, and sandbox
verification of the spec's operational success criteria.

- [ ] T058 Update `README.md`:
  - replace the "(in progress)" feature markers;
  - add an "Account-level integration ownership" section: the FR-005/FR-009 singleton, authoritative settings, `terraform import`, and the requirement that secondary deployers be data-lake admins;
  - add an "Audit trail prerequisites" section listing the CloudTrail management and data events (`AWS::S3Tables::Table`) and retention (90 days, or 1 year when designated);
  - add a "Table maintenance" section saying writers must keep compaction and snapshot management enabled;
  - add a "Compliance designation" section;
  - add a "Required tags" section covering the Enhanced keys, enforcement, and `required_tags_config`;
  - add a "Bring your own KMS key" section: merge `required_kms_key_policy_json` into the key policy before applying;
  - state that secondary deployments must use the owner's `classification_values`;
  - add a "Query isolation" section on binding personas to workgroups.
- [ ] T059 [P] Update `AGENTS.md` Components to match the implemented files: `table-buckets.tf`, `permissions.tf`, `query.tf`, `iam-documents.tf`, and `modules/catalog-integration`.
- [ ] T060 [P] Add a `security-scan` target to `Makefile` that runs `checkov --framework terraform -d .` with the same flags as `.github/workflows/terraform-ci.yml`, then run `make security-scan`. Fix or suppress every HIGH or CRITICAL finding with an inline justification comment.
- [ ] T061 Update `.github/workflows/terraform-ci.yml` so CI runs the same gates as local validation:
  - install TFLint and terraform-docs;
  - replace the direct `terraform fmt`, `terraform validate`, `checkov`, and `go test` steps with `make check`, `make lint`, `make docs` (failing if `git diff --exit-code` shows changes), `make security-scan`, and `make test`;
  - remove the commented-out TFLint block.
  This depends on T060.
- [ ] T062 Run `make plan-examples` and review the `monthly_cost_estimate` and `cost_breakdown` outputs. The defaults example must be at most $5 idle; complete, which is Production with HIPAA, must be at most $10 plus $10.
- [ ] T063 Run `make pre-commit`, then `make validate`. Both must pass with TFLint and terraform-docs installed, and no files may be left modified.
- [ ] T064 Sandbox end-to-end (Development first, then Production configuration): run `make apply-examples` and record the plan and apply durations against the 3-minute and 15-minute targets. Then verify the spec success criteria:
  - a writer creates a table through Athena without stating encryption; the table bucket policy allows it, and the table uses the zone key;
  - a writer-created table is queryable by a reader within 5 minutes;
  - a classification-bound reader can read a namespace tagged with an allowed value and is denied on a namespace with a higher classification;
  - in Production, a writer's `DROP TABLE` is denied;
  - a query over the cutoff is cancelled;
  - a reader cannot read another workgroup's results prefix;
  - unreferenced-file removal is reported enabled on each zone, and the writer-created table reports compaction and snapshot management enabled;
  - the table restores to an earlier snapshot within the RPO window;
  - a forced query failure triggers the `query_failures` alarm and delivers to the SNS topic;
  - table data events appear in the account trail.
  Then run `make destroy-examples`.
- [ ] T065 Record T064 results and any deviations in `specs/001-governed-lakehouse/plan.md` under Complexity Tracking.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1: Setup** has no dependencies and blocks every later phase. Inputs and locals are
  referenced everywhere.
- **Phase 2: Foundation** depends on Phase 1 and **blocks Phase 3**. Table buckets need keys;
  grants need the federated catalog, data-lake settings, LF-tag, and admin guards.
- **Phase 3: Data & Query** depends on Phase 2.
- **Phase 4: Operations** depends on Phase 3. Alarms need workgroups; outputs, cost counts, and
  the deployer need the final resource set.
- **Phase 5: Examples & Tests** depends on Phase 4. Examples consume the final inputs and outputs.
- **Phase N: Polish** depends on all phases.

### Resource Dependency Order (at apply time)

1. Lake Formation data-access role. It has no key dependencies.
2. KMS keys (shared, and per-zone and results when designated). Their policies reference the role
   ARN.
3. Lake Formation resource registration (`bucket/*`, with federation).
4. `s3tablescatalog` Glue catalog.
5. Data-lake settings (admins, empty defaults). For secondary deployments, the admin guard runs
   instead.
6. Classification LF-tag.
7. Table buckets.
8. Namespaces.
9. Table bucket policies.
10. Lake Formation grants, LF-tag attachments, and data cells filters.
11. Results bucket and its controls.
12. Athena workgroups.
13. CloudWatch alarms.

### Task-Level Dependencies

- T003 to T008 edit the same `variables.tf` and must run sequentially. T009 and T010 depend on
  them; T010 also depends on T002.
- T015 → T016 → T017 → T018: role → registration → catalog → settings → LF-tag.
- T021 and T022 edit `kms.tf` and depend on T019 for the role ARN output. T023 to T025 edit
  `main.tf` sequentially.
- T029 depends on T022 for the zone key ARNs.
- T030 to T035 edit `permissions.tf` sequentially, and depend on T018, T024, and T028.
- T036 to T039 edit `query.tf` sequentially. T040 depends on T036 and T039.
- T051 to T055 depend on T048 to T050. T061 depends on T060.

### Parallel Opportunities

| Phase | Parallel group | Why safe |
|-------|----------------|----------|
| 1 | T002, T011 | Different files (`dependencies.tf`, examples) with no shared symbols |
| 2 | T013, T014, T019, T020 | Separate submodule files; scaffolding only |
| 4 | T042, T043, T045 | `alarms.tf`, `modules/cost-estimation/`, `modules/deployer/` are independent |
| 5 | T048, T049, T050, then T053, T054 | Separate example directories and new test files |
| N | T059, T060 | Docs and Makefile target are independent |

### Validation Checkpoints

| Checkpoint | After | Make targets |
|------------|-------|--------------|
| T012 | Setup | `make check` |
| T026 | Foundation | `make check`, `make lint` |
| T041 | Data & Query | `make check`, `make lint`, `make plan-examples` |
| T047 | Operations | `make check`, `make lint`, `make docs` |
| T057 | Examples & Tests | `make test` (single targets for reruns) |
| T063 | Polish | `make pre-commit`, `make validate` |

### Environment Order

Development (the defaults example) → Production configuration (the complete example, with a
compliance designation) in the sandbox account (T064). Consumers then promote through their own
Ephemeral/Development/Testing → UAT → Production → MissionCritical deployments by changing only
`environment_type`, the required tags, and, where data is regulated, `compliance_frameworks`.

## Notes

- [P] tasks touch different files and have no dependencies on other [P] tasks in the same phase.
- Commit after each checkpoint passes, using a Conventional Commit message.
- Plan-only tests need AWS credentials for provider data sources. `make test` runs `check-aws`
  first.
- Do not create tables, identities, VPC endpoints, or a CloudTrail trail; all are out of scope.
