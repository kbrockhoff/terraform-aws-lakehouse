# Infrastructure Specification: Hardened Catalog and Data Permission Account Settings

**Spec ID**: `003-harden-catalog-settings`
**Created**: 2026-11-18
**Status**: Draft (clarified 2026-11-18: Q1 protocol version 4 with same-region cross-account
sharing in scope; Q2 studio integration limited to account settings; Q3 key-access mode is
selectable, service-role mode by default; amended 2026-11-18: account-settings owner flag in FR-001
and studio support flag in FR-007; amended 2026-11-18 from plan deviations D1–D4 in FR-003, FR-006,
FR-018, FR-026; amended 2026-11-18 from analysis: FR-001 default, FR-002 grouping, FR-013 key
protection, wildcard-principal scope, migration limited to ephemeral test deployments; amended
2026-11-18: dedicated catalog-metadata key in FR-009, FR-012, FR-013, FR-026; amended 2026-11-18
from sandbox results T004 and T007: FR-009 scope, FR-011 condition, FR-024 label visibility,
FR-026 no consumer key grant; amended 2026-11-19: federated table storage catalog ownership in
FR-029 to FR-031, created only when missing and never deleted; amended 2026-11-21: optional
Identity Center integration of the permission service in FR-032)
**Input**: User description: "Upgrade and improve security on the metadata catalog and central data
permission service account settings. The input is a set of design considerations for running managed
table storage protected by tag-based permissions alongside a unified analytics studio in the same
account. The considerations cover:

- how the studio's governance layer integrates with the permission service;
- the storage-registration role: a service-linked role or a customer-managed role;
- the label taxonomy;
- direct storage access that bypasses the permission service;
- when a catalog resource policy is needed;
- cross-account sharing protocol versions 4 and 5;
- encrypting catalog metadata with a customer-managed key, with the key used by each caller or by a
  dedicated service role."

## Executive Summary *(mandatory)*

This specification hardens the account- and region-wide settings that the `001-governed-lakehouse`
platform depends on: the central data permission service's settings and the metadata catalog's
settings. The goal is safe operation of the platform in an account that also hosts a unified
analytics studio. The studio creates its own project roles, compute environments, and governance
grants.

Seven changes:

- **Encrypted metadata.** Catalog metadata and stored connection credentials are encrypted with a
  dedicated customer-managed key. It is separate from the keys that encrypt table storage and query
  results.
- **Explicit settings.** Every permission-service setting is declared explicitly with a secure
  default. Today some settings are reset implicitly on each apply.
- **Sharing protocol version.** The cross-account sharing protocol version is pinned at 4, the
  minimum for federated catalogs and hybrid access. Later upgrades are a deliberate, irreversible
  choice.
- **Same-region cross-account sharing.** Tenants can share read-only, label-scoped slices of their
  data with approved consumer accounts in the same region. Sharing goes through the permission
  service and the resource-sharing service, and never weakens tenant isolation.
- **No direct storage bypass.** Managed table storage refuses direct data reads and writes that do
  not go through the permission service. This closes the bypass path that studio notebooks and
  custom compute would otherwise have.
- **Studio coexistence.** The studio's governance roles can operate alongside the platform without
  being silently removed and without being able to weaken the platform's settings.
- **Verification.** The access-verification harness from `002-lf-access-verification` gains cases
  that prove these controls hold.

## Problem Statement *(mandatory)*

### Current State

The platform's catalog-integration unit (owner deployment only) already provides:

- registration of all managed table storage with the permission service, using a customer-managed
  data-access role. The role's trust policy is restricted to the account (confused-deputy
  protection), and hybrid access is off;
- a federated catalog for managed table storage, with full-table access by external engines
  disabled;
- authoritative lists of administrators and read-only administrators;
- removal of the implicit "identity-policy-only" default access for new databases and tables.

Gaps:

- **Catalog metadata encryption is not managed.** Schemas, column names, partition values, and
  stored connection credentials use whatever the account default is. Usually that is no encryption
  or the provider-managed key, which the platform cannot revoke, audit separately, or restrict.
- **Several permission-service settings are implicit.** The settings update replaces the whole
  settings object. Settings the module does not declare are reset to provider defaults on every
  apply:
  - external data filtering and its allow list;
  - authorized session tag values;
  - trusted resource owners;
  - full-table access for external engines at the account level;
  - the cross-account sharing protocol version.

  Out-of-band changes, for example a studio console setup step that enables external data
  filtering, are silently reverted on the next apply. Nobody reviews whether they should exist.
- **The cross-account sharing protocol version is unmanaged.** Federated catalogs and hybrid access
  require version 4 or later. Upgrades are irreversible, so an unplanned upgrade under time pressure
  is risky.
- **Cross-account sharing is not supported.** `001` left it out of scope. Consumers in other
  accounts can only get data through copies or out-of-band grants, and neither is governed by
  tenant labels or bounded by tenant authority.
