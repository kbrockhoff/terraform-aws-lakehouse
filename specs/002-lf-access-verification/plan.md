# Architecture Plan: Data Permission Access Verification Harness

**Branch**: `002-lf-access-verification` | **Date**: 2026-11-10 | **Spec**: [spec.md](./spec.md)
**Input**: Infrastructure specification from `/specs/002-lf-access-verification/spec.md`

**Note**: This template is filled in by the `/iac.plan` command. For deep research, module specs, and quickstart guide, run `/iac.enrichplan` after planning.

## Summary

The harness is a Go command-line tool, `lfverify`, plus a disposable Terraform fixture. Together
they prove that Lake Formation grants on S3 Tables give each persona exactly the intended access.
The tool proves this by assuming each persona's IAM role and attempting real Glue, Lake Formation,
Athena, and S3 Tables calls.

- **Full mode** runs only in an allow-listed sandbox account (`make verify-full`). It:
  - applies the fixture, which deploys this module (`source = "../.."`) with LF-tag grants and adds
    a parallel named-resource baseline in its own table bucket;
  - creates one IAM role per persona;
  - seeds synthetic rows;
  - runs a declarative persona × operation matrix in both grant styles;
  - writes JSON, JUnit, and Markdown reports;
  - destroys the fixture and restores the data-lake settings from a pre-run snapshot.
- **Probe mode** (`make verify-probe`) runs against any existing deployment, including production.
  The runner identity is an IAM read-only, Lake Formation read-only administrator. Each persona
  session is limited by a read-only STS session policy. Write and grant expectations are checked by
  inspecting effective permissions, never by attempting them. Reports carry no data values, and a
  before-and-after snapshot of the target proves that the probe changed nothing.

Research produced one required module change. `modules/catalog-integration` sets only `admins` on
`aws_lakeformation_data_lake_settings`, and `PutDataLakeSettings` replaces the whole settings
object. Every apply therefore erases any read-only administrators. The plan adds
`governance_config.read_only_admin_principal_arns` (see Complexity Tracking).

## Technical Context

**Cloud Provider**: AWS, single account and single Region per run.

**IaC Tool**: Terraform >= 1.11, the same floor as the module. Local and CI runs use 1.14.x.

**Provider Versions**: `hashicorp/aws >= 6.59.0, < 7.0.0`, the same constraint as the root
`versions.tf`, because the fixture calls the module. The fixture also uses these resources, all
present in 6.59.0:
- `aws_lakeformation_permissions`, with `database`, `table`, `table_with_columns`,
  `data_cells_filter`, and `lf_tag_policy` blocks;
- `aws_lakeformation_resource_lf_tags`;
- `aws_lakeformation_data_cells_filter`;
- `aws_s3tables_table_bucket` and `aws_s3tables_namespace`;
- `aws_iam_role`.

**Application runtime**: Go 1.26, matching `test/go.mod`, in a separate Go module
`github.com/kbrockhoff/terraform-aws-lakehouse/verify`. Dependencies are pinned in `go.mod` and
`go.sum`:
- `aws-sdk-go-v2`, at the same minor versions as `test/go.mod`, with the service clients `sts`,
  `iam`, `glue`, `lakeformation`, `athena`, `s3`, `s3tables`, and `cloudtrail`;
- `github.com/apache/iceberg-go`, the latest release at implementation time, pinned exactly. It is
  used only by the seed step: a REST catalog against the S3 Tables Iceberg REST endpoint with SigV4
  (service `s3tables`) to append Arrow record batches;
- `go.yaml.in/yaml/v3`, for the matrix files.

**Module Versions**: The fixture references this repository's root module by relative path
(`../..`), so every run verifies the working tree. It uses no external modules.

**Curated Modules**: None apply. No curated modules exist for Lake Formation on S3 Tables, and test
IAM roles are three or four resources each, which needs no wrapper module.

**State Backend**:
- **Full mode**: local state per run, in `verify/out/<run-id>/terraform.tfstate`, set with
  `-backend-config=path=...`. Fixtures are disposable, and losing the state is recoverable with
  `lfverify cleanup --run-id`, which sweeps by name prefix and the `LfverifyRun` tag.
- **Probe mode**: creates no infrastructure and has no state.

**Environment Strategy**: There are no per-environment tfvars.
- The fixture always deploys the module with `environment_type = "Testing"` (Baseline), and
  `name_prefix = "lfv-<6 hex>"`, which is the run ID.
- Probe mode takes a target description (matrix file) per environment.

**Testing**:
- Unit tests, `go test ./...` in `verify/`, cover matrix validation, outcome classification,
  session-policy generation, the predicate evaluator, and report rendering. They run with the new
  `make test-verify` target, which `make validate` includes and which needs no AWS credentials.
- A Terratest plan-only test, `test/verify_fixture_test.go`, plans the fixture at `stage = 1` and
  `stage = 2`. It asserts the persona roles, the named-resource baseline, and the
  injection-variable behavior. It runs with `make test-verify-fixture`, which `make test` includes.

**Security Scanning**:
- Checkov and TFLint already scan recursively, so `verify/fixture` is covered.
- `make check` is extended to `terraform validate` `verify/fixture`.
- `gosec` runs through `make lint-verify` when installed. A skip does not count as a pass, per the
  principles.

