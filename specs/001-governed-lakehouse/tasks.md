---

description: "Implementation tasks for the Governed Multi-Tenant Lakehouse Platform module"
---

# Tasks: Governed Multi-Tenant Lakehouse Platform

**Input**: Design documents from `/specs/001-governed-lakehouse/`
**Prerequisites**: plan.md, spec.md (no research.md, architecture.md, modules.md, or quickstart.md)

**Organization**: Tasks are grouped by tier, following the dependency order Foundation →
Platform Data & Query → Tenant → Operations. This module has **no network tier**: every service
is a Regional AWS public endpoint (plan.md, Networking). The phase that would hold networking
instead holds the encryption, identity, account-level catalog, and LF-tag foundation that every
data resource depends on.

**Revision**: Regenerated 2026-10-14 (principles v1.3.0) with these design points:
- Tenants are S3 Tables namespaces, provisioned by `modules/tenant`. The root applies it per
  `tenants` entry, or a tenant pipeline calls it standalone through the `platform_contract`.
- Access is granted through LF-tag expressions over a per-tenant tag and platform sensitivity,
  per-regulation, and domain tags.
- The column-to-table roll-up is applied by pipelines.

## Format: `[ID] [P?] Description`

- **[ID]**: Sequential task number (T001, T002, T003...)
- **[P]**: Can run in parallel (different files, no dependencies) - optional
- **Description**: Clear action with exact file path included

## Path Conventions

- The module lives at the **repository root** (Registry layout; plan.md, Structure Decision). Do
  not use `iac/`.
- Submodules go in `modules/<name>/`, examples in `examples/<name>/`, tests in `test/`.
- **All validation runs through `make` targets** (principles, Validate Before Merge). Never invoke
  `terraform`, `tflint`, `terraform-docs`, or `go test` directly.
- Every policy uses `aws_iam_policy_document`, never inline JSON. Never build an ARN from a naming
  convention to reference another resource; use resource attributes, module outputs, the
  platform contract, or inputs. Follow the comment and section header style in `AGENTS.md`.
- **Platform/tenant boundary**: the root module never creates a tenant-scoped resource, and
  `modules/tenant` never creates or modifies a platform-owned resource. The tenant submodule reads
  platform values only from `var.platform`.

---

## Phase 1: Setup

**Purpose**: Version constraints, root inputs, environment defaults, and shared data sources.

- [x] T001 Update `versions.tf`: set the `hashicorp/aws` constraint to `>= 6.59.0, < 7.0.0` and keep `configuration_aliases = [aws.pricing]` and `required_version = ">= 1.11"`. Apply the same AWS constraint in `examples/defaults/versions.tf`, `examples/complete/versions.tf`, `modules/cost-estimation/versions.tf`, and `modules/deployer/versions.tf`.
- [x] T002 [P] Add two data sources to `dependencies.tf`:
  - `data "aws_iam_session_context" "current"` with `arn = data.aws_caller_identity.current.arn`, which resolves the deploying role ARN;
  - `data "aws_lakeformation_data_lake_settings" "current"`, with `count = var.enabled && !var.catalog_integration_config.enabled ? 1 : 0`, which secondary deployments use for their admin check.
- [x] T003 Edit `variables.tf`:
  - Remove `networktags_name`.
  - Add `zones_config` as `object({ zones = list(string) })`, defaulting to `zones = ["raw", "curated", "consumption"]`.
  - Validate `zones_config`:
    - zone names match `^[a-z0-9]([a-z0-9-]*[a-z0-9])?$` (no underscores; they become table bucket names);
    - `length(var.name_prefix) + 1 + length(zone) <= 63` for every zone (cross-variable check);
    - the zones list is unique and non-empty.
- [x] T004 Add `catalog_integration_config` to `variables.tf` (FR-007):
  - Type `object({ enabled = bool, data_access_role_arn = string, allow_full_table_external_data_access = bool })`, default `{ enabled = true, data_access_role_arn = "", allow_full_table_external_data_access = false }`.
  - Validation: `data_access_role_arn` must be a valid IAM role ARN when `enabled = false`, and empty when `enabled = true`.
- [x] T005 Add `governance_config` to `variables.tf` (FR-009, FR-010, FR-013):
  - `admin_principal_arns = list(string)`: the platform admins.
  - `sensitivity_values = list(string)`, defaulting to `["public", "internal", "confidential", "restricted"]`, ordered least to most sensitive.
  - `regulation_values = list(string)`, defaulting to `["gdpr", "hipaa", "pci_dss"]`.
  - `domain_values = list(string)`, defaulting to `["general"]`. The first entry is the default tenant domain.
  - `shared_storage_classification = string`, nullable. Null means the most sensitive `sensitivity_values` entry. It is validated as a member of `sensitivity_values` (FR-022).
  - `writer_drop_enabled`: nullable bool, default `null` so the environment default applies.
  - Validation:
    - ARNs are well formed;
    - `sensitivity_values` and `domain_values` are unique and non-empty;
    - `regulation_values` is unique, has at most 20 entries, and matches `^[a-z0-9_]{1,40}$` (50 LF-tags per resource);
    - `domain_values` matches `^[a-z0-9_-]{1,64}$`.
- [x] T006 Add `tenants` to `variables.tf` (FR-001, FR-001a, FR-002): `map(object({ ... }))`, default `{}`. This task is the source of truth for the tenant object type, with these attributes:
  - `owner = string`, `sensitivity = string`;
  - `regulations = optional(set(string), [])`, `domain = optional(string)`;
  - `zones = optional(list(string))`;
  - `compliance_frameworks = optional(set(string), [])`;
  - `stewards = optional(list(string), [])`;
  - `writers = optional(map(object({ principal_arn = string, zones = optional(list(string)) })), {})`;
  - `readers = optional(map(object({ principal_arn = string, zones = optional(list(string)), sensitivity_values = list(string), regulations = optional(set(string), []), domains = optional(list(string)) })), {})`;
  - `row_filters = optional(map(object({ reader = string, zone = string, table = string, row_filter_expression = string, column_names = optional(list(string)) })), {})`;
  - `bytes_scanned_cutoff_per_query = optional(number)`, `scanned_bytes_alarm_threshold = optional(number)`;
  - `dedicated_storage = optional(bool, false)`.
  Validate only that keys match `^[a-z][a-z0-9]{1,31}$`. All other tenant validation runs inside the submodule (T031), so both entry points enforce identical rules. Add a comment saying so, and that the type must stay identical to `modules/tenant` `var.tenant`.
- [x] T007 Add `query_config` to `variables.tf` (FR-015 to FR-017):
  - Type `object({ bytes_scanned_cutoff_per_query = number, results_expiration_days = number })`, with both `null` by default so environment defaults apply.
  - Validation: cutoff `>= 10485760` when not null, and expiration between 1 and 3650 days when not null.
  - Add `scanned_bytes_threshold` to the `alarms_config` object (number, default `null`).
  - Add `storage_config` as `object({ table_storage_class = string })`, default `{ table_storage_class = null }`, validated against `["STANDARD", "INTELLIGENT_TIERING"]` (FR-005).
- [x] T008 Add `required_tags_config` to `variables.tf` (FR-022):
  - Type `object({ enforced = bool, cost_center_key = string, data_owner_key = string, data_classification_key = string, tenant_key = string })`.
  - Default `{ enforced = null, cost_center_key = "CostCenter", data_owner_key = "DataOwner", data_classification_key = "DataClassification", tenant_key = "Tenant" }`.
  - Validate only that the key names are non-empty. Enforcement itself is the T023 precondition.
