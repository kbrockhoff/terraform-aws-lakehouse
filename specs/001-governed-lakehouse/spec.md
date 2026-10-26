# Infrastructure Specification: Governed Multi-Tenant Lakehouse Platform

**Spec ID**: `001-governed-lakehouse`
**Created**: 2026-10-13
**Status**: Draft
**Input**: User description: "lakehouse which uses S3 tables for storage, Lake Formation for fine-grained access, Athena for querying. For the s3tablesdatacatalog, manage settings from this module"
**Amended input (2026-10-14)**: "treat this infrastructure as a platform that can host multiple tenants with a tenant per s3 tables namespace / glue catalog database. fine-grained access should be managed by lake formation using LF-tags"

## Executive Summary *(mandatory)*

This specification defines a reusable infrastructure module that provisions a governed, multi-tenant
lakehouse platform in a single cloud region. The platform hosts many tenants. Each tenant owns one
namespace (catalog database) in each data zone it uses. Tenant provisioning is a self-contained
unit with two entry points. The platform can apply it for tenants listed in its own input. A tenant
provisioning pipeline can also call it standalone against an existing platform, with its own
state. Analytical data is stored as open table format tables in a managed
table storage service and discovered through the region's federated table catalog, whose settings
this module owns. All module-managed access is expressed as label-based permissions in a
centralized data permission service. A tenant label unique to each tenant isolates tenants from
each other. Within a tenant, sensitivity, compliance-regulation, and data-domain labels on
databases, tables, and columns drive table- and column-level access. A column's most restrictive
labels roll up to its table. Each tenant's delegated stewards manage fine-grained access inside
their own tenant. Each tenant queries through its own serverless,
pay-per-query workgroup. A single `environment_type` selection scales security, durability,
monitoring, and cost controls from low-cost development sandboxes to compliance-ready production
platforms. A per-tenant compliance designation supports SOC 2, HIPAA, PCI DSS, and GDPR workloads
without making non-regulated tenants pay for regulated-data infrastructure.

## Problem Statement *(mandatory)*

### Current State

The repository contains a module template that provisions only an encryption key, an optional alarm
notification topic, and a cost estimate. There is no storage for analytical tables, no catalog,
no data-access governance, and no query capability. Teams wanting a lakehouse must assemble
storage, catalog integration, permissions, and query configuration by hand for each account and
environment. The result is inconsistent security postures and unpredictable costs. Teams that share
an account also have no consistent way to keep each other's data, query results, and costs apart.

### Desired State

One module call produces a working, governed lakehouse platform. The platform provides raw,
curated, and consumption data zones backed by managed open-format table storage with automatic
table maintenance. The regional federated catalog is configured to expose those tables to analytics
services. Every data access is decided by one central permission model based on tags, and each
tenant gets query workgroups with enforced encryption, result expiration, and scan limits.

Onboarding a tenant takes a single tenant definition naming its owner, sensitivity ceiling,
regulations, data domain, stewards, writers, and readers. The definition is applied through the
platform's tenant map or by a platform-operated provisioning pipeline. One `terraform apply` then
gives the tenant isolated namespaces, label-based grants, a workgroup, and, when its data is
designated under a compliance framework, dedicated encryption.
Platform defaults derive from `environment_type`.

### Business Impact

- **Benefits**:
  - Days of manual setup reduced to one reviewed deployment, and tenant onboarding reduced to one
    input entry.
  - Consistent, auditable tenant isolation and access control for regulated data.
  - Tenants manage their own fine-grained access without platform-team tickets.
  - Per-tenant query cost attribution and scan limits.
  - No idle compute cost, and table maintenance automated rather than operated by hand.
- **Risks if not implemented**:
  - Ad hoc storage-level permissions that bypass central governance or leak data across tenants.
  - Per-table grant sprawl that cannot be audited.
  - Uncontrolled and unattributed query costs.
  - Unmaintained tables that degrade query performance and inflate storage.
  - Audit findings for missing encryption, access logging, tenant isolation, or erasure capability.

## Infrastructure Requirements *(mandatory)*

### Functional Requirements

**Tenancy**

- **FR-001**: Infrastructure MUST host multiple tenants. A tenant is exactly one namespace
  (catalog database), named after the tenant, in each data zone the tenant uses. Adding, changing,
  or removing a tenant MUST change only that tenant's resources, and MUST NOT change any resource
  owned by the platform. Tenant names MUST be validated at plan time so that they are valid as
  namespace names and as parts of storage container names.
- **FR-001a**: Tenant provisioning MUST be a separately callable unit, with two supported entry
  points:
  - **Platform-applied**: the platform applies the unit once for each tenant listed in a
    data-driven map input.
  - **Standalone**: a tenant provisioning pipeline calls the unit directly, with its own state,
    against an existing platform deployment.

  Both entry points MUST produce the same tenant resources from the same tenant definition. The
  unit MUST depend on the platform only through a single platform contract that the platform
  publishes as an output: identifiers, label vocabulary, effective environment settings, and shared
  locations. It MUST NOT look up platform resources by naming convention. Provisioning a
  standalone tenant MUST leave the platform's plan unchanged. A tenant can move between entry
  points only through a documented state move. The standalone entry point is a
  platform-operated capability. It MUST NOT be exposed to tenant-operated identities.
