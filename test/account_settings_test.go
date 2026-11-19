package test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/terraform"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Spec 003: account- and Region-wide Lake Formation and Glue Data Catalog settings.

const settingsPrefix = "module.main.module.account_settings[0]."

// catalogKeyEnv names an existing long-lived catalog key in the test account (the lfverify
// sandbox key, see verify/README.md). The module reads a supplied catalog key during planning, so
// tests that supply one need a real key; they skip without it.
const catalogKeyEnv = "LFVERIFY_CATALOG_KEY_ARN"

func existingCatalogKey(t *testing.T) string {
	t.Helper()
	key := os.Getenv(catalogKeyEnv)
	if key == "" {
		t.Skipf("set %s to an existing KMS key ARN in the test account", catalogKeyEnv)
	}
	return key
}

// dataValues returns the known values of a data source, whether it was read during planning
// (prior state) or deferred to apply (resource change).
func dataValues(plan *terraform.PlanStruct, address string) map[string]interface{} {
	if rc, ok := plan.ResourceChangesMap[address]; ok && rc.Mode == tfjson.DataResourceMode {
		return after(rc)
	}
	if plan.RawPlan.PriorState == nil || plan.RawPlan.PriorState.Values == nil {
		return nil
	}
	var found map[string]interface{}
	var walk func(m *tfjson.StateModule)
	walk = func(m *tfjson.StateModule) {
		for _, r := range m.Resources {
			if r.Address == address {
				found = r.AttributeValues
			}
		}
		for _, c := range m.ChildModules {
			walk(c)
		}
	}
	walk(plan.RawPlan.PriorState.Values.RootModule)
	return found
}

// statements returns a policy document data source's statements keyed by sid.
func statements(t *testing.T, plan *terraform.PlanStruct, address string) map[string]map[string]interface{} {
	t.Helper()
	v := dataValues(plan, address)
	require.NotNil(t, v, "policy document %s not in plan", address)
	out := map[string]map[string]interface{}{}
	for _, s := range list(v["statement"]) {
		sm := s.(map[string]interface{})
		sid, _ := sm["sid"].(string)
		out[sid] = sm
	}
	return out
}

// conditionValues returns the values of a statement condition on the given variable.
func conditionValues(stmt map[string]interface{}, variable string) []string {
	for _, c := range list(stmt["condition"]) {
		cm := c.(map[string]interface{})
		if cm["variable"] == variable {
			return strs(cm["values"])
		}
	}
	return nil
}

func encryptionAtRest(changes map[string]*tfjson.ResourceChange) map[string]interface{} {
	settings := after(changes[settingsPrefix+"aws_glue_data_catalog_encryption_settings.this"])
	return block(block(settings, "data_catalog_encryption_settings"), "encryption_at_rest")
}

// TestOwnerFlagCombinations checks FR-001: the owner flag is independent of the catalog
// integration flag, and every combination plans. Non-owners run settings_guard, whose live
// checks fail in an account without an owner deployment; that failure must come from the guard.
func TestOwnerFlagCombinations(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name               string
		owner, integration bool
	}{
		{"owner_with_integration", true, true},
		{"owner_without_integration", true, false},
		{"nonowner_with_integration", false, true},
		{"nonowner_without_integration", false, false},
	} {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			vars := completeVars(generateTestNamePrefix("own"))
			vars["account_settings_config"] = map[string]interface{}{"owner": c.owner}
			if !c.integration {
				vars["catalog_integration_config"] = map[string]interface{}{
					"enabled":              false,
					"data_access_role_arn": "arn:aws:iam::123456789012:role/owner-lf-s3tables-access",
				}
			}
			plan, err := terraform.InitAndPlanAndShowWithStructE(t, exampleOptions(t, "complete", vars))
			if !c.owner && err != nil {
				assert.Contains(t, err.Error(), "settings_guard", "a non-owner plan may fail only in its guard")
				return
			}
			require.NoError(t, err)
			changes := managedChanges(plan)
			_, hasSettings := changes[settingsPrefix+"aws_lakeformation_data_lake_settings.this"]
			_, hasIntegration := changes["module.main.module.catalog_integration[0].aws_lakeformation_resource.s3tables"]
			_, hasGuard := changes["module.main.terraform_data.settings_guard[0]"]
			assert.Equal(t, c.owner, hasSettings, "account settings follow the owner flag")
			assert.Equal(t, c.integration, hasIntegration, "registration follows the integration flag")
			assert.Equal(t, !c.owner, hasGuard, "non-owners run the settings guard")
		})
	}
}

