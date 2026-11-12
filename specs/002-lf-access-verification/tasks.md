---

description: "Implementation tasks for the Data Permission Access Verification Harness"
---

# Tasks: Data Permission Access Verification Harness

**Input**: Design documents from `/specs/002-lf-access-verification/`
**Prerequisites**: plan.md, spec.md (no research.md, architecture.md, modules.md, or quickstart.md)

**Organization**: Tasks are grouped by tier, following the dependency order Setup → Identity &
Module Foundation → Fixture Data & Grants → Application (the `lfverify` CLI) → Operations
(make targets, CI, sandbox runs) → Polish. The harness has **no network tier**: every call goes to
a Regional AWS public endpoint (plan.md, Networking). The phase that would hold networking instead
holds the module change and the persona IAM roles that every fixture grant depends on.

## Format: `[ID] [P?] Description`

- **[ID]**: Sequential task number (T001, T002, T003...)
- **[P]**: Can run in parallel (different files, no dependencies) - optional
- **Description**: Clear action with exact file path included

## Path Conventions

- The harness lives in `verify/` at the repository root (plan.md, Structure Decision). Do not use
  `iac/`.
  - `verify/fixture/` is a Terraform root configuration.
  - `verify/` is also a separate Go module, `github.com/kbrockhoff/terraform-aws-lakehouse/verify`.
- Module changes go in the root `variables.tf`, `main.tf`, and `modules/catalog-integration/`.
  Plan-only tests go in `test/`.
- **All validation runs through `make` targets** (principles, Validate Before Merge). Never invoke
  `terraform`, `tflint`, `terraform-docs`, or `go test` directly. New make targets are added in
  T005 and T045 before they are used.
- Every IAM policy uses `aws_iam_policy_document`, never inline JSON. Pass identifiers through
  resource attributes, module outputs, or variables. Never build ARNs by name convention.
- Every fixture resource name starts with `lfv-<run_id>` (`local.prefix`) and carries the
  `LfverifyRun` and `Purpose` default tags (FR-004).
- Go code: standard library first, `aws-sdk-go-v2` clients built from one `aws.Config`, and
  errors wrapped with `%w`. Never log or persist query row values (FR-030).
- Persona keys are used everywhere: in Terraform maps, the manifest, matrix files, and session
  names. The 13 keys are `admin`, `roadmin`, `meta_reader`, `meta_writer`, `reader_multi`,
  `reader_single`, `writer_multi`, `writer_single`, `reader_cols`, `reader_rows`,
  `reader_cells`, `control`, and `steward`.

---

## Phase 1: Setup

**Purpose**: Directory layout, Go module, Terraform root skeleton, and offline make targets.

- [x] T001 Create the `verify/` tree exactly as plan.md, Source Code, describes:
  - `verify/cmd/lfverify/`;
  - `verify/internal/{matrix,identity,ops,classify,predicate,runner,seed,inspect,snapshot,preflight,report}/`;
  - `verify/matrix/`;
  - `verify/fixture/`.

  Add `verify/out/` and `verify/fixture/.terraform*` to the root `.gitignore`.
- [x] T002 [P] Create `verify/go.mod`:
  - module `github.com/kbrockhoff/terraform-aws-lakehouse/verify`, `go 1.26.0`;
  - `aws-sdk-go-v2` core at the same version as `test/go.mod` (`v1.42.0`), with the service
    modules `sts`, `iam`, `glue`, `lakeformation`, `athena`, `s3`, `s3tables`, and `cloudtrail`
    at their latest versions compatible with that core;
  - `github.com/apache/iceberg-go` (latest release, exact pin), `github.com/apache/arrow-go/v18`
    (the version iceberg-go requires), and `go.yaml.in/yaml/v3`.

  Generate `verify/go.sum`.
- [x] T003 [P] Create `verify/sandbox-accounts.txt` with a header comment explaining the format
  (`<account-id> <region>` per line, `#` comments) and one entry: `492637581429 us-east-1`
  (dev-green).
- [x] T004 [P] Create the Terraform skeleton in `verify/fixture/`:
  - `versions.tf`: `required_version = ">= 1.11"`, `hashicorp/aws >= 6.59.0, < 7.0.0`.
  - `providers.tf`:
    - a default `aws` provider with `default_tags` of `LfverifyRun = var.run_id` and
      `Purpose = "access-verification"`;
    - an `aws.pricing` alias in `us-east-1`, which the module requires.
  - `variables.tf`, each with a `validation` block:
    - `run_id` (string, `^[0-9a-f]{6}$`);
    - `runner_role_arn` (IAM role ARN);
    - `stage` (number, 1 or 2, default 1);
    - `inject` (string, default `""`, one of `""`, `extra_grant`, `missing_grant`,
      `drop_column_restriction`, `drop_row_filter`, `open_default`);
    - `inject_persona` (string, default `""`, must be a persona key when `inject` is
      `extra_grant` or `missing_grant`);
    - `read_only_runner_arns` (list of IAM role ARNs, default `[]`), for the CI probe runner.
- [x] T005 Add offline targets to `Makefile`, and add `test-verify` to the `.PHONY` list:
  - `test-verify`: runs `cd verify && go vet ./... && go test ./...` without `check-aws`.
  - `test-verify-fixture`: depends on `check-aws` and runs `cd test && go test -run TestVerifyFixture -v -timeout 20m`.
  - Extend `check` to also run `terraform init -backend=false && terraform validate` in
    `verify/fixture`, with the same echo style as the examples loop.
  - Add `test-verify` to `validate` after `test`, and add `test-verify-fixture` to `test`.
- [x] T006 Checkpoint: run `make format` and `make check`. `verify/fixture` must validate with
  only variables declared. Run `make test-verify`, which passes with no packages yet.

**Checkpoint**: The project skeleton builds and validates.

---

## Phase 2: Identity & Module Foundation

**Purpose**: The module change that keeps read-only administrators (Complexity Tracking #1,
`awslkhs-t7d`), and the persona IAM roles that every grant depends on.

**⚠️ CRITICAL**: Phase 3 grants reference these roles and the new module input.

- [x] T007 Add `read_only_admin_principal_arns = optional(list(string), [])` to
  `governance_config` in the root `variables.tf`.
  - Add a `validation` block with the same IAM role or user ARN regex as `admin_principal_arns`,
    and the error message "Every read_only_admin_principal_arns entry must be an IAM role or user
    ARN."
  - Add a second validation that no ARN appears in both lists, with the error message "A
    principal cannot be both a data-lake admin and a read-only admin."
  - Update the variable `description` to mention read-only admins.
- [x] T008 [P] Add `variable "read_only_admin_principal_arns"` (`list(string)`, default `[]`, same
  validation) to `modules/catalog-integration/variables.tf`. In
  `modules/catalog-integration/main.tf`, set `read_only_admins = var.read_only_admin_principal_arns`
  on `aws_lakeformation_data_lake_settings.this`, with a comment that `PutDataLakeSettings`
  replaces the whole object, so omitting the field would erase existing read-only admins.
- [x] T009 Pass `read_only_admin_principal_arns = var.governance_config.read_only_admin_principal_arns`
  to `module "catalog_integration"` in the root `main.tf`. Depends on T007 and T008.