**Cost Estimation**: Usage-based only.
- A full run is under USD 0.10:
  - Athena scans about 600 queries over KB-sized tables, which bills the 10 MB minimum per query, so
    about 6 GB at USD 5/TB, or about USD 0.03;
  - S3 Tables storage and requests are negligible;
  - the fixture creates no KMS key, because no tenant is designated, and the platform key uses a
    7-day deletion window.
- A probe run is at most 1 GB scanned, or USD 0.005.
- The module's `cost_estimation_config` is disabled in the fixture, so the run does not need the
  pricing provider alias for estimates. The alias is still configured, because the module requires
  it.

**Target Environments**:
- **Full mode**: only accounts listed in `verify/sandbox-accounts.txt`, or in the
  `LFVERIFY_SANDBOX_ACCOUNTS` variable. The current entry is `dev-green` (492637581429,
  us-east-1).
- **Probe mode**: any account.

**Compliance**: The harness produces SOC 2, HIPAA, PCI DSS, and GDPR access-control evidence. It
uses only synthetic data in full mode and writes no data values in probe mode.

## Principles Check

Principles v1.3.0 (`.specify/memory/principles.md`).

| Principle | Status | How this plan complies |
|-----------|--------|------------------------|
| Govern Data Access by Default | ✅ | The harness verifies this principle directly. It checks that `IAMAllowedPrincipals` defaults are absent before running (FR-008). Every persona role's IAM policy grants API reachability only: Glue `Get*`, `lakeformation:GetDataAccess`, Athena on the tenant workgroups, and results-prefix S3. Lake Formation alone decides access. The fixture-owner exception (the deployer) is the only identity with direct `s3tables:*` table access. In label style, row filters are named-resource grants, which the platform documents as an exception. |
| Embed Compliance into the Architecture | ✅ | Full mode uses synthetic data only. Probe reports omit values (FR-030). Every assumed session is named `lfverify-<run-id>-<persona>`, so CloudTrail attributes all calls to the run. Full mode refuses designated tenants and production (FR-003). |
| Optimize for Cost Efficiency | ✅ | No always-on compute. The tool runs on the operator's machine or a CI runner. Athena is pay-per-query, and fixtures are KB-sized. Teardown removes everything, including query results, because the fixture results bucket uses `force_destroy`. Probe scans are capped at 1 GB per run. |
| Protect Data Durability and Recoverability | ✅ | Probe mode is non-destructive by construction: a read-only runner, read-only session policies, and a before-and-after diff. Full mode snapshots the data-lake settings before the run, restores them after destroy, and refuses accounts that are not allow-listed. |
| Establish Resource Identity and Accountability | ✅ | Every fixture resource is named `lfv-<run-id>-*`. It also carries the module's common tags plus `LfverifyRun = <run-id>` and `Purpose = access-verification`. |
| Define Small, Validated Module Contracts | ✅ | The module gains one optional input, `governance_config.read_only_admin_principal_arns` (default `[]`, validated as IAM role or user ARNs). It is justified in Complexity Tracking. The harness lives under `verify/`, outside the module interface, like `test/`. |
| Secure by Construction | ✅ | Persona roles: <br>• have trust policies that allow only the runner role ARN, with `sts:RoleSessionName` required to start with `lfverify-`; <br>• have a maximum session of 1 hour and no access keys; <br>• have policies built with `aws_iam_policy_document`. <br>Probe sessions add an inline session policy that allows only `Get*`, `List*`, `Describe*`, `Search*`, `athena:StartQueryExecution`, and results-prefix `s3:PutObject`. Probe mode checks with `iam:SimulatePrincipalPolicy` that the runner cannot call any mutating action, and aborts otherwise. |
| Validate Before Merge | ✅ | New targets: `make test-verify` (unit tests) and `make test-verify-fixture` (plan-only Terratest). `make check` and `make validate` include them. Live runs (`verify-full`, `verify-probe`) are explicit targets and never part of `make validate`, matching the existing `test-examples-full` split. |
| Make Dependencies Explicit | ✅ | The fixture passes every principal and identifier to the module as inputs. The CLI reads a fixture manifest, a Terraform output written to `verify/out/<run-id>/manifest.json`, and never discovers resources by name. Probe mode takes role ARNs, catalog IDs, and workgroups from the matrix file. |
| Lock Versions and Release Semantically | ✅ | `verify/go.mod` pins every dependency. The fixture's `versions.tf` repeats the module constraint. The new module input is a backward-compatible minor change (`feat:`). |
| Environment-Driven Defaults | ✅ | No new per-environment behavior. `read_only_admin_principal_arns` defaults to `[]` in every environment type, so there is no `environment_defaults` entry. |
| Compliance Floor, Cost Ceiling | ✅ | The harness adds no cost-incurring controls. It runs only on demand. |
| Compose, Don't Accumulate | ✅ | Metadata-only personas and the named-resource baseline are fixture-owned grants, not module features. The module stays tag-only (FR-009 of 001). The gap is recorded as a follow-up bead, not added here. |

## Infrastructure Architecture

### Compute Resources