func TestOwnerOnlyInputsRejectedOnNonOwner(t *testing.T) {
	t.Parallel()
	vars := completeVars(generateTestNamePrefix("ono"))
	vars["account_settings_config"] = map[string]interface{}{"owner": false, "cross_account_version": 5}
	_, err := terraform.InitAndPlanE(t, exampleOptions(t, "complete", vars))
	require.Error(t, err)
	assert.Contains(t, spaces.ReplaceAllString(err.Error(), " "), "does not own the account settings")
}

// TestDataLakeSettingsExplicit checks FR-003: every field is declared with its secure default.
func TestDataLakeSettingsExplicit(t *testing.T) {
	t.Parallel()
	plan := planExample(t, "defaults", map[string]interface{}{"name_prefix": generateTestNamePrefix("dls")})
	settings := after(managedChanges(plan)[settingsPrefix+"aws_lakeformation_data_lake_settings.this"])
	require.NotEmpty(t, settings)

	assert.Equal(t, false, settings["allow_external_data_filtering"])
	assert.Equal(t, false, settings["allow_full_table_external_data_access"])
	for _, k := range []string{"external_data_filtering_allow_list", "authorized_session_tag_value_list", "trusted_resource_owners", "create_database_default_permissions", "create_table_default_permissions"} {
		assert.Empty(t, list(settings[k]), k)
	}
	params, _ := settings["parameters"].(map[string]interface{})
	assert.Equal(t, map[string]interface{}{"CROSS_ACCOUNT_VERSION": "4", "SET_CONTEXT": "TRUE"}, params)
}

// TestCatalogEncryptionModes checks FR-010 and FR-011 in both key-access modes.
func TestCatalogEncryptionModes(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"service_role", "per_caller"} {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			vars := completeVars(generateTestNamePrefix("enc"))
			vars["account_settings_config"] = map[string]interface{}{"catalog_encryption": map[string]interface{}{"key_access_mode": mode}}
			plan := planExample(t, "complete", vars)
			changes := managedChanges(plan)
			stmts := statements(t, plan, settingsPrefix+"data.aws_iam_policy_document.required_catalog_key_statements")
			_, hasRole := changes[settingsPrefix+"aws_iam_role.catalog_encryption[0]"]
			_, hasSleep := changes[settingsPrefix+"time_sleep.catalog_encryption_role[0]"]
			_, roleStmt := stmts["AllowCatalogEncryptionRole"]
			users, usersStmt := stmts["AllowCatalogKeyUsers"]

			assert.Contains(t, stmts, "AllowGlueCatalogEncryption")
			if mode == "service_role" {
				assert.Equal(t, "SSE-KMS-WITH-SERVICE-ROLE", encryptionAtRest(changes)["catalog_encryption_mode"])
				assert.True(t, hasRole && hasSleep && roleStmt)
				assert.False(t, usersStmt)

				trust := statements(t, plan, settingsPrefix+"data.aws_iam_policy_document.catalog_encryption_trust")["GlueAssume"]
				assert.NotEmpty(t, conditionValues(trust, "aws:SourceAccount"))
				assert.Empty(t, conditionValues(trust, "aws:SourceArn"), "Glue sends no matching source ARN (sandbox result 3)")
			} else {
				assert.Equal(t, "SSE-KMS", encryptionAtRest(changes)["catalog_encryption_mode"])
				assert.False(t, hasRole || hasSleep || roleStmt)
				require.True(t, usersStmt)
				assert.Contains(t, strs(block(users, "principals")["identifiers"]), "arn:aws:iam::123456789012:role/sales-steward", "tenant personas are key users")
				assert.NotEmpty(t, conditionValues(users, "kms:EncryptionContext:glue_catalog_id"), "key use is limited to this catalog")
			}
		})
	}
}

// TestKeyProtectionStatement checks FR-013: deletion protection never exempts the deployer, in
// any environment type, and exempts exactly the break-glass principals.
func TestKeyProtectionStatement(t *testing.T) {
	t.Parallel()
	breakGlass := "arn:aws:iam::123456789012:role/catalog-key-break-glass"
	for _, c := range []struct {
		name       string
		env        string
		principals []string
	}{
		{"production", "Production", nil},
		{"ephemeral", "Ephemeral", nil},
		{"break_glass", "Production", []string{breakGlass}},
	} {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			vars := completeVars(generateTestNamePrefix("kp"))
			vars["environment_type"] = c.env
			if c.principals != nil {
				vars["account_settings_config"] = map[string]interface{}{"catalog_encryption": map[string]interface{}{"key_deletion_principal_arns": c.principals}}
			}
			plan := planExample(t, "complete", vars)
			protect := statements(t, plan, settingsPrefix+"data.aws_iam_policy_document.required_catalog_key_statements")["ProtectCatalogKey"]
			require.NotNil(t, protect)
			assert.Equal(t, "Deny", protect["effect"])
			assert.Equal(t, []string{"kms:DisableKey", "kms:ScheduleKeyDeletion"}, strs(protect["actions"]))
			if c.principals == nil {
				assert.Empty(t, list(protect["condition"]), "no exceptions: the deny is absolute")
			} else {
				assert.Equal(t, c.principals, conditionValues(protect, "aws:PrincipalArn"))
			}
		})
	}
}

