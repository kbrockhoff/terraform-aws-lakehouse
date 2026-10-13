# Architecture Plan: Governed Lakehouse Foundation

**Branch**: `001-governed-lakehouse` | **Date**: 2026-10-13 | **Spec**: [spec.md](./spec.md)
**Input**: Infrastructure specification from `/specs/001-governed-lakehouse/spec.md`

**Note**: This template is filled in by the `/iac.plan` command. For deep research, module specs, and quickstart guide, run `/iac.enrichplan` after planning.

## Summary

A reusable Terraform module provisions a governed, Apache Iceberg–based lakehouse in one AWS
account and Region:
- **Storage**: one Amazon S3 Tables table bucket per data zone (`raw`, `curated`), each holding
  namespaces the consumer declares.
- **Catalog**: the Region's federated `s3tablescatalog` in the AWS Glue Data Catalog exposes the
  tables to analytics services.
- **Access**: AWS Lake Formation authorizes all access, with credential vending and database,
  table, column, and row grants.
- **Query**: Amazon Athena workgroups enforce KMS-encrypted, expiring results and a per-query
  scan cutoff.

Account-wide, single-instance pieces sit behind one `catalog_integration_config.enabled` flag:
the federated catalog, the Lake Formation resource registration and its data-access role, the
data-lake settings, and the classification LF-tag. They live in a dedicated submodule, so only
one deployment per account and Region owns them.

Infrastructure that costs extra and exists only for compliance is created only when
`compliance_frameworks` names SOC 2, HIPAA, or PCI DSS. Today that is per-zone and query-results
KMS keys. Everything else derives its defaults from `environment_type`.

## Technical Context

**Cloud Provider**: AWS (single account, single Region per deployment)
**IaC Tool**: Terraform >= 1.11 (local and CI tested on 1.14.x)
**Provider Versions**: `hashicorp/aws >= 6.59.0, < 7.0.0`. The floor comes from:
- `aws_glue_catalog` with `federated_catalog`, added in 6.44.0;
- relaxed `catalog_id` validation in `aws_lakeformation_permissions`, needed for
  `<account>:s3tablescatalog/<bucket>` catalog IDs, in 6.29.0;
- a fix for reading cross-account principals in `aws_lakeformation_permissions`, in 6.59.0.

The latest release, 6.68.0, was verified to expose every resource and attribute used here.
**Module Versions**: No external modules. Internal submodules are referenced by relative path:
`./modules/catalog-integration`, `./modules/cost-estimation`, `./modules/deployer`.
**Curated Modules**: Considered and rejected:
- `terraform-aws-modules/s3-bucket/aws`: there is only one results bucket, and native resources
  avoid transitive provider constraints.
- `terraform-aws-modules/kms/aws`: the existing `kms.tf` pattern already meets the principles.

No curated modules exist for S3 Tables or for Lake Formation with S3 Tables federation.
**State Backend**: Consumer-owned. The module declares no backend. Examples use local state for
plan-only tests. Consumers are advised to use the S3 backend with native lockfiles
(`use_lockfile = true`) and a KMS-encrypted, versioned state bucket.
**Environment Strategy**: One module instance per environment, selected by `environment_type`.
Consumers keep separate state per environment.
**Testing**: Terratest plan-only tests in `test/`, run only through `make test`,
`make test-defaults`, `make test-complete`, `make test-enabled-false`, and new targets added for
new scenarios.
**Security Scanning**: Checkov (already in CI) and TFLint with the AWS ruleset (`.tflint.hcl`), run
via `make lint`.
**Cost Estimation**: Built-in `modules/cost-estimation` (AWS Price List API through the
`aws.pricing` provider alias). It reports idle fixed cost, including a separate
compliance-designation line, plus unit rates for usage-driven costs.
**Target Environments**: Ephemeral, Development, Testing (Baseline); UAT, Production,
MissionCritical (Enhanced); `None` for explicit configuration.
**Compliance**:
- SOC 2, HIPAA, PCI DSS: cost-incurring controls are gated by `compliance_frameworks`.
- GDPR: its controls add no cost and always apply.
- Audit trail: relies on an existing CloudTrail (FR-014).