- [x] T010 [P] Add two validation cases to `test/validation_test.go`:
  - an invalid `read_only_admin_principal_arns` entry (`"not-an-arn"`);
  - an ARN in both lists.

  Each case asserts its error message. In `test/complete_test.go`, assert that the planned
  `aws_lakeformation_data_lake_settings` has `read_only_admins` equal to the configured list. Add
  one read-only admin placeholder ARN to `examples/complete/terraform.auto.tfvars` in account
  123456789012.
- [x] T011 [P] Document the input in `README.md` (via `make docs`) and
  `modules/catalog-integration/README.md`. Add a CHANGELOG-ready note in the commit message body
  (`feat: support Lake Formation read-only admins`) explaining that previously every apply
  removed read-only admins.
- [x] T012 Create `verify/fixture/locals.tf`:
  - `prefix = "lfv-${var.run_id}"`.
  - `personas`: a map keyed by the 13 persona keys. Each value is
    `{ kind = "admin"|"roadmin"|"meta"|"reader"|"writer"|"control"|"steward", tenants = [...], label_grants = bool }`,
    where `tenants` holds the label-style tenants the persona touches (plan.md, Data Storage):
    - multi personas: `["alpha","beta"]`;
    - single, column, row, and cell personas and `meta_writer`: `["alpha"]`;
    - `meta_reader`: `["alpha","beta"]`.
  - `tenant_names = ["alpha","beta","gamma"]`.
  - `tables`: the three fixture tables from plan.md (`orders`, `customers`, `scratch`) with
    columns, types, the partition column, and the `ssn` restricted column.
  - `named_namespaces = ["n_alpha","n_beta","n_gamma"]`.
- [x] T013 Create `verify/fixture/personas.tf`:
  - `aws_iam_role` for each persona, named `${local.prefix}-${key}`, with
    `max_session_duration = 3600`.
  - A trust policy, built with `aws_iam_policy_document`, that allows only
    `var.runner_role_arn` to call `sts:AssumeRole` and `sts:TagSession`, with the condition
    `StringLike sts:RoleSessionName = "lfverify-${var.run_id}-*"`.
  - Policies attached as `aws_iam_role_policy`, all built from `aws_iam_policy_document`:
    - **readers, writers, meta, control, and steward**: `source_policy_documents` of the module's
      `tenants[t].persona_policy_json` for each of the persona's tenants (for control, alpha's
      policy), plus `glue:GetCatalog*`, `glue:GetDatabase*`, `glue:GetTable*`,
      `glue:GetPartitions`, `glue:SearchTables`, and `lakeformation:GetDataAccess` on `*`. Writers
      and `meta_writer` also get `glue:CreateTable`, `glue:UpdateTable`, and `glue:DeleteTable`.
      All personas get Athena and results-bucket access to the baseline workgroup from T020.
    - **`admin`**: the managed policy `arn:${partition}:iam::aws:policy/AWSLakeFormationDataAdmin`,
      through `aws_iam_role_policy_attachment`, plus Athena on all fixture workgroups.
    - **`roadmin`**: the read-only administrator action list from the Lake Formation personas
      reference, as an allow statement, plus an explicit deny of
      `lakeformation:PutDataLakeSettings`.
    - **Storage bypass (FR-022)**: no persona policy may include `s3tables:GetTableData`,
      `s3tables:GetTableMetadataLocation`, `s3tables:PutTableData`, or `s3:GetObject` on table
      storage. Add a code comment stating this.
- [x] T014 Checkpoint: run `make pre-commit` and `make test-validation`. The new validation cases
  must pass.

**Checkpoint**: The module keeps read-only admins, and the persona roles are declared.

---

## Phase 3: Fixture Data & Grants

