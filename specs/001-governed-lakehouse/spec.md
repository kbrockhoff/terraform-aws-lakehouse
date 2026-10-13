# Infrastructure Specification: Governed Lakehouse Foundation

**Spec ID**: `001-governed-lakehouse`
**Created**: 2026-10-13
**Status**: Draft
**Input**: User description: "lakehouse which uses S3 tables for storage, Lake Formation for fine-grained access, Athena for querying. For the s3tablesdatacatalog, manage settings from this module"

## Executive Summary *(mandatory)*

This specification defines a reusable infrastructure module that provisions a governed lakehouse
foundation in a single cloud region. Analytical data is stored as open table format tables in a
managed table storage service, discovered through the region's federated table catalog (whose
settings this module owns), protected by a centralized data permission service with table, column,
and row-level controls, and queried through a serverless, pay-per-query SQL engine. A single
`environment_type` selection scales security, durability, monitoring, and cost controls from
low-cost development sandboxes to compliance-ready production deployments supporting SOC 2, HIPAA,
PCI DSS, and GDPR workloads.

## Problem Statement *(mandatory)*

### Current State

The repository contains a module template that provisions only an encryption key, an optional alarm
notification topic, and a cost estimate. There is no storage for analytical tables, no catalog,
no data-access governance, and no query capability. Teams wanting a lakehouse must assemble
storage, catalog integration, permissions, and query configuration by hand for each account and
environment, producing inconsistent security postures and unpredictable costs.

### Desired State

One module call produces a working, governed lakehouse: data zones backed by managed open-format
table storage with automatic table maintenance, the regional federated catalog configured to
expose those tables to analytics services, every data access decided by one central permission
model, and query workgroups with enforced encryption, result expiration, and scan limits. Defaults
derive from `environment_type`; consumers supply only a name prefix, tags, and the principals that
need access.

### Business Impact

- **Benefits**: Days of manual setup reduced to one reviewed deployment; consistent,
  auditable access control for regulated data; no idle compute cost; query spend bounded by
  enforced scan limits; table maintenance automated rather than operated by hand.
- **Risks if not implemented**: Ad hoc storage-level permissions that bypass central governance,
  uncontrolled query costs, unmaintained tables that degrade query performance and inflate storage,
  and audit findings for missing encryption, access logging, or erasure capability.

## Infrastructure Requirements *(mandatory)*

### Functional Requirements

**Table storage**

- **FR-001**: Infrastructure MUST provide managed table storage for open table format tables, with
  one storage container per data zone. The default zones are `raw` and `curated`; consumers MAY
  override the zone list. Consumption-ready data products are served by an external semantic layer
  (see Out of Scope).
- **FR-002**: Infrastructure MUST create the namespaces consumers declare for each zone. Table
  creation and schema management are left to data pipelines (see Out of Scope).
- **FR-003**: Infrastructure MUST enable automatic unreferenced-file removal for each zone, with
  retention windows set by environment type. File compaction and snapshot expiration are
  configured per table by the table storage service and are enabled by default on every new
  table. The module creates no tables, so it MUST document that writers keep both enabled. The
  service's default snapshot retention MUST be at least the RPO of every environment type.

**Catalog integration**

- **FR-004**: Infrastructure MUST integrate the managed table storage with the region's federated
  table catalog so all tables in all zones are discoverable by the query engine and the central
  permission service without manual steps.
- **FR-005**: Infrastructure MUST manage the federated table catalog's settings from this module,
  including enrolling it in central permission governance. Because the catalog is a single shared
  resource per account and region, this management MUST be controlled by a feature flag. Exactly
  one deployment per account and region enables the flag; other deployments in that account and
  region disable it and reference the existing catalog.

**Governed access**

- **FR-006**: Infrastructure MUST register the managed table storage with the central data
  permission service so that table-data access is authorized there, not by storage-level policies.
