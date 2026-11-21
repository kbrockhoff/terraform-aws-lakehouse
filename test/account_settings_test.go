package test

import (
	"encoding/json"
	"os"
	"regexp"
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
			_, hasCatalog := changes[catalogStep]
			assert.Equal(t, c.owner, hasSettings, "account settings follow the owner flag")
			assert.Equal(t, c.owner, hasCatalog, "the s3tablescatalog check-and-create follows the owner flag (FR-029)")
			assert.Equal(t, c.integration, hasIntegration, "registration follows the integration flag")
			assert.Equal(t, !c.owner, hasGuard, "non-owners run the settings guard")
		})
	}
}

// catalogStep is the root module's s3tablescatalog check-and-create step (FR-029).
const catalogStep = "module.main.terraform_data.s3tables_catalog[0]"

// catalogInput returns the planned s3tablescatalog check-and-create input, with its catalog input
// JSON decoded.
func catalogInput(t *testing.T, plan *terraform.PlanStruct) (map[string]interface{}, map[string]interface{}) {
	t.Helper()
	rc := managedChanges(plan)[catalogStep]
	require.NotNil(t, rc, "the owner plans the s3tablescatalog check-and-create")
	in, ok := after(rc)["input"].(map[string]interface{})
	require.True(t, ok, "input is known at plan time")
	var catalog map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(in["catalog_input"].(string)), &catalog))
	return in, catalog
}

// TestS3TablesCatalogCheckAndCreate checks FR-029 to FR-031: the owner plans the check-and-create
// step with the catalog definition, and no example manages aws_glue_catalog any more.
func TestS3TablesCatalogCheckAndCreate(t *testing.T) {
	t.Parallel()
	name := generateTestNamePrefix("s3c")
	vars := completeVars(name)
	vars["tags"] = map[string]interface{}{"Environment": "Production"}
	vars["data_tags"] = map[string]interface{}{"DataAtRest": "true"}
	plan := planExample(t, "complete", vars)
	account, _ := dataValues(plan, "data.aws_caller_identity.current")["account_id"].(string)
	require.Regexp(t, `^[0-9]{12}$`, account)

	in, catalog := catalogInput(t, plan)
	assert.Equal(t, "s3tablescatalog", in["name"])
	assert.Equal(t, account, in["account_id"])
	assert.Equal(t, map[string]interface{}{
		"Identifier":     "arn:aws:s3tables:us-east-1:" + account + ":bucket/*",
		"ConnectionName": "aws:s3tables",
	}, catalog["FederatedCatalog"])
	assert.Empty(t, catalog["CreateDatabaseDefaultPermissions"], "no identity-policy-only access")
	assert.Empty(t, catalog["CreateTableDefaultPermissions"], "no identity-policy-only access")
	assert.Equal(t, "True", catalog["AllowFullTableExternalDataAccess"], "default true, as a string (sandbox result 8)")
	tg := in["tags"].(map[string]interface{})
	assert.Equal(t, "Production", tg["Environment"], "common tags reach the catalog")
	assert.NotContains(t, tg, "DataAtRest", "common tags only: the catalog holds no data at rest")

	for addr, rc := range managedChanges(plan) {
		assert.NotEqual(t, "aws_glue_catalog", rc.Type, "%s: the catalog is never a managed resource (FR-031)", addr)
	}

	vars = completeVars(generateTestNamePrefix("s3f"))
	vars["account_settings_config"] = map[string]interface{}{
		"s3tables_catalog": map[string]interface{}{"allow_full_table_external_data_access": false},
	}
	_, catalog = catalogInput(t, planExample(t, "complete", vars))
	assert.Equal(t, "False", catalog["AllowFullTableExternalDataAccess"])
}

