package test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSecondaryExampleSkipsAccountIntegration needs an account where another deployment owns the
// account settings: the test role is a Lake Formation data-lake admin and Glue catalog encryption
// is on (see README). It skips otherwise.
func TestSecondaryExampleSkipsAccountIntegration(t *testing.T) {
	t.Parallel()
	if !isLFAdmin() {
		t.Skipf("set %s=true in an account where the test role is a Lake Formation data-lake admin and catalog encryption is on", lfAdminEnv)
	}

	plan := planExample(t, "secondary", map[string]interface{}{
		"name_prefix": generateTestNamePrefix("sec"),
	})
	changes := managedChanges(plan)
	for addr := range changes {
		assert.False(t, strings.Contains(addr, "module.catalog_integration"), addr)
		assert.False(t, strings.Contains(addr, "module.account_settings"), "non-owners change no account setting: %s", addr)
	}
	// The guard reads the live settings it depends on (spec 003 FR-001)
	assert.Contains(t, planDataSources(plan), "module.main.data.aws_glue_data_catalog_encryption_settings.current[0]")
	assert.Contains(t, planDataSources(plan), "module.main.data.aws_lakeformation_data_lake_settings.current[0]")
	for _, addr := range []string{
		`module.main.aws_s3tables_table_bucket.zone["raw"]`,
		`module.main.aws_lakeformation_lf_tag.sensitivity[0]`,
		`module.main.module.tenant["research"].aws_lakeformation_lf_tag.tenant`,
		`module.main.terraform_data.settings_guard[0]`,
	} {
		assert.Contains(t, changes, addr)
	}
}