- **FR-002**: Each tenant MUST declare:
  - an owner;
  - a sensitivity ceiling, which is the most sensitive data the tenant holds;
  - the compliance regulations its data may fall under (default: none). A tenant holding data under
    a regulation whose controls cost money MUST carry the matching designation (FR-027): HIPAA data
    requires a HIPAA designation, and cardholder data requires a PCI DSS designation;
  - its data domain, which MUST be one of the platform's configured domains (default: the
    platform's default domain);
  - the zones it uses (default: all platform zones);
  - its compliance designation (default: empty; see FR-027);
  - whether it uses dedicated storage (default: no; always yes when designated, FR-028).
    Dedicated storage gives the tenant its own capacity and isolation at no idle cost;
  - its stewards, writers, and readers (FR-010);
  - optional query limits that override the platform defaults (FR-017).
- **FR-003**: Tenants MUST be isolated from each other. No principal can read or write a tenant's
  data, query results, or dedicated keys unless it is granted that access through that tenant's
  configuration or by that tenant's stewards. Cross-tenant access, such as an external semantic
  layer reading several tenants, requires the owning tenant to list the principal as a reader.

**Table storage**

- **FR-004**: Infrastructure MUST provide managed table storage for open table format tables, with
  one shared storage container per data zone. Tenants without a compliance designation keep their
  namespaces in these shared containers, unless they request dedicated storage (FR-002).
  Designated tenants always get dedicated containers (FR-028).
  - The default zones are `raw`, `curated`, and `consumption`, in that order. Consumers MAY
    override the zone list and get exactly the zones they list.
  - Raw holds landed source data. Curated holds conformed and cleansed data. Consumption (the
    "gold" layer in medallion terms) holds published, business-ready data products. "Gold" is an
    informal term only; the module does not use it as a zone name or alias.
  - The external semantic layer reads from tenants' consumption namespaces (see Out of Scope).
  - Documentation MUST describe each default zone's purpose and the personas that normally receive
    grants on it.
- **FR-005**: Infrastructure MUST enable automatic unreferenced-file removal for each storage
  container, with retention windows set by environment type.
  - File compaction and snapshot expiration are configured per table by the table storage service
    and are enabled by default on every new table. The module creates no tables, so it MUST
    document that writers keep both enabled.
  - The service's default snapshot retention MUST be at least the RPO of every environment type.
  - Infrastructure MUST set the default storage class for new tables in every storage container
    (shared and dedicated) from environment type. Enhanced environment types use automatic
    access-based tiering. Baseline environment types use standard storage, because their data is
    short-lived. Consumers MAY override the default, and writers MAY set a class per table.
  - Query results are exempt from tiering, because they expire within 30 days (FR-016).

**Catalog integration**

- **FR-006**: Infrastructure MUST integrate the managed table storage with the region's federated
  table catalog. All tenant namespaces and tables in all zones MUST then be discoverable by the
  query engine and the central permission service without manual steps.
- **FR-007**: Infrastructure MUST manage the federated table catalog's settings from this module,
  including enrolling it in central permission governance.
  - The catalog is a single shared resource per account and region, so a feature flag MUST control
    this management.
  - Exactly one deployment per account and region enables the flag. Other deployments in that
    account and region disable it and reference the existing catalog.

**Governed access**

- **FR-008**: Infrastructure MUST register the managed table storage with the central data
  permission service. Table-data access is then authorized there, not by storage-level policies.
- **FR-009**: Every module-managed grant on tenant data MUST be a label-based permission (a label
  expression), except the row-level filters in FR-012. Labels are the permission service's
  metadata tags. They are distinct from the cloud resource tags in FR-022.
  - The permission service uses this label vocabulary:
    - a **tenant** label for each tenant, created and owned by that tenant's provisioning unit and
      used only for that tenant. Tenants therefore never modify a shared label, and a standalone
      tenant needs no platform change;
    - a **sensitivity** label, whose values are the configured sensitivity levels, ordered from
      least to most sensitive (default: public, internal, confidential, restricted);
    - one **regulation** label per configured compliance regulation (default: GDPR, HIPAA, PCI
      DSS), each with the values "yes" and "no". Data can fall under several regulations at once,
      and a label holds one value per resource, so each regulation needs its own label;
    - a **domain** label, whose values are the platform's configured data domains. The first
      configured domain is the platform's default domain, used when a tenant declares none
      (FR-002). The default configuration is a single domain, `general`.

    The platform owns the sensitivity, regulation, and domain labels. Each tenant unit owns only
    its tenant label.
  - The tenant unit labels every tenant namespace (database). Its tenant label is the tenant's own.
    Its sensitivity, regulation, and domain labels are the tenant's most restrictive values: the
    sensitivity ceiling, "yes" for each of the tenant's regulations, and the tenant's domain.
    Tables and columns inherit the database labels until they are labeled explicitly (FR-011a), so
    an unlabeled new table is treated as the tenant's most restrictive data (fail closed).
  - Sensitivity, regulation, and domain labels MAY be applied to tables and columns. Only the tenant
    unit applies the tenant label, and only to databases.
  - Label keys MUST be scoped to the deployment, so several deployments in one account and region do
    not collide.