## Principles Check

Principles v1.2.1 (`.specify/memory/principles.md`). All gates pass after the `/iac.analyze`
remediation (findings P1–P4, U1–U3, I1–I6, A1–A2, C1–C6). Deviations from the spec that research
forced are listed under Complexity Tracking.

| Principle | Status | How this plan complies |
|-----------|--------|------------------------|
| Govern Data Access by Default | ✅ | Lake Formation is the only authorization path. The catalog is created with empty `CreateDatabaseDefaultPermissions` and `CreateTableDefaultPermissions`, which removes `IAM_ALLOWED_PRINCIPALS`, and data-lake settings default permissions are likewise empty. Credential vending through a registered role means principals need no S3 Tables IAM data permissions. Grants are made only to input principals. Query results are isolated per workgroup: each writes to its own prefix, and each persona's policy document covers only its bound workgroups (FR-012a). Tables can only be created with the zone's KMS key (table bucket policy). |
| Embed Compliance into the Architecture | ✅ | No-cost controls always apply: KMS CMK, TLS-deny policies, public access block, residency (no replication), and erasure through unreferenced-file removal windows. Cost-incurring framework controls are gated by `compliance_frameworks`. A `check` block warns when a framework is designated in a Baseline environment type. |
| Optimize for Cost Efficiency | ✅ | Serverless only: S3 Tables, Glue Data Catalog, Lake Formation, and Athena. Athena has a per-query `bytes_scanned_cutoff_per_query`, and the results bucket has lifecycle expiration. Unreferenced-file removal is on. The cost estimate is extended. |
| Protect Data Durability and Recoverability | ✅ | S3 Tables snapshot management default (max snapshot age 120 h) is at least every environment type's RPO. Writers get `DROP` only in Baseline (`writer_drop_enabled`); in Enhanced only data-lake admins can drop tables, because a dropped table and its snapshots are unrecoverable. A table bucket cannot be deleted while it holds tables, and `force_destroy` is set only for Ephemeral. Enhanced uses a 30-day KMS deletion window. |
| Establish Resource Identity and Accountability | ✅ | `name_prefix` naming on every resource; `common_tags` on all and `common_data_tags` on table buckets, the results bucket, and keys. Compliance-only resources carry a `ComplianceFrameworks` tag. When `required_tags_enforced` is in effect (default in Enhanced), a precondition requires the cost-center and data-owner keys in `tags` and a data-classification key in `data_tags`. The classification value must be in `governance_config.classification_values` and at least as sensitive as every namespace classification. Key names and enforcement come from `required_tags_config`. |
| Define Small, Validated Module Contracts | ✅ | Inputs are grouped in `*_config` objects with `validation` blocks. Account-level singletons live in the `modules/catalog-integration` submodule. Outputs are ARNs and names only. The unused `networktags_name` input is removed. |
| Secure by Construction | ✅ | All policies use `aws_iam_policy_document`. Service trusts use `aws:SourceAccount` and `aws:SourceArn` conditions. Bucket and table bucket policies deny `aws:SecureTransport = false` for non-service principals (`aws:PrincipalIsAWSService = false`, because AWS redacts the TLS key on service-to-service calls). No wildcard principals in Allow statements. |
| Validate Before Merge | ✅ | Every validation step in tasks uses `make` targets. New plan-only tests are added for each feature flag and invalid-input case. The CI workflow is switched to the same `make` targets, with TFLint and terraform-docs installed. |
| Make Dependencies Explicit | ✅ | A deployment that doesn't own the catalog receives `catalog_integration_config.data_access_role_arn` as an input; it never looks it up by name. The owning deployment uses the submodule's `data_access_role_arn` output. No ARN is built from a naming convention. All data sources stay in `dependencies.tf`. |
| Lock Versions and Release Semantically | ✅ | `versions.tf` pins `aws >= 6.59.0, < 7.0.0`. Releases via release-please. |
| Environment-Driven Defaults | ✅ | New per-environment keys are added to `environment_defaults` (see Environment Configuration), including `writer_drop_enabled`, `scanned_bytes_alarm_threshold`, and `required_tags_enforced`. Each is overridable through a `*_config` input, so `None` takes explicit values like every other key. `allow_full_table_external_data_access` is an account-level governance choice owned by the catalog-integrating deployment; it defaults to `false` everywhere and does not vary by environment. |
| Compliance Floor, Cost Ceiling | ✅ | The no-cost floor is always on. Per-zone keys are created only for designated data. |
| Compose, Don't Accumulate | ✅ | Out of scope: ingestion, tables and schemas, identities, semantic layer, networking. Persona IAM policy documents are outputs; the module doesn't attach them. |

