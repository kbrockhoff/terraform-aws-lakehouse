# AWS Lakehouse Terraform Module Guide for AI Agents

Provisions a multi-tenant, Apache Iceberg-based lakehouse platform on AWS for analytics workloads, with governed access,
compliance-ready defaults (SOC 2, HIPAA, PCI DSS, GDPR), and cost-efficient serverless querying. Configuration
defaults derive from `environment_type`. Design principles are in `.specify/memory/principles.md`; they take
precedence over this guide when the two conflict.

## Components

### Tenancy
- Each tenant is one S3 Tables namespace (Glue database) per zone it uses
- `modules/tenant` provisions everything tenant-scoped; the root applies it per `tenants` map entry, or a tenant pipeline calls it standalone with its own state
- The tenant submodule depends on the platform only through the typed `platform_contract` output and never modifies platform-owned resources
- Tenants are isolated: data, query results, and (for designated tenants) keys, table buckets, and results buckets are tenant-scoped

### Storage Layers
- Separate raw, curated, and consumption (gold) data zones with public access blocked
- Shared table bucket per zone; compliance-designated tenants get dedicated buckets with their own key
- Each zone is an isolated access boundary; the external semantic layer reads tenants' consumption namespaces
- Encryption with customer-managed KMS keys and TLS-only access
- Versioning and lifecycle tiering/expiration driven by environment type

### Open Table Catalog
- Catalog databases (tenant namespaces) for Apache Iceberg tables in each data zone
- Table snapshot retention aligned with the environment's RPO

### Governed Access
- Lake Formation LF-tag grants over a per-tenant `tenant-<name>` key (owned by the tenant submodule) and platform-owned `sensitivity`, `reg-<regulation>` (yes/no per regulation), and `domain` tags, for named principals only
- Module labels tenant databases with the tenant's most restrictive values (fail closed); writer pipelines label tables and columns, rolling the most restrictive column up to the table
- Column-level control through those tags; domain is an optional reader filter; row filters are the only named-resource grants
- Delegated tenant stewards label and grant within their own tenant; audit via the existing CloudTrail

### Query Engine Setup
- One query workgroup per tenant with encrypted, expiring, tenant-prefixed query results
- Per-query scan limits (overridable per tenant) and per-tenant cost attribution

## Terraform Module Guide

### Core Principles

* **One Responsibility:** Each module should have a single, clear purpose.
* **Composition over Configuration:** Build focused modules that work together, not all-in-one solutions.
* **Minimal, Predictable Interfaces:** Expose only essential variables and outputs. Avoid leaking implementation details.
* **Fail Fast:** Validate inputs and stop early on errors.
* **Clear Documentation:** Explain purpose, usage, and design decisions.

### Module Design

**Good Examples:**

```hcl
# Single-purpose modules
module "vpc" { source = "./modules/vpc" name = var.name cidr_block = var.cidr }
module "db" { source = "./modules/rds" name = "${var.name}-db" subnets = module.vpc.db_subnets }
```

**Avoid:**

```hcl
module "monolith" { create_vpc = true; create_db = true; create_lb = true; ... }
```

### Interfaces

* **Inputs:** Only what's needed; group related config in objects; use sensible defaults and validation.
* **Feature Flags:** Use booleans for optional resources.
* **Outputs:** Only what's useful for composition—IDs, endpoints, etc.
  *Don’t expose internal resource details.*

### Patterns

* **Consistent Naming & Tagging:** Use predictable names and common tags.
* **Version Pinning:** Lock Terraform and provider versions in `versions.tf`.
* **Centralize Data Sources:** Place all external data lookups in one file and pass results to submodules.

### File Structure

```
/main.tf           # Core logic
/variables.tf      # Inputs
/outputs.tf        # Outputs
/locals.tf         # Computed values
/versions.tf       # Version constraints
/dependencies.tf   # Data sources
/examples/         # Usage demos
/modules/          # Optional submodules
/test/             # Automated tests
```

### AWS Best Practices

* Use `aws_iam_policy_document` instead of inline JSON for policies.
* Default to encryption with customer-managed KMS.
* Create log groups explicitly.

### Avoid

* **God Modules:** Don’t build modules that do everything.
* **Too Many Inputs:** Limit options; use environment-based defaults.
* **Implicit Dependencies:** Accept IDs as inputs, not naming assumptions.
* **Tight Coupling:** Modules shouldn’t assume other modules’ structure.

### Documentation & Testing

* Provide clear examples and rationale in `README.md`.
* Use `terraform-docs` for reference docs.
* Ensure modules are easily testable (`terraform plan` with example configs).

### Release & CI/CD

* Tag releases on `main` with semantic versioning.
* Use GitHub Actions (or similar) to run format, validate, and test steps automatically.