- **FR-010**: Infrastructure MUST grant data permissions only to principals supplied as inputs,
  grouped by persona.
  - **Platform administrators**: administer the whole deployment, including the label vocabulary and
    permission settings (FR-013). They are the only persona that can change tenant labels.
  - **Tenant stewards**: delegated administrators for one tenant. Within that tenant only, they can:
    - correct the sensitivity, regulation, and domain labels on the tenant's tables and columns,
      within the tenant's limits;
    - grant and revoke the tenant's data permissions to other principals.

    Stewards cannot change tenant labels, label above the tenant's sensitivity ceiling, mark a
    regulation the tenant does not declare, or act on another tenant's resources.
  - **Writers**: data pipelines that create and modify tables in the tenant's namespaces in the
    zones assigned to them. Writers MUST label the tables and columns they create or change
    (FR-011a), within the same limits as stewards. In Enhanced environment types, writers cannot
    drop tables, because a dropped table and its snapshots cannot be recovered. Dropping is
    reserved for platform administrators and stewards.
  - **Readers**: analysts, applications, and the external semantic layer. They get read-only access
    to the tenant's namespaces in assigned zones, limited to:
    - an assigned set of sensitivity levels;
    - an assigned set of regulations (default: none, so regulated data is hidden);
    - optionally, a set of domains. When none is set, the domain does not limit access.

    The semantic layer's reader principal is normally assigned only the consumption zone.
  - A principal MAY hold personas in several tenants. Each assignment is declared in the owning
    tenant's configuration.
- **FR-011**: Infrastructure MUST support column-level and table-level fine-grained access through
  sensitivity, regulation, and domain labels, in every environment type. A reader sees only the
  tables and columns that meet all three conditions:
  - their sensitivity is in the reader's assigned levels;
  - every regulation they are marked "yes" for is in the reader's assigned regulations;
  - their domain is in the reader's assigned domains, when the reader has any.
- **FR-011a**: Labels MUST follow a roll-up rule: a table's labels are at least as restrictive as
  its most restrictive column.
  - A table's sensitivity is the highest sensitivity of any of its columns.
  - A table is marked "yes" for a regulation if any of its columns is.
  - The domain does not roll up, because domains have no restrictiveness order. A table has its
    own domain, and columns MAY differ.
  - Tags inherit downward, so the roll-up is applied by labeling, not by the permission service.
    Writer pipelines MUST apply it when they create a table or change its schema. When they do,
    they label every column explicitly, so that raising the table label does not change the
    columns' own labels. Stewards apply the same rule when they correct labels.
  - The module does not inspect tables and cannot enforce the rule. It MUST document the rule and
    a reference labeling procedure for pipelines, and MUST output the tag keys and allowed values
    that pipelines need.
- **FR-012**: Infrastructure MUST support optional row-level filters on named, existing tables of
  a tenant, granted to that tenant's readers. Row filters cannot be expressed as tag expressions.
  They are the only grants that name specific resources, and the documentation MUST say so.
- **FR-013**: Infrastructure MUST manage the account-level data permission service settings,
  controlled by the same feature flag as federated catalog management (FR-007). These settings are
  the data-lake administrator list and the default permissions for newly created databases and
  tables. When the flag is enabled, the module MUST:
  - set the administrator list to exactly the consumer-supplied platform administrator principals;
  - remove the implicit open default permissions on new databases and tables, so access is granted
    only through explicit grants.

  When the flag is disabled, the module MUST NOT modify these settings.
- **FR-014**: Each zone MUST be a separate access boundary within a tenant. A persona assigned to
  some zones of a tenant MUST NOT receive access to that tenant's other zones. For example, a
  reader granted only the consumption zone cannot read curated or raw data. No principal receives
  access to any zone or tenant by default.

**Query engine**

- **FR-015**: Infrastructure MUST provide one serverless query workgroup per tenant. The workgroup
  configuration MUST be enforced, so clients cannot override the result location, encryption, or
  scan limit.
- **FR-016**: Infrastructure MUST provide a dedicated query-results object storage location. It
  MUST be encrypted with a customer-managed key, block all public access, deny non-TLS requests,
  and expire results automatically after an environment-type-dependent period.
- **FR-017**: Infrastructure MUST enforce a per-query data-scanned limit on every workgroup. Its
  default is determined by environment type, and a tenant MAY override it.
- **FR-018**: Query results MUST be isolated per tenant.
  - Each tenant's workgroup writes to its own results location.
  - Each persona is bound to the workgroups of the tenants that list it, so a principal can read
    results only from those workgroups.
  - Query usage is attributable to the tenant whose workgroup ran the query.

**Cross-cutting**

- **FR-019**: Infrastructure MUST encrypt table storage and query results with a customer-managed
  key. The module creates the platform key by default, or uses a consumer-supplied key. A
  consumer-supplied key's policy cannot be changed by the module, so the module MUST output the key
  policy statements the key needs for table maintenance, catalog data access, and query results.
- **FR-020**: Infrastructure MUST rely on an existing account-wide or organization-wide audit trail
  for data-access and permission-change records, including grants made by stewards. It MUST NOT
  create its own audit trail. The module documents the audit events and retention its consumers'
  trail must capture to meet the compliance requirements below.