- **Direct storage access can bypass the permission service.** Table storage policies enforce
  transport and key requirements but grant or deny nothing by identity. A studio project role or
  notebook with broad storage permissions in its identity policy can call the table storage data
  APIs directly. That reads or writes table data with no tag-based check, and no column or row
  restrictions apply.
- **The studio's governance roles have no first-class place.** The studio's governance layer needs
  its access-management role to be a permission-service administrator so it can fulfill
  subscriptions. The administrator list is authoritative, so adding that role outside the module
  gets it removed. Putting it in the general administrator list gives it the same rights as
  platform deployers, without saying why it is there.

### Desired State

All account-wide catalog and permission-service settings are declared in one place, with secure,
explicit, reviewable values. Then:

- catalog metadata is encrypted with a key the platform controls;
- every direct path to table data is closed except the permission service's own credential vending
  and explicitly listed exemptions;
- the studio's integration roles are named inputs with a documented purpose;
- the sharing protocol version is a deliberate, recorded choice;
- approved consumer accounts in the same region read tenant-approved, label-scoped data in place,
  and every share can be revoked by removing it from configuration;
- the verification harness proves each control on every sandbox run.

### Business Impact

- **Benefits:**
  - Schema metadata for regulated tenants becomes covered by the same customer-managed,
    auditable, revocable encryption as the data. This is an explicit expectation of SOC 2,
    HIPAA, and PCI DSS assessors.
  - The highest-risk bypass in a shared studio account is closed: direct table storage reads that
    ignore column and row restrictions.
  - Silent setting drift is removed.
- **Risks if not implemented:**
  - A studio user with a broad storage policy can read restricted columns and other tenants' rows
    without any permission-service check. This defeats tenant isolation (principle: Govern Data
    Access by Default).
  - Auditors find unencrypted or provider-key metadata.
  - A later need for cross-account sharing forces an untested, irreversible protocol upgrade in
    production.
- **Change risk:**
  - Encrypting catalog metadata affects every principal that reads the catalog.
  - Denying direct storage access breaks any existing workload that relies on it.

  Both changes need a migration path and sandbox verification before they reach Enhanced
  environments.

## Infrastructure Requirements *(mandatory)*

### Functional Requirements

#### Ownership and Scope

- **FR-001**: The account settings in this specification apply to the whole account and region.
  A dedicated boolean **account-settings owner** flag MUST say whether this deployment manages
  them. Exactly one deployment per account and region sets the flag to true. By default the flag takes
  the value of the existing catalog-integration flag, so a deployment that owns the catalog
  integration also owns the settings unless configured otherwise.
  - **Settings covered by the flag:**
    - the permission-service settings (FR-003, FR-005), including the authoritative administrator
      lists and the studio governance integration principals (FR-007);
    - the catalog resource policy (FR-006);
    - catalog metadata encryption and its key-access mode, role, and key protections (FR-009 to
      FR-014);
    - the federated table storage catalog, created only when missing and never deleted (FR-029
      to FR-031);
    - the optional Identity Center integration of the permission service (FR-032).
  - **The flag is independent of the catalog-integration flag.**
    - A deployment that owns the catalog integration but not the account settings MUST run the
      same plan-time administrator guard that secondary deployments run today.
    - A deployment that owns the account settings but not the catalog integration MUST still
      manage the settings listed above.
  - **When the flag is false**, the deployment MUST NOT create or change any covered setting.
    Instead it MUST:
    - check at plan time that the live values meet the minimums it depends on: metadata encryption
      is on, and the sharing protocol version is at least the required minimum;
    - fail with a message naming the owner deployment when a minimum is not met.
  - **Turning the flag off** on an existing owner MUST NOT reset the live settings or disable
    metadata encryption. The settings stay in place for the new owner to take over, and the
    documentation MUST describe the hand-over steps.
  - **Validation**: inputs that only apply to the owner (for example the sharing protocol version,
    the key-access mode, and the resource-policy deny statements) MUST be rejected when they are
    set to non-default values on a non-owner deployment.
- **FR-002**: Every new setting MUST be grouped into the existing configuration objects or into new
  `*_config` objects, one per concern (account settings, studio support, sharing), with validated,
  typed fields and secure defaults. The top-level `enabled` flag
  MUST continue to disable everything.

#### Permission-Service Settings

- **FR-003**: Infrastructure MUST declare every field of the permission service's account settings
  explicitly, so no apply implicitly resets a value. Defaults:
  - **External data filtering**: off. The allow list is empty.
  - **Authorized session tag values**: empty.
  - **Trusted resource owners**: empty.
  - **Full-table access by external engines** (account level): off. This matches the existing
    catalog-level setting.
  - **Default permissions for new databases and tables**: none. This is unchanged: no implicit
    identity-policy-only access.
  - **Service parameters**: the settings update replaces the parameter set as a whole, so every
    parameter MUST be declared. That includes the sharing protocol version (FR-005) and the
    session-context propagation parameter, which stays enabled as it is by default so that audit
    records keep the calling identity. (Amended 2026-11-18, plan D4.)
