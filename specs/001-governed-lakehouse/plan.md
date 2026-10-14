# Architecture Plan: Governed Multi-Tenant Lakehouse Platform

**Branch**: `001-governed-lakehouse` | **Date**: 2026-10-14 | **Spec**: [spec.md](./spec.md)
**Input**: Infrastructure specification from `/specs/001-governed-lakehouse/spec.md`

**Note**: This template is filled in by the `/iac.plan` command. For deep research, module specs, and quickstart guide, run `/iac.enrichplan` after planning.

**Revision**: 2026-10-14. The plan was redesigned as a multi-tenant platform: one tenant per S3
Tables namespace (Lake Formation database), with LF-tag-based access control. It supersedes the
2026-10-13 single-tenant plan.

## Summary

A reusable Terraform module provisions a governed, Apache Iceberg–based lakehouse platform in one
AWS account and Region. The platform hosts many tenants.

- **Tenancy**: each tenant is one S3 Tables namespace, named after the tenant, in each zone it
  uses. In Lake Formation that namespace surfaces as the database
  `s3tablescatalog/<bucket>/<tenant>`. Everything a tenant owns is provisioned by the
  `modules/tenant` submodule. The submodule has two entry points: the root module calls it once
  per entry in the `tenants` map, or a tenant provisioning pipeline calls it standalone with its
  own state. Both consume the root's `platform_contract` output.
- **Storage**: one shared Amazon S3 Tables table bucket per data zone (`raw`, `curated`,
  `consumption`) holds every tenant's namespaces unless the tenant has dedicated storage. A tenant
  gets dedicated table buckets per zone when it sets `dedicated_storage` (for capacity or
  isolation) or is designated under SOC 2, HIPAA, or PCI DSS. Designated tenants' buckets are
  encrypted with their own KMS key; other dedicated buckets use the platform key.
- **Catalog**: the Region's federated `s3tablescatalog` in the AWS Glue Data Catalog exposes the
  tables to analytics services.
- **Access**: AWS Lake Formation authorizes all access through credential vending.
  - Module-managed grants are LF-tag expression grants (LF-TBAC) over deployment-scoped LF-tags:
    - a per-tenant `<prefix>-tenant-<tenant>`, owned by that tenant's submodule;
    - the platform-owned `<prefix>-sensitivity`, one `<prefix>-reg-<regulation>` per regulation,
      and `<prefix>-domain`.
  - The module labels tenant databases with the tenant's most restrictive values. Writer pipelines
    label tables and columns and apply the column-to-table roll-up. Stewards correct labels and
    grant within the tenant.
  - Row filters (data cells filters) are the only named-resource grants.
- **Query**: one Amazon Athena workgroup per tenant enforces KMS-encrypted, expiring results under
  a tenant prefix and a per-query scan cutoff.

Account-wide, single-instance pieces sit behind one `catalog_integration_config.enabled` flag:
the federated catalog, the Lake Formation resource registration and its data-access role, and the
data-lake settings. They live in a dedicated submodule, so only one deployment per account and
Region owns them.

Infrastructure that costs extra and exists only for compliance is created per designated tenant:
a tenant KMS key, plus dedicated table buckets and results encryption that use it. Everything
else derives its defaults from `environment_type`.

## Technical Context

**Cloud Provider**: AWS (single account, single Region per deployment)
**IaC Tool**: Terraform >= 1.11 (local and CI tested on 1.14.x). `optional()` object attributes
are used in the `tenants` type.
**Provider Versions**: `hashicorp/aws >= 6.59.0, < 7.0.0`. The floor comes from:
- `aws_glue_catalog` with `federated_catalog`, added in 6.44.0;
- relaxed `catalog_id` validation in `aws_lakeformation_permissions`, needed for
  `<account>:s3tablescatalog/<bucket>` catalog IDs, in 6.29.0;
- a fix for reading cross-account principals in `aws_lakeformation_permissions`, in 6.59.0.

The Terraform argument that sets a table bucket's default storage class (for Intelligent-Tiering,
added to S3 Tables in December 2025) is not yet confirmed, nor is the minimum provider version
that has it. T049 confirms both. If that version is above 6.59.0, the floor rises and
`versions.tf` changes accordingly.

The latest release, 6.68.0, was verified to expose every resource and attribute used here. Two
items are confirmed in the sandbox during T049:
- `lf_tag_policy` grants scoped to an S3 Tables sub-catalog `catalog_id`;
- the `GRANT_WITH_LF_TAG_EXPRESSION` LF-tag permission.

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
**Environment Strategy**: One platform instance per environment, selected by `environment_type`.
Consumers keep separate state per environment. Tenants are configuration inside a platform
instance, not separate instances.
**Testing**: Terratest plan-only tests in `test/`, run only through `make test`,
`make test-defaults`, `make test-complete`, `make test-enabled-false`, and new targets added for
new scenarios.
**Security Scanning**: Checkov (already in CI) and TFLint with the AWS ruleset (`.tflint.hcl`), run
via `make lint`.
**Cost Estimation (FR-024)**: Built-in `modules/cost-estimation`, which reads the AWS Price List
API through the `aws.pricing` provider alias. It uses only `aws_pricing_product` data sources, so
it creates no resources.
- `cost_estimation_config.enabled` (default `true`) gates it. The flag travels in the platform
  contract as `cost_estimation_enabled`, so the tenant unit follows the platform setting.
- Both the root (platform resources) and `modules/tenant` (one tenant) call it.
- Outputs:
  - `monthly_cost_estimate`: the idle fixed total;
  - `cost_breakdown`: per service;
  - `compliance_monthly_cost`;
  - `usage_rates`: S3 Tables storage per GB-month, maintenance per object, Athena per TB
    scanned, and S3 Standard per GB-month.
- The root `monthly_cost_estimate` sums the platform estimate and the platform-applied tenants.
**Target Environments**: Ephemeral, Development, Testing (Baseline); UAT, Production,
MissionCritical (Enhanced); `None` for explicit configuration.
**Compliance**:
- SOC 2, HIPAA, PCI DSS: cost-incurring controls are gated per tenant by
  `tenants[*].compliance_frameworks`.
- GDPR: its controls add no cost and always apply.
- Audit trail: relies on an existing CloudTrail (FR-020).

## Principles Check

Principles v1.3.0 (`.specify/memory/principles.md`). The tenant isolation and tag-based access
guidance was added in that amendment. Deviations from the spec that research forced are listed
under Complexity Tracking.

