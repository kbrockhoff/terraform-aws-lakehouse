# Architecture Plan: Hardened Catalog and Data Permission Account Settings

**Branch**: `003-harden-catalog-settings` | **Date**: 2026-11-18 | **Spec**: [spec.md](./spec.md)
**Input**: Infrastructure specification from `/specs/003-harden-catalog-settings/spec.md`

**Note**: This template is filled in by the `/iac.plan` command. For deep research, module specs, and quickstart guide, run `/iac.enrichplan` after planning.

## Summary

This plan extends the existing `terraform-aws-lakehouse` module (spec 001) in place. It does not
create a new stack.

Today the `catalog_integration_config.enabled` flag controls every account- and Region-wide
setting. This plan moves those settings into a new `modules/account-settings` submodule. A new
owner flag, `account_settings_config.owner`, controls that submodule independently of the catalog
integration. The submodule owns:

- **Lake Formation data-lake settings**: moved out of `modules/catalog-integration` with `moved`
  blocks. Every field is now explicit. The `parameters` map pins
  `CROSS_ACCOUNT_VERSION = "4"` and keeps `SET_CONTEXT`.
- **Dedicated catalog-metadata KMS key** (`aws_kms_key.catalog`, alias `<prefix>-catalog-cmk`):
  separate from the platform key `aws_kms_key.main` and from the tenant keys, which encrypt the
  table buckets and query results. The submodule creates it unless the consumer supplies
  `catalog_encryption.kms_key_arn`.
- **Glue Data Catalog encryption settings**: catalog metadata and connection passwords are
  encrypted with the dedicated catalog key.
  - The default mode is `SSE-KMS-WITH-SERVICE-ROLE`, using a new catalog-encryption IAM role.
  - `SSE-KMS` (per-caller) mode is selectable.
- **Optional Glue resource policy**: off by default. When enabled, it is limited to
  `glue:ShareResource` for `ram.amazonaws.com` plus deny statements the consumer supplies.

Other changes:

- **SageMaker Unified Studio support**: a `studio_config.enabled` flag adds the studio's
  provisioning and domain roles to the authoritative data-lake admins.
  - A deployment that isn't the owner verifies that these roles are already admins.
  - The Lake Formation settings that SageMaker Unified Studio features need stay explicit
    opt-ins: full-table external access for IAM-based domains, and external data filtering with
    the `Amazon EMR` session tag for EMR fine-grained access.
- **Bypass prevention**: a new statement in every table-bucket policy (shared zone buckets and
  dedicated tenant buckets) denies direct S3 Tables data-plane and table-mutation calls. The
  exemptions are:
  - the Lake Formation data-access role;
  - AWS service principals: S3 Tables maintenance and S3 metadata;
  - an explicit list of exempt principals.
- **Same-Region cross-account sharing**:
  - The root holds an approved list of consumer accounts.
  - Each tenant declares `shares`, which become read-only LF-tag expression grants to the
    consumer account ID. Each grant is always bounded by the tenant tag and carries the grant
    option for the same permissions.
  - Lake Formation creates the RAM shares itself on cross-account version 4.
  - Each share also grants the consumer account `DESCRIBE` (with grant option) on the LF-tag keys
    and values in its expression, so the consumer administrator can re-grant by tag.
  - No key policy names a consumer account. Shared `s3tablescatalog` metadata isn't covered by
    Glue catalog encryption (sandbox result 5).
- **Key protection**: a deny statement in the catalog key's policy stops that key from
  being scheduled for deletion or disabled in every environment type, except by break-glass
  principals. Changing the catalog key after installation fails at plan time outside Ephemeral
  environments. Ephemeral owners are warned to use an external, long-lived catalog key, and the
  verify fixture does this.
- **Verification**: `lfverify` gains cases for the bypass deny, the settings, and the encryption
  state, plus a second-account consumer persona for the sharing cases.

## Technical Context

**Cloud Provider**: AWS. One account and Region per deployment. Consumer accounts for sharing
are in the same Region.
**IaC Tool**: Terraform `>= 1.11`. The local tool is 1.16.5. The plan uses `moved` blocks, which
work across module boundaries. Consumers handing over ownership use `removed` blocks with
`lifecycle { destroy = false }`, which need Terraform 1.7 or later and are already within the floor.
**Provider Versions**: `hashicorp/aws >= 6.59.0`, with no upper bound (awslkhs-jakp). The floor is unchanged (T003 confirmed
6.59.0 has every attribute). `hashicorp/time >= 0.12` is new: the catalog-encryption role needs a
propagation delay before Glue accepts it (sandbox result 2).
The latest release, 6.73.0, was checked with `terraform providers schema` and exposes every
attribute this plan needs:

