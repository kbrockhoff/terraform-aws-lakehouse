package test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/terraform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// actions returns every action a rendered policy document allows.
func actions(t *testing.T, plan *terraform.PlanStruct, output string) map[string]bool {
	t.Helper()
	return statementActions(t, plan, output, "Allow")
}

// statementActions returns every action in the rendered policy's statements with the given effect.
func statementActions(t *testing.T, plan *terraform.PlanStruct, output, effect string) map[string]bool {
	t.Helper()
	doc, ok := outputAfter(plan, output).(string)
	require.True(t, ok, "%s must be known at plan time", output)
	var policy struct {
		Statement []struct {
			Effect string      `json:"Effect"`
			Action interface{} `json:"Action"`
		} `json:"Statement"`
	}
	require.NoError(t, json.Unmarshal([]byte(doc), &policy))
	out := map[string]bool{}
	for _, s := range policy.Statement {
		if s.Effect != effect {
			continue
		}
		switch a := s.Action.(type) {
		case string:
			out[a] = true
		case []interface{}:
			for _, x := range a {
				out[x.(string)] = true
			}
		}
	}
	return out
}

// createActions maps each resource type in the complete example to the action that creates it.
var createActions = map[string]string{
	"aws_s3tables_table_bucket":                          "s3tables:CreateTableBucket",
	"aws_s3tables_table_bucket_policy":                   "s3tables:PutTableBucketPolicy",
	"aws_s3tables_namespace":                             "s3tables:CreateNamespace",
	"aws_lakeformation_lf_tag":                           "lakeformation:CreateLFTag",
	"aws_lakeformation_resource_lf_tags":                 "lakeformation:AddLFTagsToResource",
	"aws_lakeformation_resource_lf_tag":                  "lakeformation:AddLFTagsToResource",
	"aws_lakeformation_permissions":                      "lakeformation:GrantPermissions",
	"aws_lakeformation_data_cells_filter":                "lakeformation:CreateDataCellsFilter",
	"aws_lakeformation_resource":                         "lakeformation:RegisterResource",
	"aws_lakeformation_data_lake_settings":               "lakeformation:PutDataLakeSettings",
	"aws_athena_workgroup":                               "athena:CreateWorkGroup",
	"aws_s3_bucket":                                      "s3:CreateBucket",
	"aws_s3_bucket_policy":                               "s3:PutBucketPolicy",
	"aws_s3_bucket_public_access_block":                  "s3:PutBucketPublicAccessBlock",
	"aws_s3_bucket_ownership_controls":                   "s3:PutBucketOwnershipControls",
	"aws_s3_bucket_server_side_encryption_configuration": "s3:PutEncryptionConfiguration",
	"aws_s3_bucket_lifecycle_configuration":              "s3:PutLifecycleConfiguration",
	"aws_kms_key":                                        "kms:CreateKey",
	"aws_kms_alias":                                      "kms:CreateAlias",
	"aws_cloudwatch_metric_alarm":                        "cloudwatch:PutMetricAlarm",
	"aws_sns_topic":                                      "sns:CreateTopic",
	"aws_iam_role":                                       "iam:CreateRole",
	"aws_iam_role_policy":                                "iam:PutRolePolicy",
	"aws_glue_data_catalog_encryption_settings":          "glue:PutDataCatalogEncryptionSettings",
	"aws_glue_resource_policy":                           "glue:PutResourcePolicy",
	"terraform_data":                                     "",
	"time_sleep":                                         "",
}