| Principle | Status | How this plan complies |
|-----------|--------|------------------------|
| Govern Data Access by Default | ✅ | Lake Formation is the only authorization path. The catalog and data-lake settings are created with empty default permissions, which removes `IAM_ALLOWED_PRINCIPALS`. Module-managed grants, made only to input principals, are LF-tag expressions scoped by tenant, sensitivity, regulation, and optionally domain (FR-009). Unlabeled tables inherit the tenant's most restrictive database labels and fail closed. Stewards cannot `ASSOCIATE` the tenant tag, so they cannot move data between tenants. Results are isolated by tenant workgroup and prefix (FR-018). Tables can only be created with the bucket's KMS key (table bucket policy). |
| Embed Compliance into the Architecture | ✅ | No-cost controls always apply: KMS CMK, TLS-deny policies, public access block, residency (no replication), tenant isolation, tag-based sensitivity and regulation labels, and erasure through unreferenced-file removal windows. Cost-incurring framework controls are gated per tenant. A `check` block warns when a designated tenant exists in a Baseline environment type. |
| Optimize for Cost Efficiency | ✅ | Serverless only: S3 Tables, Glue Data Catalog, Lake Formation, and Athena. Non-designated tenants share zone buckets and add $0 idle cost. Each tenant workgroup has a `bytes_scanned_cutoff_per_query`, and the results bucket has lifecycle expiration. Per-tenant workgroups give usage attribution. The cost estimate is extended. Enhanced table buckets default new tables to the S3 Tables Intelligent-Tiering storage class (FR-005), which moves data to Infrequent Access after 30 days and Archive Instant Access after 90. Baseline uses Standard because its data is short-lived. Query results expire instead of tiering. |
| Protect Data Durability and Recoverability | ✅ | The S3 Tables snapshot management default (max snapshot age 120 h) is at least every environment type's RPO. Writers get `DROP` only in Baseline (`writer_drop_enabled`). In Enhanced only data-lake admins and stewards can drop tables. A table bucket cannot be deleted while it holds tables, and `force_destroy` is set only for Ephemeral. Enhanced uses a 30-day KMS deletion window. |
| Establish Resource Identity and Accountability | ✅ | `name_prefix` naming on every resource, and `common_tags` on all. Tenant resources also carry the `Tenant` tag and `DataOwner = tenant.owner`. Storage carries a computed `DataClassification` (FR-022). Compliance-only resources carry `ComplianceFrameworks`. In Enhanced, a precondition requires the cost-center and data-owner keys in `tags`. |
| Define Small, Validated Module Contracts | ✅ | Inputs are grouped in `*_config` objects plus one `tenants` map, all with `validation` blocks. Account-level singletons live in `modules/catalog-integration`, and everything tenant-scoped lives in `modules/tenant`. That submodule is applied by the root or called standalone through the typed `platform_contract`. Outputs are ARNs, names, and policy JSON only. The unused `networktags_name` input is removed. |
| Secure by Construction | ✅ | All policies use `aws_iam_policy_document`. Service trusts use `aws:SourceAccount` and `aws:SourceArn` conditions. Bucket and table bucket policies deny `aws:SecureTransport = false` for non-service principals. The results bucket denies designated-tenant prefixes any KMS key other than the tenant's. No wildcard principals in Allow statements. |
| Validate Before Merge | ✅ | Every validation step in tasks uses `make` targets. New plan-only tests cover tenancy, each feature flag, per-tenant designation, and invalid inputs. The CI workflow is switched to the same `make` targets, with TFLint and terraform-docs installed. |
| Make Dependencies Explicit | ✅ | A deployment that doesn't own the catalog receives `catalog_integration_config.data_access_role_arn` as an input; it never looks it up by name. Principals are inputs. The tenant submodule depends on the platform only through the typed `platform_contract` object, never through name-based lookups. Data sources stay in each module's `dependencies.tf`. |
| Lock Versions and Release Semantically | ✅ | `versions.tf` pins `aws >= 6.59.0, < 7.0.0`. Releases via release-please. |
| Environment-Driven Defaults | ✅ | New per-environment keys are added to `environment_defaults` (see Environment Configuration). Each is overridable through a `*_config` input or a tenant attribute, so `None` takes explicit values. |
| Compliance Floor, Cost Ceiling | ✅ | The no-cost floor, including tenant isolation and tag-based grants, is always on. Tenant keys and dedicated buckets are created only for designated tenants. |
| Compose, Don't Accumulate | ✅ | Out of scope: ingestion, transformation between zones, tables and schemas, table and column labels (writer pipelines and stewards own them, per the roll-up contract), identities, onboarding portals, semantic layer, networking. Persona IAM policy documents are outputs; the module doesn't attach them. |

## Infrastructure Architecture

### Tenancy Model