## Infrastructure Architecture

### Compute Resources

No provisioned compute. All processing is serverless and billed per use:
- **Amazon Athena** (engine version 3) runs SQL queries. It is billed per TB scanned, so idle cost
  is $0.
- **S3 Tables managed maintenance** runs compaction, snapshot expiration, and unreferenced-file
  removal. It is billed per object processed.
- No Lambda, Glue jobs, EMR, or Redshift. Ingestion and transformation compute belong to separate
  modules.

### Data Storage

**Table storage (FR-001 to FR-003): `aws_s3tables_table_bucket`, one per zone**
- Zones default to `["raw", "curated"]` and are overridable through `zones_config`.
- Bucket name: `${name_prefix}-${zone}`. Zone names are validated separately from namespace
  names: they match `^[a-z0-9]([a-z0-9-]*[a-z0-9])?$` (lowercase letters, digits, hyphens; no
  underscores), and `length(name_prefix) + 1 + length(zone)` must be at most 63.
- `encryption_configuration`: `sse_algorithm = "aws:kms"` with the shared module key. When the
  compliance designation is non-empty, each zone gets its own key instead.
- `maintenance_configuration.iceberg_unreferenced_file_removal` is enabled, with
  `unreferenced_days` and `non_current_days` set by environment type (see Environment
  Configuration).
- `force_destroy = true` only for `Ephemeral`.

**Namespaces (FR-002): `aws_s3tables_namespace`**
- One per entry in `zones_config.namespaces[zone]`. These surface in Lake Formation as databases
  `s3tablescatalog/<bucket>/<namespace>`.

**Table-level maintenance (FR-003)**
- Compaction and snapshot management can only be configured per table (`aws_s3tables_table`).
  The module creates no tables, so tables inherit the S3 Tables defaults:
  - compaction enabled;
  - snapshot management enabled, keeping at least 1 snapshot, with a 120-hour maximum age.
- 120 hours meets the largest RPO (24 hours). Writers must not disable either setting; the README
  documents this.
- Erasure purge path in Enhanced: up to 5 days of snapshot age, plus `unreferenced_days` of 3,
  plus `non_current_days` of 7, comes to about 15 days. That is within the 30-day erasure target.

**Query results (FR-011): `aws_s3_bucket` named `${name_prefix}-athena-results-${account_id}`**
- Public access block on all four settings; object ownership `BucketOwnerEnforced`.
- SSE-KMS with S3 Bucket Keys enabled (this cuts KMS request cost).
- Each workgroup writes to its own prefix, `s3://<bucket>/<workgroup>/` (FR-012a).
- Bucket policy denies non-TLS requests from non-service principals and denies uploads that are not
  KMS-encrypted.
- Lifecycle: results expire after 7 days (Baseline) or 30 days (Enhanced); incomplete multipart
  uploads are aborted after 1 day.
- Versioning is not enabled: results are transient and must actually be deleted.
- Athena managed query results (`managed_query_results_configuration`) was rejected because
  FR-011 requires a dedicated location with retention that depends on environment type.

**Catalog: `aws_glue_catalog` named `s3tablescatalog`** (catalog-integration submodule only)
- `federated_catalog`: `identifier = arn:${partition}:s3tables:${region}:${account}:bucket/*`,
  `connection_name = "aws:s3tables"`.
