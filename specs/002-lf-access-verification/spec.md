# Infrastructure Specification: Data Permission Access Verification Harness

**Spec ID**: `002-lf-access-verification`
**Created**: 2026-11-10
**Status**: Draft (clarified 2026-11-10)
**Input**: User description: "a test application that verifies lake formation configuration by testing access. handle both with and without LF-Tags. Should test lake formation administrators, read-only administrators, catalog metadata-only readers and writers, data readers and writers with access to multiple s3table namespaces and a single namespace, readers with access to only certain columns and/or rows. use the aws mcp server to check the rules for all these"

## Executive Summary *(mandatory)*

This specification defines a test harness that proves a lakehouse's central data permission
configuration works as intended. It does this by acting as each persona and attempting real
operations, rather than by reading grant definitions. The harness assumes a set of test identities,
one per persona. Each identity attempts a fixed catalog of metadata, data, and permission operations
against synthetic fixture tables in managed table storage namespaces. The harness compares every
outcome (allowed or denied, and which columns and rows came back) with a declared expectation and
produces a pass/fail access matrix.

The harness has two modes. **Full mode** deploys the lakehouse module and its own fixtures in a
sandbox, runs every case by attempting it, and tears everything down. **Probe mode** checks an
existing deployment, including production, by using only non-destructive operations, and never
changes it.

The harness covers two grant styles:

- **Label-based**: grants are expressions over permission-service metadata tags (labels) on
  namespaces, tables, and columns. This is the style the `001-governed-lakehouse` platform uses.
- **Named-resource**: grants name specific catalogs, namespaces, tables, and columns.

The same persona expectations must hold in both styles. The only exception is row-level
restrictions, which the permission service supports only as named-resource grants.

The harness covers these personas:

- permission-service administrators;
- read-only administrators;
- metadata-only readers and metadata-only writers;
- data readers and data writers, each with access to several namespaces or to exactly one;
- readers restricted to certain columns, certain rows, or both;
- an unauthorized control identity.

## Problem Statement *(mandatory)*

### Current State

The `001-governed-lakehouse` module is validated by plan-only contract tests. A one-off manual
sandbox run (T076) checked 60 access cases using hand-written commands. That run could not verify
row-level filters because the fixture tables held no rows (tracked as `awslkhs-cdq`). It did not
cover read-only administrators, metadata-only personas, or named-resource grants. No repeatable way
exists to prove that a deployed permission configuration grants exactly the intended access.
Permission rules are subtle, for example:

- a table's creator automatically receives full permissions on it;
- column-restricted access cannot be combined with write permissions on the same table;
- an administrator added after a resource was created has no implicit data access to it.

A configuration can therefore look correct in code while granting too much or too little.

### Desired State

One command deploys a disposable fixture environment, runs the full access matrix for both grant
styles, reports every expected-versus-observed outcome, and tears everything down. Each persona has
positive tests (the access it should have works) and negative tests (everything else is denied). A
regression in the module's grants, or a change in the platform's permission behavior, causes a
failing run with evidence that identifies the operation, the identity, and the response.

### Business Impact

- **Benefits**:
  - Evidence that tenant isolation, column restrictions, and row restrictions actually hold, for
    SOC 2, HIPAA, PCI DSS, and GDPR audits.
  - Over-permissive or under-permissive grants are caught before release instead of in production.
  - It closes the row-filter verification gap left by the T076 run (`awslkhs-cdq`).
  - A documented, executable record of how each persona's permissions behave.
- **Risks if not implemented**:
  - A misconfigured grant silently exposes restricted columns, rows, or another tenant's data.
  - Platform permission-behavior changes go unnoticed until a user reports missing or excess
    access.
  - Audit claims about access control rest on configuration review alone.

## Infrastructure Requirements *(mandatory)*

### Functional Requirements

**Scope and execution**

- **FR-001**: Each mode MUST be runnable as a single on-demand command. In full mode, the command
  provisions fixtures, executes the access matrix, writes the report, and tears down fixtures. Each
  step MUST also be runnable individually so that failures can be investigated. In probe mode, the
  command verifies the runner's permissions, executes the probes, and writes the report.
