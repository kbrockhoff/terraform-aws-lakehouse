package test

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAddingTenantTouchesOnlyThatTenant checks FR-001: onboarding a tenant changes nothing but
// that tenant's module instance.
func TestAddingTenantTouchesOnlyThatTenant(t *testing.T) {
	t.Parallel()
	name := generateTestNamePrefix("ten")

	before := planExample(t, "complete", completeVars(name))
	vars := completeVars(name)
	vars["extra_tenants"] = map[string]interface{}{
		"marketing": map[string]interface{}{
			"owner":       "marketing-data@example.com",
			"sensitivity": "internal",
		},
	}
	with := planExample(t, "complete", vars)

	beforeChanges, withChanges := managedChanges(before), managedChanges(with)
	marketing := `module.main.module.tenant["marketing"].`
	added := 0
	for addr := range withChanges {
		if _, ok := beforeChanges[addr]; !ok {
			assert.True(t, strings.HasPrefix(addr, marketing), "unexpected new address %s", addr)
			added++
		}
	}
	for addr, rc := range beforeChanges {
		other, ok := withChanges[addr]
		if assert.True(t, ok, "address disappeared: %s", addr) {
			a, _ := json.Marshal(rc.Change.After)
			b, _ := json.Marshal(other.Change.After)
			assert.JSONEq(t, string(a), string(b), "%s changed when a tenant was added", addr)
		}
	}
	assert.Greater(t, added, 0, "the marketing tenant must add resources")
}

var spaces = regexp.MustCompile(`\s+`)

// typeExpr returns the whitespace-normalized type expression of a variable block.
func typeExpr(t *testing.T, path, variable string) string {
	t.Helper()
	src, err := os.ReadFile(path)
	require.NoError(t, err)
	text := string(src)
	start := strings.Index(text, `variable "`+variable+`" {`)
	require.GreaterOrEqual(t, start, 0, "variable %s not found in %s", variable, path)
	typeAt := strings.Index(text[start:], "type = ") + start + len("type = ")
	depth, end := 0, typeAt
	for i := typeAt; i < len(text); i++ {
		switch text[i] {
		case '(', '{':
			depth++
		case ')', '}':
			depth--
		}
		if depth == 0 && text[i] == ')' {
			end = i + 1
			break
		}
	}
	return spaces.ReplaceAllString(text[typeAt:end], " ")
}

// TestTenantTypeInSync checks that the root tenants element type and the submodule tenant type
// are identical, so one definition works in both entry points.
func TestTenantTypeInSync(t *testing.T) {
	t.Parallel()
	root := typeExpr(t, "../variables.tf", "tenants")
	tenant := typeExpr(t, "../modules/tenant/variables.tf", "tenant")
	require.True(t, strings.HasPrefix(root, "map(") && strings.HasSuffix(root, ")"), root)
	assert.Equal(t, tenant, strings.TrimSuffix(strings.TrimPrefix(root, "map("), ")"))
}

func TestZoneOverrideWithoutConsumption(t *testing.T) {
	t.Parallel()
	plan := planExample(t, "defaults", map[string]interface{}{
		"name_prefix":  generateTestNamePrefix("zon"),
		"zones_config": map[string]interface{}{"zones": []string{"raw", "curated"}},
	})
	changes := managedChanges(plan)
	assert.NotEmpty(t, changes)
	for addr, rc := range changes {
		assert.NotContains(t, addr, "consumption")
		if p := block(after(rc), "lf_tag_policy"); p != nil {
			assert.NotContains(t, p["catalog_id"], "consumption", addr)
		}
	}
}
