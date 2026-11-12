# Access Verification Harness (`lfverify`)

`lfverify` proves that Lake Formation grants on S3 Tables give each persona exactly the access
intended. It does not read grant definitions. It assumes each persona's IAM role and attempts
real Glue, Lake Formation, Athena, and S3 Tables calls, then compares every outcome with a
declared expectation matrix. Specification: [`specs/002-lf-access-verification`](../specs/002-lf-access-verification/spec.md).

It covers two grant styles:

- **Label**: LF-tag expressions. This is the style this module uses.
- **Named**: grants on named catalogs, databases, tables, and columns.

It covers these personas:

- `admin` and `roadmin` (read-only administrator);
- `meta_reader` and `meta_writer` (metadata only);
- `reader_multi`, `reader_single`, `writer_multi`, and `writer_single`;
- `reader_cols`, `reader_rows`, and `reader_cells` (column, row, and cell restrictions);
- an unauthorized `control`.

## Modes

| | Full mode | Probe mode |
|---|---|---|
| Target | A disposable fixture in an allow-listed sandbox account | An existing deployment, production included |
| Changes | Deploys and destroys the fixture; temporarily owns the data-lake settings | None, by construction |
| Writes, grants, drops | Attempted | Verified by inspecting effective permissions |
| Data in reports | Synthetic fixture data, column names and counts only | Column names and counts only |
| Command | `make verify-full` | `make verify-probe MATRIX=…` |

### Full-mode safety rules

- **Allow-listed accounts only.** Full mode runs only in accounts listed in
  [`sandbox-accounts.txt`](sandbox-accounts.txt) or in `LFVERIFY_SANDBOX_ACCOUNTS`. It refuses
  when `LFVERIFY_ENV` is `Production` or `MissionCritical`.
- **No foreign admins.** Full mode refuses when the account's data-lake admins include anyone
  other than the runner, because that means another deployment owns the catalog integration.
  The fixture deploys this module with `catalog_integration_config.enabled = true`, so the
  account must not hold another module deployment.
- **Settings are restored.** Preflight saves the data-lake settings to
  `out/<run-id>/settings.before.json`. Teardown writes them back and verifies them.
  Destroying the settings resource does not restore earlier values.
- **Teardown always runs.** `verify-full` tears down even when the run fails. The exception is a
  run that preflight refused: nothing was deployed, so `verify-down` stops without touching
  Terraform. It requires the `settings.before.json` that only a passing preflight writes.
- **Prefixed names.** Every fixture name starts with `lfv-<run-id>`, and every fixture resource
  carries the `LfverifyRun` tag.

### Probe-mode safety rules

- **Read-only runner.** The runner must be read-only in IAM. Preflight simulates about 40
  mutating actions and aborts if any is allowed. The runner must also be listed in the target
  deployment's `governance_config.read_only_admin_principal_arns`.
- **Existing trust only.** Persona roles must already trust the runner for session names
  `lfverify-*`. The harness never edits trust policies.
- **Read-only sessions.** Every persona session carries a read-only session policy, so even a
  writer persona cannot write. Athena still needs to store results: set
  `LFVERIFY_PROBE_RESULTS_BUCKETS` and `LFVERIFY_PROBE_KMS_KEYS` (comma-separated) to the
  workgroups' result buckets and keys.
- **Capped scans.** Queries return at most `query_row_limit` rows, and the run stops querying
  after `scan_cap_bytes`.
- **Proof of no change.**
  - `snapshot` captures the catalogs, LF-tags, assignments, filters, permissions, and settings
    before and after the run, and `diff` fails on any difference.
  - `make verify-probe-audit RUN_ID=…` lists non-read-only CloudTrail events from the run's
    sessions. Query starts (`StartQueryExecution`) are counted separately rather than treated as
    changes: CloudTrail marks them as not read-only, but probe mode runs read-only queries by
    design. Run it at least 15 minutes after the probe, because CloudTrail delivers events
    late.

## Prerequisites

- Go 1.26, Terraform >= 1.11, the AWS CLI, and `openssl`.
- An AWS session (`AWS_PROFILE=…`) for the runner:
  - **Full mode**: the runner becomes the fixture owner and a data-lake admin, and needs to
    create IAM roles, S3 Tables, Glue, Lake Formation, Athena, S3, and KMS resources.
  - **Probe mode**: the runner needs `ReadOnlyAccess`, `sts:AssumeRole` on the persona roles,
    `iam:SimulatePrincipalPolicy` on itself, and `lakeformation:GetDataAccess`. Lake Formation
    requires `GetDataAccess` to list permissions on federated S3 Tables databases;
    `ReadOnlyAccess` lacks it.
- Optional: `RUNNER_ROLE_ARN` when the role running Terraform is not the runner. By default the
  fixture uses the session's role.

## Make targets

| Target | Does |
|---|---|
| `make test-verify` | Offline vet and unit tests. Part of `make validate` |
| `make lint-verify` | gosec, pinned and run through `go run`. Part of `make lint`. Every finding fails; a suppression needs `#nosec <rule> -- <reason>` |
| `make test-verify-fixture` | Plan-only Terratest of the fixture at both stages and with overlays. Part of `make test` |
| `make verify-full` | New run ID → preflight → apply stage 1 → seed → apply stage 2 → run → destroy → cleanup |
| `make verify-up`, `verify-seed`, `verify-run`, `verify-down` | The same steps one at a time. They use the last run ID, or `RUN_ID=…` |
| `make verify-detect` | Applies each misconfiguration overlay and requires at least one failing case (Detection SLO) |
| `make verify-cleanup RUN_ID=…` | Sweeps an interrupted run by prefix and restores the settings |
| `make verify-probe MATRIX=…` | Probe preflight → snapshot → run → snapshot → diff |
| `make verify-probe-audit RUN_ID=…` | CloudTrail change events from the probe's sessions |

