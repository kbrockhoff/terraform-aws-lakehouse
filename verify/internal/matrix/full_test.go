package matrix

import (
	"slices"
	"testing"
)

// TestFullMatrixCoverage guards the checked-in full-mode matrix: every persona in FR-009 to
// FR-020 has a positive and a negative case in each grant style, every FR-021 rule boundary is
// attempted, and every persona tries the storage bypass (FR-022).
func TestFullMatrixCoverage(t *testing.T) {
	m, err := Load("../../matrix/full.yaml")
	if err != nil {
		t.Fatal(err)
	}
	personas := []string{
		"admin", "roadmin", "meta_reader", "meta_writer", "reader_multi", "reader_single",
		"writer_multi", "writer_single", "reader_cols", "reader_rows", "reader_cells", "control",
	}
	type key struct {
		persona string
		style   Style
	}
	positive, negative := map[key]bool{}, map[key]bool{}
	storage := map[string]bool{}
	ops := map[Op]bool{}
	for _, inst := range m.Expand() {
		k := key{inst.Persona.Key, inst.Style}
		switch inst.Case.Expect {
		case Allowed:
			positive[k] = true
		case Denied, Absent, Rejected:
			negative[k] = true
		}
		if inst.Case.Op == StorageRead && inst.Case.Expect == Denied {
			storage[inst.Persona.Key] = true
		}
		ops[inst.Case.Op] = true
	}
	for _, p := range personas {
		for _, s := range []Style{Label, Named} {
			if p != "control" && !positive[key{p, s}] {
				t.Errorf("%s has no positive %s case", p, s)
			}
			if !negative[key{p, s}] {
				t.Errorf("%s has no negative %s case", p, s)
			}
		}
		if !storage[p] {
			t.Errorf("%s has no storage bypass case", p)
		}
	}
	for _, op := range []Op{GrantColumnsWithWrite, GrantExcludePartition, CreateFilterMapColumn} {
		if !ops[op] {
			t.Errorf("rule boundary %s is not attempted", op)
		}
	}
	for _, p := range m.Personas {
		if !slices.Contains(personas, p.Key) {
			t.Errorf("unexpected persona %s; personas must exist in the fixture manifest", p.Key)
		}
	}
}

func TestProbeExampleIsValid(t *testing.T) {
	m, err := Load("../../matrix/probe.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if m.Mode != Probe {
		t.Errorf("mode %s", m.Mode)
	}
}
