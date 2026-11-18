package test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/terraform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Spec 003: same-Region cross-account shares (FR-021 to FR-028). The complete example approves
// consumer 111122223333 and shares the sales tenant's consumption zone with it.

const consumerAccount = "111122223333"

func sharePrefix(tenant string) string {
	return `module.main.module.tenant["` + tenant + `"].`
}

func shareOverride(tenant string, share map[string]interface{}) map[string]interface{} {
	return override(tenant, map[string]interface{}{"shares": map[string]interface{}{"partner": share}})
}

func TestShareGrantsReadOnlyWithGrantOption(t *testing.T) {
	t.Parallel()
	name := generateTestNamePrefix("shr")
	changes := managedChanges(planExample(t, "complete", completeVars(name)))
	key := func(k string) string { return name + "-" + k }

	db := changes[sharePrefix("sales")+`aws_lakeformation_permissions.share_database["partner"]`]
	require.NotNil(t, db)
	assert.Equal(t, consumerAccount, after(db)["principal"])
	assert.Equal(t, []string{"DESCRIBE"}, strs(after(db)["permissions"]))
	assert.Equal(t, []string{"DESCRIBE"}, strs(after(db)["permissions_with_grant_option"]))
	assert.Equal(t, map[string][]string{
		key("tenant-sales"): {"member"},
		key("zone"):         {"consumption"},
	}, lfExpression(db))

	tbl := changes[sharePrefix("sales")+`aws_lakeformation_permissions.share_table["partner"]`]
	require.NotNil(t, tbl)
	assert.Equal(t, []string{"DESCRIBE", "SELECT"}, strs(after(tbl)["permissions"]))
	assert.Equal(t, []string{"DESCRIBE", "SELECT"}, strs(after(tbl)["permissions_with_grant_option"]), "grant option limited to the same read permissions")
	expr := lfExpression(tbl)
	assert.Equal(t, []string{"member"}, expr[key("tenant-sales")], "every share is bounded by the tenant tag")
	assert.Equal(t, []string{"internal", "public"}, expr[key("sensitivity")])
	for _, r := range []string{"reg-gdpr", "reg-hipaa", "reg-pci_dss"} {
		assert.Equal(t, []string{"no"}, expr[key(r)], "regulated data is excluded by default")
	}
	assert.Equal(t, []string{"no"}, expr[key("tenant-sales-rf-gdpranalyst")], "row-filtered tables are not shared")
}

func TestShareTagDescribe(t *testing.T) {
	t.Parallel()
	name := generateTestNamePrefix("shd")
	grants := withPrefix(managedChanges(planExample(t, "complete", completeVars(name))), sharePrefix("sales")+"aws_lakeformation_permissions.share_tag_describe[")
	keys := map[string][]string{}
	for addr, rc := range grants {
		a := after(rc)
		assert.Equal(t, consumerAccount, a["principal"], addr)
		assert.Equal(t, []string{"DESCRIBE"}, strs(a["permissions"]), addr)
		assert.Equal(t, []string{"DESCRIBE"}, strs(a["permissions_with_grant_option"]), addr)
		tag := block(a, "lf_tag")
		require.NotNil(t, tag, addr)
		keys[tag["key"].(string)] = strs(tag["values"])
	}
	// Tenant, zone, sensitivity, three regulations, and the row-filter exclusion key
	assert.Len(t, keys, 7)
	assert.Equal(t, []string{"internal", "public"}, keys[name+"-sensitivity"], "only the shared values")
	assert.Equal(t, []string{"consumption"}, keys[name+"-zone"])
}

// TestNoConsumerInKeyPolicies checks FR-026: no key policy names a consumer account.
func TestNoConsumerInKeyPolicies(t *testing.T) {
	t.Parallel()
	plan := planExample(t, "complete", completeVars(generateTestNamePrefix("nck")))
	checked := 0
	for _, addr := range planDataSources(plan) {
		if !strings.Contains(addr, "aws_iam_policy_document") || !(strings.Contains(addr, "kms") || strings.Contains(addr, "catalog_key")) {
			continue
		}
		doc, err := json.Marshal(dataValues(plan, addr))
		require.NoError(t, err)
		assert.NotContains(t, string(doc), consumerAccount, addr)
		checked++
	}
	assert.GreaterOrEqual(t, checked, 3, "platform, catalog, and designated tenant key policies")
}

func TestShareRejectsUnapprovedAccount(t *testing.T) {
	t.Parallel()
	vars := completeVars(generateTestNamePrefix("sun"))
	for k, v := range shareOverride("finance", map[string]interface{}{"account_id": "444455556666", "sensitivity_values": []string{"public"}}) {
		vars[k] = v
	}
	_, err := terraform.InitAndPlanE(t, exampleOptions(t, "complete", vars))
	require.Error(t, err)
	assert.Contains(t, spaces.ReplaceAllString(err.Error(), " "), "target accounts that are not approved")
}

func TestShareRejectsCurrentAccount(t *testing.T) {
	t.Parallel()
	plan := planExample(t, "complete", completeVars(generateTestNamePrefix("scu")))
	account, _ := dataValues(plan, "data.aws_caller_identity.current")["account_id"].(string)
	require.Regexp(t, `^[0-9]{12}$`, account)

	vars := completeVars(generateTestNamePrefix("scu"))
	vars["sharing_config"] = map[string]interface{}{"consumer_accounts": map[string]interface{}{
		consumerAccount: map[string]interface{}{"purpose": "example"},
		account:         map[string]interface{}{"purpose": "self"},
	}}
	_, err := terraform.InitAndPlanE(t, exampleOptions(t, "complete", vars))
	require.Error(t, err)
	msg := spaces.ReplaceAllString(err.Error(), " ")
	assert.True(t, strings.Contains(msg, "must not include this account") || strings.Contains(msg, "cannot target the platform's own account"), msg)
}

func TestDesignatedTenantShareRequiresAcknowledgement(t *testing.T) {
	t.Parallel()
	vars := completeVars(generateTestNamePrefix("sda"))
	for k, v := range shareOverride("claims", map[string]interface{}{"account_id": consumerAccount, "sensitivity_values": []string{"public"}}) {
		vars[k] = v
	}
	_, err := terraform.InitAndPlanE(t, exampleOptions(t, "complete", vars))
	require.Error(t, err)
	assert.Contains(t, spaces.ReplaceAllString(err.Error(), " "), "Set compliance_acknowledged = true")
}

// TestRemovingApprovedAccountStillReferencedFails checks FR-025.
func TestRemovingApprovedAccountStillReferencedFails(t *testing.T) {
	t.Parallel()
	vars := completeVars(generateTestNamePrefix("srm"))
	vars["sharing_config"] = map[string]interface{}{"consumer_accounts": map[string]interface{}{}}
	_, err := terraform.InitAndPlanE(t, exampleOptions(t, "complete", vars))
	require.Error(t, err)
	assert.Contains(t, spaces.ReplaceAllString(err.Error(), " "), "partner target accounts that are not approved")
}