**Purpose**: Both grant styles in `verify/fixture/`: the label style (module under test plus
fixture-owned metadata grants) and the named-resource baseline, with the two-stage apply
(Complexity Tracking #2) and misconfiguration overlays.

**Dependencies**: Phase 2.

- [x] T015 Create `verify/fixture/platform.tf` with `module "lakehouse" { source = "../.." }`:
  - `name_prefix = local.prefix`, `environment_type = "Testing"`.
  - Pass the `aws.pricing` provider alias.
  - `cost_estimation_config = { enabled = false }`.
  - `catalog_integration_config` uses the module default (`enabled = true`).
  - `governance_config`:
    - `admin_principal_arns = [var.runner_role_arn, aws_iam_role.persona["admin"].arn]`;
    - `read_only_admin_principal_arns = concat([aws_iam_role.persona["roadmin"].arn], var.read_only_runner_arns)`;
    - `domain_values = ["general"]`.
  - `tenants`:
    - `alpha` with zones `["raw","curated"]`, `beta` with `["curated"]`, and `gamma` with
      `["curated"]`, all with sensitivity `restricted` and owner `lfverify@example.com`;
    - stewards: `steward` in alpha;
    - writers: `writer_multi` in alpha and beta, `writer_single` in alpha;
    - readers with sensitivity values `public` through `restricted`: `reader_multi` in alpha and
      beta, `reader_single`, `reader_rows`, and `reader_cells` in alpha;
    - reader `reader_cols` in alpha with sensitivity values `["public","internal","confidential"]`.
  - `row_filters` only when `var.stage == 2`:
    - `rows_eu` (`reader_rows`, curated, `orders`, `region = 'EU'`);
    - `rows_us` (`reader_rows`, curated, `orders`, `region = 'US'`);
    - `cells_eu` (`reader_cells`, curated, `orders`, `region = 'EU'`, with `column_names`
      excluding `ssn`).

  `bytes_scanned_cutoff_per_query` is left at the default.
- [x] T016 [P] Create `verify/fixture/grants-label.tf`, using the module's `lf_tag_vocabulary`
  and `tenants` outputs for keys:
  - `aws_lakeformation_lf_tag.mw`, with key `${local.prefix}-mw` and values `["yes"]`;
  - `aws_lakeformation_permissions` with an `lf_tag_policy` block for `meta_reader`: `DESCRIBE` on
    `DATABASE` and on `TABLE`, with the expression tenant tag of alpha or beta;
  - for `meta_writer`: `CREATE_TABLE` on `DATABASE` with expression `tenant = alpha` and
    `zone = curated`, plus `ALTER` and `DROP` on `TABLE` with expression `${local.prefix}-mw = yes`.

  Grants are scoped to the S3 Tables sub-catalog IDs from the module outputs.
- [x] T017 [P] Create `verify/fixture/labels.tf`. Everything is `count` or `for_each` gated on
  `var.stage == 2`. For each label-style namespace's `orders` table:
  - an `aws_lakeformation_resource_lf_tags` on `table_with_columns` sets the sensitivity tag to
    `restricted` on `ssn` and `internal` on the other columns;
  - the table sensitivity is set to `restricted`, following the FR-011a roll-up from 001.

  Also assign `${local.prefix}-mw = yes` to alpha curated `scratch` only.
- [x] T018 Create `verify/fixture/baseline.tf`:
  - `aws_s3tables_table_bucket` `${local.prefix}-named`, encrypted with
    `module.lakehouse.kms_key_arn` (SSE-KMS);
  - `aws_s3tables_namespace` for each of `n_alpha`, `n_beta`, and `n_gamma`;
  - an `aws_s3_bucket` `${local.prefix}-named-results` with `force_destroy = true`, a public
    access block, SSE-KMS with the platform key, a TLS-deny bucket policy, and a 1-day expiry
    lifecycle rule;
  - `aws_athena_workgroup` `${local.prefix}-named`, with enforced configuration, engine version 3,
    SSE-KMS result encryption, `bytes_scanned_cutoff_per_query = 1073741824`, and
    `force_destroy = true`.

  Add a Checkov skip comment with justification only where a check fails, for example access
  logging on a disposable bucket.
- [x] T019 Create `verify/fixture/grants-named.tf`. These are named-resource grants on the
  baseline catalog `${account}:s3tablescatalog/${local.prefix}-named`; no LF-tags are used. Use
  database-level resources at stage 1 and table-level resources at stage 2:
  - `meta_reader`: `DESCRIBE` on databases `n_alpha` and `n_beta`, and on all their tables
    (`wildcard = true`).
  - `meta_writer`: `CREATE_TABLE` on `n_alpha`, plus `ALTER` and `DROP` on table `n_alpha.scratch`.
  - `reader_multi`: `SELECT` and `DESCRIBE` on all tables in `n_alpha` and `n_beta`.
    `reader_single`: the same on `n_alpha` only.
  - `writer_multi` and `writer_single`: `SELECT`, `INSERT`, `DELETE`, `ALTER`, and `DESCRIBE`,
    with the same namespaces as the readers.
  - `reader_cols`: `SELECT` on `table_with_columns`, `n_alpha.orders`,
    `excluded_column_names = ["ssn"]`, plus `n_alpha.customers` with all columns.
  - `reader_rows`: two `aws_lakeformation_data_cells_filter` resources, `n_rows_eu` and
    `n_rows_us`, on `n_alpha.orders`, granted with `SELECT` through `data_cells_filter` blocks.
  - `reader_cells`: filter `n_cells_eu` (`region = 'EU'`, excluding the `ssn` column), granted
    the same way.
  - `steward`: `ALL` with grant option on `n_alpha`.
  - `control` and `n_gamma`: no grants.

  Depends on T018.
- [x] T020 [P] Add Athena and results access for the baseline workgroup to the persona policies
  in `verify/fixture/personas.tf`:
  - `athena:StartQueryExecution`, `GetQueryExecution`, `GetQueryResults`, `StopQueryExecution`,
    and `GetWorkGroup` on the baseline workgroup ARN;
  - `s3:GetObject`, `s3:PutObject`, `s3:ListBucket`, and `s3:GetBucketLocation` on
    `${results_bucket}/${persona}/*`;
  - `kms:Decrypt` and `kms:GenerateDataKey` on the platform key.

  Depends on T018.
- [x] T021 Create `verify/fixture/inject.tf`. Each overlay is gated by `var.inject`, and stage 2
  is required:
  - `extra_grant`: grant `SELECT` on all tables in `n_gamma`, plus a label grant on
    `tenant = gamma`, to `var.inject_persona`.
  - `missing_grant`: the persona's named grants take
    `count = var.inject == "missing_grant" && var.inject_persona == key ? 0 : 1`. Restructure the
    T019 resources as `for_each` over a local map that `inject.tf`'s locals filter.
  - `drop_column_restriction`: switch the `reader_cols` named grant to all columns, and the label
    reader's `sensitivity_values` to all four values. This passes through a local that
    `platform.tf` reads.
  - `drop_row_filter`: replace `n_rows_eu` and `n_rows_us` with an all-rows `SELECT` grant.
  - `open_default`: `aws_lakeformation_permissions` granting `ALL` on `n_alpha` to
    `IAM_ALLOWED_PRINCIPALS`.
- [x] T022 Create `verify/fixture/outputs.tf` with `output "manifest"`, a sensitive=false JSON
  object that `lfverify` consumes. It contains:
  - `run_id`, `region`, `account_id`, `prefix`, `stage`;
  - `personas`, mapping each persona key to its role ARN;
  - `styles`:
    - `label`: per tenant and zone, the `catalog_id`, `database`, `workgroup`, and results
      location from `module.lakehouse.tenants`, plus `lf_tag_keys` from `lf_tag_vocabulary` and
      the `mw` tag key;
    - `named`: `catalog_id`, the databases, workgroup `${local.prefix}-named`, the results
      bucket, and the filter names.
  - `tables`: from `local.tables`, including the partition column, the restricted column, and the
    map column;
  - `table_bucket_arns`: both styles;
  - `kms_key_arn`.
- [x] T023 Create `test/verify_fixture_test.go` (`TestVerifyFixture`), plan-only, following
  `helpers_test.go` patterns, with a unique `run_id` per test. It covers these subtests:
  - `stage1`: 13 IAM roles, no `aws_lakeformation_data_cells_filter` and no table-level grants,
    and data-lake settings `read_only_admins` containing the roadmin role.
  - `stage2`: the named filters, the `table_with_columns` grant excluding `ssn`, the module
    `row_filters`, and the column LF-tag assignments are present.
  - `inject_open_default`: a grant to `IAM_ALLOWED_PRINCIPALS` is planned.
  - `inject_requires_persona`: `inject = "extra_grant"` without `inject_persona` fails validation.

  Use placeholder ARNs in account 123456789012. Plans do not resolve principals.
- [x] T024 Checkpoint: run `make format`, `make check`, `make lint`, `make security-scan`, and
  `make test-verify-fixture`. All must pass. Fix Checkov findings or suppress them with an inline
  justification.

**Checkpoint**: The fixture plans cleanly at both stages and with each overlay.

---

## Phase 4: Application (`lfverify` CLI)

**Purpose**: The Go tool that seeds, runs, inspects, reports, and cleans up (plan.md, Compute
Resources). Each package ships with unit tests that need no AWS access.

**Dependencies**: T022 (manifest shape) for the full-mode packages. The matrix, classify,
predicate, and report packages can start in parallel with Phase 3.

### Core packages

- [x] T025 [P] `verify/internal/matrix/`: types, loader, and validation for the matrix YAML.
  - The top level has `mode` (`full|probe`), `defaults` (`settle_seconds: 120`,
    `positive_retry_seconds: 120`, `query_row_limit: 10`, `scan_cap_bytes: 1073741824`), and
    `personas`.
  - Each persona has `key`, `role_arn` (probe only; full mode resolves it from the manifest), and
    `cases`.
  - Each case has:
    - `id`;
    - `style` (`label|named|both`);
    - `phase` (`static|scenario|boundary`);
    - `op`, one of `list_catalogs`, `list_databases`, `get_database`, `list_tables`,
      `get_table`, `select`, `select_columns`, `count`, `insert`, `update`, `delete`,
      `create_table`, `alter_table`, `drop_table`, `grant`, `revoke`, `add_lf_tags`,
      `create_lf_tag`, `create_filter`, `list_permissions`, `get_settings`, `put_settings`,
      `storage_read`;
    - `resource`, as `{tenant|namespace, table, columns}` with symbolic names that the manifest
      resolves;
    - `expect`, one of `allowed`, `denied`, `rejected`, `absent`, or `inspect:<permission set>`;
    - `expect_columns`, `expect_rows` (`{count, keys}`), `row_predicate`
      (`{column, op, values}`), `max_rows`, `rule` (a short citation of the documented rule), and
      `note`.

  Validation rejects:
  - unknown ops;
  - write ops in probe mode (FR-027);
  - `expect_rows` with values in probe mode (FR-030);
  - duplicate IDs.

  Unit tests use valid and invalid YAML fixtures in `testdata/`.
- [x] T026 [P] `verify/internal/classify/`: `Classify(err error, athenaReason string, manifest)`
  returns `ALLOWED|DENIED|REJECTED|ERROR` and the evidence `{code, message, request_id}`, with
  these mappings (plan.md, Outcome classification):
  - **DENIED**: `AccessDeniedException`, `AccessDenied`, Athena
    `Insufficient Lake Formation permission`, `TABLE_NOT_FOUND`, `SCHEMA_NOT_FOUND`, or
    `COLUMN_NOT_FOUND` when the manifest says the object exists. Glue `EntityNotFoundException`
    on an existing object is also DENIED.
  - **REJECTED**: `InvalidInputException`.
  - **ERROR**: anything else, including `INTERNAL_ERROR_QUERY_ENGINE`.

  A table-driven unit test covers each error string.
- [x] T027 [P] `verify/internal/predicate/`: evaluate a structured row predicate
  (`eq|ne|in|lt|gt` over string, number, and date) against one row of `map[string]string`.
  - Return `match bool`, or `ErrColumnNotVisible`.
  - It must never return or log values.
  - Unit tests cover each operator and the missing-column case.
- [x] T028 [P] `verify/internal/report/`: a case result model with these fields:
  - `id`, `persona`, `style`, `op`, `resource`, `expected`, `observed`;
  - `status`: `pass|fail|not-run|verified-by-inspection`;
  - `reason` and `evidence`;
  - `duration`.

  Writers:
  - `report.json`;
  - `junit.xml`, with one testsuite per persona and style, where not-run is a JUnit `error` and
    fail is a `failure`;
  - `report.md`, with a per-persona table and summary counts.

  A unit test asserts that the evidence fields contain no row values: the report model has no
  field for them.
- [x] T029 `verify/internal/identity/`:
  - `Assume(ctx, roleARN, persona, runID, probe bool)` calls STS `AssumeRole` with
    `RoleSessionName = "lfverify-<runID>-<persona>"` (truncated to 64 characters) and
    `DurationSeconds = 3600`. When `probe` is true, it sets the inline session policy from
    `ProbeSessionPolicy(resultsPrefixes, kmsKeyARNs)`. The policy JSON is built with typed
    structs, not string templates, and contains only the actions listed in plan.md, Security.
  - Cache sessions per persona and refresh them before expiry.
  - Unit tests check the policy JSON: no `Put*`, `Create*`, `Delete*`, `Update*`, `Grant*`,
    `Revoke*`, or `Add*` actions except `s3:PutObject` scoped to results prefixes.

### Operations

- [x] T030 [P] `verify/internal/ops/catalog.go`: Glue `GetCatalogs`, `GetDatabases`,
  `GetDatabase`, `GetTables`, `GetTable`, `CreateTable`, `UpdateTable` (add a nullable
  `note2 string` column), and `DeleteTable`. Each takes a `CatalogId` (an S3 Tables sub-catalog
  ID from the manifest) and returns an `ops.Result{Outcome, Columns, Names}`. Visible column names
  from `GetTable` drive `expect_columns`.
- [x] T031 [P] `verify/internal/ops/permissions.go`: Lake Formation `GrantPermissions`,
  `RevokePermissions`, `ListPermissions` (paginated), `CreateLFTag`, `AddLFTagsToResource`,
  `RemoveLFTagsFromResource`, `GetResourceLFTags` (with `ShowAssignedLFTags`),
  `CreateDataCellsFilter`, `DeleteDataCellsFilter`, `GetDataLakeSettings`, and
  `PutDataLakeSettings`.
  - Grants carry a `cleanup` closure that the runner always calls: grants that were accepted
    unexpectedly are revoked, and created tags and filters are deleted.
  - `PutDataLakeSettings` in a test case writes back the value just read, so a successful call
    is harmless.
- [x] T032 [P] `verify/internal/ops/query.go`: Athena `StartQueryExecution` on the manifest
  workgroup with `QueryExecutionContext{Catalog: "s3tablescatalog/<bucket>", Database}`, polling
  with backoff up to 5 minutes, and `GetQueryResults` (paginated).
  - Returns column names, the row count, and, only for full mode, the key-column values from
    `expect_rows.keys`. In probe mode it streams rows into the predicate evaluator and discards
    them.
  - Records `DataScannedInBytes`.
  - SQL is generated for `select`, `select_columns`, `count`, `insert` (one deterministic row with
    a run-unique key), `update`, and `delete` (of that key only). Identifiers are quoted;
    persona-provided strings are never interpolated.
- [x] T033 [P] `verify/internal/ops/storage.go`: storage bypass (FR-022). It calls
  `s3tables:GetTableMetadataLocation` and then `s3:GetObject` on the metadata location, using
  persona credentials, and expects DENIED at the first call that fails. If the first call
  unexpectedly succeeds, the second call is still attempted and recorded.

### Full-mode steps

- [x] T034 `verify/internal/preflight/`:
  - **Full mode**:
    - the account and Region must be listed in `verify/sandbox-accounts.txt` or
      `LFVERIFY_SANDBOX_ACCOUNTS`;
    - `GetDataLakeSettings` admins must be a subset of `{runner}` before the run, or the tool
      refuses: another deployment owns the settings;
    - the target must not be marked production, meaning `LFVERIFY_ENV` is not `Production` or
      `MissionCritical` (FR-003);
    - save the settings to `out/<run-id>/settings.before.json`.
  - **Probe mode**:
    - `iam:SimulatePrincipalPolicy` on the runner over a fixed list of mutating actions
      (`lakeformation:GrantPermissions`, `RevokePermissions`, `PutDataLakeSettings`,
      `AddLFTagsToResource`, `CreateLFTag`, `DeleteLFTag`, `CreateDataCellsFilter`;
      `glue:CreateTable`, `UpdateTable`, `DeleteTable`, `CreateDatabase`, `DeleteDatabase`;
      `s3tables:PutTableData`, `DeleteTable`, `CreateTable`; `iam:PutRolePolicy`,
      `UpdateAssumeRolePolicy`, `AttachRolePolicy`; `athena:UpdateWorkGroup`;
      `s3:PutBucketPolicy`; and `s3:DeleteObject` on non-results buckets). Any `allowed` result
      aborts the run with the list of actions;
    - the runner must appear in `ReadOnlyAdmins`.
  - **Both modes (FR-008)**: `ListPermissions` with principal `IAM_ALLOWED_PRINCIPALS` on every
    target database must return nothing, or the run fails.

  Unit tests use a fake SDK interface.
- [x] T035 `verify/internal/seed/`. As the runner (fixture owner), for each namespace in both
  styles:
  - Create `orders`, `customers`, and `scratch` through Athena DDL
    (`CREATE TABLE ... PARTITIONED BY (order_date) TBLPROPERTIES ('table_type'='ICEBERG')`), in
    the matching workgroup.
  - Append deterministic rows with iceberg-go:
    - Use a REST catalog at `https://s3tables.<region>.amazonaws.com/iceberg`, with
      `warehouse = <table bucket ARN>`, SigV4 signing with service `s3tables`, and
      `rest.WithSigV4`.
    - Use Arrow record batches built from a seeded generator: 200 orders (80 EU, 70 US, 50
      APAC), 50 customers, and 5 scratch rows.
  - Verify `count(*)` per table.

  On an iceberg-go failure, report the error and stop (plan.md Risk 3). The fallback is decided
  then, not silently.
  - Unit-test the generator: the same seed gives the same rows and the expected counts per region.
- [x] T036 `verify/internal/runner/`: expands the matrix against the manifest and executes it.
  - **Static phase**: a worker pool of 8. Positive cases retry with exponential backoff up to
    `positive_retry_seconds`. Negative cases wait until `settle_seconds` after the
    `last_mutation` timestamp.
  - **Scenario phase**: sequential:
    - relabel `orders.amount` to `restricted`, assert that `reader_cols` loses it, restore it,
      and assert that the column returns (FR-017);
    - move `customers` in alpha to the `gamma` tenant tag, assert that `reader_single` is denied,
      and restore it (FR-023);
    - create `late_table` in alpha curated and in `n_alpha` as the runner, and assert visibility
      for `reader_multi` (FR-013);
    - the union of `rows_eu` and `rows_us` gives 150 rows (FR-018);
    - add an all-rows filter grant, assert 200 rows, and revoke it (FR-018).
  - **Boundary phase** (FR-021), with the admin persona attempting:
    - column-limited `SELECT` plus `INSERT` for one principal;
    - excluding the partition column `order_date`;
    - a filter expression on `attrs` (a map column).

    Each case expects `rejected`.
  - Every mutation runs its cleanup in a `defer`, and the runner sets `last_mutation`.
  - Probe mode skips the scenario and boundary phases, and enforces `scan_cap_bytes`; once the
    cap is reached, the remaining query cases are not-run (FR-031).
- [x] T037 `verify/internal/inspect/`: probe-mode effective permissions (FR-029). For a persona
  and a resource:
  - `ListPermissions(Principal)` gathers named grants, and `LFTagPolicy` grants with their
    expressions.
  - `GetResourceLFTags(ShowAssignedLFTags=false)` gets the effective tags, inherited ones
    included.
  - A grant matches when every expression key has one of its values on the resource.
  - The function returns the union permission set.

  Unit tests cover expression matching, inheritance from database to table to column, and an
  exact match versus a superset.
- [x] T038 [P] `verify/internal/snapshot/`: probe mode's before-and-after capture (Non-interference
  SLO). The snapshot holds:
  - the catalogs, databases, and tables (names, column names, and the Iceberg `metadata_location`
    parameter);
  - every LF-tag and its values;
  - resource tag assignments;
  - data cell filters;
  - the full `ListPermissions` output;
  - `GetDataLakeSettings`.

  It is serialized as sorted JSON. `Diff(before, after)` returns a list of differences. Probe
  query results in the results bucket are excluded.
