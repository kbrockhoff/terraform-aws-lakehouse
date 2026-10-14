package test

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/terraform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const shared = "module.main."

var tenantOwners = map[string]string{
	"sales":   "sales-data@example.com",
	"finance": "finance-data@example.com",
	"claims":  "claims-data@example.com",
}

func completeVars(name string) map[string]interface{} {
	return map[string]interface{}{"name_prefix": name}
}

func TestTerraformCompleteExample(t *testing.T) {
	t.Parallel()
	name := generateTestNamePrefix("comp")
	plan := planExample(t, "complete", completeVars(name))
	changes := managedChanges(plan)
	tenantPrefix := func(tn string) string { return shared + `module.tenant["` + tn + `"].` }

	// Grants: tag-based everywhere except data cells filter grants
	readers := map[string]map[string][]string{}
	for addr, rc := range changes {
		if rc.Type != "aws_lakeformation_permissions" {
			continue
		}
		a := after(rc)
		switch {
		case block(a, "data_cells_filter") != nil:
		case block(a, "lf_tag") != nil:
		default:
			assert.NotNil(t, block(a, "lf_tag_policy"), "%s must be an lf_tag_policy grant", addr)
		}
		if strings.Contains(addr, ".reader_table[") {
			readers[addr] = lfExpression(rc)
		}
	}

	key := func(k string) string { return name + "-" + k }
	regKeys := []string{key("reg-gdpr"), key("reg-hipaa"), key("reg-pci_dss")}
	require.NotEmpty(t, readers)
	for addr, expr := range readers {
		tn := tenantOf(addr)
		assert.Contains(t, expr, key("tenant-"+tn), addr)
		assert.Contains(t, expr, key("sensitivity"), addr)
		for _, r := range regKeys {
			assert.Contains(t, expr, r, addr)
		}
	}
	semantic := readers[tenantPrefix("sales")+`aws_lakeformation_permissions.reader_table["semantic/consumption"]`]
	for _, r := range regKeys {
		assert.Equal(t, []string{"no"}, semantic[r], "semantic layer must see no regulated columns")
	}
	gdpr := readers[tenantPrefix("sales")+`aws_lakeformation_permissions.reader_table["gdpranalyst/raw"]`]
	assert.Equal(t, []string{"no", "yes"}, gdpr[key("reg-gdpr")])
	for addr, expr := range readers {
		_, hasDomain := expr[key("domain")]
		assert.Equal(t, strings.Contains(addr, `tenant["finance"]`) && strings.Contains(addr, `controller/`), hasDomain,
			"only the domain-limited finance reader filters by domain: %s", addr)
	}

	// claims database labels
	claimsDB := after(changes[tenantPrefix("claims")+`aws_lakeformation_resource_lf_tags.database["raw"]`])
	labels := map[string]string{}
	for _, l := range list(claimsDB["lf_tag"]) {
		lm := l.(map[string]interface{})
		labels[lm["key"].(string)] = lm["value"].(string)
	}
	assert.Equal(t, "yes", labels[key("reg-hipaa")])
	assert.Equal(t, "yes", labels[key("reg-gdpr")])
	assert.Equal(t, "restricted", labels[key("sensitivity")])
	assert.Equal(t, "health", labels[key("domain")])

	// sales labeling scope L(T)
	for _, persona := range []string{"steward_label", "writer_label"} {
		scope := map[string][]string{}
		for addr, rc := range withPrefix(changes, tenantPrefix("sales")+"aws_lakeformation_permissions."+persona+"[") {
			lt := block(after(rc), "lf_tag")
			scope[lt["key"].(string)] = strs(lt["values"])
			_ = addr
		}
		assert.Equal(t, []string{"confidential", "internal", "public"}, scope[key("sensitivity")], persona)
		assert.Equal(t, []string{"no", "yes"}, scope[key("reg-gdpr")], persona)
		assert.Equal(t, []string{"no"}, scope[key("reg-hipaa")], persona)
	}

	// Stewards: grantable, but never ASSOCIATE on the tenant key
	for addr, rc := range changes {
		a := after(rc)
		if strings.Contains(addr, ".steward_database[") || strings.Contains(addr, ".steward_table[") {
			assert.Equal(t, strs(a["permissions"]), strs(a["permissions_with_grant_option"]), addr)
		}
		if strings.Contains(addr, ".steward_tenant_tag[") {
			assert.NotContains(t, strs(a["permissions"]), "ASSOCIATE", addr)
		}
		// Production: writers cannot DROP
		if strings.Contains(addr, ".writer_table[") {
			assert.NotContains(t, strs(a["permissions"]), "DROP", addr)
		}
	}

	// The semantic layer is granted only in consumption catalogs
	semanticARN := "arn:aws:iam::123456789012:role/semantic-layer"
	for addr, rc := range changes {
		a := after(rc)
		if rc.Type == "aws_lakeformation_permissions" && a["principal"] == semanticARN {
			if p := block(a, "lf_tag_policy"); p != nil {
				assert.True(t, strings.HasSuffix(p["catalog_id"].(string), "-consumption"), addr)
			}
		}
	}

	// Workgroup result locations
	for _, tn := range []string{"sales", "finance"} {
		wg := after(changes[tenantPrefix(tn)+"aws_athena_workgroup.this"])
		loc := block(block(wg, "configuration"), "result_configuration")["output_location"].(string)
		assert.True(t, strings.HasSuffix(loc, "/"+tn+"/"), loc)
	}
	claimsWG := after(changes[tenantPrefix("claims")+"aws_athena_workgroup.this"])
	claimsLoc := block(block(claimsWG, "configuration"), "result_configuration")["output_location"].(string)
	assert.Equal(t, "s3://"+name+"-claims-athena-results-", claimsLoc[:len("s3://"+name+"-claims-athena-results-")])

	// Production defaults published by the platform (the provider cannot set the bucket default
	// storage class yet; plan.md, Complexity Tracking)
	contract, _ := outputAfter(plan, "platform_contract").(map[string]interface{})
	effective, _ := contract["effective"].(map[string]interface{})
	assert.Equal(t, "INTELLIGENT_TIERING", effective["table_storage_class"])

	// Both alarms per tenant
	for tn := range tenantOwners {
		assert.Contains(t, changes, tenantPrefix(tn)+"aws_cloudwatch_metric_alarm.query_failures[0]")
		assert.Contains(t, changes, tenantPrefix(tn)+"aws_cloudwatch_metric_alarm.scanned_bytes[0]")
	}

	// Tags (FR-022)
	for addr, rc := range changes {
		tg, ok := tags(rc)
		if !ok {
			continue
		}
		assert.Equal(t, "data-platform", tg["CostCenter"], addr)
		assert.NotEmpty(t, tg["DataOwner"], addr)
		if tn := tenantOf(addr); tn != "" {
			assert.Equal(t, tn, tg["Tenant"], addr)
			assert.Equal(t, tenantOwners[tn], tg["DataOwner"], addr)
		}
	}
	for addr, rc := range withPrefix(changes, tenantPrefix("claims")+"aws_s3tables_table_bucket.dedicated[") {
		tg, _ := tags(rc)
		assert.Equal(t, "restricted", tg["DataClassification"], addr)
		assert.Equal(t, "HIPAA", tg["ComplianceFrameworks"], addr)
	}
	for addr, rc := range changes {
		if strings.HasPrefix(addr, shared+"aws_s3tables_table_bucket.zone[") || addr == shared+"aws_s3_bucket.query_results[0]" {
			tg, _ := tags(rc)
			assert.Equal(t, "restricted", tg["DataClassification"], addr)
		}
	}

	// Production budget: platform 10, three tenants at 0.50, plus 2 for the designated key
	assert.LessOrEqual(t, number(outputAfter(plan, "monthly_cost_estimate")), 10+3*0.50+2)
}