- **FR-004**: Each non-default value in FR-003 MUST be an explicit input with a validation rule and a
  documented security consequence. For example, enabling external data filtering MUST require a
  non-empty allow list of accounts. It MUST NOT allow the current account to be trusted implicitly.
- **FR-005**: Infrastructure MUST set the cross-account sharing protocol version, with a default
  of **4**. Version 4 is the minimum for federated catalogs and hybrid access.
  - The version MUST be an input.
  - Validation MUST reject versions below 4.
  - Lowering the version MUST fail at plan time, because upgrades are irreversible.
  - Choosing a version above 4 MUST be documented as irreversible. The documentation MUST list the
    extra resource-sharing permissions that a custom cross-account grantor policy then needs.
- **FR-006**: Infrastructure MUST manage the metadata catalog resource policy authoritatively in the
  owner deployment. By default it MUST create no policy.
  - Same-account access is governed by identity policies and the permission service.
    Label-based sharing through the resource-sharing service needs no catalog resource policy.
  - If a consumer enables the policy, it MUST contain only:
    - a statement letting the resource-sharing service share this account's catalog, databases,
      and tables;
    - optional deny statements supplied as validated inputs.
  - It MUST NOT allow any external principal directly. Cross-account access is granted only
    through the permission service (FR-021).
  - An unmanaged catalog resource policy that lacks the resource-sharing statement breaks every
    share. The infrastructure tooling can't read the catalog resource policy at plan time, so:
    - the access-verification harness MUST detect such a policy (FR-020 case 4);
    - the documentation MUST tell owners to check for an existing policy before enabling sharing,
      and to adopt it under management or remove it.

    (Amended 2026-11-18, plan D1: the check was moved from plan time to the verification harness.)

#### Studio Coexistence

- **FR-007**: A dedicated boolean **studio support** flag MUST say whether the deployment supports
  the unified analytics studio in this account. The default is false.
  - **When false**: the studio governance integration principal input MUST be empty (validation
    rejects any entries), and no studio-specific settings are applied.
  - **When true**: Infrastructure MUST accept a separate, named, non-empty input listing the studio
    governance integration principals. These are the roles the studio's governance layer uses to
    fulfill subscriptions and manage grants.
  - **On the account-settings owner**, these principals MUST be added to the permission-service
    administrators together with the platform administrators. The administrator list stays
    authoritative.
  - **On a non-owner deployment** (FR-001), the flag MUST NOT change any account setting. Instead
    the deployment MUST check at plan time that every listed principal is in the live administrator
    list. It MUST fail with a message naming the owner deployment when one is missing.
  - **Turning the flag off** on the owner MUST remove the studio principals from the administrator
    list on the next apply. The documentation MUST warn that this stops the studio from fulfilling
    subscriptions.
  - They MUST appear in outputs and documentation as integration principals, not as platform
    deployers.
  - Validation MUST reject wildcard or cross-account principals.
  - The module integrates with the studio only through account settings. It MUST NOT create the
    studio domain, projects, or project-role grants. Studio project roles get data access the
    same way as any other persona: through the tenant reader and writer inputs or through the
    studio's own subscription grants.
- **FR-008**: When studio support is enabled, the studio governance integration principals MUST
  NOT be able to change the settings
  in FR-003 or FR-005 without a module change. This follows from FR-003: each apply restores the
  declared values. A changed value MUST appear as plan drift so it gets reviewed.

#### Catalog Metadata Encryption

- **FR-009**: Infrastructure MUST encrypt catalog metadata at rest with a **dedicated catalog-metadata
  key**, in every environment type.
  - The owner deployment creates this customer-managed key unless the consumer supplies one.
  - It MUST be distinct from the platform key and from every tenant key that encrypts table
    storage or query results. A supplied key that equals one of them MUST be rejected at plan time.
  - The key MUST live with the other account-wide settings in the account-settings unit (FR-001),
    with its own policy, alias, and rotation enabled.
  - Stored connection credentials MUST also be encrypted with this key and returned to callers only
    in encrypted form.
  - The platform key's policy MUST NOT gain any catalog-related statement.
  - **Scope** (sandbox result 5): the catalog-encryption setting covers objects stored in the
    metadata catalog itself: default-catalog databases and tables, resource links, and stored
    connection credentials. It does **not** cover the metadata of managed table storage exposed
    through the federated table-storage catalog. Table storage keeps that metadata itself,
    encrypted with the table bucket's key (the platform key or a tenant key). The documentation
    MUST say so.
- **FR-010**: The key-access mode for metadata encryption MUST be a validated input with two
  values. The default is service-role mode.
  - **Service-role mode** (default): a dedicated catalog-encryption role holds the key
    permissions, so callers need only catalog and permission-service rights. This keeps the key
    policy small and works with studio project roles that are created dynamically.
  - **Per-caller mode**: every principal that reads or writes the catalog needs its own key
    permissions, and each caller appears in the key-use audit records. In this mode the module
    MUST grant key use to the principals it knows about: platform administrators, read-only
    administrators, studio integration principals (when studio support is on), tenant personas, and
    consumer accounts per FR-026. It MUST also accept an additional, validated list of key
    users. The documentation MUST state that callers outside these lists, such as dynamically
    created studio project roles, lose catalog access.
  - Switching modes MUST take one apply, with no loss of catalog access for the principals listed
    above.