- **FR-021**: Infrastructure MUST derive these defaults from `environment_type`: table maintenance
  retention, query-result expiration, scan limits, key deletion window, writer table-drop rights,
  monitoring, and alarms. `None` uses explicit configuration values.
- **FR-022**: Infrastructure MUST apply tags as follows:
  - Common tags go on all resources.
  - Tenant-specific resources also carry a `Tenant` resource tag and the tenant's owner.
  - Data storage resources carry a data classification tag:
    - a dedicated storage container carries its tenant's sensitivity ceiling;
    - shared storage containers and the shared query-results location carry the platform's
      configured shared-storage classification (default: the highest sensitivity level). It does
      not change when tenants are added or removed (FR-001).
  - A tenant without dedicated storage MUST have a sensitivity ceiling at or below the
    shared-storage classification. The tenant unit validates this against the platform contract.

  In Enhanced environment types, the plan MUST fail unless the common tags include a cost center
  and a data owner. Every tenant's sensitivity ceiling, regulations, and domain MUST be among the
  configured values.
- **FR-023**: When monitoring or alarms are enabled, infrastructure MUST raise alarms per tenant
  workgroup on query failures and on scanned-data volume exceeding a configurable threshold. Alarms
  MUST route to the module's alarm notification topic.
- **FR-024**: Infrastructure MUST provide a monthly cost estimate as an optional capability.
  A feature flag controls it, and it is enabled by default. The estimate MUST:
  - cover every cost-incurring resource type that the platform and the tenant unit create:
    encryption keys, table storage and table maintenance, query-results storage, alarms, and the
    notification topic. Resource types with no standing charge (catalog entries, permissions,
    workgroups, namespaces, and tags) are reported at $0 or omitted;
  - report a fixed idle monthly total, a per-service breakdown, and per-unit rates for
    usage-driven costs: stored table data, data scanned per query, maintenance operations, and
    query-results storage;
  - report compliance-only cost (FR-028) as a separate amount;
  - use the cloud provider's published prices for the deployment region, create no resources,
    and incur no cost.

  The tenant unit MUST produce its own estimate in both entry points. The platform's estimate
  covers platform resources plus platform-applied tenants. Standalone tenants report through
  their own unit. Disabling the estimate MUST NOT change any other resource.
- **FR-025**: Infrastructure MUST provide a separately callable deployer policy unit. It grants
  the least-privilege permissions a deploying identity needs to create, update, and destroy the
  module's resources, and it attaches to a consumer-supplied role. It creates no roles. It MUST
  offer two scopes:
  - a **platform scope**, covering every resource type that the platform and platform-applied
    tenants manage;
  - a **tenant scope**, for standalone tenant provisioning pipelines (FR-001a). It covers only the
    tenant unit's resource types and excludes catalog integration, account-level permission
    settings, identity management, and platform-owned storage.

  In both scopes, the policy MUST also include read access to the published price list when the
  cost estimate is enabled.

  It MUST list every action explicitly, with no wildcard actions. It MUST scope resources to the
  deployment's name prefix wherever the service supports resource-level permissions, and
  document each exception. It MUST document that administrator membership in the central
  permission service (FR-013) is a separate prerequisite that an identity policy cannot grant.
- **FR-026**: Infrastructure MUST expose outputs for composition:
  - zone storage identifiers;
  - the platform contract consumed by standalone tenant provisioning (FR-001a);
  - the label vocabulary keys and allowed values;
  - the encryption key identifiers;
  - for platform-applied tenants, the tenant unit's outputs.

  The tenant unit MUST output:
  - its catalog and namespace names by zone;
  - its tenant label key;
  - its workgroup name and query-results location;
  - its key identifier, when designated;
  - its persona policy documents.

  Setting `enabled = false` MUST create no resources.

**Compliance designation**

- **FR-027**: Infrastructure MUST accept a compliance designation per tenant. It lists zero or more
  of SOC 2, HIPAA, and PCI DSS that the tenant's data falls under, and defaults to empty.
- **FR-028**: Infrastructure that incurs additional cost and exists only to satisfy SOC 2, HIPAA,
  or PCI DSS MUST be created only for tenants whose designation includes the corresponding
  framework. Each such resource MUST be traceable to the tenant and framework(s) that require it.
  With no designated tenants, no compliance-only cost-incurring resources are created in any
  environment type. Currently this covers:
  - a dedicated customer-managed encryption key per designated tenant;
  - storage for the designated tenant that is encrypted by default with that key, so its tables
    cannot be stored under another tenant's key;
  - a dedicated query-results location for the designated tenant, encrypted with that key. The
    tenant unit owns it, so a standalone tenant never edits a platform resource;
  - any compliance-only resources introduced by later requirements, which follow the same rule.
- **FR-029**: Controls that add no infrastructure cost MUST apply to every tenant regardless of
  designation:
  - encryption with a customer-managed key, TLS-only access, and public-access blocking;
  - tenant isolation, least-privilege grants, and tag-based fine-grained grants;
  - deletion protection, regional residency, and GDPR erasure support.
- **FR-030**: When any tenant has a non-empty designation and the environment type is Ephemeral,
  Development, or Testing, infrastructure MUST surface a plan-time warning naming those tenants.
  The plan still proceeds, so compliance features can be tested, but regulated data must not be
  loaded there.

