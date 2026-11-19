# Sandbox Results: Hardened Catalog Settings (plan T-S1)

Sandbox account `492637581429` (us-east-1, profile `dev-green`). Each section records one plan
T-S1 item: the command, what was observed, the date, and the design impact. Items are filled in by
tasks T002 to T007 and T063.

## 1. Live cross-account version and `SET_CONTEXT` casing (T002)

- **Command**: `aws lakeformation get-data-lake-settings --region us-east-1`,
  `aws glue get-data-catalog-encryption-settings --region us-east-1`, and
  `aws glue get-resource-policy --region us-east-1`
- **Observed** (no lakehouse fixture deployed at the time):
  - `Parameters`: `{ "CROSS_ACCOUNT_VERSION": "1", "SET_CONTEXT": "TRUE" }`
  - `DataLakeAdmins`, `ReadOnlyAdmins`, `CreateDatabaseDefaultPermissions`,
    `CreateTableDefaultPermissions`, `TrustedResourceOwners` and `ExternalDataFilteringAllowList`
    are all empty.
  - `AllowExternalDataFiltering`: `false`. `AllowFullTableExternalDataAccess` and
    `AuthorizedSessionTagValueList` are absent from the response, which means false and empty.
  - Catalog encryption: `CatalogEncryptionMode = DISABLED`,
    `ReturnConnectionPasswordEncrypted = false`.
  - Glue resource policy: `EntityNotFoundException` (none exists).
- **Date**: 2026-11-18
- **Design impact**:
  - **`SET_CONTEXT`**: the key and value are both uppercase (`"SET_CONTEXT" = "TRUE"`), as the
    plan assumed. The module uses that exact form.
  - **Cross-account version**: the sandbox is on version **1**. The first owner apply raises it to
    4, which changes account-wide state. AWS documents only the upgrade to 5 as irreversible.
    Assume going back from 4 to 1 is not possible. **Upgraded to 4 on 2026-11-18 during T007**
    (user-approved), and the sandbox stays on 4.
  - **Live-version precondition**: the check that the live version is not above the configured
    one passes on the first apply, because 1 < 4.
  - **Encryption migration guard (R4)**: the starting state is `DISABLED`, not `aws/glue`, so this
    guard doesn't fire in the sandbox.
  - **Catalog resource policy (D1)**: no unmanaged policy exists, so it doesn't affect sharing here.

## 2. Provider delete behavior (T004)

- **Command**: scratch configuration (local state, outside the module) with
  `aws_lakeformation_data_lake_settings` (admins = deployer SSO role,
  `parameters = { CROSS_ACCOUNT_VERSION = "1", SET_CONTEXT = "TRUE" }`), a Glue catalog-encryption
  role, and `aws_glue_data_catalog_encryption_settings` (`SSE-KMS-WITH-SERVICE-ROLE` with the T009
  key, connection passwords encrypted). Ran `terraform apply`, created a probe database, ran
  `terraform destroy`, then read back both settings and the probe database.
- **Observed**:
  - **Encryption settings**: destroying `aws_glue_data_catalog_encryption_settings` **resets the
    catalog to `DISABLED`** and sets `ReturnConnectionPasswordEncrypted = false`. It is not a no-op.
  - **Data-lake settings**: destroying `aws_lakeformation_data_lake_settings` **empties
    `DataLakeAdmins`**. `Parameters` (`CROSS_ACCOUNT_VERSION`, `SET_CONTEXT`), the empty default
    permissions, and the other fields keep their values. Destroy doesn't restore
    `IAM_ALLOWED_PRINCIPALS` defaults.
  - **Probe database**: written while encryption was on, it is **still readable** after destroy
    disabled encryption and deleted the role, through the deployer's key access via account-root
    delegation. Existing objects stay encrypted with the key after the mode changes.
  - **First apply**: creating the role and the encryption settings in one apply fails with
    `InvalidInputException: IAM Role … is not assumable by Glue. Please ensure that the Trust Policy
    for the role is correct.` (IAM propagation). A second apply succeeds. Reproduced twice.