- **FR-011**: In service-role mode the catalog-encryption role MUST:
  - be trusted only by the metadata catalog service, with a confused-deputy condition limited to this
    account. The catalog service sends no source resource matching the catalog when it assumes the
    role, and a source-resource condition breaks catalog access, so no region condition is added
    (sandbox result 3; same situation as D2);
  - hold key permissions only on the configured key;
  - receive common tags only. It is not a data-at-rest resource, so it MUST NOT receive data tags.
- **FR-012**: The key policy MUST allow the metadata catalog service to use the key only in this
  account's catalog encryption context. It MUST NOT grant broader key use to the catalog service.
  The dedicated key is used for nothing but catalog metadata and connection credentials.
- **FR-013**: Infrastructure MUST prevent the metadata key from becoming unusable while the catalog
  depends on it:
  - Destroying or disabling the owner deployment MUST NOT schedule deletion of, or disable, the
    catalog-metadata key. The key policy MUST deny both actions to every principal except
    explicitly listed break-glass principals. This holds in **every** environment type, Ephemeral
    included, because catalog metadata is account-wide and outlives any single deployment.
  - Ephemeral and test deployments that own the account settings SHOULD use a long-lived catalog
    key supplied from outside the deployment, so they can be torn down freely. The module MUST warn
    when an Ephemeral owner encrypts the catalog with a key it created itself.
  - The catalog key is chosen at installation. Changing it on a deployment whose catalog is
    already encrypted MUST fail at plan time, except in Ephemeral environments. No prior
    installations exist, so key migration is not supported.
- **FR-014**: The module MUST document the migration effect on existing catalog objects. Only
  objects written or updated after encryption is enabled are encrypted. The documentation MUST
  include a procedure to re-encrypt existing platform databases and tables.

#### Federated Table Storage Catalog

- **FR-029**: The account-settings owner MUST ensure that the account's federated table storage
  catalog (`s3tablescatalog`) exists in its Region. At apply time it MUST check whether the catalog
  exists and create it only when it does not.
  - **Created with**: a federated catalog over every table storage bucket in the account and
    Region (`arn:<partition>:s3tables:<region>:<account>:bucket/*`, connection `aws:s3tables`),
    empty default permissions for new databases and tables (no identity-policy-only access), the
    common tags, and full-table access by external engines set from a new input that defaults to
    **true**. The documentation MUST state the consequence: third-party engines get credentials
    without session tags when the caller has full table access.
  - **An existing catalog** MUST be left exactly as it is: no update, no overwrite, and no plan
    failure. Its attributes are reported by the verification harness instead (FR-020).
  - **Safety**: the check-and-create step MUST confirm that the credentials it runs with belong to
    the deployment's account and Region, and fail otherwise.
  - **Ordering**: the check and creation MUST complete before the deployment creates or changes
    any other resource (amended 2026-11-19). Neither administrator rights nor the registration of
    the table storage with the permission service is needed first (sandbox result 8). Only
    read-only lookups and plan-time checks may run before it.
- **FR-030**: The federated table storage catalog MUST never be deleted by this module: not on
  destroy, not when the owner flag or the catalog-integration flag is turned off, not on teardown
  of an Ephemeral environment, and not during an upgrade from an earlier version. Deleting it would
  drop every catalog grant and every bucket catalog under it for all deployments in the account.
- **FR-031**: The catalog-integration unit MUST no longer create or delete the catalog. An existing
  deployment whose state holds the catalog MUST stop managing it without deleting it, in the same
  apply that upgrades it. The catalog-integration unit keeps the storage registration and the
  data-access role.

#### Identity Center Integration

- **FR-032**: The account-settings owner MUST support an optional integration of the permission
  service with an existing IAM Identity Center instance (added 2026-11-21). The integration lets
  administrators grant data permissions to Identity Center users and groups, and lets query
  engines that use trusted identity propagation act as those users. The default is off.
  - **Inputs**: an enabled flag (default false) and the Identity Center instance ARN, grouped
    under the account-settings configuration (FR-002).
    - When enabled, the instance ARN MUST be a valid Identity Center instance ARN without
      wildcards.
    - When disabled, the instance ARN MUST be empty. Validation rejects any value, the same way
      the studio flag does (FR-007).
  - **When enabled**, the owner MUST create the integration for this account's default catalog.
    It MUST be created after the permission-service settings (FR-003), because the deploying role
    must already be an administrator.
  - **Owner only** (FR-001): a non-owner deployment MUST reject a non-default value. Nothing else
    in the platform depends on the integration, so non-owners run no live check for it.
  - **Turning it off** on the owner MUST delete the integration on the next apply. The
    documentation MUST warn that permissions granted to Identity Center users and groups then stop
    working, and that engines using trusted identity propagation lose access through it.
  - **An integration created outside the module**: the service allows one per catalog, so the
    create fails while another exists. The documentation MUST describe adopting it with an import
    instead.
  - **Outputs**: the integration's application ARN, empty when the integration is off or the
    deployment is not the owner.
  - **Deployer permissions**: the deployer policy MUST grant the integration's permission-service
    and Identity Center application actions only when the integration is requested, scoped to the
    configured instance and its applications. The applications belong to the instance's account,
    which is usually not this one (amended 2026-11-21 from plan D5, sandbox result 9).
  - The integration MUST NOT configure external filtering targets or cross-account share
    recipients. The infrastructure provider cannot set them yet (see Out of Scope).

