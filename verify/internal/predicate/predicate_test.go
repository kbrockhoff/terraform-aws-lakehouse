package predicate

import (
	"errors"
	"testing"
)

func TestMatch(t *testing.T) {
	row := map[string]string{"region": "EU", "amount": "12.50", "order_date": "2026-03-01"}
	cases := []struct {
		name string
		p    Predicate
		want bool
	}{
		{"eq match", Predicate{"region", Eq, []string{"EU"}}, true},
		{"eq miss", Predicate{"region", Eq, []string{"US"}}, false},
		{"ne", Predicate{"region", Ne, []string{"US"}}, true},
		{"in", Predicate{"region", In, []string{"US", "EU"}}, true},
		{"in miss", Predicate{"region", In, []string{"US", "APAC"}}, false},
		{"lt number", Predicate{"amount", Lt, []string{"100"}}, true},
		{"gt number is numeric not lexical", Predicate{"amount", Gt, []string{"9"}}, true},
		{"lt date", Predicate{"order_date", Lt, []string{"2026-04-01"}}, true},
		{"gt date", Predicate{"order_date", Gt, []string{"2026-04-01"}}, false},
		{"gt string", Predicate{"region", Gt, []string{"APAC"}}, true},
	}
	for _, c := range cases {
		got, err := c.p.Match(row)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestMatchMissingColumn(t *testing.T) {
	_, err := Predicate{"ssn", Eq, []string{"x"}}.Match(map[string]string{"region": "EU"})
	if !errors.Is(err, ErrColumnNotVisible) {
		t.Fatalf("got %v, want ErrColumnNotVisible", err)
	}
}

func TestValidate(t *testing.T) {
	bad := []Predicate{
		{"", Eq, []string{"x"}},
		{"c", In, nil},
		{"c", Eq, []string{"a", "b"}},
		{"c", "like", []string{"a"}},
	}
	for _, p := range bad {
		if p.Validate() == nil {
			t.Errorf("%+v: want error", p)
		}
	}
}

func TestCompareTypeMismatch(t *testing.T) {
	if _, err := (Predicate{"amount", Lt, []string{"abc"}}).Match(map[string]string{"amount": "5"}); err == nil {
		t.Fatal("want error comparing a number with a non-number")
	}
}