- [x] T009 Extend `locals.environment_defaults` in `locals.tf` with the keys and values from the plan.md Environment Configuration table:
  - `query_results_expiration_days`, `bytes_scanned_cutoff_per_query`, `unreferenced_days`, `non_current_days`, `force_destroy`;
  - `writer_drop_enabled`, `required_tags_enforced`, `scanned_bytes_alarm_threshold`;
  - `table_storage_class`: `STANDARD` for Baseline and `INTELLIGENT_TIERING` for Enhanced (FR-005).
  - The `None` entry reads every key from its `*_config` input, and a validation requires the nullable inputs to be non-null when `environment_type = "None"`.
  - For other environment types, a non-null input overrides the default (`coalesce(input, default)`).
- [x] T010 Add derived platform locals to `locals.tf`:
  - `zones` (set) and `zone_bucket_names` (`"${local.name_prefix}-${zone}"`);
  - `regulation_designation = { hipaa = "HIPAA", pci_dss = "PCI_DSS" }`;
  - `baseline_environment = contains(["Ephemeral", "Development", "Testing"], var.environment_type)`;
  - `deployer_role_arn = data.aws_iam_session_context.current.issuer_arn`;
  - `data_access_role_arn`: `module.catalog_integration[0].data_access_role_arn` when integration is enabled, otherwise `var.catalog_integration_config.data_access_role_arn`;
  - `shared_storage_classification = coalesce(var.governance_config.shared_storage_classification, local.sensitivity_values[length(local.sensitivity_values) - 1])`. Comment that it is a platform setting, never computed from tenants: a tenant change must not modify platform resources (FR-001), and standalone tenants are invisible to the platform (FR-022).
- [x] T011 [P] Remove the `networktags_name` argument from `examples/complete/main.tf`, `examples/complete/variables.tf`, `examples/defaults/main.tf`, `examples/defaults/variables.tf`, and the `terraform.auto.tfvars` files that set it.
- [x] T012 **Checkpoint**: run `make check`. Formatting and `validate` must pass for all submodules and examples before Phase 2.

---

## Phase 2: Foundation Tier (Encryption, Identity, Catalog Integration, Platform LF-Tags)

**Purpose**: The platform key, the Lake Formation data-access role, the federated
`s3tablescatalog`, the data-lake settings, the admin guards, and the platform-owned LF-tags.

**⚠️ CRITICAL**: Phase 3 cannot start until this phase passes its checkpoint.

### Catalog-integration submodule (FR-006, FR-007, FR-013)

- [x] T013 [P] Create `modules/catalog-integration/versions.tf` with `required_version = ">= 1.11"` and `hashicorp/aws >= 6.59.0, < 7.0.0`.
- [x] T014 [P] Create `modules/catalog-integration/variables.tf` with these inputs, each with a description and validation. It takes no KMS key ARNs, which avoids a dependency cycle with key policies.
  - `name_prefix`, `account_id`, `partition`, `region`, `tags`;
  - `admin_principal_arns` (list, non-empty);
  - `deployer_role_arn` (string);
  - `allow_full_table_external_data_access` (bool).
- [x] T015 Create `modules/catalog-integration/iam.tf` with the Lake Formation data-access role `aws_iam_role.lf_s3tables_access`, named `${name_prefix}-lf-s3tables-access`:
  - Trust: `lakeformation.amazonaws.com` for `sts:AssumeRole`, `sts:SetContext`, `sts:SetSourceIdentity`, conditioned on `aws:SourceAccount`.
  - Inline policy, via `aws_iam_policy_document`:
    - S3 Tables data and metadata actions on `arn:${partition}:s3tables:${region}:${account_id}:bucket/*`;
    - `kms:Decrypt` and `kms:GenerateDataKey` on `*`, conditioned on `kms:ViaService = s3tables.${region}.amazonaws.com` and `aws:ResourceAccount = ${account_id}`. The platform and tenant key policies do the per-key scoping.
- [x] T016 Create `modules/catalog-integration/main.tf` with:
  - `aws_lakeformation_resource.s3tables`: `arn = "arn:${partition}:s3tables:${region}:${account_id}:bucket/*"`, the role ARN from T015, `with_federation = true`, `hybrid_access_enabled = false`.
  - `aws_glue_catalog.s3tables`: `name = "s3tablescatalog"`, `federated_catalog { identifier = <same ARN>, connection_name = "aws:s3tables" }`, no default-permission blocks, `allow_full_table_external_data_access` from input, `depends_on` the resource registration.
- [x] T017 Add `aws_lakeformation_data_lake_settings.this` to `modules/catalog-integration/main.tf`:
  - `admins = var.admin_principal_arns`, with empty default-permission blocks.
  - A `lifecycle.precondition` that `contains(var.admin_principal_arns, var.deployer_role_arn)`, with an error message explaining that the deploying role must be a data-lake admin.
  - A comment that this resource is authoritative for all data-lake settings, so tenant pipeline roles must be listed here.
- [x] T018 [P] Create `modules/catalog-integration/outputs.tf` with `data_access_role_arn`, `catalog_name`, and `catalog_arn`.
- [x] T019 [P] Create `modules/catalog-integration/README.md`:
  - purpose and the one-owner-per-account-and-Region rule;
  - the authoritative data-lake settings warning;
  - the requirement that every secondary deployment's deploying role and every tenant pipeline role appear in `admin_principal_arns`;
  - how to `terraform import` an existing console-created `s3tablescatalog`;
  - a terraform-docs injection block.

### Platform encryption (FR-019, FR-029)

- [x] T020 Extend `data.aws_iam_policy_document.kms_key_policy` in `kms.tf` with:
  - `AllowS3TablesMaintenance`: service `maintenance.s3tables.amazonaws.com`, actions `kms:GenerateDataKey` and `kms:Decrypt`, condition `StringLike kms:EncryptionContext:aws:s3:arn = "arn:${partition}:s3tables:${region}:${account_id}:bucket/${name_prefix}-*/*"`.
  - `AllowLakeFormationDataAccess`: principal `local.data_access_role_arn`, actions `kms:Decrypt` and `kms:GenerateDataKey`, conditioned on `kms:ViaService = s3tables.${region}.amazonaws.com`.
  - `AllowAthenaResults`: account principals via `kms:ViaService` for `s3.${region}.amazonaws.com` and `athena.${region}.amazonaws.com`.
  - Build these three statements in a separate `data "aws_iam_policy_document" "required_kms_statements"` and merge it with `source_policy_documents`, so the same document can be output for consumer-supplied keys (T058).
- [x] T021 Wire the catalog-integration submodule in `main.tf`: `module "catalog_integration"` with `count = var.enabled && var.catalog_integration_config.enabled ? 1 : 0` and `deployer_role_arn = local.deployer_role_arn`. Pass no key ARNs. Add a comment that the dependency is one-way: submodule role → key policies.

### Guards and platform LF-tags (FR-009, FR-022)

- [x] T022 Add `resource "terraform_data" "secondary_admin_guard"` to `main.tf`, with `count` matching the T002 data source and a `lifecycle.precondition` that `contains(data.aws_lakeformation_data_lake_settings.current[0].admins, local.deployer_role_arn)`. The error message tells the operator to add the role to the owning deployment's `admin_principal_arns`.
- [x] T023 Add `resource "terraform_data" "required_tags_guard"` to `main.tf` with `count = var.enabled && local.effective_config.required_tags_enforced ? 1 : 0`. Its preconditions check that the cost-center and data-owner keys from `required_tags_config` are present and non-empty in `var.tags`. Error messages name the missing key. Every storage resource in Phase 3 `depends_on` this guard.
- [x] T024 Create `lf-tags.tf` with the platform-owned LF-tags:
  - `aws_lakeformation_lf_tag.sensitivity`: key `"${local.name_prefix}-sensitivity"`, values `governance_config.sensitivity_values`;
  - `aws_lakeformation_lf_tag.regulation` (`for_each = var.enabled ? toset(regulation_values) : []`): key `"${local.name_prefix}-reg-${each.key}"`, values `["no", "yes"]`. Comment that one key per regulation is required because a resource holds one value per LF-tag key;
  - `aws_lakeformation_lf_tag.domain`: key `"${local.name_prefix}-domain"`, values `governance_config.domain_values`.
  All `depends_on` the catalog-integration module and `terraform_data.secondary_admin_guard`. Add a comment that there is deliberately no shared tenant tag, because each tenant submodule owns `${prefix}-tenant-<name>` (FR-001a).