#### Direct Storage Bypass Prevention

- **FR-015**: Every table storage bucket the platform creates (shared zone buckets and designated
  tenants' dedicated buckets) MUST deny direct table data reads and writes, and table metadata
  changes, to every principal except:
  - the permission service's data-access role, whose vended credentials serve every governed query
    engine;
  - the table storage service's own maintenance;
  - principals listed in an explicit, validated exemption input. The default is empty.

  Cross-account consumers MUST NOT be exempted. Their engines read through credentials vended by
  this account's permission service.
- **FR-016**: Namespace and bucket management needed by the module MUST remain possible for the
  deploying role and the permission-service administrators. Data reads and writes MUST NOT be
  exempted for them unless they are listed in the exemption input.
- **FR-017**: The deny statements MUST be added alongside the existing transport and encryption
  denies, written with structured policy documents, and MUST NOT use wildcard allows.

#### Registration Role

- **FR-018**: The storage registration MUST keep using the customer-managed data-access role, not the
  service-linked role. The role's trust MUST keep the account restriction. The permission service
  documents no region condition for registration roles, and an undocumented condition could break
  credential vending, so the trust MUST NOT add one. If the provider documents a region or
  source-resource condition later, it MUST be added then. (Amended 2026-11-18, plan D2.)
- **FR-019**: Infrastructure MUST support an optional permissions boundary on the data-access role
  and on the catalog-encryption role. The boundary ARN is an input, and the default is none.

#### Cross-Account Sharing (Same Region)

- **FR-021**: Infrastructure MUST support read-only sharing of tenant data with consumer accounts
  in the same region. Sharing MUST use label-based grants from the permission service to the
  consumer account. The permission service creates and reuses the resource-sharing service's
  shares automatically. The module MUST NOT create or modify those shares directly.
- **FR-022**: The platform MUST hold an approved list of consumer accounts, as a validated input.
  The default is empty, so no sharing happens.
  - Each entry is a 12-digit account ID with a documented purpose.
  - The current account and wildcards MUST be rejected.
  - By default, consumers MUST belong to the same organization as the platform account.
    Accounts outside the organization need an explicit per-entry flag, and their shares need
    manual acceptance in the consumer account. The documentation MUST state both consequences.
- **FR-023**: Only a tenant can share its own data. Tenant authority is bounded by tenant scope, so
  each share MUST be declared in the tenant's configuration and MUST name:
  - a consumer account from the approved list;
  - the zones to share. The default is the consumption zone only.
  - the sensitivity levels to share. There is no default, so this must be stated explicitly.
  - the regulations to share. The default is none, so regulated data stays hidden, matching
    `001` readers.

  The grant expression MUST always include the tenant label. A share therefore can never reach
  another tenant's data. Designated tenants MAY share only with an explicit per-share
  acknowledgement that the consumer is in scope for the tenant's compliance frameworks.
- **FR-024**: Shared permissions MUST be read-only: describe the database, and describe and select
  the tables. Insert, delete, alter, drop, and data-location permissions MUST NOT be granted
  cross-account.
  - The grant option MUST be limited to the same read permissions, so the consumer account's
    administrator can delegate within that account but cannot widen access.
  - Column-level restrictions apply in the consumer account through the shared labels.
  - Row-level restrictions are named-resource filters. They MUST NOT be shared cross-account in
    this release.
  - **Label visibility**: so the consumer administrator can delegate by label expression, the
    consumer account MUST also be able to describe (with grant option, and only describe) the label
    keys and values used in each share's expression. Granting the right to use labels in grant
    expressions is not supported across accounts (sandbox result 5).
    - The tenant grants the keys it owns: its tenant key and its row-filter exclusion keys.
    - The platform grants its own keys (zone, sensitivity, regulations, domain), with every value,
      once per approved consumer account. Several tenants sharing with one account would otherwise
      hold the same per-value grants, and removing one share would revoke another's (PR #9
      review). The platform keys' values are vocabulary, not data.
    - A tenant has at most one share per consumer account, so no two shares hold the same grant.
- **FR-025**: Removing a share from configuration, or removing an account from the approved list,
  MUST revoke the corresponding grants on the next apply. Removing an approved account that is
  still referenced by a tenant share MUST fail at plan time with a message naming the share.