// TestS3TablesCatalogCreatedFirst checks the FR-029 ordering: every resource the owner creates or
// changes depends, directly or transitively, on the s3tablescatalog step, so the catalog exists
// first. Only data sources and the plan-time guards (terraform_data with preconditions, which
// create nothing) may run before it.
func TestS3TablesCatalogCreatedFirst(t *testing.T) {
	t.Parallel()
	opts := exampleOptions(t, "complete", completeVars(generateTestNamePrefix("ord")))
	terraform.InitAndPlan(t, opts)
	graph := terraform.RunTerraformCommandAndGetStdout(t, opts, "graph", "-plan="+opts.PlanFilePath)

	// Node names can hold escaped quotes (tenant[\"sales\"]). The apply graph orders resource
	// instances; its "(expand)" nodes don't carry the instance edges, so instances are checked.
	quoted := `"((?:[^"\\]|\\.)+)"`
	edge := regexp.MustCompile(`^\s*` + quoted + ` -> ` + quoted)
	node := regexp.MustCompile(`^\s*` + quoted + ` \[label`)
	deps := map[string][]string{}
	var nodes []string
	for _, line := range strings.Split(graph, "\n") {
		if m := edge.FindStringSubmatch(line); m != nil {
			deps[m[1]] = append(deps[m[1]], m[2])
		} else if m := node.FindStringSubmatch(line); m != nil {
			nodes = append(nodes, m[1])
		}
	}
	const step = `[root] module.main.terraform_data.s3tables_catalog[0]`
	require.Contains(t, nodes, step)
	reaches := func(from string) bool {
		seen := map[string]bool{}
		stack := []string{from}
		for len(stack) > 0 {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if n == step {
				return true
			}
			for _, d := range deps[n] {
				if !seen[d] {
					seen[d] = true
					stack = append(stack, d)
				}
			}
		}
		return false
	}
	instance := regexp.MustCompile(`\.(aws_[a-z0-9_]+|time_sleep|terraform_data)\.([a-z0-9_]+)(\[[^\]]*\])?$`)
	checked := 0
	for _, n := range nodes {
		m := instance.FindStringSubmatch(n)
		if m == nil || n == step || strings.Contains(n, ".data.") || (m[1] == "terraform_data" && strings.HasSuffix(m[2], "_guard")) {
			continue
		}
		checked++
		assert.True(t, reaches(n), "%s can be created before s3tablescatalog", n)
	}
	assert.Greater(t, checked, 100, "the graph covers the complete example's resource instances")
}

func TestOwnerOnlyInputsRejectedOnNonOwner(t *testing.T) {
	t.Parallel()
	vars := completeVars(generateTestNamePrefix("ono"))
	vars["account_settings_config"] = map[string]interface{}{"owner": false, "cross_account_version": 5}
	_, err := terraform.InitAndPlanE(t, exampleOptions(t, "complete", vars))
	require.Error(t, err)
	assert.Contains(t, spaces.ReplaceAllString(err.Error(), " "), "does not own the account settings")
}

// identityCenterInstance is a well-formed placeholder; plans never call Identity Center.
const identityCenterInstance = "arn:aws:sso:::instance/ssoins-0123456789abcdef"

// TestIdentityCenterIntegration checks FR-032: off by default, created by the owner for the
// account's catalog when enabled, and owner-only.
func TestIdentityCenterIntegration(t *testing.T) {
	t.Parallel()
	const addr = settingsPrefix + "aws_lakeformation_identity_center_configuration.this[0]"

	off := planExample(t, "complete", completeVars(generateTestNamePrefix("icf")))
	assert.NotContains(t, managedChanges(off), addr, "off by default")
	assert.Equal(t, "", outputAfter(off, "identity_center_application_arn"))

	vars := completeVars(generateTestNamePrefix("ico"))
	vars["account_settings_config"] = map[string]interface{}{
		"identity_center": map[string]interface{}{"enabled": true, "instance_arn": identityCenterInstance},
	}
	on := planExample(t, "complete", vars)
	ic := after(managedChanges(on)[addr])
	require.NotEmpty(t, ic, "the owner creates the integration")
	assert.Equal(t, identityCenterInstance, ic["instance_arn"])
	assert.Regexp(t, `^[0-9]{12}$`, ic["catalog_id"], "the account's default catalog")

	vars = completeVars(generateTestNamePrefix("icn"))
	vars["account_settings_config"] = map[string]interface{}{
		"owner":           false,
		"identity_center": map[string]interface{}{"enabled": true, "instance_arn": identityCenterInstance},
	}
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
			keyPolicies(t, plan) // the catalog key policy merges the administration document
			breakGlassStmt := statements(t, plan, settingsPrefix+"module.key_policy_statements.data.aws_iam_policy_document.administration")["KeyDeletionBreakGlass"]
			if c.principals == nil {
				assert.Empty(t, list(protect["condition"]), "no exceptions: the deny is absolute")
				assert.Nil(t, breakGlassStmt, "no break-glass statement without break-glass principals")
			} else {
				assert.Equal(t, c.principals, conditionValues(protect, "aws:PrincipalArn"))
				// The root no longer grants kms:* through IAM, so the exempted principals need their own
				// allow, matched by exact ARN so a retired break-glass role never invalidates the policy
				require.NotNil(t, breakGlassStmt)
				assert.Equal(t, c.principals, conditionValues(breakGlassStmt, "aws:PrincipalArn"))
				assert.Regexp(t, `^arn:aws:iam::[0-9]{12}:root$`, strs(block(breakGlassStmt, "principals")["identifiers"])[0])
				assert.Subset(t, strs(breakGlassStmt["actions"]), []string{"kms:ScheduleKeyDeletion", "kms:DisableKey"})
			}
		})
	}
}

