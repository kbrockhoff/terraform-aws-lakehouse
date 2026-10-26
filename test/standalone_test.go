package test

import (
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