### AI Agent Checklist

* [ ] Single responsibility & clear purpose
* [ ] Minimal, validated inputs and meaningful outputs
* [ ] Predictable naming/tagging
* [ ] Centralized external dependencies
* [ ] Example configs and docs provided
* [ ] Tested for `plan` success and edge cases

---

**Summary:**
Favor small, focused modules with clear interfaces and strong defaults. Compose modules for complex scenarios. Document intent and usage. Avoid monolithic, over-configured designs. **Simplicity and clarity beat feature-completeness.**

## Terraform Style Guide

*Based on the [Gruntwork](https://docs.gruntwork.io/guides/style/terraform-style-guide/) and [HashiCorp](https://developer.hashicorp.com/terraform/language/style) guides. Focus: simple, reasoning-friendly infrastructure.*

### Core Principles

* **Choose simple over easy:** Each component should have one clear responsibility; prefer clarity over shortcuts.
* **Explicit over implicit:** Make all dependencies and resource relationships obvious.
* **Localize change:** Changes should not require understanding distant code.
* **Composition over configuration:** Build small, independent modules with clear interfaces and minimal assumptions.

### Understanding Terraform Behavior

* Be able to answer:

  * What will be created?
  * In what order?
  * What changes if a variable changes?
  * What external dependencies exist?
* Make dependencies explicit (avoid hidden dependencies through naming/data sources).
* Ensure changes to variables affect only what is expected.

### Design Patterns

* **Module boundaries:** Each module should have a single reason to change (e.g., VPC, compute).
* **Interface design:** Use variables/outputs to define clear, minimal contracts.
* **Data flow:** Ensure values flow in one direction and are derived in locals when needed.

### Syntax and Style

* **Formatting:** Enforced by `terraform fmt` (2-space indent, 120-char lines, blank lines between resources, trailing newline).
* **Naming:** Use `snake_case` and descriptive names with consistent prefixes.
* **Comments:** Use `#` for comments, `----` for section headers, and always explain non-obvious code.
* **Variables:** Always provide descriptions and types; prefer explicit objects over free-form maps; add validation as needed.

### Avoiding Complexity

* **No implicit dependencies:** Reference resources directly, not by naming convention.
* **Avoid nested conditionals:** Use locals to simplify boolean logic.
* **No magic values:** Use variables and data sources, not hardcoded strings or numbers.
* **Single-purpose modules:** Avoid modules overloaded with flags for optional features.

### AI Agent Guidelines

* **Prioritize:** Reasoning clarity, localized change, explicit dependencies, clean interfaces, correct syntax.
* **Generate self-explanatory code:** Comment rationale for settings and explain resource decisions.
* **When refactoring:** Preserve behavior, extract complexity to locals, make dependencies explicit, and clarify logic.
* **Testing:** Verify behavioral correctness, dependency resolution, and predictable change impact.

---

**Remember:** Simple code enables reliable change and long-term maintainability.


## Terraform Test Guide

### Testing Philosophy

* **Test the module contract**: Focus on configuration, planning, outputs, and edge cases—not cloud provider itself.
* **Simple over comprehensive**: One concern per test; avoid brittle “cover everything” tests.
* **Plan-only preferred**: Validate `terraform plan` outputs, not real cloud provider state (avoid unnecessary costs).

### Test Design

* **Single responsibility**: One aspect per test (e.g., defaults, feature flags).
* **Clear failures**: Assertions should explain what’s wrong.
* **Test independence**: Use unique names and don’t share state.
* **Focus on essentials**: Test desired behaviors, not implementation details.

### Repository Organization

* **modules/**: Terraform modules
* **examples/**: Minimal (`defaults/`), full (`complete/`) and other common usage examples
* **test/**: Terratest Go tests for examples

Each example needs: `main.tf`, `variables.tf`, `outputs.tf`, `versions.tf`, `terraform.auto.tfvars`, `README.md`

### Test Implementation Pattern

* Use a unique name per test run
* Use `terraform.Plan` for validation
* Use clear, scenario-specific test names
* Organize with shared utilities in `test/shared_test.go`

**Test template:**

```go
func TestModuleExample(t *testing.T) {
    t.Parallel()
    testID := fmt.Sprintf("test-%d", time.Now().Unix())
    terraformOptions := &terraform.Options{
        TerraformDir: "../examples/defaults",
        Vars: map[string]interface{}{"name": testID},
        RetryableTerraformErrors: getRetryableErrors(),
        MaxRetries: 3,
        TimeBetweenRetries: 5 * time.Second,
    }
    defer terraform.Destroy(t, terraformOptions)
    terraform.Init(t, terraformOptions)
    planOutput := terraform.Plan(t, terraformOptions)
    assert.Contains(t, planOutput, "module.main.aws_kms_key.main[0]")
}
```

**Shared helpers:**

```go
func generateTestName(prefix string) string { /* unique Brockhoff Terraform module names */ }
func getBaseTerraformOptions(dir string) *terraform.Options { /* common options */ }
func getRetryableErrors() map[string]string { /* retry logic for transient errors */ }
```

### Test Types

* **Configuration validation**: Plan succeeds, expected resources created.
* **Feature flag**: Feature is present/absent based on input.
* **Input validation**: Bad inputs fail early.

### Error Handling

* **Retry only transient errors** (timeouts, cloud provider throttling); fail on permanent ones (validation, permissions).

### CI/CD

* Use unique resource names, plan-only tests, and timeouts to control cost and conflicts.
* Run with `make test`.

### AI Agent Guidelines

* **Review**: Is test purpose clear? Will failures be obvious? Are tests isolated and focused on contracts?
* **Generate**: Name tests after scenarios and expected outcomes. Test only what the module promises.
* **Maintain**: Preserve test intent; update assertions as needed; keep tests isolated.
* **Debug**: Check assertion error, inspect plan, validate inputs, run plan manually if needed.

### Anti-Patterns

* **Don’t**: Test all input combinations, implementation details, or write complex, multi-scenario tests in one function.
* **Do**: Test key use cases, contract, and keep logic clear.

### Key Points

1. Test what the module promises, not the cloud provider.
2. One simple, clear purpose per test.
3. Failures must be obvious and actionable.
4. Prefer plan-only tests for speed and cost.
5. All tests must be independent.

---

**Goal:** Simple, essential, and maintainable contract tests for Terraform modules.

<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:46cd31e7 -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` to see full workflow context and commands.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files

