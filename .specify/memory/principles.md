<!--
Sync Impact Report
==================
Version change: (unversioned template) → 1.0.0
Bump rationale: Initial ratification. All placeholders replaced with project-specific content.

Modified principles (placeholder → title):
- ARCHITECTURE_PRINCIPLE_1_NAME → Govern Data Access by Default
- ARCHITECTURE_PRINCIPLE_2_NAME → Embed Compliance into the Architecture
- ARCHITECTURE_PRINCIPLE_3_NAME → Optimize for Cost Efficiency
- CODE_PRINCIPLE_1_NAME → Define Small, Validated Module Contracts
- CODE_PRINCIPLE_2_NAME → Secure by Construction
- CODE_PRINCIPLE_3_NAME → Validate Before Merge
- APPROACH_1_NAME → Environment-Driven Defaults
- APPROACH_2_NAME → Compliance Floor, Cost Ceiling

Added sections (beyond the three template slots):
- Architecture: Protect Data Durability and Recoverability
- Architecture: Establish Resource Identity and Accountability
- Code: Make Dependencies Explicit
- Code: Lock Versions and Release Semantically
- Approach: Compose, Don't Accumulate

Removed sections: none

Templates and commands reviewed:
- ✅ .specify/templates/plan-template.md — "Principles Check" reads principles dynamically; no change needed
- ✅ .specify/templates/spec-template.md — already has Cost Constraints (mandatory) and Compliance
  Requirements sections; no change needed
- ✅ .specify/templates/tasks-template.md — Polish phase already includes security scanning, cost
  estimation, and tagging tasks; no change needed
- ✅ .claude/commands/iac.analyze.md, iac.plan.md, iac.enrichplan.md, iac.implement.md,
  iac.checklist.md, iac.clarify.md, iac.specify.md, iac.tasks.md, iac.principles.md — generic,
  reference `.specify/memory/principles.md`; no change needed
- ⚠ .claude/commands/iac.converge.md — uses the term "constitution" for this file; consistent in
  intent, wording alignment optional
- ✅ AGENTS.md — module goal and components filled in; references these principles
- ✅ README.md — rewritten for the lakehouse module; links to these principles

Deferred TODOs: none
-->

# AWS Lakehouse Terraform Module Principles

## Cloud Architecture Principles

### Govern Data Access by Default

A lakehouse concentrates an organization's most valuable and most sensitive data in one place, so
a single permissive grant can expose everything. Access MUST be denied unless explicitly granted
through one central, auditable permission model that covers storage, catalog, and query layers
together. No data layer is ever publicly reachable, all data is encrypted at rest with
customer-controlled keys and in transit, and human and workload identities receive only the
data-zone, table, or column access their purpose requires.

- **Baseline** (Ephemeral, Development, Testing): Public access blocked, encryption with a
  customer-managed key, transport encryption enforced, and access granted per data zone
  (raw/curated/consumption) to named roles only.
- **Enhanced** (UAT, Production, MissionCritical): Adds fine-grained (table, column, and row) access
  controls, separate keys per data zone or sensitivity class, and query isolation so workloads cannot
  read each other's results.

### Embed Compliance into the Architecture

Consumers of this module operate under SOC 2, HIPAA, PCI DSS, and GDPR. Retrofitting audit trails,
residency guarantees, or erasure capability after data lands is expensive and often impossible, so
the architecture MUST make compliant operation the default path. Every data access and permission
change is recorded in a tamper-resistant audit trail; data stays in the region where it is
deployed; sensitive data (PHI, cardholder data, personal data) is identifiable by classification
metadata and isolated in its own access boundary; and stored data supports verifiable deletion so
erasure requests remove records and their historical versions.

- **Baseline**: Controls are present and enabled with shorter retention. Regulated data MUST NOT be
  loaded into Baseline environments.
- **Enhanced**: Full audit retention, immutable audit storage, classification-driven access
  policies, and retention/expiration rules that satisfy the strictest applicable regime.

### Optimize for Cost Efficiency

Lakehouse costs grow with data volume and query activity rather than with fixed capacity, and
uncontrolled scans or forgotten data can dominate spend. When choices are otherwise equal, the
module MUST favor pay-per-use, serverless capabilities with no always-on compute, move aging data to
cheaper storage tiers automatically, cap per-query scan volume, and expire transient artifacts such
as query results. Every deployment reports an estimated monthly cost so consumers see the impact of
configuration before applying it.

- **Baseline**: Aggressive expiration of transient data, tight query scan limits, minimal retained
  versions.
- **Enhanced**: Tiering tuned to access patterns, usage attribution per workload, and limits sized
  for production query volume—still with no idle fixed-capacity resources.

### Protect Data Durability and Recoverability

Data in a lakehouse is frequently the only copy of curated business history; losing or corrupting
it can be unrecoverable. Recovery capability MUST match the RPO/RTO of the selected environment
type, and destructive changes MUST be reversible within the recovery window.

- **Baseline**: Single-region storage with object versioning where RPO is defined; Ephemeral
  environments may skip versioning and permit forced teardown.
- **Enhanced**: Versioning, deletion protection, retained table snapshots sized to the RPO, and
  optional cross-region replication for MissionCritical where residency rules allow it.

### Establish Resource Identity and Accountability

