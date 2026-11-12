# Terraform Brockhoff Module Makefile
# Optimized for AI agents to reduce LLM calls and token usage

.PHONY: help validate test clean format docs all status pre-commit lint check security-scan plan-examples destroy-examples test-defaults test-complete test-enabled-false test-tenancy test-standalone test-deployer test-secondary test-validation test-verify test-verify-fixture verify-build verify-new-run verify-init verify-up verify-seed verify-run verify-apply verify-down verify-full verify-detect verify-cleanup verify-probe verify-probe-audit

# Default target
.DEFAULT_GOAL := help

# Colors for output
CYAN := \033[36m
GREEN := \033[32m
YELLOW := \033[33m
RED := \033[31m
RESET := \033[0m

##@ Help
help: ## Display this help message
	@echo "$(CYAN)Terraform AWS Lakehouse Module$(RESET)"
	@echo "$(CYAN)AI-optimized Makefile for efficient development$(RESET)"
	@echo ""
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make $(CYAN)<target>$(RESET)\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  $(CYAN)%-15s$(RESET) %s\n", $$1, $$2 } /^##@/ { printf "\n$(YELLOW)%s$(RESET)\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Validation
pre-commit: ## Run lightweight validation before commit without push (format, validate, lint, docs)
	@echo "$(CYAN)Running pre-commit checks...$(RESET)"
	@$(MAKE) format
	@$(MAKE) check
	@$(MAKE) lint
	@$(MAKE) docs
	@echo "$(GREEN)✓ Pre-commit checks passed$(RESET)"

validate: ## Run complete validation pipeline before commit with push (format, validate, lint, security scan, test, docs)
	@echo "$(CYAN)Running complete validation pipeline...$(RESET)"
	@$(MAKE) format
	@$(MAKE) check
	@$(MAKE) lint
	@$(MAKE) security-scan
	@$(MAKE) test
	@$(MAKE) test-verify
	@$(MAKE) docs
	@echo "$(GREEN)✓ All validations passed$(RESET)"

check: ## Run Terraform validation on all modules and examples
	@echo "$(CYAN)Validating Terraform configuration...$(RESET)"
	@terraform fmt -check -recursive || (echo "$(RED)✗ Code formatting issues found$(RESET)" && exit 1)
	@echo "$(GREEN)✓ Code formatting OK$(RESET)"
	@for dir in modules/*/; do \
		if grep -qs configuration_aliases "$$dir/versions.tf"; then \
			echo "$(YELLOW)↷ $$dir needs provider aliases; validated through the examples$(RESET)"; \
		elif [ -f "$$dir/main.tf" ] || [ -f "$$dir/variables.tf" ] || [ -f "$$dir/outputs.tf" ]; then \
			echo "$(CYAN)Validating $$dir...$(RESET)"; \
			(cd "$$dir" && terraform init -backend=false && terraform validate) || exit 1; \
			echo "$(GREEN)✓ $$dir validation passed$(RESET)"; \
		fi; \
	done
	@for dir in examples/*/; do \
		echo "$(CYAN)Validating $$dir...$(RESET)"; \
		(cd "$$dir" && terraform init -backend=false && terraform validate) || exit 1; \
		echo "$(GREEN)✓ $$dir validation passed$(RESET)"; \
	done
	@echo "$(CYAN)Validating verify/fixture/...$(RESET)"
	@(cd verify/fixture && terraform init -backend=false && terraform validate) || exit 1
	@echo "$(GREEN)✓ verify/fixture/ validation passed$(RESET)"

format: ## Format all Terraform files
	@echo "$(CYAN)Formatting Terraform files...$(RESET)"
	@terraform fmt -recursive
	@echo "$(GREEN)✓ Formatting complete$(RESET)"

lint: ## Run TFLint on all Terraform files
	@echo "$(CYAN)Running TFLint...$(RESET)"
	@if command -v tflint >/dev/null 2>&1; then \
		tflint --init >/dev/null || exit 1; \
		tflint --recursive --minimum-failure-severity=error || { echo "$(RED)✗ TFLint found errors$(RESET)"; exit 1; }; \
		echo "$(GREEN)✓ TFLint passed$(RESET)"; \
	else \
		echo "$(YELLOW)⚠ TFLint not installed, skipping$(RESET)"; \
	fi

security-scan: ## Run the Checkov security scan with the same flags as CI (extra flags: CHECKOV_ARGS=...)
	@echo "$(CYAN)Running Checkov...$(RESET)"
	@if command -v checkov >/dev/null 2>&1; then \
		checkov --framework terraform --directory . --output cli --skip-check CKV_AWS_144 --quiet --compact $(CHECKOV_ARGS) \
			|| { echo "$(RED)✗ Checkov found failing checks$(RESET)"; exit 1; }; \
		echo "$(GREEN)✓ Checkov passed$(RESET)"; \
	else \
		echo "$(RED)✗ checkov not installed (pip install checkov or uv tool install checkov)$(RESET)"; exit 1; \
	fi

##@ Testing
test: check-aws ## Run all tests
	@echo "$(CYAN)Running tests...$(RESET)"
	@cd test && go mod tidy
	@cd test && go test -v -timeout 30m -parallel 4
	@echo "$(GREEN)✓ All tests passed$(RESET)"

test-defaults: check-aws ## Run only defaults example tests
	@echo "$(CYAN)Running defaults test...$(RESET)"
	@cd test && go test -v -timeout 15m -run 'TestTerraformDefaultsExample|TestCostEstimationDisabled'
	@echo "$(GREEN)✓ Defaults test passed$(RESET)"

test-complete: check-aws ## Run only complete example tests
	@echo "$(CYAN)Running complete test...$(RESET)"
	@cd test && go test -v -timeout 30m -run 'TestTerraformCompleteExample|TestCompleteExampleSecurityControls|TestComplianceDesignationGatesTenantKeys'
	@echo "$(GREEN)✓ Complete test passed$(RESET)"

test-enabled-false: check-aws ## Run only enabled=false test
	@echo "$(CYAN)Running enabled=false test...$(RESET)"
	@cd test && go test -v -timeout 15m -run TestEnabledFalse
	@echo "$(GREEN)✓ Enabled=false test passed$(RESET)"

test-tenancy: check-aws ## Run only tenancy tests (tenant isolation, type sync, zone override)
	@echo "$(CYAN)Running tenancy tests (tenant isolation, type sync, zone override)...$(RESET)"
	@cd test && go test -v -timeout 30m -run 'TestAddingTenantTouchesOnlyThatTenant|TestTenantTypeInSync|TestZoneOverrideWithoutConsumption'
	@echo "$(GREEN)✓ test-tenancy passed$(RESET)"

test-standalone: check-aws ## Run only standalone tenant tests
	@echo "$(CYAN)Running standalone tenant tests...$(RESET)"
	@cd test && go test -v -timeout 30m -run 'TestStandaloneTenant'
	@echo "$(GREEN)✓ test-standalone passed$(RESET)"

test-deployer: check-aws ## Run only deployer policy tests
	@echo "$(CYAN)Running deployer policy tests...$(RESET)"
	@cd test && go test -v -timeout 30m -run 'TestDeployerPolicies'
	@echo "$(GREEN)✓ test-deployer passed$(RESET)"

test-secondary: check-aws ## Run only secondary deployment test (needs LAKEHOUSE_TEST_LF_ADMIN=true)
	@echo "$(CYAN)Running secondary deployment test (needs LAKEHOUSE_TEST_LF_ADMIN=true)...$(RESET)"
	@cd test && go test -v -timeout 30m -run 'TestSecondaryExampleSkipsAccountIntegration'
	@echo "$(GREEN)✓ test-secondary passed$(RESET)"

test-validation: check-aws ## Run only input validation tests
	@echo "$(CYAN)Running input validation tests...$(RESET)"
	@cd test && go test -v -timeout 30m -run 'TestInvalidInputsFailPlan|TestRegulatedDataInBaselineWarns'
	@echo "$(GREEN)✓ test-validation passed$(RESET)"

test-verify: ## Run lfverify vet and unit tests (offline, no AWS credentials)
	@echo "$(CYAN)Running lfverify unit tests...$(RESET)"
	@cd verify && go vet ./... && go test ./...
	@echo "$(GREEN)✓ test-verify passed$(RESET)"

test-verify-fixture: check-aws ## Run only the plan-only access-verification fixture tests
	@echo "$(CYAN)Running access-verification fixture tests...$(RESET)"
	@cd test && go test -v -timeout 20m -run TestVerifyFixture
	@echo "$(GREEN)✓ test-verify-fixture passed$(RESET)"

##@ Access verification (live AWS runs; never part of validate or test)
# Full mode deploys verify/fixture into an allow-listed sandbox (verify/sandbox-accounts.txt),
# runs the persona matrix, and destroys everything. Probe mode observes an existing deployment
# without changing it. See verify/README.md.
VERIFY_RUN_FILE := verify/out/.last-run-id
RUN_ID ?= $(shell cat $(VERIFY_RUN_FILE) 2>/dev/null)
VERIFY_OUT = $(CURDIR)/verify/out/$(RUN_ID)
LFVERIFY := $(CURDIR)/verify/out/lfverify
VERIFY_TF = cd verify/fixture && TF_DATA_DIR=$(VERIFY_OUT)/.terraform terraform
VERIFY_VARS = -var run_id=$(RUN_ID) $(if $(RUNNER_ROLE_ARN),-var runner_role_arn=$(RUNNER_ROLE_ARN),) $(if $(READ_ONLY_RUNNER_ARNS),-var 'read_only_runner_arns=$(READ_ONLY_RUNNER_ARNS)',)
VERIFY_MANIFEST = $(VERIFY_OUT)/manifest.json
INJECT ?=
INJECT_PERSONA ?=

verify-build: ## Build the lfverify CLI into verify/out/lfverify
	@cd verify && go build -o out/lfverify ./cmd/lfverify

verify-new-run: ## Start a new run ID (six hex characters) and remember it
	@mkdir -p verify/out && openssl rand -hex 3 > $(VERIFY_RUN_FILE) && echo "$(CYAN)Run ID: $$(cat $(VERIFY_RUN_FILE))$(RESET)"

verify-init:
	@test -n "$(RUN_ID)" || { echo "$(RED)✗ RUN_ID is not set; run make verify-new-run$(RESET)"; exit 1; }
	@mkdir -p $(VERIFY_OUT)
	@$(VERIFY_TF) init -input=false -reconfigure -backend-config=path=$(VERIFY_OUT)/terraform.tfstate >/dev/null

verify-up: check-aws verify-build verify-init ## Preflight, then apply fixture stage 1
	@$(LFVERIFY) preflight --mode full --run-id $(RUN_ID) --out $(VERIFY_OUT) --allow-list $(CURDIR)/verify/sandbox-accounts.txt
	@$(VERIFY_TF) apply -input=false -auto-approve $(VERIFY_VARS) -var stage=1
	@$(VERIFY_TF) output -json manifest > $(VERIFY_MANIFEST)
	@echo "$(GREEN)✓ Fixture stage 1 applied (run $(RUN_ID))$(RESET)"

verify-seed: check-aws verify-build verify-init ## Seed fixture tables, then apply stage 2
	@$(LFVERIFY) seed --manifest $(VERIFY_MANIFEST) --out $(VERIFY_OUT)
	@$(VERIFY_TF) apply -input=false -auto-approve $(VERIFY_VARS) -var stage=2
	@$(VERIFY_TF) output -json manifest > $(VERIFY_MANIFEST)
	@$(LFVERIFY) preflight --mode full --run-id $(RUN_ID) --out $(VERIFY_OUT) --manifest $(VERIFY_MANIFEST) --allow-list $(CURDIR)/verify/sandbox-accounts.txt
	@echo "$(GREEN)✓ Fixture seeded and stage 2 applied (run $(RUN_ID))$(RESET)"

verify-run: check-aws verify-build ## Run the full-mode matrix against the applied fixture
	@$(LFVERIFY) run --mode full --matrix $(CURDIR)/verify/matrix/full.yaml --manifest $(VERIFY_MANIFEST) --out $(VERIFY_OUT)

verify-apply: verify-init ## Re-apply stage 2 with an overlay (INJECT, INJECT_PERSONA); used by detect
	@$(VERIFY_TF) apply -input=false -auto-approve $(VERIFY_VARS) -var stage=2 -var inject=$(INJECT) -var inject_persona=$(INJECT_PERSONA)
	@$(VERIFY_TF) output -json manifest > $(VERIFY_MANIFEST)

verify-down: check-aws verify-build verify-init ## Destroy the fixture, sweep leftovers, restore data-lake settings
	@# Preflight writes settings.before.json only when it passes. Without it nothing was deployed,
	@# and the stage 1 apply below would deploy into an account preflight refused (FR-003).
	@test -f $(VERIFY_OUT)/settings.before.json || { echo "$(RED)✗ No settings.before.json for run $(RUN_ID): preflight never passed, so there is nothing to tear down. Use make verify-cleanup to sweep by prefix.$(RESET)"; exit 1; }
	@# Stage 1 removes the table-level labels, grants, and filters while their tables exist; then
	@# the seeded tables are dropped, because namespaces holding tables cannot be destroyed
	@$(VERIFY_TF) apply -input=false -auto-approve $(VERIFY_VARS) -var stage=1 || echo "$(YELLOW)⚠ stage 1 apply failed; continuing teardown$(RESET)"
	@$(LFVERIFY) cleanup --tables-only --run-id $(RUN_ID) --out $(VERIFY_OUT)
	@$(VERIFY_TF) destroy -input=false -auto-approve $(VERIFY_VARS) -var stage=2 || echo "$(YELLOW)⚠ terraform destroy failed; cleanup sweeps by prefix$(RESET)"
	@$(LFVERIFY) cleanup --run-id $(RUN_ID) --out $(VERIFY_OUT)
	@echo "$(GREEN)✓ Run $(RUN_ID) torn down$(RESET)"

verify-full: check-aws ## Full mode: new run, deploy, seed, run, and always tear down
	@$(MAKE) -s verify-new-run
	@run=$$(cat $(VERIFY_RUN_FILE)); start=$$(date +%s); \
	  $(MAKE) verify-up RUN_ID=$$run && $(MAKE) verify-seed RUN_ID=$$run && $(MAKE) verify-run RUN_ID=$$run; status=$$?; \
	  $(MAKE) verify-down RUN_ID=$$run; \
	  echo "$(CYAN)Run $$run finished in $$(( $$(date +%s) - start ))s with status $$status$(RESET)"; exit $$status

verify-detect: check-aws ## Full mode: apply each misconfiguration overlay and require a failing case
	@$(MAKE) -s verify-new-run
	@run=$$(cat $(VERIFY_RUN_FILE)); \
	  $(MAKE) verify-up RUN_ID=$$run && $(MAKE) verify-seed RUN_ID=$$run && \
	  $(LFVERIFY) detect --matrix $(CURDIR)/verify/matrix/full.yaml --manifest $(CURDIR)/verify/out/$$run/manifest.json --out $(CURDIR)/verify/out/$$run \
	    --apply-cmd "$(MAKE) -s -C $(CURDIR) verify-apply RUN_ID={run} INJECT={inject} INJECT_PERSONA={persona}"; status=$$?; \
	  $(MAKE) verify-down RUN_ID=$$run; exit $$status

verify-cleanup: check-aws verify-build ## Sweep an interrupted run by prefix and restore settings (RUN_ID=...)
	@test -n "$(RUN_ID)" || { echo "$(RED)✗ RUN_ID is required$(RESET)"; exit 1; }
	@$(LFVERIFY) cleanup --run-id $(RUN_ID) --out $(VERIFY_OUT)

verify-probe: check-aws verify-build ## Probe mode against an existing deployment (MATRIX=...); read-only
	@test -n "$(MATRIX)" || { echo "$(RED)✗ MATRIX is required$(RESET)"; exit 1; }
	@$(MAKE) -s verify-new-run
	@run=$$(cat $(VERIFY_RUN_FILE)); out=$(CURDIR)/verify/out/$$run; \
	  $(LFVERIFY) preflight --mode probe --matrix $(abspath $(MATRIX)) || exit $$?; \
	  $(LFVERIFY) snapshot --matrix $(abspath $(MATRIX)) --name before --run-id $$run --out $$out || exit $$?; \
	  $(LFVERIFY) run --mode probe --matrix $(abspath $(MATRIX)) --run-id $$run --out $$out; status=$$?; \
	  $(LFVERIFY) snapshot --matrix $(abspath $(MATRIX)) --name after --run-id $$run --out $$out && \
	  $(LFVERIFY) diff --run-id $$run --out $$out || status=1; \
	  echo "$(CYAN)Probe run $$run: audit with make verify-probe-audit RUN_ID=$$run after 15 minutes$(RESET)"; exit $$status

verify-probe-audit: check-aws verify-build ## List change events from a probe run's sessions (RUN_ID=...)
	@test -n "$(RUN_ID)" || { echo "$(RED)✗ RUN_ID is required$(RESET)"; exit 1; }
	@$(LFVERIFY) audit --run-id $(RUN_ID) $(if $(SINCE),--since $(SINCE),)

##@ Documentation
docs: ## Generate documentation
	@echo "$(CYAN)Generating documentation...$(RESET)"
	@if command -v terraform-docs >/dev/null 2>&1; then \
		terraform-docs .; \
		echo "$(GREEN)✓ Documentation generated$(RESET)"; \
	else \
		echo "$(YELLOW)⚠ terraform-docs not installed, skipping$(RESET)"; \
	fi

##@ Examples
plan-examples: check-aws ## Plan all examples (useful for verification)
	@echo "$(CYAN)Planning all examples...$(RESET)"
	@# secondary and standalone-tenant need the caller to already be a Lake Formation data-lake
	@# admin; set LAKEHOUSE_TEST_LF_ADMIN=true in such an account (see test/README.md)
	@for dir in examples/*/; do \
		vars=""; \
		grep -qs 'variable "name_prefix"' "$$dir/variables.tf" && vars="-var=name_prefix=make-$$(date +%s)"; \
		if [ "$${LAKEHOUSE_TEST_LF_ADMIN:-}" != "true" ]; then \
			case "$$dir" in \
				examples/secondary/) echo "$(YELLOW)↷ $$dir skipped (needs LAKEHOUSE_TEST_LF_ADMIN=true)$(RESET)"; continue ;; \
				examples/standalone-tenant/) vars="$$vars -var=verify_deployer_admin=false" ;; \
			esac; \
		fi; \
		echo "$(CYAN)Planning $$dir...$(RESET)"; \
		(cd "$$dir" && \
			terraform init && \
			terraform plan $$vars -out=makefile.tfplan) || exit 1; \
		echo "$(GREEN)✓ $$dir plan successful$(RESET)"; \
	done

apply-examples: check-aws ## Apply examples using existing plans (requires plan-examples first)
	@echo "$(CYAN)Applying all example plans...$(RESET)"
	@echo "$(YELLOW)⚠ This will create real AWS resources and incur costs!$(RESET)"
	@echo "$(YELLOW)Press Ctrl+C within 10 seconds to cancel...$(RESET)"
	@sleep 10
	@for dir in examples/*/; do \
		if [ -f "$$dir/makefile.tfplan" ]; then \
			echo "$(CYAN)Applying $$dir...$(RESET)"; \
			(cd "$$dir" && terraform apply makefile.tfplan) || exit 1; \
			echo "$(GREEN)✓ $$dir applied successfully$(RESET)"; \
		else \
			echo "$(RED)✗ No plan file found for $$dir$(RESET)"; \
			echo "$(YELLOW)Run 'make plan-examples' first$(RESET)"; \
			exit 1; \
		fi; \
	done
	@echo "$(GREEN)✓ All examples applied successfully$(RESET)"
	@echo "$(YELLOW)⚠ Don't forget to run 'make destroy-examples' to cleanup resources$(RESET)"

destroy-examples: check-aws ## Destroy any leftover example resources (cleanup)
	@echo "$(CYAN)Cleaning up example resources...$(RESET)"
	@for dir in examples/*/; do \
		echo "$(CYAN)Destroying $$dir...$(RESET)"; \
		(cd "$$dir" && \
			terraform destroy -auto-approve || true); \
		echo "$(GREEN)✓ $$dir cleanup complete$(RESET)"; \
	done

test-examples-full: check-aws ## Full example workflow: plan -> apply -> destroy
	@echo "$(CYAN)Running full example workflow...$(RESET)"
	@echo "$(YELLOW)⚠ This will create and destroy real AWS resources!$(RESET)"
	@$(MAKE) plan-examples
	@$(MAKE) apply-examples
	@sleep 10 # Give resources time to stabilize
	@$(MAKE) destroy-examples
	@echo "$(GREEN)✓ Full example workflow completed$(RESET)"

##@ Status and Information
status: ## Show comprehensive module status
	@echo "$(CYAN)=== Module Status ===$(RESET)"
	@echo "$(YELLOW)Repository:$(RESET)"
	@git remote get-url origin 2>/dev/null || echo "No remote origin"
	@echo "$(YELLOW)Current Branch:$(RESET)"
	@git branch --show-current 2>/dev/null || echo "Not a git repository"
	@echo "$(YELLOW)Git Status:$(RESET)"
	@git status --porcelain 2>/dev/null | head -10 || echo "Clean working directory"
	@echo "$(YELLOW)Terraform Version:$(RESET)"
	@terraform version | head -1
	@echo "$(YELLOW)Go Version:$(RESET)"
	@go version 2>/dev/null || echo "Go not installed"
	@echo "$(YELLOW)Module Files:$(RESET)"
	@find . -name "*.tf" -not -path "./.terraform/*" | wc -l | xargs echo "Terraform files:"
	@echo "$(YELLOW)Test Files:$(RESET)"
	@find test -name "*_test.go" 2>/dev/null | wc -l | xargs echo "Test files:" || echo "Test files: 0"
	@echo "$(YELLOW)Examples:$(RESET)"
	@find examples -name "main.tf" 2>/dev/null | wc -l | xargs echo "Example directories:" || echo "Example directories: 0"

##@ Cleanup
clean: ## Clean up temporary files and caches
	@echo "$(CYAN)Cleaning up...$(RESET)"
	@find . -name ".terraform" -type d -exec rm -rf {} + 2>/dev/null || true
	@find . -name ".terraform.lock.hcl" -delete 2>/dev/null || true
	@find . -name "terraform.tfstate*" -delete 2>/dev/null || true
	@find . -name "*.tfplan" -delete 2>/dev/null || true
	@find . -name "*.log" -delete 2>/dev/null || true
	@echo "$(GREEN)✓ Cleanup complete$(RESET)"

##@ AWS and Prerequisites
check-aws: ## Check for valid AWS session and credentials
	@echo "$(CYAN)Checking AWS session...$(RESET)"
	@if ! command -v aws >/dev/null 2>&1; then \
		echo "$(RED)✗ AWS CLI not installed$(RESET)"; \
		echo "$(YELLOW)Install with: brew install awscli (macOS) or apt-get install awscli (Ubuntu)$(RESET)"; \
		exit 1; \
	fi
	@if ! aws sts get-caller-identity >/dev/null 2>&1; then \
		echo "$(RED)✗ No valid AWS session found$(RESET)"; \
		echo "$(YELLOW)Configure AWS credentials with:$(RESET)"; \
		echo "  aws configure"; \
		echo "  aws sso login"; \
		echo "  export AWS_PROFILE=your-profile"; \
		exit 1; \
	fi
	@echo "$(GREEN)✓ AWS session valid$(RESET)"
	@echo "$(CYAN)AWS Identity:$(RESET)"
	@aws sts get-caller-identity --output table 2>/dev/null || echo "Could not retrieve identity details"

aws-info: check-aws ## Show detailed AWS session information
	@echo "$(CYAN)=== AWS Session Details ===$(RESET)"
	@echo "$(YELLOW)Identity:$(RESET)"
	@aws sts get-caller-identity --output table
	@echo "$(YELLOW)Region:$(RESET)"
	@aws configure get region 2>/dev/null || echo "Not configured (will use default)"
	@echo "$(YELLOW)Profile:$(RESET)"
	@echo "$${AWS_PROFILE:-default}"

##@ AI Agent Shortcuts
resource-count: ## Show expected resource counts for verification
	@echo "$(CYAN)Expected Resource Counts:$(RESET)"
	@echo "$(YELLOW)Defaults example:$(RESET) 9 resources (KMS key/alias, S3 bucket + configs, DynamoDB table)"
	@echo "$(YELLOW)Complete example:$(RESET) 15 resources (+ SNS topic, 5 CloudWatch alarms)"
	@echo "$(YELLOW)Enabled=false:$(RESET) 0 resources"

verify-tests: ## Verify test structure and names
	@echo "$(CYAN)Test Verification:$(RESET)"
	@cd test && go list -f '{{.Name}}: {{.TestGoFiles}}' . 2>/dev/null || echo "No test package found"
	@echo "$(YELLOW)Expected tests:$(RESET)"
	@echo "  - TestTerraformDefaultsExample"
	@echo "  - TestTerraformCompleteExample" 
	@echo "  - TestEnabledFalse"

##@ Development
update-branch: ## Update current branch with latest main (merge if exists on origin, rebase if local only)
	@if [ -n "$$(git status --porcelain)" ]; then echo "$(RED)✗ Working directory is not clean. Please commit or stash your changes.$(RESET)"; exit 1; fi
	@echo "$(CYAN)Updating branch with latest main...$(RESET)"
	@git fetch origin
	@CURRENT_BRANCH=$$(git branch --show-current); \
	if git show-ref --verify --quiet "refs/remotes/origin/$$CURRENT_BRANCH"; then \
		echo "$(CYAN)Branch $$CURRENT_BRANCH exists on origin, merging main...$(RESET)"; \
		git merge origin/main || (echo "$(RED)✗ Merge conflict detected. Please resolve manually.$(RESET)" && exit 1); \
		echo "$(GREEN)✓ Merged main into $$CURRENT_BRANCH$(RESET)"; \
	else \
		echo "$(CYAN)Branch $$CURRENT_BRANCH is local only, rebasing on main...$(RESET)"; \
		git rebase origin/main || (echo "$(RED)✗ Rebase conflict detected. Please resolve manually.$(RESET)" && exit 1); \
		echo "$(GREEN)✓ Rebased $$CURRENT_BRANCH on main$(RESET)"; \
	fi

all: clean validate ## Run complete development workflow
	@echo "$(GREEN)✓ Complete workflow finished$(RESET)"
