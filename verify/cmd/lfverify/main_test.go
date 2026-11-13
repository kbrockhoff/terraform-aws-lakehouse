package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/matrix"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/preflight"
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

func TestVersionString(t *testing.T) {
	version, commit = "v1.2.3", "abc1234"
	defer func() { version, commit = "dev", "none" }()
	if got := versionString(); !strings.HasPrefix(got, "lfverify v1.2.3 (abc1234) go") {
		t.Errorf("got %q", got)
	}
}

// TestCommandHelp: every command answers --help with only flags it registers (FR-036)
func TestCommandHelp(t *testing.T) {
	for cmd, names := range commandFlags {
		_, err := parse(cmd, []string{"--help"})
		if !errors.Is(err, flag.ErrHelp) {
			t.Errorf("%s --help: got %v, want flag.ErrHelp", cmd, err)
		}
		probe, _ := parse(cmd, nil)
		for _, n := range names {
			if probe.fs.Lookup(n) == nil {
				t.Errorf("%s lists unknown flag %s", cmd, n)
			}
		}
		var buf bytes.Buffer
		commandHelp(&buf, cmd, probe.fs)
		for _, n := range names {
			if !strings.Contains(buf.String(), "--"+n) {
				t.Errorf("%s help omits --%s", cmd, n)
			}
		}
	}
	if err := dispatch(context.Background(), "help", nil); err != nil {
		t.Errorf("help: %v", err)
	}
}

func TestExampleMatrixNeverOverwrites(t *testing.T) {
	out := filepath.Join(t.TempDir(), "probe.yaml")
	if err := cmdExampleMatrix(&flags{out: out}); err != nil {
		t.Fatal(err)
	}
	if err := cmdExampleMatrix(&flags{out: out}); err == nil {
		t.Error("an existing file must not be overwritten")
	}
}

func TestProbeSequence(t *testing.T) {
	var calls []string
	step := func(name string, err error) func() error {
		return func() error { calls = append(calls, name); return err }
	}
	steps := func(pre, run, diff error) probeSteps {
		return probeSteps{
			preflight: step("preflight", pre),
			snapshot:  func(n string) error { calls = append(calls, "snapshot "+n); return nil },
			run:       step("run", run),
			diff:      step("diff", diff),
		}
	}

	calls = nil
	if err := probeSequence(steps(nil, nil, nil)); err != nil {
		t.Fatal(err)
	}
	if want := []string{"preflight", "snapshot before", "run", "snapshot after", "diff"}; !slices.Equal(calls, want) {
		t.Errorf("order %v, want %v", calls, want)
	}

	calls = nil
	if err := probeSequence(steps(preflight.ErrRefused, nil, nil)); !errors.Is(err, preflight.ErrRefused) || len(calls) != 1 {
		t.Errorf("a refused preflight stops the run: %v %v", err, calls)
	}

	calls = nil
	failed := errors.New("some cases failed")
	err := probeSequence(steps(nil, failed, nil))
	if !errors.Is(err, failed) || !slices.Contains(calls, "diff") {
		t.Errorf("failing cases still get the after snapshot and diff: %v %v", err, calls)
	}
}