func TestCompleteExampleSecurityControls(t *testing.T) {
	t.Parallel()
	plan := planExample(t, "complete", completeVars(generateTestNamePrefix("sec")))
	changes := managedChanges(plan)

	for addr, rc := range changes {
		a := after(rc)
		switch rc.Type {
		case "aws_s3tables_table_bucket":
			enc, _ := a["encryption_configuration"].(map[string]interface{})
			assert.Equal(t, "aws:kms", enc["sse_algorithm"], addr)
			// The key ARN is unknown at plan when the key is created in the same run
			unknown, _ := rc.Change.AfterUnknown.(map[string]interface{})
			known := enc["kms_key_arn"] != nil && enc["kms_key_arn"] != ""
			assert.True(t, known || unknown["encryption_configuration"] != nil, "%s needs a KMS key", addr)
		case "aws_s3_bucket":
			prefix := strings.TrimSuffix(addr, "aws_s3_bucket."+strings.SplitN(addr, "aws_s3_bucket.", 2)[1])
			index := strings.SplitN(addr, "aws_s3_bucket.", 2)[1]
			pab := after(changes[prefix+"aws_s3_bucket_public_access_block."+index])
			for _, k := range []string{"block_public_acls", "block_public_policy", "ignore_public_acls", "restrict_public_buckets"} {
				assert.Equal(t, true, pab[k], "%s %s", addr, k)
			}
			sse := after(changes[prefix+"aws_s3_bucket_server_side_encryption_configuration."+index])
			rule := block(sse, "rule")
			assert.Equal(t, "aws:kms", block(rule, "apply_server_side_encryption_by_default")["sse_algorithm"], addr)
		}
	}

	// Policy documents are rendered at apply (they reference new ARNs), but their statements are
	// known in the deferred data source reads.
	docs := dataChanges(plan)
	bucketPolicyDocs := 0
	for addr, rc := range docs {
		if rc.Type != "aws_iam_policy_document" {
			continue
		}
		statements := list(after(rc)["statement"])
		isBucketPolicy := regexp.MustCompile(`\.(zone_bucket|dedicated_bucket|query_results|results)\b`).MatchString(addr)
		hasTLSDeny := false
		for _, s := range statements {
			sm := s.(map[string]interface{})
			for _, c := range list(sm["condition"]) {
				cm := c.(map[string]interface{})
				if sm["effect"] == "Deny" && cm["variable"] == "aws:SecureTransport" && strs(cm["values"])[0] == "false" {
					hasTLSDeny = true
				}
			}
			if sm["effect"] == "Allow" || sm["effect"] == nil {
				for _, p := range list(sm["principals"]) {
					pm := p.(map[string]interface{})
					assert.NotContains(t, strs(pm["identifiers"]), "*", "%s allows a wildcard principal", addr)
				}
			}
		}
		if isBucketPolicy {
			bucketPolicyDocs++
			assert.True(t, hasTLSDeny, "%s must deny insecure transport", addr)
		}
	}
	assert.GreaterOrEqual(t, bucketPolicyDocs, 4, "zone, dedicated, shared results, and tenant results policies")

	// Every Lake Formation principal comes from the example's inputs or the caller (admin)
	tfvars, err := os.ReadFile("../examples/complete/tenants.auto.tfvars")
	require.NoError(t, err)
	allowed := map[string]bool{}
	for _, arn := range regexp.MustCompile(`arn:aws:iam::[0-9]{12}:role/[a-z0-9-]+`).FindAllString(string(tfvars), -1) {
		allowed[arn] = true
	}
	for addr, rc := range changes {
		if rc.Type == "aws_lakeformation_permissions" {
			principal, _ := after(rc)["principal"].(string)
			assert.True(t, allowed[principal], "%s grants to %s, which is not an input principal", addr, principal)
		}
	}
}

