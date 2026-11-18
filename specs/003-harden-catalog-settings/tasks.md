---

description: "Implementation tasks for Hardened Catalog and Data Permission Account Settings"
---

# Tasks: Hardened Catalog and Data Permission Account Settings

**Input**: Design documents from `/specs/003-harden-catalog-settings/`
**Prerequisites**: plan.md, spec.md (no research.md, architecture.md, modules.md, or quickstart.md)

**Organization**: Tasks follow the dependency order Sandbox Confirmation → Setup → Foundation
(account settings) → Platform Data (keys and buckets) → Tenant Sharing → Operations → Examples and
Tests → Verification Harness → Polish.

Like 001, this module has **no network tier**: every service is a Regional AWS public endpoint. The
tier that would hold networking holds the account-wide Lake Formation and Glue settings instead,
because every later tier depends on them.

## Format: `[ID] [P?] Description`

- **[ID]**: Sequential task number (T001, T002, T003...)
- **[P]**: Can run in parallel (different files, no dependencies) - optional
- **Description**: Clear action with exact file path included

## Path Conventions

- The module lives at the **repository root** (Registry layout). Do not use `iac/`.
- Submodules go in `modules/<name>/`, examples in `examples/<name>/`, Terratest in `test/`, and the
  verification harness in `verify/`.
- **All validation runs through `make` targets** (principles, Validate Before Merge). Never invoke
  `terraform`, `tflint`, `terraform-docs`, or `go test` directly.
- Every policy uses `aws_iam_policy_document`, never inline JSON. Reference other resources through
  resource attributes, module outputs, the platform contract, or inputs, never through ARNs built
  from naming conventions. Follow the comment and section-header style in `AGENTS.md`.
- **Platform/tenant boundary** (001): the root never creates tenant-scoped resources, and
  `modules/tenant` never modifies platform-owned resources. It reads platform values only from
  `var.platform`.
- **Tagging** (bd memory `data-tags-data-at-rest-only`): new roles and policies get `common_tags`
  only, never `data_tags`.
- Requirement IDs (FR-xxx) refer to `spec.md`. R-, D- and T-S1 references refer to `plan.md`.

---

## Phase 1: Sandbox Confirmation and Setup

**Purpose**: Confirm the undocumented AWS behaviors the design depends on (plan T-S1), then add the
new inputs. Design changes found here are recorded in `plan.md` before Phase 2 starts.

- [x] T001 Create `specs/003-harden-catalog-settings/sandbox-results.md` with one section for each
  plan T-S1 item (1–7), each with fields *Command*, *Observed*, *Date*, and *Design impact*.
- [x] T002 In sandbox `492637581429` (us-east-1), run `aws lakeformation get-data-lake-settings`
  and `aws glue get-data-catalog-encryption-settings`. Record in
  `specs/003-harden-catalog-settings/sandbox-results.md`:
  - the live `Parameters` map: the `CROSS_ACCOUNT_VERSION` value, and the key and value casing of
    `SET_CONTEXT`;
  - the live catalog encryption mode and key (T-S1 items 1 and 7 baseline).
- [x] T003 Check the provider version floor (T-S1 item 7) using the `hashicorp/aws` CHANGELOG.
  Find the oldest release that exposes both
  `aws_glue_data_catalog_encryption_settings.encryption_at_rest.catalog_encryption_service_role`
  and `aws_lakeformation_data_lake_settings.parameters`, and record it in
  `specs/003-harden-catalog-settings/sandbox-results.md`.
  - If it is above 6.59.0, raise the constraint in `versions.tf` and in every
    `examples/*/versions.tf` and `modules/*/versions.tf`.
- [x] T004 Run a throwaway Terraform configuration in the sandbox, outside the module (scratch
  directory, local state), and record the results in
  `specs/003-harden-catalog-settings/sandbox-results.md`:
  - **Delete behavior** (T-S1 item 2): apply then destroy
    `aws_glue_data_catalog_encryption_settings`, and apply then destroy
    `aws_lakeformation_data_lake_settings` with the current admins preserved. Record what each
    destroy leaves behind.
  - **Encryption role trust** (T-S1 item 3): create the catalog-encryption role with
    `aws:SourceAccount`, then with `aws:SourceAccount` and
    `aws:SourceArn = arn:aws:glue:us-east-1:<account>:catalog`. Enable
    `SSE-KMS-WITH-SERVICE-ROLE` with each and run `aws glue create-database` and `get-database`.
    Record which trust works.
  - **Cleanup**: restore the original settings from T002 afterwards.
- [x] T005 With the existing 001 sandbox deployment (`make verify-up` and `make verify-seed`), add
  a manual `DenyDirectTableAccess` statement (plan, Data Storage) to one zone bucket policy using
  the AWS CLI. Then run, and record in `specs/003-harden-catalog-settings/sandbox-results.md`
  (T-S1 item 4):
  - Athena `CREATE TABLE`, `INSERT`, `SELECT`, and `DROP TABLE` as a writer persona;
  - a bucket maintenance status check;
  - a direct `s3tables get-table-metadata-location` plus S3 `GetObject` as an IAM principal with
    `s3tables:*` and no Lake Formation grant.

  Restore the policy with `make verify-down` afterwards. If any governed operation is denied, add
  the missing exemption to the plan before T029.
- [x] T006 Record catalog lookup latency (T-S1 item 6). Time 20 `aws glue get-table` calls on a
  fixture table before and after enabling `SSE-KMS-WITH-SERVICE-ROLE` in the sandbox, and record
  the medians and the increase in `specs/003-harden-catalog-settings/sandbox-results.md`. Restore
  encryption to the T002 state afterwards.
- [x] T007 Deferred until the consumer sandbox account exists (T008). Record in
  `specs/003-harden-catalog-settings/sandbox-results.md` (T-S1 item 5):
  - **Grant**: grant `SELECT` and `DESCRIBE` with the grant option through an LF-tag expression
    from the producer to the consumer account ID on an `s3tablescatalog` table.
  - **Consumer access**: in the consumer, create a resource link and query it through Athena.
    Record whether the consumer administrator needs LF-tag `DESCRIBE` to re-grant by tag.
  - **Catalog key need (D3)**: with `SSE-KMS-WITH-SERVICE-ROLE` enabled in the producer, query
    with and without the consumer `kms:Decrypt` key-policy grant, and record which works.
- [x] T008 [P] Add the consumer sandbox account to `verify/sandbox-accounts.txt` with the comment
  `# consumer for 003 sharing cases`. It must be in the same organization and Region as
  `492637581429`. Ask the user for the account ID; never invent one.
- [x] T009 Create a long-lived sandbox catalog key for ephemeral fixtures (FR-013, U2) in
  `492637581429` us-east-1 with the AWS CLI. This is a one-time manual step, outside any fixture
  state.
  - Create a symmetric key with alias `alias/lfverify-catalog`, rotation enabled.
  - Key policy:
    - the default account-root statement (IAM delegation);
    - the `AllowGlueCatalogEncryption` statement (`kms:CreateGrant` to the account root with
      `kms:CallerAccount`, `kms:GrantIsForAWSResource = true`, and
      `kms:EncryptionContext:glue_catalog_id = 492637581429`);
    - a `ProtectCatalogKey` deny on `kms:ScheduleKeyDeletion` and `kms:DisableKey` to `*`, with no
      exceptions.
  - Don't name fixture roles in the policy. They come and go with each run, so they get key access
    through their own IAM policies (T058).
  - Record the key ARN in `verify/README.md` and in `specs/003-harden-catalog-settings/sandbox-results.md`,
    and pass it to the fixture through `LFVERIFY_CATALOG_KEY_ARN`.