- [x] T025 **Checkpoint**: run `make check`, then `make lint`. Confirm TFLint actually ran; a skip does not count.

---

## Phase 3: Platform Data & Query Tier (Shared Storage, Results, Contract)

**Purpose**: Shared zone buckets, the shared results bucket, and the `platform_contract` that the
tenant submodule consumes.

- [x] T026 Create `table-buckets.tf` (FR-004, FR-005) with `aws_s3tables_table_bucket.zone` (`for_each = var.enabled ? local.zones : []`):
  - `name = local.zone_bucket_names[each.key]`;
  - `encryption_configuration { sse_algorithm = "aws:kms", kms_key_arn = local.kms_key_arn }`;
  - `maintenance_configuration.iceberg_unreferenced_file_removal` enabled, with `unreferenced_days` and `non_current_days` from `local.effective_config`;
  - the default storage class for new tables set to `local.effective_config.table_storage_class`, using the provider argument confirmed in T049;
  - `force_destroy = local.effective_config.force_destroy`;
  - `tags = merge(local.common_data_tags, { (data_classification_key) = local.shared_storage_classification })`.

  Add `aws_s3tables_table_bucket_policy.zone`, built with `aws_iam_policy_document`. It grants nothing and has three deny statements:
  - `DenyInsecureTransport`: `s3tables:*` on the bucket and `bucket/*`, conditions `Bool aws:SecureTransport = false` and `Bool aws:PrincipalIsAWSService = false`;
  - `EnforceKMSEncryptionAlgorithm`: `s3tables:CreateTable` when `StringNotEqualsIfExists s3tables:SSEAlgorithm = aws:kms`;
  - `EnforceKMSEncryptionKey`: `s3tables:CreateTable` when `StringNotEqualsIfExists s3tables:KMSKeyArn` differs from the platform key.
  Comment why `IfExists` is used, and why bucket-default inheritance forces dedicated buckets for designated tenants.
- [x] T027 Create `query.tf` (FR-016) with the shared results bucket `aws_s3_bucket.query_results`, named `${name_prefix}-athena-results-${account_id}`:
  - a public access block with all four settings true;
  - `BucketOwnerEnforced` ownership;
  - SSE `aws:kms` with `local.kms_key_arn` and `bucket_key_enabled = true`;
  - a lifecycle that expires objects after `local.effective_config.query_results_expiration_days` days and aborts incomplete multipart uploads after 1 day;
  - a bucket policy denying non-TLS requests from non-service principals and unencrypted uploads;
  - `force_destroy` from effective config, and `tags = merge(local.common_data_tags, { (data_classification_key) = local.shared_storage_classification })`. Comment that results expire instead of tiering (FR-005).
  Comment that the policy has no per-tenant statements, so standalone tenants never edit it.
- [x] T028 Add `local.platform_contract` to `locals.tf` with exactly the fields in the plan.md platform contract table, including `cost_estimation_enabled`, `shared_storage_classification`, and `effective.table_storage_class`: identifiers, effective settings, zones with bucket names, ARNs, and catalog IDs, the LF-tag vocabulary, the regulation-designation map, the platform key, the data-access role, the results bucket, the SNS topic, and tag settings. Add the `platform_contract` output in `outputs.tf`, with a description stating that it is a versioned public interface (plan.md: Versioning). When `enabled = false` it is `null`.
- [x] T029 **Checkpoint**: run `make check`, then `make lint`.

---

## Phase 4: Tenant Submodule (`modules/tenant`, FR-001 to FR-003, FR-008 to FR-018, FR-027 to FR-030)

**Purpose**: The single implementation of a tenant, used by both entry points (FR-001a). Every
file below lives in `modules/tenant/`. Resource addresses must not depend on the entry point.

### Interface and locals

- [x] T030 [P] Create `modules/tenant/versions.tf` with `required_version = ">= 1.11"`, `hashicorp/aws >= 6.59.0, < 7.0.0`, and `configuration_aliases = [aws.pricing]`.
- [x] T031 Create `modules/tenant/variables.tf`:
  - `platform`: a typed `object` matching the plan.md contract (no `any`). Its validation requires non-empty `zones` and `lf_tags`.
  - `name`: string, validated against `^[a-z][a-z0-9]{1,31}$`.
  - `tenant`: the object type from T006, copied verbatim. A comment names T006 as the source of truth.
  - `tags`: map(string), default `{}`.
  - `verify_deployer_admin`: bool, default `true`.

  Cross-variable validation against `var.platform`, each error naming the tenant:
  - tenant and persona zones are platform zones, and persona zones are a subset of the tenant's zones;
  - every ARN matches `^arn:aws[a-z-]*:iam::[0-9]{12}:(role|user)/.+$`;
  - `sensitivity` is a platform sensitivity value, and reader levels rank at or below it;
  - `regulations` are platform regulations. A mapped regulation (`hipaa`, `pci_dss`) requires its designation in `compliance_frameworks`;
  - reader `regulations` are a subset of the tenant's;
  - `domain` and reader `domains` are platform domain values;
  - row filter `reader` and `zone` are declared;
  - `compliance_frameworks` is a subset of `["SOC2", "HIPAA", "PCI_DSS"]`;
  - cutoffs are `>= 10485760`;
  - when dedicated, every dedicated bucket name `${name_prefix}-${zone}-${name}` is at most 63 characters and does not equal any shared bucket name. When designated, the results bucket name also fits;
  - when not dedicated and not designated, `sensitivity` ranks at or below `var.platform.shared_storage_classification` (FR-022).
- [x] T032 [P] Create `modules/tenant/dependencies.tf` with:
  - `aws_caller_identity` and `aws_iam_session_context`;
  - `aws_lakeformation_data_lake_settings` with `count = var.verify_deployer_admin ? 1 : 0`.
- [x] T033 Create `modules/tenant/locals.tf`:
  - `zones`: `var.tenant.zones`, or all platform zones;
  - `designated = length(var.tenant.compliance_frameworks) > 0`;
  - `dedicated = var.tenant.dedicated_storage || local.designated`;
  - `domain`: `coalesce(var.tenant.domain, var.platform.lf_tags.domain.values[0])`;
  - `tenant_tag_key = "${var.platform.lf_tags.tenant_key_prefix}${var.name}"`;
  - `dedicated_bucket_names` (when `local.dedicated`), plus `bucket_arn[zone]` and `catalog_id[zone]`, which resolve to the dedicated bucket or to `var.platform.zones[zone]`;
  - `kms_key_arn`: the tenant key when designated, otherwise `var.platform.kms_key_arn`;
  - `results`: `{ bucket_arn, bucket_name, prefix }`, which is the dedicated bucket and `""` when designated, otherwise the shared bucket and `"${var.name}/"`;
  - `database_labels`: the tenant key `member`, the sensitivity ceiling, `yes` for the tenant's regulations and `no` for the rest, and the domain;
  - `label_scope` (`L(T)`): sensitivity levels up to the ceiling; per regulation, `["no", "yes"]` if declared and `["no"]` otherwise; and all domain values;
  - persona maps: `steward_grants` keyed `"<zone>/<arn>"`, `writer_grants` and `reader_grants` keyed `"<key>/<zone>"`, and `row_filters`;
  - effective cutoff and alarm threshold, from the tenant value or `var.platform.effective`;
  - `tags`: `merge(var.platform.common_tags, var.tags, { Tenant = var.name, DataOwner = owner })`, using the platform's tag key names.