- **FR-026**: Encryption MUST keep working for consumers without broadening key access:
  - Table data is read through credentials vended by this account's data-access role, so consumer
    accounts MUST NOT be granted use of the platform key or tenant keys for table data.
  - Shared resources are table-storage objects, whose metadata the catalog-encryption setting
    doesn't cover (FR-009, scope). Consumers therefore need **no** grant on the catalog-metadata
    key, in either key-access mode, and no consumer account MAY appear in the policy of the
    catalog key, the platform key, or any tenant key. Sandbox result 5 verified this: a consumer
    read shared tables created while service-role encryption was on, with no grant on the key.
  - If a later release shares objects stored in the metadata catalog itself, this requirement MUST
    be revisited. (Amended 2026-11-18, plan D3 resolved by T007.)
- **FR-027**: The module MUST output, per consumer account, the shared databases and the label
  expressions it was granted. Consumers use this to create resource links and local grants. The
  module MUST NOT create any resources in consumer accounts.
- **FR-028**: Cross-region sharing and replication remain out of scope. Shares MUST target only the
  deployment region, so data residency is preserved.

#### Verification

- **FR-020**: The access-verification harness (`002-lf-access-verification`) MUST gain cases that
  run in both full mode and probe mode where non-destructive:
  1. a principal with broad storage permissions in its identity policy, but no permission-service
     grant, is denied direct table data reads and writes;
  2. the same principal is denied through the governed query path;
  3. catalog metadata encryption is reported as enabled with the expected key and mode;
  4. the sharing protocol version and every FR-003 setting match the declared values, and any
     catalog resource policy includes the resource-sharing statement (FR-006);
  5. no implicit identity-policy-only grants exist on platform databases or on the catalog.

  Case 11, in both modes: the federated table storage catalog exists with the FR-029 attributes
  (federated over every table storage bucket, connection `aws:s3tables`, empty default
  permissions, and the expected full-table external access). Full mode teardown MUST confirm that
  the catalog still exists after the fixture is destroyed (FR-030).

  Case 12, in both modes: when the Identity Center integration is expected (FR-032), it exists for
  the account's default catalog with the expected instance. When it is not expected, the harness
  reports whether one exists without failing, because the owner may not manage it.

  Full mode MUST also run these cases, in both key-access modes, with a second sandbox account as
  the consumer:

  - Case 6: the consumer reads shared rows and columns within the shared labels;
  - Case 7: the consumer is denied unshared sensitivity levels, regulations, zones, and other
    tenants' data;
  - Case 8: the consumer is denied every write;
  - Case 9: the consumer is denied direct table storage access;
  - Case 10: after a share is removed, the consumer's access is revoked.

### Non-Functional Requirements

#### Performance

- Encrypting metadata MUST NOT add more than 75 ms to the median catalog lookup latency in the
  default service-role key-access mode, compared with an unencrypted baseline in the sandbox. The
  cost is a roughly fixed per-lookup decrypt plus role assumption, so a percentage target would
  depend on network distance rather than on the design. Measured: +52 ms in service-role mode and
  +34 ms in per-caller mode (sandbox-results.md item 6). Service-role mode stays the default
  despite the higher cost, because no caller needs a key grant.
- Bypass-prevention denies MUST NOT change governed query latency measurably. They are evaluated
  only on direct calls.

#### Availability

- No setting change in this specification may make governed queries unavailable for longer than the
  apply that introduces it. No prior production installations exist, so backward compatibility is
  required only for ephemeral test deployments: an existing ephemeral `001` test deployment MUST
  upgrade in one apply, with no data loss and no permission grants recreated.
- Key loss MUST NOT be reachable through normal module operations (FR-013).

#### Security

- Customer-managed key encryption for catalog metadata and stored connection credentials, in all
  environment types.
- No path to table data that skips the permission service, apart from listed exemptions.
- Every account-wide setting is explicit, validated, and visible as drift when changed out of band.
- Confused-deputy conditions on every service trust policy.
- No wildcard principals in Allow statements and no wildcard allows in any new policy. Deny
  statements MAY use a wildcard principal with exception conditions, as the existing storage
  policies do.
- Cross-account access is read-only, always bounded by the tenant label, limited to approved
  same-organization accounts by default, and revocable by configuration.
- Static analysis passes with no new unsuppressed findings.

#### Scalability

- The integration principal list and the exemption list MUST support at least 20 entries each
  without exceeding policy size limits.
- Sharing MUST support at least 20 approved consumer accounts and 100 tenant shares per deployment.
  Label-based grants keep the number of shares independent of the table count, so new tables
  under shared labels become visible to consumers without a new apply.
- No per-tenant resources are added by this specification. Tenant onboarding cost and time are
  unchanged.

## Service Level Objectives (SLOs) *(mandatory)*

- **Control coverage**: 100% of the FR-020 verification cases pass on every sandbox run before a
  release is tagged.
