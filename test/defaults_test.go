package test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTerraformDefaultsExample(t *testing.T) {
	t.Parallel()

	plan := planExample(t, "defaults", map[string]interface{}{
		"name_prefix": generateTestNamePrefix("def"),
	})
	changes := managedChanges(plan)

	tenant := `module.main.module.tenant["analytics"].`
	for _, addr := range []string{
		`module.main.aws_s3tables_table_bucket.zone["raw"]`,
		`module.main.aws_s3tables_table_bucket.zone["curated"]`,
		`module.main.aws_s3tables_table_bucket.zone["consumption"]`,
		`module.main.aws_lakeformation_lf_tag.sensitivity[0]`,
		`module.main.aws_lakeformation_lf_tag.domain[0]`,
		`module.main.aws_lakeformation_lf_tag.zone[0]`,
		`module.main.aws_lakeformation_lf_tag.regulation["gdpr"]`,
		`module.main.aws_lakeformation_lf_tag.regulation["hipaa"]`,
		`module.main.aws_lakeformation_lf_tag.regulation["pci_dss"]`,
		tenant + `aws_lakeformation_lf_tag.tenant`,
		tenant + `aws_s3tables_namespace.this["raw"]`,
		tenant + `aws_s3tables_namespace.this["curated"]`,
		tenant + `aws_s3tables_namespace.this["consumption"]`,
		tenant + `aws_lakeformation_resource_lf_tags.database["raw"]`,
		tenant + `aws_athena_workgroup.this`,
		`module.main.module.catalog_integration[0].aws_glue_catalog.s3tables`,
	} {
		assert.Contains(t, changes, addr)
	}

	for addr, rc := range changes {
		assert.NotEqual(t, "module.main.aws_lakeformation_lf_tag.tenant", addr, "the platform must not own a tenant tag")
		if tenantOf(addr) != "" {
			assert.NotContains(t, addr, "aws_kms_key.this", "non-designated tenants get no key")
			assert.NotContains(t, addr, "aws_s3tables_table_bucket.dedicated", "non-dedicated tenants get no buckets")
		}
		assert.NotEqual(t, "aws_cloudwatch_metric_alarm", rc.Type, "alarms are off in Development")
	}

	cost := number(outputAfter(plan, "monthly_cost_estimate"))
	assert.Greater(t, cost, 0.0)
	assert.LessOrEqual(t, cost, 5.0, "Baseline budget")

	// Shared storage carries the platform classification, the most sensitive value by default
	for _, zone := range []string{"raw", "curated", "consumption"} {
		zoneTags, ok := tags(changes[`module.main.aws_s3tables_table_bucket.zone["`+zone+`"]`])
		if assert.True(t, ok) {
			assert.Equal(t, "restricted", zoneTags["DataClassification"])
		}
	}

	// The provider cannot set a table bucket's default storage class yet (plan.md, Complexity
	// Tracking), so assert the effective value the platform publishes.
	contract, _ := outputAfter(plan, "platform_contract").(map[string]interface{})
	effective, _ := contract["effective"].(map[string]interface{})
	assert.Equal(t, "STANDARD", effective["table_storage_class"])
}

func TestCostEstimationDisabled(t *testing.T) {
	t.Parallel()
	name := generateTestNamePrefix("def")

	enabled := planExample(t, "defaults", map[string]interface{}{"name_prefix": name})
	disabled := planExample(t, "defaults", map[string]interface{}{
		"name_prefix":            name,
		"cost_estimation_config": map[string]interface{}{"enabled": false},
	})

	isTenantCostLookup := func(addr string) bool {
		return strings.Contains(addr, ".module.tenant[") && strings.Contains(addr, "module.cost_estimation")
	}
	enabledLookups := 0
	for _, addr := range planDataSources(enabled) {
		if isTenantCostLookup(addr) {
			enabledLookups++
		}
	}
	assert.Greater(t, enabledLookups, 0, "control: the enabled plan has tenant cost lookups")

	assert.Equal(t, 0.0, number(outputAfter(disabled, "monthly_cost_estimate")))
	for _, addr := range planDataSources(disabled) {
		assert.False(t, isTenantCostLookup(addr), "tenant cost lookups must be skipped: %s", addr)
	}
	assert.Equal(t, sortedKeys(managedChanges(enabled)), sortedKeys(managedChanges(disabled)))
}