There is no cloud compute. `lfverify` is a single static Go binary, run locally or in GitHub
Actions (`ubuntu-latest`). All queries run on Athena engine v3 in the module's per-tenant
workgroups, plus one fixture workgroup for the baseline. Athena v3 enforces Lake Formation column,
row, and cell filtering on S3 Tables.

**CLI subcommands**: each step of FR-001 runs on its own.

| Command | Mode | Does |
|---------|------|------|
| `lfverify preflight --mode full\|probe` | both | Checks the account allow-list, or for probe mode the read-only runner (`SimulatePrincipalPolicy` over a fixed list of about 40 mutating actions). Checks for a conflicting settings owner (full mode). Saves `GetDataLakeSettings` to `out/<run-id>/settings.before.json`. Checks that no `IAMAllowedPrincipals` grants exist on fixture or target databases (FR-008). |
| `lfverify seed` | full | As the fixture owner (the deployer role), creates fixture tables through Athena DDL, including the partitioned table, then appends known rows through iceberg-go to the S3 Tables Iceberg REST endpoint. This path avoids `awslkhs-cdq`, the Athena INSERT failure. It verifies row counts with `SELECT count(*)`. |
| `lfverify run --matrix <file>` | both | Executes the matrix (see Execution Model) and writes reports. |
| `lfverify detect` | full | For each injected misconfiguration, runs `terraform apply -var inject=<scenario>`, reruns only the affected cases, asserts at least one failure, and reverts. This is the Detection SLO. |
| `lfverify snapshot` / `diff` | probe | Captures and compares the target's state: catalogs, databases, tables, LF-tags, resource tag assignments, data cell filters, `ListPermissions`, and data-lake settings. |
| `lfverify audit --run-id` | probe | Calls CloudTrail `LookupEvents` for sessions `lfverify-<run-id>-*` with `readOnly = false`, and expects zero events. It runs at least 15 minutes after the probe, because of CloudTrail delivery delay. |
| `lfverify cleanup --run-id` | full | Sweeps leftovers by prefix and tag, then restores the data-lake settings from the snapshot. |

**Execution model**: Cases run in phases, so that deny checks are never evaluated before grants
propagate (spec NFR Performance).

1. **Static phase**: all read-only and attempt cases with no shared mutation run in parallel, with
   a worker pool of 8 and per-role session reuse.
   - Positive expectations retry with exponential backoff for up to 120 s.
   - Negative expectations start only after at least 120 s have passed since the last grant change
     (the fixture apply time or the last scenario mutation).
2. **Scenario phase**: runs sequentially.
   - Relabel a table and restore it (FR-017, FR-023).
   - Add a new table after the grant (FR-013).
   - Two row filters for union semantics, then an all-rows filter (FR-018).
   - Each step mutates, waits out the settle window, asserts, and reverts.
3. **Rule-boundary phase**: the test admin attempts each invalid grant in FR-021. An
   `InvalidInputException` is the expected outcome. If a grant is accepted, it is revoked
   immediately and the case is recorded as a failure.

**Outcome classification**: `internal/classify` maps each SDK error or Athena
`StateChangeReason` to one of the outcomes below. Unit tests use recorded error fixtures.

- **DENIED**:
  - `AccessDeniedException`;
  - `AccessDenied`;
  - Athena `Insufficient Lake Formation permission(s)`;
  - `TABLE_NOT_FOUND` or `SCHEMA_NOT_FOUND` for a resource that the manifest says exists;
  - `COLUMN_NOT_FOUND` for a column that the manifest says exists.
- **REJECTED**: `InvalidInputException`, used by FR-021.
- **ALLOWED**: the call succeeded. The tool captures the column list and row count.
- **ERROR**: anything else, such as `INTERNAL_ERROR_QUERY_ENGINE`. ERROR never matches an
  expectation, so the case is reported as "not run: engine error" and the run fails (FR-025).

### Data Storage

**Label style**: the module under test, called from `verify/fixture/platform.tf`.

- `name_prefix = "lfv-<run-id>"`, `environment_type = "Testing"`.
- `governance_config` with these settings:
  - `admin_principal_arns` is the deployer plus the test `admin` role;
  - `read_only_admin_principal_arns` is the test `roadmin` role and, in CI, the probe runner, so
    that probe mode can be cross-checked against full mode;
  - `domain_values = ["general"]`.
- `tenants`:
  - `alpha`, zones `raw` and `curated`;
  - `beta`, zone `curated`;
  - `gamma`, zone `curated`. It is granted to nobody except administrators, and is the control
    namespace.

  All tenants have sensitivity `restricted` and no regulations.
- Module-native personas, declared in tenant config:
  - `writers`: `writer_multi` in alpha and beta, `writer_single` in alpha;
  - `readers`:
    - `reader_multi` in alpha and beta, `reader_single` in alpha, with sensitivity values
      `public` through `restricted`;
    - `reader_cols` in alpha, with `public` through `confidential`, so that columns labeled
      `restricted` are hidden;
    - `reader_rows` and `reader_cells` in alpha, with all sensitivity values;
  - `row_filters` (stage 2): `rows_eu` (`region = 'EU'`) and `rows_us` (`region = 'US'`) for
    `reader_rows`, and `cells_eu` (`region = 'EU'`, with `column_names` excluding `ssn`) for
    `reader_cells`.
