package runner

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/classify"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/matrix"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/ops"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/report"
)

// Observation adds what the runner measured beyond the operation result.
type Observation struct {
	// Name is the object a list operation should or should not return
	Name string
	// PredicateMisses counts rows that failed the row predicate
	PredicateMisses int
	// PredicateErr is set when the predicate could not be evaluated
	PredicateErr error
}

// Evaluate compares a result with the case's expectation. An ERROR outcome is never a pass or a
// fail: the case did not run (FR-025).
func Evaluate(c matrix.Case, res ops.Result, obs Observation) (report.Status, string) {
	if res.Outcome == classify.Error {
		return report.NotRun, "engine or API error: " + firstLine(res.Evidence.Code+" "+res.Evidence.Message)
	}
	switch c.Expect {
	case matrix.Denied:
		if res.Outcome == classify.Denied {
			return report.Pass, ""
		}
		return report.Fail, fmt.Sprintf("expected denial, got %s", res.Outcome)
	case matrix.Rejected:
		switch res.Outcome {
		case classify.Rejected:
			return report.Pass, ""
		case classify.Denied:
			return report.Fail, "denied for lack of permission, so the rule under test was not reached"
		}
		return report.Fail, "the invalid grant was accepted"
	case matrix.Absent:
		if res.Outcome == classify.Denied {
			return report.Pass, "denied"
		}
		if obs.Name != "" && slices.Contains(res.Names, obs.Name) {
			return report.Fail, fmt.Sprintf("%s is listed", obs.Name)
		}
		if leaked := intersect(res.Columns, c.AbsentColumns); len(leaked) > 0 {
			return report.Fail, "columns returned: " + strings.Join(leaked, ", ")
		}
		return report.Pass, ""
	case matrix.Allowed:
		if res.Outcome != classify.Allowed {
			return report.Fail, fmt.Sprintf("expected access, got %s", res.Outcome)
		}
		return checkAllowed(c, res, obs)
	}
	return report.NotRun, fmt.Sprintf("expectation %q is not evaluated by attempting the operation", c.Expect)
}

func checkAllowed(c matrix.Case, res ops.Result, obs Observation) (report.Status, string) {
	var problems []string
	if obs.Name != "" && !slices.Contains(res.Names, obs.Name) {
		problems = append(problems, obs.Name+" is not listed")
	}
	if len(c.ExpectColumns) > 0 && !sameSet(res.Columns, c.ExpectColumns) {
		problems = append(problems, fmt.Sprintf("columns %v, want %v", sorted(res.Columns), sorted(c.ExpectColumns)))
	}
	if leaked := intersect(res.Columns, c.AbsentColumns); len(leaked) > 0 {
		problems = append(problems, "columns returned: "+strings.Join(leaked, ", "))
	}
	if r := c.ExpectRows; r != nil {
		if r.Count != nil && (res.RowCount == nil || *res.RowCount != *r.Count) {
			problems = append(problems, fmt.Sprintf("%s rows, want %d", countString(res.RowCount), *r.Count))
		}
		if len(r.Keys) > 0 && !sameSet(res.Keys, r.Keys) {
			problems = append(problems, fmt.Sprintf("%d key values differ from the %d expected", len(res.Keys), len(r.Keys)))
		}
	}
	if c.MaxRows != nil && res.RowCount != nil && *res.RowCount > *c.MaxRows {
		problems = append(problems, fmt.Sprintf("%d rows exceed the maximum %d", *res.RowCount, *c.MaxRows))
	}
	if c.RowPredicate != nil {
		if obs.PredicateErr != nil {
			return report.NotRun, "row predicate: " + obs.PredicateErr.Error()
		}
		if obs.PredicateMisses > 0 {
			problems = append(problems, fmt.Sprintf("%d rows fail the row predicate", obs.PredicateMisses))
		}
	}
	if len(problems) > 0 {
		return report.Fail, strings.Join(problems, "; ")
	}
	return report.Pass, ""
}

// EvaluateInspection compares effective permissions with the declared set exactly, so an extra
// grant fails as surely as a missing one (FR-029).
func EvaluateInspection(c matrix.Case, effective []string) (report.Status, string) {
	if sameSet(effective, c.ExpectPermissions) {
		return report.VerifiedByInspection, ""
	}
	return report.Fail, fmt.Sprintf("effective permissions %v, want %v", sorted(effective), sorted(c.ExpectPermissions))
}

// Retryable reports whether a positive expectation should be retried: a denial may be a grant
// that has not propagated yet.
func Retryable(c matrix.Case, res ops.Result) bool {
	return c.Expect == matrix.Allowed && res.Outcome == classify.Denied
}

func sameSet(a, b []string) bool {
	return slices.Equal(sorted(a), sorted(b))
}

func sorted(a []string) []string {
	out := slices.Clone(a)
	sort.Strings(out)
	return out
}

func intersect(a, b []string) []string {
	var out []string
	for _, x := range b {
		if slices.ContainsFunc(a, func(y string) bool { return strings.EqualFold(x, y) }) {
			out = append(out, x)
		}
	}
	return out
}

func countString(n *int) string {
	if n == nil {
		return "no"
	}
	return fmt.Sprint(*n)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