- **Date**: 2026-11-18
- **Design impact**:
  - **Ownership hand-over (FR-001)**: destroying the account-settings submodule disables catalog
    encryption and removes every data-lake admin. That breaks every other deployment's grants until
    a new owner applies. The README hand-over procedure MUST use
    `removed { lifecycle { destroy = false } }` for `module.account_settings`, and MUST warn that
    `owner = false` without it disables encryption and empties the admins.
  - **Key protection (FR-013)**: confirmed that objects outlive the encryption setting, so the
    catalog key must outlive every deployment. This supports the absolute deletion deny.
  - **Role propagation (T018–T020)**: the account-settings submodule needs a delay between the
    catalog-encryption role policy and `aws_glue_data_catalog_encryption_settings`. Use
    `time_sleep` (`hashicorp/time`, `create_duration = "20s"`), triggered by the role ARN and the
    policy, in service-role mode only. This adds the `hashicorp/time` provider to the module's
    `required_providers`.

## 3. Catalog-encryption role trust conditions (T004)

- **Command**: same scratch configuration. The role trust allowed `glue.amazonaws.com`
  `sts:AssumeRole` with (a) `StringEquals aws:SourceAccount = 492637581429`, then (b) (a) plus
  `ArnLike aws:SourceArn = arn:aws:glue:us-east-1:492637581429:catalog`. Ran
  `aws glue create-database` and `get-database` under each. Inspected CloudTrail `AssumeRole`
  events invoked by `glue.amazonaws.com`.
- **Observed**:
  - **(a) `aws:SourceAccount` only**: create and read both succeed.
  - **(b) with `aws:SourceArn`**: every catalog call fails with `GlueEncryptionException: The
    required permissions to access this catalog item does not exist or is incorrect`. That includes
    reading the database written under (a), and it persisted for more than 60 seconds. Reverting to
    (a) restored access within about 10 seconds.
  - **CloudTrail**: Glue assumes the role with session names
    `V_00_GLUE_KMS_GENERATE_DATA_KEY_<account>` and `V_00_GLUE_KMS_DECRYPT_<account>`, and a session
    policy that narrows each session to the single KMS action. No source ARN that matches the
    catalog is supplied.
- **Date**: 2026-11-18
- **Design impact**:
  - **Trust**: the catalog-encryption role trust uses **`aws:SourceAccount` only** (T018). Adding
    `aws:SourceArn` with the catalog ARN breaks catalog access.
  - **FR-011**: the "account and region" condition can't include Region, the same situation as D2.
    Recorded as unsupported (analysis finding I5).
  - **Role policy**: Glue uses only `kms:GenerateDataKey` and `kms:Decrypt` through the role. The
    role policy keeps `kms:Encrypt` because the AWS documentation lists it, but it was unused in
    this test.

## 4. Governed access with `DenyDirectTableAccess` (T005)

- **Command**: deployed and seeded the 001/002 fixture (run `38b08e`, `make verify-up
  verify-seed`), then ran the full 222-case matrix (`make verify-run`) as a baseline. Next, added
  the planned `DenyDirectTableAccess` statement by hand to the `lfv-38b08e-curated` table-bucket
  policy with `aws s3tables put-table-bucket-policy`:
  - **Effect and principal**: `Deny`, principal `*`.
  - **Actions**: `s3tables:GetTableData`, `PutTableData`, `CreateTable`, `DeleteTable`,
    `RenameTable`, `UpdateTableMetadataLocation` and `GetTableMetadataLocation`.
  - **Conditions**: `ArnNotEquals aws:PrincipalArn` set to the fixture's Lake Formation data-access
    role, and `Bool aws:PrincipalIsAWSService = false`.

  Then probed direct access as the deployer's SSO admin role (IAM `s3tables:*`, no Lake Formation
  grant) and reran the matrix. Teardown: `make verify-down`.