- [x] T039 `verify/cmd/lfverify/main.go`: subcommands built with the standard `flag` package.
  Each subcommand loads `aws.Config` from the environment, writes to `--out verify/out/<run-id>`,
  and exits non-zero on any fail or not-run (FR-025). Exit code 2 means a usage or preflight
  refusal.
  - `preflight --mode full|probe --manifest <path>`
  - `seed --manifest`
  - `run --mode --matrix --manifest` (the manifest is full mode only)
  - `detect --manifest --fixture-dir`. For each scenario (one `extra_grant` and one
    `missing_grant` per reader or writer persona, plus `drop_column_restriction`,
    `drop_row_filter`, and `open_default`), it:
    - runs `terraform apply -auto-approve -var stage=2 -var inject=… -var inject_persona=…`;
    - reruns only that persona's cases, or all cases for `open_default`;
    - requires at least one failing case;
    - reverts with `inject=""`.

    It writes `detect.json`.
  - `snapshot --matrix --name before|after` and `diff`
  - `audit --run-id --since` uses CloudTrail `LookupEvents` with
    `Username = lfverify-<run-id>-*` and `ReadOnly = false`. It expects zero events, and warns if
    run less than 15 minutes after the probe.
  - `cleanup --run-id`: see T040.
- [x] T040 Implement `cleanup` in `verify/internal/preflight/cleanup.go`. It sweeps resources by
  the `lfv-<run-id>` prefix and the `LfverifyRun` tag, in this order:
  1. data cell filters;
  2. LF permissions granted to the persona roles;
  3. LF-tags with the prefix;
  4. tables, namespaces, and table buckets;
  5. Athena workgroups;
  6. results buckets, emptied first;
  7. persona roles and their policies.

  It then writes `settings.before.json` back with `PutDataLakeSettings`, verifies the result with
  `GetDataLakeSettings`, and fails with a diff if they differ. It must be idempotent.