- [x] T010 Edit `variables.tf` to add `account_settings_config` (FR-001 to FR-006, FR-009 to FR-014,
  FR-019).
  - **Type**: `object({ ... })` with these attributes:
    - `owner = optional(bool)`. Null means "follow `catalog_integration_config.enabled`".
    - `cross_account_version = optional(number, 4)`.
    - `data_lake = optional(object({ allow_external_data_filtering = optional(bool, false),
      external_data_filtering_allow_list = optional(list(string), []),
      authorized_session_tag_values = optional(list(string), []),
      allow_full_table_external_data_access = optional(bool, false),
      trusted_resource_owners = optional(list(string), []) }), {})`.
    - `catalog_encryption = optional(object({ key_access_mode = optional(string, "service_role"),
      kms_key_arn = optional(string, ""), additional_key_user_arns = optional(list(string), []),
      key_deletion_principal_arns = optional(list(string), []) }), {})`.
    - `catalog_resource_policy = optional(object({ enabled = optional(bool, false),
      deny_policy_jsons = optional(list(string), []) }), {})`.
    - `permissions_boundary_arn = optional(string, "")`.
  - **Default**: `{}`.
  - **Validation blocks**:
    - `cross_account_version` is 4 or 5;
    - `key_access_mode` is `service_role` or `per_caller`;
    - `kms_key_arn` is empty or a KMS key ARN;
    - every principal list holds IAM role or user ARNs with no `*`;
    - allow-list entries are 12 digits;
    - `allow_external_data_filtering` requires a non-empty allow list (FR-004);
    - every `deny_policy_jsons` entry decodes to a document whose statements all have
      `Effect = "Deny"`;
    - `permissions_boundary_arn` is empty or an IAM policy ARN;
    - `length(additional_key_user_arns) <= 100`.
  - **Descriptions**: each attribute's description states its security consequence (FR-004).
    `cross_account_version` says that 5 is irreversible and lists the RAM permissions a custom
    grantor policy needs (R12).
- [x] T011 Edit `variables.tf` to add `studio_config` (FR-007, FR-008).
  - **Type**: `object({ enabled = optional(bool, false),
    integration_principal_arns = optional(list(string), []) })`, defaulting to `{}`.
  - **Validation blocks**:
    - `enabled == length(integration_principal_arns) > 0`, with separate error messages for each
      direction;
    - every entry is a role ARN with no `*`.
  - **Description**: names the SageMaker Unified Studio blueprint provisioning role
    (`AmazonSageMakerProvisioning-<account>`) and the IAM-based domain execution role as the
    expected entries (R8). It states that the module creates no studio resources.
- [x] T012 Edit `variables.tf`:
  - **`sharing_config`** (FR-022): `object({ consumer_accounts = optional(map(object({ purpose =
    string, outside_organization = optional(bool, false) })), {}) })`, default `{}`. Validate that
    keys match `^[0-9]{12}$`, that `purpose` is non-empty, and that there are at most 50 entries.
  - **`governance_config.direct_access_exempt_principal_arns`**: add it as
    `optional(list(string), [])`, validated as IAM role or user ARNs with no `*` and at most 20
    entries (FR-015, FR-016).
- [x] T013 Edit the `tenants` type in `variables.tf` to add `shares` (FR-023, FR-024):
  - **Type**: `shares = optional(map(object({ account_id = string, zones = optional(list(string),
    ["consumption"]), sensitivity_values = list(string), regulations = optional(set(string), []),
    domains = optional(list(string)), compliance_acknowledged = optional(bool, false) })), {})`.
  - **Validation**: every `account_id` is 12 digits; `sensitivity_values` is non-empty and drawn
    from `governance_config.sensitivity_values`.
  - **Sync**: copy the identical attribute into the `tenant` variable type in
    `modules/tenant/variables.tf`. `TestTenantTypeInSync` enforces this.
- [x] T014 Edit `locals.tf` to add:
  - `settings_owner = coalesce(var.account_settings_config.owner,
    var.catalog_integration_config.enabled)`;
  - `supplied_catalog_kms_key_arn = var.account_settings_config.catalog_encryption.kms_key_arn`.
    Empty means `modules/account-settings` creates the dedicated catalog key. It never falls back
    to `local.kms_key_arn`, the platform key (FR-009);
  - `tenant_persona_arns`: a distinct, sorted list of every `principal_arn` in the readers, writers,
    metadata readers, and metadata writers of `var.tenants`, plus every steward. Used for per-caller
    key users.
  - Owner-only checks as `check` blocks: when `!local.settings_owner`, `cross_account_version`,
    `data_lake`, `catalog_encryption`, `catalog_resource_policy`, and `permissions_boundary_arn`
    must equal their defaults (FR-001 validation). Use `lifecycle.precondition` on
    `terraform_data.settings_guard` (T025) so the plan fails rather than warns.
- [x] T015 Run `make check`. This is the setup checkpoint.

**Checkpoint**: Undocumented behaviors are confirmed (except T007, which waits for the consumer
account), and the new inputs validate.

---

## Phase 2: Foundation Tier (Account-Wide Settings, FR-001 to FR-014, FR-018, FR-019)

**Purpose**: The new `modules/account-settings` submodule, controlled by the owner flag, and the
guards for deployments that don't own the settings. Every later tier depends on the data-lake admin
list it manages.

### Account-settings submodule

- [ ] T016 [P] Create `modules/account-settings/versions.tf`, copying `required_version` and the
  `hashicorp/aws` constraint from the root `versions.tf` (no pricing alias), plus
  `hashicorp/time >= 0.12` for the role propagation delay (sandbox result 2). Add the same `time`
  requirement to the root `versions.tf`. Create
  `modules/account-settings/variables.tf` with these variables, each validated like its root
  counterpart:
  - `name_prefix`, `account_id`, `partition`, `region`, `tags`;
  - `admin_principal_arns`, `read_only_admin_principal_arns`, `deployer_role_arn`;
  - `cross_account_version`, `data_lake` (object), `catalog_encryption` (object; `kms_key_arn`
    empty means "create the dedicated key"), `catalog_resource_policy` (object),
    `permissions_boundary_arn`;
  - `environment_type` and `kms_key_deletion_window_days` (from the root effective config);
  - `key_user_arns` (tenant persona ARNs for per-caller mode);
  - `storage_key_arns` (platform key ARN plus known tenant key ARNs, used only to reject a supplied
    catalog key that equals one of them).

  Add the header comment: "The dedicated catalog key, its policy, and the catalog-encryption role
  live together here. The key policy references the role, the role's inline policy references the
  key, and the role itself depends on neither, so the graph has no cycle."