- **FR-002**: The harness MUST provide two modes.
  - **Full mode** deploys the `001-governed-lakehouse` module in its label-based configuration as
    the system under test. Alongside it, the harness provisions an equivalent named-resource
    configuration as the comparison baseline. Full mode runs the complete matrix in FR-009 to
    FR-023 against fixtures it creates (FR-005 to FR-008), then tears them down.
  - **Probe mode** targets an already-deployed lakehouse that the harness did not create. It
    verifies the deployment's existing persona identities against a declared expectation matrix,
    using only the non-destructive operations defined in FR-027 to FR-031.
- **FR-003**: The harness MUST restrict each mode to these environments.
  - **Full mode** runs only in a dedicated, disposable sandbox or another non-production
    environment. Full mode temporarily changes the administrator lists, so it MUST refuse to start
    when the target is marked as production or holds a tenant with a compliance designation.
  - **Probe mode** may run in any environment, including production.
- **FR-004**: Every full-mode run MUST use a unique run identifier in all fixture names. Concurrent
  runs and leftovers from failed runs therefore never collide, and the teardown step removes
  everything that carries the identifier. Probe-mode runs also carry a run identifier, but only in
  the report and in the audit trail (session names). They create no resources.

**Fixtures (full mode)**

- **FR-005**: The harness MUST provision synthetic fixtures. No real or personal data is used.
  - At least three namespaces in managed table storage, so that "several namespaces" and "exactly
    one namespace" are distinguishable, and at least one namespace is never granted to anyone except
    administrators.
  - In each namespace, at least two open-table-format tables containing committed rows. At least
    one table has a partition column, a column designated restricted, and a column whose values
    allow row filters to select a known subset of rows.
  - A known row count and known values per table, so that results can be checked exactly.
- **FR-006**: Fixture tables MUST be created by a dedicated fixture-owner identity, never by a
  persona under test. The permission service automatically gives a table's creator full permissions
  on it, so a persona that created a table would get false-positive access.
- **FR-007**: The harness MUST provision the fixtures and persona grants in both grant styles, in
  separate catalogs or namespace sets, so that one run covers both:
  - **Label-based**: labels on namespaces, tables, and columns, and grants as label expressions.
    Column restrictions use column labels.
  - **Named-resource**: grants on named catalogs, namespaces, tables, and column include or exclude
    lists. No labels are used to decide access.

  Row and cell restrictions use named data filters in both styles, because label expressions cannot
  express row conditions. The report MUST label these cases as the documented named-resource
  exception.
- **FR-008**: The harness MUST confirm before testing that the fixture resources carry no implicit
  open default permission, that is, no grant that defers access control to identity policies alone.
  It MUST fail the run if one exists, because such a grant would make every access test meaningless.

**Personas and expected access (full mode)**

The harness MUST provide one test identity for each persona below. Each identity has only the
identity-policy permissions needed to call the catalog, permission, and query APIs, so that the
permission service, not the identity policy, decides the outcome. Expected access is defined by the
permission service's published rules, summarized in the Notes.

- **FR-009 Administrator**: The identity is on the permission service's administrator list. Tests
  MUST verify that it can:
  - view all catalogs, namespaces, tables, and permissions;
  - grant and revoke permissions on every fixture namespace and table;
  - create, assign, and remove labels;
  - create and delete data filters.

  Tests MUST also record whether the administrator can read the data of tables created by the
  fixture owner without first granting itself access. The published rule is that it cannot.
- **FR-010 Read-only administrator**: The identity is on the read-only administrator list. Tests
  MUST verify that it can view all catalogs, namespaces, tables, labels, label assignments, data
  filters, and permissions. Tests MUST also verify that every change is denied:
  - granting and revoking;
  - creating, assigning, or removing labels;
  - creating, altering, or dropping namespaces, tables, or data filters;
  - changing permission-service settings.

  Reading table data MUST be denied unless separately granted.