- `create_database_default_permissions` and `create_table_default_permissions` are empty, so Lake
  Formation grants are the only access.
- `allow_full_table_external_data_access = false` by default, configurable.

### Networking

There are no network resources. Every service used is a Regional AWS public-endpoint service
reached over TLS 1.2 or higher. No VPC is needed, and VPC endpoints are out of scope (spec, Out
of Scope). The template's `networktags_name` input is removed because nothing uses it.

### Security

**Account-level integration (FR-004, FR-005, FR-009): `modules/catalog-integration`**

This submodule is called with `count = catalog_integration_config.enabled ? 1 : 0`. It contains:

- **`aws_iam_role` named `${name_prefix}-lf-s3tables-access`**, the Lake Formation data-access
  role.
  - Trust: `lakeformation.amazonaws.com` for `sts:AssumeRole`, `sts:SetContext`, and
    `sts:SetSourceIdentity`, conditioned on `aws:SourceAccount`.
  - Policy, from `aws_iam_policy_document`:
    - `s3tables:*` table data and metadata actions on
      `arn:…:s3tables:${region}:${account}:bucket/*`;
    - `kms:Decrypt` and `kms:GenerateDataKey` on `*`, conditioned on
      `kms:ViaService = s3tables.${region}.amazonaws.com` and `aws:ResourceAccount = ${account}`.
      The role must decrypt the keys of every table bucket in the account and Region, including
      secondary deployments' keys. Each key policy does the scoping. The submodule therefore takes
      no key ARNs, which removes the dependency cycle between the keys and the role.
- **`aws_lakeformation_resource`**:
  - `arn = arn:…:s3tables:${region}:${account}:bucket/*`
  - `role_arn` set to the role above
  - `with_federation = true`
  - `hybrid_access_enabled = false`
- **`aws_glue_catalog` named `s3tablescatalog`**, as described under Data Storage. It
  `depends_on` the resource registration.
- **`aws_lakeformation_data_lake_settings`**:
  - `admins` is set exactly to `governance_config.admin_principal_arns`.
  - `create_database_default_permissions` and `create_table_default_permissions` are empty.
  - A `precondition` fails the plan if the deploying role (`aws_iam_session_context.issuer_arn`)
    is not in the admin list. Without that, Terraform would lock itself out of making grants.
  - This resource is authoritative: it also resets `trusted_resource_owners` and
    external-filtering settings. The README states this.
- **`aws_lakeformation_lf_tag` named `classification`**, used for Enhanced classification-based
  grants (FR-008). Tag keys are account-wide, so the tag lives here. Its values come from
  `governance_config.classification_values`, default `["public", "internal", "confidential",
  "restricted"]`.

Deployments that don't own the integration pass `catalog_integration_config.data_access_role_arn`.
The root module adds that role to its own table-bucket KMS key policies. The owning deployment
uses the role from its submodule's output.

A secondary deployment also creates Lake Formation grants, so its deploying role must be one of
the owner's data-lake admins. The root module reads `data "aws_lakeformation_data_lake_settings"`
when integration is disabled, and a precondition fails the plan if the deploying role is not in
`admins`.

**Grants (FR-006 to FR-008): `aws_lakeformation_permissions`**

Every grant uses `catalog_id = "${account}:s3tablescatalog/${bucket}"`.

| Persona | Scope | Permissions |
|---------|-------|-------------|
| Admins | Data-lake admins (settings) | Implicit full control |
| Writers | Each assigned zone's namespaces (database) | `CREATE_TABLE`, `DESCRIBE`, `ALTER` |
| Writers | Table wildcard in those namespaces | `SELECT`, `INSERT`, `DELETE`, `ALTER`, `DESCRIBE`, plus `DROP` only when `writer_drop_enabled` (Baseline) |
| Readers | Assigned zone or namespace, table wildcard | `SELECT`, `DESCRIBE` |
| Readers, column-restricted | `table_with_columns` for named tables, with `column_names` or `excluded_column_names` | `SELECT` |
| Readers, row-filtered | `aws_lakeformation_data_cells_filter` on named tables, then `data_cells_filter` grant | `SELECT` |
| Readers, Enhanced | `lf_tag_policy` on the reader's `classification_values` | `SELECT`, `DESCRIBE` |