- **FR-007**: Infrastructure MUST grant data permissions only to principals supplied as inputs,
  grouped by persona:
  - **Data administrators**: manage permissions within the lakehouse.
  - **Writers**: data pipelines that create and modify tables in assigned zones. In Enhanced
    environment types, writers cannot drop tables, because a dropped table and its snapshots
    cannot be recovered; dropping is reserved for data administrators.
  - **Readers**: analysts, applications, and the external semantic layer, with read-only access to
    assigned zones or namespaces.
- **FR-008**: Infrastructure MUST support fine-grained access (column-level selection and
  row-level filters) for reader grants, and support classification-label-based grants for
  Enhanced environment types.
- **FR-009**: Infrastructure MUST manage the account-level data permission service settings,
  controlled by the same feature flag as federated catalog management (FR-005). These settings are
  the data-lake administrator list and the default permissions for newly created databases and
  tables. When enabled, the module MUST:
  - set the administrator list to exactly the consumer-supplied administrator principals;
  - remove the implicit open default permissions on new databases and tables, so access is granted
    only through explicit grants.

  When the flag is disabled, the module MUST NOT modify these settings.

**Query engine**

- **FR-010**: Infrastructure MUST provide at least one serverless query workgroup. The workgroup
  configuration MUST be enforced, so clients cannot override the result location, encryption, or
  scan limit.
- **FR-011**: Infrastructure MUST provide a dedicated query-results object storage location. It
  MUST be encrypted with the module's key, block all public access, deny non-TLS requests, and
  expire results automatically after an environment-type-dependent period.
- **FR-012**: Infrastructure MUST enforce a per-query data-scanned limit on every workgroup, with
  defaults determined by environment type.
- **FR-012a**: Query results MUST be isolated per workgroup. Each workgroup writes to its own
  results location, and each persona is bound to specific workgroups, so a principal can read
  results only from the workgroups it is bound to.

**Cross-cutting**

- **FR-013**: Infrastructure MUST encrypt table storage and query results with a customer-managed
  key. The module creates the key by default, or uses a consumer-supplied key.
  When a consumer-supplied key is used, the module cannot change that key's policy, so it MUST
  output the key policy statements the key needs for table maintenance, catalog data access, and
  query results.
- **FR-014**: Infrastructure MUST rely on an existing account-wide or organization-wide audit
  trail for data-access and permission-change records, and MUST NOT create its own audit trail.
  The module documents the audit events and retention its consumers' trail must capture to meet
  the compliance requirements below.
- **FR-015**: Infrastructure MUST derive defaults for table maintenance retention, query-result
  expiration, scan limits, key deletion window, monitoring, and alarms from `environment_type`.
  `None` uses explicit configuration values.
- **FR-016**: Infrastructure MUST apply common tags to all resources, and additional data tags to
  all data storage resources. In Enhanced environment types, the plan MUST fail unless the common
  tags include a cost center and a data owner, and the data tags include a data classification
  whose value is one of the configured classification values. That data classification represents
  the most sensitive classification of any namespace in the deployment.
- **FR-017**: When monitoring or alarms are enabled, infrastructure MUST raise an alarm on query
  failures and on scanned-data volume exceeding a configurable threshold. Alarms MUST route to the
  module's alarm notification topic.
- **FR-018**: Infrastructure MUST extend the monthly cost estimate to cover table storage, table
  maintenance, the query-results location, and alarms.
- **FR-019**: Infrastructure MUST extend the deployer least-privilege policy to cover every
  resource type this module manages.
- **FR-020**: Infrastructure MUST expose outputs for composition: zone storage identifiers, catalog
  and namespace names, workgroup names, the query-results location, and the encryption key
  identifier. Setting `enabled = false` MUST create no resources.

**Compliance designation**

- **FR-021**: Infrastructure MUST accept a compliance designation input. It lists zero or more of
  SOC 2, HIPAA, and PCI DSS that the deployment's data falls under, and defaults to empty.