- **FR-011 Metadata-only reader**: The identity has describe-only access to two namespaces and their
  tables. Tests MUST verify that it can:
  - list and describe those namespaces;
  - see the tables in them;
  - read full table schemas.

  Tests MUST verify that it is denied:
  - querying any row;
  - creating, altering, or dropping anything;
  - describing the ungranted namespace.
- **FR-012 Metadata-only writer**: The identity has create-table access in one namespace and alter
  and drop access on one existing fixture table there. Tests MUST verify that it can:
  - create a new table definition in the namespace;
  - alter the schema of the granted table;
  - drop the granted table (on a disposable copy).

  Tests MUST verify that it is denied:
  - reading or writing rows of fixture-owner tables;
  - altering or dropping ungranted tables;
  - creating tables in other namespaces.

  The report MUST record that the writer receives full permissions on tables it creates itself.
- **FR-013 Data reader, several namespaces**: The identity has read access to every table in two
  namespaces. Tests MUST verify that it can:
  - query every row and column of those tables;
  - see a table added to a granted namespace after the grant. In label style, this happens through
    inherited labels; in named style, only if the grant covers all tables in the namespace.

  Tests MUST verify that it is denied:
  - the third namespace;
  - inserting, updating, or deleting rows;
  - altering or dropping tables.
- **FR-014 Data reader, single namespace**: The same as FR-013, but limited to exactly one
  namespace. Tests MUST verify denial on both other namespaces.
- **FR-015 Data writer, several namespaces**: The identity has read, insert, delete, and alter
  access to the tables in two namespaces. Tests MUST verify that it can:
  - insert, update, and delete rows in those tables;
  - read its changes back.

  Tests MUST verify that it is denied:
  - any data or metadata change in the third namespace;
  - changing labels or grants.
- **FR-016 Data writer, single namespace**: The same as FR-015, but limited to exactly one
  namespace. Tests MUST verify denial on both other namespaces.
- **FR-017 Column-restricted reader**: The identity can read a table except its restricted column.
  Tests MUST verify that:
  - the schema the identity sees omits the restricted column;
  - a query that selects only permitted columns returns every row;
  - a query that names the restricted column, or that selects all columns, is denied or omits the
    column, whichever the query engine documents.

  In label style, tests MUST also verify that relabeling another column as restricted removes it
  from the next query, and that restoring the label returns it.
- **FR-018 Row-restricted reader**: The identity can read a table only through a row filter. Tests
  MUST verify that:
  - queries return exactly the expected row subset, compared by count and key values;
  - aggregates computed over the table cover only that subset;
  - when the identity holds two row filters on the same table, it sees the union of the subsets;
  - adding an all-rows filter grants every row.
- **FR-019 Column-and-row-restricted reader**: The identity can read a table through a filter that
  restricts both columns and rows. Tests MUST verify that only the permitted cells are returned,
  and that neither the restricted column nor any filtered-out row is ever exposed.
- **FR-020 Unauthorized control**: The identity has the same identity policy as the readers but no
  data permissions. Every catalog, data, and permission operation in the matrix MUST be denied, or
  MUST return no fixture resources.

**Rule-boundary checks (full mode)**

- **FR-021**: The harness MUST verify that the permission service rejects these invalid grants:
  - column-restricted read access together with insert, delete, alter, or drop on the same table;
  - column restriction on a partition column;
  - a row filter that references an unsupported column type.

  Each rejection is an expected outcome. An accepted grant is a failure.
- **FR-022**: The harness MUST verify that no persona can bypass the permission service. Reading a
  fixture table's underlying storage directly, with the persona's own credentials, MUST be denied
  for every persona other than the fixture owner.
- **FR-023**: The harness MUST verify that label-based access follows the labels. Moving a table's
  label to a value outside a reader's expression removes that reader's access. Restoring the label
  restores it. A table with no labels of its own inherits its namespace's labels.

**Results and evidence**

- **FR-024**: The harness MUST produce a report that contains:
  - one row per persona × grant style × operation × resource, with the expected outcome, the
    observed outcome, pass/fail, and evidence (request identifier and error code, or the returned
    column list and row count);
  - a machine-readable copy for continuous integration;
  - a human-readable summary grouped by persona.