### Encryption and storage (FR-019, FR-028)

- [x] T034 Create `modules/tenant/kms.tf`: `aws_kms_key.this` and `aws_kms_alias.this`, with `count = local.designated ? 1 : 0`.
  - The policy statements match the platform's T020 statements: maintenance (scoped to `local.dedicated_bucket_names` ARNs), `var.platform.data_access_role_arn`, and Athena and S3 via `kms:ViaService`.
  - Rotation is on, and the deletion window comes from the platform effective settings.
  - Tags add `ComplianceFrameworks`.
- [x] T035 Create `modules/tenant/storage.tf`:
  - `aws_s3tables_table_bucket.dedicated` (`for_each = local.dedicated ? local.zones : []`):
    - encrypted with `local.kms_key_arn`, which is the tenant key when designated and the platform key otherwise;
    - maintenance, default storage class (`var.platform.effective.table_storage_class`), and `force_destroy` from the platform effective settings;
    - tags with `DataClassification` set to the tenant ceiling, plus `ComplianceFrameworks` when designated;
  - `aws_s3tables_table_bucket_policy.dedicated`, with the same three deny statements as T026, keyed to `local.kms_key_arn`;
  - `aws_s3tables_namespace.this` (`for_each = local.zones`), with `namespace = var.name` and `table_bucket_arn = local.bucket_arn[each.key]`.

### LF-tags and grants (FR-003, FR-009 to FR-014)

- [x] T036 Create `modules/tenant/lf-tags.tf`:
  - `aws_lakeformation_lf_tag.tenant`: key `local.tenant_tag_key`, values `["member"]`;
  - `aws_lakeformation_resource_lf_tags.database` (`for_each = local.zones`) on `database { catalog_id = local.catalog_id[each.key], name = var.name }`, setting every key in `local.database_labels`.
  Comment that:
  - labels are the tenant's most restrictive values, so unlabeled tables fail closed;
  - every key must resolve on every table and column for expressions to match;
  - the submodule never labels tables or columns (FR-011a).
- [x] T037 Create `modules/tenant/permissions.tf` with steward grants. For each `local.steward_grants` entry, create two `aws_lakeformation_permissions` `lf_tag_policy` grants on `local.catalog_id[zone]`, with expression `tenant-T = member`:
  - `DATABASE` with `["CREATE_TABLE", "ALTER", "DROP", "DESCRIBE"]`;
  - `TABLE` with `["SELECT", "INSERT", "DELETE", "ALTER", "DROP", "DESCRIBE"]`.
  On both, `permissions_with_grant_option` equals `permissions`.
- [x] T038 Add LF-tag permissions to `permissions.tf`:
  - each steward gets an `lf_tag` permission per key in `local.label_scope`, with `["DESCRIBE", "ASSOCIATE", "GRANT_WITH_LF_TAG_EXPRESSION"]`, plus `lf_tag { key = local.tenant_tag_key, values = ["member"] }` with `["DESCRIBE", "GRANT_WITH_LF_TAG_EXPRESSION"]`;
  - each writer gets an `lf_tag` permission per key in `local.label_scope`, with `["DESCRIBE", "ASSOCIATE"]`.
  Comment that nobody but platform admins can `ASSOCIATE` the tenant key.
- [x] T039 Add writer grants to `permissions.tf`. Per `local.writer_grants` entry, create two grants with expression `tenant-T = member`:
  - `DATABASE` with `["CREATE_TABLE", "DESCRIBE"]`;
  - `TABLE` with `concat(["SELECT", "INSERT", "DELETE", "ALTER", "DESCRIBE"], var.platform.effective.writer_drop_enabled ? ["DROP"] : [])`.
- [x] T040 Add reader grants to `permissions.tf`. Per `local.reader_grants` entry, create:
  - a `DATABASE` grant with `["DESCRIBE"]` on `tenant-T = member`;
  - a `TABLE` grant with `["SELECT", "DESCRIBE"]` and this expression:
    - `tenant-T = member`;
    - `sensitivity ∈ reader.sensitivity_values`;
    - for every platform regulation `r`, `reg-r ∈ ["no", "yes"]` when `r` is in the reader's regulations, else `["no"]`;
    - `domain ∈ reader.domains`, only when `domains` is non-null.
  Comment that LF-TBAC evaluates column tags (FR-011).
- [x] T041 Add row filters to `permissions.tf` (FR-012). Per `local.row_filters` entry, create an `aws_lakeformation_data_cells_filter` on `local.catalog_id[zone]` / `var.name` / `table`, and a `data_cells_filter` grant with `["SELECT"]` to the reader's ARN. Comment that this is the only named-resource grant.

  Every grant in this file `depends_on`:
  - `aws_lakeformation_resource_lf_tags.database`;
  - `aws_lakeformation_lf_tag.tenant`;
  - `terraform_data.admin_guard`.

### Query, policies, alarms, guards (FR-015 to FR-018, FR-023, FR-030)

- [x] T042 Create `modules/tenant/query.tf`:
  - When designated, create `aws_s3_bucket.results` named `${name_prefix}-${name}-athena-results-${account_id}`, with the same controls as T027. It uses the tenant key, and its bucket policy also denies uploads whose `s3:x-amz-server-side-encryption-aws-kms-key-id` is not the tenant key.
  - `aws_athena_workgroup.this`, named `${name_prefix}-${name}`:
    - `enforce_workgroup_configuration = true`;
    - the effective cutoff;
    - `publish_cloudwatch_metrics_enabled` from the platform effective settings;
    - Athena engine version 3;
    - `output_location = "s3://${local.results.bucket_name}/${local.results.prefix}"`;
    - `SSE_KMS` with `local.kms_key_arn`;
    - `expected_bucket_owner`;
    - `force_destroy` from the platform effective settings;
    - tags from `local.tags`.
- [x] T043 [P] Create `modules/tenant/iam-documents.tf`. Create one `data "aws_iam_policy_document"` per steward, writer, and reader entry, keyed `"<persona>/<key>"`, with the statements in plan.md "Persona IAM policy documents". Scope them to `aws_athena_workgroup.this.arn`, the `local.results` location, and `local.kms_key_arn`.
- [x] T044 [P] Create `modules/tenant/alarms.tf`. When `var.platform.effective.alarms_enabled`, create `query_failures` and `scanned_bytes` alarms on the workgroup, routed to `var.platform.alarm_sns_topic_arn`, with the effective threshold.
- [x] T045 Create `modules/tenant/main.tf`:
  - `terraform_data.admin_guard`, with `count = var.verify_deployer_admin ? 1 : 0` and a precondition that the session issuer ARN is in the data-lake settings `admins`. The error message names the pipeline role and the platform's `admin_principal_arns`.
  - `check "regulated_data_in_baseline"` (FR-030): warns when `local.designated && var.platform.baseline_environment`, naming the tenant.
  - `module "cost_estimation"` (`source = "../cost-estimation"`, providers `aws.pricing`), with `enabled = var.platform.cost_estimation_enabled` and this tenant's counts: designated buckets, keys, results bucket, workgroup, and alarms. Tenant keys count as compliance-only (FR-024).
- [x] T046 Create `modules/tenant/outputs.tf` with the outputs listed in plan.md (Tenant Submodule):
  - `namespaces`, `catalog_ids`, and `table_bucket_arns` (maps by zone);
  - `tenant_tag_key`, `workgroup_name`, `query_results_location`, and `kms_key_arn`;
  - `persona_policy_json`;
  - `monthly_cost_estimate` and `compliance_monthly_cost`.