- Column-restricted and row-filtered grants reference tables by name. Those tables must already
  exist (created by writers), so these inputs are optional and default to empty.
- The external semantic layer is an ordinary reader principal.
- Classification inputs:
  - `governance_config.namespace_classifications` maps each `"<zone>/<namespace>"` to one
    classification value. It is attached to the namespace with `aws_lakeformation_resource_lf_tags`.
  - Each reader's `classification_values` lists the values it may read.
  - Both are validated against `classification_values`. Namespaces without a classification are
    reachable only through explicit zone or namespace grants.
  - Lake Formation tag-based access control is supported on S3 Tables federated catalogs, with no
    catalog recreation needed. LF-tags can only be attached to resources that already exist, so
    attachments depend on the namespaces.
  - The `data_tags` classification on storage resources is the deployment's most sensitive
    classification. Validation checks that it ranks at or above every `namespace_classifications`
    value, using the order of `classification_values` (least to most sensitive).
  - The LF-tag belongs to the owning deployment. Secondary deployments must use the same
    `classification_values`, or attachments fail at apply. The README states this.

**Persona IAM policy documents (outputs only)**

The module outputs least-privilege JSON for each writer and reader entry (maps keyed by the
`governance_config` key) but never attaches it, because it doesn't own the roles. Each entry has
a `workgroups` list binding it to specific workgroups (FR-012a). Each document contains:
- `glue:GetCatalog*`, `glue:GetDatabase*`, `glue:GetTable*`;
- `lakeformation:GetDataAccess`;
- `athena:StartQueryExecution`, `GetQuery*`, `StopQueryExecution` on the bound workgroups only;
- `s3:GetObject` and `s3:PutObject` on the bound workgroups' prefixes only, plus
  `s3:GetBucketLocation`;
- KMS use on the results key, via `kms:ViaService`.

**Encryption (FR-013, FR-022)**

- Shared key: the existing `aws_kms_key.main`, which is created by default or supplied by the
  consumer. Its key policy gains statements for:
  - `maintenance.s3tables.amazonaws.com`: `kms:GenerateDataKey` and `kms:Decrypt`, with
    `StringLike kms:EncryptionContext:aws:s3:arn = arn:…:s3tables:${region}:${account}:bucket/${name_prefix}-*/*`.
    This is the pattern documented in the S3 Tables SSE-KMS permission requirements; without it
    `CreateTable` fails;
  - the Lake Formation data-access role;
  - Athena and S3 for results, via `kms:ViaService`.
- Designated data adds `aws_kms_key.zone[zone]` and `aws_kms_key.query_results`, each with the
  same policy pattern and a `ComplianceFrameworks` tag. Their count is driven by
  `local.compliance_designated = length(var.compliance_frameworks) > 0`.
- Rotation enabled; deletion window taken from `environment_defaults`.
- When the consumer supplies a key (`encryption_config.create_kms_key = false`), the module can't
  edit its policy. It outputs `required_kms_key_policy_json`, which contains the maintenance, Lake
  Formation data-access, and results statements, for the consumer to merge into their key policy.
  The README states this as a prerequisite.

**Transport and exposure**

- The results bucket policy denies requests where `aws:SecureTransport = false` and
  `aws:PrincipalIsAWSService = false`. AWS redacts `aws:SecureTransport` on service-to-service
  calls, so this exclusion keeps Athena's forward-access requests working.
