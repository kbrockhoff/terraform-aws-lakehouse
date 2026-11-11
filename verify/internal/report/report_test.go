package report

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/classify"
)

func sample() *Run {
	n := 150
	start := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	return &Run{RunID: "abc123", Mode: "full", Started: start, Finished: start.Add(90 * time.Second), Results: []Result{
		{ID: "rr-count/label", Persona: "reader_rows", Style: "label", Op: "count", Resource: "alpha/curated.orders",
			Expected: "allowed", Observed: Observed{Outcome: classify.Allowed, RowCount: &n}, Status: Pass},
		{ID: "rc-ssn/named", Persona: "reader_cols", Style: "named", Op: "select_columns", Resource: "n_alpha.orders",
			Expected: "denied", Observed: Observed{Outcome: classify.Allowed}, Status: Fail, Reason: "column | ssn returned"},
		{ID: "wm-insert/label", Persona: "writer_multi", Style: "label", Op: "insert", Resource: "alpha/curated.customers",
			Expected: "allowed", Observed: Observed{Outcome: classify.Error}, Status: NotRun, Reason: "engine error",
			Evidence: classify.Evidence{Code: "INTERNAL_ERROR_QUERY_ENGINE"}},
	}}
}

func TestTotalsAndFailed(t *testing.T) {
	r := sample()
	got := r.Totals()
	if got[Pass] != 1 || got[Fail] != 1 || got[NotRun] != 1 {
		t.Errorf("totals: %v", got)
	}
	if !r.Failed() {
		t.Error("a fail or not-run must fail the run")
	}
	r.Results = r.Results[:1]
	if r.Failed() {
		t.Error("a passing run must not fail")
	}
}

func TestWriteAll(t *testing.T) {
	dir := t.TempDir()
	if err := sample().WriteAll(dir); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var back Run
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Results) != 3 || *back.Results[0].Observed.RowCount != 150 {
		t.Errorf("json round trip: %+v", back.Results)
	}
	junit, _ := os.ReadFile(filepath.Join(dir, "junit.xml"))
	for _, want := range []string{`name="reader_cols/named"`, `failures="1"`, `errors="1"`, "not run: engine error"} {
		if !strings.Contains(string(junit), want) {
			t.Errorf("junit missing %q", want)
		}
	}
	md, _ := os.ReadFile(filepath.Join(dir, "report.md"))
	for _, want := range []string{"3 cases: 1 pass, 0 verified by inspection, 1 fail, 1 not run. Duration 1m30s.", "## reader_rows", `column \| ssn returned`, "ALLOWED, 150 rows"} {
		if !strings.Contains(string(md), want) {
			t.Errorf("markdown missing %q", want)
		}
	}
}

// TestObservedHasNoValueFields guards FR-030: the report model must not be able to carry row
// values. Adding a field here is a deliberate decision that needs a spec change.
func TestObservedHasNoValueFields(t *testing.T) {
	var fields []string
	ty := reflect.TypeOf(Observed{})
	for i := 0; i < ty.NumField(); i++ {
		fields = append(fields, ty.Field(i).Name)
	}
	want := "Outcome,Columns,RowCount,Permissions,ScanBytes"
	if got := strings.Join(fields, ","); got != want {
		t.Errorf("Observed fields changed: got %s, want %s", got, want)
	}
}

func TestMarkdownEscapesNewlines(t *testing.T) {
	r := sample()
	r.Results[0].Reason = "line1\nline2"
	var buf bytes.Buffer
	if err := r.WriteMarkdown(&buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "line1\nline2") {
		t.Error("newline inside a table cell breaks the table")
	}
}