- [ ] T017 Create `modules/account-settings/main.tf` with `aws_lakeformation_data_lake_settings.this`,
  moved verbatim from `modules/catalog-integration/main.tf`, including its comments and the
  deployer precondition. Then extend it:
  - Set `allow_external_data_filtering`, `external_data_filtering_allow_list`,
    `authorized_session_tag_value_list`, `allow_full_table_external_data_access`, and
    `trusted_resource_owners` from `var.data_lake`.
  - Set `parameters = { CROSS_ACCOUNT_VERSION = tostring(var.cross_account_version), SET_CONTEXT =
    "TRUE" }`, using the casing recorded in T002 (D4).
  - Add a comment explaining that `PutDataLakeSettings` replaces every field, so each one is
    declared here (FR-003).
  - Add a second precondition: when `data.aws_lakeformation_data_lake_settings.current` returns a
    `CROSS_ACCOUNT_VERSION`, it must be `<= var.cross_account_version`. The error message explains
    that versions are not lowered (FR-005). Add that data source to the same file. Guard against a
    missing key with `try(..., "0")`.
- [ ] T018 Create `modules/account-settings/iam.tf` with:
  - `aws_iam_role.catalog_encryption`, with `count = var.catalog_encryption.key_access_mode ==
    "service_role" ? 1 : 0`, name `${var.name_prefix}-glue-catalog-encryption`,
    `permissions_boundary = var.permissions_boundary_arn != "" ? var.permissions_boundary_arn :
    null`, and `tags = var.tags`;
  - the trust document: `glue.amazonaws.com`, `sts:AssumeRole`, `aws:SourceAccount` set to the
    account **only**. Sandbox result 3 showed that `aws:SourceArn` with the catalog ARN breaks
    catalog access, so add a comment citing it (FR-011);
  - `aws_iam_role_policy.catalog_encryption`, granting `kms:Decrypt`, `kms:Encrypt`, and
    `kms:GenerateDataKey` on `local.catalog_key_arn` only (the created or supplied catalog key).
- [ ] T019 Create `modules/account-settings/kms.tf` with the dedicated catalog-metadata key
  (FR-009, FR-012, FR-013, FR-026):
  - **Key**: `aws_kms_key.catalog`, with `count = var.catalog_encryption.kms_key_arn == "" ? 1 :
    0`, `enable_key_rotation = true`, `deletion_window_in_days =
    var.kms_key_deletion_window_days`, description `"Catalog metadata key for
    ${var.name_prefix}"`, and `tags = merge(var.tags, { Name = "${var.name_prefix}-catalog-cmk"
    })`. Common tags only.
  - **Alias**: `aws_kms_alias.catalog`, `alias/${var.name_prefix}-catalog-cmk`.
  - **Local**: `catalog_key_arn = var.catalog_encryption.kms_key_arn != "" ?
    var.catalog_encryption.kms_key_arn : aws_kms_key.catalog[0].arn`.
  - **`data.aws_iam_policy_document.required_catalog_key_statements`**, with these statements:
    - `AllowGlueCatalogEncryption`: `kms:CreateGrant` to the account root, with
      `kms:CallerAccount = account`, `kms:GrantIsForAWSResource = true`, and
      `kms:EncryptionContext:glue_catalog_id = account` (R4).
    - `AllowCatalogEncryptionRole`, service-role mode only: `kms:Encrypt`, `kms:Decrypt`,
      `kms:GenerateDataKey`, and `kms:DescribeKey` to the catalog-encryption role.
    - `AllowCatalogKeyUsers`, per-caller mode only: `kms:Encrypt`, `kms:Decrypt`,
      `kms:GenerateDataKey`, `kms:DescribeKey`, and `kms:CreateGrant`, with condition
      `kms:EncryptionContext:glue_catalog_id = account`. The principals are the distinct union of:
      - admins and read-only admins;
      - studio principals;
      - `var.key_user_arns`;
      - `additional_key_user_arns`.

      Add a precondition on the key capping the total at 200 (32 KB key-policy limit).
    - `ProtectCatalogKey` (FR-013): `Deny` `kms:ScheduleKeyDeletion` and `kms:DisableKey` to
      principal `*`, with `ArnNotEquals aws:PrincipalArn` set to `key_deletion_principal_arns`.
      Omit the condition when the list is empty, which makes the deny absolute. This applies in
      **every** environment type. Add a comment that `PutKeyPolicy` stays allowed for recovery and
      that Ephemeral owners should supply an external key.
  - **`data.aws_iam_policy_document.catalog_key_policy`**: `source_policy_documents` set to the
    required statements, plus `EnableRootAccess` (`kms:*` to the account root with
    `aws:PrincipalAccount = account`), copied from the root `kms.tf` pattern.
  - **Precondition (FR-009)**: a supplied `kms_key_arn` must not be in `var.storage_key_arns`.
- [ ] T020 Add to `modules/account-settings/main.tf`:
  - **`time_sleep.catalog_encryption_role`**, with `count` matching the role: `create_duration =
    "20s"`, `triggers = { role_arn = <role ARN>, policy = <role policy ID> }`. Glue rejects a new
    role as "not assumable" for about 10–15 seconds (sandbox result 2).
  - **`aws_glue_data_catalog_encryption_settings.this`** (FR-009, FR-010):
    - `catalog_encryption_mode` is `SSE-KMS-WITH-SERVICE-ROLE` or `SSE-KMS`;
    - `sse_aws_kms_key_id = local.catalog_key_arn`;
    - `catalog_encryption_service_role` is the role ARN in service-role mode;
    - `connection_password_encryption { return_connection_password_encrypted = true,
      aws_kms_key_id = <same key> }`;
    - `depends_on` the role policy and `time_sleep.catalog_encryption_role`.
  - **`data "aws_glue_data_catalog_encryption_settings" "current"`**.
  - **Precondition (R4)**: fail when the live mode is `SSE-KMS`, the live key is the AWS-managed
    key (the `aws/glue` alias, or a key ID resolved through `data "aws_kms_alias"` for `aws/glue`),
    and the target is service-role mode. The message references the README section "Migrating
    catalog encryption" (T066).
  - **Precondition (FR-013, key fixed at installation)**: when
    `var.environment_type != "Ephemeral"` (new submodule variable, passed from the root) and the live
    `encryption_at_rest[0].sse_aws_kms_key_id` is non-empty, it must equal
    `local.catalog_key_arn`. Compare after normalizing alias, key ID, or ARN through
    `data "aws_kms_key"`. The message says key changes are unsupported after installation.
  - **`check "ephemeral_catalog_key"`** (FR-013): when the environment type is Ephemeral and
    the submodule creates the catalog key (`var.catalog_encryption.kms_key_arn == ""`), warn that
    teardown can't delete a key protecting
    account-wide catalog metadata. Recommend `catalog_encryption.kms_key_arn`.
  - **Comment**: enabling encryption only covers objects written afterwards (FR-014), and disabling
    it leaves existing objects encrypted, which is why the key is protected (FR-013). If T004 showed
    that destroy disables encryption, say so.
- [ ] T021 Add `aws_glue_resource_policy.this` to `modules/account-settings/main.tf`, with
  `count = var.catalog_resource_policy.enabled ? 1 : 0` (FR-006):
  - **Policy document**: built with `data "aws_iam_policy_document" "catalog_resource_policy"`.
    - Statement `RamShareResource`: `glue:ShareResource` for the `ram.amazonaws.com` service
      principal, on `arn:<partition>:glue:<region>:<account>:catalog`, `.../database/*`, and
      `.../table/*/*`.
    - `override_policy_documents = var.catalog_resource_policy.deny_policy_jsons`.
  - **`enable_hybrid = "TRUE"`** (R3).
  - **Comment**: no policy is needed for LF-tag sharing on version 4. An unmanaged policy is
    detected by `lfverify` (D1).