### Matrices

- [x] T041 Write `verify/matrix/full.yaml` for all 13 personas in both styles (`style: both`,
  unless noted). Cite the governing rule in `rule:` on each case.
  - `admin` (FR-009): list and get on every namespace and table; `list_permissions`;
    `grant`/`revoke` `SELECT` on alpha `customers` to `control`, with cleanup; `create_lf_tag` and
    `add_lf_tags`; `create_filter`. It also has `select` on `orders` without a self-grant,
    expected `denied`, with the rule "admins added after resource creation get no implicit data
    access" and `note: confirm (plan Risk 2)`.
  - `roadmin` (FR-010): every list and get, including the `s3tablescatalog` sub-catalogs
    (`note: confirm (plan Risk 1)`); `list_permissions`; LF-tag and filter reads. The following
    are expected `denied`: `grant`, `revoke`, `create_lf_tag`, `add_lf_tags`, `create_table`,
    `alter_table`, `drop_table`, `create_filter`, `put_settings`, and `select`.
  - `meta_reader` (FR-011): `get_database` and `list_tables` on alpha and beta; `get_table` with
    the full schema in `expect_columns`. Expected `denied`: `select`, `count`, `alter_table`,
    `drop_table`, `create_table`, and `get_database` on gamma.
  - `meta_writer` (FR-012): `create_table` in alpha curated (the new table is noted as
    creator-owned); `alter_table` and `drop_table` on `scratch`. Expected `denied`: `select` and
    `insert` on `orders`, `alter_table` on `customers`, and `create_table` in beta.
  - `reader_multi` and `reader_single` (FR-013, FR-014): `select` with exact counts on every
    granted table. Expected `denied`: `insert`, `update`, `delete`, `alter_table`, `drop_table`,
    and `select` on each ungranted namespace.
  - `writer_multi` and `writer_single` (FR-015, FR-016): `insert` → `select` the key → `update` →
    `delete` on each granted namespace's `customers`. Expected `denied`: every op in ungranted
    namespaces, `add_lf_tags`, and `grant`.
  - `reader_cols` (FR-017):
    - `get_table` with `expect_columns` excluding `ssn`;
    - `select_columns [order_id, region, amount]` with 200 rows;
    - `select_columns [ssn]` expected `denied`;
    - `select` (`*`) expected `absent: [ssn]`;
    - `customers` with all columns.
  - `reader_rows` (FR-018): `count` gives 150 (EU and US union in the static phase); `select`
    with keys; the scenario cases from T036.
  - `reader_cells` (FR-019): `count` gives 80; `get_table` and `select` exclude `ssn`; every
    returned row has `region = EU`.
  - `control` (FR-020): every op on every resource expected `denied` or `absent`.
  - `storage_read` for every persona except the runner, expected `denied` (FR-022).
  - Label-only cases: relabel (FR-023) and inheritance on `late_table`.
  - Named-only cases: the wildcard-versus-named-table difference for late tables (FR-013 note).
