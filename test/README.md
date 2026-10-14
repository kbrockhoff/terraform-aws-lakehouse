# Tests

Only runs `terraform plan` to keep tests fast. Recommend running `terraform apply` after to ensure resources are created.

Every test is plan-only: it copies the repository to a temp folder, runs `terraform init` and
`terraform plan` against an example, and inspects the JSON plan. Nothing is created, but the
plans read live account data (caller identity, Price List, Lake Formation settings), so AWS
credentials are required.

## Lake Formation admin prerequisite

`make test-secondary` and the admin check in `make test-standalone` need the role running the
tests to **already be a Lake Formation data-lake admin** in the sandbox account. Set
`LAKEHOUSE_TEST_LF_ADMIN=true` in such an account:

- without it, `TestSecondaryExampleSkipsAccountIntegration` is skipped;
- without it, the standalone tests plan with `verify_deployer_admin = false`.

The other tests pass the caller as the example's admin list, so they need no prior setup.

## Not covered at plan time

The persona IAM policy documents reference the workgroup and results bucket ARNs, which are
unknown until apply, so their scoping is verified in the sandbox end-to-end run (T076), not here.
