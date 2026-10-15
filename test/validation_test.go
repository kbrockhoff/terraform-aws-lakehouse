package test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/terraform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type invalidCase struct {
	name  string
	vars  map[string]interface{}
	error string
}

func override(tenant string, attrs map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"tenant_overrides": map[string]interface{}{tenant: attrs}}
}

func reader(attrs map[string]interface{}) map[string]interface{} {
	r := map[string]interface{}{"principal_arn": "arn:aws:iam::123456789012:role/test-reader"}
	for k, v := range attrs {
		r[k] = v
	}
	return map[string]interface{}{"readers": map[string]interface{}{"test": r}}
}

func merged(maps ...map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

func TestInvalidInputsFailPlan(t *testing.T) {
	t.Parallel()

	tooManyRegulations := make([]string, 21)
	for i := range tooManyRegulations {
		tooManyRegulations[i] = fmt.Sprintf("reg%d", i)
	}
	// 24-character prefix, so <prefix>-consumption-<32-char tenant> exceeds 63 characters
	longPrefix := "abcdefghijklmnopqrstuvwx"

	cases := []invalidCase{
		{"invalid tenant name", map[string]interface{}{"extra_tenants": map[string]interface{}{
			"Sales_1": map[string]interface{}{"owner": "x@example.com", "sensitivity": "internal"}}},
			"Tenant names must match"},
		{"tenant zone not a platform zone", override("finance", map[string]interface{}{"zones": []string{"gold"}}),
			"must be a platform zone"},
		{"writer zone outside tenant zones", override("sales", map[string]interface{}{"zones": []string{"raw"}}),
			"writer and reader zones must be among the tenant's zones"},
		{"empty principal ARN", override("sales", map[string]interface{}{"stewards": []string{""}}),
			"must be an IAM role or user ARN"},
		{"reader above tenant ceiling", override("finance", reader(map[string]interface{}{
			"sensitivity_values": []string{"restricted"}})),
			"at or below the tenant ceiling"},
		{"sensitivity not configured", override("finance", map[string]interface{}{"sensitivity": "secret"}),
			"sensitivity must be one of"},
		{"regulation not configured", override("finance", map[string]interface{}{"regulations": []string{"sox"}}),
			"regulations must be platform regulation values"},
		{"hipaa without designation", map[string]interface{}{"claims_compliance_frameworks": []string{}},
			"require the matching compliance_frameworks designation"},
		{"reader regulation not on tenant", override("finance", reader(map[string]interface{}{
			"sensitivity_values": []string{"public"}, "regulations": []string{"gdpr"}})),
			"reader regulations must be a subset"},
		{"domain not configured", override("finance", map[string]interface{}{"domain": "marketing"}),
			"domain and reader domains must be platform domain values"},
		{"too many regulation values", map[string]interface{}{"governance_config": map[string]interface{}{
			"regulation_values": tooManyRegulations}},
			"at most 20 entries"},
		{"row filter with undeclared reader", override("sales", map[string]interface{}{"row_filters": map[string]interface{}{
			"x": map[string]interface{}{"reader": "nobody", "zone": "curated", "table": "t", "row_filter_expression": "1 = 1"}}}),
			"row filter's reader must be a declared reader"},
		{"platform cutoff too small", map[string]interface{}{"query_config": map[string]interface{}{
			"bytes_scanned_cutoff_per_query": 1000}},
			"must be at least 10485760"},
		{"tenant cutoff too small", override("finance", map[string]interface{}{"bytes_scanned_cutoff_per_query": 1000}),
			"bytes_scanned_cutoff_per_query must be at least 10485760"},
		{"unknown compliance framework", map[string]interface{}{"claims_compliance_frameworks": []string{"HIPAA", "ISO27001"}},
			"compliance_frameworks may contain only"},
		{"integration disabled without role", map[string]interface{}{"catalog_integration_config": map[string]interface{}{
			"enabled": false, "data_access_role_arn": "", "allow_full_table_external_data_access": false}},
			"valid IAM role ARN when enabled is false"},
		{"production without DataOwner tag", map[string]interface{}{"tags": map[string]interface{}{"CostCenter": "data-platform"}},
			`"DataOwner" tag`},
		{"zone with underscore", map[string]interface{}{"zones_config": map[string]interface{}{"zones": []string{"raw", "raw_v2"}}},
			"no underscores"},
		{"dedicated bucket name too long", merged(map[string]interface{}{"name_prefix": longPrefix},
			map[string]interface{}{"extra_tenants": map[string]interface{}{
				"abcdefghijklmnopqrstuvwxyz012345": map[string]interface{}{
					"owner": "x@example.com", "sensitivity": "internal", "compliance_frameworks": []string{"SOC2"}}}}),
			"must be at most 63 characters"},
		{"dedicated bucket collides with shared bucket", map[string]interface{}{
			"zones_config": map[string]interface{}{"zones": []string{"raw", "raw-xy"}},
			"extra_tenants": map[string]interface{}{
				"xy": map[string]interface{}{"owner": "x@example.com", "sensitivity": "internal", "zones": []string{"raw"}, "dedicated_storage": true}}},
			"must not equal a shared zone bucket name"},
		{"non-dedicated tenant above shared classification", map[string]interface{}{"governance_config": map[string]interface{}{
			"domain_values":                 []string{"general", "sales", "finance", "health"},
			"shared_storage_classification": "internal"}},
			"ranks above the shared storage classification"},
		{"unsupported storage class", map[string]interface{}{"storage_config": map[string]interface{}{"table_storage_class": "GLACIER"}},
			"must be STANDARD or INTELLIGENT_TIERING"},
		{"None without required tag enforcement", map[string]interface{}{"environment_type": "None"},
			"required_tags_config.enforced is required"},
	}

	for _, c := range cases {
		c := c
		t.Run(strings.ReplaceAll(c.name, " ", "_"), func(t *testing.T) {
			t.Parallel()
			vars := completeVars(generateTestNamePrefix("val"))
			for k, v := range c.vars {
				vars[k] = v
			}
			_, err := terraform.InitAndPlanE(t, exampleOptions(t, "complete", vars))
			require.Error(t, err, "plan must fail")
			assert.Contains(t, spaces.ReplaceAllString(err.Error(), " "), c.error)
		})
	}
}

// TestRegulatedDataInBaselineWarns checks FR-030: a designated tenant in a Baseline environment
// plans, with a warning naming the tenant.
func TestRegulatedDataInBaselineWarns(t *testing.T) {
	t.Parallel()
	vars := completeVars(generateTestNamePrefix("reg"))
	vars["environment_type"] = "Development"
	out, err := terraform.InitAndPlanE(t, exampleOptions(t, "complete", vars))
	require.NoError(t, err)
	assert.Contains(t, out, "regulated_data_in_baseline")
	assert.Contains(t, spaces.ReplaceAllString(out, " "), "Tenant claims is designated")
}