`make validate` and `make test` never run the live `verify-*` targets.

The matrix's drop cases remove `scratch` at the end of a run; the runner then re-creates and
re-seeds it, so the fixture stays whole for teardown. Its labels and the metadata writer's grant
on it are not restored, so use `verify-full` rather than repeated `verify-run` for clean runs.

## Matrix format

Matrices are YAML. Unknown fields are errors, so a typo cannot silently drop an expectation.
See [`matrix/full.yaml`](matrix/full.yaml) and [`matrix/probe.example.yaml`](matrix/probe.example.yaml).

```yaml
mode: full            # or probe
defaults:             # optional; these are the defaults
  settle_seconds: 120          # wait after the last fixture apply before negative checks
  scenario_settle_seconds: 30  # wait after each scenario setup before negative checks
  positive_retry_seconds: 120  # retry window for positive checks (grant propagation)
  query_row_limit: 10          # probe mode
  scan_cap_bytes: 1073741824   # probe mode
personas:
  - key: reader_cols           # full mode: a fixture persona; probe mode: also role_arn
    cases:
      - id: rc-schema
        style: both            # label, named, or both (default)
        phase: static          # static (default, parallel), scenario, or boundary
        op: get_table
        resource: { tenant: alpha, table: orders }   # probe: catalog_id, database, table, workgroup
        expect: allowed        # allowed, denied, rejected, absent, or inspect (probe)
        expect_columns: [order_id, region, amount, attrs, order_date]
        rule: GetTable returns only the columns the caller may see
```

The operations are:

- **catalog**: `list_catalogs`, `list_databases`, `get_database`, `list_tables`, `get_table`;
- **query**: `select`, `select_columns`, `count`;
- **data writes**: `insert`, `update`, `delete`;
- **metadata writes**: `create_table`, `alter_table`, `drop_table`;
- **permissions**: `grant`, `revoke`, `add_lf_tags`, `create_lf_tag`, `create_filter`,
  `list_permissions`;
- **settings**: `get_settings`, `put_settings`;
- **storage bypass**: `storage_read`;
- **rule boundaries**: `grant_columns_with_write`, `grant_exclude_partition`,
  `create_filter_map_column`.

Optional checks are:

- `expect_columns` and `absent_columns`;
- `expect_rows` (`count`, plus `key_column`/`keys` in full mode only);
- `row_predicate` (`{column, op: eq|ne|in|lt|gt, values}`), evaluated in memory;
- `max_rows`;
- `expect_permissions`, used with `expect: inspect`;
- `setup`, a scenario mutation that is applied and reverted around the case:
  `relabel_column_restricted`, `relabel_table_restricted`, `late_table`, or `all_rows_filter`.

### Updating expectations

Each expectation encodes a documented Lake Formation rule, cited in `rule:`. When AWS behavior
changes, or a first run shows that an undocumented case behaves differently, change the
expectation deliberately and add a `note:` with the observation and its date. Never loosen an
expectation just to make a run pass. Fix the fixture or the module instead.

## Reports

Each run writes `verify/out/<run-id>/`:

- `report.json`: every case, with its expected and observed outcome, status, reason, and
  evidence (error code and request ID).
- `junit.xml`: one test suite per persona and style. Failures are JUnit failures, and cases that
  did not run are JUnit errors.
- `report.md`: a summary table per persona.
- `detect.json`, `snapshot.*.json`, `diff.txt`: written by `detect`, `snapshot`, and `diff`.

The statuses are `pass`, `fail`, `not-run`, and `verified-by-inspection`. Any `fail` or
`not-run` exits 1. A case is `not-run` when its call fails for a reason other than
authorization, such as `INTERNAL_ERROR_QUERY_ENGINE`, so an engine fault never counts as a pass.

## Interrupted runs

`make verify-cleanup RUN_ID=<id>` works even without Terraform state. It deletes, in order:

1. data cells filters;
2. grants to `lfv-<id>-*` roles;
3. `lfv-<id>-*` LF-tags;
4. table buckets, with their tables and namespaces;
5. workgroups;
6. results buckets;
7. the account integration (the `bucket/*` registration and the `s3tablescatalog` federated
   catalog), but only when the registration's role carries the run prefix, so another
   deployment's integration is never touched;
8. persona roles;
9. KMS keys tagged `LfverifyRun=<id>`, scheduled for deletion with the 7-day minimum window.

It then restores the data-lake settings from `settings.before.json`.

## Behavior confirmed in the first sandbox run

The plan lists four behaviors the documentation leaves open (plan.md, Risks). The matrix marks
the affected cases with `note: confirm`:

1. Whether read-only administrators can see the S3 Tables bucket catalogs.
2. Whether a data-lake admin who did not create a table has implicit DESCRIBE or SELECT on it.
3. Whether iceberg-go appends work through the S3 Tables Iceberg REST endpoint.
4. The Athena error text for column-level denial. `select *` omitting the column and
   `COLUMN_NOT_FOUND` both count as denied.

Two open issues affect expected results:

- `awslkhs-cdq`: Athena `INSERT` into S3 Tables fails in the sandbox, so writer
  data cases report `not-run`.
- `awslkhs-fysh` (fixed): before the fix, a row-filtered reader in the label style also held the
  module's tag-based table grant, so row and cell filters restricted nothing. Runs `36ea41` and
  `24b160` recorded the failures.
