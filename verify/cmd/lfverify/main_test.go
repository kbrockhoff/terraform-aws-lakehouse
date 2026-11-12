package main

import (
	"errors"
	"slices"
	"testing"

	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/matrix"
)

func TestDetectScenarios(t *testing.T) {
	got := detectScenarios([]string{"admin", "reader_single", "writer_multi", "meta_reader"})
	want := [][2]string{
		{"extra_grant", "reader_single"}, {"missing_grant", "reader_single"},
		{"extra_grant", "writer_multi"}, {"missing_grant", "writer_multi"},
		{"drop_column_restriction", "reader_cols"}, {"drop_row_filter", "reader_rows"}, {"open_default", ""},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestProbeDatabasesDeduplicates(t *testing.T) {
	m := &matrix.Matrix{Personas: []matrix.Persona{
		{Cases: []matrix.Case{{Resource: matrix.Resource{CatalogID: "c", Database: "a"}}, {Resource: matrix.Resource{CatalogID: "c", Database: "a"}}}},
		{Cases: []matrix.Case{{Resource: matrix.Resource{CatalogID: "c", Database: "b"}}, {Op: matrix.GetSettings}}},
	}}
	if got := probeDatabases(m); len(got) != 2 {
		t.Errorf("got %v", got)
	}
}

// The run ID reaches output paths and the detect overlay command, so only the fixture's form passes
func TestParseRunID(t *testing.T) {
	for _, id := range []string{"../etc", "abc12", "ABC123", "abc123; rm -rf ~", "abcdefg"} {
		if _, err := parse("run", []string{"--run-id", id}); !errors.Is(err, errUsage) {
			t.Errorf("run ID %q: got %v, want a usage error", id, err)
		}
	}
	if _, err := parse("run", []string{"--run-id", "0a1b2c"}); err != nil {
		t.Errorf("valid run ID rejected: %v", err)
	}
}

func TestParseRejectsBadMode(t *testing.T) {
	if _, err := parse("run", []string{"--mode", "chaos"}); !errors.Is(err, errUsage) {
		t.Errorf("got %v", err)
	}
}

func TestProbeResources(t *testing.T) {
	t.Setenv("LFVERIFY_PROBE_RESULTS_BUCKETS", "results")
	t.Setenv("LFVERIFY_PROBE_KMS_KEYS", "k1")
	m := &matrix.Matrix{Mode: matrix.Probe, Personas: []matrix.Persona{{
		Key: "reader", RoleARN: "arn:aws:iam::111:role/reader",
		Cases: []matrix.Case{{Resource: matrix.Resource{CatalogID: "111:s3tablescatalog/curated", Database: "sales", Table: "orders", Workgroup: "wg"}}},
	}}}
	e := &env{account: "111", role: "arn:aws:iam::111:role/runner"}
	e.cfg.Region = "us-east-1"
	got := probeResources(m, e)
	for _, want := range []string{
		"arn:aws:iam::111:role/reader",
		"arn:aws:glue:us-east-1:111:catalog/s3tablescatalog/curated",
		"arn:aws:glue:us-east-1:111:database/s3tablescatalog/curated/sales",
		"arn:aws:glue:us-east-1:111:table/s3tablescatalog/curated/sales/orders",
		"arn:aws:s3tables:us-east-1:111:bucket/curated",
		"arn:aws:athena:us-east-1:111:workgroup/wg",
		"arn:aws:s3:::results",
		"arn:aws:kms:us-east-1:111:key/k1",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %s in %v", want, got)
		}
	}
}