- [x] T047 [P] Create `modules/tenant/README.md` covering:
  - both entry points, with the HCL from plan.md;
  - publishing and consuming the `platform_contract` through remote state or an SSM parameter with `jsonencode`/`jsondecode`;
  - that tenant provisioning pipelines are platform-operated. The pipeline role is a data-lake admin, uses the `scope = "tenant"` deployer policy, and is never handed to tenant-operated identities (FR-001a);
  - name uniqueness across entry points;
  - moving a tenant between entry points with `terraform state mv` or `moved`/`removed` and `import` blocks;
  - the labeling and roll-up contract;
  - a terraform-docs injection block.
- [x] T048 Wire the root in `main.tf`: `module "tenant"` with `for_each = var.enabled ? var.tenants : {}`, providers `aws` and `aws.pricing`, `platform = local.platform_contract`, `name = each.key`, `tenant = each.value`, and `verify_deployer_admin = false`. It `depends_on` the catalog-integration module, the root admin guards, and `lf-tags.tf`, because the contract carries only values.
- [x] T049 **Checkpoint and sandbox verification**: run `make check` and `make lint`. Then run `make plan-examples`, and `make apply-examples` for `examples/complete`, and confirm:
  - `lf_tag_policy` grants with an `s3tablescatalog/<bucket>` `catalog_id` apply and authorize a reader;
  - `GRANT_WITH_LF_TAG_EXPRESSION` is accepted;
  - the Terraform argument and minimum provider version that set a table bucket's default storage class exist. Update `versions.tf` if the floor rises. A table created in a Production bucket reports the `INTELLIGENT_TIERING` storage class;
  - a table whose rolled-up sensitivity is above a reader's levels, with every column labeled explicitly and some columns within those levels, returns those columns to the reader. If this fails, apply the plan.md fallback (informational `table-*` keys) before Phase 5.

  Record the outcome in plan.md Complexity Tracking. Stop and revise the plan if either of the first two fails.

---

## Phase 5: Operations Tier (Cost, Deployer, Root Outputs, Platform Alarms)

**Purpose**: The platform cost estimate, deployer policies for both scopes, and root outputs.

- [x] T050 [P] Extend `modules/cost-estimation/variables.tf`, `modules/cost-estimation/main.tf`, `modules/cost-estimation/locals.tf`, and `modules/cost-estimation/outputs.tf` (FR-024). The module is used by both the root and the tenant submodule.
  - New inputs: `table_bucket_count`, `s3_bucket_count`, `athena_workgroup_count`, `alarm_count`, `kms_key_count`, and `compliance_kms_key_count`.
  - Price List lookups for the S3 Tables storage and maintenance rates, the Athena per-TB rate, and S3 Standard storage.
  - Outputs:
    - `monthly_cost_estimate`: the idle fixed total;
    - `cost_breakdown`: per service, including $0 lines for no-charge resource types;
    - `compliance_monthly_cost`;
    - `usage_rates`: S3 Tables storage per GB-month (Standard, and the Intelligent-Tiering Frequent, Infrequent, and Archive Instant Access tiers plus its monitoring charge), maintenance per object, Athena per TB scanned, and S3 Standard per GB-month.
  - The module creates no resources; keep it data-source only.
- [x] T051 Update `cost-estimation.tf` to pass platform-only counts: zone buckets, the shared results bucket, the platform key, and the SNS topic. Add `local.total_monthly_cost_estimate`, which sums the platform estimate and every platform-applied `module.tenant[*].monthly_cost_estimate`. Standalone tenants report through their own module (FR-024).
- [x] T052 [P] Extend `modules/deployer/main.tf` and `modules/deployer/variables.tf` (FR-025) with least-privilege statements that list every action explicitly (no wildcard actions).
  - Add `scope` (`"platform"` default, or `"tenant"`, validated) and `cost_estimation_enabled` (default `true`, which gates the existing `PricingAPIAccess` statement).
  - Keep the existing KMS, CloudWatch, SNS, and caller-identity statements.
  - Use `${name_prefix}`-scoped resources wherever supported. Every `"*"` resource carries an inline comment naming why the service has no resource-level permission.
  - Update `modules/deployer/README.md` to:
    - describe both scopes;
    - state that the unit creates no roles and attaches to `iam_role_arn`;
    - state that Lake Formation data-lake admin membership must be granted separately (FR-013).
  - **s3tables**: `CreateTableBucket`, `DeleteTableBucket`, `GetTableBucket`, `ListTableBuckets`, `PutTableBucketEncryption`, `GetTableBucketEncryption`, `PutTableBucketMaintenanceConfiguration`, `GetTableBucketMaintenanceConfiguration`, `PutTableBucketStorageClass`, `GetTableBucketStorageClass`, `PutTableBucketPolicy`, `GetTableBucketPolicy`, `DeleteTableBucketPolicy`, `CreateNamespace`, `DeleteNamespace`, `GetNamespace`, `ListNamespaces`, `TagResource`, `UntagResource`, `ListTagsForResource`.
  - **glue**, platform scope only: `CreateCatalog`, `GetCatalog`, `UpdateCatalog`, `DeleteCatalog`.
  - **lakeformation**:
    - platform scope only: `RegisterResource`, `DeregisterResource`, `DescribeResource`, `PutDataLakeSettings`;
    - both scopes: `GetDataLakeSettings`, `GrantPermissions`, `RevokePermissions`, `ListPermissions`, `CreateLFTag`, `DeleteLFTag`, `GetLFTag`, `UpdateLFTag`, `AddLFTagsToResource`, `RemoveLFTagsFromResource`, `GetResourceLFTags`, `CreateDataCellsFilter`, `DeleteDataCellsFilter`, `GetDataCellsFilter`.
  - **athena**: `CreateWorkGroup`, `UpdateWorkGroup`, `DeleteWorkGroup`, `GetWorkGroup`, `TagResource`, `UntagResource`, `ListTagsForResource`.
  - **s3**: bucket configuration actions on `${name_prefix}-athena-results-*` (platform scope) or `${name_prefix}-*-athena-results-*` (tenant scope).
  - **kms**: key and alias management.
  - **iam**, platform scope only: `CreateRole`, `DeleteRole`, `GetRole`, `PutRolePolicy`, `DeleteRolePolicy`, `GetRolePolicy`, `TagRole`, and `PassRole` on `${name_prefix}-lf-s3tables-access`, with `iam:PassedToService = lakeformation.amazonaws.com`.
  - **cloudwatch**: `PutMetricAlarm`, `DeleteAlarms`, `DescribeAlarms`.
- [x] T053 Rewrite `outputs.tf` (FR-026):
  - `zone_table_bucket_arns` (map by zone) and `catalog_name`;
  - `platform_contract` (T028);
  - `lf_tag_vocabulary`: the platform keys, allowed values, sensitivity order, and tenant key prefix, for pipeline labeling;
  - `data_access_role_arn`, `query_results_bucket_arn`, and `kms_key_arn`;
  - `tenants`: a map by tenant of the `module.tenant` outputs;
  - `required_kms_key_policy_json`: the T020 `required_kms_statements` document.
  Keep the existing alarm and cost outputs, with `monthly_cost_estimate` as the T051 total. Every output returns empty or null values when `enabled = false`.
- [x] T054 **Checkpoint**: run `make check`, then `make lint`, then `make docs`. Confirm terraform-docs ran and updated `README.md` and `modules/*/README.md`.

---

## Phase 6: Examples & Contract Tests

**Purpose**: Plan-only coverage of every module promise in spec.md Success Criteria (Code
Validation), for both tenant entry points.

