# lfverify User Guide

`lfverify` checks that the Lake Formation grants on an existing lakehouse deployment give each
persona the access you intend, and no more. It assumes each persona's IAM role and makes real,
read-only Glue, Lake Formation, and Athena calls, then compares every outcome with an expectation
matrix you write. It never changes the deployment, so you can run it against production.

This guide covers the standalone executable published with each release. It describes probe
mode, which needs only the executable, AWS credentials, and a matrix file. Full mode deploys a
disposable test fixture into a sandbox account and runs from a checkout of the repository; see
`verify/README.md` there.

## Contents

- [Install](#install)
- [Prerequisites](#prerequisites)
- [Set up the probe runner role](#set-up-the-probe-runner-role)
- [Let the runner assume each persona](#let-the-runner-assume-each-persona)
- [Write an expectation matrix](#write-an-expectation-matrix)
- [Run a probe](#run-a-probe)
- [Read the results](#read-the-results)
- [Check the audit trail](#check-the-audit-trail)
- [Troubleshooting](#troubleshooting)
- [What probe mode does not do](#what-probe-mode-does-not-do)

## Install

Each release has one archive per platform and a `SHA256SUMS` file:

| Platform | Archive |
|---|---|
| Linux x86-64 | `lfverify_<version>_linux_amd64.tar.gz` |
| Linux ARM64 | `lfverify_<version>_linux_arm64.tar.gz` |
| macOS Intel | `lfverify_<version>_darwin_amd64.tar.gz` |
| macOS Apple silicon | `lfverify_<version>_darwin_arm64.tar.gz` |
| Windows x86-64 | `lfverify_<version>_windows_amd64.zip` |
| Windows ARM64 | `lfverify_<version>_windows_arm64.zip` |

Each archive holds the executable, `LICENSE`, this guide and the example policies in `docs/`, and
`probe.example.yaml`.

1. Download your platform's archive and `SHA256SUMS` from the release page.
2. Verify the checksum. The output must match the archive's line in `SHA256SUMS`.
   - Linux: `sha256sum --ignore-missing -c SHA256SUMS`
   - macOS: `shasum -a 256 --ignore-missing -c SHA256SUMS`
   - Windows PowerShell: `(Get-FileHash .\lfverify_<version>_windows_amd64.zip -Algorithm SHA256).Hash`,
     then compare it with the line in `SHA256SUMS` (case does not matter).
3. Extract the archive and put the executable on your `PATH`:
   - Linux and macOS: `tar -xzf lfverify_<version>_<os>_<arch>.tar.gz`, then move
     `lfverify` to a directory on your `PATH`, such as `~/.local/bin`.
   - Windows: extract the zip and add its folder to your `PATH`, or run `lfverify.exe` by its
     full path.
4. Check it: `lfverify version` prints the release version, commit, and platform.

The executables are not code-signed in this version:

- **macOS**: the first run is blocked because the file is quarantined. After verifying the
  checksum, clear the attribute with `xattr -d com.apple.quarantine ./lfverify`.
- **Windows**: SmartScreen may warn about an unrecognized app. After verifying the checksum, choose
  **More info**, then **Run anyway**, or unblock the zip first with `Unblock-File`.

## Prerequisites

- An existing deployment of the lakehouse module, with the Lake Formation and S3 Tables
  integration enabled.
- The IAM role of each persona you want to check, for example the roles listed in the
  deployment's tenant `readers` and `writers`.
- AWS credentials for the probe runner role (below), in any form the AWS SDK reads:
  `AWS_PROFILE`, environment variables, SSO, or an instance or container role.
- The Region, through `AWS_REGION` or the profile.

## Set up the probe runner role

The runner is the identity `lfverify` runs as. It must be read-only: preflight checks this with
IAM policy simulation and refuses to run otherwise.

1. Create an IAM role, for example `lfverify-probe-runner`, that you can assume.
2. Attach the AWS managed policy `ReadOnlyAccess`.
3. Attach an inline policy based on [`probe-runner-policy.json`](probe-runner-policy.json):
   - `sts:AssumeRole` on the persona roles you will check;
   - `iam:SimulatePrincipalPolicy` on the runner role itself, for the read-only check;
   - `lakeformation:GetDataAccess`, which Lake Formation requires to list permissions on S3
     Tables databases. `ReadOnlyAccess` does not include it.
4. Make the runner a Lake Formation read-only administrator. In the module, add its ARN to
   `governance_config.read_only_admin_principal_arns` and apply. Never add it to
   `admin_principal_arns`: a Lake Formation administrator can change grants, and IAM simulation
   cannot see Lake Formation administrator rights, so preflight would not catch it.

Give the runner nothing else. Preflight simulates about 50 write actions, against `*` and against
the deployment's own resources, and refuses if any is allowed.

## Let the runner assume each persona

Add the statement in [`persona-trust-statement.json`](persona-trust-statement.json) to each
persona role's trust policy, with the runner's ARN. The condition limits the runner to session
names that start with `lfverify-`, so its sessions are easy to find in CloudTrail. `lfverify`
never changes trust policies itself.

## Write an expectation matrix

Start from the template:

```sh
lfverify example-matrix --out probe.yaml
```

Replace the example role ARNs, catalog IDs, databases, tables, and workgroups with your
deployment's. A matrix is YAML; unknown fields are errors, so a typo cannot silently drop a check.

```yaml
mode: probe
defaults:
  query_row_limit: 10          # rows a query may return
  scan_cap_bytes: 1073741824   # total data scanned per run; later queries are not run
personas:
  - key: sales_eu_analyst      # lowercase letters, digits, and _
    role_arn: arn:aws:iam::111122223333:role/sales-eu-analyst
    cases:
      - id: eu-analyst-rows    # unique across the file
        op: select
        resource:
          catalog_id: "111122223333:s3tablescatalog/prod-curated"
          database: sales
          table: orders
          workgroup: prod-sales
        expect: allowed
        row_predicate: { column: region, op: eq, values: [EU] }
        rule: Row filter limits the analyst to EU orders
```

### Resources

| Field | Meaning |
|---|---|
| `catalog_id` | `<account>:s3tablescatalog/<table bucket>` for S3 Tables, or the account ID for the default catalog |
| `database` | The namespace (Glue database) |
| `table` | The table, for table-level operations |
| `workgroup` | The Athena workgroup the persona queries with; required for query operations |
| `columns` | Columns for `select_columns` |

### Operations

| Operation | What it attempts |
|---|---|
| `list_catalogs`, `list_databases`, `list_tables` | List what the persona can see |
| `get_database`, `get_table` | Describe a database or table (`get_table` returns the visible columns) |
| `select`, `select_columns`, `count` | Read-only Athena queries, capped at `query_row_limit` rows |
| `list_permissions`, `get_settings` | Read Lake Formation permissions or settings |
| `insert`, `update`, `delete`, `create_table`, `alter_table`, `drop_table`, `grant`, `revoke`, `add_lf_tags`, `create_lf_tag`, `create_filter`, `put_settings` | Write operations: in probe mode only with `expect: inspect` |

### Expectations

| `expect` | Passes when |
|---|---|
| `allowed` | The call succeeds, and every optional check below holds |
| `denied` | Lake Formation or IAM denies the call, or hides the resource |
| `absent` | The named columns (`absent_columns`), or the listed object, are not returned |
| `inspect` | For write operations: the persona's effective permissions on the table include every entry in `expect_permissions`, for example `[INSERT, DELETE]`. Nothing is attempted |

Optional checks on `allowed` cases:

- `expect_columns`: exactly these columns are visible;
- `absent_columns`: none of these columns are visible;
- `row_predicate`: every returned row satisfies `{column, op, values}`, with `op` one of `eq`,
  `ne`, `in`, `lt`, `gt`. It is evaluated in memory; no value is written out;
- `max_rows`: the row count does not exceed this;
- `expect_rows: {count: N}`: `count` returns exactly N.

Use `rule:` to say which rule a case proves, and `note:` for anything a reviewer should know.
`style: label` or `style: named` only labels the case in reports.

## Run a probe

```sh
export AWS_PROFILE=lfverify-probe-runner   # or other credentials for the runner
export LFVERIFY_PROBE_RESULTS_BUCKETS=prod-athena-results-111122223333
export LFVERIFY_PROBE_KMS_KEYS=arn:aws:kms:us-east-1:111122223333:key/1234abcd-...
lfverify probe --matrix probe.yaml
```

`LFVERIFY_PROBE_RESULTS_BUCKETS` and `LFVERIFY_PROBE_KMS_KEYS` (comma-separated) name the Athena
result buckets and their KMS keys. Each persona session carries a read-only session policy that
allows writing only query results there, so even a writer persona cannot write data.

`lfverify probe` runs these steps and writes everything to `out/<run-id>/` in the current
directory, or to `--out`:

1. `preflight --mode probe`: the runner is read-only and a read-only administrator, and no
   database grants open access to `IAMAllowedPrincipals`;
2. `snapshot --name before`: databases, tables, LF-tags and their assignments, filters,
   permissions, and settings;
3. `run`: every case, as its persona;
4. `snapshot --name after`;
5. `diff`: fails if anything changed.

The steps also run on their own, for example to repeat a run without new snapshots:

```sh
lfverify preflight --mode probe --matrix probe.yaml
lfverify snapshot --matrix probe.yaml --name before --run-id 0a1b2c
lfverify run --mode probe --matrix probe.yaml --run-id 0a1b2c
lfverify snapshot --matrix probe.yaml --name after --run-id 0a1b2c
lfverify diff --run-id 0a1b2c
```

`lfverify <command> --help` lists a command's flags; `lfverify help` lists the commands.

## Read the results

Each run writes to `out/<run-id>/`:

| File | Contents |
|---|---|
| `report.md` | A summary table per persona |
| `report.json` | Every case: expected and observed outcome, status, reason, and evidence (error code and request ID) |
| `junit.xml` | One test suite per persona, for CI systems |
| `snapshot.before.json`, `snapshot.after.json`, `diff.txt` | The before and after state, and their differences |

Case statuses:

| Status | Meaning |
|---|---|
| `pass` | The outcome matched the expectation |
| `fail` | It did not: access is wider or narrower than declared |
| `verified-by-inspection` | An `inspect` case whose effective permissions matched |
| `not-run` | The call failed for a reason other than authorization, such as an engine error or the scan cap; never counted as a pass |

Reports contain column names, row counts, error codes, and request IDs only. Query error text is
withheld in probe mode, because engine errors can quote data values.

Exit status:

| Code | Meaning |
|---|---|
| 0 | Every case passed or was verified by inspection, and nothing changed |
| 1 | A case failed or did not run, the snapshots differ, or an AWS call failed |
| 2 | A usage error, or preflight refused to run |

## Check the audit trail

CloudTrail delivers events up to 15 minutes late. After that, list any non-read-only events from
the run's sessions; `lfverify probe` prints the exact command at the end:

```sh
lfverify audit --run-id 0a1b2c --since 2026-11-12T15:00:00Z
```

Athena query starts are counted separately: CloudTrail marks them as not read-only, but probe
queries only read. Any other event fails the audit.

## Troubleshooting

| Message | Cause and fix |
|---|---|
| `probe runner ... can call mutating actions [...]` | The runner can write. Remove the listed permissions, or check for broad policies such as `AdministratorAccess`. |
| `probe runner ... is not a Lake Formation read-only admin` | Add the runner to `governance_config.read_only_admin_principal_arns` and apply. |
| `simulate runner policy: ... AccessDenied` | Allow `iam:SimulatePrincipalPolicy` on the runner role itself. |
| `ListPermissions ... AccessDeniedException` on S3 Tables databases | Allow `lakeformation:GetDataAccess` for the runner. |
| `assume <persona>: ... AccessDenied` | The persona's trust policy does not allow the runner, or the session-name condition is missing `lfverify-*`. |
| `CATALOG_NOT_FOUND` on every query | The deployment lacks the one-time `DESCRIBE` grant on each bucket catalog for `<account>:IAMPrincipals` (module README, Required One-Time Grants). |
| `IAM_ALLOWED_PRINCIPALS holds permissions on ...` | The database still grants open default access, so identity policies alone decide access and the checks would be meaningless. Revoke it. |
| `not-run ... probe scan cap ... reached` | Raise `scan_cap_bytes`, or use `count` and narrower queries. |
| `probe target changed: N differences` | Something changed the target during the run. Read `diff.txt`; it may be another deployment pipeline, not `lfverify`. |

## What probe mode does not do

- It never writes, grants, revokes, labels, or changes settings, even to show that a write is
  denied. Write expectations are checked by inspecting effective permissions (`expect: inspect`).
- It checks only the resources and personas named in your matrix.
- It reads only through Athena; other query engines are not checked.
- It does not check cross-account or cross-Region sharing.
- Full mode, which deploys a test fixture and attempts every operation, needs a checkout of the
  repository and an allow-listed sandbox account. See `verify/README.md` in the repository.