// keyPolicies maps each module-created key policy document (platform, catalog metadata, and
// designated-tenant keys) to the shared administration document merged into it. The key
// policies reference roles created in the same plan, so their merged JSON is unknown until
// apply; the administration documents are known, and the tests read their statements.
func keyPolicies(t *testing.T, plan *terraform.PlanStruct) map[string]string {
	t.Helper()
	out := map[string]string{}
	admin := regexp.MustCompile(`^(.*)module\.key_policy_statements(\[0\])?\.data\.aws_iam_policy_document\.administration$`)
	for _, addr := range planDataSources(plan) {
		m := admin.FindStringSubmatch(addr)
		if m == nil {
			continue
		}
		var policy string
		for _, name := range []string{"kms_key_policy[0]", "kms[0]", "catalog_key_policy"} {
			if dataValues(plan, m[1]+"data.aws_iam_policy_document."+name) != nil {
				policy = m[1] + "data.aws_iam_policy_document." + name
			}
		}
		if policy == "" {
			continue // a platform with create_kms_key = false renders the statements without a key
		}
		// The key policy merges this document's JSON
		adminJSON, _ := dataValues(plan, addr)["json"].(string)
		require.NotEmpty(t, adminJSON, addr)
		assert.Contains(t, strs(dataValues(plan, policy)["source_policy_documents"]), adminJSON, "%s merges %s", policy, addr)
		out[policy] = addr
	}
	return out
}

func keyAdministrators(t *testing.T, plan *terraform.PlanStruct, adminAddr string) []string {
	t.Helper()
	admins := statements(t, plan, adminAddr)["KeyAdministrators"]
	require.NotNil(t, admins, adminAddr)
	return strs(block(admins, "principals")["identifiers"])
}

func keyAdminVars(prefix string, admins []string) map[string]interface{} {
	vars := completeVars(generateTestNamePrefix(prefix))
	vars["encryption_config"] = map[string]interface{}{
		"create_kms_key":               true,
		"kms_key_id":                   "",
		"kms_key_deletion_window_days": 30,
		"key_administrator_arns":       admins,
	}
	return vars
}

