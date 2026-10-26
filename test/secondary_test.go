package test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSecondaryExampleSkipsAccountIntegration needs the test role to be a Lake Formation
// data-lake admin already (see README); it skips otherwise.
func TestSecondaryExampleSkipsAccountIntegration(t *testing.T) {
	t.Parallel()
	if !isLFAdmin() {
		t.Skipf("set %s=true in an account where the test role is a Lake Formation data-lake admin", lfAdminEnv)
	}

	plan := planExample(t, "secondary", map[string]interface{}{
		"name_prefix": generateTestNamePrefix("sec"),
	})
	changes := managedChanges(plan)
	for addr := range changes {
		assert.False(t, strings.Contains(addr, "module.catalog_integration"), addr)
	}
	for _, addr := range []string{
		`module.main.aws_s3tables_table_bucket.zone["raw"]`,
		`module.main.aws_lakeformation_lf_tag.sensitivity[0]`,
		`module.main.module.tenant["research"].aws_lakeformation_lf_tag.tenant`,
		`module.main.terraform_data.secondary_admin_guard[0]`,
	} {
		assert.Contains(t, changes, addr)
	}
}