func TestEnabledFalse(t *testing.T) {
	t.Parallel()
	vars := completeVars(generateTestNamePrefix("comp"))
	vars["enabled"] = false
	plan := planExample(t, "complete", vars)
	for addr, rc := range managedChanges(plan) {
		assert.False(t, rc.Change.Actions.Create(), "%s must not be created when enabled = false", addr)
	}
}

// TestComplianceDesignationGatesTenantKeys checks that a designation adds exactly the claims
// tenant's key, dedicated buckets, and results bucket, and nothing outside the claims tenant.
func TestComplianceDesignationGatesTenantKeys(t *testing.T) {
	t.Parallel()
	name := generateTestNamePrefix("cds")
	claims := shared + `module.tenant["claims"].`
	zones := []string{"raw", "curated", "consumption"}

	type tc struct {
		frameworks []string
		dedicated  bool
	}
	cases := map[string]tc{
		"none":      {frameworks: []string{}},
		"soc2":      {frameworks: []string{"SOC2"}},
		"hipaa":     {frameworks: []string{"HIPAA"}},
		"pci":       {frameworks: []string{"PCI_DSS"}},
		"dedicated": {frameworks: []string{}, dedicated: true},
	}

	plans := map[string]*terraform.PlanStruct{}
	for label, c := range cases {
		vars := completeVars(name)
		vars["claims_compliance_frameworks"] = c.frameworks
		overrides := map[string]interface{}{"dedicated_storage": c.dedicated}
		if label != "hipaa" {
			// Without hipaa the claims reader may not keep hipaa either
			overrides["regulations"] = []string{"gdpr"}
			overrides["readers"] = map[string]interface{}{
				"adjuster": map[string]interface{}{
					"principal_arn":      "arn:aws:iam::123456789012:role/claims-adjuster",
					"sensitivity_values": []string{"public", "internal", "confidential", "restricted"},
					"regulations":        []string{"gdpr"},
				},
			}
		}
		vars["tenant_overrides"] = map[string]interface{}{"claims": overrides}
		plans[label] = planExample(t, "complete", vars)
	}

	outsideClaims := func(p *terraform.PlanStruct) map[string]string {
		out := map[string]string{}
		for addr, rc := range managedChanges(p) {
			if !strings.HasPrefix(addr, claims) {
				b, _ := json.Marshal(rc.Change.After)
				out[addr] = string(b)
			}
		}
		return out
	}
	baseline := outsideClaims(plans["none"])

	for label, c := range cases {
		changes := managedChanges(plans[label])
		designated := len(c.frameworks) > 0

		assert.Equal(t, designated, changes[claims+"aws_kms_key.this[0]"] != nil, "%s: tenant key", label)
		assert.Equal(t, designated, changes[claims+"aws_s3_bucket.results[0]"] != nil, "%s: results bucket", label)
		for _, z := range zones {
			bucket := changes[claims+`aws_s3tables_table_bucket.dedicated["`+z+`"]`]
			assert.Equal(t, designated || c.dedicated, bucket != nil, "%s: dedicated %s bucket", label, z)
		}
		tenants, _ := outputAfter(plans[label], "tenants").(map[string]interface{})
		claimsOut, _ := tenants["claims"].(map[string]interface{})
		catalogs, _ := claimsOut["catalog_ids"].(map[string]interface{})
		for _, z := range zones {
			// Catalog IDs are built from bucket names: the shared zone bucket or the dedicated one
			want := name + "-" + z
			if designated || c.dedicated {
				want += "-claims"
			}
			catalog, _ := catalogs[z].(string)
			assert.True(t, strings.HasSuffix(catalog, ":s3tablescatalog/"+want), "%s: %s catalog %q", label, z, catalog)
		}
		if c.dedicated && !designated {
			for _, z := range zones {
				enc, _ := after(changes[claims+`aws_s3tables_table_bucket.dedicated["`+z+`"]`])["encryption_configuration"].(map[string]interface{})
				assert.Equal(t, "aws:kms", enc["sse_algorithm"], "dedicated buckets use the platform key")
			}
		}

		assert.Equal(t, baseline, outsideClaims(plans[label]), "%s: nothing outside claims may change", label)

		compliance := number(claimsOut["compliance_monthly_cost"])
		if designated {
			assert.Greater(t, compliance, 0.0, label)
		} else {
			assert.Equal(t, 0.0, compliance, label)
		}
	}
}