- **Drift**: zero unreviewed differences between declared and live account settings, measured by a
  no-change plan on the owner deployment immediately after apply.
- **Migration**: an existing ephemeral `001` test deployment upgrades with one apply, and 100% of the `002` access
  matrix still passes afterwards.
- **Latency**: median catalog lookup latency increases by at most 75 ms after encryption in the
  default service-role mode (see Performance).
- **Revocation**: a removed share stops granting consumer access within 15 minutes of the apply
  completing, measured by the FR-020 case 10 run.
- **Share isolation**: zero consumer reads outside the shared labels across all FR-020
  cross-account cases.

## Cost Constraints *(mandatory)*

### Budget

- One new fixed-cost resource per owner deployment: the dedicated catalog-metadata key, about USD 1
  per month. None is created when the consumer supplies the key. The optional catalog-encryption
  role, the policy changes, and the shares have no charge.
- Consumers pay for their own queries. Shared data is not copied, so sharing adds no storage cost.
- Incremental key-request charges come from catalog reads and writes. The expected cost is less
  than USD 5 per month for a typical deployment. The module's cost estimate MUST include this line.

### Cost Optimization

- One catalog key per account and region, not per deployment or tenant. Only the owner deployment
  creates it.
- The dedicated key is a security boundary, not a compliance-only control. It keeps consumer
  grants and the absolute deletion protection away from the data keys. So it applies in every
  environment type, like the other controls here, which have no infrastructure cost.

## Compliance Requirements *(include if applicable)*

### Regulatory Frameworks

SOC 2, HIPAA, PCI DSS, and GDPR, as in `001-governed-lakehouse`. Metadata such as column names and
partition values can disclose regulated information, so it is protected with the same key controls
as the data.

### Data Requirements

- Catalog metadata stays in the deployment region and is encrypted with a key whose use is recorded
  in the existing audit trail (001 FR-020).
- Key administration stays separate from data-lake administration. The key policy's administrator
  statements are not granted to studio integration principals.
- Permission-service setting changes are recorded in the audit trail. The module documents which
  events the consumer's trail must capture.
- Cross-account grants, share creation, and consumer data access are recorded in this account's
  audit trail. Shared data never leaves the deployment region.
- Sharing regulated data requires the designated tenant's explicit per-share acknowledgement
  (FR-023). By default, regulated data is excluded from every share.

## Success Criteria *(mandatory)*

### Code Validation

- [x] All new inputs are typed, grouped into `*_config` objects, defaulted securely, and validated.
- [x] Format, validate, lint, docs, and unit/contract tests pass through the `make` targets.
- [x] Non-owner deployments (account-settings owner flag false) plan successfully against a
      compliant owner, fail with a clear message against a non-compliant one, and change no account
      setting (FR-001).
- [x] All four combinations of the account-settings owner flag and the catalog-integration flag
      plan successfully. Owner-only inputs are rejected on non-owner deployments.
- [x] With studio support off, a non-empty studio principal list is rejected. With it on, an empty
      list is rejected, and a non-owner fails when a listed principal is not a live administrator
      (FR-007).
- [x] New resources carry common tags only. No data tags are applied to keys or roles.
- [x] With the Identity Center integration off, a non-empty instance ARN is rejected. With it on, a
      missing or malformed ARN is rejected, and a non-owner rejects any non-default value
      (FR-032).

### Security Validation

- [x] Static analysis reports no new unsuppressed HIGH or CRITICAL findings.
- [x] Catalog metadata and connection credentials are encrypted with the dedicated catalog-metadata
      key in every environment type. That key differs from the platform and tenant keys, and the
      platform key policy contains no catalog or consumer statements.
- [x] A principal with full storage permissions in its identity policy and no permission-service
      grant cannot read or write table data directly (verified in the sandbox).
- [x] Every account-wide permission-service setting matches its declared value, and an out-of-band
      change appears as drift.
- [x] No wildcard principals in Allow statements, no wildcard allows, and confused-deputy
      conditions on every new service trust.
- [x] Plan-time validation rejects shares that target unapproved accounts, the current account,
      write permissions, or another tenant's scope.
- [x] A consumer account can read only the shared labels, can't write, and loses access when the
      share is removed (verified in the sandbox, in both key-access modes).

### Performance Validation

- [x] Median catalog lookup latency increases by at most 75 ms after encryption in service-role
      mode, in the sandbox run.

### Operational Validation

- [x] An existing ephemeral `001` test deployment upgrades with one apply, and the full `002` access
      matrix passes afterwards.
- [x] The re-encryption procedure for existing catalog objects is documented and was exercised once
      in the sandbox.
- [x] Destroying the owner deployment cannot leave catalog metadata encrypted with a key that is
      scheduled for deletion.
- [x] Documentation explains each account-wide setting, its default, the consequence of changing it,
      and how to register studio integration principals.
- [x] Consumer onboarding documentation covers accepting shares from outside the organization,
      creating resource links, and granting access locally. It uses the per-consumer output
      (FR-027).