- [x] T042 [P] Write `verify/matrix/probe.example.yaml`. It is a commented template for target
  owners:
  - A header explains:
    - the read-only runner requirements;
    - `read_only_admin_principal_arns`;
    - persona trust for the runner role with session names `lfverify-*`;
    - that writes are verified by inspection.
  - Example personas: one reader with `row_predicate` and `max_rows`, one column-restricted reader
    with `expect_columns`, and one writer with `expect: inspect: [SELECT, INSERT, DELETE, ALTER]`.
  - It passes `matrix` validation in probe mode.
- [x] T043 Add `verify/internal/matrix/full_test.go`. It loads `verify/matrix/full.yaml` and
  asserts that:
  - every persona in FR-009 to FR-020 has at least one positive and one negative case in each
    style;
  - every FR-021 boundary case exists;
  - every persona has a `storage_read` case.
- [x] T044 Checkpoint: run `make test-verify` (vet plus unit tests, all packages) and
  `make pre-commit`.

**Checkpoint**: The CLI builds and all unit tests pass offline.

---

## Phase 5: Operations (make targets, CI, sandbox runs)

**Purpose**: Single-command entry points (FR-001), optional CI, and the live runs that prove the
SLOs.

**Dependencies**: Phases 3 and 4.

- [x] T045 Add live targets to `Makefile` under a `##@ Access verification` section. Each target
  depends on `check-aws`.
  - `RUN_ID ?=` is a random six-character hex value from `$(shell openssl rand -hex 3)`, cached
    in `verify/out/.last-run-id`.
  - `RUNNER_ROLE_ARN` is derived with `aws sts get-caller-identity` and the session-context role.
  - `FIXTURE_DIR = verify/fixture`, `OUT = verify/out/$(RUN_ID)`.
  - Targets:
    - `verify-build` runs `go build -o verify/out/lfverify ./cmd/lfverify` in `verify/`.
    - `verify-up` runs preflight full, then `terraform init` with backend path
      `$(OUT)/terraform.tfstate` and `TF_DATA_DIR=$(OUT)/.terraform`, `apply` with `stage=1`,
      then writes the manifest JSON with `terraform output -json manifest`.
    - `verify-seed` runs seed, then `apply` with `stage=2`, then refreshes the manifest.
    - `verify-run` runs the full-mode `run` with `matrix/full.yaml`.
    - `verify-down` runs `terraform destroy` at `stage=2`, then `lfverify cleanup`.
    - `verify-full` runs `verify-build`, `verify-up`, `verify-seed`, and `verify-run`, always
      followed by `verify-down`. It exits with the `verify-run` status.
    - `verify-detect` runs `verify-up`, `verify-seed`, `lfverify detect`, and `verify-down`.
    - `verify-cleanup` requires `RUN_ID`.
    - `verify-probe` requires `MATRIX`, and runs preflight probe, `snapshot before`, `run`,
      `snapshot after`, and `diff`. It fails on any difference.
    - `verify-probe-audit` requires `RUN_ID`.

  The `make test-*` targets must never invoke these targets.
- [x] T046 [P] Create `.github/workflows/verify-access.yml`:
  - `workflow_dispatch`, with inputs `mode` (`full|detect`) and `region` (default `us-east-1`);
  - `environment: development` and `permissions: id-token: write, contents: read`;
  - steps:
    - `aws-actions/configure-aws-credentials` with the runner role, read from a repository
      variable;
    - set up Terraform 1.14.x and Go 1.26;
    - `make verify-full` or `make verify-detect`;
    - upload `verify/out/**/report.*`, `junit.xml`, and `detect.json` as artifacts;
    - publish JUnit annotations;
    - an `if: always()` step that runs `make verify-cleanup RUN_ID=$(cat verify/out/.last-run-id)`.

  Pin every action to a full commit SHA. Add a comment that the runner role trust must use the
  immutable OIDC subject from plan.md, Security.
- [x] T047 [P] Write `verify/README.md`, covering:
  - the purpose;
  - the two modes and their safety rules (allow-list, settings snapshot and restore, the
    read-only runner, session policies, no data values);
  - prerequisites (sandbox account in `sandbox-accounts.txt`, runner role is a data-lake admin);
  - each make target;
  - the matrix format, with a link to `probe.example.yaml`;
  - reading the reports;
  - interrupted-run cleanup;
  - the "confirm in first run" risks from plan.md;
  - how to update expectations deliberately when AWS behavior changes (spec Assumptions).
- [x] T048 Sandbox: run `make verify-full` in dev-green.
  - Record the run ID and duration (the target is 30 minutes or less), and the case totals.
  - Resolve plan Risks 1–4 by updating `verify/matrix/full.yaml`. Each changed expectation gets a
    `note:` citing the observation and date.
  - Writer DML cases are expected to be `not-run: engine error` while `awslkhs-cdq` is open.
    Record them; do not change their expectation.
  - Result (2026-11-11): first run `36ea41`, then `24b160` and `790b58`; risks 1–4 resolved with
    dated `note:` entries in `full.yaml`; the 14 DML cases are not run, as expected. Runs, totals,
    and durations are in `sandbox-results.md`.
- [x] T049 Sandbox: run `make verify-full` 10 consecutive times against an unchanged
  configuration (Determinism SLO). Compare `report.json` pass/fail per case ID across the runs.
  Any flaky case is fixed in the runner's retry and settle logic, not by loosening expectations.
  - Result (2026-11-12): all 211 cases had the same status and outcome in all 10 runs; no flaky
    case. Runs took 873–937 s, except one slowed by the host sleeping. Fixed `duration_ns` always
    being 0. See `sandbox-results.md`, Determinism.
- [x] T050 Sandbox: run `make verify-detect`. Every scenario must produce at least one failing
  case with identifying evidence (Detection SLO). Save `detect.json`.
- [x] T051 Sandbox: probe-mode cross-check (success criteria, Operational Validation).
  - Apply the fixture with `read_only_runner_arns` set to a read-only probe runner role, created
    manually per `verify/README.md` with `ReadOnlyAccess`, `AssumeRole` on the personas, and
    self-simulate.
  - Write `verify/out/<run-id>/probe.yaml` from the manifest: the same personas, with probe-safe
    cases only.
  - Run `make verify-probe` as the probe runner, and confirm that every case both modes verify
    has the same pass/fail and that `diff` reports no change.
  - At least 15 minutes later, run `make verify-probe-audit` and confirm zero change events.
  - Then confirm that `lfverify preflight --mode probe` aborts when the runner temporarily holds
    `lakeformation:GrantPermissions` (Security Validation).
