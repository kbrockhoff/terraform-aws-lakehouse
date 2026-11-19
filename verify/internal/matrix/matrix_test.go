package matrix

import (
	"strings"
	"testing"
)

func TestLoadFullDefaultsAndExpand(t *testing.T) {
	m, err := Load("testdata/full_valid.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if m.Defaults.SettleSeconds != 120 || m.Defaults.QueryRowLimit != 10 || m.Defaults.ScanCapBytes != 1<<30 {
		t.Errorf("defaults not applied: %+v", m.Defaults)
	}
	c := m.Personas[0].Cases[0]
	if c.Style != Both || c.Phase != Static || c.Resource.Zone != "curated" {
		t.Errorf("case defaults not applied: %+v", c)
	}
	inst := m.Expand()
	var ids []string
	for _, i := range inst {
		ids = append(ids, i.ID)
	}
	want := "rs-select-orders/label,rs-select-orders/named,rs-select-gamma/label,rs-select-gamma/named,rs-relabel"
	if got := strings.Join(ids, ","); got != want {
		t.Errorf("expand: got %s, want %s", got, want)
	}
}

func TestLoadProbe(t *testing.T) {
	m, err := Load("testdata/probe_valid.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if m.Defaults.ScanCapBytes != 1048576 {
		t.Errorf("explicit default overridden: %d", m.Defaults.ScanCapBytes)
	}
	if n := len(m.Expand()); n != 2 {
		t.Errorf("probe cases do not expand by style: got %d", n)
	}
}