- **Observed**:
  - **Baseline** (no deny): 222/222 pass. Without the deny, the admin role reads the table's Iceberg
    metadata file directly with `s3api get-object` (2,125 bytes). This is the bypass.
  - **Direct access under the deny**: `GetTableMetadataLocation` is denied with `explicit deny in a
    resource-based policy`, and S3 `GetObject` of that same real metadata key and `PutObject` under
    the warehouse location both return `AccessDenied`. `GetTable` and `ListTables`, which aren't in
    the deny, still return names and the warehouse location, but no data.
  - **First matrix rerun under the deny**: 218 pass. Three cases (`mw-alter-scratch` label and
    named, `mw-drop-scratch/label`) failed with `Insufficient Lake Formation permission(s):
    Required Describe on scratch`, and one was not run because of a TLS timeout to Athena. These
    failures **were not caused by the deny**. The named-style bucket has no deny, and the cause is
    fixture state: the baseline run's final scenario drops and recreates `scratch`, which loses the
    table-level labels and grants applied by Terraform at stage 2. So a second matrix run on one
    fixture needs `make verify-apply` first (a limitation of the 002 harness).
  - **Harness cleanup is denied**: lfverify's own cleanup and undo steps call
    `s3tables:DeleteTable` directly as the runner, so they were denied and left two tables behind
    (`late_table`, `lfv_ct_meta_writer`).
  - **Clean rerun**: after `make verify-apply`, deleting the leftovers, and re-applying the deny with
    the runner role added to the exemption list: **222/222 pass**. Every persona's governed
    operations work under the deny, including Athena `CREATE TABLE`, `INSERT`, `ALTER TABLE`,
    `DROP TABLE` and `SELECT`, plus every existing storage-bypass case. Lake Formation and Athena
    perform the S3 Tables calls through the data-access role or an AWS service principal, never
    as the caller.
  - **Maintenance**: no S3 Tables maintenance job ran during the roughly 30 minutes the deny was in
    place (the last runs were before it), so maintenance under the deny is **not yet confirmed**.
- **Date**: 2026-11-18
- **Design impact**:
  - **Deny actions**: the action list in plan.md and T029 holds as written. No governed operation
    needed an extra exemption.
  - **Fixture runner exemption (T058)**: the fixture MUST add the lfverify runner role to
    `governance_config.direct_access_exempt_principal_arns`, or lfverify setup and cleanup must
    switch to governed calls. The exemption is simpler, and it doesn't weaken the persona cases,
    which run as non-exempt persona roles.
  - **Maintenance check (T063)**: confirm that a maintenance job (compaction, snapshot management,
    or unreferenced-file removal) completes on a table in a bucket that has the deny, and record
    the timestamp.
  - **Rerunning the matrix**: a second `make verify-run` on one fixture needs `make verify-apply`
    first. T063's mode-switch sequence already re-applies between runs.

## 5. Cross-account LF-tag grant on `s3tablescatalog` (T007)

