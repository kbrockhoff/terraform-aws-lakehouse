<!--
Sync Impact Report
==================
Version change: 1.2.2 → 1.3.0
Bump rationale: MINOR. Materially expands guidance: the module is a multi-tenant platform, tenant
isolation is part of the access model, and fine-grained access is tag-based in every environment
type. The tags cover sensitivity, regulations, and domain, and the most restrictive column labels
roll up to the table. The compliance designation applies per tenant. No principle removed or redefined.

Modified principles:
- Govern Data Access by Default — adds tenant isolation; tag-based (sensitivity, regulation,
  domain) fine-grained access moves from Enhanced to every tier; adds fail-closed defaults and
  column-to-table roll-up; designated keys are per tenant
- Embed Compliance into the Architecture — designation is made per tenant
- Optimize for Cost Efficiency — Enhanced usage attribution is per tenant
- Define Small, Validated Module Contracts — tenants are data-driven inputs
- Compose, Don't Accumulate — tenant onboarding portals are out of scope

Added sections: none
Removed sections: none

Templates and commands reviewed:
- ✅ .specify/templates/plan-template.md, spec-template.md, tasks-template.md — no change needed
- ✅ .claude/commands/iac.*.md — no change needed
- ✅ specs/001-governed-lakehouse/spec.md, plan.md, tasks.md — redesigned 2026-10-14 as a
  multi-tenant platform with LF-tag access control
- ✅ AGENTS.md, README.md — components and features updated
- ⚠ .github/workflows/terraform-ci.yml — still invokes terraform and go test directly; task T073
  switches it to make targets during implementation

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
tenant, data-zone, table, or column access their purpose requires.

The module is a platform shared by many tenants, so tenant isolation is part of this floor: no
principal reaches a tenant's data, query results, or keys unless that tenant grants it. Access is
expressed as tag-based policies over tenant, sensitivity, regulation, and domain metadata on
databases, tables, and columns, not as per-resource grants. Tag-based policies scale with tenant
count and stay auditable. A table's labels are at least as restrictive as its most restrictive
column, and anything unlabeled inherits its tenant's most restrictive labels, so missing metadata
hides data rather than exposing it. Any named-resource grant
(such as a row filter) is a documented exception. Authority delegated to tenants is always bounded
by their own tenant scope.

- **Baseline** (Ephemeral, Development, Testing): Public access blocked, encryption with a
  customer-managed key, transport encryption enforced, tenant isolation, and tag-based access per
  tenant, data zone (raw/curated/consumption), sensitivity, and regulation, granted to named roles
  only.
- **Enhanced** (UAT, Production, MissionCritical): Adds stricter destructive-action limits and
  per-tenant monitoring. Query isolation applies in every tier, so tenants cannot read each other's
  results.
- **Designated data** (any environment type): A designated tenant gets its own key and storage
  encrypted with it, consistent with Embed Compliance into the Architecture.

### Embed Compliance into the Architecture

Consumers of this module operate under SOC 2, HIPAA, PCI DSS, and GDPR. Retrofitting audit trails,
residency guarantees, or erasure capability after data lands is expensive and often impossible, so
the architecture MUST make compliant operation the default path. Every data access and permission
change is recorded in a tamper-resistant audit trail; data stays in the region where it is
deployed; sensitive data (PHI, cardholder data, personal data) is identifiable by classification
metadata and isolated in its own access boundary; and stored data supports verifiable deletion so
erasure requests remove records and their historical versions.

Controls that add no infrastructure cost apply in every deployment. Controls that add cost and
exist only for SOC 2, HIPAA, or PCI DSS MUST be activated by an explicit designation that a
tenant's data falls under that framework, never by environment type alone. This way
non-regulated tenants, including production ones, do not pay for regulated-data infrastructure.

- **Baseline**: No-cost controls are enabled with shorter retention. Regulated data MUST NOT be
  loaded into Baseline environments; designating a framework there is allowed only to test
  compliance features.
- **Enhanced**: Full audit retention, sensitivity- and regulation-driven access policies, and
  retention/expiration rules that satisfy the strictest applicable regime. Cost-incurring
  framework controls are added when the data is designated.

### Optimize for Cost Efficiency

Lakehouse costs grow with data volume and query activity rather than with fixed capacity, and
uncontrolled scans or forgotten data can dominate spend. When choices are otherwise equal, the
module MUST favor pay-per-use, serverless capabilities with no always-on compute, move aging data to
cheaper storage tiers automatically, cap per-query scan volume, and expire transient artifacts such
as query results. Every deployment reports an estimated monthly cost so consumers see the impact of
configuration before applying it.

- **Baseline**: Aggressive expiration of transient data, tight query scan limits, minimal retained
  versions.
- **Enhanced**: Tiering tuned to access patterns, usage attribution per tenant, and limits sized
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

The module provisions the lakehouse platform—storage layers, open table catalog, governed access,
and query engine setup for its tenants—and nothing else. Tenants are data-driven input entries, so
onboarding a tenant changes configuration, not code. Inputs MUST be minimal, typed, grouped into `*_config`
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

Every change MUST pass the validation gates defined in the `Makefile`, and local and agent-driven
validation MUST run through those `make` targets rather than invoking `terraform`, `tflint`,
`terraform-docs`, or `go test` directly, so every contributor runs identical checks. Use
`make pre-commit` before each commit (format, check, lint, docs) and `make validate` before each
push (adds `make test`); run single targets such as `make check`, `make lint`, or
`make test-complete` while iterating. A target that skips because its tool is not installed does
not count as passing. Tests run by `make test` are Terratest plan-only tests against the examples
that verify the module contract—expected resources, feature-flag presence/absence, and input
validation failures—with unique names per run and no shared state. GitHub Actions runs the same
gates on every pull request.

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
disabled to save money. The floor is every no-cost control plus the cost-incurring controls of
each framework the data is designated under. Cost-incurring controls for frameworks the data is
not designated under are not part of the floor. Above the floor, the lowest-cost option that
meets the environment's RPO/RTO wins. Added complexity—a new resource, input, or always-on component—requires a documented
justification tied to a compliance requirement, a recovery objective, or a measured cost saving.

### Compose, Don't Accumulate

The MVP covers storage layers, catalog, governed access, and query setup for tenants. Ingestion
pipelines, transformation jobs, streaming, semantic layers, BI tooling, tenant onboarding portals,
and network foundations belong in
separate modules that consume this module's outputs. Requests to add them here are resolved by
exposing the outputs they need, not by adding feature flags.

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

**Version**: 1.3.0 | **Ratified**: 2026-10-13 | **Last Amended**: 2026-10-14