func TestDeployerPolicies(t *testing.T) {
	t.Parallel()

	withPricing := planExample(t, "deployer", map[string]interface{}{})
	noPricing := planExample(t, "deployer", map[string]interface{}{"cost_estimation_enabled": false})
	complete := planExample(t, "complete", completeVars(generateTestNamePrefix("dep")))

	platform := actions(t, withPricing, "platform_policy_json")
	tenant := actions(t, withPricing, "tenant_policy_json")

	for _, scope := range []map[string]bool{platform, tenant} {
		for a := range scope {
			assert.NotContains(t, a, "*", "wildcard action %s", a)
		}
		assert.True(t, scope["s3tables:PutTableBucketStorageClass"])
		assert.True(t, scope["s3tables:GetTableBucketStorageClass"])
		assert.True(t, scope["pricing:GetProducts"])
	}

	for _, a := range []string{"glue:CreateCatalog", "lakeformation:PutDataLakeSettings", "lakeformation:RegisterResource"} {
		assert.False(t, tenant[a], "tenant scope must not include %s", a)
	}
	// iam:GetRole is the one IAM action both scopes need: aws_iam_session_context calls it to
	// resolve the deploying role (plan.md, Complexity Tracking).
	for a := range tenant {
		if strings.HasPrefix(a, "iam:") {
			assert.Equal(t, "iam:GetRole", a, "tenant scope must not manage IAM")
		}
	}

	// The shared s3tablescatalog is created only when missing and never changed or deleted (FR-030)
	for _, a := range []string{"glue:DeleteCatalog", "glue:UpdateCatalog"} {
		assert.False(t, platform[a], "platform scope must not allow %s", a)
	}
	assert.True(t, statementActions(t, withPricing, "platform_policy_json", "Deny")["glue:DeleteCatalog"], "platform scope denies deleting s3tablescatalog")
	for _, a := range []string{"glue:CreateCatalog", "glue:GetCatalog", "glue:TagResource", "sts:GetCallerIdentity"} {
		assert.True(t, platform[a], "platform scope needs %s for the s3tablescatalog check-and-create", a)
	}

	for addr, rc := range managedChanges(complete) {
		action, known := createActions[rc.Type]
		if assert.True(t, known, "no create action mapped for %s (%s)", rc.Type, addr) && action != "" {
			assert.True(t, platform[action], "platform scope lacks %s for %s", action, rc.Type)
		}
	}

	assert.False(t, actions(t, noPricing, "platform_policy_json")["pricing:GetProducts"])
	assert.False(t, actions(t, noPricing, "tenant_policy_json")["pricing:GetProducts"])
}

// TestDeployerIdentityCenterPermissions checks FR-032: the Identity Center integration actions
// are granted to the platform deployer only when requested, never to a tenant pipeline.
func TestDeployerIdentityCenterPermissions(t *testing.T) {
	t.Parallel()
	icActions := []string{
		"lakeformation:CreateLakeFormationIdentityCenterConfiguration",
		"lakeformation:DeleteLakeFormationIdentityCenterConfiguration",
		"sso:CreateApplication",
		"sso:DeleteApplication",
		"sso:PutApplicationAuthenticationMethod",
	}

	off := planExample(t, "deployer", map[string]interface{}{})
	const instance = "arn:aws:sso:::instance/ssoins-7223dc852c4ffcf3"
	on := planExample(t, "deployer", map[string]interface{}{"identity_center_instance_arn": instance})
	for _, a := range icActions {
		assert.False(t, actions(t, off, "platform_policy_json")[a], "%s without identity_center_instance_arn", a)
		assert.True(t, actions(t, on, "platform_policy_json")[a], "platform scope needs %s", a)
		assert.False(t, actions(t, on, "tenant_policy_json")[a], "tenant scope must not include %s", a)
	}
	// Scoped to the instance, its applications in any owner account, and the Lake Formation provider
	policy, _ := outputAfter(on, "platform_policy_json").(string)
	assert.Contains(t, policy, `"`+instance+`"`)
	assert.Contains(t, policy, "arn:aws:sso::*:application/ssoins-7223dc852c4ffcf3/*")
	assert.Contains(t, policy, "arn:aws:sso::aws:applicationProvider/lakeformation")
	assert.NotContains(t, policy, "arn:aws:sso:::instance/*")

	// IAM counts managed policy size without whitespace
	for _, name := range []string{"basic", "lakehouse"} {
		policy, _ := after(managedChanges(on)["module.platform.aws_iam_policy."+name+"[0]"])["policy"].(string)
		require.NotEmpty(t, policy, name)
		assert.Less(t, len(strings.Join(strings.Fields(policy), "")), 6144, "%s policy exceeds the managed policy size quota", name)
	}
}