- Table bucket policy (`aws_s3tables_table_bucket_policy`) grants nothing and has three deny
  statements:
  - `DenyInsecureTransport`: denies `s3tables:*` with the same two conditions. Table bucket
    policies accept global condition keys.
  - `EnforceKMSEncryptionAlgorithm`: denies `s3tables:CreateTable` when
    `StringNotEqualsIfExists s3tables:SSEAlgorithm` is not `aws:kms`.
  - `EnforceKMSEncryptionKey`: denies `s3tables:CreateTable` when
    `StringNotEqualsIfExists s3tables:KMSKeyArn` is not the zone's key.

  `IfExists` matters here. Writers that create tables through Athena, Glue, or Lake Formation
  vended credentials usually state no encryption, and the table inherits the bucket default (the
  zone key). A plain `StringNotEquals` would deny those requests when the key is absent.

  The last two stop writers from creating tables with weaker or foreign encryption (FR-013).

**Compliance designation (FR-021 to FR-024)**

- `variable "compliance_frameworks"`: `set(string)`, default `[]`, validated against
  `["SOC2", "HIPAA", "PCI_DSS"]`.
- A `check "regulated_data_in_baseline"` block warns when the set is non-empty and the environment
  type is `Ephemeral`, `Development`, or `Testing`.

**Audit (FR-014)**

No trail is created. The README lists the CloudTrail configuration consumers need:
- management events for `lakeformation`, `glue`, `s3tables`, and `athena`;
- data events for `AWS::S3Tables::Table`;
- retention of at least 90 days, or at least 1 year when designated.

**Deployer (FR-019)**

`modules/deployer` is extended with least-privilege actions for:
- `s3tables`: table buckets, namespaces, policies, and maintenance;
- `glue`: catalog;
- `lakeformation`: resource registration, settings, permissions, LF-tags, and data cells filters.
  All actions are listed explicitly; there are no wildcards inside action names;
- `athena`: workgroups;
- `s3`: the results bucket;
- `kms`;
- `iam`: creating the role, putting its policy, and `PassRole`, with `iam:PassedToService =
  lakeformation.amazonaws.com`;
- `cloudwatch`: alarms.

### Environment Configuration

These keys are added to `locals.environment_defaults`. `None` takes the explicit `*_config`
values.

| Key | Ephemeral | Development | Testing | UAT | Production | MissionCritical |
|-----|-----------|-------------|---------|-----|------------|-----------------|
| `query_results_expiration_days` | 1 | 7 | 7 | 30 | 30 | 30 |
| `bytes_scanned_cutoff_per_query` | 10 GB | 10 GB | 10 GB | 1 TB | 1 TB | 1 TB |
| `unreferenced_days` / `non_current_days` | 1 / 1 | 3 / 7 | 3 / 7 | 3 / 7 | 3 / 7 | 3 / 7 |
| `force_destroy` | true | false | false | false | false | false |
| `classification_grants_enabled` | false | false | false | true | true | true |
| `writer_drop_enabled` | true | true | true | false | false | false |
| `required_tags_enforced` | false | false | false | true | true | true |
| `scanned_bytes_alarm_threshold` (per day) | 100 GB | 100 GB | 100 GB | 10 TB | 10 TB | 10 TB |
| `monitoring_enabled` / `alarms_enabled` | existing | existing | existing | existing | existing | existing |
| `kms_key_deletion_window_days` | existing | existing | existing | existing | existing | existing |

New inputs, grouped as objects with validation:

- **`zones_config`**: `{ zones = list(string), namespaces = map(list(string)) }`. Validated:
  - zones match `^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`, and the resulting bucket name is at most 63
    characters (a cross-variable check against `name_prefix`);
  - namespaces match `^[a-z0-9_]{1,255}$` and only reference declared zones.
- **`catalog_integration_config`**: `{ enabled = bool, data_access_role_arn = string,
  allow_full_table_external_data_access = bool }`. Validated: `data_access_role_arn` is required
  when `enabled = false`.
- **`governance_config`**: `{ admin_principal_arns = list(string), writers = map(object),
  readers = map(object), classification_values = list(string),
  namespace_classifications = map(string), reader_column_grants = map(object),
  reader_row_filters = map(object) }`.
  - Writer and reader objects include `principal_arn`, `zones`, and `workgroups`; readers add
    `namespaces` and `classification_values`.
  - Validated: ARNs are well formed; every zone, namespace, workgroup, and classification value
    referenced is declared.
