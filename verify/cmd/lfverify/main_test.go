package main

import (
	"errors"
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

func TestParseRejectsBadMode(t *testing.T) {
	if _, err := parse("run", []string{"--mode", "chaos"}); !errors.Is(err, errUsage) {
		t.Errorf("got %v", err)
	}
}