- [x] T055 [P] Update `examples/defaults/main.tf`, `variables.tf`, `terraform.auto.tfvars`, and `outputs.tf`: required inputs only, with catalog integration enabled and `environment_type = "Development"`.
  - `name_prefix`.
  - `admin_principal_arns`: a variable defaulting to `[]`. When it's empty, `main.tf` passes `[data.aws_iam_session_context.current.issuer_arn]`, which means the role running the plan. The example declares `aws_caller_identity` and `aws_iam_session_context`, so the FR-013 precondition passes for whoever runs `make test`.
  - One minimal tenant, `analytics`, with `owner` and `sensitivity = "internal"`, no regulations, the default domain, and no personas.
- [x] T056 [P] Update `examples/complete/main.tf`, `variables.tf`, `terraform.auto.tfvars`, and `outputs.tf`. Set every input:
  - `domain_values = ["general", "sales", "finance", "health"]`;
  - three tenants:
    - `sales`: domain `sales`, all zones, sensitivity `confidential`, and regulations `["gdpr"]`. Personas: one steward, writers in raw and curated, a consumption-only semantic-layer reader with `["public", "internal"]` and no regulations, and a GDPR-assigned analyst. It also has a row filter;
    - `finance`: domain `finance` and sensitivity `confidential`. It has readers with two sensitivity levels, one of them limited to `domains = ["finance"]`, and a tenant scan cutoff override;
    - `claims`: domain `health`, sensitivity `restricted`, regulations `["hipaa", "gdpr"]`, and `compliance_frameworks` from a variable defaulting to `["HIPAA"]`;
  - the same semantic-layer principal listed as a consumption reader in `sales` and `finance`;
  - `environment_type = "Production"`, with alarms enabled;
  - `tags` with `CostCenter` and `DataOwner`;
  - the same caller-default admin list pattern as T055.
  The tenant definitions live in `tenants.auto.tfvars`, so `examples/standalone-tenant` can reuse the `sales` definition.
- [x] T057 [P] Create `examples/secondary/` (`main.tf`, `variables.tf`, `outputs.tf`, `versions.tf`, `terraform.auto.tfvars`, `README.md`) with `catalog_integration_config = { enabled = false, data_access_role_arn = <variable> }` and one tenant. The README states that the deploying role must be in the owning deployment's admin list.
- [x] T058 [P] Create `examples/standalone-tenant/` (`main.tf`, `variables.tf`, `outputs.tf`, `versions.tf`, `terraform.auto.tfvars`, `README.md`). It calls `../../modules/tenant` directly:
  - `platform` comes from a `platform_contract` variable, set in `platform_contract.auto.tfvars.json`, a synthetic contract fixture that matches `examples/complete`'s shape;
  - `name = "sales"`, with the `sales` definition copied from `examples/complete/tenants.auto.tfvars`;
  - `verify_deployer_admin = true`.
  The README shows the production pattern, which reads the contract from remote state or SSM.
- [x] T059 [P] Create `examples/deployer/` (`main.tf`, `variables.tf`, `outputs.tf`, `versions.tf`, `terraform.auto.tfvars`, `README.md`). It calls `../../modules/deployer` twice, once with `scope = "platform"` and once with `scope = "tenant"`, against a placeholder role ARN variable, and outputs both rendered policy JSON documents.
- [x] T060 Update `test/defaults_test.go` to assert:
  - present:
    - `module.main.aws_s3tables_table_bucket.zone["raw"]`, `["curated"]`, and `["consumption"]`;
    - `aws_lakeformation_lf_tag.sensitivity`, `.domain`, and `.regulation["gdpr"]`, `["hipaa"]`, and `["pci_dss"]`;
    - `module.main.module.tenant["analytics"].aws_lakeformation_lf_tag.tenant`;
    - `...aws_s3tables_namespace.this["raw"]`, `["curated"]`, and `["consumption"]`;
    - `...aws_lakeformation_resource_lf_tags.database["raw"]`;
    - `...aws_athena_workgroup.this`;
    - `module.main.module.catalog_integration[0].aws_glue_catalog.s3tables`;
  - absent: any root-level `aws_lakeformation_lf_tag.tenant`, any `aws_kms_key.this` or `aws_s3tables_table_bucket.dedicated` in a tenant module, and any `aws_cloudwatch_metric_alarm`;
  - the planned `monthly_cost_estimate` output is greater than 0 and at most 5 (Baseline budget);
  - the zone buckets set the `STANDARD` default storage class, and carry `DataClassification` equal to the most sensitive `sensitivity_values` entry.
  Add `TestCostEstimationDisabled`, which plans `examples/defaults` with `cost_estimation_config = { enabled = false }`. It asserts:
  - `monthly_cost_estimate = 0`;
  - no tenant `module.cost_estimation` data sources;
  - the same resource set as the default plan.
- [x] T061 Update `test/complete_test.go`. `TestTerraformCompleteExample` asserts:
  - every `aws_lakeformation_permissions` resource except data cells filter grants uses `lf_tag_policy`;
  - every reader table grant expression includes the tenant's own key, the sensitivity, and every `reg-*` key:
    - the semantic-layer reader's regulation keys allow only `no`;
    - the GDPR analyst's `reg-gdpr` allows `no` and `yes`;
    - only the domain-limited `finance` reader's expression includes `domain`;
  - `claims` database attachments set `reg-hipaa = yes`, `reg-gdpr = yes`, `sensitivity = restricted`, and `domain = health`;
  - `sales` writer and steward `lf_tag` permissions cover sensitivity up to `confidential` and `reg-gdpr` `["no", "yes"]`, but `reg-hipaa` `["no"]` only;
  - steward grants have grant options, and steward permissions on the tenant key lack `ASSOCIATE`;
  - the semantic-layer reader has grants only in consumption catalogs;
  - writer table grants do not contain `"DROP"` (Production);
  - non-designated workgroups' `output_location` ends in `/<tenant>/`, and `claims` writes to its dedicated results bucket;
  - shared and dedicated table buckets set the `INTELLIGENT_TIERING` default storage class (Production);
  - both alarms exist per tenant.

  It also:
  - runs `terraform.OutputJson` on the `tenants` output and asserts each `persona_policy_json` entry's S3 and Athena resources cover only its tenant's results location and workgroup;
  - keeps `TestEnabledFalse`, asserting no resources to add.

  Tag assertions (FR-022) in `TestTerraformCompleteExample`:
  - every planned resource that supports tags carries the `CostCenter` and `DataOwner` common tags;
  - every resource under `module.tenant["<t>"]` that supports tags carries `Tenant = <t>` and `DataOwner = <tenant owner>`;
  - `module.tenant["claims"]` dedicated buckets carry `DataClassification = restricted` and `ComplianceFrameworks = HIPAA`;
  - zone buckets and the shared results bucket carry `DataClassification` equal to `shared_storage_classification`.

  Add `TestCompleteExampleSecurityControls`, which plans `examples/complete` and asserts:
  - every `aws_s3tables_table_bucket` has `encryption_configuration.sse_algorithm = "aws:kms"` with a non-empty key ARN;
  - every `aws_s3_bucket` has a matching `aws_s3_bucket_public_access_block` with all four settings `true`, and an SSE-KMS configuration;
  - every bucket policy and table bucket policy contains a `Deny` statement conditioned on `aws:SecureTransport = false`;
  - every `aws_lakeformation_permissions.principal` is in the set of ARNs from the example's `admin_principal_arns` and tenant persona inputs;
  - no policy document (bucket, table bucket, key, persona output) has a `Principal` of `"*"` or `{"AWS": "*"}` in an `Allow` statement.
  T072 (Checkov) stays as the static scan.