- `aws_glue_data_catalog_encryption_settings.encryption_at_rest.catalog_encryption_service_role`;
- `aws_lakeformation_data_lake_settings`: `parameters`,
  `allow_full_table_external_data_access`, `allow_external_data_filtering`,
  `external_data_filtering_allow_list`, `authorized_session_tag_value_list` and
  `trusted_resource_owners`;
- `aws_glue_resource_policy.enable_hybrid`;
- `aws_lakeformation_permissions` with `permissions_with_grant_option` and `lf_tag_policy`;
- data sources `aws_glue_data_catalog_encryption_settings` and
  `aws_lakeformation_data_lake_settings`, the latter including `parameters`.

There is **no** data source for the Glue resource policy (see Spec Deviations, D1).

Sandbox task T-S1 must confirm whether the floor needs to rise. It checks the oldest provider
version that has `catalog_encryption_service_role` and `parameters` on the data-lake settings.
**Module Versions**: No external modules. New internal submodule: `./modules/account-settings`.
**Curated Modules**: None apply. No curated module covers Lake Formation settings, Glue catalog
encryption, or S3 Tables bucket policies. Native resources keep the existing pattern.
**State Backend**: Consumer-owned and unchanged. The module declares no backend.
**Environment Strategy**: Unchanged. `environment_type` drives the effective defaults. Every
control in this plan applies to all environment types, because none of them costs extra. The
only tier-dependent behaviors are Ephemeral-only: the catalog key may be changed, and a warning
appears when an Ephemeral owner encrypts the catalog with a key it created itself.
**Testing**:

- **Terratest, plan-only**, under `test/`:
  - new validation, flag-combination, and secondary-deployment cases;
  - complete-example assertions for the bucket deny, key-policy statements, and share grants.
- **`lfverify` unit tests**: `make test-verify`.
- **`lfverify` full-mode sandbox runs**: `make verify-full`. They need a second sandbox account
  for the consumer cases.
**Security Scanning**: Checkov (`make security-scan`), TFLint, and gosec for `verify/`. All are
unchanged.
**Cost Estimation**: The existing `modules/cost-estimation` counts one more KMS key (about USD 1
per month) when the owner deployment creates the catalog key, and gains a usage line for catalog
KMS requests.
**Target Environments**: All `environment_type` values. Sandbox account `492637581429`
(us-east-1) is the producer. A second sandbox account in the same organization is the consumer
and needs to be added to `verify/sandbox-accounts.txt`.
**Compliance**: SOC 2, HIPAA, PCI DSS and GDPR, as in 001.

### Research Findings That Shape the Design

The findings come from AWS documentation reviewed on 2026-11-18. Items marked *sandbox* are not
documented and are confirmed by T-S1.

- **R1**
  - *Finding*: v4 is the minimum for sharing federated-catalog objects (S3 Tables) and hybrid-mode
    resources across accounts. v5 is documented as irreversible. Reversibility of v1–v4 is
    undocumented, and the new-account default is undocumented.
  - *Design consequence*: Pin `"4"`. A precondition stops the plan when the live version is higher
    than the configured one, so the module never attempts a downgrade.
- **R2**
  - *Finding*: `PutDataLakeSettings` replaces the whole object. `Parameters` carries both
    `CROSS_ACCOUNT_VERSION` and `SET_CONTEXT`, which defaults to true and needs `sts:SetContext` in
    the registered role's trust; the current role has it.
  - *Design consequence*: `parameters = { CROSS_ACCOUNT_VERSION = "4", SET_CONTEXT = "TRUE" }`. T-S1
    reads the live casing first.
- **R3**
  - *Finding*: On v3+ a Glue resource policy is **not** needed for LF-tag cross-account grants. If a
    policy exists, it must allow `glue:ShareResource` for `ram.amazonaws.com` and be put with
    `EnableHybrid = TRUE`.
  - *Design consequence*: No policy by default. An optional policy always includes that statement
    and sets `enable_hybrid = "TRUE"`.
- **R4**
  - *Finding*: Glue encryption is not retroactive, and disabling it leaves existing objects
    encrypted. Enabling the service role makes objects encrypted with `aws/glue` **inaccessible**.
    The encryption context key is `glue_catalog_id`.
  - *Design consequence*: A precondition on the live encryption state blocks a migration from
    `aws/glue`. The key must outlive the catalog (FR-013). The key-policy conditions use
    `kms:EncryptionContext:glue_catalog_id`.