### Non-Functional Requirements

#### Performance

- A newly created table in any tenant namespace is queryable by an authorized reader of that tenant
  within 5 minutes of creation. No manual catalog or permission steps are needed beyond the
  module's tag-based grants.
- A newly onboarded tenant has working namespaces, grants, and a workgroup after one
  `terraform apply`.
- Automatic compaction runs on every table (the service default), so query performance does not
  degrade as small files accumulate. The target file size follows the service default unless a
  writer overrides it.
- `terraform plan` for the complete example completes in under 3 minutes. A first full deployment
  completes in under 15 minutes.

#### Availability

- Availability equals the regional managed services' published availability. There are no
  self-managed or single-instance components.
- Recovery objectives follow `environment_type` and are the same for every tenant and zone:
  - Ephemeral: no RPO, 48-hour RTO.
  - Development and Testing: 24-hour RPO, 48-hour RTO.
  - UAT: 12-hour RPO, 24-hour RTO.
  - Production: 1-hour RPO, 4-hour RTO.
  - MissionCritical: 5-minute RPO, 1-hour RTO.
- Table snapshots are retained long enough to restore any table to a point within the RPO window.
  Enhanced environment types protect storage and keys against accidental deletion.

#### Security

- All table data and query results are encrypted at rest with a customer-managed key. Designated
  tenants use their own dedicated key.
- All requests use TLS 1.2 or higher. Non-TLS requests to storage are denied.
- No lakehouse storage is publicly accessible.
- Table-data access is authorized only through the central permission service. No resource policy
  grants a wildcard principal.
- Tenants are isolated (FR-003). Within a tenant, principals receive only the zones, tables, and
  columns their persona's sensitivity, regulation, and domain assignments allow, and only the rows
  any row filter allows.
- Unlabeled tables inherit the tenant's most restrictive labels, so missing labels hide data
  rather than expose it.
- Stewards' and writers' labeling authority is bounded by their tenant label, the sensitivity
  ceiling, and the tenant's declared regulations.
- Every data access and permission change, including steward grants, is recorded in the existing
  tamper-resistant audit trail (FR-020).
- Regulated data (PHI, cardholder data, personal data) is not loaded into Baseline environment
  types: Ephemeral, Development, and Testing (FR-030).
- Cost-incurring SOC 2, HIPAA, and PCI DSS controls are active exactly for tenants whose
  designation includes the framework (FR-027, FR-028).

#### Scalability

- The platform supports at least 100 tenants. Tenants sharing storage share a per-zone table
  capacity equal to the storage service's per-container table quota (10,000 by default,
  adjustable). A tenant needing more capacity or isolation MAY request dedicated storage, which
  gives it its own capacity per zone at no idle cost. Documentation lists the relevant service
  quotas.
- Tenants, zones, personas, and row filters are data-driven inputs. Adding or removing one changes
  only the resources for that item. Other tenants' and zones' resources are not replaced.
- No component uses provisioned capacity. Storage and query capacity scale with usage.

## Service Level Objectives (SLOs) *(mandatory)*

- **Availability**: Lakehouse query and storage availability ≥ 99.9%, measured monthly from the
  provider's service health data for the deployment region.
- **Durability**: Zero loss of committed table data, verified by restoring a table to a snapshot
  within the RPO window during operational validation.
- **Recovery**: Point-in-time table restore achieved within the environment type's RTO, using
  retained snapshots.
- **Tenant onboarding**: A new tenant entry, or a new principal added to a tenant persona, has
  working access after one `terraform apply`, with no manual console steps (100% of grants).
- **Tenant isolation**: 0 successful reads of a tenant's tables or query results by principals not
  granted that tenant, verified by negative access tests during operational validation.
- **Zone isolation**: 0 successful reads of curated or raw tables by principals granted only a
  tenant's consumption zone, verified the same way.
- **Cost control**: 0 queries exceed their workgroup's scan limit, verified from query-engine
  metrics over a 30-day window.

## Cost Constraints *(mandatory)*

### Budget

- **Baseline (Ephemeral, Development, Testing), no designated tenants**: idle monthly cost of at
  most $5, excluding stored data and query scans, regardless of tenant count. This is roughly the
  cost of one platform key. Namespaces, workgroups, and tags carry no standing charge.
- **Enhanced (UAT, Production, MissionCritical), no designated tenants**: idle monthly cost of at
  most $10, plus at most $0.50 per tenant for its workgroup alarms. These amounts exclude stored
  data and query scans.
- **Designated tenants**: each adds at most $2 per month idle on top of the platform budget, for
  its compliance-only resources (FR-028). The cost estimate output reports this amount per tenant.
- Usage-driven costs (stored data, data scanned, maintenance operations) are reported by the cost
  estimate output, not capped by budget. Per-tenant query spend is attributable through each
  tenant's workgroup.

### Cost Optimization

- Pay-per-use services only. No always-on compute or provisioned capacity.
- Per-query scan limits by default: 10 GB for Baseline and 1 TB for Enhanced. Both are configurable
  per platform and per tenant.