- [x] T062 Add `TestComplianceDesignationGatesTenantKeys` to `test/complete_test.go`. It is table-driven over `claims` designations `[]`, `["SOC2"]`, `["HIPAA"]`, and `["PCI_DSS"]`, and sets `regulations = ["gdpr"]` for every case except `HIPAA`, so validation passes:
  - with `[]`, there is no `module.tenant["claims"].aws_kms_key.this`, no dedicated bucket, and no dedicated results bucket, and the `claims` namespaces sit in the shared zone buckets;
  - with each framework, the plan contains exactly the tenant key, `aws_s3tables_table_bucket.dedicated[<zone>]` for each zone, and `aws_s3_bucket.results`;
  - in every case, no root-level resource and no `sales` or `finance` resource changes;
  - the planned `tenants["claims"].compliance_monthly_cost` output is 0 with `[]` and greater than 0 otherwise.
  Add a case with `claims` set to `compliance_frameworks = []`, `regulations = ["gdpr"]`, and `dedicated_storage = true`. It plans dedicated buckets encrypted with the platform key, and no tenant key or dedicated results bucket.
  Also assert in `TestTerraformCompleteExample` that the planned `monthly_cost_estimate` is at most `10 + 3 × 0.50 + 2`.
- [x] T063 [P] Create `test/tenancy_test.go`:
  - `TestAddingTenantTouchesOnlyThatTenant`: plans `examples/complete` with and without an extra tenant `marketing`. Every differing address must start with `module.main.module.tenant["marketing"]`, with no root-level changes.
  - `TestTenantTypeInSync`: parses `variables.tf` and `modules/tenant/variables.tf` and asserts the `tenants` element type and the `tenant` type expressions are identical.
  - `TestZoneOverrideWithoutConsumption`: plans `examples/defaults` with `zones_config = { zones = ["raw", "curated"] }` and asserts no `consumption` bucket, namespace, or grant is present.
- [x] T064 [P] Create `test/standalone_test.go`:
  - `TestStandaloneTenantMatchesPlatformApplied`: plans `examples/standalone-tenant` and `examples/complete`. Strip the prefixes `module.tenant` and `module.main.module.tenant["sales"]`; the remaining resource address sets for `sales` must be equal.
  - `TestStandaloneTenantReportsOwnCost`: the planned `monthly_cost_estimate` output of `examples/standalone-tenant` is greater than 0.
  - `TestStandaloneTenantRejectsInvalidAgainstContract`: sets a domain absent from the fixture contract and asserts the validation error.
- [x] T065 [P] Create `test/deployer_test.go`, which plans `examples/deployer` and parses both policy outputs. `TestDeployerPolicies` asserts:
  - no statement action contains `*`;
  - the tenant scope contains none of `glue:CreateCatalog`, `lakeformation:PutDataLakeSettings`, `lakeformation:RegisterResource`, or any `iam:` action;
  - the platform scope contains a create action for every resource type in the `examples/complete` plan (`aws_s3tables_*`, `aws_lakeformation_*`, `aws_athena_workgroup`, `aws_s3_bucket*`, `aws_kms_*`, `aws_cloudwatch_metric_alarm`, `aws_sns_topic`, `aws_iam_role*`, `aws_glue_catalog`), using a type-to-action map kept in the test;
  - `pricing:GetProducts` is present only when `cost_estimation_enabled = true`;
  - both scopes contain `s3tables:PutTableBucketStorageClass` and `s3tables:GetTableBucketStorageClass` (FR-005 default storage class).
- [x] T066 [P] Create `test/secondary_test.go` with `TestSecondaryExampleSkipsAccountIntegration`. It asserts the plan has no `module.catalog_integration` resources, but contains the zone buckets, platform LF-tags, a tenant module, and `terraform_data.secondary_admin_guard[0]`. Document in `test/README.md` that the roles running `make test-secondary` and `make test-standalone` must already be Lake Formation data-lake admins in the sandbox account.
- [x] T067 [P] Create `test/validation_test.go` with table-driven, plan-expected-failure cases, each asserting the error message:
  - tenant `Sales_1` (invalid name);
  - a tenant zone that is not a platform zone;
  - a writer zone outside its tenant's zones;
  - an empty principal ARN;
  - a reader sensitivity level above the tenant ceiling;
  - a tenant sensitivity not in `sensitivity_values`;
  - a tenant regulation not in `regulation_values`;
  - a tenant with `hipaa` but no `HIPAA` designation;
  - a reader regulation the tenant does not declare;
  - a tenant domain not in `domain_values`;
  - more than 20 `regulation_values`;
  - a row filter naming an undeclared reader;
  - `bytes_scanned_cutoff_per_query = 1000` at the platform or tenant level;
  - an unknown compliance framework;
  - integration disabled without a role ARN;
  - `environment_type = "Production"` without the `DataOwner` tag;
  - a zone named `raw_v2` (underscore);
  - a designated tenant whose dedicated bucket name exceeds 63 characters or collides with a shared bucket;
  - a non-dedicated tenant whose `sensitivity` ranks above `shared_storage_classification`;
  - `storage_config.table_storage_class = "GLACIER"`;
  - `environment_type = "None"` with `required_tags_config.enforced = null`.
  Add `TestRegulatedDataInBaselineWarns`, which plans `examples/complete` with `environment_type = "Development"` and asserts the plan succeeds and its output contains the `regulated_data_in_baseline` warning naming `claims` (FR-030).
- [x] T068 Add `test-tenancy`, `test-standalone`, `test-deployer`, `test-secondary`, and `test-validation` targets to `Makefile`, following the existing `test-complete` pattern. Add them to `make help` output via `##` comments.
- [x] T069 **Checkpoint**: run `make test`. All plan-only tests must pass. Fix failures and rerun the single failing target.

---

## Phase N: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, security scan, CI alignment, cost check, full gates, and sandbox
verification of the spec's operational success criteria.

- [x] T070 Update `README.md`:
  - replace the "(in progress)" feature markers;
  - add a "Tenancy model" section covering:
    - tenant = namespace per zone;
    - the two entry points (platform `tenants` map, or standalone `modules/tenant` called by a pipeline), linking `modules/tenant/README.md`;
    - the `platform_contract`;
    - the shared versus dedicated buckets, and when to set `dedicated_storage`;
    - that tenant provisioning pipelines are platform-operated (FR-001a);
    - a "Service quotas" table: tables per table bucket (10,000), table buckets per Region (100), LF-tag keys per account, and Athena workgroups per account, each with its effect on tenant count;
    - default storage classes per environment type, and how to override them;
  - add a "Data zones" section describing `raw`, `curated`, and `consumption` (gold), the personas that normally receive grants on each, zone isolation, and that the external semantic layer reads tenants' consumption namespaces;
  - add an "Access control with LF-tags" section covering:
    - the vocabulary: per-tenant keys, sensitivity, per-regulation yes/no keys, and domain;
    - fail-closed database defaults;
    - grant expressions per persona, and column-level control;
    - the domain reader filter;
    - row filters as the only named-resource exception;
    - the steward runbook;
  - add a "Labeling and roll-up contract" section for pipeline authors, with AWS CLI and boto3 reference snippets;
  - add an "Account-level integration ownership" section;
  - add an "Audit trail prerequisites" section;
  - add a "Table maintenance" section, including GDPR rebuild and purge of consumption data products;
  - add "Compliance designation", "Required tags", "Bring your own KMS key", and "Query isolation" sections.
- [x] T071 [P] Update `AGENTS.md` Components to match the implemented files, including `modules/tenant` and `modules/catalog-integration`.
- [x] T072 [P] Add a `security-scan` target to `Makefile` that runs `checkov --framework terraform -d .` with the same flags as `.github/workflows/terraform-ci.yml`, then run `make security-scan`. Fix or suppress every HIGH or CRITICAL finding with an inline justification comment.
- [x] T073 Update `.github/workflows/terraform-ci.yml` so CI runs the same gates as local validation:
  - install TFLint and terraform-docs;
  - replace the direct `terraform fmt`, `terraform validate`, `checkov`, and `go test` steps with `make check`, `make lint`, `make docs` (failing if `git diff --exit-code` shows changes), `make security-scan`, and `make test`;
  - remove the commented-out TFLint block.
  This depends on T072.