- **Command**: deployed and seeded the fixture in the producer (`492637581429`, run `124b34`), then
  raised its `CROSS_ACCOUNT_VERSION` from 1 to **4**. The upgrade was user-approved, made with a
  full-settings `PutDataLakeSettings` call, and is permanent. Then, from the producer to consumer
  account `566376086856` (`dev-blue`, same organization `o-kmnavciu4n`), made grants shaped like
  T035:
  - **Database**: `DESCRIBE`, with grant option, on `tenant-alpha=member, zone=curated`.
  - **Table**: `SELECT, DESCRIBE`, with grant option, on `tenant-alpha=member, zone=curated,
    sensitivity in (public, internal)` and `reg-*=no`.

  The consumer setup:
  - temporarily added the consumer's SSO admin role as a data-lake admin (settings snapshotted,
    then restored exactly);
  - created resource link `t007_alpha_link` to `492637581429:s3tablescatalog/lfv-124b34-curated`
    / `alpha`, and queried it through Athena.

  For D3, enabled `SSE-KMS-WITH-SERVICE-ROLE` in the producer with the T009 key (whose policy
  doesn't name the consumer), created an S3 Tables table and a Glue-native database under
  encryption, queried from the consumer, and inspected KMS CloudTrail events in both accounts.
- **Observed**:
  - **Shares**: both grants succeeded. Lake Formation created two RAM shares
    (`LakeFormation-V4-…`, `allowExternalPrincipals = false`). The consumer, in the same
    organization, received them with **no invitation to accept**.
  - **Consumer query results**:
    - `customers` (50 rows) and `scratch` (5) are readable.
    - `orders`, labelled `restricted` at the table but with `internal` columns, is readable **only
      for its `internal` columns**. `SELECT ssn` fails with `COLUMN_NOT_FOUND … not authorized`, and
      `SELECT *` omits `ssn`.
    - Tenant `beta` and zone `raw` return `AccessDeniedException`. Cross-account column-level label
      enforcement works.
  - **Consumer metadata visibility**: the consumer admin can `GetTable` and list every table in the
    shared database, including `orders`, with names and schema. The data stays label-filtered.
  - **Re-granting by tag (T036)**: the consumer admin's LF-tag-expression grant to its own role
    **failed** (`AccessDeniedException`) until the producer granted the consumer account
    **`DESCRIBE` (with grant option) on the LF-tag keys and values in the expression**. Granting
    `GRANT_WITH_LF_TAG_EXPRESSION` cross-account is rejected: `Granting permission
    MANAGE_TAG_POLICY is not supported cross account`.
  - **Catalog key (D3)**: with service-role encryption on, Glue made KMS calls (`GenerateDataKey`,
    `Decrypt` with `glue_catalog_id`) **only for the Glue-native control database**. It made none
    for creating the S3 Tables table and none for the consumer's queries, and the consumer account
    made no use of the key. The consumer read the table created under encryption (`t007_enc`)
    and the older tables with **no consumer grant on the catalog key**.
  - **Revocation**: revoking the four cross-account grants moved both RAM shares to `DELETED`
    immediately.
  - **Studio admin roles**: the consumer account hosts a SageMaker Unified Studio domain. Its
    data-lake admins are `AmazonSageMakerProvisioning-566376086856` **and**
    `AmazonSageMakerManageAccess-us-east-1-dzd_…`, and it still has `IAM_ALLOWED_PRINCIPALS`
    default permissions.
- **Date**: 2026-11-18
- **Design impact**:
  - **Sharing approach confirmed**: LF-tag expression grants to an account ID on `s3tablescatalog`
    objects, with grant option, work on version 4 and automatically create and delete
    organization-only RAM shares (FR-021, FR-024, FR-025).
  - **T036 branch needed**: add `share_tag_describe`, granting `DESCRIBE` with grant option (and
    only `DESCRIBE`) on the tenant tag and each platform tag key and value in the share expression
    to every consumer account.
  - **D3 / FR-026**: Glue catalog encryption doesn't cover `s3tablescatalog` (federated S3 Tables)
    metadata, which S3 Tables stores under the table bucket's key. Consumers of S3 Tables shares
    need **no** catalog-key grant in either mode. Drop `AllowSharingConsumersCatalogDecrypt`
    (pending the FR-026 and FR-009 decision).
  - **FR-009 scope**: the dedicated catalog key protects only Glue-native catalog objects (default
    catalog databases, resource links, connections), not the platform's S3 Tables schemas. The
    spec should state this.
  - **Studio integration principals**: `studio_config.integration_principal_arns` documentation and
    examples should list both the provisioning role and the domain's
    `AmazonSageMakerManageAccess-<region>-<domain>` role (R8).
  - **Consumer metadata visibility**: a consumer administrator can see the names and schemas of
    every table in a shared database, not only the label-matching ones. Data access is still
    label- and column-filtered. Note this in the README sharing section.

## 6. Catalog lookup latency before and after encryption (T006)

- **Command**: a Go timing probe (AWS SDK for Go v2, the same versions as `verify/`) that makes
  one warm-up `glue:GetTable` call and then times 20 calls, reporting the median. It ran from the
  developer workstation against scratch database `t006_probe`, comparing:
  - `plain_tbl`, written with encryption `DISABLED`;
  - `enc_tbl`, written under `SSE-KMS-WITH-SERVICE-ROLE` with the T009 key;
  - `caller_tbl`, written under `SSE-KMS` (per-caller).

  Runs were interleaved so network drift affects both sides equally. Afterwards the scratch objects
  were deleted, the settings destroyed, and the T002 baseline confirmed.
- **Observed** (medians of 20 calls per run):
  - **Service-role mode**, 5 interleaved runs: unencrypted 145–155 ms (mean 150.5 ms); encrypted
    188–218 ms (mean 202.1 ms). About **+52 ms (+34%)**.
  - **Per-caller mode**, 4 interleaved runs: unencrypted 113–128 ms (mean 121.5 ms); encrypted
    148–160 ms (mean 155.8 ms). About **+34 ms (+28%)**.
  - Reading an unencrypted object stays at baseline after encryption is enabled. Only objects
    written under encryption pay the decrypt cost.
- **Date**: 2026-11-18
- **Design impact**:
  - **NFR not met**: the Performance NFR and SLO "median catalog lookup latency increases by at
    most 10%" are **not met** in either key-access mode. The cost is a roughly fixed 35–50 ms per
    lookup: an extra KMS decrypt, plus a role assumption in service-role mode.
  - **Query-level effect**: Athena query planning makes a handful of catalog lookups per query, so
    the end-to-end effect is likely well under 10% of a governed query that takes seconds. That
    has not been measured.
  - **Decision (2026-11-19)**: the spec now states an absolute budget of at most 75 ms added per
    lookup in the default service-role mode, which the measured +52 ms meets. Service-role stays
    the default (`awslkhs-cnb7.1`).

## 7. Provider version floor (T003)

- **Command**: `terraform init` with `hashicorp/aws = 6.59.0` (the current floor) in a scratch
  directory, then `terraform providers schema -json`
- **Observed**: provider 6.59.0 already exposes:
  - `aws_glue_data_catalog_encryption_settings.data_catalog_encryption_settings.encryption_at_rest.catalog_encryption_service_role`;
  - every `aws_lakeformation_data_lake_settings` field the plan uses, including `parameters`,
    `allow_full_table_external_data_access`, `allow_external_data_filtering`,
    `external_data_filtering_allow_list`, `authorized_session_tag_value_list` and
    `trusted_resource_owners`;
  - `parameters` on the `aws_lakeformation_data_lake_settings` data source;
  - `enable_hybrid` on `aws_glue_resource_policy`.
- **Date**: 2026-11-18
- **Design impact**: no change. The constraint stays at `>= 6.59.0, < 7.0.0`.

## Full-mode runs (T063)

The fixture owns the account settings and supplies the long-lived catalog key
(`LFVERIFY_CATALOG_KEY_ARN`, T009). Producer `492637581429`, consumer `566376086856` (`dev-blue`),
us-east-1, 2026-11-18.

- **`bca1ea`** (service_role, no consumer): stage 1 applied. This was the first real apply of
  `modules/account-settings`, encryption and role delay included. The SSO session then expired,
  and the run was torn down after re-login.
- **`7d85f8`** (service_role, no consumer, 229 cases): 228 pass, 1 not run.
  `admin-no-iam-allowed-principals` didn't run because `ListPermissions` needs a resource when it
  filters by principal. Fixed to query each database and its tables.
- **`9850da`** (per_caller, no consumer, 229 cases): **229 pass**, teardown clean.
- **`dbc990`** (service_role, with consumer): the consumer admin was added, keeping the account's
  two SageMaker Unified Studio admins (3 admins in total). Stage 2 then failed: the resource link
  read back its target's `parameters` and `federated_database`, and updating them needs `ALTER` on
  the producer database. Fixed with `ignore_changes`. Teardown clean.
- **`73a09e`** (service_role, with consumer, `RELEASE=true`, 238 cases): **238 pass**, teardown
  clean in both accounts.
- **`4dd029`** (key-access mode switch, FR-010): deployed and seeded in service_role, then
  switched with one `make verify-apply` each way.
  - **service_role to per_caller**: 229 of 229 cases pass.
  - **back to service_role**: 229 of 229 cases pass. Teardown clean.
  - **What each switch changed in the module**: the encryption settings were updated in place, and
    the catalog-encryption role, its policy, and the propagation delay were removed and recreated.
    **No module grant (`aws_lakeformation_permissions`) was replaced or destroyed.**
  - **Unrelated replacements**: the fixture's own `owner_*` grants and the module's
    `resource_lf_tags.row_filtered_table` were replaced on every re-apply. That is a standing diff,
    unrelated to the mode switch and unchanged since v0.3.1, filed as `awslkhs-8oof`.

Observations:

- **Bypass deny (FR-015)**: `bypass-storage-read` and `bypass-storage-write` pass in every run, so
  a role with `s3tables:*` and S3 object access on table storage is refused, and `bypass-select`
  is denied by Lake Formation.
- **Account settings (FR-003, FR-005, FR-009)**: `admin-account-settings` and
  `admin-catalog-encryption` match in both modes: version 4, `SET_CONTEXT` on, every external
  engine setting off, no default permissions, the long-lived key, and encrypted connection
  passwords. No catalog resource policy exists, and no `IAM_ALLOWED_PRINCIPALS` grants exist on
  platform databases or tables.
- **Sharing (FR-021 to FR-025)**: in the consumer account, the persona reads the shared `customers`
  table (50 rows) through a resource link. The row-filtered `orders` table is not shared, the
  other tenant and zone are denied, direct storage reads are refused, inserts and drops are
  denied, and `consumer-revoked` is denied after `revoke_share`, within the 30-second scenario
  settle (SLO: 15 minutes).
- **Consumer re-grant by tag**: the fixture's consumer grants by LF-tag expression succeed, which
  confirms `share_tag_describe` (sandbox result 5) end to end.
- **Consumer cleanup**: after teardown, the consumer account has no `lfv_` databases or roles and
  no active RAM shares. Its admins are the two studio roles plus the lfverify consumer admin, which
  stays by design (`make verify-consumer-admin`).

### Phase 7 close-out (T063 maintenance, T064 upgrade, T065 detect)

- **`5a6451`** (`make verify-detect`, T065): all 20 overlays were detected, including the new
  `bypass_exempt` (2 failing cases: `bypass-storage-read` and `bypass-storage-write` succeed once
  the bypass persona is exempt from `DenyDirectTableAccess`). Teardown clean, settings restored.
- **`4c8b49`** (`make verify-upgrade` from `v0.3.1`, first attempt): `verify-upgrade-check` failed
  before the upgrade apply. The fixture's `module "lakehouse"` had `depends_on =
  [aws_iam_role.persona]`, and the upgrade adds the `bypass` persona, so every data source in the
  module was deferred. `account_id` became unknown and forced replacement of the s3tables catalog,
  its Lake Formation registration, the query-results bucket, data cells filters, and database
  labels. **This was a fixture defect, not a module upgrade defect.** Fix: the fixture builds
  persona ARNs from `aws_iam_role.persona[k].name` (known at plan) and drops the module-level
  `depends_on`. The check now scopes the grant rule to `module.lakehouse` (the fixture's own
  `owner_*` grants are the standing diff in `awslkhs-8oof`) and lists any other module
  replacement. Teardown clean.
- **`998be6`** (`make verify-upgrade` from `v0.3.1`, T064): **229 of 229 cases pass** after the
  upgrade, run against the current `full.yaml` (a superset of the 002 matrix). Teardown clean.
  - **Upgrade plan**: 29 to add, 4 to change, 19 to destroy.
    - The data-lake settings moved from `module.catalog_integration[0]` to
      `module.account_settings[0]` and were updated in place.
    - **No `module.lakehouse` `aws_lakeformation_permissions` resource was replaced or destroyed.**
    - Module replacements: only `row_filtered_table["reader_cells/curated/orders"]` and
      `["reader_rows/curated/orders"]` (the standing diff in `awslkhs-8oof`).
    - Fixture replacements: the `owner_*` grants and three fixture table labels (standing diff).
  - **Table maintenance behind the deny (T063)**: the upgrade apply added `DenyDirectTableAccess`
    to the zone buckets at about 04:08Z (2026-11-19). Afterwards, maintenance jobs reported
    `Successful` on the denied buckets (`get-table-maintenance-job-status`):
    - `icebergSnapshotManagement` on `curated` `alpha.customers` (04:09:38Z), `alpha.scratch`
      (04:10:02Z), `beta.scratch` (04:09:43Z), and `gamma.customers` (04:10:01Z);
    - `icebergUnreferencedFileRemoval` on `curated` `beta.customers` (04:10:08Z) and `raw`
      `alpha.orders` (04:09:42Z).

    The service principal exemption in the deny is sufficient for S3 Tables maintenance.
    Compaction last ran at 04:05Z, before the deny, and had no new data to compact afterwards.

### Success-criteria gap checks (T072)

All four checks ran on run **`6e9eff`** (2026-11-19): producer `492637581429`, `per_caller` mode, consumer
`566376086856`. Teardown was clean, and the settings were restored (catalog `DISABLED`).

1. **Sharing in `per_caller` mode**: `make verify-up verify-seed verify-run` with `RELEASE=true` passed
   **238 of 238 cases**, including all 9 consumer cases. Sharing is now verified in both key-access modes
   (service-role in `73a09e`).
2. **Re-encryption procedure** (README, "Migrating catalog encryption"): role `t072-nokey` had Glue read
   permissions and Lake Formation `DESCRIBE`, but no KMS permissions. In `per_caller` mode it can read only
   unencrypted catalog objects.
   - Table `t3` was created with encryption `DISABLED`. After the owner re-enabled encryption, `t072-nokey` still
     read `t3` (17:11:53Z), so existing objects stayed unencrypted.
   - After the README `update-table` rewrite, `t072-nokey` got `GlueEncryptionException` (`kms:Decrypt` denied
     on the catalog key) for `t3` (17:12:05Z). The rewrite re-encrypted it. The admin still read it.
   - Table `t2`, written under encryption, was denied throughout, which confirms that the probe distinguishes
     the two.
3. **Non-owner against a live owner**: a scratch copy of `examples/secondary` (`owner = false`) planned against
   the run.
   - With the run's data-access role: the plan **succeeded** (29 to add). `settings_guard` passed, and the plan
     only read the data-lake and encryption settings; it managed none.
   - With `studio_config` naming a role that is not an admin: **failed** at plan time with
     "studio_config.integration_principal_arns must already be data-lake admins".
4. **Out-of-band drift**: catalog encryption was set to `DISABLED` and `AllowFullTableExternalDataAccess` to
   `true` with the CLI.
   - The fixture plan reported "Objects have changed outside of Terraform" and planned in-place updates
     restoring `SSE-KMS` and encrypted connection passwords, and `allow_full_table_external_data_access` to
     `false`.
   - `make verify-apply` restored both.
   - An attempt to drift `AllowExternalDataFiltering` instead was rejected by the API ("Empty list is not
     allowed") because the allow list was empty.

### PR #9 review fixes

- **`70da1f`** (`make verify-full`, service_role, consumer, `RELEASE=true`, 2026-11-19): **238 of 238 cases
  pass** after the review fixes, with the platform now owning the consumer's tag `DESCRIBE` grants (six
  `consumer_tag_describe` grants) and tenants granting only their own keys. The consumer re-grant by tag
  succeeds. Teardown clean, settings restored.

### awslkhs-8oof: row-filter exclusion labels

- **Cause**: `aws_lakeformation_resource_lf_tags` (provider 6.73.0, unchanged on `main`) reads every tag assigned
  to the table and deletes every tag in its state. Every apply replaced it, and the destroy removed writer labels
  and the other readers' row-filter exclusion keys, so a filtered reader could transiently match the table with
  its tag grant.
- **Fix**: the module's `row_filtered_table` and the fixture's table and column labels use the singular
  `aws_lakeformation_resource_lf_tag`, which reads and removes only its own key. `removed` blocks with
  `destroy = false` forget the plural resources without calling their delete.
- **`9b5164`** (2026-11-19): deployed at `d5776cc` (plural), then upgraded in one apply.
  - The plan forgot every plural label resource ("will not be destroyed") and created the singular ones.
  - The `alpha/curated/orders` labels (sensitivity `restricted`, both readers' exclusion keys `yes`) were
    unchanged before the upgrade, after it, and after the matrix.
  - The re-plan shows no label changes. Its remaining replacements are the fixture's own `owner_*` and
    `consumer_link` grants (`awslkhs-c921`).
  - `verify-run` with the consumer: **238 of 238 pass**. Teardown clean.