- **R5**
  - *Finding*: Accounts that receive shared catalog resources need permissions on the catalog
    encryption key, and `aws/glue` cannot be shared. It is undocumented whether the service role
    removes this need.
  - *Design consequence*: **Superseded by sandbox result 5.** Glue catalog encryption doesn't cover
    `s3tablescatalog` metadata, and a consumer read shared tables created under service-role
    encryption with no key grant. No key policy names a consumer account. (D3 resolved.)
- **R6**
  - *Finding*: Table data is read through credentials Lake Formation vends from the registered role.
    Consumers need no table-key access.
  - *Design consequence*: Table-bucket and tenant keys stay unchanged for consumers.
- **R7**
  - *Finding*: Vended S3 requests carry the registered role's assumed-role session. AWS warns not to
    restrict `maintenance.s3tables.amazonaws.com` or `metadata.s3.amazonaws.com`.
  - *Design consequence*: The bypass deny exempts `aws:PrincipalArn` = data-access role, with
    `aws:PrincipalIsAWSService = false`. *Sandbox*: governed `CREATE TABLE` and `INSERT` from Athena
    still succeed.
- **R8**
  - *Finding*: SageMaker Unified Studio requires its blueprint provisioning role
    (`AmazonSageMakerProvisioning-<account>`) to be a data-lake admin, and recommends the domain
    execution role for IAM-based domains. IAM-based domains need account-level
    `AllowFullTableExternalDataAccess`. EMR fine-grained access needs `AllowExternalDataFiltering`
    plus the session tag `Amazon EMR`. Sandbox result 5: a deployed domain makes both
    `AmazonSageMakerProvisioning-<account>` and `AmazonSageMakerManageAccess-<region>-<domain>`
    data-lake admins.
  - *Design consequence*: `studio_config.integration_principal_arns` holds the admin roles. The Lake
    Formation settings those features need stay explicit opt-ins in
    `account_settings_config.data_lake`, each documented (FR-004).
- **R9**
  - *Finding*: LF-tags are per account and assignable to `s3tablescatalog` objects.
  - *Design consequence*: The existing taxonomy is reused, and share grants use
    `lf_tag_policy.catalog_id = account_id`, as reader grants already do.
- **R10**
  - *Finding*: Registered-role trust is documented only with `aws:SourceAccount`. No `aws:SourceArn`
    or Region condition is documented. The Glue service-role example has no condition.
  - *Design consequence*: The registration-role trust is unchanged (Spec Deviation D2). The
    catalog-encryption role gets `aws:SourceAccount` **only**. Sandbox result 3: adding
    `aws:SourceArn` = the catalog ARN breaks all catalog access, because Glue sends no matching
    source ARN.
- **R11**
  - *Finding*: Cross-account grants to an account need the grant option so the consumer admin can
    re-grant. The consumer can only narrow access. Drop or Super on a database can't be granted
    cross-account. Revoke grants before deleting resources, or RAM shares are orphaned.
  - *Design consequence*: Shares grant `DESCRIBE` / `SELECT` with the grant option for the same
    permissions. Share grants depend on the database LF-tag assignment, so a destroy revokes them
    first.
- **R12**
  - *Finding*: The grantor needs `AWSLakeFormationCrossAccountManager`-equivalent RAM permissions,
    including `ram:AssociateResourceSharePermission` and `ram:ListResourceSharePermissions`.
  - *Design consequence*: `modules/deployer` gains a `LakeFormationCrossAccount` statement scoped to
    `ram:ResourceShareName = LakeFormation*`.

## Principles Check

Principles are configured (`.specify/memory/principles.md` v1.3.0).

- **Govern Data Access by Default**
  - *Gate*: Deny unless granted. One permission model. Tenant isolation. Label-based grants.
  - *How the plan satisfies it*: The bypass deny makes Lake Formation the only data path. Shares are
    LF-tag expression grants that always include the tenant term, so a share can't cross tenants.
    Row filters are not shared. No `IAM_ALLOWED_PRINCIPALS` defaults.
- **Embed Compliance into the Architecture**
  - *Gate*: Audited, region-resident, designation-driven.
  - *How the plan satisfies it*: Catalog metadata is encrypted with a customer-managed key in every
    tier. Shares stay in the Region. Regulated labels are excluded unless the share sets
    `compliance_acknowledged`. The audit trail is reused (001 FR-020).
- **Optimize for Cost Efficiency**
  - *Gate*: No idle cost; costs estimated.
  - *How the plan satisfies it*: Adds one key per account and Region (owner only, about USD 1 per
    month), created for a security boundary rather than a compliance-only control. The estimate
    covers the key and the KMS requests.
- **Protect Data Durability and Recoverability**
  - *Gate*: Destructive changes reversible.
  - *How the plan satisfies it*: Key-policy deny on `ScheduleKeyDeletion` and `DisableKey` (FR-013).
    Hand-over keeps the live settings (`removed` with `destroy = false`).
