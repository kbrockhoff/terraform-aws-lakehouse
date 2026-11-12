# Spec 002 Sandbox Results

Runs 2026-11-11 and 2026-11-12 in the `dev-green` sandbox (account 492637581429, us-east-1), with
`make verify-full`, `make verify-detect`, and `make verify-probe`. Each full run deploys the
`verify/fixture` root, which calls this module with three tenants (`alpha`, `beta`, `gamma`) and
adds the named-resource baseline, 13 persona roles, and seeded synthetic tables. Fixes found during
the runs are committed; see plan.md, Deviations found during implementation.

**Result: 211 cases, 197 pass, 0 fail, 14 not run** (run `77dc1f`, the reference run below). The
14 not-run cases are Athena DML cases blocked by `awslkhs-cdq`. A full run takes about 15 minutes
end to end: 898 s for `77dc1f`, of which the matrix itself took 7 min 17 s. Target: 30 minutes.

## Not verified in these runs

| Check | Reason | Tracking |
|-------|--------|----------|
| Writer `INSERT`, `UPDATE`, and `DELETE` (11 cases) | Athena DML into S3 Tables fails with `INTERNAL_ERROR_QUERY_ENGINE`, so the cases report not run | awslkhs-cdq |
| Reader `UPDATE` and `DELETE` are denied (`rm-update`, `rm-delete`, both styles) | Same engine error; a denial cannot be told apart from the engine fault | awslkhs-cdq |
| Label-style Lake Formation denial on the gamma tenant | Gamma queries are refused by IAM (no access to gamma's workgroup) before Lake Formation is consulted. The named style shows the Lake Formation denial (plan.md D22) | awslkhs-ptos |
| Missing-grant detection in the label style | The label-style grants belong to the module, so `missing_grant` overlays remove only named grants. Label detection is covered by `drop_column_restriction`, `drop_row_filter`, and `open_default` | awslkhs-sr2a |
| Writes and grants in probe mode | By design (FR-029): verified by inspecting effective permissions, never attempted | — |
| Steward persona | Not in the v1 matrix; covered by 001's T076 | — |
| Metadata-only personas as module inputs | The module does not model them; the fixture grants them directly | awslkhs-lfj |
| Bucket-catalog `DESCRIBE` grant from Terraform | `lfverify seed` makes it, because the provider cannot | awslkhs-qrp |
| Static analysis of the Go code (`gosec`) | The planned `make lint-verify` target was not built | awslkhs-46y5 |

## Runs

| Run | Mode | Date | Cases | Pass | Fail | Not run | By inspection | Notes |
|-----|------|------|------:|-----:|-----:|--------:|--------------:|-------|
| `36ea41` | full | 11-11 | 201 | 136 | 12 | 53 | — | First run. Row and cell filters failed (`awslkhs-fysh`); catalog visibility and classifier gaps |
| `24b160` | full | 11-11 | 201 | 181 | 6 | 14 | — | Row and cell filter failures remain (module bug) |
| `790b58` | full | 11-11 | 201 | 187 | 0 | 14 | — | After the row-filter module fix |
| `dede00` | detect | 11-11 | — | — | — | — | — | 17 of 17 misconfigurations detected, about 75 min |
| `2651ed` | probe | 11-11 | 105 | 101 | 0 | 4 | 0 | Probe runner lacked `lakeformation:GetDataAccess` |
| `043438` | probe | 11-11 | 105 | 101 | 2 | 0 | 2 | Table-level grants missing from the unfiltered listing |
| `c9cea7` | probe | 11-11 | 105 | 101 | 0 | 0 | 4 | Cross-check run, against fixture `513eb0` |
| `513eb0` | full | 11-11 | 211 | 197 | 0 | 14 | — | Cross-check run; matrix with gamma-denied cases added |
| `77dc1f` | full | 11-12 | 211 | 197 | 0 | 14 | — | T049 run 1; reference run |
| `d66355`, `3aebc6`, `3215bf`, `166f1f`, `14f318`, `623dbd`, `71a0ea`, `4b3990`, `ddc587` | full | 11-12 | 211 each | 197 | 0 | 14 | — | T049 runs 2–10 |

| `67009c` | recovery | 11-12 | — | — | — | — | — | T052 attempt 1: no leftovers, but cleanup exited 1 on the all-tables grant revoke |
| `86aeab` | recovery | 11-12 | — | — | — | — | — | T052: interrupted after seed, state deleted, swept clean |

Runs `c39bc9` and `92eafa` failed during fixture apply and produced no report; plan.md D5 and D6
record the fixes.

## Results by persona (run `77dc1f`)

| Persona | Label pass | Label not run | Named pass | Named not run |
|---------|-----------:|--------------:|-----------:|--------------:|
| `admin` | 14 | 0 | 13 | 0 |
| `roadmin` | 15 | 0 | 14 | 0 |
| `meta_reader` | 10 | 0 | 10 | 0 |
| `meta_writer` | 8 | 0 | 8 | 0 |
| `reader_multi` | 10 | 2 | 9 | 2 |
| `reader_single` | 5 | 0 | 5 | 0 |
| `writer_multi` | 6 | 4 | 6 | 4 |
| `writer_single` | 4 | 1 | 4 | 1 |
| `reader_cols` | 11 | 0 | 7 | 0 |
| `reader_rows` | 6 | 0 | 6 | 0 |
| `reader_cells` | 6 | 0 | 6 | 0 |
| `control` | 7 | 0 | 7 | 0 |
| **Total** | **102** | **7** | **95** | **7** |

No case failed. The label column includes the scenario cases (relabel, late table, row-filter
union), which run in the label style only.

## Plan risks 1–4

1. **Read-only administrators see the S3 Tables bucket catalogs.** Confirmed in `36ea41`.
2. **The `admin` persona can `DESCRIBE` tables it did not create, but cannot query them.** The
   documented expectation of no implicit access was half right; the matrix records both.
3. **iceberg-go appends work** through the S3 Tables Iceberg REST endpoint with Lake Formation
   integration enabled. Every run seeded all tables this way; no fallback was needed.
4. **Column denial:** a named excluded column fails with `COLUMN_NOT_FOUND: Column 'ssn' cannot be
   resolved or requester is not authorized to access requested resources`, and `SELECT *` omits
   it. Both styles behave the same.

## Row filters (`awslkhs-fysh`, `awslkhs-cdq`)

The row-filter cases blocked in 001's T076 now execute and pass for reads, because `seed` writes
rows through iceberg-go instead of Athena `INSERT`.

- `36ea41` and `24b160`, before the module fix: in the label style, a reader filtered to EU and US
  orders saw all 200 rows, and a cell-filtered reader saw every row and the excluded `ssn` column.
  The reader's tag-based table grant also covered the filtered table, and Lake Formation unions
  grants.
- `790b58` onward, after commit `bca2b33`: `rr-count-union` (EU ∪ US = 150 rows),
  `rr-rows-match`, `rce-count`, `rce-cells`, and `rce-restricted-column` pass in both styles.

`awslkhs-92h` is updated: row-limiting is verified for reads. Its other data checks remain open.

## Detection (`dede00`, Detection SLO)

Every injected misconfiguration made at least one case fail, with the failing case IDs as evidence.

| Overlay | Persona | Failing cases |
|---------|---------|---------------|
| `extra_grant` | `reader_multi` | `rm-select-gamma/named`, `rm-list-databases-gamma/named` |
| `missing_grant` | `reader_multi` | `rm-count-alpha/named`, `rm-count-beta/named`, `rm-select-customers/named` |
| `extra_grant` | `reader_single` | `rs-select-gamma/named` |
| `missing_grant` | `reader_single` | `rs-count-alpha/named` |
| `extra_grant` | `writer_multi` | `wm-select-gamma/named` |
| `missing_grant` | `writer_multi` | `wm-count-alpha/named` |
| `extra_grant` | `writer_single` | `ws-select-gamma/named` |
| `missing_grant` | `writer_single` | `ws-count-alpha/named` |
| `extra_grant` | `reader_cols` | `rc-select-gamma/named` |
| `missing_grant` | `reader_cols` | `rc-schema/named`, `rc-permitted-columns/named`, `rc-customers/named` |
| `extra_grant` | `reader_rows` | `rr-select-gamma/named` |
| `missing_grant` | `reader_rows` | `rr-count-union/named`, `rr-rows-match/named` |
| `extra_grant` | `reader_cells` | `rce-select-gamma/named` |
| `missing_grant` | `reader_cells` | `rce-count/named`, `rce-schema/named`, `rce-cells/named` |
| `drop_column_restriction` | `reader_cols` | `rc-schema`, `rc-restricted-column`, `rc-select-star` (both styles) |
| `drop_row_filter` | `reader_rows` | `rr-count-union`, `rr-rows-match` (both styles) |
| `open_default` | — | Preflight refused: `IAM_ALLOWED_PRINCIPALS holds permissions on …/n_alpha` (FR-008) |

The detect run took about 75 minutes, mostly Terraform applies between overlays (one took 18
minutes). Rerunning the affected cases took about 2 minutes in total. Teardown destroyed 180
resources with no sweep needed.

## Determinism (T049, Determinism SLO)

**Met.** Ten consecutive `make verify-full` runs against an unchanged configuration
(`77dc1f`, `d66355`, `3aebc6`, `3215bf`, `166f1f`, `14f318`, `623dbd`, `71a0ea`, `4b3990`,
`ddc587`, 10:26–13:44 on 11-12) produced identical results. Every run had 197 pass and 14 not run,
and all 211 case IDs had the same status and observed outcome in every run. `513eb0`, from the
day before, also matches. No retry or settle change was needed.

| | Runs | End to end | Matrix |
|---|---|---|---|
| Typical | 9 | 873–937 s | 7–8 min |
| `14f318` | 1 | 3,791 s | 55 min |

`14f318` was slow because the Mac running it went into idle sleep at 11:48 and woke only for brief
maintenance until 12:38 (`pmset` log). The log's two gaps, 11:49–12:23 and 12:24–12:38, match the
sleep periods. Its results were still identical, after nearly an hour asleep with persona sessions
of up to one hour. Excluding sleep, every run met the 30-minute target.

The runs also showed that `report.json` recorded `duration_ns = 0` for every case: the deferred
timer wrote to a copy of the result. That is fixed, with a unit test, so per-case durations and
JUnit times are recorded from now on.

After the tenth run, the account held no `lfv-` resources, no S3 Tables catalog integration, and
empty admin and read-only admin lists, as before the runs.

## Probe mode (T051)

Fixture `513eb0` was applied with a temporary read-only probe runner (`ReadOnlyAccess`,
`AssumeRole` on the personas, `SimulatePrincipalPolicy` on itself, and
`lakeformation:GetDataAccess`), listed as a read-only administrator.

- **Probe run `c9cea7`:** 105 cases, 101 pass, 4 verified by inspection, 0 fail.
- **Full run on the same fixture:** 211 cases, 197 pass, 0 fail, 14 not run.
- **Cross-check:** all 101 cases that both modes verify have the same outcome.
- **Non-interference:** the before and after snapshots of `c9cea7` are identical.
- **Read-only runner (FR-027):** with `lakeformation:GrantPermissions` temporarily added to the
  runner, preflight refused with exit 2, naming the action. The policy was then removed.
- **CloudTrail audit** of `2651ed`, `043438`, and `c9cea7`: 0 change events. Each run made 63
  Athena query starts, which CloudTrail marks as not read-only and the audit counts separately.
- **No data values (FR-030):** probe reports carry column names, counts, and request IDs only.

The probe runner role was deleted afterwards.

## Production refusal (T053, FR-003)

Full-mode preflight refused, with exit 2 and no settings file written, for:

- an empty allow-list;
- the allow-listed account in another Region (us-west-2);
- `LFVERIFY_ENV=Production`;
- `LFVERIFY_ENV=MissionCritical`;
- `LFVERIFY_ENV=Production` with the account allowed through `LFVERIFY_SANDBOX_ACCOUNTS`.

The check found that `verify-full` and `verify-detect` ran teardown after a refused preflight, and
that teardown's stage 1 apply would have deployed into the refused account. `verify-down` now
requires the settings snapshot that only a passing preflight writes (commit `a3d5f61`).

## Interrupted-run recovery (T052, Cleanup SLO)

**Met** (run `86aeab`). The script ran `make verify-up` and `make verify-seed`, then stopped as
if killed, so there was no run and no `verify-down`. It deleted the run's `terraform.tfstate` and
ran `make verify-cleanup RUN_ID=86aeab`. Cleanup took 57 s and exited 0. Fifteen minutes later:

- no IAM role, LF-tag, table bucket, workgroup, S3 bucket, data cells filter, or Lake Formation
  grant with the run prefix remained;
- the `bucket/*` registration and the `s3tablescatalog` federated catalog were gone;
- the only resource still tagged `LfverifyRun=86aeab` was the platform KMS key, pending deletion
  with the 7-day minimum window;
- the data-lake settings equalled `settings.before.json`.

The first attempt, `67009c`, also ended with nothing left and the settings restored, but cleanup
exited 1. With no `terraform destroy` before the sweep, the sweep met grants that earlier runs
never left for it. Lake Formation lists the named style's all-tables SELECT as a column grant on
the table name `ALL_TABLES`, and revoking that form fails with `Table not found`. The grant step
then stopped, skipping the remaining grants; they went away only because their buckets were
deleted. The sweep now revokes that form as a table wildcard and keeps going after a failed
revoke (commit `c90110a`). In `86aeab` every grant was revoked, all-tables SELECT included.

## Settings restore

From `790b58` on, every full and detect run restored the data-lake settings from
`settings.before.json`, `lfverify cleanup` verified them, and the account held no `lfv-<run-id>`
resources afterwards other than KMS keys pending deletion. The two earlier teardowns failed: `36ea41`
left the catalog integration, its registration, and the platform key, removed by hand; `24b160`
failed on the refresh of dropped tables and wildcard revokes. Plan.md D11 records the fixes.

## Cost

Cost Explorer, Amazon Athena in us-east-1, for the run days:

| Date | Data scanned | Cost |
|------|-------------:|-----:|
| 2026-11-11 | 0.71 GB | USD 0.0036 |
| 2026-11-12 (partial; billing data lags) | 0.58 GB | USD 0.0029 |

That covers every run in the Runs table plus the two failed applies, so a run costs well under the
plan's USD 0.10 estimate. The largest single query scanned 16.7 KB; each query bills the 10 MB
minimum. Other cost is KMS: one platform key per run at USD 1 per month, prorated until its
7-day deletion window ends.

## Success criteria review (T059)

Reviewed 2026-11-12. A criterion is ticked in `spec.md` only where this file or the `make validate`
output shows the evidence.

| Criterion | Met | Evidence |
|-----------|-----|----------|
| Validation gates pass | Yes | T055 `make validate`: every step ran, no tool skipped. After the later Go and docs commits, `make pre-commit` and `make test-verify` pass; no `.tf` file changed since T055. |
| Matrix declared as data, covering FR-009 to FR-020 in both styles | Yes | `verify/matrix/full.yaml`; Results by persona lists all 12 matrix personas in both styles. Row filters in the label style are named-resource grants, the documented exception. |
| Every fixture and identity name includes the run ID | Yes | T052: the sweep found every resource by the `lfv-<run-id>` prefix or the `LfverifyRun` tag, and nothing was left. Persona sessions are named `lfverify-<run-id>-<persona>` (visible in denial messages). |
| No HIGH or CRITICAL static-analysis findings | **Partly** | Terraform: Checkov 773 passed, 0 failed; every inline suppression carries a justification. Go: the planned `gosec` scan was not built, so the Go code has no static-analysis evidence (`awslkhs-46y5`). Not ticked. |
| Identity policies grant API reachability only | Yes | Review of `verify/fixture/personas.tf` and the module's persona documents: `s3:GetObject` and `s3:PutObject` are limited to the query results bucket; no persona allows `s3tables:GetTableData`, `GetTableMetadataLocation`, or `PutTableData`. The fixture's only `s3tables:*` statement is a Deny on insecure transport. |
| Admin and read-only admin lists restored after teardown | Yes | Settings restore section; T052 settings match; both lists empty after T049, as before. |
| Storage-bypass checks pass for every persona | Yes | Run `ddc587`: `storage_read` is DENIED and passes for all 12 personas in both styles. |
| Probe aborts when its runner can change the target | Yes | Probe mode: preflight exit 2 naming `lakeformation:GrantPermissions`. |
| Probe reports contain no data values | Yes | `c9cea7/report.json` holds outcomes, column names, row counts, scan bytes, permissions, and error evidence only; none of the seeded region values or `900-00-` SSNs appear. |
| A full run completes in 30 minutes or less | Yes | Determinism: 873–937 s per run. |
| Ten consecutive runs produce identical results | Yes | Determinism: all 211 cases identical across 10 runs. |
| Each injected misconfiguration fails at least one case | Yes | Detection: 17 of 17. |
| Teardown after an interrupted run removes every run resource | Yes | T052 (`86aeab`). The KMS key is scheduled for deletion with the 7-day minimum AWS allows; it cannot be removed sooner. |
| Row-filter cases blocked in T076 execute and pass | Yes | Row filters section: they pass for reads from `790b58` on. |
| Probe and full mode agree on every shared case | Yes | Probe mode: 101 of 101. |
| A probe leaves its target unchanged | Yes | Probe mode: `c9cea7` snapshots identical; CloudTrail audit 0 change events. |
| Full mode refuses a production target | Yes | Production refusal (T053). |

16 of 17 criteria are met. The open one is static analysis of the Go code (`awslkhs-46y5`).