func TestInvalid(t *testing.T) {
	cases := []struct {
		name, yaml, want string
	}{
		{"unknown field", "mode: full\npersonas:\n  - key: a\n    cases:\n      - id: x\n        op: select\n        expct: allowed\n", "field expct not found"},
		{"unknown op", "mode: full\npersonas:\n  - key: a\n    cases:\n      - {id: x, op: truncate, resource: {tenant: alpha}, expect: denied}\n", `unknown op "truncate"`},
		{"duplicate id", "mode: full\npersonas:\n  - key: a\n    cases:\n      - {id: x, op: select, resource: {tenant: alpha, table: t}, expect: denied}\n      - {id: x, op: count, resource: {tenant: alpha, table: t}, expect: denied}\n", `duplicate case id "x"`},
		{"probe write", "mode: probe\npersonas:\n  - key: a\n    role_arn: arn:aws:iam::1:role/a\n    cases:\n      - {id: x, op: insert, resource: {catalog_id: c, database: d, table: t, workgroup: w}, expect: denied}\n", "probe mode never attempts insert"},
		{"probe row values", "mode: probe\npersonas:\n  - key: a\n    role_arn: arn:aws:iam::1:role/a\n    cases:\n      - {id: x, op: select, resource: {catalog_id: c, database: d, table: t, workgroup: w}, expect: allowed, expect_rows: {keys: [\"1\"]}}\n", "cannot declare row key values"},
		{"probe without role", "mode: probe\npersonas:\n  - key: a\n    cases:\n      - {id: x, op: get_table, resource: {catalog_id: c, database: d, table: t}, expect: allowed}\n", "role_arn is required"},
		{"probe scenario", "mode: probe\npersonas:\n  - key: a\n    role_arn: arn:aws:iam::1:role/a\n    cases:\n      - {id: x, phase: scenario, op: get_table, resource: {catalog_id: c, database: d, table: t}, expect: allowed}\n", "static cases only"},
		{"full inspect", "mode: full\npersonas:\n  - key: a\n    cases:\n      - {id: x, op: insert, resource: {tenant: alpha, table: t}, expect: inspect, expect_permissions: [INSERT]}\n", "expect inspect is for probe mode"},
		{"full with role", "mode: full\npersonas:\n  - key: a\n    role_arn: arn:aws:iam::1:role/a\n    cases:\n      - {id: x, op: get_table, resource: {tenant: alpha, table: t}, expect: allowed}\n", "comes from the manifest"},
		{"bad predicate", "mode: full\npersonas:\n  - key: a\n    cases:\n      - {id: x, op: select, resource: {tenant: alpha, table: t}, expect: allowed, row_predicate: {column: r, op: like, values: [a]}}\n", "unknown predicate op"},
		{"absent without columns", "mode: full\npersonas:\n  - key: a\n    cases:\n      - {id: x, op: select, resource: {tenant: alpha, table: t}, expect: absent}\n", "needs absent_columns"},
		{"bad mode", "mode: chaos\npersonas:\n  - key: a\n    cases: []\n", "mode must be full or probe"},
		{"unsafe persona key", "mode: full\npersonas:\n  - key: \"a; rm -rf x\"\n    cases: []\n", "must match"},
		{"path in persona key", "mode: full\npersonas:\n  - key: ../a\n    cases: []\n", "must match"},
		{"denied_by without denial", "mode: full\npersonas:\n  - key: a\n    cases:\n      - {id: x, op: select, resource: {tenant: alpha, table: t}, expect: allowed, denied_by: lake_formation}\n", "denied_by needs expect: denied"},
		{"denied_by on storage_read", "mode: full\npersonas:\n  - key: a\n    cases:\n      - {id: x, op: storage_read, resource: {tenant: alpha, table: t}, expect: denied, denied_by: lake_formation}\n", "does not apply to storage_read"},
		{"unknown denied_by", "mode: full\npersonas:\n  - key: a\n    cases:\n      - {id: x, op: select, resource: {tenant: alpha, table: t}, expect: denied, denied_by: iam}\n", `unknown denied_by "iam"`},
		{"via on both styles", "mode: full\npersonas:\n  - key: a\n    cases:\n      - {id: x, op: select, resource: {tenant: gamma, table: t, via: named}, expect: denied}\n", "applies only to label-style query cases"},
		{"via on a catalog op", "mode: full\npersonas:\n  - key: a\n    cases:\n      - {id: x, style: label, op: get_table, resource: {tenant: gamma, table: t, via: named}, expect: denied}\n", "applies only to label-style query cases"},
		{"via in probe mode", "mode: probe\npersonas:\n  - key: a\n    role_arn: arn:aws:iam::1:role/a\n    cases:\n      - {id: x, op: select, resource: {catalog_id: c, database: d, table: t, workgroup: w, via: named}, expect: denied}\n", "via is for full mode"},
		// Spec 003 check ops and direct storage writes
		{"check op without match", "mode: full\npersonas:\n  - key: a\n    cases:\n      - {id: x, style: label, op: account_settings, expect: allowed}\n", "check ops need expect: match"},
		{"match on an access op", "mode: full\npersonas:\n  - key: a\n    cases:\n      - {id: x, op: select, resource: {tenant: alpha, table: t}, expect: match}\n", "expect match is for check ops"},
		{"expect_values without match", "mode: full\npersonas:\n  - key: a\n    cases:\n      - {id: x, op: select, resource: {tenant: alpha, table: t}, expect: denied, expect_values: {mode: SSE-KMS}}\n", "expect_values needs expect: match"},
		{"check op on both styles", "mode: full\npersonas:\n  - key: a\n    cases:\n      - {id: x, op: catalog_encryption, expect: match}\n", "use style: label"},
		{"denied_by on storage_write", "mode: full\npersonas:\n  - key: a\n    cases:\n      - {id: x, op: storage_write, resource: {tenant: alpha, table: t}, expect: denied, denied_by: lake_formation}\n", "does not apply to storage_write"},
		{"probe IAM_ALLOWED_PRINCIPALS check without a database", "mode: probe\npersonas:\n  - key: a\n    role_arn: arn:aws:iam::1:role/a\n    cases:\n      - {id: x, op: no_iam_allowed_principals, expect: match}\n", "probe resources need catalog_id and database"},
		{"probe storage_write", "mode: probe\npersonas:\n  - key: a\n    role_arn: arn:aws:iam::1:role/a\n    cases:\n      - {id: x, op: storage_write, resource: {catalog_id: c, database: d, table: t}, expect: denied}\n", "probe mode never attempts storage_write"},
	}
	for _, c := range cases {
		_, err := Parse([]byte(c.yaml))
		if err == nil {
			t.Errorf("%s: want error", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %q does not contain %q", c.name, err, c.want)
		}
	}
}

func TestIsWrite(t *testing.T) {
	for _, o := range []Op{Insert, Grant, PutSettings, DropTable, StorageWrite} {
		if !o.IsWrite() {
			t.Errorf("%s must be a write", o)
		}
	}
	for _, o := range []Op{Select, GetSettings, ListPermissions, StorageRead, AccountSettings, CatalogEncryption, CatalogResourcePolicy, NoIAMAllowedPrincipals} {
		if o.IsWrite() {
			t.Errorf("%s must not be a write", o)
		}
	}
}

// TestCheckOpsValid checks that check ops parse in both modes; probe check cases need no
// catalog or database.
func TestCheckOpsValid(t *testing.T) {
	for _, y := range []string{
		"mode: full\npersonas:\n  - key: admin\n    cases:\n      - {id: s, style: label, op: account_settings, expect: match, expect_values: {cross_account_version: manifest}}\n      - {id: r, style: label, op: catalog_resource_policy, expect: match}\n",
		"mode: probe\npersonas:\n  - key: admin\n    role_arn: arn:aws:iam::1:role/a\n    cases:\n      - {id: e, op: catalog_encryption, expect: match, expect_values: {mode: SSE-KMS-WITH-SERVICE-ROLE}}\n      - {id: i, op: no_iam_allowed_principals, resource: {catalog_id: c, database: d}, expect: match}\n",
	} {
		if _, err := Parse([]byte(y)); err != nil {
			t.Errorf("valid check ops rejected: %v", err)
		}
	}
}