- Fixture-owned label grants, in `grants-label.tf`, for personas the module does not model:
  - `meta_reader`: `DESCRIBE` on `DATABASE` and `TABLE` for `tenant ∈ {alpha, beta}`;
  - `meta_writer`: `CREATE_TABLE` on `DATABASE` for `tenant = alpha`, `zone = curated`, plus `ALTER`
    and `DROP` on `TABLE` for the fixture-only LF-tag `lfv-<run-id>-mw = yes`. That tag is assigned
    to exactly one table, `alpha.curated.scratch`.

**Named-resource baseline**: in `verify/fixture/baseline.tf`.

- One table bucket, `lfv-<run-id>-named`, encrypted with the platform KMS key, with three
  namespaces: `n_alpha`, `n_beta`, and `n_gamma`.
- The same tables and rows as the label style. No LF-tags are assigned anywhere in this catalog.
- Grants in `grants-named.tf` reproduce each persona with `database`, `table` (with wildcard where
  "all tables" is intended), and `table_with_columns` (`excluded_column_names = ["ssn"]`) blocks.
  `aws_lakeformation_data_cells_filter` resources are used for the row and cell personas.
- One Athena workgroup, `lfv-<run-id>-named`, with results in the fixture results bucket. It has
  `force_destroy`, SSE-KMS, and a 1 GB scan cutoff.

**Fixture tables**: the same in both styles and every namespace. Rows are deterministic and
generated from a seed, so every expected count and key is known.

| Table | Columns | Rows | Purpose |
|-------|---------|------|---------|
| `orders` | `order_id bigint`, `region string`, `amount decimal(12,2)`, `ssn string` (column label `restricted`), `attrs map<string,string>`, `order_date date` (partition) | 200: 80 EU, 70 US, 50 APAC | Column, row, and cell checks, the partition-column rule, and the map-type filter rule |
| `customers` | `customer_id bigint`, `name string`, `tier string` | 50 | Multi-table read and write checks |
| `scratch` | `id bigint`, `note string` | 5 | Metadata-writer alter and drop target, recreated by `seed` for each run |

**Two-stage apply**: tables must exist before table-level grants, labels, and data cell filters
can be created. The fixture therefore has `variable "stage"`:

- `stage = 1` creates the platform, buckets, namespaces, roles, and database-level grants;
- `seed` then creates the tables and rows;
- `stage = 2` adds the table-level labels, column labels, named table grants, filters, and module
  `row_filters`.

Destroy runs at `stage = 2`.

**Query results**: the module results bucket serves the label style. The fixture results bucket
serves the baseline. Both have `force_destroy = true` (Testing environment type) and lifecycle
expiry.

**Reports**: written locally to `verify/out/<run-id>/`. The directory is in `.gitignore`.

- `report.json` is the full case list.
- `junit.xml` holds one testsuite per persona and style, for CI annotations.
- `report.md` is a summary grouped by persona.

### Networking

No network resources. All calls go to public AWS service endpoints over TLS 1.2+. Both modes use
the SDK defaults and need no VPC.

### Security

**Persona roles**: in full mode, `verify/fixture/personas.tf` creates 13 roles named
`lfv-<run-id>-<persona>`:

- `admin` and `roadmin`;
- `meta_reader` and `meta_writer`;
- `reader_multi` and `reader_single`;
- `writer_multi` and `writer_single`;
- `reader_cols`, `reader_rows`, and `reader_cells`;
- `control`, which has the reader IAM policy and no LF grants;
- `steward`, held in reserve. It is not in the v1 matrix, because 001's T076 already covers it.

Each role:
- trusts only the runner role ARN, with
  `StringLike sts:RoleSessionName = "lfverify-<run-id>-*"`;
- has `max_session_duration = 3600`;
- uses an IAM policy chosen by persona:
  - **readers, writers, metadata personas, and control**: the module's
    `tenants[*].persona_policy_json` output for their tenants, which covers Athena workgroup and
    results-prefix access, plus `glue:Get*` and `glue:Search*` on the S3 Tables catalogs, plus
    `lakeformation:GetDataAccess`. Writers also get `glue:CreateTable`, `glue:UpdateTable`, and
    `glue:DeleteTable`, so that Lake Formation alone decides. For the same reason, the metadata
    writer gets the same Glue write actions;
  - **`admin`**: the `AWSLakeFormationDataAdmin` managed policy, plus Athena on all fixture
    workgroups;
  - **`roadmin`**: the documented read-only administrator policy, written as an
    `aws_iam_policy_document` that includes the explicit `lakeformation:PutDataLakeSettings` deny;
  - **storage bypass (FR-022)**: no persona IAM policy grants `s3tables:GetTableData`,
    `s3tables:GetTableMetadataLocation`, or `s3:GetObject` on table storage. The bypass case
    attempts these calls directly and expects DENIED.

**Fixture owner**: the deployer or runner role, which is already a data-lake admin, as 001
requires. It creates every table, so creator permissions never land on a persona (FR-006). The
`admin` persona is a different role, which keeps the FR-009 implicit-data-access check meaningful.

