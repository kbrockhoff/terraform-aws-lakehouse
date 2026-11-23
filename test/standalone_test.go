package test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/terraform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// standaloneVars turns off the deploying-role admin check unless the test account has the test
// role as a Lake Formation admin.
func standaloneVars() map[string]interface{} {
	return map[string]interface{}{"verify_deployer_admin": isLFAdmin()}
}

// TestStandaloneTenantMatchesPlatformApplied checks FR-001a: the same tenant definition yields
// the same resources whether a pipeline or the platform applies it.
func TestStandaloneTenantMatchesPlatformApplied(t *testing.T) {
	t.Parallel()

	standalone := planExample(t, "standalone-tenant", standaloneVars())
	platform := planExample(t, "complete", completeVars("complete-ex"))

	strip := func(changes map[string]interface{}, prefix string) []string {
		var out []string
		for addr := range changes {
			if strings.HasPrefix(addr, prefix) && !strings.Contains(addr, "terraform_data.admin_guard") {
				out = append(out, strings.TrimPrefix(addr, prefix))
			}
		}
		return out
	}
	toAny := func(plan *terraform.PlanStruct) map[string]interface{} {
		m := map[string]interface{}{}
		for k, v := range managedChanges(plan) {
			m[k] = v
		}
		return m
	}
	s := strip(toAny(standalone), "module.tenant.")
	p := strip(toAny(platform), `module.main.module.tenant["sales"].`)
	require.NotEmpty(t, s)
	assert.ElementsMatch(t, p, s)
}

func TestStandaloneTenantReportsOwnCost(t *testing.T) {
	t.Parallel()
	plan := planExample(t, "standalone-tenant", standaloneVars())
	assert.Greater(t, number(outputAfter(plan, "monthly_cost_estimate")), 0.0)
}

func TestStandaloneTenantRejectsInvalidAgainstContract(t *testing.T) {
	t.Parallel()
	vars := standaloneVars()
	vars["tenant_overrides"] = map[string]interface{}{"domain": "marketing"}
	_, err := terraform.InitAndPlanE(t, exampleOptions(t, "standalone-tenant", vars))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "domain and reader domains must be platform domain values")
}

// TestStandaloneTenantNeedsKeyAdministrators checks that a designated standalone tenant fails the
// plan, rather than creating a key nobody on the platform administers, when its platform contract
// predates key administrators (the example's contract has none).
func TestStandaloneTenantNeedsKeyAdministrators(t *testing.T) {
	t.Parallel()
	vars := standaloneVars()
	vars["tenant_overrides"] = map[string]interface{}{"compliance_frameworks": []string{"HIPAA"}, "regulations": []string{"gdpr", "hipaa"}}
	_, err := terraform.InitAndPlanE(t, exampleOptions(t, "standalone-tenant", vars))
	require.Error(t, err)
	assert.Contains(t, spaces.ReplaceAllString(err.Error(), " "), "the platform contract names no key administrators")
}

// TestStandaloneTenantValidatesContractKeyPrincipals checks that a standalone tenant validates the
// KMS principals it takes from the contract, which bypasses the root's input validation.
func TestStandaloneTenantValidatesContractKeyPrincipals(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../examples/standalone-tenant/platform_contract.auto.tfvars.json")
	require.NoError(t, err)
	var tfvars map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &tfvars))
	contract := tfvars["platform_contract"].(map[string]interface{})
	for _, c := range []struct{ name, arn string }{
		{"wildcard", "*"},
		{"other account", "arn:aws:iam::999999999999:role/key-admin"},
	} {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			bad := map[string]interface{}{}
			for k, v := range contract {
				bad[k] = v
			}
			bad["key_administrator_arns"] = []string{c.arn}
			vars := standaloneVars()
			vars["platform_contract"] = bad
			_, err := terraform.InitAndPlanE(t, exampleOptions(t, "standalone-tenant", vars))
			require.Error(t, err)
			assert.Contains(t, spaces.ReplaceAllString(err.Error(), " "), "platform.key_administrator_arns entries must be IAM role or user ARNs without wildcards, in the platform account")
		})
	}
}

// TestStandaloneTenantRejectsUnapprovedShare checks spec 003 FR-023: a standalone tenant shares
// only with accounts the platform contract approves.
func TestStandaloneTenantRejectsUnapprovedShare(t *testing.T) {
	t.Parallel()
	vars := standaloneVars()
	vars["tenant_overrides"] = map[string]interface{}{"shares": map[string]interface{}{
		"partner": map[string]interface{}{"account_id": "444455556666", "sensitivity_values": []string{"public"}},
	}}
	_, err := terraform.InitAndPlanE(t, exampleOptions(t, "standalone-tenant", vars))
	require.Error(t, err)
	assert.Contains(t, spaces.ReplaceAllString(err.Error(), " "), "target accounts that are not approved")
}