- [x] T074 Run `make plan-examples` and review the cost outputs:
  - defaults must be at most $5 idle;
  - complete (Production, three tenants, one HIPAA) must be at most $10 + 3 × $0.50 for the platform and tenants, plus at most $2 compliance cost for `claims`;
  - standalone-tenant reports its own estimate.
- [x] T075 Run `make pre-commit`, then `make validate`. Both must pass with TFLint and terraform-docs installed, and no files may be left modified.
- [x] T076 Sandbox end-to-end (Development first, then Production configuration): run `make apply-examples` and record the plan and apply durations against the 3-minute and 15-minute targets. Then verify the spec success criteria:
  - **Fail-closed labeling and roll-up**:
    - a `sales` writer creates a table through Athena without stating encryption; the table inherits the database LF-tags and the bucket key;
    - before labeling, the table is invisible to the semantic-layer reader;
    - the writer labels it per the roll-up contract, with one `confidential` column and one GDPR column, so the table rolls up to `confidential` and `reg-gdpr = yes`;
    - within 5 minutes, the semantic-layer reader sees only the `public` and `internal` non-GDPR columns, and the GDPR analyst also sees the GDPR column;
    - the domain-limited `finance` reader is denied on a `finance` table relabeled to another domain;
  - **Tenant and zone isolation**:
    - a `finance` reader is denied on `sales` tables and on the `sales/` results prefix;
    - the semantic-layer reader can read `sales` and `finance` consumption tables, and is denied on their curated and raw tables;
  - **Delegated labeling and grants**:
    - a `sales` steward raises a column to `confidential`, and a reader limited to `["public", "internal"]` no longer sees it;
    - the steward and writer cannot add or change any tenant key, cannot label `restricted` or `reg-hipaa = yes`, and cannot label or grant on `finance` tables;
    - a steward grant to a new principal works and appears in CloudTrail;
    - `AWS::S3Tables::Table` data events for the reader's query, and the writer's `AddLFTagsToResource` management event, appear in the account trail;
    - the row filter limits rows for its reader;
  - **Standalone tenant**:
    - publish `platform_contract` to SSM;
    - apply a standalone tenant `marketing` from a separate state using that contract;
    - confirm it works end to end;
    - rerun `make plan-examples` for the platform and confirm it shows no changes;
  - **Deployer least privilege**:
    - attach the platform-scope deployer policy (from `examples/deployer`) to a fresh role that is also a data-lake admin, and apply and destroy `examples/complete` with only that role;
    - repeat with the tenant-scope policy for the standalone tenant;
    - record any `AccessDenied` as a T052 fix, not a broadened wildcard;
  - **Designated tenant**: `claims` tables are encrypted with the `claims` key, and its results land in its dedicated bucket under that key;
  - **Operations**:
    - in Production, a writer's `DROP TABLE` is denied;
    - a query over the cutoff is cancelled;
    - unreferenced-file removal is reported enabled on each bucket, and the writer-created table reports compaction and snapshot management enabled;
    - the table restores to an earlier snapshot within the RPO window;
    - a forced query failure triggers the tenant's `query_failures` alarm and delivers to the SNS topic.
  Then destroy the standalone tenant, then run `make destroy-examples`.
- [x] T077 Record T076 results and any deviations in `specs/001-governed-lakehouse/plan.md` under Complexity Tracking.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1: Setup** has no dependencies and blocks every later phase.
- **Phase 2: Foundation** depends on Phase 1 and **blocks Phase 3**.
- **Phase 3: Platform Data & Query** depends on Phase 2. The `platform_contract` (T028) **blocks
  Phase 4**.
- **Phase 4: Tenant Submodule** depends on Phase 3. Its sandbox verification (T049) gates
  Phase 5.
- **Phase 5: Operations** depends on Phase 4. Cost totals and root outputs need the tenant
  outputs.
- **Phase 6: Examples & Tests** depends on Phase 5.
- **Phase N: Polish** depends on all phases.

### Resource Dependency Order (at apply time)

1. Lake Formation data-access role. It has no key dependencies.
2. The platform KMS key. Its policy references the role ARN.
3. Lake Formation resource registration (`bucket/*`, with federation).
4. `s3tablescatalog` Glue catalog.
5. Data-lake settings (admins, empty defaults). For secondary deployments, the admin guard runs
   instead.
6. Platform LF-tags (`<prefix>-sensitivity`, `<prefix>-reg-*`, `<prefix>-domain`).
7. Shared zone table buckets and policies; shared results bucket.
8. Per tenant (platform-applied in parallel, or standalone later):
   1. tenant key, dedicated buckets, and dedicated results bucket (designated only);
   2. tenant LF-tag;
   3. namespaces;
   4. database tag attachments;
   5. grants, tag permissions, and data cells filters;
   6. workgroup;
   7. alarms.

### Task-Level Dependencies

- T003 to T008 edit the same `variables.tf` and must run sequentially. T031 copies the tenant
  type from T006. T009 and T010 depend on T003 to T008; T010 also depends on T002.
- T015 → T016 → T017: role → registration and catalog → settings.
- T020 depends on T018. T021 to T023 edit `main.tf` sequentially. T024 depends on T017 and T022.
- T028 depends on T024, T026, and T027.
- T031 → T033 → T034 → T035 → T036 → T037 to T041 (sequential `permissions.tf`) → T042 → T045 →
  T046 → T048.
- T053 depends on T046 and T048. T060 to T067 depend on T055 to T059. T059 and T065 depend on
  T052. T073 depends on T072.

### Parallel Opportunities

| Phase | Parallel group | Why safe |
|-------|----------------|----------|
| 1 | T002, T011 | Different files (`dependencies.tf`, examples) with no shared symbols |
| 2 | T013, T014, T018, T019 | Separate submodule files; scaffolding only |
| 4 | T030, T032; then T043, T044, T047 | Separate tenant submodule files |
| 5 | T050, T052 | `modules/cost-estimation/` and `modules/deployer/` are independent |
| 6 | T055 to T059, then T063 to T067 | Separate example directories and new test files |
| N | T071, T072 | Docs and Makefile target are independent |

### Validation Checkpoints

| Checkpoint | After | Make targets |
|------------|-------|--------------|
| T012 | Setup | `make check` |
| T025 | Foundation | `make check`, `make lint` |
| T029 | Platform Data & Query | `make check`, `make lint` |
| T049 | Tenant Submodule | `make check`, `make lint`, plus the sandbox verification |
| T054 | Operations | `make check`, `make lint`, `make docs` |
| T069 | Examples & Tests | `make test` (single targets for reruns) |
| T075 | Polish | `make pre-commit`, `make validate` |

### Environment Order

Development (the defaults example) → Production configuration (the complete example, with a
designated tenant, plus a standalone tenant) in the sandbox account (T076). Consumers then
promote through their own Ephemeral/Development/Testing → UAT → Production → MissionCritical
platforms by changing only `environment_type` and the required tags. Tenant definitions,
including their designations, carry over unchanged in either entry point.

## Notes

- [P] tasks touch different files and have no dependencies on other [P] tasks in the same phase.
- Commit after each checkpoint passes, using a Conventional Commit message.
- Plan-only tests need AWS credentials for provider data sources. `make test` runs `check-aws`
  first.
- Do not create tables, table- or column-level LF-tag assignments (pipelines own them per
  FR-011a), identities, VPC endpoints, a CloudTrail trail, a tenant registry, or the SSM parameter
  that publishes the contract; all are out of scope.