- **Establish Resource Identity and Accountability**
  - *Gate*: Common tags; data tags only on data-at-rest.
  - *How the plan satisfies it*: The new role gets `common_tags` only, not data tags (bd memory
    `data-tags-data-at-rest-only`).
- **Define Small, Validated Module Contracts**
  - *Gate*: `*_config` objects, validation, focused submodules.
  - *How the plan satisfies it*: New `account_settings_config`, `studio_config` and
    `sharing_config`, a `shares` attribute in `tenants`, and a new focused submodule
    `modules/account-settings`.
- **Secure by Construction**
  - *Gate*: `aws_iam_policy_document` only; no wildcard principals; confused-deputy conditions;
    static analysis.
  - *How the plan satisfies it*: All new policies are documents. The only `*` principals are on Deny
    statements, matching the existing bucket policies. `aws:SourceAccount` is set on the new service
    trust. Consumer-supplied resource-policy statements must be Deny-only (validated).
- **Validate Before Merge**
  - *Gate*: `make` targets.
  - *How the plan satisfies it*: New Terratest cases run under `make test`, and `lfverify` cases
    under `make test-verify` and `make verify-full`.
- **Compose, Don't Accumulate**
  - *Gate*: No studio domain or portals.
  - *How the plan satisfies it*: The module never creates SageMaker Unified Studio or DataZone
    resources, or anything in consumer accounts.

**Gate result**: PASS. One justified complexity item is recorded below.

## Infrastructure Architecture

### Compute Resources

No compute. Everything is control-plane configuration.

### Data Storage

- **Table buckets**: the shared zone buckets (`table-buckets.tf`) and dedicated tenant buckets
  (`modules/tenant/storage.tf`). Their existing bucket policies gain one statement,
  `DenyDirectTableAccess`:
  - **Effect and principal**: `Deny` for principal `*`.
  - **Actions**: `s3tables:GetTableData`, `s3tables:PutTableData`, `s3tables:CreateTable`,
    `s3tables:DeleteTable`, `s3tables:RenameTable`, `s3tables:UpdateTableMetadataLocation`,
    `s3tables:GetTableMetadataLocation` and `s3tables:PutTableMaintenanceConfiguration`.
  - **Resources**: `<bucket>` and `<bucket>/*`.
  - **Conditions**:
    - `ArnNotEquals aws:PrincipalArn` = [data-access role,
      `governance_config.direct_access_exempt_principal_arns`…];
    - `Bool aws:PrincipalIsAWSService = false`, which leaves S3 Tables maintenance and S3 metadata
      unrestricted.
  - **Not denied**: namespace and bucket management (`CreateNamespace`, `DeleteNamespace`,
    bucket policy, encryption and maintenance), so the deployer keeps them (FR-016).
  - **Table maintenance settings**: if the module manages per-table maintenance, T-S1 checks
    whether `PutTableMaintenanceConfiguration` must also be exempted for the deployer. Today the
    module sets maintenance only at bucket level, so it is likely not needed.
  - **Contract**: the exempt list and the data-access role are already in, or are added to,
    `platform_contract`, so standalone tenants apply the same deny.
- **Catalog metadata**: Glue Data Catalog encryption at rest, using the dedicated catalog key.
  - By default `modules/account-settings` creates `aws_kms_key.catalog`. The platform key
    `aws_kms_key.main` keeps encrypting the table buckets and query results only, and its policy is
    unchanged by this plan.
  - `account_settings_config.catalog_encryption.kms_key_arn` supplies an external catalog key
    instead (used by Ephemeral fixtures). It gets its required statements through a new
    `required_catalog_kms_key_policy_json` output.
  - A precondition rejects a supplied key equal to the platform key (`local.kms_key_arn`) or to
    any tenant key known to the root.
  - Connection passwords use the catalog key with `return_connection_password_encrypted = true`.

### Networking

No changes. Network foundations remain out of scope in 001.

### Security

**New submodule `modules/account-settings`** (count = `var.enabled && local.settings_owner`):

1. `aws_lakeformation_data_lake_settings.this`. Moved here from `modules/catalog-integration` by a
   root `moved` block.
   - **Admins and read-only admins**: `admins` = `governance_config.admin_principal_arns` plus
     `studio_config.integration_principal_arns` when the studio is enabled. `read_only_admins`
     is unchanged.
   - **Default permissions**: no `create_*_default_permissions` blocks, as today.
   - **External engine settings**: `allow_external_data_filtering`,
     `external_data_filtering_allow_list`, `authorized_session_tag_value_list`,
     `allow_full_table_external_data_access` and `trusted_resource_owners`, all from
     `account_settings_config.data_lake`. Defaults are false or empty.
   - **Parameters**: `{ CROSS_ACCOUNT_VERSION = tostring(cross_account_version), SET_CONTEXT =
     "TRUE" }`.
   - **Preconditions**:
     - the deployer is in the admins (existing check);
     - the live `CROSS_ACCOUNT_VERSION` is not above the configured value. It is read through the
       data source, which needs the deployer to already be an admin; on a first apply this check
       is skipped when the data source returns no admins.
