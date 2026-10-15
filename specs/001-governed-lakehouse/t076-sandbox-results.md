# T076 Sandbox End-to-End Results

Run 2026-10-14 in the `dev-green` sandbox (account 492637581429, us-east-1), with the
`examples/complete` configuration (Production, three tenants, `name_prefix = "lhsbx"`), real
persona roles, and a standalone `marketing` tenant applied from separate state through an SSM
contract. Fixes found during the run are committed; see plan.md Complexity Tracking.

**Result: 60 checks passed, 0 failed.** Durations, measured with only the platform-scope deployer
role: plan 3 s, apply 83 s (113 resources), destroy 28 s. Targets: 3 minutes and 15 minutes.

## Not verified in this run

| Check | Reason | Tracking |
|-------|--------|----------|
| Row filter limits rows | Athena `INSERT` fails in the sandbox, so tables hold no rows | awslkhs-cdq |
| Query over the cutoff is cancelled | Needs data | awslkhs-cdq |
| Restore to an earlier snapshot within the RPO | Needs committed snapshots | awslkhs-cdq |
| `AWS::S3Tables::Table` data events | The sandbox has no CloudTrail trail; the module creates none (FR-020) | consumer setup |
| Development configuration first | Only Production was applied; Baseline defaults are covered by plan-only tests | — |
| Default table storage class | Not supported by provider 6.69 | awslkhs-oix |

## Checks

```text
PASS  writer creates a curated table through Athena (no encryption stated)
PASS  writer's table inherits the platform bucket key
PASS  writer's table inherits the database labels (tenant, zone, confidential, gdpr=yes, domain)
PASS    ...inherited sensitivity is the tenant ceiling
PASS    ...inherited zone is curated
PASS  control: the finance controller reads a finance-domain table
PASS  the domain-limited finance reader is denied once the table is relabeled to another domain
PASS  a finance reader is denied on sales tables
PASS  a finance reader is denied on the sales/ results prefix
PASS  the semantic layer reads finance consumption tables
PASS  the semantic layer is denied on sales curated tables
PASS  steward creates a consumption table
PASS    ...the table exists (guard for the next check)
PASS  before labeling, the table is invisible to the semantic layer
PASS  steward labels every column and rolls up the table (confidential, gdpr=yes)
PASS  after labeling, the semantic layer sees only public/internal non-GDPR columns
PASS  the GDPR analyst also sees the GDPR column
PASS  steward raises a column to confidential
PASS  a public/internal reader no longer sees the raised column
PASS  lhsbx-sales-steward cannot add or change a tenant key
PASS  lhsbx-sales-steward cannot change the zone key
PASS  lhsbx-sales-steward cannot label restricted
PASS  lhsbx-sales-steward cannot label reg-hipaa = yes
PASS  lhsbx-sales-steward cannot label finance tables
PASS  lhsbx-sales-ingest cannot add or change a tenant key
PASS  lhsbx-sales-ingest cannot change the zone key
PASS  lhsbx-sales-ingest cannot label restricted
PASS  lhsbx-sales-ingest cannot label reg-hipaa = yes
PASS  lhsbx-sales-ingest cannot label finance tables
PASS  steward cannot grant on finance tables
PASS  steward grant to a new principal (tag expression within the tenant) works
PASS  claims steward creates a table in the dedicated bucket
PASS  claims tables are encrypted with the claims key
PASS  claims adjuster queries the claims table
PASS  claims results land in the dedicated bucket under the claims key (lhsbx-claims-athena-results-492637581429/833b0e15-7e6f-4ddc-a0f4-418c0a99a95c.csv.metadata)
PASS  in Production, a writer's DROP TABLE is denied
PASS  unreferenced-file removal enabled on lhsbx-raw
PASS  unreferenced-file removal enabled on lhsbx-curated
PASS  unreferenced-file removal enabled on lhsbx-consumption
PASS  unreferenced-file removal enabled on lhsbx-raw-claims
PASS  unreferenced-file removal enabled on lhsbx-curated-claims
PASS  unreferenced-file removal enabled on lhsbx-consumption-claims
PASS  writer-created table: compaction enabled
PASS  writer-created table: snapshot management enabled
PASS  forced query failure in the sales workgroup
PASS  pipeline creates a marketing table
PASS  standalone tenant reader queries its table end to end
PASS  standalone tenant reader is denied on sales
PASS  platform plan shows no changes after the standalone tenant (exit 0)
PASS  a failed query triggers the sales query_failures alarm
PASS  the alarm notification is delivered to the platform SNS topic (after the key-policy fix)
PASS  steward creates the curated orders table the row filter names
PASS  the row filter (data cells filter and its grant) applies once the table exists (after a provider workaround)
PASS    ...the filter exists with its expression
PASS    ...its reader can query through the filter (row limiting needs data: blocked by awslkhs-cdq)
PASS  the steward's grant appears in CloudTrail
PASS  labeling (AddLFTagsToResource) by tenant personas appears in CloudTrail
### Deployer least privilege
PASS  tenant-scope deployer applies (13 resources, 5 s) and destroys (6 s) the standalone tenant with only its policy, after T052 fixes (GetDataAccess, Glue reads, shared results location, optional contract SSM read)
PASS  standalone tenant offboarding works after the workgroup force_destroy fix
### Deployer platform scope
PASS  platform-scope deployer: clean plan 3 s and apply 83 s (113 resources), destroy 28 s, with only its policies (after the T052 fixes)
```