- **`required_tags_config`**: `{ enforced = bool, cost_center_key = string, data_owner_key =
  string, data_classification_key = string }`.
  - Defaults: `enforced = null`, so the environment default applies; keys `CostCenter`,
    `DataOwner`, `DataClassification`.
  - The key names let consumers whose context module uses different keys map them.
  - Enforcement is a `lifecycle.precondition` on `terraform_data.required_tags_guard`, driven by
    `local.effective_config.required_tags_enforced`. Variable validation can't read locals, so this
    keeps one source of truth across all environment types, including `None`.
- **`governance_config.writer_drop_enabled`**: nullable bool, `null` by default so the environment
  default applies. Under `None` the explicit value is required.
- **`query_config`**: `{ workgroups = list(string), bytes_scanned_cutoff_per_query = number,
  results_expiration_days = number }`. Validated: cutoff of at least 10 MB, the Athena minimum.
- **`compliance_frameworks`**: `set(string)`.

**Monitoring (FR-017)**

- Workgroups set `publish_cloudwatch_metrics_enabled` to the effective `monitoring_enabled`.
- When alarms are enabled, each workgroup gets two `aws_cloudwatch_metric_alarm` resources:
  - failed queries: `AWS/Athena` metric `TotalExecutionTime` sample count with
    `QueryState = FAILED`;
  - scanned bytes: `ProcessedBytes` sum over 1 day above the effective
    `scanned_bytes_alarm_threshold` (environment default, overridable in `alarms_config`).
- Both alarms route to the existing SNS topic.

### Complexity Level

**Selected: Baseline and Enhanced in one module, chosen at runtime by `environment_type`, plus an
orthogonal compliance designation.**

- **Baseline** (Ephemeral, Development, Testing):
  - shared CMK;
  - 10 GB scan cutoff;
  - 7-day result retention (1 day for Ephemeral);
  - no classification grants;
  - monitoring and alarms off;
  - idle at most $5 per month.
- **Enhanced** (UAT, Production, MissionCritical):
  - classification-based grants;
  - 1 TB scan cutoff;
  - 30-day result retention;
  - monitoring and alarms on;
  - 30-day KMS deletion window;
  - idle at most $10 per month.
- **Compliance designation**, in any environment type: per-zone and results KMS keys, adding about
  $3 per month for two zones plus results, within the $10 ceiling.

Rationale: the infrastructure is serverless in both tiers. Only controls, retention, and
observability differ, so one code path with environment-driven defaults is simpler than separate
variants.

### State Management

- The module is a reusable library and declares no `backend`. Consumers own state.
- Recommended consumer backend:
  - S3 with `use_lockfile = true` (native S3 locking; no DynamoDB table);
  - a versioned, SSE-KMS state bucket with public access blocked;
  - one state file per environment.
- State contains no secrets: the module accepts no credentials and outputs none.
- Examples use local state and are exercised only by plan-only Terratest through `make test`.
  `make plan-examples`, `make apply-examples`, and `make destroy-examples` are used for manual
  end-to-end checks against a sandbox account.
- Account-level singletons (the federated catalog and data-lake settings) must be in exactly one
  state per account and Region. Importing an existing `s3tablescatalog` is documented in the README
  (`terraform import` of `aws_glue_catalog`) for accounts where integration was enabled in the
  console.

## Project Structure

### Documentation (this infrastructure)

```text
specs/001-governed-lakehouse/
├── spec.md              # Infrastructure specification (technology-agnostic) - /iac.specify
├── plan.md              # This file - architecture plan - /iac.plan
├── checklists/
│   └── requirements.md  # Spec quality checklist
└── tasks.md             # Implementation tasks - /iac.tasks
```

### Source Code (repository root)