- **FR-025**: The harness MUST exit with a failure status when any case fails, or when a case could
  not be executed. A case that was not executed is reported as "not run" with a reason, never as a
  pass.
- **FR-026**: Expected access MUST be declared as data (a persona-by-operation matrix), separate
  from the test logic, so that adding a persona or operation changes the declaration, not the code.
  Both modes use the same declaration format, and the report states which mode produced it.

**Probe mode**

- **FR-027**: In probe mode, the harness MUST NOT change the target in any way. In particular, it
  MUST NOT:
  - create, alter, or drop namespaces, tables, labels, label assignments, or data filters;
  - grant or revoke permissions;
  - change the administrator lists or permission-service settings;
  - write, update, or delete rows;
  - change identity policies or trust relationships.

  The runner identity MUST be limited to read-only permissions and to assuming the declared persona
  identities, so that a defect in the harness cannot cause a change. The harness MUST verify this
  before it runs any probe, and abort if the runner can change anything.
- **FR-028**: Probe mode MUST verify each declared persona by assuming the deployment's existing
  identity for that persona. Each persona identity must already trust the runner; the harness never
  adds that trust. Probe mode runs only these operations:
  - listing and describing catalogs, namespaces, tables, columns, labels, and filters;
  - reading the persona's own effective permissions;
  - read-only queries that return at most a configured number of rows (default 10), or only
    aggregate counts.

  Each assumed session MUST be scoped down to read-only operations, so that a write cannot succeed
  even when the persona itself is allowed to write.
- **FR-029**: Probe mode MUST verify write, alter, drop, and grant expectations without attempting
  them. It compares the persona's effective permissions, as reported by the permission service,
  with the declared matrix. The report MUST mark these cases as "verified by inspection", unlike
  full mode, which verifies them by attempting the operation.
- **FR-030**: Probe mode MUST verify these restrictions without exposing data:
  - column restrictions, by comparing the visible schema with the declared permitted columns;
  - row restrictions, by checking that every returned row satisfies the declared filter
    predicate, and that the row count does not exceed the declared maximum for that persona.

  Probe reports MUST NOT contain any data values, only column names, row counts, and pass/fail.
- **FR-031**: Probe mode MUST cap the data scanned by each query. The total scan per run MUST stay
  within the probe budget (Cost Constraints). A query that would exceed the cap is reported as "not
  run", never as a pass.

### Non-Functional Requirements

#### Performance

- A full-mode run, including provisioning and teardown and both grant styles, completes in 30
  minutes or less.
- A probe-mode run completes in 10 minutes or less for up to 20 personas.
- Grant changes take time to propagate, so positive expectations are retried for up to 2 minutes
  before the case fails. Negative expectations are evaluated only after the same propagation window
  has elapsed since the last grant change, so that a deny observed before propagation is never
  counted as a pass.

#### Availability

- The harness is an on-demand tool. It has no uptime target.
- A run that is interrupted at any point can be cleaned up by rerunning teardown with its run
  identifier.

#### Security

- Test identities are created for the run, have no long-lived credentials, can be assumed only by
  the harness runner, and are deleted at teardown.
- Test identity policies grant only API reachability. Data access comes only from the permission
  service.
- Fixture data is synthetic, and fixture storage is encrypted, private, and TLS-only, consistent
  with the platform's security floor.
- In full mode, the administrator list and the read-only administrator list are restored to their
  pre-run values at teardown. A run never leaves a test identity on either list.
- Probe mode is read-only by construction (FR-027, FR-028). The runner and the scoped-down persona
  sessions hold no permission that could change the target. Every probe session is named with the
  run identifier, so that the audit trail attributes it to the run.
- Reports contain no credentials and no fixture values beyond the column names, row counts, and key
  values needed as evidence.

#### Scalability

- The persona and operation matrix grows by declaration (FR-026). Adding a persona or a namespace
  does not require code changes.

## Service Level Objectives (SLOs) *(mandatory)*

- **Coverage**: 100% of declared persona × grant style × operation cases are executed in a full run.
  Every case not run is listed with its reason.
