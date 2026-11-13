package runner

import (
	"strings"
	"testing"

	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/classify"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/matrix"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/ops"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/predicate"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/report"
)

func n(i int) *int { return &i }

func TestEvaluate(t *testing.T) {
	allowed := func(f func(*ops.Result)) ops.Result {
		r := ops.Result{Outcome: classify.Allowed}
		if f != nil {
			f(&r)
		}
		return r
	}
	cases := []struct {
		name string
		c    matrix.Case
		res  ops.Result
		obs  Observation
		want report.Status
		why  string
	}{
		{"error is not run", matrix.Case{Expect: matrix.Denied}, ops.Result{Outcome: classify.Error, Evidence: classify.Evidence{Code: "INTERNAL_ERROR_QUERY_ENGINE"}}, Observation{}, report.NotRun, "INTERNAL_ERROR"},
		{"denied as expected", matrix.Case{Expect: matrix.Denied}, ops.Result{Outcome: classify.Denied}, Observation{}, report.Pass, ""},
		{"over-permissive", matrix.Case{Expect: matrix.Denied}, allowed(nil), Observation{}, report.Fail, "expected denial"},
		{"rejected", matrix.Case{Expect: matrix.Rejected}, ops.Result{Outcome: classify.Rejected}, Observation{}, report.Pass, ""},
		{"rejection accepted", matrix.Case{Expect: matrix.Rejected}, allowed(nil), Observation{}, report.Fail, "accepted"},
		{"rejection denied", matrix.Case{Expect: matrix.Rejected}, ops.Result{Outcome: classify.Denied}, Observation{}, report.Fail, "not reached"},
		{"absent column omitted", matrix.Case{Expect: matrix.Absent, AbsentColumns: []string{"ssn"}}, allowed(func(r *ops.Result) { r.Columns = []string{"order_id"} }), Observation{}, report.Pass, ""},
		{"absent column leaked", matrix.Case{Expect: matrix.Absent, AbsentColumns: []string{"ssn"}}, allowed(func(r *ops.Result) { r.Columns = []string{"order_id", "SSN"} }), Observation{}, report.Fail, "ssn"},
		{"absent via denial", matrix.Case{Expect: matrix.Absent, AbsentColumns: []string{"ssn"}}, ops.Result{Outcome: classify.Denied}, Observation{}, report.Pass, "denied"},
		{"absent from list", matrix.Case{Expect: matrix.Absent}, allowed(func(r *ops.Result) { r.Names = []string{"alpha"} }), Observation{Name: "gamma"}, report.Pass, ""},
		{"absent on a rejected request", matrix.Case{Expect: matrix.Absent, AbsentColumns: []string{"ssn"}}, ops.Result{Outcome: classify.Rejected, Evidence: classify.Evidence{Code: "InvalidInputException"}}, Observation{Name: "gamma"}, report.NotRun, "absence was not shown"},
		{"listed when absent expected", matrix.Case{Expect: matrix.Absent}, allowed(func(r *ops.Result) { r.Names = []string{"gamma"} }), Observation{Name: "gamma"}, report.Fail, "listed"},
		{"count matches", matrix.Case{Expect: matrix.Allowed, ExpectRows: &matrix.Rows{Count: n(150)}}, allowed(func(r *ops.Result) { r.RowCount = n(150) }), Observation{}, report.Pass, ""},
		{"row filter not applied", matrix.Case{Expect: matrix.Allowed, ExpectRows: &matrix.Rows{Count: n(150)}}, allowed(func(r *ops.Result) { r.RowCount = n(200) }), Observation{}, report.Fail, "200 rows, want 150"},
		{"schema", matrix.Case{Expect: matrix.Allowed, ExpectColumns: []string{"b", "a"}}, allowed(func(r *ops.Result) { r.Columns = []string{"a", "b"} }), Observation{}, report.Pass, ""},
		{"schema extra column", matrix.Case{Expect: matrix.Allowed, ExpectColumns: []string{"a"}}, allowed(func(r *ops.Result) { r.Columns = []string{"a", "ssn"} }), Observation{}, report.Fail, "columns"},
		{"keys", matrix.Case{Expect: matrix.Allowed, ExpectRows: &matrix.Rows{Keys: []string{"1", "2"}}}, allowed(func(r *ops.Result) { r.Keys = []string{"2", "3"} }), Observation{}, report.Fail, "key values"},
		{"predicate misses", matrix.Case{Expect: matrix.Allowed, RowPredicate: &predicate.Predicate{Column: "region", Op: predicate.Eq, Values: []string{"EU"}}}, allowed(nil), Observation{PredicateMisses: 3}, report.Fail, "3 rows fail"},
		{"predicate column hidden", matrix.Case{Expect: matrix.Allowed, RowPredicate: &predicate.Predicate{Column: "region", Op: predicate.Eq, Values: []string{"EU"}}}, allowed(nil), Observation{PredicateErr: predicate.ErrColumnNotVisible}, report.NotRun, "not visible"},
		{"max rows", matrix.Case{Expect: matrix.Allowed, MaxRows: n(10)}, allowed(func(r *ops.Result) { r.RowCount = n(11) }), Observation{}, report.Fail, "exceed"},
		{"not listed", matrix.Case{Expect: matrix.Allowed}, allowed(func(r *ops.Result) { r.Names = []string{"alpha"} }), Observation{Name: "late_table"}, report.Fail, "not listed"},
		{"under-permissive", matrix.Case{Expect: matrix.Allowed}, ops.Result{Outcome: classify.Denied}, Observation{}, report.Fail, "expected access"},
	}
	for _, c := range cases {
		got, why := Evaluate(c.c, c.res, c.obs)
		if got != c.want || !strings.Contains(why, c.why) {
			t.Errorf("%s: got %s %q, want %s containing %q", c.name, got, why, c.want, c.why)
		}
	}
}