```text
.
├── versions.tf               # Terraform >= 1.11; aws >= 6.59.0, < 7.0.0 with aws.pricing alias
├── dependencies.tf           # caller identity, partition, region, iam_session_context,
│                             #   lakeformation_data_lake_settings (secondary deployments only)
├── variables.tf              # + zones_config, catalog_integration_config, governance_config,
│                             #   query_config, required_tags_config, compliance_frameworks;
│                             #   - networktags_name
├── locals.tf                 # environment_defaults extended; zone/namespace/grant maps; designation
├── main.tf                   # catalog-integration submodule call; check block (FR-024)
├── kms.tf                    # shared key policy extended; per-zone + results keys (designated)
├── table-buckets.tf          # aws_s3tables_table_bucket, _namespace, _table_bucket_policy
├── permissions.tf            # aws_lakeformation_permissions, _data_cells_filter, LF-tag grants
├── query.tf                  # aws_athena_workgroup; results aws_s3_bucket + controls
├── iam-documents.tf          # persona aws_iam_policy_document outputs (not attached)
├── alarms.tf                 # SNS topic (existing) + Athena alarms
├── cost-estimation.tf        # pricing submodule inputs extended
├── outputs.tf                # bucket ARNs, catalog IDs, namespaces, workgroups, key ARNs, policies
├── modules/
│   ├── catalog-integration/  # NEW: LF data-access role, LF resource registration,
│   │                         #   s3tablescatalog, data-lake settings, classification LF-tag
│   ├── cost-estimation/      # extended: S3 Tables, Athena, S3 results, extra KMS keys
│   └── deployer/             # extended: least-privilege deploy/destroy policy
├── examples/
│   ├── defaults/             # name_prefix + admin principal only; catalog integration enabled
│   ├── complete/             # all inputs incl. designation, readers with column/row filters
│   └── secondary/            # NEW: catalog_integration_config.enabled = false + role ARN input
├── test/
│   ├── main_test.go          # existing helpers (base options, name prefix, retryable errors)
│   ├── defaults_test.go      # resources present; catalog integration resources present
│   ├── complete_test.go      # designation → per-zone keys for each framework; classification
│   │                         #   grants; no writer DROP in Enhanced; existing TestEnabledFalse
│   ├── secondary_test.go     # NEW: no catalog/settings resources when flag disabled
│   └── validation_test.go    # NEW: invalid inputs fail at plan (incl. missing Enhanced tags);
│                             #   FR-024 warning appears for designated data in Development
├── .github/workflows/
│   └── terraform-ci.yml      # switched to make targets; installs TFLint and terraform-docs
├── Makefile                  # + test-secondary, test-validation, security-scan targets
└── README.md                 # usage, audit-trail prerequisites, singleton ownership, import
```

**Structure Decision**: The module keeps the repository-root layout that the template and
`AGENTS.md` define, with files split by concern. It is not placed in `iac/`: this repository is
itself a Terraform module, and the Registry layout requires root `.tf` files.

Account-level singleton resources are extracted into `modules/catalog-integration`. That gives
them one reason to change, makes ownership explicit, and lets a platform team call them
standalone.

`examples/secondary` and the new tests exercise the feature-flag contracts the spec requires.

## Complexity Tracking

These are spec adjustments that research forced, not principle violations.

| Spec item | Adjustment | Reason |
|-----------|------------|--------|
| FR-003 (compaction and snapshot expiration per zone) | Resolved: spec FR-003 now matches. The module configures unreferenced-file removal per zone; compaction and snapshot management use the S3 Tables per-table defaults and the README tells writers to keep them. | These settings exist only on `aws_s3tables_table`, and the module creates no tables (spec, Out of Scope). The defaults satisfy every RPO and the 30-day erasure target. |
| FR-008 (column and row filters) | Column- and row-filter grants require table names that already exist. They are optional inputs applied after writers create the tables. | Lake Formation data cells filters and column grants target concrete tables. |
| FR-009 (admins set exactly to the input list) | A plan precondition requires the deploying role to be in the admin list. Secondary deployments check the same thing against the live settings. | Without that, Terraform could remove its own admin rights and fail every later grant. |
| FR-005 (non-owner deployments) | Non-owner deployments must pass the integration's data-access role ARN. | That role must be granted use of every table bucket's KMS key, and the principles forbid name-based lookups. |
