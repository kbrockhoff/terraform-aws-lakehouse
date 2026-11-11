package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/matrix"
)

const sample = `{
  "run_id": "abc123", "account_id": "111122223333", "region": "us-east-1", "prefix": "lfv-abc123", "stage": 2,
  "inject": "", "runner": "arn:aws:iam::111122223333:role/runner",
  "personas": {"reader_single": "arn:aws:iam::111122223333:role/lfv-abc123-reader-single"},
  "styles": {
    "label": {"namespaces": {"alpha/curated": {"catalog_id": "111122223333:s3tablescatalog/lfv-abc123-curated", "database": "alpha",
      "table_bucket_arn": "arn:aws:s3tables:us-east-1:111122223333:bucket/lfv-abc123-curated", "workgroup": "lfv-abc123-alpha",
      "results_location": "s3://r/alpha/"}}, "tag_keys": {"zone": "lfv-abc123-zone", "mw": "lfv-abc123-mw"}, "filters": {"rows_eu": "alpha-rows_eu"}},
    "named": {"namespaces": {"n_alpha": {"catalog_id": "111122223333:s3tablescatalog/lfv-abc123-named", "database": "n_alpha",
      "table_bucket_arn": "arn:aws:s3tables:us-east-1:111122223333:bucket/lfv-abc123-named", "workgroup": "lfv-abc123-named",
      "results_location": "s3://n/"}}, "filters": {}}
  },
  "tables": {"orders": {"columns": [{"name": "order_id", "type": "bigint"}, {"name": "ssn", "type": "string"}],
    "partition_column": null, "restricted_column": "ssn", "map_column": null, "rows": 200}},
  "kms_key_arn": "arn:aws:kms:us-east-1:111122223333:key/k"
}`

func load(t *testing.T) *Manifest {
	t.Helper()
	p := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(p, []byte(sample), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestResolve(t *testing.T) {
	m := load(t)
	l, err := m.Resolve(matrix.Label, matrix.Resource{Tenant: "alpha", Table: "orders"})
	if err != nil {
		t.Fatal(err)
	}
	if l.Database != "alpha" || l.Workgroup != "lfv-abc123-alpha" || !l.Exists || l.AthenaCatalog() != "s3tablescatalog/lfv-abc123-curated" {
		t.Errorf("label target: %+v", l)
	}
	n, err := m.Resolve(matrix.Named, matrix.Resource{Tenant: "alpha", Table: "late_table"})
	if err != nil {
		t.Fatal(err)
	}
	if n.Database != "n_alpha" || n.Exists {
		t.Errorf("named target: %+v", n)
	}
	if _, err := m.Resolve(matrix.Label, matrix.Resource{Tenant: "zeta"}); err == nil {
		t.Error("unknown tenant must fail")
	}
	if got := (Target{Key: "n_alpha", Table: "orders", Columns: []string{"a", "b"}}).Label(); got != "n_alpha.orders(a,b)" {
		t.Errorf("label: %s", got)
	}
	if cols := m.Tables["orders"].ColumnNames(); len(cols) != 2 || cols[1] != "ssn" {
		t.Errorf("columns: %v", cols)
	}
}

func TestProbeFromFull(t *testing.T) {
	m := load(t)
	m.Personas["writer_multi"] = "arn:aws:iam::111122223333:role/lfv-abc123-writer-multi"
	m.Tables["customers"] = Table{Columns: []Column{{Name: "customer_id", Type: "bigint"}}, Rows: 50}
	m.Styles.Named.Namespaces["n_alpha"] = m.Styles.Named.Namespaces["n_alpha"]
	full, err := matrix.Parse([]byte(`mode: full
personas:
  - key: reader_single
    cases:
      - {id: c, op: count, resource: {tenant: alpha, table: orders}, expect: allowed, expect_rows: {count: 200}}
      - {id: s, op: select, resource: {tenant: alpha, table: orders}, expect: allowed, expect_rows: {count: 200, key_column: order_id, keys: ["1"]}}
      - {id: w, phase: scenario, op: insert, resource: {tenant: alpha, table: orders}, expect: denied}
      - {id: b, op: storage_read, resource: {tenant: alpha, table: orders}, expect: denied}
`))
	if err != nil {
		t.Fatal(err)
	}
	p, err := ProbeFromFull(full, m)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]matrix.Case{}
	for _, per := range p.Personas {
		for _, c := range per.Cases {
			ids[c.ID] = c
		}
	}
	if _, ok := ids["w/label"]; ok {
		t.Error("writes must not be probed")
	}
	if _, ok := ids["b/label"]; ok {
		t.Error("storage reads must not be probed")
	}
	c := ids["c/label"]
	if c.Resource.Database != "alpha" || c.Resource.Workgroup != "lfv-abc123-alpha" || c.ExpectRows == nil || *c.ExpectRows.Count != 200 {
		t.Errorf("count case: %+v", c)
	}
	if s := ids["s/named"]; s.ExpectRows != nil {
		t.Errorf("select counts and keys must be dropped: %+v", s.ExpectRows)
	}
	if ids["writer_multi-inspect-customers/label"].Expect != matrix.Inspect {
		t.Error("inspection cases are added")
	}
}