- **FR-022**: Any infrastructure that incurs additional cost and exists only to satisfy SOC 2,
  HIPAA, or PCI DSS MUST be created only when the designation includes the corresponding
  framework. Each such resource MUST be traceable to the framework(s) that require it. With an
  empty designation, no compliance-only cost-incurring resources are created in any environment
  type. Currently this covers:
  - a dedicated customer-managed encryption key per data zone and for query results (isolation
    of regulated data);
  - any compliance-only resources introduced by later requirements, which follow the same rule.
- **FR-023**: Controls that add no infrastructure cost MUST apply regardless of designation.
  These are encryption with the module's customer-managed key, TLS-only access, public-access
  blocking, least-privilege and fine-grained grants, deletion protection, regional residency, and
  GDPR erasure support.
- **FR-024**: When the designation is non-empty and the environment type is Ephemeral,
  Development, or Testing, infrastructure MUST surface a plan-time warning. The plan still
  proceeds, so compliance features can be tested, but regulated data must not be loaded there.

### Non-Functional Requirements

#### Performance

- A newly created table in any zone is queryable by an authorized reader within 5 minutes of
  creation, with no manual catalog or permission steps beyond the module's grants.
- Automatic compaction runs on every table (the service default) so that query performance does
  not degrade as small files accumulate. The target file size follows the service default unless
  a writer overrides it.
- `terraform plan` for the complete example completes in under 3 minutes; a first full deployment
  completes in under 15 minutes.

#### Availability

- Availability equals the regional managed services' published availability. There are no
  self-managed or single-instance components.
- Recovery objectives follow `environment_type`. Ephemeral has no RPO and a 48-hour RTO.
  Development and Testing: 24h RPO, 48h RTO. UAT: 12h RPO, 24h RTO. Production: 1h RPO, 4h RTO.
  MissionCritical: 5m RPO, 1h RTO.
- Table snapshots are retained long enough to restore any table to a point within the RPO window.
  Enhanced environment types protect storage and keys against accidental deletion.

#### Security

- All table data and query results are encrypted at rest with a customer-managed key.
- All requests use TLS 1.2 or higher. Non-TLS requests to storage are denied.
- No lakehouse storage is publicly accessible.
- Table-data access is authorized only through the central permission service. No resource policy
  grants a wildcard principal.
- Principals receive only the zones, namespaces, tables, columns, and rows their persona requires.
- Every data access and permission change is recorded in the existing tamper-resistant audit
  trail (FR-014).
- Regulated data (PHI, cardholder data, personal data) is not loaded into Baseline environment
  types: Ephemeral, Development, and Testing (FR-024).
- Cost-incurring SOC 2, HIPAA, and PCI DSS controls are active exactly when the compliance
  designation includes the framework (FR-021 to FR-023).

#### Scalability

- Each zone supports at least 100 namespaces and 1,000 tables without module changes.
- Zones, namespaces, workgroups, and principal grants are data-driven inputs. Adding one changes
  only the resources for that item.
- No component uses provisioned capacity. Storage and query capacity scale with usage.

## Service Level Objectives (SLOs) *(mandatory)*

- **Availability**: Lakehouse query and storage availability ≥ 99.9%, measured monthly from the
  provider's service health data for the deployment region.
- **Durability**: Zero loss of committed table data, verified by restoring a table to a snapshot
  within the RPO window during operational validation.
- **Recovery**: Point-in-time table restore achieved within the environment type's RTO, using
  retained snapshots.
- **Access provisioning**: A new principal added to a persona input has working access after one
  `terraform apply`, with no manual console steps (100% of grants).
- **Cost control**: 0 queries exceed the workgroup scan limit, verified from query-engine metrics
  over a 30-day window.

## Cost Constraints *(mandatory)*

### Budget

- **Baseline (Ephemeral, Development, Testing), no compliance designation**: idle monthly cost of
  at most $5, excluding stored data and query scans. This is roughly the cost of one encryption key
  plus negligible storage.
- **Enhanced (UAT, Production, MissionCritical), no compliance designation**: idle monthly cost of
  at most $10, excluding stored data and query scans. This adds alarms and notifications.
- **Compliance designation**: adds at most $10 per month idle on top of the environment budget,
  for the compliance-only resources in FR-022. The cost estimate output reports this amount
  separately.
