package test

import (
	"crypto/rand"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/terraform"
	test_structure "github.com/gruntwork-io/terratest/modules/test-structure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureOptions copies the repository to a temp folder and returns options for the access
// verification fixture. Every call gets a fresh run_id, so parallel plans never share names.
func fixtureOptions(t *testing.T, vars map[string]interface{}) *terraform.Options {
	t.Helper()
	dir := test_structure.CopyTerraformFolderToTemp(t, "..", filepath.Join("verify", "fixture"))
	opts := getBaseTerraformOptions(dir)
	b := make([]byte, 3)
	_, err := rand.Read(b)
	require.NoError(t, err)
	opts.Vars = map[string]interface{}{"run_id": hex.EncodeToString(b)}
	for k, v := range vars {
		opts.Vars[k] = v
	}
	opts.PlanFilePath = filepath.Join(dir, "plan.tfplan")
	return opts
}

func countWithPrefix(changes map[string]bool, prefix string) int {
	n := 0
	for addr := range changes {
		if strings.HasPrefix(addr, prefix) {
			n++
		}
	}
	return n
}

// TestVerifyFixture plans the access verification fixture (spec 002) at both stages and with
// overlays: stage 1 has no table-level grants, stage 2 adds them, and overlays change exactly
// the grants they target.
func TestVerifyFixture(t *testing.T) {
	t.Parallel()

	addresses := func(t *testing.T, vars map[string]interface{}) map[string]bool {
		plan := terraform.InitAndPlanAndShowWithStruct(t, fixtureOptions(t, vars))
		out := map[string]bool{}
		for addr := range managedChanges(plan) {
			out[addr] = true
		}
		return out
	}

	t.Run("stage1", func(t *testing.T) {
		t.Parallel()
		a := addresses(t, map[string]interface{}{"stage": 1})
		assert.Equal(t, 13, countWithPrefix(a, "aws_iam_role.persona["), "one role per persona")
		assert.Contains(t, a, "module.lakehouse.module.catalog_integration[0].aws_lakeformation_data_lake_settings.this")
		assert.Zero(t, countWithPrefix(a, "aws_lakeformation_data_cells_filter."), "named filters need tables")
		assert.Zero(t, countWithPrefix(a, "aws_lakeformation_permissions.named_table["))
		assert.Zero(t, countWithPrefix(a, "aws_lakeformation_permissions.named_columns["))
		assert.Zero(t, countWithPrefix(a, "aws_lakeformation_resource_lf_tags."))
		for addr := range a {
			assert.NotContains(t, addr, "aws_lakeformation_data_cells_filter.this", "module row filters need tables")
		}
		assert.Equal(t, 4, countWithPrefix(a, "aws_lakeformation_permissions.meta_reader["), "DATABASE and TABLE in alpha and beta")
	})

	t.Run("stage2", func(t *testing.T) {
		t.Parallel()
		a := addresses(t, map[string]interface{}{"stage": 2})
		assert.Equal(t, 3, countWithPrefix(a, "aws_lakeformation_data_cells_filter.named["))
		assert.Contains(t, a, `aws_lakeformation_permissions.named_columns["reader_cols/columns/n_alpha/orders"]`)
		assert.Contains(t, a, `aws_lakeformation_permissions.named_table["meta_writer/table/n_alpha/scratch"]`)
		assert.Equal(t, 3, countWithPrefix(a, `module.lakehouse.module.tenant["alpha"].aws_lakeformation_data_cells_filter.this[`))
		// alpha raw, alpha curated, beta curated, gamma curated
		assert.Equal(t, 4, countWithPrefix(a, "aws_lakeformation_resource_lf_tags.orders_restricted_columns["))
		assert.Contains(t, a, "aws_lakeformation_resource_lf_tags.scratch_mw[0]")
		assert.NotContains(t, a, "aws_lakeformation_permissions.inject_open_default[0]")
	})

	t.Run("inject_open_default", func(t *testing.T) {
		t.Parallel()
		plan := terraform.InitAndPlanAndShowWithStruct(t, fixtureOptions(t, map[string]interface{}{
			"stage": 2, "inject": "open_default",
		}))
		rc := managedChanges(plan)["aws_lakeformation_permissions.inject_open_default[0]"]
		require.NotNil(t, rc)
		assert.Equal(t, "IAM_ALLOWED_PRINCIPALS", after(rc)["principal"])
	})

	t.Run("inject_missing_grant", func(t *testing.T) {
		t.Parallel()
		a := addresses(t, map[string]interface{}{"stage": 2, "inject": "missing_grant", "inject_persona": "reader_rows"})
		assert.NotContains(t, a, `aws_lakeformation_permissions.named_filter["n_rows_eu"]`)
		assert.NotContains(t, a, `aws_lakeformation_permissions.named_database["reader_rows/database/n_alpha"]`)
		assert.Contains(t, a, `aws_lakeformation_permissions.named_filter["n_cells_eu"]`, "other personas keep their grants")
	})

	t.Run("inject_requires_persona", func(t *testing.T) {
		t.Parallel()
		_, err := terraform.InitAndPlanE(t, fixtureOptions(t, map[string]interface{}{"stage": 2, "inject": "extra_grant"}))
		require.Error(t, err)
		assert.Contains(t, spaces.ReplaceAllString(err.Error(), " "), "inject_persona must be a persona key")
	})
}