- Query results expire after 7 days in Baseline and 30 days in Enhanced.
- Automatic compaction and unreferenced-file removal keep storage and scan volume from growing
  with stale files. Snapshot retention is no longer than the RPO requires plus a safety margin.
- Table data moves automatically to cheaper access tiers in Enhanced environment types (FR-005).
- Ephemeral environments allow forced teardown, so abandoned resources do not keep accruing cost.
- Compliance-only cost-incurring infrastructure is gated by each tenant's designation, not by
  environment type, so non-regulated tenants do not pay for it (FR-028).
- Tenants share zone storage containers unless designated or given dedicated storage, so
  onboarding a tenant adds no idle storage cost.

## Compliance Requirements *(include if applicable)*

### Regulatory Frameworks

SOC 2 Type II, HIPAA, PCI DSS, GDPR.

The module supports all four. Cost-incurring SOC 2, HIPAA, and PCI DSS controls activate only for
tenants whose designation lists the framework (FR-027, FR-028). GDPR controls add no
infrastructure cost and always apply (FR-029).

### Data Requirements

- **Residency**: All lakehouse data, catalog metadata, query results, and audit records stay in the
  deployment region. Cross-region replication is not enabled by this module.
- **Classification**: Every tenant declares a sensitivity ceiling, its regulations, and its
  domain. Every tenant namespace, table, and column carries sensitivity, regulation, and domain
  labels, either inherited or explicit, rolled up per FR-011a. Data storage resources carry
  data-classification resource tags: the platform's shared-storage classification on shared
  storage, and the tenant's ceiling on dedicated storage (FR-022).
- **Isolation**: Each tenant is an isolated access boundary (FR-003). Regulated data is marked with
  regulation labels and hidden from readers not assigned that regulation. PHI and cardholder data
  belong to a designated tenant, which gets a dedicated encryption key and storage encrypted with
  it (FR-002, FR-028).
- **Erasure (GDPR)**: Tables support row-level deletion. Snapshot expiration and unreferenced-file
  removal physically purge deleted records within 30 days of deletion in Enhanced environment
  types. Erasure applies to every zone where a record appears. Documentation MUST remind data
  teams that consumption data products derived from personal data must be rebuilt or purged when
  the source records are erased.
- **Audit retention**: The existing audit trail must keep records for at least 90 days in
  Baseline. When any tenant is designated, it must keep them for at least 1 year, with longer
  retention for HIPAA documentation requirements. This retention is a consumer prerequisite, not
  configured by this module.

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
- [ ] Tests run by `make test` confirm the default zone contract:
  - a default plan contains three shared zone storage containers (`raw`, `curated`,
    `consumption`);
  - an overridden zone list without `consumption` produces no consumption-zone resources.
- [ ] Tests run by `make test` confirm the tenancy contract:
  - each tenant gets one namespace per assigned zone, labeled with its tenant and its most
    restrictive sensitivity, regulation, and domain values, and one workgroup;
  - reader grant expressions include the sensitivity and every regulation label, and include the
    domain label only for readers with assigned domains;
  - every grant on tenant data is tag-based except row filters;
  - adding a tenant to the input adds only that tenant's resources and changes no platform
    resource;
  - the tenant unit planned standalone against the platform contract produces the same resource
    set as the platform-applied path for the same tenant definition, and the platform's plan shows
    no changes afterwards.
- [ ] Tests run by `make test` confirm the feature-flag contract: account-level catalog and
  permission settings management on or off, `enabled = false`, and monitoring and alarms on or off.
- [ ] Tests run by `make test` confirm the compliance designation contract:
  - with no designated tenants, a Production plan contains no compliance-only resources;
  - a tenant designated with SOC 2, HIPAA, or PCI DSS gets exactly the resources FR-028 lists;
  - other tenants are unchanged.
- [ ] Tests run by `make test` confirm that invalid inputs fail at plan time with clear messages.
  Examples:
  - an invalid tenant name;
  - a tenant zone that is not a platform zone;
  - an empty principal ARN;
  - a reader sensitivity level above the tenant's ceiling;
  - a tenant regulation that is not configured, or HIPAA or PCI DSS data without the matching
    designation;
  - a scan limit below the minimum;
  - an Enhanced environment type missing a required tag.
- [ ] Tests run by `make test` confirm the cost estimate contract:
  - the defaults example's idle estimate is within the Baseline budget;
  - the complete example's idle estimate is within the Enhanced budget plus the per-tenant
    allowances;
  - a designated tenant reports a non-zero compliance amount separately, and the amount is zero
    without a designation;
  - the standalone tenant unit reports its own estimate;
  - disabling the estimate yields a zero estimate and no other resource changes.
- [ ] Tests run by `make test` confirm the deployer contract:
  - both scopes render without wildcard actions;
  - the tenant scope contains no catalog-integration, permission-settings, or identity-management
    actions;
  - the platform scope contains an action for every resource type in the complete example's plan.
- [ ] Tests run by `make test` confirm common resource tags on all resources, `Tenant` and owner
  resource tags on tenant resources, and data-classification resource tags on data storage
  resources (FR-022).
- [ ] `make validate` passes as the full gate before pushing.

### Security Validation

- [ ] Static security scan reports no HIGH or CRITICAL findings without a documented suppression.
- [ ] Encryption with a customer-managed key is configured for every data store, and every
  designated tenant's storage and results use that tenant's key.