- Usage-driven costs (stored data, data scanned, maintenance operations) are reported by the cost
  estimate output, not capped by budget.

### Cost Optimization

- Pay-per-use services only. No always-on compute or provisioned capacity.
- Per-query scan limits by default: 10 GB for Baseline, 1 TB for Enhanced. Both are configurable.
- Query results expire after 7 days in Baseline and 30 days in Enhanced.
- Automatic compaction and unreferenced-file removal keep storage and scan volume from growing
  with stale files. Snapshot retention is no longer than the RPO requires plus a safety margin.
- Ephemeral environments allow forced teardown, so abandoned resources do not keep accruing cost.
- Compliance-only cost-incurring infrastructure is gated by the compliance designation, not by
  environment type, so non-regulated production deployments do not pay for it (FR-022).

## Compliance Requirements *(include if applicable)*

### Regulatory Frameworks

SOC 2 Type II, HIPAA, PCI DSS, GDPR.

The module supports all four, but cost-incurring SOC 2, HIPAA, and PCI DSS controls activate only
for frameworks listed in the deployment's compliance designation (FR-021, FR-022). GDPR controls
add no infrastructure cost and always apply (FR-023).

### Data Requirements

- **Residency**: All lakehouse data, catalog metadata, query results, and audit records stay in the
  deployment region. Cross-region replication is not enabled by this module.
- **Classification**: Data storage resources carry data-classification tags. Enhanced environment
  types support classification-label-based access grants (FR-008).
- **Isolation**: Regulated data is placed in dedicated namespaces with dedicated reader grants.
  When the compliance designation is non-empty, each zone and the query results use a dedicated
  encryption key (FR-022).
- **Erasure (GDPR)**: Tables support row-level deletion. Snapshot expiration and unreferenced-file
  removal physically purge deleted records within 30 days of deletion in Enhanced environment
  types.
- **Audit retention**: The existing audit trail must keep records for at least 90 days in
  Baseline. When the compliance designation is non-empty, it must keep them for at least 1 year,
  with longer retention for HIPAA documentation requirements. This retention is a consumer
  prerequisite, not configured by this module.

## Success Criteria *(mandatory)*

### Code Validation

All code validation runs through `make` targets. Do not invoke `terraform`, `tflint`,
`terraform-docs`, or `go test` directly.

- [ ] `make pre-commit` passes: it runs `format`, `check`, `lint`, and `docs`. Running it leaves no
  uncommitted changes from formatting or documentation regeneration.
- [ ] `make check` passes: formatting is clean and `validate` succeeds for every submodule and
  example.
- [ ] `make lint` passes with TFLint installed. The target only warns and skips if TFLint is
  missing, so a skip does not count as a pass.
- [ ] `make docs` runs with terraform-docs installed and the generated README sections are
  current. A skip because terraform-docs is missing does not count as a pass.
- [ ] `make test` passes. Plan-only tests can also be run one at a time with `make test-defaults`,
  `make test-complete`, and `make test-enabled-false`.
- [ ] Tests run by `make test` confirm the feature-flag contract: account-level catalog and
  permission settings management on or off, `enabled = false`, and monitoring and alarms on or off.
- [ ] Tests run by `make test` confirm the compliance designation contract. With an empty
  designation, a Production plan contains no compliance-only resources. With each of SOC 2,
  HIPAA, and PCI DSS designated, the plan contains exactly the resources FR-022 lists for that
  framework.
- [ ] Tests run by `make test` confirm that invalid inputs fail at plan time with clear messages.
  Examples: an unknown zone name, an empty principal ARN, a scan limit below the minimum, and an
  Enhanced environment type missing a required tag.
- [ ] `make plan-examples` shows common tags on all resources and data tags on data storage
  resources.
- [ ] `make validate` passes as the full gate before pushing.

### Security Validation

