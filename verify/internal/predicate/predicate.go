package predicate

import (
	"errors"
	"fmt"
	"strconv"
	"time"
)

// ErrColumnNotVisible means the predicate column is absent from the row, so the persona cannot
// see it and the case cannot be evaluated.
var ErrColumnNotVisible = errors.New("predicate column not visible")

// Op is a comparison operator.
type Op string

const (
	Eq Op = "eq"
	Ne Op = "ne"
	In Op = "in"
	Lt Op = "lt"
	Gt Op = "gt"
)

// Predicate is a structured row condition. Matrix files use it instead of SQL so it can be
// evaluated in memory against rows that are never stored or logged.
type Predicate struct {
	Column string   `yaml:"column" json:"column"`
	Op     Op       `yaml:"op" json:"op"`
	Values []string `yaml:"values" json:"values"`
}

// Validate checks the operator and the number of values.
func (p Predicate) Validate() error {
	if p.Column == "" {
		return errors.New("predicate column is empty")
	}
	switch p.Op {
	case In:
		if len(p.Values) == 0 {
			return errors.New("predicate op in needs at least one value")
		}
	case Eq, Ne, Lt, Gt:
		if len(p.Values) != 1 {
			return fmt.Errorf("predicate op %s needs exactly one value", p.Op)
		}
	default:
		return fmt.Errorf("unknown predicate op %q", p.Op)
	}
	return nil
}

// Match reports whether row satisfies the predicate. It returns only a boolean, so callers never
// handle the value itself.
func (p Predicate) Match(row map[string]string) (bool, error) {
	if err := p.Validate(); err != nil {
		return false, err
	}
	v, ok := row[p.Column]
	if !ok {
		return false, ErrColumnNotVisible
	}
	switch p.Op {
	case Eq:
		return v == p.Values[0], nil
	case Ne:
		return v != p.Values[0], nil
	case In:
		for _, want := range p.Values {
			if v == want {
				return true, nil
			}
		}
		return false, nil
	default:
		c, err := compare(v, p.Values[0])
		if err != nil {
			return false, err
		}
		if p.Op == Lt {
			return c < 0, nil
		}
		return c > 0, nil
	}
}

// compare orders numbers numerically, ISO dates chronologically, and anything else as strings.
func compare(a, b string) (int, error) {
	if x, errA := strconv.ParseFloat(a, 64); errA == nil {
		y, errB := strconv.ParseFloat(b, 64)
		if errB != nil {
			return 0, fmt.Errorf("predicate value is not a number")
		}
		return cmp(x < y, x > y), nil
	}
	if x, errA := time.Parse(time.DateOnly, a); errA == nil {
		y, errB := time.Parse(time.DateOnly, b)
		if errB != nil {
			return 0, fmt.Errorf("predicate value is not a date")
		}
		return cmp(x.Before(y), x.After(y)), nil
	}
	return cmp(a < b, a > b), nil
}

func cmp(less, greater bool) int {
	switch {
	case less:
		return -1
	case greater:
		return 1
	}
	return 0
}