- [ ] No storage is publicly accessible, and all storage denies non-TLS access.
- [ ] Module-managed table-data permissions exist only for principals supplied as inputs. No
  wildcard principals appear in any policy.
- [ ] Deployer policy contains only the actions needed to create and destroy module resources.
  This is verified by applying and destroying the complete example with a role that holds only
  the platform-scope policy, and the standalone tenant with a role that holds only the
  tenant-scope policy. Each role also has administrator membership in the permission service.
- [ ] Each persona's policy grants query-results access only to the workgroups of the tenants that
  list it.
- [ ] In Enhanced environment types, no writer grant includes the right to drop tables.
- [ ] A principal granted only tenant A cannot read tenant B's tables or query results.
- [ ] A principal granted only a tenant's consumption zone cannot read its curated or raw tables.
- [ ] A reader cannot see columns labeled above its sensitivity levels, or marked "yes" for a
  regulation it is not assigned.
- [ ] A new, unlabeled table is invisible to readers below the tenant's ceiling until a pipeline
  labels it.
- [ ] A pipeline that labels a table per FR-011a (one restricted column) leaves its other columns
  visible to a reader limited to lower sensitivity levels.
- [ ] A steward cannot change a tenant label, label above the tenant's ceiling, or grant access to
  another tenant's data.

### Performance Validation

- [ ] A table created by a writer principal is queryable by a reader principal of the same tenant
  within 5 minutes.
- [ ] A query that exceeds the workgroup scan limit is cancelled by the platform.
- [ ] Unreferenced-file removal is reported enabled on every storage container, and a
  writer-created table reports compaction and snapshot management enabled.

### Operational Validation

- [ ] Alarms on query failures and scan volume deliver to the notification topic when enabled.
- [ ] Table restore to a snapshot within the RPO window succeeds.
- [ ] A manual review of the `make plan-examples` cost outputs confirms the estimates are
  plausible against the published prices.
- [ ] Data-access events and steward grant events appear in the audit trail.

## Assumptions *(include if making assumptions)*

- Each deployment targets a single cloud region in a single account. One platform deployment per
  account and region is the expected pattern, with tenants as its unit of separation.
- Consumers supply the names of existing principals (roles) for platform administrators and for
  each tenant's stewards, writers, and readers. The module does not create human or pipeline
  identities.
- Each tenant has exactly one namespace per zone it uses. Tenants that need more structure use
  table naming within their namespace, or are split into several tenants.
- Tables are created by data pipelines or administrators using the writer persona. The module
  creates zones, tenant namespaces, and tag labels on namespaces. It does not create tables or
  schemas, and it does not label tables or columns; stewards do that.
- Default zones are `raw`, `curated`, and `consumption`. The consumption zone is a default rather
  than a feature flag: an empty zone has no idle cost, and the zone-list override already allows
  opting out.
- All tenants and zones share the same environment-driven retention and recovery defaults.
  Consumption data products can usually be rebuilt from curated data, but the module does not lower
  consumption-zone recovery targets on that basis.
- Grants that stewards make are managed outside the module's state. The module's inputs remain the
  source of truth for module-managed grants, and the audit trail records steward grants.
- Table format is Apache Iceberg. This is the only format the managed table storage supports.
- Where several module instances share an account and region, only one enables account-level
  catalog and permission settings management (FR-007, FR-013). That deployment receives the
  complete data-lake administrator list.
- An account-wide or organization-wide audit trail already captures management and data-access
  events for the deployment region (FR-020).
- Name prefix and tags come from the shared context module, as in the existing template.
- Tenant provisioning pipelines are operated by the platform team, not by tenants, even when a
  tenant requests the provisioning. Their roles are platform administrators (FR-013) and have
  read access to the platform contract (for example, the platform's remote state outputs or a
  parameter the platform publishes). Tenants exercise delegated authority only through stewards
  (FR-010), which stays bounded to their own tenant. A tenant-operated, least-privilege
  provisioning path is deferred to a later specification.
- Tenant names must be unique across both entry points. No central registry prevents a standalone
  tenant from reusing a platform-applied tenant's name. Such a collision fails at apply, because
  the namespace and tenant label already exist.
- Service quotas on storage containers, tag keys, or workgroups may need increases for large
  tenant counts or many designated tenants. Documentation lists the relevant quotas.

## Out of Scope *(include if explicitly excluding items)*

- Data ingestion, transformation jobs, and streaming pipelines (separate modules). This includes
  jobs that promote data from raw to curated and from curated to consumption.
- Table definitions, schemas, table-level data loading, and table- or column-level labels, which
  writer pipelines and stewards assign (FR-011a).
- Identity creation (roles, users, federation) for platform administrators and tenant personas.
- Tenant self-service onboarding workflows, portals, and the tenant provisioning pipelines
  themselves. Tenants are onboarded through the platform's tenant map or by calling the tenant
  unit.
- A central tenant registry or name reservation across entry points.
- The semantic layer itself: business-facing metrics, models, and data-product APIs are provided
  by an external semantic layer that reads tenants' consumption namespaces through reader grants.