2. `aws_iam_role.catalog_encryption` (service-role mode only):
   - **Trust**: `glue.amazonaws.com`, `sts:AssumeRole`, `aws:SourceAccount` = account only
     (sandbox result 3).
   - **Inline policy**: `kms:Decrypt`, `kms:Encrypt` and `kms:GenerateDataKey` on the configured
     key ARN only.
   - **Optional**: `permissions_boundary`.
   - **Tags**: common tags only.
3. `time_sleep.catalog_encryption_role` (service-role mode only): `create_duration = "20s"`,
   triggered by the role ARN and the role policy. Sandbox result 2 showed Glue rejects a new role
   as "not assumable" for about 10–15 seconds.
4. `aws_glue_data_catalog_encryption_settings.this`, which depends on item 3:
   - **Mode**: `SSE-KMS-WITH-SERVICE-ROLE` with the role from item 2, or `SSE-KMS`.
   - **Connection passwords**: encrypted with the same key.
   - **Precondition** (read through the `aws_glue_data_catalog_encryption_settings` data source):
     fail if the live mode is `SSE-KMS` with the AWS-managed `aws/glue` key and the target is
     service-role mode (R4). The message points to the documented re-encryption procedure.
   - **Precondition (FR-013)**: outside Ephemeral environments, fail when the live
     `SseAwsKmsKeyId` is set and differs from the configured key. The catalog key is fixed at
     installation; no prior installations exist, so key migration is unsupported.
   - **`check` block (FR-013)**: warn when an Ephemeral owner uses the module-created catalog key.
     Teardown can't delete that key, so the warning recommends
     `catalog_encryption.kms_key_arn` pointing at a long-lived key.
5. `aws_glue_resource_policy.this`. Created only when
   `account_settings_config.catalog_resource_policy.enabled`.
   - The document is built with `aws_iam_policy_document`. It has a `RamShareResource` statement
     for `glue:ShareResource` to `ram.amazonaws.com` on `catalog`, `database/*` and `table/*/*`.
   - Consumer-supplied `deny_policy_jsons` are merged through `override_policy_documents` and
     validated to contain only `Deny` effects.
   - `enable_hybrid = "TRUE"`.

**Catalog key in `modules/account-settings/kms.tf`**:

- `aws_kms_key.catalog`, with `count = var.catalog_encryption.kms_key_arn == "" ? 1 : 0`:
  - symmetric, `enable_key_rotation = true`;
  - `deletion_window_in_days` from the root's effective deletion window;
  - description `"Catalog metadata key for ${name_prefix}"`;
  - common tags only, because a key holds no data at rest.
- `aws_kms_alias.catalog`: `alias/<prefix>-catalog-cmk`, within the deployer's existing
  `alias/<prefix>-*` scope.
- `local.catalog_key_arn`: the created key, or the supplied ARN. It feeds the encryption settings
  and the role policy.
- The key policy is built in the submodule. It doesn't create a cycle: the policy references the
  role, the role's inline policy references the key, and the role itself depends on neither.
  Statements:
  - `EnableRootAccess`: `kms:*` to the account root with `aws:PrincipalAccount` set to the
    account, matching root `kms.tf`. This allows IAM delegation and prevents lockout.
  - `AllowGlueCatalogEncryption`: grants `kms:CreateGrant` to the account root, conditioned on
    `kms:CallerAccount`, `kms:GrantIsForAWSResource = true` and
    `kms:EncryptionContext:glue_catalog_id = <account>`.
  - `AllowCatalogEncryptionRole`, in service-role mode: grants `Encrypt`, `Decrypt`,
    `GenerateDataKey` and `DescribeKey` to the catalog-encryption role.
  - `AllowCatalogKeyUsers`, in per-caller mode: grants `Encrypt`, `Decrypt`, `GenerateDataKey`,
    `DescribeKey` and `CreateGrant`, conditioned on `glue_catalog_id`. The principals are:
    - admins and read-only admins;
    - studio principals;
    - the reader, writer, steward and metadata persona ARNs from `var.tenants`, passed in as
      `key_user_arns`;
    - `additional_key_user_arns`.

    Validation caps the total at 200 to stay under the 32 KB key-policy limit.
  - `ProtectCatalogKey`: `Deny` `kms:ScheduleKeyDeletion` and `kms:DisableKey` to `*` unless
    `aws:PrincipalArn` is in `catalog_encryption.key_deletion_principal_arns`. This applies in
    every environment type (FR-013). The platform key keeps its normal deletion window.
    - Removing the protection requires changing the input, which is a reviewed change.
    - `PutKeyPolicy` stays allowed so recovery is possible.