**Data-lake settings safety**: full mode only.

- Preflight saves the current settings.
- If the current admins include a principal outside `{runner}`, preflight refuses. This means
  another module deployment owns the account integration.
- After destroy, `cleanup` writes back the saved settings with `PutDataLakeSettings` and verifies
  them with `GetDataLakeSettings`. Destroying `aws_lakeformation_data_lake_settings` does not
  restore the earlier values.

**Probe mode**:

- **Runner**: an IAM role with `ReadOnlyAccess`, plus `sts:AssumeRole` on the declared persona
  roles and `iam:SimulatePrincipalPolicy` on itself. The target deployment lists it in
  `read_only_admin_principal_arns`, which needs the module change.
- **Session policy**: each persona is assumed with an inline session policy that allows:
  - `glue:Get*`, `glue:Search*`, `lakeformation:GetDataAccess`, and `lakeformation:List*`,
    `Get*`, and `Search*`;
  - `athena:StartQueryExecution`, `GetQueryExecution`, `GetQueryResults`, and `GetWorkGroup`;
  - `s3:GetObject` and `s3:PutObject` on the persona's results prefix only, because Athena must
    write results;
  - `kms:Decrypt` and `kms:GenerateDataKey` for the results key.

  Because a session policy can only narrow the role's permissions, a write cannot succeed even
  when the persona is allowed to write.
- **Inspection (FR-029)**: the runner, as read-only admin, calls
  `ListPermissions(Principal = persona)`. That returns named grants and `LFTagPolicy` grants. The
  tool resolves tag expressions against `GetResourceLFTags` of each target resource, including
  inherited tags, and compares the effective permission set with the matrix. These cases are
  reported as `verified-by-inspection`.
- **No data values (FR-030)**: query rows are streamed into the predicate evaluator and the column
  collector, and are never logged, stored, or written out. Matrix row predicates are structured
  (`{column, op: in|eq|ne|lt|gt, values}`), not SQL, so the tool evaluates them in memory. A
  predicate on a column the persona cannot see makes the case `not-run: predicate column not
  visible`.
- **Scan cap (FR-031)**: each query uses `LIMIT 10` (configurable) or `count(*)`. The tool sums
  `DataScannedInBytes` from `GetQueryExecution`. Once the total exceeds 1 GB, the remaining query
  cases are reported as not run. The target workgroup's own enforced cutoff applies as well.

**Encryption**: the fixture uses the module's platform KMS key for table buckets and results.
Reports contain no secrets. Session credentials stay in memory only.

**CI credentials**: an optional `.github/workflows/verify-access.yml` (`workflow_dispatch` only)
assumes the sandbox runner role through GitHub OIDC. The trust policy must match the repository's
immutable subject format, `repo:kbrockhoff@1822996/terraform-aws-lakehouse@1417696478:environment:development`,
not `repo:kbrockhoff/terraform-aws-lakehouse:*`.

### Environment Configuration

| Setting | Full mode | Probe mode |
|---------|-----------|------------|
| Target account | Allow-listed sandbox only (`verify/sandbox-accounts.txt` or `LFVERIFY_SANDBOX_ACCOUNTS`) | Any account. The account comes from the runner credentials |
| Module `environment_type` | Fixed `Testing` | The target's own setting |
| Inputs | Generated: `RUN_ID`, `AWS_REGION`, `RUNNER_ROLE_ARN` | `MATRIX=<path>`: personas with role ARNs, catalog, database, and table identifiers, workgroups, expected outcomes and columns, row predicates, and maximum counts |
| Matrix | `verify/matrix/full.yaml`, checked in | Supplied by the target owner. `verify/matrix/probe.example.yaml` is the template |
| Refusal rules | Account not allow-listed; settings owned by another deployment | Runner can mutate (simulation shows any allowed mutating action); runner not a read-only admin |

**Make targets**:

- **Live targets**, which require `check-aws`:
  - `verify-full` runs preflight → apply stage 1 → seed → apply stage 2 → run → destroy → cleanup;
  - `verify-up`, `verify-seed`, `verify-run`, and `verify-down` run the individual steps;
  - `verify-cleanup RUN_ID=…`;
  - `verify-detect`;
  - `verify-probe MATRIX=…`;
  - `verify-probe-audit RUN_ID=…`.
- **Offline targets**: `test-verify` and `test-verify-fixture`.

`verify-full` always runs destroy and cleanup, even when `run` fails, and then exits with the `run`
exit code.

### Complexity Level

**Baseline**: this is test tooling for a disposable sandbox. It needs no high availability,
monitoring, or alarms, and uses local state. Probe mode is Enhanced only in its safety controls
(read-only by construction, a non-interference diff, and the CloudTrail audit), because it may
touch production.

### State Management

- **Full mode**: local backend at `verify/out/<run-id>/terraform.tfstate`, with no locking. Run IDs
  are unique, so runs never share state, and `.terraform` data is kept in the run directory
  (`TF_DATA_DIR`). If the state is lost or the run is interrupted, `make verify-cleanup RUN_ID=…`
  deletes resources by name prefix and tag in dependency order: data cell filters, then
  permissions, LF-tags, tables, namespaces, table buckets, workgroups, results objects and bucket,
  and IAM roles. It then restores the settings snapshot.