- Zone- or tenant-specific retention, tiering, or recovery profiles.
- Business intelligence and visualization tooling.
- Network foundations and private endpoints.
- Cross-account and cross-region data sharing and replication (candidate for a later spec;
  MissionCritical replication deferred).
- Sensitive-data discovery, automated classification scanning, and automated roll-up enforcement
  or drift detection for table labels.

## Dependencies *(include if external dependencies exist)*

- Shared context module for `name_prefix` and tags.
- Existing principals for platform administrators and for each tenant's personas, including the
  external semantic layer's reader principal.
- Cloud provider support in the infrastructure tool for managed table storage, federated catalog
  integration, central permission resources, and tag-based permissions on federated catalogs.
- Read access to the cloud provider's published price list from the deploying identity, when the
  cost estimate is enabled (FR-024).
- Existing account-wide or organization-wide audit trail with retention that meets the compliance
  requirements (FR-020).

## Notes

- Technology choices named in the Input lines are deliberately excluded from the body of this spec
  and are resolved in `/iac.plan`.
- The federated catalog and account-level permission settings are shared per account and region.
  One feature flag governs both (FR-007, FR-013), so that deployments do not overwrite each other.
- History before 2026-10-14 used the earlier requirement numbering:
  - Clarifications resolved 2026-10-13: account-level permission settings sit behind the
    catalog-management flag; audit logging relies on an existing trail; and cost-incurring
    compliance infrastructure is gated by designation, not environment type.
  - The 2026-10-13 `/iac.analyze` amendments added these requirements:
    - the per-table maintenance model (now FR-005);
    - query-results isolation (now FR-018);
    - no writer table drops in Enhanced (now FR-010);
    - required tags (now FR-022);
    - key policy outputs for consumer-supplied keys (now FR-019).
- Amended 2026-10-14 (consumption zone): restored `consumption` as a third default zone (FR-004)
  with zone access isolation (FR-014). This supersedes the draft `002-gold-zone` spec.
- Amended 2026-10-14 (multi-tenant platform): the module is now a platform hosting many tenants,
  with one tenant per namespace in each zone (FR-001 to FR-003). Requirements were renumbered.
  Clarifications resolved:
  - **Fine-grained access**: all module-managed grants are tag expressions over a tenant tag and a
    classification tag (FR-009, FR-011), in every environment type. Row-level filters remain
    named-resource grants, as the documented exception (FR-012).
  - **Delegation**: each tenant has delegated stewards who correct labels and grant within their
    tenant (FR-010).
  - **Query isolation**: one workgroup and results location per tenant, with per-tenant scan
    limits and cost attribution (FR-015 to FR-018).
  - **Compliance designation**: the designation moved from the deployment to the tenant.
    Designated tenants get a dedicated key, storage encrypted with it, and results encrypted with
    it (FR-027, FR-028).
- Amended 2026-10-14 (tag vocabulary): the classification tag was renamed to sensitivity. Added
  per-regulation tags and a domain tag, applied optionally to databases, tables, and columns
  (FR-009, FR-011). Added the column-to-table roll-up rule (FR-011a). Clarifications resolved:
  - **Roll-up**: writer pipelines apply it at table creation and schema change; stewards correct
    afterwards. The module documents it but does not automate it.
  - **Domain**: an optional reader filter. Tenant isolation is unchanged.
  - **Regulations**: one yes/no tag per regulation, because a tag holds one value per resource.
  - **Defaults**: database labels are the tenant's most restrictive values, so unlabeled tables
    fail closed.
- Amended 2026-10-14 (tenant unit): tenant provisioning is a separately callable unit, applied by
  the platform from its tenant map or called standalone by a tenant provisioning pipeline, through
  a published platform contract (FR-001a). To keep standalone tenants from writing to shared
  platform state, three changes follow:
  - each tenant owns its own tenant label instead of a value in a shared tenant tag;
  - domains come only from the platform's configured list;
  - designated tenants own a dedicated query-results location (FR-028).
- Amended 2026-10-14 (submodule coverage review): FR-024 now defines the cost estimate's scope,
  output shape, opt-out flag, and per-unit behavior. FR-025 now defines the deployer unit's two
  scopes, its no-wildcard and prefix-scoping rules, its price-list access, and the separate
  administrator prerequisite. Matching success criteria and a price-list dependency were added.
- Amended 2026-10-14 after `/iac.analyze` (multi-tenant):
  - **P1**: FR-005 adds environment-driven default storage classes, with automatic tiering in
    Enhanced.
  - **I1**: FR-022 makes shared-storage classification a platform setting that tenants must fit
    under.
  - **A1**: tenant provisioning pipelines are platform-operated (FR-001a, Assumptions).
  - **S1**: Scalability uses the per-container table quota, and FR-002 adds optional dedicated
    storage.
- Amended 2026-10-14 after the second multi-tenant `/iac.analyze`:
  - **F3**: "label" now means permission-service tags and "tag" means cloud resource tags.
  - **U1**: FR-009 defines the default domain.
  - **G2**: the resource-tag criterion is a `make test` check.
- Amended 2026-10-14 after the third multi-tenant `/iac.analyze`: the Desired State mentions both
  entry points (F4). The operational cost criterion became a manual plausibility review,
  because the cost contract is tested under Code Validation (D1). Long lines were rewrapped (L1).