- **Determinism**: Repeating a full run against an unchanged configuration yields identical
  pass/fail results in 10 consecutive runs (no flaky cases).
- **Detection**: Every intentionally injected misconfiguration is detected by at least one failing
  case. The injected set is: one extra grant per persona, one missing grant per persona, one removed
  column restriction, one removed row filter, and one restored open default permission.
- **Cleanup**: Zero fixture resources, test identities, grants, labels, or filters carrying the run
  identifier remain 15 minutes after teardown completes.
- **Duration**: A full-mode run completes within 30 minutes. A probe-mode run with up to 20
  personas completes within 10 minutes.
- **Non-interference**: Probe mode makes zero changes. A comparison of the target's catalog,
  permissions, labels, filters, and administrator lists before and after a probe run shows no
  difference, and the audit trail for the run's sessions contains no change events.

## Cost Constraints *(mandatory)*

### Budget

- Each full-mode run costs less than USD 2 in usage charges (queries, storage, and API calls),
  with no standing cost between runs.
- Each probe-mode run scans at most 1 GB of table data in total (less than USD 0.01 at typical
  per-query pricing), and is charged to the workgroups of the tenants it probes.

### Cost Optimization

- Fixtures are kept small (at most a few hundred rows per table), so query scan charges are
  negligible.
- No always-on compute is used. Queries run on pay-per-query serverless capacity.
- Teardown removes all storage, including query results, at the end of every run.

## Compliance Requirements *(include if applicable)*

### Regulatory Frameworks

The harness produces evidence for the access-control controls of SOC 2 (logical access), HIPAA
(minimum necessary access), PCI DSS (restricting cardholder-data access by need to know), and GDPR
(data minimization). It does not itself process regulated data.

### Data Requirements

- Full mode uses only synthetic data. Columns that stand in for regulated fields are clearly fake.
- Probe mode reads real data only to evaluate expectations. It never writes data values to the
  report, logs, or any output (FR-030). Query results produced by probes follow the target
  workgroup's existing encryption and expiration.
- Reports are retained by the caller, for example as continuous-integration artifacts. The harness
  stores nothing after teardown.
- All permission changes made by the harness are recorded in the account's existing audit trail.

## Success Criteria *(mandatory)*

### Code Validation

- [x] All harness infrastructure passes the repository's validation gates (`make pre-commit`,
      `make validate`).
- [x] The expected-access matrix is declared as data and covers every persona in FR-009 to FR-020
      in both grant styles, with the documented row-filter exception.
- [x] Every fixture and identity name includes the run identifier.

### Security Validation

- [x] No HIGH or CRITICAL static-analysis findings, or each one is suppressed with a justification.
- [x] Test identity policies grant API reachability only. A review confirms that no identity policy
      grants direct data access, except the fixture owner's.
- [x] After teardown, the administrator and read-only administrator lists equal their pre-run
      values.
- [x] The storage-bypass checks (FR-022) pass for every persona.
- [x] Probe mode aborts when its runner identity holds any permission that can change the target
      (FR-027).
- [x] Probe reports contain no data values (FR-030).

### Performance Validation

- [x] A full run completes in 30 minutes or less.
- [x] Ten consecutive runs against an unchanged configuration produce identical results.

### Operational Validation

- [x] Each injected misconfiguration listed under Detection makes at least one case fail, with
      evidence that identifies it.
- [x] Running teardown after an interrupted run removes every resource that carries the run
      identifier.
- [x] The row-filter cases previously blocked in T076 (`awslkhs-cdq`) execute and pass.
- [x] A probe-mode run against the full-mode deployment, with the same expectation matrix, produces
      the same pass/fail outcome for every case that both modes can verify.
- [x] A before-and-after comparison of a probe-mode target shows no change (Non-interference SLO).
- [x] Full mode refuses to start against a target marked as production (FR-003).

## Assumptions *(include if making assumptions)*

- In full mode, the target is a single sandbox or non-production account and region, in which the
  runner may change the permission service's administrator lists and catalog settings for the
  run's duration.