- [ ] T022 Create `modules/account-settings/outputs.tf` with outputs:
  - `data_lake_settings_id`;
  - `catalog_encryption_role_arn` (empty when per-caller);
  - `catalog_encryption_mode`;
  - `cross_account_version`.

### Catalog-integration changes

- [ ] T023 Edit `modules/catalog-integration`:
  - **Remove** `aws_lakeformation_data_lake_settings.this` from `main.tf`, and remove the
    `admin_principal_arns`, `read_only_admin_principal_arns`, and `deployer_role_arn` variables
    from `variables.tf`, along with their uses.
  - **Add** `variable "settings_ready"` (string, no default, description "ID of the data-lake
    settings or the settings guard; orders registration after admin rights exist"). Reference it
    from `aws_lakeformation_resource.s3tables` and `aws_glue_catalog.s3tables` through
    `depends_on` on a `terraform_data.settings_ready` resource whose `input = var.settings_ready`.
  - **Add** `variable "permissions_boundary_arn"` (string, default `""`) and set
    `permissions_boundary` on `aws_iam_role.lf_s3tables_access` in `iam.tf` (FR-019).
  - **Comment**: in `iam.tf`, document that the trust policy keeps only `aws:SourceAccount` because
    AWS documents no Region condition (FR-018, D2).

### Root wiring and guards

- [ ] T024 Edit `main.tf`:
  - **Module call**: add `module "account_settings"` with `count = var.enabled &&
    local.settings_owner ? 1 : 0`, passing the T016 inputs:
    - `admin_principal_arns = concat(var.governance_config.admin_principal_arns,
      var.studio_config.enabled ? var.studio_config.integration_principal_arns : [])`;
    - `catalog_encryption = var.account_settings_config.catalog_encryption` (an empty
      `kms_key_arn` means the submodule creates the dedicated key);
    - `environment_type = var.environment_type` and `kms_key_deletion_window_days =
      local.effective_config.kms_key_deletion_window_days`;
    - `key_user_arns = local.tenant_persona_arns`;
    - `storage_key_arns = compact([local.kms_key_arn])`. Tenant keys are created inside the tenant
      modules after this call; the T027 root precondition checks them instead.
    - `tags = local.common_tags`.
  - **`module "catalog_integration"`**: drop the removed inputs, and pass `settings_ready =
    local.settings_owner ? module.account_settings[0].data_lake_settings_id :
    terraform_data.settings_guard[0].id` and `permissions_boundary_arn`.
  - **`moved` blocks** for upgrades:
    - `module.catalog_integration[0].aws_lakeformation_data_lake_settings.this` →
      `module.account_settings[0].aws_lakeformation_data_lake_settings.this`;
    - `terraform_data.secondary_admin_guard` → `terraform_data.settings_guard`.
- [ ] T025 Edit `main.tf` and `dependencies.tf` so deployments that don't own the settings run a
  guard instead (FR-001, FR-007).
  - **Rename** `terraform_data.secondary_admin_guard` to `terraform_data.settings_guard`, with
    `count = var.enabled && !local.settings_owner ? 1 : 0`.
  - **Data sources** in `dependencies.tf`, with the same count:
    - change the `aws_lakeformation_data_lake_settings.current` count to the same condition;
    - add `data "aws_glue_data_catalog_encryption_settings" "current"`.
  - **Preconditions**:
    1. the deployer is in the live admins (existing check, with the message naming the owner
       deployment);
    2. the live encryption mode is not `DISABLED`;
    3. if any tenant has non-empty `shares`, the live `CROSS_ACCOUNT_VERSION` is at least 4;
    4. if `studio_config.enabled`, every integration principal is in the live admins;
    5. owner-only inputs are at their defaults (from T014).
- [ ] T026 Run `make check` and `make lint`. This is the foundation checkpoint.

**Checkpoint**: Account-wide settings are managed by exactly one deployment, and every other
deployment is guarded.

---

## Phase 3: Platform Data Tier (FR-013, FR-015 to FR-017, FR-026)

**Purpose**: Keep the platform key free of catalog statements, expose the catalog key, and add the
bypass deny on shared buckets and the contract fields that tenants need.

- [ ] T027 Edit `kms.tf` and `main.tf` to keep the catalog and storage keys separate (FR-009,
  FR-026):
  - **`kms.tf`**: add no statements to `data.aws_iam_policy_document.kms_key_policy` or
    `required_kms_statements`. Add a comment above the platform key: "Encrypts table buckets and
    query results only; catalog metadata uses the dedicated key in modules/account-settings."
  - **`main.tf`**: add a precondition on
    a new `terraform_data.catalog_key_guard` with `count = var.enabled && local.settings_owner ? 1
    : 0`. When a catalog key is supplied, it must not equal any `kms_key_arn` in
    `module.tenant[*]` outputs (designated tenant keys).
- [ ] T028 Edit `modules/account-settings/outputs.tf` and the root `outputs.tf`:
  - `catalog_kms_key_arn` (the created or supplied key);
  - `required_catalog_kms_key_policy_json`, from `required_catalog_key_statements`, for
    consumer-supplied catalog keys (FR-009). Its description says the root-access statement is
    the consumer's own responsibility;
  - update the `required_kms_key_policy_json` description to say it covers the platform key only.
- [ ] T029 [P] Edit `table-buckets.tf` to add `DenyDirectTableAccess` to
  `data.aws_iam_policy_document.zone_bucket` (FR-015 to FR-017):
  - **Effect and principal**: `Deny`, principal `*`.
  - **Actions**: `s3tables:GetTableData`, `s3tables:PutTableData`, `s3tables:CreateTable`,
    `s3tables:DeleteTable`, `s3tables:RenameTable`, `s3tables:UpdateTableMetadataLocation`,
    `s3tables:GetTableMetadataLocation`, plus any action T005 showed is needed.
  - **Resources**: the bucket ARN and `/*`.
  - **Conditions**: `ArnNotEquals aws:PrincipalArn` = `concat([local.data_access_role_arn],
    var.governance_config.direct_access_exempt_principal_arns)`, and `Bool
    aws:PrincipalIsAWSService = false`.
  - **Comments**: the deny leaves S3 Tables maintenance and S3 metadata service principals
    unrestricted (R7), and namespace and bucket management stay with the deployer (FR-016).
  - Update the file's leading comment ("Grants nothing; ...") to mention the deny.
- [ ] T030 Edit `locals.tf` to add to `local.platform_contract`:
  - `sharing = { consumer_accounts = var.sharing_config.consumer_accounts }`. Don't put the
    protocol version in the contract: a value copied at platform-apply time can be stale, so
    tenants read the live value instead (T034);
  - `direct_access = { exempt_principal_arns =
    var.governance_config.direct_access_exempt_principal_arns }`.

  Confirm that `data_access_role_arn` is already in the contract (it is, from 001). Mirror the new
  fields in the `platform` variable type in `modules/tenant/variables.tf`, and in
  `examples/standalone-tenant/platform_contract.auto.tfvars.json` with empty values.
- [ ] T031 Run `make check` and `make lint`. This is the platform data checkpoint.

**Checkpoint**: Catalog metadata is encrypted with a protected key, and shared buckets refuse
direct access.

---

## Phase 4: Tenant Tier (FR-015, FR-021 to FR-028)

**Purpose**: Apply the bypass deny to dedicated buckets and add tenant-declared, read-only,
LF-tag-bounded shares.

- [ ] T032 [P] Edit `modules/tenant/storage.tf` to add the same `DenyDirectTableAccess` statement as
  T029 to `data.aws_iam_policy_document.dedicated_bucket`. It uses
  `var.platform.data_access_role_arn` and `var.platform.direct_access.exempt_principal_arns`.
- [ ] T033 Edit `modules/tenant/locals.tf`:
  - Generalize the reader expression builder so the same code builds the LF-tag expression for a
    reader or a share: tenant term + zones + sensitivity + regulations + domains, with the
    fail-closed regulation handling of readers.
  - Add `local.share_grants`, `{ for k, s in var.tenant.shares : k => s }`, with the zones resolved
    against the tenant's zones.
  - Add `local.share_expressions`.
- [ ] T034 Edit `modules/tenant/main.tf` to add preconditions on a new `terraform_data.share_guard`,
  with `count = length(var.tenant.shares) > 0 ? 1 : 0` (FR-023, FR-025, FR-028):
  - every `account_id` is a key of `var.platform.sharing.consumer_accounts`. The message names the
    share and tells the user to add the account to the platform `sharing_config`;
  - no `account_id` equals `var.platform.account_id`;
  - designated tenants (`local.designated`) have `compliance_acknowledged = true` on every share;
  - every share's zones are within the tenant's zones;
  - the **live** `CROSS_ACCOUNT_VERSION` is at least 4. In `modules/tenant/dependencies.tf`, change
    the count of `data "aws_lakeformation_data_lake_settings" "current"` to
    `var.verify_deployer_admin || length(var.tenant.shares) > 0 ? 1 : 0` and read
    `try(tonumber(parameters["CROSS_ACCOUNT_VERSION"]), 0)`. Update the existing admin-guard
    reference if its index expression changes. This covers platform-applied and standalone tenants
    against owner and non-owner platforms (I2).
- [ ] T035 Create `modules/tenant/sharing.tf` with a section header and two grants per share
  (FR-021, FR-024):
  - **`aws_lakeformation_permissions.share_database`**: `principal = each.value.account_id`,
    `permissions = ["DESCRIBE"]`, `permissions_with_grant_option = ["DESCRIBE"]`, and
    `lf_tag_policy { catalog_id = local.account_id, resource_type = "DATABASE" }`, with expression
    tenant term + zones.
  - **`aws_lakeformation_permissions.share_table`**: `permissions = ["SELECT", "DESCRIBE"]` and
    `permissions_with_grant_option = ["SELECT", "DESCRIBE"]`, with `resource_type = "TABLE"` and
    expression `local.share_expressions[each.key]`.
  - **`depends_on`** for both: `aws_lakeformation_resource_lf_tags.database`,
    `aws_lakeformation_lf_tag.tenant`, `terraform_data.share_guard`, and
    `terraform_data.admin_guard`. This revokes grants before tags or namespaces are destroyed (R11).
  - **Comment**: Lake Formation creates and reuses the RAM shares itself (FR-021), so the module
    never manages `aws_ram_*`.
- [ ] T036 Edit `modules/tenant/sharing.tf` to add `aws_lakeformation_permissions.share_tag_describe`
  (FR-024, label visibility). It grants `DESCRIBE`, with grant option, through an `lf_tag` block,
  on the tenant tag and on each platform tag key and value in the share expression, to each
  distinct consumer account. Sandbox result 5 showed the consumer administrator can't re-grant by
  tag without it. Grant only `DESCRIBE`, because `GRANT_WITH_LF_TAG_EXPRESSION` is rejected across
  accounts (`MANAGE_TAG_POLICY is not supported cross account`).
- [ ] T037 Edit `modules/tenant/outputs.tf` to add `output "shares"` (FR-027): a map keyed by share
  name, each with `account_id`, `catalog_ids` (`<account>:s3tablescatalog/<bucket>` per shared
  zone), `database` (the tenant namespace name), and `expression` (the list of `{ key, values }`).
- [ ] T038 Run `make check` and `make lint`. This is the tenant checkpoint.

**Checkpoint**: Tenants can share only their own labeled data, read-only, with approved accounts.

---

## Phase 5: Operations Tier (Deployer, Cost, Outputs)

- [ ] T039 [P] Edit `modules/deployer/main.tf` (R12):
  - **`GlueCatalogSettings`** statement: `glue:GetDataCatalogEncryptionSettings`,
    `glue:PutDataCatalogEncryptionSettings`, `glue:GetResourcePolicy`, `glue:PutResourcePolicy`,
    and `glue:DeleteResourcePolicy` on `arn:<partition>:glue:<region>:<account>:catalog`.
  - **`LakeFormationCrossAccount`** statement: `ram:CreateResourceShare`,
    `ram:AssociateResourceShare`, `ram:DisassociateResourceShare`, `ram:UpdateResourceShare`,
    `ram:DeleteResourceShare`, `ram:AssociateResourceSharePermission`, and
    `ram:DisassociateResourceSharePermission`, with `StringLike ram:ResourceShareName =
    LakeFormation*`. A separate unconditioned statement grants `ram:GetResourceShares` and
    `ram:ListResourceSharePermissions`, because those actions don't support the condition key.
  - **Existing role-management statement**: confirm it covers creating, tagging, and passing
    `${name_prefix}-glue-catalog-encryption`, and `iam:PassRole` to `glue.amazonaws.com`. Extend it
    if not.
  - **Permissions boundary**: allow `iam:PutRolePermissionsBoundary` on the two module roles.
  - **Key aliases**: add `kms:ListAliases` usage for the `aws/glue` alias lookup in T020 if not
    already present.
- [ ] T040 [P] Edit `modules/cost-estimation` (`variables.tf`, `locals.tf`, `outputs.tf`) and the
  root call in `cost-estimation.tf`:
  - Add 1 to `kms_key_count` when the deployment owns the settings and no catalog key is supplied
    (the dedicated catalog key).
  - Add the usage line `catalog_kms_requests`. It is estimated
  at `catalog_kms_requests_per_month` (new input, default 100000) × the KMS request price looked
  up for the Region, included only when the deployment owns the settings. It must stay under
  USD 5 at the default (spec, Cost Constraints).
- [ ] T041 Edit `outputs.tf` to add:
  - `account_settings_owner` (bool);
  - `catalog_encryption_role_arn` (empty when not the owner or in per-caller mode);
  - `catalog_encryption_mode`;
  - `studio_integration_principal_arns`, described as "Integration principals, not platform
    deployers" (FR-007);
  - `shares` in the root `tenants` output, from each tenant's `shares` output.

  Keep `data_access_role_arn` unchanged.
- [ ] T042 Run `make check`, `make lint`, and `make docs`. This is the operations checkpoint.

---

## Phase 6: Examples and Contract Tests (Plan-Only Terratest)

- [ ] T043 [P] Edit `examples/complete` (`main.tf`, `variables.tf`, `terraform.auto.tfvars`,
  `tenants.auto.tfvars`):
  - set `studio_config = { enabled = true, integration_principal_arns = [<placeholder role ARN in
    the example account pattern used today>] }`;
  - set `sharing_config.consumer_accounts = { "111122223333" = { purpose = "example consumer
    (plan only)" } }`;
  - give one tenant a `shares` entry with `account_id = "111122223333"` and `sensitivity_values =
    ["public", "internal"]`;
  - set `governance_config.direct_access_exempt_principal_arns = []`.
- [ ] T044 [P] Edit `examples/secondary` to set `account_settings_config = { owner = false }`
  explicitly, with a comment explaining the owner flag. Keep everything else unchanged.
- [ ] T045 [P] Create `test/account_settings_test.go` with plan-only tests:
  - `TestOwnerFlagCombinations`: a table test over the four combinations of `owner` and
    `catalog_integration_config.enabled` against `examples/defaults` with `-var` overrides. Assert
    the presence or absence of `module.account_settings[0]`, `module.catalog_integration[0]`, and
    `terraform_data.settings_guard[0]`.
  - `TestOwnerOnlyInputsRejectedOnNonOwner`.
  - `TestDataLakeSettingsExplicit`: in the defaults plan, every FR-003 field is planned with its
    secure default, and `parameters` holds `CROSS_ACCOUNT_VERSION = "4"` and the `SET_CONTEXT`
    entry.
  - `TestCatalogEncryptionModes`: `service_role` plans the role and `SSE-KMS-WITH-SERVICE-ROLE`;
    `per_caller` plans no role and `SSE-KMS`. Assert the matching key-policy statement in each.
  - `TestKeyProtectionStatement`: `ProtectCatalogKey` never exempts the deployer, in any
    environment type, and exempts exactly `key_deletion_principal_arns`.
  - `TestEphemeralOwnerWarnsOnModuleCreatedCatalogKey`: an Ephemeral owner whose submodule creates
    the catalog key produces the `ephemeral_catalog_key` warning in the plan output, and supplying
    `catalog_encryption.kms_key_arn` removes it.
  - `TestNoCatalogResourcePolicyByDefault`, plus a case with `enabled = true` asserting the
    `RamShareResource` statement and `enable_hybrid = "TRUE"`.
  - `TestCatalogKeyDistinctFromPlatformKey`:
    - the owner plan creates `module.account_settings[0].aws_kms_key.catalog[0]` alongside
      `aws_kms_key.main[0]`;
    - the encryption settings reference the catalog key;
    - the platform key policy contains no `glue_catalog_id` condition and no `ProtectCatalogKey`;
    - supplying the platform key ARN as `catalog_encryption.kms_key_arn` fails the plan.

  Use the helpers in `test/helpers_test.go`.
- [ ] T046 [P] Create `test/sharing_test.go` with plan-only tests:
  - `TestShareGrantsReadOnlyWithGrantOption`: the complete example plans `share_database` and
    `share_table` with exactly the permissions from T035, and the expressions include the tenant
    tag key.
  - `TestShareRejectsUnapprovedAccount`, `TestShareRejectsCurrentAccount`, and
    `TestDesignatedTenantShareRequiresAcknowledgement`.
  - `TestNoConsumerInKeyPolicies`: with consumers approved, no consumer account principal appears
    in the catalog key, platform key, or any tenant key policy (FR-026).
  - `TestShareTagDescribe`: each share plans `share_tag_describe` grants with exactly
    `DESCRIBE` and grant option `DESCRIBE`, for the tag keys and values in its expression.
  - `TestRemovingApprovedAccountStillReferencedFails`.
- [ ] T047 Edit `test/validation_test.go` (`TestInvalidInputsFailPlan` table) to add cases:
  - `cross_account_version = 3`;
  - external filtering with an empty allow list;
  - a non-Deny `deny_policy_jsons`;
  - studio enabled with no principals;
  - studio disabled with principals;
  - a wildcard exempt principal;
  - a consumer account that isn't 12 digits;
  - an invalid `key_access_mode`.
- [ ] T048 Edit `test/complete_test.go`:
  - extend `TestCompleteExampleSecurityControls` to assert that `DenyDirectTableAccess` is in
    every zone and dedicated bucket policy, exempts `data_access_role_arn`, and carries the
    `aws:PrincipalIsAWSService` condition;
  - extend `TestDataTagsOnlyOnDataAtRest` to assert that the catalog-encryption role and the
    catalog key have no data tags.
- [ ] T049 Edit `test/secondary_test.go` (`TestSecondaryExampleSkipsAccountIntegration`) to assert:
  - no `module.account_settings` resources;
  - `terraform_data.settings_guard` is present;
  - the plan contains the `aws_glue_data_catalog_encryption_settings.current` data source.
- [ ] T050 Edit `test/standalone_test.go` and `examples/standalone-tenant` so the standalone tenant
  plan still matches the platform-applied plan with the new contract fields
  (`TestStandaloneTenantMatchesPlatformApplied`). Add a standalone `shares` rejection test
  against an empty contract `consumer_accounts`.
- [ ] T051 Add a Makefile target `test-account-settings` in `Makefile`, modeled on
  `test-tenancy`, running `-run 'TestOwner|TestDataLake|TestCatalog|TestKey|TestNoCatalog|TestShare|TestNoConsumer|TestRemoving|TestDesignatedTenantShare'`.
  Add it to the `help` output.
- [ ] T052 Run `make test`. This is the examples and tests checkpoint (use single targets for
  reruns).

**Checkpoint**: Every FR in this spec has a plan-time contract test.

---

## Phase 7: Verification Harness (`verify/`, FR-020)

**Purpose**: Extend `lfverify` (spec 002) so sandbox runs prove the controls.

- [ ] T053 [P] Edit `verify/fixture/personas.tf` to add persona `bypass`:
  - an IAM role trusted like the others, with an inline policy allowing `s3tables:*` and
    `s3:GetObject`/`s3:PutObject` on the fixture zone buckets, plus Athena, Glue read, and
    `lakeformation:GetDataAccess`;
  - **no** Lake Formation grants.

  Add it to `verify/fixture/outputs.tf` and to the persona list in `verify/internal/manifest`.
- [ ] T054 Edit `verify/internal/ops/storage.go` to add `StorageWrite` (an S3 `PutObject` of a
  small object under the table's warehouse location, cleaned up on success). Add the
  `matrix.StorageWrite` op in `verify/internal/matrix/matrix.go` and dispatch it in
  `verify/internal/runner/dispatch.go`. Add unit tests in `verify/internal/ops/storage_test.go`.
- [ ] T055 Add ops `AccountSettings` and `CatalogEncryption` to `verify/internal/ops/catalog.go`,
  with new matrix ops `account_settings` and `catalog_encryption` and dispatch cases:
  - `AccountSettings` reads the data-lake settings and compares every FR-003 field and the
    `CROSS_ACCOUNT_VERSION` parameter against expectations in the case's `expect_values`
    (add that matrix field).
  - `CatalogEncryption` reads `GetDataCatalogEncryptionSettings` and compares the mode and key.
  - Add `CatalogResourcePolicy`, which reads `glue:GetResourcePolicy` and fails when a policy
    exists without a `glue:ShareResource` statement for `ram.amazonaws.com` (D1).
  - Add unit tests with fake clients, following the existing pattern.
- [ ] T056 Edit `verify/matrix/full.yaml` and `verify/matrix/probe.example.yaml` to add FR-020
  cases 1–5:
  - `bypass-storage-read` (denied), `bypass-storage-write` (denied), and `bypass-select` (denied)
    for persona `bypass`;
  - `admin-account-settings`, `admin-catalog-encryption`, and `admin-catalog-resource-policy`
    (expect match);
  - `admin-no-iam-allowed-principals` (`list_permissions` with `IAM_ALLOWED_PRINCIPALS` absent on
    platform databases and the catalog).

  Cite FR-020 and the spec in `rule`. In the probe example, include only the non-destructive cases.
- [ ] T057 Add the consumer account to the fixture:
  - **Provider**: add `provider "aws" { alias = "consumer" }` in `verify/fixture/providers.tf`,
    assuming `var.consumer_role_arn` (new variable in `verify/fixture/variables.tf`, default `""`
    meaning sharing cases are off).
  - **`verify/fixture/sharing.tf`**:
    - add a share from tenant `alpha` (consumption zone, `public` and `internal`) to the consumer
      account via the module's `tenants` input in `verify/fixture/platform.tf`;
    - in the consumer account, create a resource-link database to each shared catalog database, a
      consumer persona role, and an LF grant on the link.
  - **Gating**: everything is gated on `var.consumer_role_arn != ""`.
- [ ] T058 Wire the long-lived catalog key into the fixture (FR-013, U2):
  - Add `variable "catalog_kms_key_arn"` (required, no default) to `verify/fixture/variables.tf`.
    Validate it as a KMS key or alias ARN.
  - Pass it as `account_settings_config.catalog_encryption.kms_key_arn` in
    `verify/fixture/platform.tf`. Add `variable "catalog_key_access_mode"` (default
    `service_role`) and pass it as the key-access mode.
  - In `verify/fixture/personas.tf`, when the mode is `per_caller`, add an inline policy granting
    `kms:Encrypt`, `kms:Decrypt`, `kms:GenerateDataKey`, `kms:DescribeKey`, and `kms:CreateGrant`
    on the key, conditioned on `kms:EncryptionContext:glue_catalog_id`, to every persona and to the
    lfverify runner role. The key policy delegates to IAM (T009).
  - In `verify/fixture/platform.tf`, add the lfverify runner role (`local.runner_role_arn` or
    equivalent) to `governance_config.direct_access_exempt_principal_arns`. lfverify setup and
    cleanup call `s3tables:DeleteTable` directly, which the deny otherwise blocks (T005,
    sandbox-results.md item 4). Persona roles stay non-exempt.
  - In the `Makefile` `verify-up`, `verify-apply`, and `verify-down` targets, pass
    `LFVERIFY_CATALOG_KEY_ARN` and `CATALOG_KEY_ACCESS_MODE`. Fail early with a clear message when
    `LFVERIFY_CATALOG_KEY_ARN` is unset.
- [ ] T059 Edit `verify/internal/preflight/preflight.go` to:
  - check the consumer account against `verify/sandbox-accounts.txt` when sharing cases are on;
  - accept pending RAM invitations named `LakeFormation*` in the consumer account when the
    consumer is outside the organization;
  - fail the run with "sharing cases not run" when `LFVERIFY_REQUIRE_SHARING=true` and no consumer
    is configured (used for release gating).

  Add tests in `verify/internal/preflight/preflight_test.go`.
- [ ] T060 Edit `verify/matrix/full.yaml` to add persona `consumer` with FR-020 cases 6–10:
  - `consumer-select-shared` (allowed, columns and rows within the shared labels);
  - `consumer-select-unshared-sensitivity`, `consumer-select-regulated`, `consumer-other-zone`, and
    `consumer-other-tenant` (denied);
  - `consumer-insert` and `consumer-drop` (denied);
  - `consumer-storage-read` (denied);
  - `consumer-revoked`: a scenario-phase case with a setup that revokes the share's grants through
    the admin and then expects denied, with cleanup restoring them.

  Mark the persona `requires: consumer` (add that matrix field) so it is skipped with a reported
  reason when no consumer is configured.
- [ ] T061 Edit the `Makefile` `verify-full` and `verify-up` targets to pass
  `CONSUMER_ROLE_ARN` through to the fixture and to set `LFVERIFY_REQUIRE_SHARING` when
  `RELEASE=true`. Document the variables in `verify/README.md`.
- [ ] T062 Run `make test-verify` and `make test-verify-fixture`. This is the harness checkpoint
  (offline and plan-only).
- [ ] T063 Run `make verify-full` in the sandbox, once in each key-access mode
  (`CATALOG_KEY_ACCESS_MODE`), with `LFVERIFY_CATALOG_KEY_ARN` set (T009). Then check the mode
  switch (FR-010, C2):
  - run `make verify-up verify-seed` with `CATALOG_KEY_ACCESS_MODE=service_role`;
  - switch to `per_caller` with one `make verify-apply`, and confirm the plan replaces no
    `aws_lakeformation_permissions` resources;
  - run `make verify-run` and require every case to pass;
  - switch back the same way, then `make verify-down`.

  Also confirm that an S3 Tables maintenance job completes successfully on a fixture table in a
  bucket that has `DenyDirectTableAccess` (`aws s3tables get-table-maintenance-job-status`), and
  record the timestamp. This was not observed in T005.

  Add `CONSUMER_ROLE_ARN` once T008 is done. Record the results, the latency comparison from T006, and
  the revocation time for `consumer-revoked` (SLO: 15 minutes or less) in
  `specs/003-harden-catalog-settings/sandbox-results.md`. Then run the T007 checks if they are
  still open, and update D3 in `plan.md` and FR-026 in `spec.md` if the service-role consumer grant
  turns out to be unnecessary.
- [ ] T064 Add and run an ephemeral upgrade test (SLO Migration, C1):
  - Add a `verify-upgrade` target to the `Makefile` that:
    1. creates a temporary git worktree of tag `v0.3.1` in the scratch directory;
    2. applies `verify/fixture` from that worktree (stage 1 and seed) with a new run ID;
    3. switches the fixture's module source back to this branch and runs one apply with
       `-detailed-exitcode`;
    4. fails if the plan shows any `aws_lakeformation_permissions` replace or delete actions, or
       if the data-lake settings are anything but moved and updated in place;
    5. runs `make verify-run` with the 002 matrix and requires every case to pass;
    6. always runs `make verify-down` and removes the worktree.

    Fixture files that differ between `v0.3.1` and this branch come from the worktree for steps 1
    and 2.
  - Run it in the sandbox and record the plan summary and results in
    `specs/003-harden-catalog-settings/sandbox-results.md`.
  - This is the only backward-compatibility guarantee. No production installations exist (spec,
    Assumptions).
- [ ] T065 Run `make verify-detect` and confirm that the new cases fail under an injected
  misconfiguration. Add an `inject` value `bypass_exempt`, which adds the `bypass` persona to
  `direct_access_exempt_principal_arns`, in `verify/fixture/inject.tf` and
  `verify/fixture/variables.tf`.

**Checkpoint**: The sandbox proves the bypass deny, the settings, encryption in both modes, and,
once the consumer exists, sharing and revocation.

---

## Phase N: Polish and Cross-Cutting Concerns

- [ ] T066 [P] Edit `README.md` (hand-written sections around the generated docs) to add:
  - **Account settings ownership**: the owner flag, the four combinations, and the hand-over
    procedure using `removed { from = module.<name>.module.account_settings[0] lifecycle { destroy
    = false } }`. Include the behavior observed in T004.
  - **Migrating catalog encryption** (FR-014, R4): the `aws/glue` warning, the asynchronous
    enablement, and a procedure to re-encrypt existing platform databases and tables by
    rewriting them, for example `aws glue update-database` and `update-table` with unchanged
    input.
  - **SageMaker Unified Studio**: which roles go in `integration_principal_arns`, and the explicit
    `data_lake` settings that IAM-based domains and EMR fine-grained access need (R8), with their
    security consequences.
  - **Cross-account sharing**: the approved accounts, tenant `shares`, consumer onboarding (accept
    RAM invitations outside the organization, create a resource link, grant locally) using the
    `shares` output, and revocation.
  - **Direct storage access**: the deny, the exemptions, and the recommended organization SCP
    (documented only).
  - **Catalog resource policy**: check for an existing policy before enabling sharing (D1).
- [ ] T067 [P] Edit `AGENTS.md`:
  - **Components**: add `modules/account-settings` and `modules/tenant/sharing.tf`, and replace
    "`catalog_integration_config.enabled` owns the data-lake settings" with the owner flag.
  - **Mirroring**: apply the same edits to `CLAUDE.md` where it repeats `AGENTS.md` content (the
    beads note says the two files are independent).
- [ ] T068 [P] Edit `modules/account-settings/README.md` (terraform-docs markers plus a short
  purpose section) and `modules/catalog-integration/README.md` (data-lake settings moved out).
- [ ] T069 [P] Edit `specs/001-governed-lakehouse/spec.md`, *Out of Scope*: change the
  cross-account bullet to read "Cross-region data sharing and replication (cross-account sharing
  within the Region is specified in 003-harden-catalog-settings)".
- [ ] T070 Run `make security-scan`. Fix or suppress any new Checkov findings with a justification
  comment. The expected suppression is for `*` principals on Deny-only statements, which already
  follows the existing bucket-policy pattern.
- [ ] T071 Run `make pre-commit` and then `make validate`. This is the final checkpoint.
- [ ] T072 Walk through the spec's Success Criteria checklist against the test and sandbox
  results, and record any criterion that is not met as a beads issue (`bd create`). Leave the
  checkbox unticked if any criterion is blocked by the missing consumer account.

---

## Dependencies and Execution Order

### Phase Dependencies

- **Phase 1** (Sandbox and Setup): T002–T006 must finish before T017–T020 and T029, because their
  results set the casing, trust conditions, and deny actions. T007 waits for T008 and only gates
  T036 and T063. T009 (long-lived sandbox catalog key) must finish before T058 and any fixture
  apply.
- **Phase 2** (Foundation) depends on T010–T014 and **blocks Phase 3**, because the key policy
  references the account-settings role output.
- **Phase 3** (Platform Data) depends on Phase 2. The contract fields (T030) **block Phase 4**.
- **Phase 4** (Tenant) depends on T030.
- **Phase 5** (Operations) depends on Phases 2–4.
- **Phase 6** (Examples and Tests) depends on Phase 5.
- **Phase 7** (Harness) depends on Phase 6. The fixture applies the module.
- **Phase N** depends on all phases. T069 can be done at any time.

### Resource Dependency Order (at apply time)

1. Lake Formation data-access role (catalog-integration), then the catalog-encryption role
   (account-settings, service-role mode).
2. Platform KMS key and its policy, unchanged by this feature (table and results encryption only).
   Then the dedicated catalog key in `modules/account-settings`. Its policy references the
   catalog-encryption role and the per-caller key users. No consumer account appears in any key
   policy.
3. Data-lake settings: admins including studio principals, explicit fields, and parameters. On
   deployments that don't own the settings, the settings guard runs instead.
4. Glue catalog encryption settings, then the optional Glue resource policy.
5. Lake Formation registration and `s3tablescatalog`, after `settings_ready`.
6. Platform LF-tags, then the zone buckets and their policies (with `DenyDirectTableAccess`).
7. Per tenant:
   1. dedicated buckets and their deny;
   2. tags;
   3. namespaces;
   4. database tags;
   5. grants;
   6. share guard and share grants.

   On destroy, share grants are revoked before tags and namespaces.

### Task-Level Dependencies

- **`variables.tf` edits**: T010 → T011 → T012 → T013 run sequentially in one file. T014 depends
  on all four.
- **Account-settings submodule**: T016 → T017 → T018 → T019 → T020 → T021 → T022.
- **Wiring**: T023 depends on T017. T024 depends on T022 and T023. T025 depends on T024.
- **Catalog key**: T019 depends on T018 (role) and precedes T020 (encryption settings). T027 and
  T028 depend on T019 and T024. T029 is in a different file and runs in parallel with T027.
- **Tenant**: T032 depends on T030. T033 → T034 → T035 → T036 → T037.
- **Tests**: T045–T050 depend on T043 and T044. T051 depends on T045 and T046.
- **Harness**: T053, T054, and T055 run in parallel. T056 depends on T053–T055. T057 → T058 → T059
  → T060 → T061. T058 depends on T009. T063 depends on T062 and T058, and on T008 for the sharing
  cases. T064 (upgrade test) depends on T063. T065 depends on T063.

### Parallel Opportunities

- **Phase 1**: T008 and T009 can run with T002–T006. T008 is an account provisioning request,
  and T009 is a one-time CLI key creation.
- **Phase 2**: T016 scaffolding can run while T010–T014 are edited. They are different files.
- **Phase 3**: T029 (`table-buckets.tf`) can run with T027 and T028 (`kms.tf`, `main.tf`,
  outputs).
- **Phase 4**: T032 (`storage.tf`) can run with T033 (`locals.tf`).
- **Phase 5**: T039 (`modules/deployer`) can run with T040 (`modules/cost-estimation`).
- **Phase 6**: T043 and T044 (separate example directories) can run in parallel, then T045 and T046
  (new test files).
- **Phase 7**: T053, T054, and T055 can run in parallel. They touch the fixture, `ops/storage.go`,
  and `ops/catalog.go`.
- **Phase N**: T066, T067, T068, and T069 are independent documents.

### Validation Checkpoints

- T015, after Setup: `make check`.
- T026, after Foundation: `make check`, `make lint`.
- T031, after Platform Data: `make check`, `make lint`.
- T038, after Tenant: `make check`, `make lint`.
- T042, after Operations: `make check`, `make lint`, `make docs`.
- T052, after Examples and Tests: `make test`.
- T062, T063, and T064, after the Harness: `make test-verify`, `make test-verify-fixture`,
  `make verify-full` in both modes plus the mode switch, and `make verify-upgrade`.
- T070 and T071, after Polish: `make security-scan`, `make pre-commit`, `make validate`.

### Environment Order

The module ships one release, but it must be proven in this order:

1. sandbox `492637581429` full mode and mode switch (T063), then the ephemeral `v0.3.1` upgrade
   (T064);
2. consumer-account sharing cases once T008 exists;
3. consumers' Development environments;
4. UAT and Production. Probe mode can run against these without changes.

The irreversible account-wide changes (version pin, encryption enablement) must be applied first
in a non-production account.

## Notes

- `[P]` tasks touch different files and don't depend on each other.
- T007, T036's conditional branch, and the sharing half of T063 are blocked on a second sandbox
  account (T008). The rest of the feature can ship to review without them, but the release gate
  (`RELEASE=true`) requires them.
- Use the `iac-tasks-to-beads` skill to load this file into beads before starting implementation.