- [x] T052 Sandbox: interrupted-run recovery (Operational Validation). Start `make verify-full`,
  kill it after `verify-seed`, delete `$(OUT)/terraform.tfstate`, then run
  `make verify-cleanup RUN_ID=…`. Confirm that, 15 minutes later, no resource with the prefix or
  tag remains (Cleanup SLO), and that the data-lake settings equal `settings.before.json`.
  - Result (2026-11-12, run `86aeab`): cleanup exited 0 in 57 s; 15 minutes later no prefixed or
    tagged resource remained except the KMS key pending deletion, and the settings matched. The
    first attempt (`67009c`) exposed the all-tables grant revoke failure, fixed in `c90110a`.
- [x] T053 Confirm that full-mode preflight refuses a non-allow-listed account and
  `LFVERIFY_ENV=Production` (FR-003), by running `lfverify preflight` with a temporary empty
  allow-list.
  - Result (2026-11-12, dev-green): exit 2 with the reason for an empty allow-list, an
    allow-listed account in another Region, `LFVERIFY_ENV=Production`, `MissionCritical`, and
    `Production` with the account allowed through `LFVERIFY_SANDBOX_ACCOUNTS`. No settings file
    was written.
  - Found and fixed: `verify-full` and `verify-detect` ran `verify-down` after a refused
    preflight, and its stage 1 apply would have deployed into the refused account. `verify-down`
    now requires `settings.before.json`.
- [x] T054 Write `specs/002-lf-access-verification/sandbox-results.md` in the T076 results style.
  Include:
  - the run IDs and durations;
  - pass, fail, and not-run counts per persona and style;
  - the confirmed answers to plan Risks 1–4;
  - the Detection and Determinism results;
  - the probe cross-check and audit results;
  - the cost observed from Athena `DataScannedInBytes` totals;
  - a "Not verified" table with the tracking beads.

  If row-filter cases now pass, record that `awslkhs-cdq`'s row-filter gap is closed for reads,
  and update `awslkhs-92h`.

**Checkpoint**: All SLOs are measured in the sandbox, and the results are recorded.

---

## Phase 6: Polish & Validation

- [x] T055 Run `make validate` (format, check, lint, security-scan, test, test-verify, docs). All
  steps must run and pass; a target skipped for a missing tool does not count.
  - Result (2026-11-12): every step ran and passed, with no tool skipped. Checkov: 773 passed,
    0 failed. `test`: all plan-only tests pass in 827 s, including the 5 `TestVerifyFixture`
    subtests. `TestSecondaryExampleSkipsAccountIntegration` skips by design unless
    `LAKEHOUSE_TEST_LF_ADMIN=true`. `format` and `docs` changed no files.
- [x] T056 [P] Update `AGENTS.md` and `CLAUDE.md` together:
  - add the `verify/` directory to the layout;
  - add the rule that live `verify-*` targets never run from `make validate`;
  - add the persona key list.

  Update the root `README.md` with a short "Access verification" section that links to
  `verify/README.md`.
- [x] T057 [P] Record deviations from plan.md found during implementation, such as an iceberg-go
  fallback or classifier strings, in `specs/002-lf-access-verification/plan.md` under Complexity
  Tracking.
- [x] T058 Update beads:
  - close `awslkhs-t7d` after T007–T011 merge;
  - add a note to `awslkhs-lfj` with the metadata-persona findings from T048;
  - file new beads for any not-run or failed case that the harness exposed in the module.
  - Result (2026-11-12):
    - `awslkhs-t7d` and `awslkhs-fysh` were closed with their fixes (`aab5dc5`, `bca2b33`).
    - `awslkhs-lfj` has the metadata-persona findings, including that a metadata writer that
      creates a table receives SELECT on it as its creator.
    - The only not-run cases are the 14 DML cases; `awslkhs-cdq` notes them, including that reader
      DML denials cannot be verified while it is open. `awslkhs-92h` notes that row filters are
      verified for reads. No case failed, and no new module bug was found.
    - New harness coverage beads: `awslkhs-ptos` (label-style Lake Formation denial on gamma),
      `awslkhs-sr2a` (label-style missing-grant detection), and `awslkhs-46y5` (gosec).
- [x] T059 Final review against spec success criteria. Tick each box in `spec.md` Success
  Criteria only with evidence from `sandbox-results.md` or the `make validate` output.
  - Result (2026-11-12): all 17 success criteria ticked with evidence (`sandbox-results.md`,
    Success criteria review). Static analysis was ticked after `awslkhs-46y5` added
    `make lint-verify` (gosec).


---

## Phase 7: Standalone Distribution (amendment 2026-11-12, FR-032 to FR-036)

**Purpose**: Ship `lfverify` with every release as checksummed executables for Linux, macOS, and
Windows (amd64 and arm64), usable for probe mode without a repository checkout, with operator
documentation. Design: plan.md, Standalone Distribution.

### CLI

- [ ] T060 [P] Add `lfverify version` in `verify/cmd/lfverify/main.go` (FR-033):
  - package variables `version = "dev"` and `commit = "none"`, set by `-ldflags -X`;
  - prints `lfverify <version> (<commit>) <go version> <os>/<arch>`;
  - unit test in `main_test.go`.
- [ ] T061 [P] Embed the probe matrix template and add `lfverify example-matrix [--out <file>]`
  (FR-034):
  - new Go package in `verify/matrix/embed.go` with `//go:embed probe.example.yaml`;
  - writes to stdout, or to `--out` without overwriting an existing file;
  - unit test that the embedded bytes equal the file and pass `matrix.Parse` in probe mode.
- [ ] T062 Add `lfverify probe --matrix <file>` in `verify/cmd/lfverify/main.go` (FR-034):
  - runs the `make verify-probe` sequence in one process: new run ID, preflight, before
    snapshot, run, after snapshot, diff;
  - exits non-zero on any failure, and still takes the after snapshot and diff when cases fail;
  - prints the `lfverify audit` command to run 15 minutes later;
  - change `make verify-probe` in `Makefile` to call it;
  - unit test of the sequencing with fakes for the step functions.
- [ ] T063 [P] Command help (FR-036): `lfverify help` and `lfverify <command> --help` list each
  command's own flags with descriptions, in `verify/cmd/lfverify/main.go`. Unit test that every
  command answers `--help` with exit 0.

### Build and release

- [ ] T064 Add `make verify-dist VERSION=<v>` in `Makefile` (FR-032, FR-033):
  - builds `linux`, `darwin`, and `windows` on `amd64` and `arm64` into `verify/out/dist/`, with
    `CGO_ENABLED=0 -trimpath -ldflags "-s -w -X main.version=$(VERSION) -X main.commit=<sha>"`;
  - archives each as `lfverify_<v>_<os>_<arch>.tar.gz`, or `.zip` for Windows, containing the
    executable, `LICENSE`, `verify/docs/`, and `verify/matrix/probe.example.yaml`;
  - writes `SHA256SUMS`;
  - add `verify-dist` to `.PHONY`.
- [ ] T065 Run `make verify-dist VERSION=ci` in the lint job of
  `.github/workflows/terraform-ci.yml`, so a cross-platform build break fails CI.
