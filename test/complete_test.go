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
		assert.Contains(t, expr, key("zone"), addr)
		assert.Contains(t, expr, key("sensitivity"), addr)
		for _, r := range regKeys {
			assert.Contains(t, expr, r, addr)
		}
	}
	semantic := readers[tenantPrefix("sales")+`aws_lakeformation_permissions.reader_table["semantic"]`]
	for _, r := range regKeys {
		assert.Equal(t, []string{"no"}, semantic[r], "semantic layer must see no regulated columns")
	}
	gdpr := readers[tenantPrefix("sales")+`aws_lakeformation_permissions.reader_table["gdpranalyst"]`]
	assert.Equal(t, []string{"no", "yes"}, gdpr[key("reg-gdpr")])
	for addr, expr := range readers {
		_, hasDomain := expr[key("domain")]
		assert.Equal(t, strings.Contains(addr, `tenant["finance"]`) && strings.Contains(addr, `["controller"]`), hasDomain,
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

	// Grants are account-level, and the semantic layer's expressions reach only the consumption
	// zone (Lake Formation rejects grants scoped to a bucket catalog; plan.md, Grants)
	semanticARN := "arn:aws:iam::123456789012:role/semantic-layer"
	semanticGrants := 0
	for addr, rc := range changes {
		a := after(rc)
		p := block(a, "lf_tag_policy")
		if rc.Type != "aws_lakeformation_permissions" || p == nil {
			continue
		}
		assert.Regexp(t, `^[0-9]{12}$`, p["catalog_id"], "%s must be an account-level grant", addr)
		assert.Contains(t, lfExpression(rc), key("zone"), "%s must limit zones", addr)
		if a["principal"] == semanticARN {
			semanticGrants++
			assert.Equal(t, []string{"consumption"}, lfExpression(rc)[key("zone")], addr)
		}
	}
	assert.Equal(t, 4, semanticGrants, "database and table grants in sales and finance")

	// Row filters (awslkhs-fysh): the filtered reader's tag grant requires its tenant-owned
	// row-filter key to be "no", the filtered table carries "yes", and databases carry "no", so the
	// data cells filter is that reader's only access to the table. Other readers are unaffected.
	rfKey := key("tenant-sales-rf-gdpranalyst")
	assert.Equal(t, []string{"no"}, gdpr[rfKey], "the filtered reader's grant skips row-filtered tables")
	_, semanticHasRF := semantic[rfKey]
	assert.False(t, semanticHasRF, "readers without filters keep their grants")
	rfTag := changes[tenantPrefix("sales")+`aws_lakeformation_lf_tag.row_filter["gdpranalyst"]`]
	require.NotNil(t, rfTag)
	assert.Equal(t, []string{"no", "yes"}, strs(after(rfTag)["values"]))
	filtered := after(changes[tenantPrefix("sales")+`aws_lakeformation_resource_lf_tags.row_filtered_table["gdpranalyst/curated/orders"]`])
	require.NotNil(t, filtered)
	assert.Equal(t, "orders", block(filtered, "table")["name"])
	assert.Equal(t, "yes", list(filtered["lf_tag"])[0].(map[string]interface{})["value"])
	salesDB := after(changes[tenantPrefix("sales")+`aws_lakeformation_resource_lf_tags.database["curated"]`])
	dbLabels := map[string]string{}
	for _, l := range list(salesDB["lf_tag"]) {
		lm := l.(map[string]interface{})
		dbLabels[lm["key"].(string)] = lm["value"].(string)
	}
	assert.Equal(t, "no", dbLabels[rfKey], "databases default every table to unfiltered")

	// Read-only admins are authoritative like admins, so applies no longer erase them
	settings := after(changes[shared+`module.account_settings[0].aws_lakeformation_data_lake_settings.this`])
	assert.Equal(t, []string{"arn:aws:iam::123456789012:role/lakehouse-auditor"}, strs(settings["read_only_admins"]))

	// Databases carry their zone, which only platform admins can associate
	for _, z := range []string{"raw", "curated", "consumption"} {
		db := after(changes[tenantPrefix("sales")+`aws_lakeformation_resource_lf_tags.database["`+z+`"]`])
		zoneLabel := ""
		for _, l := range list(db["lf_tag"]) {
			lm := l.(map[string]interface{})
			if lm["key"] == key("zone") {
				zoneLabel = lm["value"].(string)
			}
		}
		assert.Equal(t, z, zoneLabel)
	}
	for addr, rc := range withPrefix(changes, tenantPrefix("sales")+"aws_lakeformation_permissions.") {
		if lt := block(after(rc), "lf_tag"); lt != nil && lt["key"] == key("zone") {
			assert.NotContains(t, strs(after(rc)["permissions"]), "ASSOCIATE", addr)
		}
	}

	// Workgroup result locations
	for _, tn := range []string{"sales", "finance"} {
		wg := after(changes[tenantPrefix(tn)+"aws_athena_workgroup.this"])
		loc := block(block(wg, "configuration"), "result_configuration")["output_location"].(string)
		assert.True(t, strings.HasSuffix(loc, "/"+tn+"/"), loc)
	}
	// The DML workgroup is opt-in (awslkhs-vpuy)
	for addr, rc := range withPrefix(changes, shared+"module.tenant[") {
		assert.False(t, strings.HasPrefix(rc.Name, "dml"), "%s: no DML workgroups or alarms by default", addr)
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

		// Failed DDL publishes under its own QueryType, so the failure alarm sums DML and DDL
		var queryTypes []string
		for _, q := range list(after(changes[tenantPrefix(tn)+"aws_cloudwatch_metric_alarm.query_failures[0]"])["metric_query"]) {
			if dims, ok := block(q.(map[string]interface{}), "metric")["dimensions"].(map[string]interface{}); ok {
				assert.Equal(t, "FAILED", dims["QueryState"], tn)
				queryTypes = append(queryTypes, dims["QueryType"].(string))
			}
		}
		assert.ElementsMatch(t, []string{"DML", "DDL"}, queryTypes, tn)
	}

	// Tags: the caller's tags plus only the module metadata tags (awslkhs-bavj)
	allowedTags := map[string]bool{
		"Environment": true, "Example": true,
		"Name": true, "ModuleName": true, "ModuleVersion": true, "ModuleEnvType": true,
	}
	for addr, rc := range changes {
		tg, ok := tags(rc)
		if !ok {
			continue
		}
		assert.Equal(t, "kbrockhoff/lakehouse/aws", tg["ModuleName"], addr)
		assert.NotEmpty(t, tg["ModuleVersion"], addr)
		assert.Equal(t, "Production", tg["ModuleEnvType"], addr)
		for k := range tg {
			assert.True(t, allowedTags[k], "%s: unexpected module-created tag %s", addr, k)
		}
	}

	// Production budget: platform 10, three tenants at 0.50, plus 2 for the designated key
	assert.LessOrEqual(t, number(outputAfter(plan, "monthly_cost_estimate")), 10+3*0.50+2)
}

// TestDataTagsOnlyOnDataAtRest checks that data_tags reach only resources containing
// data-at-rest, never KMS keys.
func TestDataTagsOnlyOnDataAtRest(t *testing.T) {
	t.Parallel()
	vars := completeVars(generateTestNamePrefix("dtag"))
	vars["tags"] = map[string]interface{}{"Environment": "Production"}
	vars["data_tags"] = map[string]interface{}{"DataAtRest": "true"}
	changes := managedChanges(planExample(t, "complete", vars))

	dataAtRest := map[string]bool{
		"aws_s3tables_table_bucket": true,
		"aws_s3_bucket":             true,
	}
	kmsKeys := 0
	for addr, rc := range changes {
		tg, ok := tags(rc)
		if !ok {
			continue
		}
		if rc.Type == "aws_kms_key" {
			kmsKeys++
		}
		if dataAtRest[rc.Type] {
			assert.Equal(t, "true", tg["DataAtRest"], addr)
		} else {
			assert.NotContains(t, tg, "DataAtRest", addr)
		}
	}
	assert.GreaterOrEqual(t, kmsKeys, 3, "platform, catalog metadata, and designated tenant KMS keys")
	assert.Contains(t, changes, "module.main.module.account_settings[0].aws_kms_key.catalog[0]")
	assert.Contains(t, changes, "module.main.module.account_settings[0].aws_iam_role.catalog_encryption[0]")
}

// TestDMLWorkgroupOptIn checks awslkhs-vpuy: with dml_workgroup_enabled, each tenant gets a DML
// workgroup without a result encryption setting (it breaks Athena DML into S3 Tables), the read
// workgroup keeps SSE_KMS, and a tenant can opt out.
// TestMetadataPersonas checks the schema-only personas (awslkhs-lfj): metadata readers get DESCRIBE,
// metadata writers CREATE_TABLE plus ALTER on steward-tagged tables (no DROP in Production), and
// neither gets data permissions.
func TestMetadataPersonas(t *testing.T) {
	t.Parallel()
	name := generateTestNamePrefix("meta")
	plan := planExample(t, "complete", completeVars(name))
	changes := managedChanges(plan)
	sales := shared + `module.tenant["sales"].`
	zoneKey := name + "-zone"

	tenantKey, _ := after(changes[sales+"aws_lakeformation_lf_tag.tenant"])["key"].(string)
	require.NotEmpty(t, tenantKey)
	metaKey := tenantKey + "-meta"
	tag := after(changes[sales+"aws_lakeformation_lf_tag.metadata_writer[0]"])
	assert.Equal(t, metaKey, tag["key"])
	assert.Equal(t, []string{"yes"}, strs(tag["values"]))
	assert.NotContains(t, changes, shared+`module.tenant["finance"].aws_lakeformation_lf_tag.metadata_writer[0]`, "only tenants with metadata writers get the key")

	for _, rt := range []string{"DATABASE", "TABLE"} {
		rc := changes[sales+`aws_lakeformation_permissions.metadata_reader["catalog/`+rt+`"]`]
		require.NotNil(t, rc, rt)
		assert.Equal(t, []string{"DESCRIBE"}, strs(after(rc)["permissions"]), rt)
		assert.Equal(t, []string{"consumption", "curated", "raw"}, lfExpression(rc)[zoneKey], rt)
	}

	db := changes[sales+`aws_lakeformation_permissions.metadata_writer_database["schema"]`]
	require.NotNil(t, db)
	assert.Equal(t, []string{"CREATE_TABLE", "DESCRIBE"}, strs(after(db)["permissions"]))
	assert.Equal(t, []string{"curated"}, lfExpression(db)[zoneKey])

	tbl := changes[sales+`aws_lakeformation_permissions.metadata_writer_table["schema"]`]
	require.NotNil(t, tbl)
	assert.Equal(t, []string{"ALTER", "DESCRIBE"}, strs(after(tbl)["permissions"]), "no DROP in Production")
	assert.Equal(t, []string{"yes"}, lfExpression(tbl)[metaKey], "only tables stewards tag")

	// Stewards may associate the key; writers may not
	assert.Contains(t, changes, sales+`aws_lakeformation_permissions.steward_label["arn:aws:iam::123456789012:role/sales-steward/`+metaKey+`"]`)
	for addr := range withPrefix(changes, sales+"aws_lakeformation_permissions.writer_label[") {
		assert.NotContains(t, addr, metaKey, addr)
	}

	docs := dataChanges(plan)
	sids := func(key string) []string {
		rc := docs[sales+`data.aws_iam_policy_document.persona["`+key+`"]`]
		require.NotNil(t, rc, key)
		var out []string
		for _, s := range list(after(rc)["statement"]) {
			sm := s.(map[string]interface{})
			out = append(out, sm["sid"].(string))
			if sm["sid"] == "MetadataWriterTables" {
				assert.NotContains(t, strs(sm["actions"]), "glue:DeleteTable", "no DROP in Production")
			}
		}
		return out
	}
	reader, writer := sids("metadata_reader/catalog"), sids("metadata_writer/schema")
	assert.NotContains(t, reader, "MetadataWriterTables")
	assert.Contains(t, writer, "MetadataWriterTables")
	for _, s := range [][]string{reader, writer} {
		assert.NotContains(t, s, "Labeling", "metadata personas do not label")
	}
}

func TestDMLWorkgroupOptIn(t *testing.T) {
	t.Parallel()
	name := generateTestNamePrefix("dml")
	vars := completeVars(name)
	vars["query_config"] = map[string]interface{}{"dml_workgroup_enabled": true}
	vars["extra_tenants"] = map[string]interface{}{
		"nodml": map[string]interface{}{"owner": "x@example.com", "sensitivity": "internal", "dml_workgroup_enabled": false},
	}
	plan := planExample(t, "complete", vars)
	changes := managedChanges(plan)
	tenantPrefix := func(tn string) string { return shared + `module.tenant["` + tn + `"].` }

	for tn := range tenantOwners {
		read := block(block(after(changes[tenantPrefix(tn)+"aws_athena_workgroup.this"]), "configuration"), "result_configuration")
		assert.Equal(t, "SSE_KMS", block(read, "encryption_configuration")["encryption_option"], tn)

		require.Contains(t, changes, tenantPrefix(tn)+"aws_athena_workgroup.dml[0]")
		dml := after(changes[tenantPrefix(tn)+"aws_athena_workgroup.dml[0]"])
		assert.Equal(t, name+"-"+tn+"-dml", dml["name"])
		res := block(block(dml, "configuration"), "result_configuration")
		assert.Nil(t, block(res, "encryption_configuration"), "%s DML workgroup must not set result encryption", tn)
		assert.True(t, strings.HasSuffix(res["output_location"].(string), "dml/"), res["output_location"])

		assert.Contains(t, changes, tenantPrefix(tn)+"aws_cloudwatch_metric_alarm.dml_query_failures[0]")
		assert.Contains(t, changes, tenantPrefix(tn)+"aws_cloudwatch_metric_alarm.dml_scanned_bytes[0]")
	}
	assert.NotContains(t, changes, tenantPrefix("nodml")+"aws_athena_workgroup.dml[0]", "tenant override wins")
	assert.Contains(t, changes, tenantPrefix("nodml")+"aws_athena_workgroup.this")

	contract, _ := outputAfter(plan, "platform_contract").(map[string]interface{})
	effective, _ := contract["effective"].(map[string]interface{})
	assert.Equal(t, true, effective["dml_workgroup_enabled"])
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
		hasTLSDeny, hasSseCDeny := false, false
		for _, s := range statements {
			sm := s.(map[string]interface{})
			if sm["sid"] == "DenySseC" && sm["effect"] == "Deny" {
				hasSseCDeny = true
			}
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
		// Table buckets refuse direct data access except through Lake Formation (spec 003 FR-015)
		if regexp.MustCompile(`\.(zone_bucket|dedicated_bucket)\b`).MatchString(addr) {
			hasDirectDeny := false
			for _, s := range statements {
				sm := s.(map[string]interface{})
				if sm["sid"] != "DenyDirectTableAccess" || sm["effect"] != "Deny" {
					continue
				}
				vars := map[string]bool{}
				for _, c := range list(sm["condition"]) {
					vars[c.(map[string]interface{})["variable"].(string)] = true
				}
				hasDirectDeny = vars["aws:PrincipalArn"] && vars["aws:PrincipalIsAWSService"] &&
					assert.Contains(t, strs(sm["actions"]), "s3tables:GetTableData", addr)
			}
			assert.True(t, hasDirectDeny, "%s must deny direct table access", addr)
		}
		// SSE-C uploads would skip the results key (awslkhs-pdpf)
		if regexp.MustCompile(`\.(query_results|results)\b`).MatchString(addr) {
			assert.True(t, hasSseCDeny, "%s must deny SSE-C uploads", addr)
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
	// Shares grant to approved consumer account IDs (spec 003 FR-022)
	settings, err := os.ReadFile("../examples/complete/terraform.auto.tfvars")
	require.NoError(t, err)
	for _, m := range regexp.MustCompile(`"([0-9]{12})" = \{`).FindAllStringSubmatch(string(settings), -1) {
		allowed[m[1]] = true
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

// TestRowFilterExclusionFollowsPrincipal: Lake Formation unions grants per principal, so an
// unfiltered reader entry that shares a principal with a row-filtered entry must also skip the
// filtered table. A reader key containing "/" must still resolve to its own reader, zone, and
// table (PR #3 review, awslkhs-pg8j and awslkhs-ufrk).
func TestRowFilterExclusionFollowsPrincipal(t *testing.T) {
	t.Parallel()
	name := generateTestNamePrefix("rfp")
	analyst := "arn:aws:iam::123456789012:role/sales-eu-analyst"
	vars := merged(completeVars(name), override("sales", map[string]interface{}{
		"readers": map[string]interface{}{
			"eu/analyst": map[string]interface{}{"principal_arn": analyst, "sensitivity_values": []string{"public", "internal"}},
			"analyst":    map[string]interface{}{"principal_arn": analyst, "sensitivity_values": []string{"public"}},
			"semantic": map[string]interface{}{"principal_arn": "arn:aws:iam::123456789012:role/semantic-layer",
				"zones": []string{"consumption"}, "sensitivity_values": []string{"public", "internal"}},
		},
		"row_filters": map[string]interface{}{
			"eu": map[string]interface{}{"reader": "eu/analyst", "zone": "curated", "table": "orders",
				"row_filter_expression": "region = 'EU'"},
		},
	}))
	changes := managedChanges(planExample(t, "complete", vars))
	sales := shared + `module.tenant["sales"].`
	rfKey := name + "-tenant-sales-rf-eu/analyst"

	for _, k := range []string{"eu/analyst", "analyst"} {
		rc := changes[sales+`aws_lakeformation_permissions.reader_table["`+k+`"]`]
		require.NotNil(t, rc, k)
		assert.Equal(t, []string{"no"}, lfExpression(rc)[rfKey], "%s shares the filtered principal, so it must skip the filtered table", k)
	}
	semantic := changes[sales+`aws_lakeformation_permissions.reader_table["semantic"]`]
	require.NotNil(t, semantic)
	_, has := lfExpression(semantic)[rfKey]
	assert.False(t, has, "other principals keep their grants")

	filtered := changes[sales+`aws_lakeformation_resource_lf_tags.row_filtered_table["eu/analyst/curated/orders"]`]
	require.NotNil(t, filtered, "the filtered table is labeled under the full reader key")
	assert.Equal(t, "orders", block(after(filtered), "table")["name"])
}