- **Probe mode**: has no Terraform state. Snapshots are JSON files in `verify/out/<run-id>/`.

## Project Structure

### Documentation (this infrastructure)

```text
specs/002-lf-access-verification/
├── spec.md              # Infrastructure specification - /iac.specify
├── plan.md              # This file - /iac.plan
├── tasks.md             # Implementation tasks - /iac.tasks
└── checklists/
    └── requirements.md  # Spec quality checklist
```

### Source Code (repository root)

```text
verify/
├── README.md                    # Modes, prerequisites, make targets, matrix format, safety rules
├── go.mod / go.sum              # Separate Go module: github.com/kbrockhoff/terraform-aws-lakehouse/verify
├── sandbox-accounts.txt         # Full-mode allow-list (account-id region)
├── cmd/lfverify/main.go         # Subcommands: preflight, seed, run, detect, snapshot, diff, audit, cleanup
├── internal/
│   ├── matrix/                  # YAML schema, loading, validation, persona/operation expansion
│   ├── identity/                # STS AssumeRole, session naming, probe session policies
│   ├── ops/                     # Operations: catalog (Glue), permissions (LF), query (Athena), storage (S3 Tables/S3), settings
│   ├── classify/                # Error → ALLOWED/DENIED/REJECTED/ERROR mapping
│   ├── predicate/               # Structured row-predicate evaluator (no SQL)
│   ├── runner/                  # Phases, worker pool, retry and settle windows
│   ├── seed/                    # Athena DDL + iceberg-go REST appends, deterministic rows
│   ├── inspect/                 # Effective-permission resolution for probe mode (named + LF-tag)
│   ├── snapshot/                # Target state capture and diff
│   ├── preflight/               # Allow-list, settings owner, read-only runner simulation, open-default check
│   └── report/                  # JSON, JUnit XML, Markdown
├── matrix/
│   ├── full.yaml                # Full-mode expectations: 13 personas × 2 styles
│   └── probe.example.yaml       # Probe-mode template for target owners
├── fixture/                     # Terraform root (full mode)
│   ├── versions.tf              # Terraform >= 1.11, aws >= 6.59.0, < 7.0.0
│   ├── providers.tf             # aws + aws.pricing alias, default_tags LfverifyRun/Purpose
│   ├── variables.tf             # run_id, runner_role_arn, stage, inject
│   ├── locals.tf                # Persona map, tenant map, table definitions
│   ├── platform.tf              # module "lakehouse" { source = "../.." } (label style)
│   ├── personas.tf              # Persona IAM roles and policies
│   ├── grants-label.tf          # Metadata-persona LF-tag policy grants, fixture-only tag
│   ├── labels.tf                # Column/table LF-tag assignments (stage 2)
│   ├── baseline.tf              # Named-style table bucket, namespaces, workgroup, results bucket
│   ├── grants-named.tf          # Named-resource grants and data cell filters (stage 2 for tables)
│   ├── inject.tf                # Misconfiguration overlays for detect (inject = "" by default)
│   └── outputs.tf               # manifest (JSON consumed by lfverify)
└── out/                         # Run output (gitignored)

# Module change (Complexity Tracking #1)
variables.tf                     # governance_config.read_only_admin_principal_arns
main.tf                          # pass-through to modules/catalog-integration
modules/catalog-integration/
├── variables.tf                 # read_only_admin_principal_arns
└── main.tf                      # read_only_admins on aws_lakeformation_data_lake_settings

test/
├── verify_fixture_test.go       # Plan-only: stage 1/2 resources, inject overlays
└── validation_test.go           # + invalid read_only_admin_principal_arns

Makefile                         # verify-* and test-verify* targets; check validates verify/fixture
.github/workflows/verify-access.yml  # workflow_dispatch full-mode run via OIDC (optional)
.gitignore                       # verify/out/
```

**Structure Decision**: The harness lives in a top-level `verify/` directory with its own Go
module and Terraform root.

- It is test tooling, like `test/`. It is not part of the module interface, and its dependencies
  (iceberg-go, extra SDK clients) stay out of `test/go.mod`.
- The fixture is a root configuration rather than an `examples/` entry. Examples are documentation
  for consumers and are planned by `make plan-examples` and the existing tests. The fixture
  creates IAM roles and changes account-level settings, so it must never run through those
  targets.

## Complexity Tracking

