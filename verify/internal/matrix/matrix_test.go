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
	for _, o := range []Op{Insert, Grant, PutSettings, DropTable} {
		if !o.IsWrite() {
			t.Errorf("%s must be a write", o)
		}
	}
	for _, o := range []Op{Select, GetSettings, ListPermissions, StorageRead} {
		if o.IsWrite() {
			t.Errorf("%s must not be a write", o)
		}
	}
}