- In probe mode, the target deployment's owners supply the expectation matrix and the persona
  identities, and configure those identities to trust the probe runner.
- Production is identified by the deployment's environment type (Production or MissionCritical) or
  by an explicit flag on the target.
- The serverless query engine is the engine used to verify data access, because it enforces column,
  row, and cell restrictions. Other engines are out of scope for v1.
- Fixture tables are written by the fixture owner through the query engine or the table storage
  API, so they contain committed rows before any persona test runs.
- "Several namespaces" means two granted namespaces and one ungranted control namespace. These are
  enough to show cross-namespace grants and isolation.
- The persona definitions follow the permission service's suggested personas. Metadata-only
  personas correspond to describe-only and create/alter/drop-only grants. Data personas correspond
  to select-only (reader) and select/insert/delete/alter (writer) grants.
- Expected outcomes encode the permission service's published rules as of 2026-11. When the
  service's documented behavior changes, the expectation matrix is updated deliberately, not to
  make a failing run pass.

## Out of Scope *(include if explicitly excluding items)*

- Cross-account and cross-region sharing.
- Federated identity-directory principals. Test identities are account-local roles.
- Query engines other than the serverless query engine.
- Views and resource links. Cell-level security is not supported on them.
- Nested-column filtering.
- Performance or load testing of the query engine.
- Probe mode attempting writes, grants, or administrator-list changes, even when they are expected
  to be denied. Probe mode verifies these by inspection only (FR-029).
- Changing the `001-governed-lakehouse` module's grants. The harness reports findings, and fixes
  are made in the module.

## Dependencies *(include if external dependencies exist)*

- An account and region where the managed table storage is integrated with the federated catalog
  and the central data permission service.
- Full-mode runner credentials that can create identities, fixtures, grants, labels, and data
  filters, and update the administrator lists.
- The `001-governed-lakehouse` module, which full mode deploys as the system under test.
- For probe mode, an existing deployment with persona identities that trust the runner, and a
  read-only runner identity.
- The account's existing audit trail, for the permission-change record.

## Notes

- The cloud provider's documentation tool server failed to connect during specification, so these
  rules were checked against the provider's public documentation instead. The pages used were the
  permission service's personas reference, its permissions reference, the managed table storage
  integration and grant guides, and the data filtering limitations:
  - Administrators can grant permissions on every resource, but administrators added after a
    resource was created have no implicit data or describe access to it. They must grant themselves
    access.
  - Read-only administrators can view principals, catalog resources, permissions, labels, and
    filters, but cannot make changes.
  - Describe-only access shows a namespace or a table and its schema. It does not allow queries.
    Describe on a namespace does not reveal which tables it contains.
  - Any other permission on a resource implies describe on it.
  - A table's creator receives every permission on it, with the right to grant them.
  - Column-restricted read access hides the excluded columns from schema reads. It cannot coexist
    with insert, delete, alter, or drop on the same table, and it cannot apply to partition columns.
  - Row filters require the grantor to hold unrestricted read access with the grant option. The
    following rules also apply:
    - multiple filters for one principal combine as a union;
    - an all-rows filter overrides predicate filters;
    - array and map types cannot appear in filter expressions;
    - at most 100 filters can apply per principal per table;
    - filters are not supported on views or resource links.
  - Tables in managed table storage support both label-based and named-resource grants, including
    column and row restrictions.
  - Label expressions cannot express row conditions.
- Any behavior that the documentation leaves unspecified for the managed table storage catalog is
  recorded by the harness as observed behavior, and the expectation is confirmed during the first
  sandbox run. Examples include read-only administrator visibility of that catalog, and whether
  administrators have implicit data access there.
- Clarifications (2026-11-10):
  - Q: What does the harness verify? A: The `001-governed-lakehouse` module in label mode, with a
    harness-provisioned named-resource baseline, plus a non-destructive probe mode for existing
    deployments (FR-002).
  - Q: Where may it run? A: Full mode runs only in sandbox or non-production environments. Probe
    mode may run in any environment, including production, and is read-only (FR-003, FR-027 to
    FR-031).