func TestEvaluateInspection(t *testing.T) {
	c := matrix.Case{ExpectPermissions: []string{"SELECT", "INSERT"}}
	if s, _ := EvaluateInspection(c, []string{"INSERT", "SELECT"}); s != report.VerifiedByInspection {
		t.Errorf("exact set: %s", s)
	}
	if s, why := EvaluateInspection(c, []string{"INSERT", "SELECT", "DROP"}); s != report.Fail || !strings.Contains(why, "DROP") {
		t.Errorf("extra grant must fail: %s %s", s, why)
	}
}

func TestRetryable(t *testing.T) {
	if !Retryable(matrix.Case{Expect: matrix.Allowed}, ops.Result{Outcome: classify.Denied}) {
		t.Error("a denied positive case retries")
	}
	if Retryable(matrix.Case{Expect: matrix.Denied}, ops.Result{Outcome: classify.Allowed}) {
		t.Error("negative cases never retry")
	}
}

func TestRowKey(t *testing.T) {
	a, b := rowKey("abc123", "writer_multi/label/x"), rowKey("abc123", "writer_multi/named/x")
	if a == b || a < 900000 || a >= 990000 || rowKey("abc123", "writer_multi/label/x") != a {
		t.Errorf("row keys must be deterministic, distinct, and outside the seeded range: %d %d", a, b)
	}
}

// TestRedactQueryEvidence: probe query evidence keeps the code and request ID only (PR #3
// review, awslkhs-o5bm)
func TestRedactQueryEvidence(t *testing.T) {
	ev := RedactQueryEvidence(classify.Evidence{Code: "INVALID_CAST_ARGUMENT", RequestID: "r1", Message: "Value cannot be cast: '900-00-0001'"})
	if ev.Code != "INVALID_CAST_ARGUMENT" || ev.RequestID != "r1" || strings.Contains(ev.Message, "900-00") {
		t.Errorf("got %+v", ev)
	}
	status, why := Evaluate(matrix.Case{Expect: matrix.Allowed}, ops.Result{Outcome: classify.Error, Evidence: ev}, Observation{})
	if status != report.NotRun || strings.Contains(why, "900-00") || !strings.Contains(why, "INVALID_CAST_ARGUMENT") {
		t.Errorf("reason leaks or loses the code: %s %q", status, why)
	}
}