| Item | Why needed | Simpler alternative rejected because |
|------|------------|--------------------------------------|
| 1. New module input `governance_config.read_only_admin_principal_arns` (default `[]`) passed to `read_only_admins` | `PutDataLakeSettings` replaces the whole settings object. Because the module sets only `admins`, every apply erases read-only administrators. That defeats FR-010 and leaves probe mode (FR-027, FR-028) without a read-only runner in any module-managed account. It also silently breaks consumers' audit roles. This is a compliance requirement: the audit persona must survive applies. | Setting read-only admins at runtime with `PutDataLakeSettings` from the harness is reverted by the next module apply. It also makes the harness mutate settings in probe mode, which FR-027 forbids. |
| 2. Two-stage fixture apply (`stage` variable) | Table-level grants, column labels, data cell filters, and module `row_filters` need existing tables. Tables are created by `seed` as the fixture owner, because the provider cannot declare a partition spec and creator permissions must not reach personas. | One stage with `aws_s3tables_table` cannot create the partitioned `orders` table that FR-021 needs, and cannot insert rows. Two separate root configs would need `terraform_remote_state` wiring and two states to clean up. |
| 3. Seeding through iceberg-go and the S3 Tables Iceberg REST endpoint instead of Athena `INSERT` | Athena `INSERT` into S3 Tables fails in the sandbox (`awslkhs-cdq`). Fixture rows must exist for every row, cell, and data check (FR-005, FR-018). | Seeding with Athena `INSERT` would block the whole run on the open bug. Writer personas still use Athena DML, because that is the path under test. While `awslkhs-cdq` is open, those cases report `not-run: engine error` and the run fails visibly. |
| 4. Fixture-owned LF-tag grants for metadata-only personas | The module models only stewards, writers, and readers. The spec requires metadata-only readers and writers in both styles (FR-011, FR-012). | Adding metadata personas to the module expands its contract without a consumer request, which Compose, Don't Accumulate forbids. A follow-up bead records the gap. |
| 5. Probe inspection resolves LF-tag expressions itself | `ListPermissions` returns `LFTagPolicy` grants as expressions, not as effective table permissions. FR-029 needs the effective set per resource. | `GetEffectivePermissionsForPath` covers only S3 locations, not catalog resources. Attempting writes in production is forbidden. |

**Risks to confirm in the first sandbox run**:

1. Whether read-only administrators can see the `s3tablescatalog` sub-catalogs. The documentation
   is silent. The matrix starts with the documented expectation (visible) and records what it
   observes.
2. Whether the `admin` persona has implicit `DESCRIBE` or data access on tables the deployer
   created. The documented expectation is none.
3. iceberg-go appends through the S3 Tables REST endpoint with Lake Formation integration enabled.
   The fixture owner holds direct `s3tables` IAM permissions. The fallback is the S3 Tables
   `PutTableData` path or PyIceberg, recorded as a deviation.
4. The Athena error text for column-level denial. The documented behavior is that excluded columns
   are absent from the schema, so `SELECT *` omits them and a named column fails with
   `COLUMN_NOT_FOUND`. `classify` treats both as satisfying "denied or omitted" (FR-017).

**Risk outcomes**, observed in dev-green runs `36ea41` through `513eb0` (2026-11-11 to 2026-11-12):

1. Read-only administrators see the bucket catalogs, as documented.
2. The `admin` persona can `DESCRIBE` tables it did not create but cannot query them. The
   documented expectation of no implicit access was half right; the matrix records both.
3. iceberg-go appends work through the S3 Tables Iceberg REST endpoint with Lake Formation
   integration enabled. No fallback was needed.
4. A named excluded column fails with `COLUMN_NOT_FOUND: Column 'ssn' cannot be resolved or
   requester is not authorized to access requested resources`, and `SELECT *` omits it.

### Deviations found during implementation

Recorded for T057 (2026-11-12). The plan text above is unchanged. Where they disagree, this
section and `verify/README.md` describe what was built.

**Module**

| # | Planned | Built | Why |
|---|---------|-------|-----|
| D1 | One module change (item 1) | A second change: each reader with row filters gets a tenant-owned LF-tag key `<prefix>-tenant-<tenant>-rf-<reader>` (`no`/`yes`). Databases carry `no`, filtered tables `yes`, and that reader's tag grant requires `no`. Spec 001 FR-012 is amended. Commit `bca2b33`, `awslkhs-fysh`. | Run `36ea41` showed module `row_filters` restricted nothing: the reader's tag-based table grant also covered the filtered table, and Lake Formation unions grants. A row-filtered reader saw all 200 rows. |

**Dependencies**

| # | Planned | Built | Why |
|---|---------|-------|-----|
| D2 | `aws-sdk-go-v2` at `test/go.mod`'s minor versions (core v1.42.0) | Core v1.48.0 and current service clients; adds `arrow-go/v18` v18.8.0 and the `kms` client | iceberg-go v0.7.0 requires core >= v1.45.1. Arrow builds the record batches iceberg-go appends; `kms` serves the cleanup sweep (D11). |
| D3 | `gosec` through `make lint-verify`, run when installed | Built later (`awslkhs-46y5`): `make lint-verify` runs gosec v2.30.0 through `go run`, so it needs only Go and is never skipped; `make lint` calls it. | The first scan found 18 issues. Run IDs and persona keys are now validated, three unchecked errors are handled, and 14 operator-path and operator-command findings carry `#nosec` justifications. |

**Fixture**