| Concept | AWS realization |
|---------|-----------------|
| Platform | Root module instance: catalog integration, zone table buckets, the platform-owned LF-tags, the shared results bucket, the platform key, the SNS topic, and the `platform_contract` output |
| Tenant unit | One `modules/tenant` instance: everything listed below that is tenant-scoped |
| Zone | Shared table bucket `${name_prefix}-${zone}` (catalog `<account>:s3tablescatalog/${name_prefix}-${zone}`) |
| Tenant | S3 Tables namespace `<tenant>` in each assigned zone's bucket, which is the Lake Formation database `<tenant>` in that bucket's catalog |
| Dedicated tenant | Same namespaces, in dedicated buckets `${name_prefix}-${zone}-${tenant}` when `dedicated_storage = true` or the tenant is designated. Encrypted with the tenant key `module.tenant["<tenant>"].aws_kms_key.this` when designated, otherwise the platform key |
| Designated tenant | A dedicated tenant that also gets its own KMS key and a dedicated results bucket (FR-028) |
| Tenant boundary | Tenant-owned LF-tag `${name_prefix}-tenant-<tenant> = member` on each tenant database |
| Sensitivity, regulations, domain | LF-tags `${name_prefix}-sensitivity`, `${name_prefix}-reg-<r>`, `${name_prefix}-domain` on each tenant database (= tenant's most restrictive values); pipelines label tables and columns with roll-up |
| Query isolation | Workgroup `${name_prefix}-${tenant}`, writing to results prefix `s3://<shared results>/<tenant>/`. A designated tenant writes to its own bucket `${name_prefix}-${tenant}-athena-results-${account_id}` |

Inside the tenant submodule, `local.zones` is the tenant's zone set. Every zone-scoped resource
uses it with `for_each`. The root instantiates the submodule with `for_each = var.tenants`, so
onboarding or removing a tenant adds or removes only `module.tenant["<tenant>"]`. A standalone
tenant touches no platform resource at all.

### Tenant Submodule and Platform Contract (FR-001a)

**`modules/tenant`** is the single implementation of a tenant. It has two entry points:

```hcl
# Platform-applied (root main.tf)
module "tenant" {
  source   = "./modules/tenant"
  for_each = var.enabled ? var.tenants : {}
  providers = { aws = aws, aws.pricing = aws.pricing }

  platform               = local.platform_contract
  name                   = each.key
  tenant                 = each.value
  verify_deployer_admin  = false # the root already guards (FR-013)
}

# Standalone (tenant provisioning pipeline, separate state)
module "tenant_sales" {
  source   = "git::https://github.com/kbrockhoff/terraform-aws-lakehouse.git//modules/tenant?ref=vX.Y.Z"
  platform = data.terraform_remote_state.platform.outputs.platform_contract # or a decoded SSM parameter
  name     = "sales"
  tenant   = { owner = "...", sensitivity = "confidential", ... }
  tags     = module.context.tags
}
```

- **Inputs**:
  - `platform`: a typed object, the platform contract below;
  - `name`: the tenant name;
  - `tenant`: the same object type as one `var.tenants` value, so one definition works in both
    entry points;
  - `tags`: optional extra tags;
  - `verify_deployer_admin`: bool, default `true`. When true, the submodule reads
    `aws_lakeformation_data_lake_settings` and fails the plan if the deploying role is not a
    data-lake admin.
- **Providers**: `aws` and `aws.pricing`, the same as the root.
- **Validation**: all tenant validation lives in the submodule's `variables.tf`, as cross-variable
  validation against `var.platform`, which Terraform >= 1.9 supports. That covers zones, ARNs,
  sensitivity (including the shared-storage classification limit for non-dedicated tenants),
  regulations, domain, row filters, cutoffs, and bucket names. Both entry points
  therefore enforce identical rules, and the root's `tenants` variable keeps only its type.
- **Resources owned**, all tenant-scoped:
  - the tenant LF-tag;
  - namespaces;
  - database tag attachments;
  - persona grants, LF-tag permissions, and data cells filters;
  - the workgroup and alarms;
  - for dedicated tenants: dedicated table buckets and their policies;
  - for designated tenants: the KMS key and the dedicated results bucket;
  - the FR-030 `check` block;
  - the tenant cost estimate (`../cost-estimation`).
- **Outputs**:
  - `namespaces`, `catalog_ids`, and `table_bucket_arns`, each a map by zone;
  - `tenant_tag_key`, `workgroup_name`, `query_results_location`, and `kms_key_arn`;
  - `persona_policy_json`, a map keyed `"<persona>/<key>"`;
  - `monthly_cost_estimate` and `compliance_monthly_cost`.

**`platform_contract`**, a root output and the submodule's `platform` input. It is a single object
of identifiers and effective settings only, never resource objects:

| Field | Content |
|-------|---------|
| `name_prefix`, `account_id`, `partition`, `region` | Naming and ARN context |
| `environment_type`, `baseline_environment` | For FR-030 and defaults |
| `effective` | `bytes_scanned_cutoff_per_query`, `query_results_expiration_days`, `unreferenced_days`, `non_current_days`, `force_destroy`, `writer_drop_enabled`, `monitoring_enabled`, `alarms_enabled`, `scanned_bytes_alarm_threshold`, `kms_key_deletion_window_days`, `table_storage_class` |
| `shared_storage_classification` | Classification tag of the shared storage; non-dedicated tenant ceilings must rank at or below it (FR-022) |
| `zones` | Map of zone to `{ bucket_name, bucket_arn, catalog_id }` for the shared buckets |
| `lf_tags` | `{ sensitivity = { key, values }, regulations = map(r => key), domain = { key, values }, tenant_key_prefix }` |
| `regulation_designation` | `{ hipaa = "HIPAA", pci_dss = "PCI_DSS" }` |
| `kms_key_arn` | The platform key, used by non-designated tenants |
| `data_access_role_arn` | For tenant key policies |
| `query_results` | `{ bucket_name, bucket_arn }` for the shared results bucket |
| `alarm_sns_topic_arn` | Alarm routing |
| `cost_estimation_enabled` | Whether tenant units produce a cost estimate (FR-024) |
| `common_tags`, `data_tags`, `required_tags` | Tag keys, the `Tenant` key, and the classification key |

- **Publishing**: the contract contains no secrets. The README shows two ways to share it with
  pipelines. One is Terraform remote state. The other is publishing it with
  `jsonencode(module.lakehouse.platform_contract)` to an SSM parameter in the consumer's root
  module and reading it with `jsondecode`. The module itself does not create that parameter
  (Compose, Don't Accumulate).
- **Versioning**: the contract is part of the module's public interface. Adding fields is a
  minor change; removing or renaming one is a major change. The tenant submodule pins the
  contract shape through its `platform` variable type.
- **Moving a tenant between entry points**: the README documents `terraform state mv` (or
  `removed` and `import` blocks) between `module.tenant["x"]` in platform state and the
  pipeline's module address. Resource addresses inside the submodule are identical in both
  entry points.

### Compute Resources

No provisioned compute. All processing is serverless and billed per use:
- **Amazon Athena** (engine version 3) runs SQL queries. It is billed per TB scanned, so idle cost
  is $0.
- **S3 Tables managed maintenance** runs compaction, snapshot expiration, and unreferenced-file
  removal. It is billed per object processed.
- No Lambda, Glue jobs, EMR, or Redshift. Ingestion and transformation compute belong to separate
  modules.

### Data Storage

**Shared zone storage (FR-004, FR-005): `aws_s3tables_table_bucket.zone`, one per zone**
- Zones default to `["raw", "curated", "consumption"]` and are overridable through `zones_config`.
  - Raw holds landed source data, curated holds conformed data, and consumption holds published
    data products (the medallion "gold" layer).
  - The README documents each zone's purpose and typical persona grants.
- The shared bucket is created for every zone, even before any tenant uses it. An empty bucket has
  no idle cost, and this keeps bucket addresses stable as tenants come and go.
- Name `${name_prefix}-${zone}`. Zone names match `^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`, and
  `length(name_prefix) + 1 + length(zone) <= 63`.
- `encryption_configuration`: `aws:kms` with the platform key (`local.kms_key_arn`).
- `maintenance_configuration.iceberg_unreferenced_file_removal` is enabled, with
  `unreferenced_days` and `non_current_days` from environment defaults.
- Default storage class for new tables: the effective `table_storage_class` (FR-005).
- `force_destroy = true` only for `Ephemeral`.
- The `DataClassification` tag is `local.shared_storage_classification`, a platform setting that
  never depends on tenants (FR-022). The tenant unit rejects non-dedicated tenants whose ceiling
  ranks above it.

**Dedicated tenant storage (FR-002, FR-028): `aws_s3tables_table_bucket.dedicated`, tenant submodule**
- One per tenant zone, created when `local.dedicated = var.tenant.dedicated_storage ||
  local.designated`.
- Name `${name_prefix}-${zone}-${tenant}`. A designated tenant's bucket is encrypted with the tenant
  key (`aws_kms_key.this`); a dedicated but non-designated tenant's bucket uses the platform key.
  Maintenance, default storage class, and `force_destroy` match the shared buckets.
- Dedicated storage gives the tenant its own 10,000-table quota per zone, for large tenants
  (spec, Scalability). It has no idle cost.
- Tags add `Tenant`, the tenant's `DataOwner`, and, only when designated, `ComplianceFrameworks`.
- Validation:
  - `length(name_prefix) + length(zone) + length(tenant) + 2 <= 63`;
  - no dedicated bucket name equals a shared bucket name. For example, a zone `raw-x` would
    collide with tenant `x` in zone `raw`.

**Tenant namespaces (FR-001, FR-002): `aws_s3tables_namespace.this`, tenant submodule**
- One per tenant zone. `namespace = <tenant>`, and `table_bucket_arn` is either the dedicated
  bucket or `platform.zones[zone].bucket_arn`.
- Tenant names match `^[a-z][a-z0-9]{1,31}$`. That subset is valid both as an S3 Tables namespace
  (no hyphens) and as part of a table bucket name (no underscores).

**Table-level maintenance (FR-005)**
- Compaction and snapshot management can only be configured per table (`aws_s3tables_table`).
  The module creates no tables, so tables inherit the S3 Tables defaults:
  - compaction enabled;
  - snapshot management enabled, keeping at least 1 snapshot, with a 120-hour maximum age.
- 120 hours meets the largest RPO (24 hours). Writers must not disable either setting; the README
  documents this.
- Erasure purge path in Enhanced: up to 5 days of snapshot age, plus `unreferenced_days` of 3,
  plus `non_current_days` of 7, comes to about 15 days. That is within the 30-day erasure target.

**Query results (FR-016, FR-018): `aws_s3_bucket` named `${name_prefix}-athena-results-${account_id}`**
- Public access block on all four settings; object ownership `BucketOwnerEnforced`.
- Default SSE-KMS with the platform key and S3 Bucket Keys enabled.
- Each tenant workgroup writes to `s3://<bucket>/<tenant>/`.
- The bucket policy denies:
  - non-TLS requests from non-service principals;
  - uploads that are not KMS-encrypted.

  The policy has no per-tenant statements, so a standalone tenant never needs to edit it.
- Lifecycle: results expire after 7 days (Baseline) or 30 days (Enhanced); incomplete multipart
  uploads are aborted after 1 day.
- Versioning is not enabled: results are transient and must actually be deleted.

**Designated tenant results (FR-028): `aws_s3_bucket.tenant_results`, tenant submodule**
- Named `${name_prefix}-${tenant}-athena-results-${account_id}`, with the same controls as the
  shared bucket.
- Default encryption uses the tenant key, and the bucket policy denies uploads under any other
  KMS key.
- Expiration comes from `platform.effective.query_results_expiration_days`.

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

**Account-level integration (FR-006, FR-007, FR-013): `modules/catalog-integration`**

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
      Each key policy (platform and tenant) does the scoping, so the submodule takes no key ARNs.
      That removes the dependency cycle between the keys and the role.
- **`aws_lakeformation_resource`**:
  - `arn = arn:…:s3tables:${region}:${account}:bucket/*`
  - `role_arn` set to the role above
  - `with_federation = true`
  - `hybrid_access_enabled = false`
- **`aws_glue_catalog` named `s3tablescatalog`**, as described under Data Storage. It
  `depends_on` the resource registration.
- **`aws_lakeformation_data_lake_settings`**:
  - `admins` is set exactly to `governance_config.admin_principal_arns`, the platform admins.
  - `create_database_default_permissions` and `create_table_default_permissions` are empty.
  - A `precondition` fails the plan if the deploying role (`aws_iam_session_context.issuer_arn`)
    is not in the admin list. Without that, Terraform would lock itself out of making grants.
  - This resource is authoritative: it also resets `trusted_resource_owners` and
    external-filtering settings. The README states this.

Deployments that don't own the integration pass `catalog_integration_config.data_access_role_arn`.
A secondary deployment also creates Lake Formation grants and LF-tags, so its deploying role must
be one of the owner's data-lake admins. The root module reads
`data "aws_lakeformation_data_lake_settings"` when integration is disabled, and a precondition
fails the plan if the deploying role is not in `admins`.

**LF-tag vocabulary (FR-009): `aws_lakeformation_lf_tag`**

| Resource | Owner | Key | Values |
|----------|-------|-----|--------|
| `aws_lakeformation_lf_tag.tenant` | tenant submodule | `${name_prefix}-tenant-${tenant}` | `["member"]` |
| `aws_lakeformation_lf_tag.sensitivity` | root | `${name_prefix}-sensitivity` | `governance_config.sensitivity_values`, ordered least to most sensitive, default `["public", "internal", "confidential", "restricted"]` |
| `aws_lakeformation_lf_tag.regulation[r]` | root | `${name_prefix}-reg-${r}` | `["no", "yes"]`, one tag per `governance_config.regulation_values`, default `["gdpr", "hipaa", "pci_dss"]` |
| `aws_lakeformation_lf_tag.domain` | root | `${name_prefix}-domain` | `governance_config.domain_values`, default `["general"]` |

- **Per-tenant keys**: each tenant owns its tenant key, so neither entry point modifies a shared
  LF-tag when a tenant is added or removed (FR-001, FR-001a). Isolation is unchanged:
  - only the owning tenant's databases carry the key;
  - grant expressions include it;
  - only platform admins can `ASSOCIATE` it.

  Lake Formation limits LF-tag keys per account (1,000 by default at planning time; verify in
  Service Quotas). That limit, minus the platform keys, bounds the tenant count, and the README
  lists it.

- **Per-regulation keys**: Lake Formation assigns one value per LF-tag key per resource, so a
  single `regulation` key could not mark a column as both GDPR and HIPAA. A yes/no key per
  regulation can. Lake Formation allows 50 LF-tags per resource. Validation caps
  `regulation_values` at 20, leaving room for the tenant, sensitivity, and domain tags plus
  consumer tags.
- **Regulations versus designations**: the regulation tags describe data, while
  `compliance_frameworks` provisions infrastructure. `local.regulation_designation` maps `hipaa`
  to `HIPAA` and `pci_dss` to `PCI_DSS`. A tenant declaring a mapped regulation must carry the
  matching designation. GDPR and unmapped regulations need none (FR-002, FR-029).
- **Prefixed keys**: keys carry `name_prefix`, so separate deployments in one account don't
  collide, and a secondary deployment doesn't depend on another deployment's vocabulary.
- **Domains**: domain values come only from the platform configuration, so a standalone tenant
  never extends the domain tag. Tenant `domain` defaults to the first `domain_values` entry
  (`general`).
- **Dependencies**: all tags `depends_on` the data-lake settings (owner) or the admin guard
  (secondary).

**Database tag attachments: `aws_lakeformation_resource_lf_tags.tenant_database`**

There is one attachment per `"<tenant>/<zone>"` on
`database { catalog_id = <bucket catalog>, name = <tenant> }`. It sets the tenant's most
restrictive values:
- `${name_prefix}-tenant-${tenant} = member`;
- `${name_prefix}-sensitivity = <tenant.sensitivity>`, the ceiling;
- `${name_prefix}-reg-${r} = "yes"` for each `r` in `tenant.regulations`, and `"no"` for every
  other configured regulation;
- `${name_prefix}-domain = <tenant.domain>`, which defaults to the platform's default domain.

Tables and columns inherit these values until a pipeline or steward labels them. The database is
therefore always at least as restrictive as any table in it, and an unlabeled table fails closed:
it is visible only to readers assigned the ceiling and all of the tenant's regulations. Every
table and column always resolves a value for every key, which LF-tag expressions require, because
a resource missing a key in an expression never matches.

**Table and column labels and roll-up (FR-011a): pipeline contract, not module resources**

The module never creates `aws_lakeformation_resource_lf_tags` on tables or columns. Writer
pipelines label tables and columns through `AddLFTagsToResource` when they create a table or
change its schema. The README publishes this procedure, with an AWS CLI and boto3 reference
snippet:

1. Label **every column** explicitly with `sensitivity`, every `reg-*` key, and optionally
   `domain`. Explicit column labels stop a raised table label from cascading to the columns.
2. Compute the table labels:
   - `sensitivity` is the highest-ranked column value;
   - `reg-r` is `yes` if any column is `yes`;
   - `domain` is the table's own domain, with no roll-up.
3. Apply the table labels.
4. On schema change, label the new columns and recompute the table.

Stewards follow the same procedure when they correct labels. Neither the module nor Lake
Formation enforces the roll-up, so drift is possible. Detecting drift is out of scope (spec, Out
of Scope). The `lf_tag_vocabulary` output gives pipelines the keys, allowed values, and
sensitivity order.

**Grants (FR-008 to FR-014): `aws_lakeformation_permissions`**

Every tag-based grant is an `lf_tag_policy` scoped to one table bucket's catalog
(`catalog_id = "${account}:s3tablescatalog/${bucket}"`), for each zone the persona is assigned.
All grants and tag permissions below are created by the tenant submodule. The notation in the table:
- `T` is the tenant. The expression `tenant = T` means the tenant-owned tag
  `${name_prefix}-tenant-${T} = member`.
- `S(x)` is a list of sensitivity levels.
- `D(x)` is a list of domains.
- `R(x)` is the expression term over every configured regulation: for each `r`, `reg-r ∈ ["no",
  "yes"]` when `r` is in the reader's regulations, and `reg-r ∈ ["no"]` otherwise.
- `L(T)` is the labeling scope of tenant `T`:
  - `sensitivity`: levels up to `T`'s ceiling;
  - `reg-r`: `["no", "yes"]` for each of `T`'s regulations, and `["no"]` for every other
    regulation;
  - `domain`: all values.

| Persona | Resource | Expression | Permissions | Grantable |
|---------|----------|-----------|-------------|-----------|
| Platform admins | Data-lake admins (settings) | — | Implicit full control | — |
| Stewards | `DATABASE` | `tenant = T` | `CREATE_TABLE`, `ALTER`, `DROP`, `DESCRIBE` | same |
| Stewards | `TABLE` | `tenant = T` | `SELECT`, `INSERT`, `DELETE`, `ALTER`, `DROP`, `DESCRIBE` | same |
| Stewards | LF-tags `sensitivity`, `reg-*`, `domain` | `L(T)` | `DESCRIBE`, `ASSOCIATE`, `GRANT_WITH_LF_TAG_EXPRESSION` | — |
| Stewards | LF-tag `tenant-T` | `["member"]` | `DESCRIBE`, `GRANT_WITH_LF_TAG_EXPRESSION` (no `ASSOCIATE`) | — |
| Writers | `DATABASE` | `tenant = T` | `CREATE_TABLE`, `DESCRIBE` | — |
| Writers | `TABLE` | `tenant = T` | `SELECT`, `INSERT`, `DELETE`, `ALTER`, `DESCRIBE`, plus `DROP` only when `writer_drop_enabled` (Baseline) | — |
| Writers | LF-tags `sensitivity`, `reg-*`, `domain` | `L(T)` | `DESCRIBE`, `ASSOCIATE` | — |
| Readers | `DATABASE` | `tenant = T` | `DESCRIBE` | — |
| Readers | `TABLE` | `tenant = T` AND `sensitivity ∈ S(reader)` AND `R(reader)` AND, when set, `domain ∈ D(reader)` | `SELECT`, `DESCRIBE` | — |
| Readers, row-filtered | `aws_lakeformation_data_cells_filter` on a named table, then a `data_cells_filter` grant | — | `SELECT` | — |

- **Column-level control (FR-011)**: LF-TBAC evaluates column tags. A column whose sensitivity,
  regulation, or domain falls outside a reader's expression is excluded from the reader's
  `SELECT`.
  - A reader's `S` must be a subset of the levels up to the tenant ceiling.
  - Its regulations must be a subset of the tenant's.
  - Its domains must be configured values.
- **Rolled-up tables with lower-labeled columns**: after roll-up, a table can carry a label above
  a reader's expression while some columns match it. The design expects LF-TBAC to grant the
  matching columns. T049 verifies this in the sandbox. If Lake Formation instead denies the whole
  table, the fallback is to move the rolled-up table labels to separate informational keys
  (`<prefix>-table-sensitivity`, `<prefix>-table-reg-*`) that grants never reference (see
  Complexity Tracking).
- **Tenant isolation (FR-003)**:
  - Every expression includes `tenant = T`, and only platform admins can `ASSOCIATE` the tenant
    tag. Without `ASSOCIATE`, nobody else can retag a database, table, or column into another
    tenant.
  - Stewards' grantable permissions and `GRANT_WITH_LF_TAG_EXPRESSION` cover only `tenant = T`,
    so their own grants stay inside the tenant.
  - Writers and stewards can `ASSOCIATE` sensitivity, regulation, and domain values only within
    `L(T)`, and only on resources they can `ALTER`, which are their tenant's.
- **Labeling trust**: a writer or steward can lower a column's label within `L(T)`, which widens
  what readers see. This is inherent to delegated labeling. Such changes are bounded to the
  tenant, recorded by CloudTrail (`AddLFTagsToResource`), and listed under Complexity Tracking.
- **Zone isolation (FR-014)**: grants are made per bucket catalog, only for the zones listed on
  the persona. A consumption-only reader therefore has no grant in the raw or curated catalogs.
- **Cross-tenant readers**: a principal listed as a reader by several tenants, such as the
  semantic layer, gets one grant set per tenant. Each grant set is declared by the owning tenant.
- **Row filters (FR-012)**: `tenants[T].row_filters` entries name a zone, table, reader key,
  `row_filter_expression`, and optional `column_names`. The tables must already exist, so this
  input is optional and applied after writers create them. These are the only named-resource
  grants, and the README states that.
- **Dependencies**: every grant `depends_on` the LF-tags, the tag attachments, the
  catalog-integration module or admin guard, and the namespaces.

**Persona IAM policy documents (outputs only)**

The tenant submodule outputs least-privilege JSON for each persona entry, keyed
`"<steward|writer|reader>/<key>"`. The root re-exports these for platform-applied tenants. It never attaches them, because it doesn't own the
roles. Each document contains:
- `glue:GetCatalog*`, `glue:GetDatabase*`, `glue:GetTable*`, `lakeformation:GetDataAccess`;
- `athena:StartQueryExecution`, `GetQuery*`, and `StopQueryExecution` on that tenant's workgroup
  only;
- `s3:GetObject` and `s3:PutObject` on the tenant's results location only (the shared
  `<results>/<tenant>/*` prefix or the dedicated bucket), plus `s3:GetBucketLocation`;
- KMS use, via `kms:ViaService`, on the tenant's results key: the tenant key when designated,
  otherwise the platform key.

Writers additionally get:
- `glue:CreateTable` and `glue:UpdateTable`, plus `glue:DeleteTable` only when
  `writer_drop_enabled`;
- `lakeformation:AddLFTagsToResource`, `RemoveLFTagsFromResource`, `GetResourceLFTags`,
  `GetLFTag`, and `ListLFTags`, for labeling.

Stewards additionally get:
- `lakeformation:GrantPermissions`, `RevokePermissions`, `ListPermissions`;
- `AddLFTagsToResource`, `RemoveLFTagsFromResource`, `GetResourceLFTags`, `GetLFTag`, `ListLFTags`;
- `SearchTablesByLFTags`, `SearchDatabasesByLFTags`, `CreateDataCellsFilter`, and
  `GetDataCellsFilter`.

Lake Formation itself limits these actions to the tenant scope above.

**Encryption (FR-019, FR-028)**

- Platform key: the existing `aws_kms_key.main`, which is created by default or supplied by the
  consumer. Its key policy gains statements for:
  - `maintenance.s3tables.amazonaws.com`: `kms:GenerateDataKey` and `kms:Decrypt`, with
    `StringLike kms:EncryptionContext:aws:s3:arn = arn:…:s3tables:${region}:${account}:bucket/${name_prefix}-*/*`.
    This is the pattern documented in the S3 Tables SSE-KMS permission requirements; without it
    `CreateTable` fails;
  - the Lake Formation data-access role;
  - Athena and S3 for results, via `kms:ViaService`.
- Designated tenants add `aws_kms_key.this` with an alias in the tenant submodule. Its policy has the same
  three statements, with the maintenance condition limited to that tenant's dedicated bucket ARNs
  (from the submodule's `local.dedicated_bucket_names`). It carries the `Tenant` and `ComplianceFrameworks` tags.
- Rotation enabled; deletion window taken from `environment_defaults`.
- When the consumer supplies the platform key (`encryption_config.create_kms_key = false`), the
  module can't edit its policy. It outputs `required_kms_key_policy_json` for the consumer to merge
  into their key policy. Tenant keys are always module-created.

**Transport and exposure**

- The results bucket policy denies requests where `aws:SecureTransport = false` and
  `aws:PrincipalIsAWSService = false`. AWS redacts `aws:SecureTransport` on service-to-service
  calls, so this exclusion keeps Athena's forward-access requests working.
- Table bucket policy (`aws_s3tables_table_bucket_policy`), one per shared and dedicated bucket,
  grants nothing and has three deny statements:
  - `DenyInsecureTransport`: denies `s3tables:*` under the same two conditions.
  - `EnforceKMSEncryptionAlgorithm`: denies `s3tables:CreateTable` when
    `StringNotEqualsIfExists s3tables:SSEAlgorithm` is not `aws:kms`.
  - `EnforceKMSEncryptionKey`: denies `s3tables:CreateTable` when
    `StringNotEqualsIfExists s3tables:KMSKeyArn` is not that bucket's key.

  `IfExists` matters here. Writers that create tables through Athena or Lake Formation vended
  credentials usually state no encryption, so the table inherits the bucket default key. A plain
  `StringNotEquals` would deny those requests. This bucket-default inheritance is why designated
  tenants need dedicated buckets (see Complexity Tracking).

**Compliance designation (FR-027 to FR-030)**

- `tenants[*].compliance_frameworks`: `set(string)`, default `[]`, validated against
  `["SOC2", "HIPAA", "PCI_DSS"]`.
- `local.designated_tenants` is the set of tenants with a non-empty designation.
- A `check "regulated_data_in_baseline"` block warns, naming the tenants, when
  `designated_tenants` is non-empty and the environment type is Baseline.

**Audit (FR-020)**

No trail is created. The README lists the CloudTrail configuration consumers need:
- management events for `lakeformation` (including steward `GrantPermissions` and
  `AddLFTagsToResource`), `glue`, `s3tables`, and `athena`;
- data events for `AWS::S3Tables::Table`;
- retention of at least 90 days, or at least 1 year when any tenant is designated.

**Deployer (FR-025)**

`modules/deployer` is a separately callable unit that the root never instantiates. Consumers call
it from their bootstrap or CI configuration. It creates `aws_iam_policy` and attaches it to the
consumer-supplied `iam_role_arn`, and it creates no roles. It keeps the existing statements:
- KMS read and provisioning, with aliases limited to `${name_prefix}-*`;
- CloudWatch alarms limited to `${name_prefix}*`;
- SNS topics limited to `${name_prefix}*`;
- caller-identity basics;
- `pricing:GetProducts` and related reads, included only when cost estimation is enabled
  (`cost_estimation_enabled` input, default `true`).

It is extended with least-privilege actions for:
- `s3tables`: table buckets, namespaces, policies, maintenance, and the default storage class
  (`PutTableBucketStorageClass`, `GetTableBucketStorageClass`);
- `glue`: catalog;
- `lakeformation`: resource registration, settings, permissions, LF-tags and LF-tag attachments,
  and data cells filters. All actions are listed explicitly; there are no wildcards inside action
  names;
- `athena`: workgroups;
- `s3`: the results bucket;
- `kms`;
- `iam`: creating the role, putting its policy, and `PassRole`, with `iam:PassedToService =
  lakeformation.amazonaws.com`;
- `cloudwatch`: alarms.

A `scope` variable (`"platform"` by default, or `"tenant"`) selects a narrower policy for tenant
provisioning pipelines. The tenant scope omits catalog integration, data-lake settings, IAM, and
the platform buckets, and covers only what `modules/tenant` creates: namespaces, dedicated buckets,
the tenant LF-tag and attachments, grants, the workgroup, the dedicated results bucket, keys, and
alarms. The pipeline role must also be a data-lake admin (spec, Assumptions). That is a Lake
Formation setting (FR-013), not an IAM permission, so the README says it must be granted
separately.

Resource scoping:
- Statements use `${name_prefix}`-scoped ARNs wherever the service supports resource-level
  permissions.
- These actions have no resource-level support and use `"*"`, each with an inline comment naming
  the reason:
  - Lake Formation permission and LF-tag APIs, whose authorization happens in Lake Formation;
  - `pricing:*` reads;
  - `sts:GetCallerIdentity`;
  - list and describe calls.
- The policy contains no wildcard actions.

### Environment Configuration

These keys are added to `locals.environment_defaults`. `None` takes the explicit `*_config`
values.

| Key | Ephemeral | Development | Testing | UAT | Production | MissionCritical |
|-----|-----------|-------------|---------|-----|------------|-----------------|
| `query_results_expiration_days` | 1 | 7 | 7 | 30 | 30 | 30 |
| `bytes_scanned_cutoff_per_query` | 10 GB | 10 GB | 10 GB | 1 TB | 1 TB | 1 TB |
| `unreferenced_days` / `non_current_days` | 1 / 1 | 3 / 7 | 3 / 7 | 3 / 7 | 3 / 7 | 3 / 7 |
| `force_destroy` | true | false | false | false | false | false |
| `table_storage_class` | STANDARD | STANDARD | STANDARD | INTELLIGENT_TIERING | INTELLIGENT_TIERING | INTELLIGENT_TIERING |
| `writer_drop_enabled` | true | true | true | false | false | false |
| `required_tags_enforced` | false | false | false | true | true | true |
| `scanned_bytes_alarm_threshold` (per day, per tenant) | 100 GB | 100 GB | 100 GB | 10 TB | 10 TB | 10 TB |
| `monitoring_enabled` / `alarms_enabled` | existing | existing | existing | existing | existing | existing |
| `kms_key_deletion_window_days` | existing | existing | existing | existing | existing | existing |

The former `classification_grants_enabled` key is removed. Tag-based sensitivity, regulation, and
domain grants add no cost, so they apply in every environment type (FR-029).

New inputs, with validation:

- **`zones_config`**: `{ zones = list(string) }`, default `["raw", "curated", "consumption"]`.
  - Zone names match `^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`.
  - Bucket names are at most 63 characters.
  - The list is unique and non-empty.
- **`tenants`**: `map(object({ ... }))`, default `{}`, keyed by tenant name. The object type is
  shared with `modules/tenant` `var.tenant`, and its validation lives in the submodule:
  - `owner` (string, required);
  - `sensitivity` (string, required): the tenant's ceiling;
  - `regulations` (`optional(set(string), [])`): regulations the tenant's data may fall under;
  - `domain` (`optional(string)`, default the platform's first domain value);
  - `zones` (`optional(list(string))`, default all platform zones);
  - `compliance_frameworks` (`optional(set(string), [])`);
  - `stewards` (`optional(list(string), [])`): principal ARNs;
  - `writers` (`optional(map(object({ principal_arn, zones = optional(list(string)) })), {})`);
  - `readers` (`optional(map(object({ principal_arn, zones = optional(list(string)), sensitivity_values = list(string), regulations = optional(set(string), []), domains = optional(list(string)) })), {})`);
  - `row_filters` (`optional(map(object({ reader, zone, table, row_filter_expression, column_names = optional(list(string)) })), {})`);
  - `bytes_scanned_cutoff_per_query` and `scanned_bytes_alarm_threshold` (`optional(number)`);
  - `dedicated_storage` (`optional(bool, false)`), which is forced true when designated.

  Validated:
  - tenant names match `^[a-z][a-z0-9]{1,31}$`;
  - every zone is a platform zone, and every persona zone is one of the tenant's zones;
  - every ARN matches `^arn:aws[a-z-]*:iam::[0-9]{12}:(role|user)/.+$`;
  - `sensitivity` is in `sensitivity_values`, and each reader's `sensitivity_values` are at or
    below it;
  - tenant `regulations` are configured `regulation_values`. A tenant with `hipaa` or `pci_dss`
    must list `HIPAA` or `PCI_DSS` in `compliance_frameworks`;
  - reader `regulations` are a subset of the tenant's;
  - the tenant `domain` and reader `domains` are platform domain values;
  - each row filter's `reader` and `zone` are declared;
  - frameworks are known;
  - cutoffs are at least 10 MB;
  - bucket names fit and do not collide.
- **`catalog_integration_config`**: `{ enabled = bool, data_access_role_arn = string,
  allow_full_table_external_data_access = bool }`. Validated: `data_access_role_arn` is required
  when `enabled = false`.
- **`governance_config`**: `{ admin_principal_arns = list(string), sensitivity_values =
  list(string), regulation_values = list(string), domain_values = list(string),
  writer_drop_enabled = bool, shared_storage_classification = string }`.
  - `regulation_values` defaults to `["gdpr", "hipaa", "pci_dss"]` and is limited to at most 20
    entries matching `^[a-z0-9_]{1,40}$`.
  - `domain_values` defaults to `["general"]` and must be non-empty. The first entry is the
    default tenant domain.
  - `shared_storage_classification` is nullable; null means the last (most sensitive)
    `sensitivity_values` entry.
  - `writer_drop_enabled` is nullable, so the environment default applies. Under `None` the
    explicit value is required.
- **`required_tags_config`**: `{ enforced = bool, cost_center_key = string, data_owner_key =
  string, data_classification_key = string, tenant_key = string }`.
  - Defaults: `enforced = null`, so the environment default applies; keys `CostCenter`,
    `DataOwner`, `DataClassification`, `Tenant`.
  - The module writes `data_classification_key` itself (FR-022): the platform's
    `shared_storage_classification` on shared storage, and the tenant's ceiling on dedicated
    storage.
  - Enforcement is a `lifecycle.precondition` on `terraform_data.required_tags_guard`. It checks
    that `tags` has the cost-center and data-owner keys.
- **`storage_config`**: `{ table_storage_class = string }`. Nullable, so the environment default
  applies. Validated against `["STANDARD", "INTELLIGENT_TIERING"]`.
- **`query_config`**: `{ bytes_scanned_cutoff_per_query = number, results_expiration_days =
  number }`. These are the platform defaults that tenants may override. The fixed `workgroups`
  list is removed; workgroups come from tenants.

The deployment-level `compliance_frameworks` variable from the earlier plan is replaced by the
per-tenant attribute.

**Monitoring (FR-023)**

- Workgroups set `publish_cloudwatch_metrics_enabled` to the effective `monitoring_enabled`.
- When alarms are enabled, each tenant workgroup gets two `aws_cloudwatch_metric_alarm` resources:
  - failed queries: `AWS/Athena` metric `TotalExecutionTime` sample count with
    `QueryState = FAILED`;
  - scanned bytes: `ProcessedBytes` sum over 1 day above the tenant's threshold, or the
    environment default.
- Both alarms route to the existing SNS topic and carry the `Tenant` tag.

### Complexity Level

**Selected: Baseline and Enhanced in one module, chosen at runtime by `environment_type`, plus an
orthogonal per-tenant compliance designation.**

- **Baseline** (Ephemeral, Development, Testing):
  - shared CMK;
  - tag-based tenant, sensitivity, regulation, and domain grants;
  - 10 GB scan cutoff;
  - 7-day result retention (1 day for Ephemeral);
  - writers may drop tables;
  - monitoring and alarms off;
  - idle at most $5 per month at any tenant count.
- **Enhanced** (UAT, Production, MissionCritical):
  - the same grants, but writers cannot drop tables;
  - 1 TB scan cutoff;
  - 30-day result retention;
  - monitoring and per-tenant alarms on;
  - 30-day KMS deletion window;
  - idle at most $10 per month plus about $0.20 per tenant for alarms.
- **Designated tenant**, in any environment type: a tenant KMS key (about $1 per month), dedicated
  table buckets and a dedicated results bucket ($0 idle), all encrypted with the tenant key. That is within the $2 per tenant
  ceiling.

Rationale: the infrastructure is serverless in both tiers. Only controls, retention, and
observability differ, so one code path with environment-driven defaults is simpler than separate
variants. Tenancy is data, not code paths: every tenant uses the same resources, keyed by name.

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
- Standalone tenants live in their pipelines' state. The platform's state never references them,
  so a standalone tenant apply cannot drift the platform plan. The platform must outlive its
  standalone tenants: destroying it first fails, because the shared buckets still hold tenant
  namespaces.
- Steward-made grants and table and column LF-tag assignments are outside Terraform state by
  design. The module never manages table-level resources, so it does not revert them.

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
├── variables.tf              # + zones_config, tenants, catalog_integration_config,
│                             #   governance_config, query_config, required_tags_config;
│                             #   - networktags_name
├── locals.tf                 # environment_defaults extended; zone bucket names; platform_contract
├── main.tf                   # catalog-integration and tenant submodule calls; guards
├── kms.tf                    # platform key policy extended
├── table-buckets.tf          # shared zone table buckets and their policies
├── lf-tags.tf                # platform LF-tags: sensitivity, reg-*, domain
├── query.tf                  # shared results aws_s3_bucket + controls
├── alarms.tf                 # SNS topic (existing)
├── cost-estimation.tf        # pricing submodule inputs extended (platform resources)
├── outputs.tf                # zone buckets, platform_contract, LF-tag vocabulary, key ARNs,
│                             #   tenants (re-exported submodule outputs)
├── modules/
│   ├── catalog-integration/  # NEW: LF data-access role, LF resource registration,
│   │                         #   s3tablescatalog, data-lake settings
│   ├── tenant/               # NEW: one tenant, platform-applied or standalone
│   │   ├── variables.tf      #   platform (contract), name, tenant, tags, verify_deployer_admin
│   │   ├── dependencies.tf   #   session context; data-lake settings when verifying admin
│   │   ├── locals.tf         #   zones, bucket/catalog per zone, labels, L(T), persona maps
│   │   ├── kms.tf            #   tenant key (designated)
│   │   ├── storage.tf        #   dedicated table buckets + policies (designated); namespaces
│   │   ├── lf-tags.tf        #   tenant LF-tag; database tag attachments
│   │   ├── permissions.tf    #   LF-tag policy grants; tag permissions; data cells filters
│   │   ├── query.tf          #   workgroup; dedicated results bucket (designated)
│   │   ├── iam-documents.tf  #   persona policy documents
│   │   ├── alarms.tf         #   workgroup alarms
│   │   ├── main.tf           #   guards; FR-030 check; cost estimate
│   │   ├── outputs.tf
│   │   └── README.md         #   both entry points, contract, state move
│   ├── cost-estimation/      # extended: S3 Tables, Athena, S3 results, KMS keys; used by both
│   └── deployer/             # extended: least-privilege deploy/destroy policy, with a
│                             #   `scope = "platform" | "tenant"` variant for tenant pipelines
├── examples/
│   ├── defaults/             # name_prefix, admin principal, one minimal tenant
│   ├── complete/             # three tenants: one HIPAA-designated, personas, row filter
│   ├── secondary/            # NEW: catalog_integration_config.enabled = false + role ARN input
│   ├── standalone-tenant/    # NEW: modules/tenant called directly with a platform contract
│   └── deployer/             # NEW: modules/deployer rendered in platform and tenant scope
├── test/
│   ├── main_test.go          # existing helpers (base options, name prefix, retryable errors)
│   ├── defaults_test.go      # shared zone buckets, tenant namespaces/tags/workgroup present
│   ├── complete_test.go      # tag-policy grants; per-tenant designation; no writer DROP in
│   │                         #   Enhanced; results isolation; existing TestEnabledFalse
│   ├── tenancy_test.go       # NEW: adding a tenant adds only module.tenant["x"]; zone override
│   ├── standalone_test.go    # NEW: standalone plan matches the platform-applied resource set
│   ├── deployer_test.go      # NEW: no wildcard actions; tenant scope excludes platform actions
│   ├── secondary_test.go     # NEW: no catalog/settings resources when flag disabled
│   └── validation_test.go    # NEW: invalid inputs fail at plan; FR-030 warning
├── .github/workflows/
│   └── terraform-ci.yml      # switched to make targets; installs TFLint and terraform-docs
├── Makefile                  # + test-tenancy, test-standalone, test-deployer, test-secondary,
│                             #   test-validation,
│                             #   security-scan
└── README.md                 # tenancy, LF-tag vocabulary and labeling/roll-up contract, stewards,
                              #   audit prerequisites, import
```

**Structure Decision**: The module keeps the repository-root layout that the template and
`AGENTS.md` define, with files split by concern. It is not placed in `iac/`: this repository is
itself a Terraform module, and the Registry layout requires root `.tf` files.

- Account-level singleton resources are extracted into `modules/catalog-integration`. That gives
  them one reason to change, makes ownership explicit, and lets a platform team call them
  standalone.
- Platform-owned LF-tags move to the root module because they are deployment-scoped (prefixed
  keys).
- Tenants are a submodule (FR-001a). The submodule boundary is real: tenant pipelines own tenant
  state, and the platform contract is the only coupling, which is explicit and typed. One
  implementation serves both entry points, so they cannot diverge.

## Complexity Tracking

These are spec adjustments that research forced, not principle violations.

| Spec item | Adjustment | Reason |
|-----------|------------|--------|
| FR-005 (compaction and snapshot expiration) | The module configures unreferenced-file removal per bucket; compaction and snapshot management use the S3 Tables per-table defaults, and the README tells writers to keep them. | These settings exist only on `aws_s3tables_table`, and the module creates no tables. The defaults satisfy every RPO and the 30-day erasure target. |
| FR-028 (designated-tenant storage) | Designated tenants get dedicated table buckets per zone instead of a namespace in the shared bucket. | S3 Tables encryption defaults are per bucket. Tables created through Athena or vended credentials inherit the bucket key, so a per-namespace key in a shared bucket cannot be enforced. Dedicated buckets have no idle cost. S3 Tables limits table buckets per Region. The default is 100 since 2026-10, up from 10, which allows about 32 dedicated tenants with three zones; the README lists the quota. |
| Scalability (tables per zone) | Shared buckets cap at the S3 Tables per-bucket table quota (10,000 by default, adjustable) across all tenants sharing a zone. Tenants that need more set `dedicated_storage = true`. | One bucket per zone is shared, and the quota is per bucket. Reusing the dedicated-bucket path adds no new mechanism. |
| FR-022 (shared-storage classification) | Shared buckets and the shared results bucket carry a fixed platform classification, not one computed from tenants. Non-dedicated tenants must fit under it. | A tag computed from tenants would change platform resources on tenant changes (FR-001) and could not see standalone tenants. |
| FR-001a (pipeline authority) | Tenant provisioning pipelines are platform-operated and run as data-lake admins. A tenant-operated, least-privilege path, using LF-tag creator rights plus scoped grantable permissions instead of admin, is deferred to a later spec. | Creating LF-tags and grants for a new tenant needs authority no tenant-scoped identity holds yet. Keeping pipelines platform-operated means no tenant-held identity exceeds its tenant scope (principles, Govern Data Access by Default). |
| FR-001a (standalone tenants) | Each tenant owns its own LF-tag key `${prefix}-tenant-<tenant>` (value `member`) instead of a value on a shared tenant key. Domain values are platform-only. Designated tenants own a dedicated results bucket instead of a statement in the shared bucket policy. | A standalone tenant has separate state and must never modify a platform-owned resource. A shared tag's value list and a shared bucket policy are single resources. The cost is one LF-tag key per tenant, bounded by the account LF-tag quota. |
| FR-001a (name collisions) | There is no registry across entry points. A duplicate tenant name fails at apply, on the existing namespace or LF-tag key. | A registry would need shared mutable state, which is what the standalone entry point exists to avoid. |
| FR-009 / FR-012 (tag-based grants, row filters) | Row filters stay named data cells filter grants and need existing tables. They are optional inputs applied after writers create the tables. | Data cells filters cannot be granted through LF-tag expressions. |
| FR-009 (tag grants on federated catalogs) | `lf_tag_policy` grants are made once per bucket catalog in the persona's zones. T049 verifies sub-catalog scoping in the sandbox before Phase 4. | LF-tag policies carry a `catalog_id`. S3 Tables databases live in per-bucket sub-catalogs of `s3tablescatalog`. |
| FR-011 (rolled-up table with lower-labeled columns) | The design relies on LF-TBAC granting the matching columns of a table whose rolled-up label is above the reader's expression. T049 verifies this. If it fails, the rolled-up labels move to informational `table-*` keys that grants never reference, and column labels alone drive access. | LF-tags inherit downward only. The roll-up adds a table label that column-level evaluation must not escalate. |
| FR-011a (roll-up not enforced) | Roll-up is a pipeline contract, documented with a reference procedure. The module does not detect drift. | Clarified 2026-10-14: pipeline responsibility. The module has no compute, and LF-tags do not aggregate upward. |
| FR-010 (delegated labeling can lower labels) | Writers and stewards may lower a column's labels within `L(T)`, which widens reader visibility inside the tenant. | Delegated labeling is the design. Changes are tenant-bounded and recorded in CloudTrail. |
| FR-010 (stewards grant to principals not in inputs) | Steward grants are outside module state and may name any principal. They are bounded to `tenant = T` and recorded by CloudTrail. | This is delegation by design (clarified 2026-10-14). Module-managed grants still go only to input principals. |
| FR-013 (admins set exactly to the input list) | A plan precondition requires the deploying role to be in the admin list. Secondary deployments check the same thing against the live settings. | Without that, Terraform could remove its own admin rights and fail every later grant. |
| FR-007 (non-owner deployments) | Non-owner deployments must pass the integration's data-access role ARN. | That role must be granted use of every table bucket's KMS key, and the principles forbid name-based lookups. |
| Environment Configuration (`None`) | Under `environment_type = "None"`, `unreferenced_days`, `non_current_days`, and `force_destroy` take the Development values (3, 7, false). | No `*_config` input carries them, and adding one for three rarely tuned values is not worth the surface. Found during T009. |
| T010 (`data_access_role_arn` local) | The local is added with the catalog-integration wiring in T021, not in T010. | It references `module.catalog_integration`, which does not exist until T021, so adding it earlier fails the T012 `make check` checkpoint. |
| FR-005 (default table storage class) | The shared and dedicated table buckets do not set a default storage class for new tables. `table_storage_class` is still computed and published in the platform contract. | AWS provider 6.69 has no `aws_s3tables_table_bucket` argument for it, so T026 had no confirmed argument to use. Tracked as a follow-up bead. |
| T048 (`module.tenant` dependencies) | `module "tenant"` has no module-level `depends_on`. The platform contract references the platform LF-tags, which depend on the catalog integration and the admin guard, so tenant resources still wait for them. | A module-level `depends_on` defers every data source in the module to apply, so the tenant cost estimate and the planned `monthly_cost_estimate` became unknown at plan time. Found during Phase 6. |
| FR-025 (tenant deployer scope has no IAM actions) | Both deployer scopes include `iam:GetRole` on `role/*`. | `aws_iam_session_context`, which resolves the deploying role for the admin checks, calls `iam:GetRole`. The tenant scope has no other IAM action. |
| T061 (persona policy scoping) | Persona policy JSON scoping is checked in the T076 sandbox run, not in plan-only tests. | The documents reference the workgroup and results bucket ARNs, which are unknown until apply. Building ARNs from names to know them earlier is forbidden by the principles. |
| T049 / T076 (sandbox account) | Before applying the examples in account 566376086856, add its existing data-lake admins (two SageMaker service roles) to `admin_principal_arns`. | The data-lake settings resource is authoritative and would remove them, which can break the SageMaker domain that uses them. Found during Phase 6. |
| T049 (sandbox, account 492637581429, 2026-10-14) | **Failed item 1, which needs a plan revision.** `lf_tag_policy` grants with `catalog_id = <account>:s3tablescatalog/<bucket>` are rejected (`CatalogId of the resource must a valid AWS account id`). A probe showed that an account-level `lf_tag_policy` grant (`catalog_id = <account>`) applies and reaches tenant databases in every bucket catalog, and still isolates tenants: the sales steward saw only `sales`. Zone isolation (FR-014) therefore cannot come from catalog scoping. **Passed:** catalog integration (after fixes), namespaces, database LF-tag attachments in bucket catalogs, `lf_tag` permissions including `GRANT_WITH_LF_TAG_EXPRESSION` (item 2). **Not run:** items 3 and 4. | The proposed revision is a platform-owned `${prefix}-zone` LF-tag on each tenant database (value = zone, ASSOCIATE for admins only), plus `zone ∈ persona.zones` in every grant expression, with one account-level grant per persona instead of one per zone. |
| FR-007 (catalog integration ordering and defaults) | The data-lake settings are created before the registration and the catalog. The settings have no `create_*_default_permissions` blocks. | Found in the T049 sandbox: `CreateCatalog` needs the deployer to be a data-lake admin first, and an empty default-permission block sends an empty principal, which the API rejects. Omitting the blocks cleared the `IAM_ALLOWED_PRINCIPALS` defaults (verified with `get-data-lake-settings`). |