Compliance audits and cost attribution both depend on knowing what every resource is, who owns it,
and what data it holds. Every resource MUST carry predictable names and common tags, and data
storage resources MUST carry additional data-classification tags.

- **Baseline**: Names and tags flow from the shared context module; module name, version, and
  environment type are tagged automatically.
- **Enhanced**: Classification, data owner, and cost-center tags are required and validated.

## IaC Code Principles

### Define Small, Validated Module Contracts

The module provisions the lakehouse foundation—storage layers, open table catalog, governed access,
and query engine setup—and nothing else. Inputs MUST be minimal, typed, grouped into `*_config`
objects with sensible defaults, and validated with `validation` blocks so misconfiguration fails at
`plan` rather than at `apply`. Optional capabilities are toggled with boolean feature flags, the
top-level `enabled` flag disables all resources, and outputs expose only identifiers needed for
composition (ARNs, names, IDs)—never internal resource objects. Distinct responsibilities live in
focused submodules under `modules/`.

### Secure by Construction

Insecure configuration MUST be impossible or require explicit override, not merely discouraged.
IAM and resource policies are written with `aws_iam_policy_document`, never inline JSON; policies
use no wildcard principals and include confused-deputy conditions on service trust; storage policies
deny non-TLS access; KMS encryption is created by default with an option to supply an existing key;
log groups are created explicitly with retention and encryption; and no secrets are accepted as
plain variables or emitted as outputs. Static analysis (Checkov or Trivy) runs on every change and
findings are fixed or explicitly suppressed with justification.

### Validate Before Merge

Every change MUST pass `terraform fmt -check`, `terraform validate`, TFLint, and `terraform-docs`
regeneration, plus Terratest plan-only tests against `examples/defaults` and `examples/complete`.
Tests verify the module contract—expected resources, feature-flag presence/absence, and input
validation failures—with unique names per run and no shared state. These checks run via
`make test` locally and in GitHub Actions on every pull request.

### Make Dependencies Explicit

Hidden coupling makes changes unpredictable and audits hard to reason about. External lookups
MUST be centralized in `dependencies.tf`; identifiers of existing resources (KMS keys, SNS topics,
VPC endpoints, principals) are accepted as inputs rather than discovered by naming convention; and
values flow one way from variables through `locals.tf` into resources and submodules.

### Lock Versions and Release Semantically

Consumers in regulated environments need reproducible plans and auditable change history. Terraform
and provider constraints MUST be declared in `versions.tf`, nested module sources pinned to exact
versions, and releases tagged on `main` using semantic versioning via release-please, with
`module_version` and the CHANGELOG kept in sync.

## Implementation Approaches

### Environment-Driven Defaults

Consumers select one `environment_type`; the module derives security, durability, monitoring, and
cost settings from it so most callers set few or no individual options. Ephemeral, Development, and
Testing map to Baseline; UAT, Production, and MissionCritical map to Enhanced, with RPO/RTO targets
as documented in the README. `None` disables derived defaults and uses explicit `*_config` values
for consumers with requirements outside these profiles. New capabilities MUST define their
per-environment defaults in the `environment_defaults` map in `locals.tf` before they are exposed
as inputs.

### Compliance Floor, Cost Ceiling

When trade-offs arise, security and compliance controls form a non-negotiable floor: they are never
disabled to save money. Above that floor, the lowest-cost option that meets the environment's
RPO/RTO wins. Added complexity—a new resource, input, or always-on component—requires a documented
justification tied to a compliance requirement, a recovery objective, or a measured cost saving.

### Compose, Don't Accumulate

The MVP covers storage layers, catalog, governed access, and query setup. Ingestion pipelines,
transformation jobs, streaming, BI tooling, and network foundations belong in separate modules that
consume this module's outputs. Requests to add them here are resolved by exposing the outputs they
need, not by adding feature flags.

## Governance

**Authority and Precedence**: These principles govern all specifications, plans, tasks, and code
for this module. They supersede individual preference and conflicting tactical guidance; when in
doubt, they decide.

**Compliance and Review**: `/iac.plan` MUST complete a Principles Check against every principle,
and `/iac.analyze` treats any conflict with a MUST statement as CRITICAL. Pull request review
confirms alignment, and automated CI checks enforce the Validate Before Merge and Secure by
Construction principles.

**Justification for Complexity**: Anything beyond Baseline patterns, and every new input or
resource, is recorded with rationale in the plan's Complexity Tracking section.

**Deviation and Exceptions**: Deviations require explicit acknowledgment, documented rationale, and
maintainer approval in the pull request. Static-analysis suppressions carry an inline justification.
Exceptions are reviewed at each minor release for patterns that warrant amending these principles.

**Amendment and Versioning**: Amendments are made through `/iac.principles` in a dedicated pull
request and follow semantic versioning: MAJOR for removing or redefining a principle, MINOR for
adding a principle or materially expanding guidance, PATCH for clarifications. Amendments that
affect existing consumers include migration guidance in the CHANGELOG.

**Relationship to Operational Guidance**: These principles establish WHAT and WHY. Day-to-day HOW
(file layout, style, and test patterns) lives in `AGENTS.md`, which adapts more frequently and MUST
NOT contradict these principles.

**Version**: 1.0.0 | **Ratified**: 2026-10-13 | **Last Amended**: 2026-10-13