- [ ] T066 Create `.github/workflows/lfverify-release.yml` (FR-032, FR-033):
  - triggers: `workflow_call` (input `tag`) and `workflow_dispatch` (dry run, no tag);
  - jobs:
    - `check`: `make test-verify`, `make lint-verify`;
    - `build`: `make verify-dist VERSION=<tag or dev-<sha>>`, then upload the artifact;
    - `smoke`: matrix of `ubuntu-latest`, `ubuntu-24.04-arm`, `macos-latest`, `macos-13`, and
      `windows-latest`; verify the archive against `SHA256SUMS`, extract it, run
      `lfverify version` (must print the tag) and `lfverify example-matrix`;
    - `publish` (tag only, needs `smoke`): `gh release upload <tag>` of every archive and
      `SHA256SUMS`;
  - permissions: `contents: read`, and `contents: write` on `publish` only;
  - actions pinned by major tag, as in the existing CI.
- [ ] T067 Wire `.github/workflows/release-please.yml`:
  - expose `release_created` and `tag_name` as job outputs;
  - add a job that calls `lfverify-release.yml` with `tag: ${{ needs.release-please.outputs.tag_name }}`
    when `release_created` is true.

### User documentation

- [ ] T068 [P] Write `verify/docs/user-guide.md` for operators (FR-035):
  - install per OS: download, `sha256sum` / `shasum -a 256` / `Get-FileHash` checks, PATH, the
    macOS quarantine attribute and Windows SmartScreen prompt for unsigned executables;
  - prerequisites;
  - creating the read-only probe runner role and adding it to
    `governance_config.read_only_admin_principal_arns`;
  - persona trust for session names `lfverify-*`;
  - `LFVERIFY_PROBE_RESULTS_BUCKETS` and `LFVERIFY_PROBE_KMS_KEYS`;
  - `lfverify example-matrix`, then the matrix format reference with examples;
  - `lfverify probe` and the individual steps;
  - reports, statuses, and exit codes;
  - `lfverify audit`;
  - troubleshooting: `GetDataAccess`, `CATALOG_NOT_FOUND`, preflight refusals, expired sessions;
  - probe-mode limits;
  - full mode needs the repository.
- [ ] T069 [P] Add `verify/docs/probe-runner-policy.json` (ReadOnlyAccess companion: `sts:AssumeRole`
  on persona roles, `iam:SimulatePrincipalPolicy` on itself, `lakeformation:GetDataAccess`) and
  `verify/docs/persona-trust-statement.json`, referenced from the user guide.
- [ ] T070 Update contributor and repository docs:
  - `verify/README.md`: link the user guide, and list `version`, `probe`, `example-matrix`, and
    `make verify-dist`;
  - root `README.md` Access Verification section: download from the latest release;
  - `AGENTS.md`: `verify/docs/`, `make verify-dist`, and the release workflow.

### Validation

- [ ] T071 Release dry run:
  - run `lfverify-release.yml` with `workflow_dispatch` on the branch; confirm every job passes,
    the six archives verify against `SHA256SUMS`, and the smoke jobs print the version;
  - after the next real release, confirm the assets are on the release page and that a failed
    smoke test would block `publish`;
  - record both in `sandbox-results.md`.
- [ ] T072 Standalone acceptance in dev-green (Distribution Validation):
  - deploy a fixture with the repository (`make verify-up`, `make verify-seed`) and a read-only
    probe runner;
  - in an empty directory outside the repository, using only a downloaded archive and its user
    guide: `lfverify example-matrix`, edit the matrix (or use `lfverify probe-matrix` output
    copied in), run `lfverify probe`, then `lfverify audit` 15 minutes later;
  - compare the results with a probe run from the repository;
  - tear the fixture down;
  - record the result in `sandbox-results.md`;
  - tick the Distribution Validation criteria in `spec.md` with evidence, and run `make validate`.

**Checkpoint**: A release publishes six checksummed archives with the user guide. Smoke tests pass
on Linux, macOS, and Windows. A probe run succeeds from a downloaded archive with no repository
checkout.

---

## Dependencies & Execution Order

```text
Phase 1 Setup (T001–T006)
   │
Phase 2 Identity & Module Foundation (T007–T014)
   │   T007 ─┬─ T009 ── T010, T011
   │   T008 ─┘
   │   T012 ── T013
   │
Phase 3 Fixture (T015–T024)              Phase 4a Core packages (T025–T029) ← can start after Phase 1
   │   T015 ── T016, T017 [P]                │
   │   T018 ── T019, T020                    │
   │   T019 ── T021 ── T022 ── T023 ── T024  │
   │                                         │
   └──────────────┬──────────────────────────┘
                  │
Phase 4b Ops & steps (T030–T040), Matrices (T041–T044)  ← needs T022 manifest shape
                  │
Phase 5 Operations (T045–T047), then sandbox runs T048 → T049 → T050 → T051 → T052 → T053 → T054
                  │
Phase 6 Polish (T055–T059)
                  │
Phase 7 Standalone distribution (amendment)
   T060, T061, T063 [P] ── T062 ── T064 ── T065, T066 ── T067 ── T071
   T068, T069 [P] ── T070
   T071 ── T072
```

- The module change (T007–T011) can merge on its own ahead of the harness. It fixes
  `awslkhs-t7d` independently.
- Live runs, in order: T048 confirms the expectations first. Determinism (T049) and Detection
  (T050) run only on the corrected matrix.
- Phase 7: T064 needs the docs (T068, T069) in place to package them, but builds without them. T067
  must follow T066, and T071 needs both. T072 needs a published or dry-run archive (T071).

## Parallel Execution Examples

```text
# Phase 7:
T060 version  |  T061 example-matrix  |  T063 help  |  T068 user guide  |  T069 example policies

# Phase 1, after T001:
T002 verify/go.mod   |  T003 verify/sandbox-accounts.txt  |  T004 verify/fixture skeleton

# Phase 2:
T007 root variables.tf  |  T008 modules/catalog-integration   (then T009)
T010 tests              |  T011 docs                          (after T009)

# Phase 3, after T015:
T016 grants-label.tf  |  T017 labels.tf  |  T018 baseline.tf

# Phase 4a, independent of Phase 3:
T025 matrix  |  T026 classify  |  T027 predicate  |  T028 report

# Phase 4b, after T022 and T029:
T030 catalog.go  |  T031 permissions.go  |  T032 query.go  |  T033 storage.go  |  T038 snapshot

# Phase 5:
T046 workflow  |  T047 README   (alongside T045)
```

## Validation Checkpoints

| After | Command(s) | Gate |
|-------|-----------|------|
| T006 | `make format`, `make check`, `make test-verify` | The skeleton validates |
| T014 | `make pre-commit`, `make test-validation` | The module change is valid and its validation tests pass |
| T024 | `make check`, `make lint`, `make security-scan`, `make test-verify-fixture` | The fixture plans at stages 1 and 2 and with each overlay |
| T044 | `make test-verify`, `make pre-commit` | The CLI's unit tests pass offline |
| T048–T054 | `make verify-full`, `verify-detect`, `verify-probe`, `verify-probe-audit`, `verify-cleanup` | The SLOs are measured in the sandbox |
| T055 | `make validate` | Every gate passes |

## Environment Order

Sandbox (dev-green, full mode) → sandbox (probe mode against the full-mode deployment) → any
consumer environment in probe mode only, driven by the target owner's matrix. Full mode never
runs outside allow-listed sandbox accounts.