**Architecture in one line:** issues live in a local Dolt DB; sync uses `refs/dolt/data` on your git remote; `.beads/issues.jsonl` is a passive export. See https://github.com/gastownhall/beads/blob/main/docs/core-concepts/sync-concepts.md for details and anti-patterns.

## Agent Context Profiles

The managed Beads block is task-tracking guidance, not permission to override repository, user, or orchestrator instructions.

- **Conservative (default)**: Use `bd` for task tracking. Do not run git commits, git pushes, or Dolt remote sync unless explicitly asked. At handoff, report changed files, validation, and suggested next commands.
- **Minimal**: Keep tool instruction files as pointers to `bd prime`; use the same conservative git policy unless active instructions say otherwise.
- **Team-maintainer**: Only when the repository explicitly opts in, agents may close beads, run quality gates, commit, and push as part of session close. A current "do not commit" or "do not push" instruction still wins.

## Session Completion

This protocol applies when ending a Beads implementation workflow. It is subordinate to explicit user, repository, and orchestrator instructions.

1. **File issues for remaining work** - Create beads for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **Handle git/sync by active profile**:
   ```bash
   # Conservative/minimal/default: report status and proposed commands; wait for approval.
   git status

   # Team-maintainer opt-in only, unless current instructions forbid it:
   git pull --rebase
   bd dolt push
   git push
   git status
   ```
5. **Hand off** - Summarize changes, validation, issue status, and any blocked sync/commit/push step

**Critical rules:**
- Explicit user or orchestrator instructions override this Beads block.
- Do not commit or push without clear authority from the active profile or the current user request.
- If a required sync or push is blocked, stop and report the exact command and error.
<!-- END BEADS INTEGRATION -->

<!-- BEGIN BEADS CODEX SETUP: generated by bd setup codex -->
## Beads Issue Tracker

Use Beads (`bd`) for durable task tracking in repositories that include it. Use the `beads` skill at `.agents/skills/beads/SKILL.md` (project install) or `~/.agents/skills/beads/SKILL.md` (global install) for Beads workflow guidance, then use the `bd` CLI for issue operations.

### Quick Reference

```bash
bd ready                # Find available work
bd show <id>            # View issue details
bd update <id> --claim  # Claim work
bd close <id>           # Complete work
bd prime                # Refresh Beads context
```

### Rules

- Use `bd` for all task tracking; do not create markdown TODO lists.
- Run `bd prime` when Beads context is missing or stale. Codex 0.129.0+ can load Beads context automatically through native hooks; use `/hooks` to inspect or toggle them.
- Keep persistent project memory in Beads via `bd remember`; do not create ad hoc memory files.

**Architecture in one line:** issues live in a local Dolt DB; sync uses `refs/dolt/data` on your git remote; `.beads/issues.jsonl` is a passive export. See https://github.com/gastownhall/beads/blob/main/docs/core-concepts/sync-concepts.md for details and anti-patterns.
<!-- END BEADS CODEX SETUP -->