- [x] Switching between key-access modes takes one apply with no loss of catalog access for listed
      principals.
- [x] In an account without the federated table storage catalog, the owner's first apply creates it
      with the FR-029 attributes. In an account that has it, the apply leaves it unchanged.
- [x] The catalog still exists after the owner deployment is destroyed, after the owner flag is
      turned off, and after an upgrade from a version that managed it in the catalog-integration
      unit (FR-030, FR-031).
- [x] With the Identity Center integration enabled, the owner's apply creates it for the expected
      instance, FR-020 case 12 passes, and turning it off removes it in one apply (FR-032).

## Assumptions *(include if making assumptions)*

- There are no prior installations of this module outside ephemeral test deployments. Backward
  compatibility and migration paths are required only for those test deployments.

- The analytics studio and the platform run in the same account and region. The studio's domain is
  created with its permission-service integration enabled before any platform tables are published
  to it.
- The studio's governance layer fulfills subscriptions through permission-service grants. It
  therefore needs administrator rights, which FR-007 provides. A deployed studio domain makes two
  of its roles administrators: the blueprint provisioning role and the domain's manage-access role
  (sandbox result 5). Both belong in the integration principal list.
- Governed query engines (the interactive query service, the studio's notebook and serverless Spark
  compute, and the warehouse engine) read table data through credentials vended by the
  permission service. Engines that need direct storage access are not governed and must be listed
  as exemptions explicitly.
- The existing label taxonomy (tenant, sensitivity, regulation, domain, zone) from `001` is final
  for this release. Labels live in the account's default catalog and apply to the federated table
  storage catalog. No per-catalog label definitions are needed for same-account use.
- Organization-wide guardrails, such as service control policies that block direct storage access
  at the organization level, are owned by the organization's security team. This module documents
  the recommended guardrail but does not create it.
- Existing catalog objects are few and owned by the platform, so re-encrypting them by rewriting them
  is practical.
- Consumer accounts run their own permission service. Their administrators create resource links
  and local grants, and their engines query through credentials vended by this account.
- The account's resource-sharing service has sharing within the organization enabled, so
  same-organization shares are accepted automatically.

- When the Identity Center integration is enabled (FR-032), an organization or account instance of
  IAM Identity Center already exists and is reachable from this account and Region, and the
  deploying role may create Identity Center applications in it.

- Wherever the account-settings owner is applied, the AWS command-line tool is installed and the
  environment's credentials resolve to the deployment's account and Region (FR-029). Terraform's own
  provider credentials are not visible to it.

## Out of Scope *(include if explicitly excluding items)*

- Creating or configuring the analytics studio domain, projects, blueprints, subscription approval
  workflows, or lineage.
- Organization-level service control policies.
- Separate data-access roles per sensitivity level or per data zone. A single customer-managed role
  plus bucket-level denies is sufficient for same-account isolation.
- Cross-region sharing and replication (unchanged from `001`).
- Cross-account write access, sharing row-level filters, and sharing metadata-only roles.
- Resources in consumer accounts: resource links, local grants, and share acceptance.
- Upgrading the sharing protocol version beyond 4 (an input allows it; the default stays at 4).
- Additional catalogs beyond the federated table storage catalog.
- Automated detection of sensitive columns.
- Identity Center itself: creating the instance, its identity source, users, groups, or the
  trusted token issuers of query engines (FR-032 only connects the permission service to an
  existing instance).
- Granting data permissions to Identity Center users and groups through the tenant persona
  inputs. Persona inputs stay IAM role and user ARNs in this release.
- External filtering targets and cross-account share recipients of the Identity Center
  integration, and Identity Center settings on the interactive query service workgroups.

## Dependencies *(include if external dependencies exist)*

- `001-governed-lakehouse`: owner and secondary deployment model, platform key, table storage
  buckets, data-access role.
- `002-lf-access-verification`: harness extended by FR-020.
- An existing account- or organization-wide audit trail (001 FR-020).
- The deploying role is a permission-service administrator and can administer the platform key.
- A second disposable sandbox account in the same organization and region, to act as consumer in
  full-mode verification. Today `verify/sandbox-accounts.txt` lists only one account.

## Notes

- The source material contained several claims that this spec deliberately does not rely on:
  - that the service-linked role changes audit attribution;
  - that label definitions are scoped per catalog;
  - that "non-integrated" studio domains bypass the permission service.

  The planning phase MUST verify each such behavior against current provider documentation and in
  the sandbox before the design depends on it.
- Pinning the sharing protocol version and enabling metadata encryption change account-wide state
  that outlives this module. Both are covered by FR-013 and the migration success criteria.
- This specification moves same-region cross-account sharing into scope. That supersedes the
  corresponding out-of-scope line in `001-governed-lakehouse`. Cross-region sharing stays out of
  scope.
- The source material said a catalog resource policy is required for cross-account sharing.
  FR-006 instead creates none by default, because label-based shares through the resource-sharing
  service don't need one under protocol version 4. Planning MUST confirm this.