- [ ] Static security scan reports no HIGH or CRITICAL findings without a documented suppression.
- [ ] Encryption with a customer-managed key is configured for every data store.
- [ ] No storage is publicly accessible, and all storage denies non-TLS access.
- [ ] Table-data permissions exist only for principals supplied as inputs. No wildcard principals
  appear in any policy.
- [ ] Deployer policy contains only the actions needed to create and destroy module resources.
- [ ] Each persona's policy grants query-results access only to the locations of its bound
  workgroups.
- [ ] In Enhanced environment types, no writer grant includes the right to drop tables.

### Performance Validation

- [ ] A table created by a writer principal is queryable by a reader principal within 5 minutes.
- [ ] A query that exceeds the workgroup scan limit is cancelled by the platform.
- [ ] Unreferenced-file removal is reported enabled on every zone, and a writer-created table
  reports compaction and snapshot management enabled.

### Operational Validation

- [ ] Alarms on query failures and scan volume deliver to the notification topic when enabled.
- [ ] Table restore to a snapshot within the RPO window succeeds.
- [ ] Cost estimate output includes every new resource type.
- [ ] Data-access events for lakehouse tables appear in the audit trail.

## Assumptions *(include if making assumptions)*

- Each deployment targets a single cloud region in a single account.
- Consumers supply the names of existing principals (roles) for administrators, writers, and
  readers. The module does not create human or pipeline identities.
- Tables are created by data pipelines or administrators using the writer persona. The module
  creates zones and namespaces, not tables or schemas.
- Default zones are `raw` and `curated`. A single default query workgroup is
  created, and consumers may declare more.
- Table format is Apache Iceberg. This is the only format the managed table storage supports.
- Where several module instances share an account and region (for example, separate domains), only
  one enables account-level catalog and permission settings management (FR-005, FR-009). That
  deployment receives the complete data-lake administrator list.
- An account-wide or organization-wide audit trail already captures management and data-access
  events for the deployment region (FR-014).
- Name prefix and tags come from the shared context module, as in the existing template.
- The compliance designation applies to the whole deployment. Data under different designations
  that must be isolated from each other goes in separate deployments.

## Out of Scope *(include if explicitly excluding items)*

- Data ingestion, transformation jobs, and streaming pipelines (separate modules).
- Table definitions, schemas, and table-level data loading.
- Identity creation (roles, users, federation) for personas.
- Consumption zone and semantic layer: business-facing data products, metrics, and models are
  provided by an external semantic layer that reads curated data through reader grants.
- Business intelligence and visualization tooling.
- Network foundations and private endpoints.
- Cross-account and cross-region data sharing and replication (candidate for a later spec;
  MissionCritical replication deferred).
- Sensitive-data discovery and automated classification scanning.

## Dependencies *(include if external dependencies exist)*

- Shared context module for `name_prefix` and tags.
- Existing principals for each persona, including the external semantic layer's reader principal.
- Cloud provider support in the infrastructure tool for managed table storage, federated catalog
  integration, and central permission resources.
- Existing account-wide or organization-wide audit trail with retention that meets the compliance
  requirements (FR-014).

## Notes

- Technology choices named in the Input line are deliberately excluded from the body of this spec
  and are resolved in `/iac.plan`.
- The federated catalog and account-level permission settings are shared per account and region.
  One feature flag governs both (FR-005, FR-009) so that deployments do not overwrite each other.
- Clarifications resolved 2026-10-13:
  - Account-level permission settings are managed behind the catalog-management flag.
  - Audit logging relies on an existing trail.
  - Cost-incurring SOC 2, HIPAA, and PCI DSS infrastructure is gated by a compliance designation,
    not by environment type.
- Amended 2026-10-13 after `/iac.analyze`:
  - FR-003 matches the per-table maintenance model.
  - FR-012a adds query-results isolation.
  - FR-007 removes table drops from writers in Enhanced.
  - FR-016 requires and validates cost-center, owner, and classification tags in Enhanced.
- Amended 2026-10-13 after the second `/iac.analyze`:
  - FR-013 requires the key policy statements to be output for consumer-supplied keys.
  - FR-016 defines the data classification tag as the deployment's most sensitive classification.
