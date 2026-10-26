package test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/terraform"
	test_structure "github.com/gruntwork-io/terratest/modules/test-structure"
	tfjson "github.com/hashicorp/terraform-json"
)

// lfAdminEnv marks an AWS account where the test role is already a Lake Formation data-lake
// admin. Tests that need that (secondary deployment, standalone admin check) skip without it.
const lfAdminEnv = "LAKEHOUSE_TEST_LF_ADMIN"

func isLFAdmin() bool {
	return os.Getenv(lfAdminEnv) == "true"
}

// exampleOptions copies the repository to a temp folder, so parallel tests never share a
// .terraform directory, and returns options for the given example.
func exampleOptions(t *testing.T, example string, vars map[string]interface{}) *terraform.Options {
	t.Helper()
	dir := test_structure.CopyTerraformFolderToTemp(t, "..", filepath.Join("examples", example))
	opts := getBaseTerraformOptions(dir)
	opts.Vars = vars
	opts.PlanFilePath = filepath.Join(dir, "plan.tfplan")
	return opts
}

// planExample runs init and a plan-only run and returns the parsed plan.
func planExample(t *testing.T, example string, vars map[string]interface{}) *terraform.PlanStruct {
	t.Helper()
	return terraform.InitAndPlanAndShowWithStruct(t, exampleOptions(t, example, vars))
}

// managedChanges returns the managed (non data source) resource changes keyed by address.
func managedChanges(plan *terraform.PlanStruct) map[string]*tfjson.ResourceChange {
	out := map[string]*tfjson.ResourceChange{}
	for addr, rc := range plan.ResourceChangesMap {
		if rc.Mode == tfjson.ManagedResourceMode {
			out[addr] = rc
		}
	}
	return out
}

// dataChanges returns data source reads deferred to apply, keyed by address. Their known
// arguments (for example policy document statements) are still in Change.After.
func dataChanges(plan *terraform.PlanStruct) map[string]*tfjson.ResourceChange {
	out := map[string]*tfjson.ResourceChange{}
	for addr, rc := range plan.ResourceChangesMap {
		if rc.Mode == tfjson.DataResourceMode {
			out[addr] = rc
		}
	}
	return out
}

// planDataSources returns every data source address in the plan: deferred reads and those
// already read during planning (prior state).
func planDataSources(plan *terraform.PlanStruct) []string {
	seen := map[string]bool{}
	for addr, rc := range plan.ResourceChangesMap {
		if rc.Mode == tfjson.DataResourceMode {
			seen[addr] = true
		}
	}
	if plan.RawPlan.PriorState != nil && plan.RawPlan.PriorState.Values != nil {
		var walk func(m *tfjson.StateModule)
		walk = func(m *tfjson.StateModule) {
			for _, r := range m.Resources {
				if r.Mode == tfjson.DataResourceMode {
					seen[r.Address] = true
				}
			}
			for _, c := range m.ChildModules {
				walk(c)
			}
		}
		walk(plan.RawPlan.PriorState.Values.RootModule)
	}
	return sortedKeys(seen)
}

func after(rc *tfjson.ResourceChange) map[string]interface{} {
	if m, ok := rc.Change.After.(map[string]interface{}); ok {
		return m
	}
	return map[string]interface{}{}
}

// block returns the first element of a nested block list attribute.
func block(m map[string]interface{}, name string) map[string]interface{} {
	if l, ok := m[name].([]interface{}); ok && len(l) > 0 {
		if b, ok := l[0].(map[string]interface{}); ok {
			return b
		}
	}
	return nil
}

func list(v interface{}) []interface{} {
	if l, ok := v.([]interface{}); ok {
		return l
	}
	return nil
}

func strs(v interface{}) []string {
	var out []string
	for _, x := range list(v) {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func tags(rc *tfjson.ResourceChange) (map[string]interface{}, bool) {
	t, ok := after(rc)["tags"].(map[string]interface{})
	return t, ok
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func withPrefix(m map[string]*tfjson.ResourceChange, prefix string) map[string]*tfjson.ResourceChange {
	out := map[string]*tfjson.ResourceChange{}
	for addr, rc := range m {
		if strings.HasPrefix(addr, prefix) {
			out[addr] = rc
		}
	}
	return out
}

// outputAfter returns the known part of a planned output value.
func outputAfter(plan *terraform.PlanStruct, name string) interface{} {
	if oc, ok := plan.RawPlan.OutputChanges[name]; ok && oc != nil {
		return oc.After
	}
	return nil
}

func number(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return -1
}

// lfExpression returns an lf_tag_policy expression as key -> sorted values.
func lfExpression(rc *tfjson.ResourceChange) map[string][]string {
	out := map[string][]string{}
	policy := block(after(rc), "lf_tag_policy")
	if policy == nil {
		return out
	}
	for _, e := range list(policy["expression"]) {
		em := e.(map[string]interface{})
		out[em["key"].(string)] = strs(em["values"])
	}
	return out
}

var tenantAddr = regexp.MustCompile(`^module\.main\.module\.tenant\["([^"]+)"\]\.`)

// tenantOf returns the tenant name of a platform-applied tenant address, or "".
func tenantOf(addr string) string {
	if m := tenantAddr.FindStringSubmatch(addr); m != nil {
		return m[1]
	}
	return ""
}
