package test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/terraform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// actions returns every action in a rendered policy document.
func actions(t *testing.T, plan *terraform.PlanStruct, output string) map[string]bool {
	t.Helper()
	doc, ok := outputAfter(plan, output).(string)
	require.True(t, ok, "%s must be known at plan time", output)
	var policy struct {
		Statement []struct {
			Action interface{} `json:"Action"`
		} `json:"Statement"`
	}
	require.NoError(t, json.Unmarshal([]byte(doc), &policy))
	out := map[string]bool{}
	for _, s := range policy.Statement {
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
	"aws_glue_catalog":                                   "glue:CreateCatalog",
	"terraform_data":                                     "",
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

	for addr, rc := range managedChanges(complete) {
		action, known := createActions[rc.Type]
		if assert.True(t, known, "no create action mapped for %s (%s)", rc.Type, addr) && action != "" {
			assert.True(t, platform[action], "platform scope lacks %s for %s", action, rc.Type)
		}
	}

	assert.False(t, actions(t, noPricing, "platform_policy_json")["pricing:GetProducts"])
	assert.False(t, actions(t, noPricing, "tenant_policy_json")["pricing:GetProducts"])
}