func TestEphemeralOwnerWarnsOnModuleCreatedCatalogKey(t *testing.T) {
	t.Parallel()
	supplied := existingCatalogKey(t)
	vars := completeVars(generateTestNamePrefix("eph"))
	vars["environment_type"] = "Ephemeral"
	out, err := terraform.InitAndPlanE(t, exampleOptions(t, "complete", vars))
	require.NoError(t, err)
	assert.Contains(t, out, "ephemeral_catalog_key")

	vars = completeVars(generateTestNamePrefix("eph"))
	vars["environment_type"] = "Ephemeral"
	vars["account_settings_config"] = map[string]interface{}{"catalog_encryption": map[string]interface{}{"kms_key_arn": supplied}}
	out, err = terraform.InitAndPlanE(t, exampleOptions(t, "complete", vars))
	require.NoError(t, err)
	assert.NotContains(t, out, "ephemeral_catalog_key", "a supplied long-lived key needs no warning")
}

// TestNoCatalogResourcePolicyByDefault checks FR-006.
func TestNoCatalogResourcePolicyByDefault(t *testing.T) {
	t.Parallel()
	changes := managedChanges(planExample(t, "complete", completeVars(generateTestNamePrefix("rp"))))
	assert.NotContains(t, changes, settingsPrefix+"aws_glue_resource_policy.this[0]")

	vars := completeVars(generateTestNamePrefix("rp"))
	deny, err := json.Marshal(map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{{
			"Sid": "DenyOutsideOrg", "Effect": "Deny", "Principal": "*", "Action": "glue:*", "Resource": "*",
			"Condition": map[string]interface{}{"StringNotEquals": map[string]interface{}{"aws:PrincipalOrgID": "o-example"}},
		}},
	})
	require.NoError(t, err)
	jsonVars := map[string]interface{}{"account_settings_config": map[string]interface{}{"catalog_resource_policy": map[string]interface{}{
		"enabled": true, "deny_policy_jsons": []string{string(deny)},
	}}}
	plan := terraform.InitAndPlanAndShowWithStruct(t, exampleOptionsJSONVars(t, "complete", vars, jsonVars))
	policy := after(managedChanges(plan)[settingsPrefix+"aws_glue_resource_policy.this[0]"])
	assert.Equal(t, "TRUE", policy["enable_hybrid"])
	doc, _ := policy["policy"].(string)
	assert.Contains(t, doc, `"RamShareResource"`)
	assert.Contains(t, doc, `"ram.amazonaws.com"`)
	assert.Contains(t, doc, `"DenyOutsideOrg"`)
}

// TestCatalogKeyDistinctFromPlatformKey checks FR-009 and FR-026: a dedicated catalog key, and
// the platform key policy gains no catalog statement.
func TestCatalogKeyDistinctFromPlatformKey(t *testing.T) {
	t.Parallel()
	plan := planExample(t, "complete", completeVars(generateTestNamePrefix("ck")))
	changes := managedChanges(plan)
	assert.Contains(t, changes, "module.main.aws_kms_key.main[0]")
	assert.Contains(t, changes, settingsPrefix+"aws_kms_key.catalog[0]")
	assert.Contains(t, changes, settingsPrefix+"aws_kms_alias.catalog[0]")

	platform, err := json.Marshal(dataValues(plan, "module.main.data.aws_iam_policy_document.kms_key_policy[0]"))
	require.NoError(t, err)
	assert.NotContains(t, string(platform), "glue_catalog_id")
	assert.NotContains(t, string(platform), "ProtectCatalogKey")

	// A supplied catalog key equal to the platform key is rejected
	vars := completeVars(generateTestNamePrefix("ck"))
	platformKey := existingCatalogKey(t)
	vars["encryption_config"] = map[string]interface{}{"create_kms_key": false, "kms_key_id": platformKey, "kms_key_deletion_window_days": 30}
	vars["account_settings_config"] = map[string]interface{}{"catalog_encryption": map[string]interface{}{"kms_key_arn": platformKey}}
	_, err = terraform.InitAndPlanE(t, exampleOptions(t, "complete", vars))
	require.Error(t, err)
	assert.True(t, strings.Contains(spaces.ReplaceAllString(err.Error(), " "), "is a table-storage or query-results key"), err.Error())
}