// TestKeyAdministratorStatements checks every module-created key policy: only the explicit
// key_administrator_arns manage the key, without using it, and the account root may only read it.
// The list is never derived from the session, and the plan fails unless the deployer is listed.
func TestKeyAdministratorStatements(t *testing.T) {
	t.Parallel()

	// The complete example defaults the list to the deploying role
	base := planExample(t, "complete", completeVars(generateTestNamePrefix("kab")))
	var deployer []string
	for _, adminAddr := range keyPolicies(t, base) {
		deployer = keyAdministrators(t, base, adminAddr)
		break
	}
	require.Len(t, deployer, 1, "exactly the listed role, nothing added")
	account := strings.Split(deployer[0], ":")[4]

	extra := "arn:aws:iam::" + account + ":role/key-admin"
	plan := planExample(t, "complete", keyAdminVars("ka", []string{extra, deployer[0]}))
	policies := keyPolicies(t, plan)
	assert.GreaterOrEqual(t, len(policies), 3, "platform, catalog, and designated tenant key policies")
	for policy, adminAddr := range policies {
		assert.NotContains(t, statements(t, plan, policy), "EnableRootAccess", policy)

		stmts := statements(t, plan, adminAddr)
		assert.ElementsMatch(t, []string{deployer[0], extra}, keyAdministrators(t, plan, adminAddr), "%s: exactly the configured administrators", policy)
		admins := stmts["KeyAdministrators"]
		for _, a := range strs(admins["actions"]) {
			assert.NotContains(t, []string{"kms:*", "kms:Decrypt", "kms:Encrypt", "kms:GenerateDataKey", "kms:CreateGrant", "kms:ReEncryptFrom", "kms:ReEncryptTo"}, a, "%s: administrators cannot use the key", policy)
		}
		assert.Contains(t, strs(admins["actions"]), "kms:PutKeyPolicy", policy)
		assert.Equal(t, []string{account}, conditionValues(admins, "aws:PrincipalAccount"), "%s: administrators are limited to this account", policy)

		root := stmts["AccountReadOnly"]
		require.NotNil(t, root, policy)
		assert.Equal(t, []string{"kms:Describe*", "kms:Get*", "kms:List*"}, strs(root["actions"]), policy)
	}

	for _, c := range []struct {
		name, want string
		admins     []string
	}{
		{"deployer not listed", "is not in encryption_config.key_administrator_arns", []string{extra}},
		{"cross-account administrator", "must be in this account", []string{deployer[0], "arn:aws:iam::999999999999:role/key-admin"}},
	} {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := terraform.InitAndPlanE(t, exampleOptions(t, "complete", keyAdminVars("kar", c.admins)))
			require.Error(t, err)
			assert.Contains(t, spaces.ReplaceAllString(err.Error(), " "), c.want)
		})
	}
}

// TestKeyDelegatedStatements checks the account-delegated key use that remains: each statement is
// matched by exact principal ARN (a retired role never invalidates the policy), and exemptions only
// reach the module's table storage.
func TestKeyDelegatedStatements(t *testing.T) {
	t.Parallel()
	exempt := "arn:aws:iam::123456789012:role/migration"
	writer := "arn:aws:iam::123456789012:role/glue-connections"
	name := generateTestNamePrefix("kd")
	vars := completeVars(name)
	vars["governance_config"] = map[string]interface{}{
		"domain_values":                       []string{"general", "sales", "finance", "health"},
		"direct_access_exempt_principal_arns": []string{exempt},
	}
	vars["account_settings_config"] = map[string]interface{}{
		"catalog_encryption": map[string]interface{}{"connection_writer_arns": []string{writer}},
	}
	plan := planExample(t, "complete", vars)

	exemptions := 0
	for _, addr := range planDataSources(plan) {
		if !strings.HasSuffix(addr, "module.key_policy_statements[0].data.aws_iam_policy_document.direct_access[0]") {
			continue
		}
		stmt := statements(t, plan, addr)["AllowDirectAccessExemptions"]
		require.NotNil(t, stmt, addr)
		assert.Regexp(t, `^arn:aws:iam::[0-9]{12}:root$`, strs(block(stmt, "principals")["identifiers"])[0], addr)
		assert.Equal(t, []string{exempt}, conditionValues(stmt, "aws:PrincipalArn"), addr)
		contexts := conditionValues(stmt, "kms:EncryptionContext:aws:s3:arn")
		require.NotEmpty(t, contexts, "%s: limited to table storage", addr)
		for _, c := range contexts {
			assert.Regexp(t, `^arn:aws:s3tables:[a-z0-9-]+:[0-9]{12}:bucket/`+name+`-[^/]*/\*$`, c, addr)
		}
		exemptions++
	}
	assert.GreaterOrEqual(t, exemptions, 2, "platform and designated-tenant keys")

	writers := statements(t, plan, settingsPrefix+"data.aws_iam_policy_document.required_catalog_key_statements")["AllowConnectionPasswordEncryption"]
	require.NotNil(t, writers, "connection writers keep kms:Encrypt for Glue connection passwords")
	assert.Equal(t, []string{"kms:Encrypt"}, strs(writers["actions"]))
	assert.Equal(t, []string{writer}, conditionValues(writers, "aws:PrincipalArn"))
	assert.Regexp(t, `^glue\.`, conditionValues(writers, "kms:ViaService")[0])
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