- The required statements other than `EnableRootAccess` are also rendered as
  `data.aws_iam_policy_document.required_catalog_key_statements`. They are exposed through the
  root output `required_catalog_kms_key_policy_json` for consumer-supplied catalog keys.

**Root `kms.tf`**: unchanged. The platform key policy gains no catalog, consumer, or
key-protection statements (FR-009, FR-026).

**`modules/catalog-integration`**:

- Loses the data-lake settings resource, through the `moved` block.
- Gains a `settings_ready` input so registration still waits for admin rights. It receives the
  account-settings submodule's output, or the guard's ID when this deployment doesn't own the
  settings.
- Gains an optional `permissions_boundary_arn` for the data-access role.
- The trust policy is unchanged (D2).

**Root guards** (`main.tf`): when this deployment doesn't own the account settings, it checks:

1. the deployer is an admin (existing check);
2. the live encryption mode is not `DISABLED`;
3. the live `CROSS_ACCOUNT_VERSION` is at least 4, checked only when any tenant has `shares`;
4. every studio principal is in the live admins, when the studio is enabled.

The existing `secondary_admin_guard` becomes `settings_guard`, with a `moved` block.

**Cross-account sharing**:

- Root `sharing_config.consumer_accounts`, a map keyed by account ID:
  `{ purpose = string, outside_organization = optional(bool, false) }`.
  - Validation: 12 digits; not the current account (checked by a precondition, because
    validation can't reference data sources); at most 50 entries.
  - The map is added to `platform_contract.sharing`. The contract carries no protocol version,
    because a value copied at platform-apply time could be stale.
- `tenants[*].shares`: `map(object({ account_id, zones = optional(list, ["consumption"]),
  sensitivity_values = list(string), regulations = optional(set, []), domains = optional(list),
  compliance_acknowledged = optional(bool, false) }))`.
  - The account must be in the contract's approved list. This is a tenant precondition, so the
    standalone entry point is covered too.
  - Designated tenants need `compliance_acknowledged`.
  - When `shares` is non-empty, the tenant module reads the live data-lake settings and fails
    unless `CROSS_ACCOUNT_VERSION` is at least 4. This covers platform-applied and standalone
    tenants, against owner and non-owner platforms alike.
- `modules/tenant/sharing.tf` creates two grants per share:
  - `aws_lakeformation_permissions.share_database`: principal = account ID,
    `permissions = ["DESCRIBE"]`, `permissions_with_grant_option = ["DESCRIBE"]`, and an LF-tag
    expression of tenant tag + zones.
  - `aws_lakeformation_permissions.share_table`: `["SELECT", "DESCRIBE"]` with the grant option
    for the same permissions, and an expression of tenant + zone + sensitivity + regulation +
    domain. It reuses `local.reader_expressions`, generalized to take the share entry.
  - Both depend on `aws_lakeformation_resource_lf_tags.database` (R11 revoke ordering).
- The consumer administrator needs `DESCRIBE` on the LF-tag keys and values used, to re-grant by
  tag (sandbox result 5). `aws_lakeformation_permissions.share_tag_describe` grants `DESCRIBE`
  with grant option, through an `lf_tag` block, to each consumer account for the tenant tag and
  each platform tag key and value in the share expression. `GRANT_WITH_LF_TAG_EXPRESSION` can't be
  granted across accounts (`MANAGE_TAG_POLICY is not supported cross account`).
- New tenant output `shares`, rolled up in the root `tenants` output: consumer account → shared
  catalog IDs (`<account>:s3tablescatalog/<bucket>`), database names, and expressions (FR-027).
- Removing a share destroys its grants, which revokes access. Removing an approved account still
  referenced by a share fails the tenant precondition (FR-025).

**Deployer policy (`modules/deployer`)**:

- **New statements**:
  - `GlueCatalogSettings`: `glue:GetDataCatalogEncryptionSettings`,
    `glue:PutDataCatalogEncryptionSettings`, `glue:GetResourcePolicy`, `glue:PutResourcePolicy` and
    `glue:DeleteResourcePolicy`.
  - `LakeFormationCrossAccount`: `ram:CreateResourceShare`, `ram:AssociateResourceShare`,
    `ram:DisassociateResourceShare`, `ram:GetResourceShares`, `ram:UpdateResourceShare`,
    `ram:DeleteResourceShare`, `ram:ListResourceSharePermissions`,
    `ram:AssociateResourceSharePermission` and `ram:DisassociateResourceSharePermission`, with
    `ram:ResourceShareName` like `LakeFormation*` where the action supports it. The list mirrors
    `AWSLakeFormationCrossAccountManager`.
- **Extended statements**: `lakeformation:PutDataLakeSettings` is already covered, and the
  existing role-management statement already covers the new role name (`iam:PassRole` for
  `<prefix>-glue-catalog-encryption`).

### Environment Configuration

- **New root inputs, all optional with secure defaults**:
  - `account_settings_config`:
    - `owner` (bool, defaults to `catalog_integration_config.enabled`, so existing secondary
      deployments stay non-owners);
    - `cross_account_version` (number, default 4, validated 4–5);
    - `data_lake`: `allow_external_data_filtering`, `external_data_filtering_allow_list`,
      `authorized_session_tag_values`, `allow_full_table_external_data_access`,
      `trusted_resource_owners`;
    - `catalog_encryption`: `key_access_mode` (`service_role` | `per_caller`), `kms_key_arn`,
      `additional_key_user_arns`, `key_deletion_principal_arns`;
    - `catalog_resource_policy`: `enabled`, `deny_policy_jsons`;
    - `permissions_boundary_arn`.
  - `studio_config`: `enabled` (default false) and `integration_principal_arns`.
  - `sharing_config`: `consumer_accounts`.
  - `governance_config.direct_access_exempt_principal_arns` (new optional attribute).
- **Validation** (FR-001, FR-004, FR-007):
  - Owner-only fields must be at their defaults when `owner` is false.
  - External data filtering requires a non-empty allow list.
  - The studio flag and its principal list must agree.
  - No wildcards, and all principals are same-account ARNs.
  - Owner-only checks that compare against the account use preconditions.
- **Examples**:
  - `examples/complete`: studio support, one approved consumer account (a placeholder ID, plan
    only), and a tenant share.
  - `examples/secondary`: a non-owner.
  - `examples/defaults`: unchanged, which shows the secure defaults.

### Complexity Level

**Baseline and Enhanced alike**. Every control has no infrastructure cost, so it applies to every
environment type. Ephemeral differs only in allowing the catalog key to change and in the warning
about module-created catalog keys. Disposable stacks that own the settings supply a long-lived
catalog key (`catalog_encryption.kms_key_arn`), so `verify-down` never deletes a key that encrypts
account-wide catalog metadata.

### State Management

Consumer-owned backend, unchanged. There are no prior installations outside ephemeral test
deployments, so migration is needed only for those:

- **Moves**: `moved` blocks relocate the data-lake settings
  (`module.catalog_integration[0].aws_lakeformation_data_lake_settings.this` →
  `module.account_settings[0].aws_lakeformation_data_lake_settings.this`) and the guard. Upgrading
  is a single apply (SLO Migration). The sandbox upgrade test applies the fixture at `v0.3.1`,
  then upgrades it to this branch.
- **Ownership hand-over** (FR-001): the consumer adds `removed { from =
  module.<name>.module.account_settings[0]  lifecycle { destroy = false } }` before setting
  `owner = false`.
  - Otherwise Terraform would destroy the settings. The provider's delete either resets the
    data-lake settings or is a no-op; T-S1 records which for both resources.
  - The README documents this sequence and the new owner's import commands.

## Project Structure

### Documentation (this infrastructure)

```text
specs/003-harden-catalog-settings/
├── spec.md              # Infrastructure specification
├── plan.md              # This file
├── checklists/
│   └── requirements.md  # Spec quality checklist
└── tasks.md             # /iac.tasks (not yet created)
```

### Source Code (repository root)

```text
.
├── main.tf                     # + module "account_settings", settings_guard (renamed), moved blocks
├── dependencies.tf             # + live encryption-settings data source; data-lake settings read for all checks
├── variables.tf                # + account_settings_config, studio_config, sharing_config, tenants[*].shares,
│                               #   governance_config.direct_access_exempt_principal_arns
├── locals.tf                   # settings_owner, catalog key ARN, persona key users, platform_contract.sharing / direct_access
├── kms.tf                      # unchanged: platform key keeps table and results encryption only
├── table-buckets.tf            # + DenyDirectTableAccess
├── outputs.tf                  # + catalog_encryption_role_arn, account_settings_owner, shares in tenants output
├── modules/
│   ├── account-settings/       # NEW: data-lake settings, catalog key, encryption role + settings, Glue policy
│   │   ├── main.tf
│   │   ├── kms.tf              # dedicated catalog-metadata key, alias, and policy
│   │   ├── iam.tf
│   │   ├── variables.tf
│   │   ├── outputs.tf
│   │   ├── versions.tf
│   │   └── README.md
│   ├── catalog-integration/    # − data-lake settings; + settings_ready, permissions_boundary_arn
│   ├── tenant/
│   │   ├── sharing.tf          # NEW: cross-account LF-tag grants, share outputs
│   │   ├── storage.tf          # + DenyDirectTableAccess on dedicated buckets
│   │   ├── variables.tf        # + shares in tenant type, platform.sharing, platform.direct_access
│   │   └── main.tf             # + share preconditions
│   ├── deployer/main.tf        # + GlueCatalogSettings, LakeFormationCrossAccount
│   └── cost-estimation/        # + catalog KMS request line
├── examples/{complete,secondary,standalone-tenant}/   # updated inputs
├── test/                       # + account_settings_test.go, sharing_test.go; extended validation/secondary/complete tests
└── verify/
    ├── fixture/                # + bypass persona (broad s3tables IAM, no LF grant), consumer persona,
    │                           #   share grants
    ├── internal/ops/           # + direct s3tables data call, settings and encryption inspection ops
    ├── internal/preflight/     # + consumer-account preflight; RAM invitation acceptance when outside the org
    └── matrix/full.yaml, probe.example.yaml  # + FR-020 cases 1–10
```

**Structure Decision**: Extend the existing root module and its submodules. The new
`modules/account-settings` groups everything that the account-settings owner flag controls, the
same way `catalog-integration` groups the registration singleton. The per-tenant sharing grants
go in `modules/tenant` so that tenant authority bounds them (FR-023).

## Spec Deviations (applied to spec.md 2026-11-18)

- **D1**
  - *Spec*: FR-006: "fail at plan time when an unmanaged catalog resource policy exists that lacks
    the resource-sharing statement"
  - *Deviation*: Detected by an `lfverify` probe case and documented in the README, not at plan
    time.
  - *Reason*: No Terraform data source reads a Glue resource policy, and the module avoids
    `external` scripts.
- **D2**
  - *Spec*: FR-018: "add a Region restriction where the permission service supports it"
  - *Deviation*: No Region condition on the registration role.
  - *Reason*: AWS documents only `aws:SourceAccount` for registered roles (R10), and an undocumented
    condition risks breaking credential vending. The spec's "where supported" clause allows this;
    record it as not supported.
- **D3** (resolved by T007, sandbox result 5)
  - *Spec*: FR-026
  - *Deviation*: none any more. Consumers get **no** catalog-key grant in either mode.
  - *Reason*: Glue catalog encryption doesn't cover `s3tablescatalog` metadata, and the consumer
    read shared tables created under encryption without a key grant. FR-009 and FR-026 were
    amended.
- **D4**
  - *Spec*: FR-003: list of explicit fields
  - *Deviation*: Adds `SET_CONTEXT` to the explicit parameters.
  - *Reason*: `PutDataLakeSettings` replaces `Parameters` wholesale (R2).

## Sandbox Confirmation (T-S1, first implementation task)

Run in `492637581429`, with the consumer account once one is provisioned. Record the results in
`specs/003-harden-catalog-settings/sandbox-results.md`.

1. The live `CROSS_ACCOUNT_VERSION` default and the casing of `SET_CONTEXT`.
2. Provider delete behavior for `aws_lakeformation_data_lake_settings` and
   `aws_glue_data_catalog_encryption_settings`.
3. The catalog-encryption role works with `aws:SourceAccount`, and with or without
   `aws:SourceArn`.
4. Governed Athena DDL, DML and SELECT, and table maintenance, still work with
   `DenyDirectTableAccess` in place. Direct `GetTableData` by a broad-IAM role is denied.
5. An LF-tag expression grant to an external account on `s3tablescatalog` objects works with the
   grant option. Whether the consumer needs LF-tag `DESCRIBE` to re-grant. Consumer queries with
   and without the metadata-key grant in service-role mode (D3).
6. Catalog lookup latency before and after encryption (Performance NFR).
7. The oldest provider version exposing `catalog_encryption_service_role` and data-lake
   `parameters` (version floor).

## Complexity Tracking

- **Two independent singleton flags: catalog-integration and account-settings owner, giving four
  valid combinations**
  - *Why Needed*: The spec (FR-001) asks for a dedicated owner flag, so settings ownership can move
    without moving the S3 Tables registration.
  - *Simpler Alternative Rejected Because*: One flag can't express a deployment that registers
    storage while another deployment governs the settings. The extra guard path reuses the existing
    secondary-deployment guard.