| # | Planned | Built | Why |
|---|---------|-------|-----|
| D4 | `environment_type = "Testing"` | `"Ephemeral"` | Gives the results bucket `force_destroy` and short retention, which a disposable fixture needs. |
| D5 | Role ARNs passed from `aws_iam_role` attributes | Role ARNs derived from fixture-assigned names (`local.role_arn`); `module.lakehouse` depends on the roles, and every fixture `aws_lakeformation_*` resource depends on `module.lakehouse` | The module keys steward resources by principal ARN, and `for_each` keys cannot be unknown at plan time. Run `92eafa` created a fixture LF-tag before the runner became an admin. |
| D6 | The fixture owner, as data-lake admin, creates tables | New `grants-owner.tf` grants the runner `ALL` with grant option on each database and its tables | Admins get no implicit permissions on databases they did not create. Run `c39bc9` failed with no `CREATE_TABLE`. |
| D7 | Not planned | `seed` grants `DESCRIBE` on each bucket catalog to `<account>:IAMPrincipals` | It is the module README's one-time grant, which the provider cannot express (`awslkhs-qrp`). Without it every Athena query fails with `CATALOG_NOT_FOUND`. |
| D8 | Metadata personas: one LF-tag grant over `tenant ∈ {alpha, beta}` | One grant per tenant | Each tenant has its own tag key, and keys in one expression are AND-ed. |
| D9 | `customers` and `scratch` inherit the tenant's `restricted` label | Labeled `internal` | Relabeling a table that was already restricted to `restricted` changed nothing, so the scenario proved nothing. |
| D10 | Persona roles trust only the runner | An extra `ProbeRunners` statement trusts `var.read_only_runner_arns` for session names `lfverify-*` | Needed for the full/probe cross-check (T051). |

**Teardown and safety**

| # | Planned | Built | Why |
|---|---------|-------|-----|
| D11 | Destroy at `stage = 2`, then `cleanup`. The sweep covers filters, grants, LF-tags, tables, namespaces, buckets, workgroups, results, and roles. | `verify-down`: apply `stage = 1`, `cleanup --tables-only`, `terraform destroy`, then `cleanup`. The sweep adds the account integration (only when the registration role carries the run prefix) and KMS keys tagged with the run, and revokes wildcard table grants without the `ALL_TABLES` name. | Namespaces that still hold tables cannot be destroyed (run `36ea41`). Refresh failed on tag assignments of tables already dropped, and revoking a wildcard grant listed with a name failed (run `24b160`). |
| D12 | Not planned | The runner re-creates and re-seeds `scratch` after the drop cases | Run `790b58`: a dropped `scratch` broke the teardown refresh. |
| D13 | `verify-full` always runs destroy and cleanup | Teardown is skipped when preflight refused: `verify-down` requires `settings.before.json` | T053: after a refused preflight, the `stage = 1` apply would have deployed into the refused account (FR-003). |
| D14 | Preflight refuses any admin outside `{runner}` | Also allows the fixture's own `admin` persona when rerun with a manifest | Preflight reruns after stage 2, when the fixture's admin is already listed. |

**Classification**

| # | Planned | Built | Why |
|---|---------|-------|-----|
| D15 | Denial strings listed under Outcome classification | Adds `insufficient permissions to execute`, `does not have any privilege`, `CATALOG_NOT_FOUND`, `(table\|schema\|database) … does not exist\|not found`, and `column … cannot be resolved` | The first sandbox runs left real denials unclassified. These are the phrasings observed. |

**Probe mode**

| # | Planned | Built | Why |
|---|---------|-------|-----|
| D16 | Inspection calls `ListPermissions(Principal = persona)` | Inspection combines the unfiltered listing, filtered by principal on the client, with `ListPermissions` per table resource. A `TableWithColumns` named `ALL_TABLES` counts as a wildcard. | A principal filter requires a resource. The unfiltered listing omits table-level grants in federated S3 Tables catalogs. |
| D17 | Runner: `ReadOnlyAccess` + `AssumeRole` + `SimulatePrincipalPolicy` | Also `lakeformation:GetDataAccess` | Lake Formation requires it to list permissions on federated databases; `ReadOnlyAccess` lacks it. |
| D18 | Snapshot compares settings as returned | Sorts admins, read-only admins, and trusted owners first, and adds per-table permission listings | Lake Formation returns the principal lists in no stable order, which made unchanged settings differ. Per-table listings close the gap from D16. |
| D19 | The audit expects zero `readOnly = false` events | `StartQueryExecution` and `StopQueryExecution` are counted separately as query starts | CloudTrail marks Athena query starts as not read-only; probe mode runs read-only queries by design. |
| D20 | Subcommands as listed | Adds `lfverify probe-matrix`, which derives a probe matrix from `full.yaml` and a manifest | Needed for the full/probe cross-check (T051). |

**Matrix and coverage**

| # | Planned | Built | Why |
|---|---------|-------|-----|
| D21 | Documented expectations | Changed with dated notes: `admin-add-lf-tags` is denied (admins cannot associate LF-tags another principal created), `admin-exclude-partition` is allowed (the rule applies to Hive partition keys, not Iceberg partition columns), and revoke cases target `scratch`. Gamma-denied cases were added for the writers and the column, row, and cell readers; the matrix has 211 cases. | Observed in run `36ea41`. The revoke and grant cases raced on one target. |
| D22 | Gamma-denied cases prove Lake Formation denial in both styles | In the label style, gamma queries are refused by IAM (no access to gamma's workgroup) before Lake Formation is consulted. The named style still shows the Lake Formation denial. | The module's persona policies scope Athena to the tenant's workgroups. The label-style cases still pass, but they prove the IAM boundary, not the Lake Formation one. |
