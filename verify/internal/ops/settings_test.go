package ops

import (
	"slices"
	"strings"
	"testing"
)

func TestCompare(t *testing.T) {
	actual := map[string]string{"mode": "SSE-KMS", "kms_key_arn": "k"}
	if got := compare(actual, map[string]string{"mode": "SSE-KMS", "kms_key_arn": "k"}); len(got) != 0 {
		t.Errorf("equal values: %v", got)
	}
	got := compare(actual, map[string]string{"mode": "DISABLED", "typo": "x"})
	if len(got) != 2 || !strings.Contains(got[0], `mode is "SSE-KMS", want "DISABLED"`) || !strings.Contains(got[1], "typo is not a known setting") {
		t.Errorf("mismatches: %v", got)
	}
}

func TestJoined(t *testing.T) {
	if got := joined(nil); got != "none" {
		t.Errorf("empty: %s", got)
	}
	in := []string{"b", "a"}
	if got := joined(in); got != "a,b" || !slices.Equal(in, []string{"b", "a"}) {
		t.Errorf("sorted copy: %s %v", got, in)
	}
}

func TestAllowsRAMShare(t *testing.T) {
	ok := `{"Statement":[{"Effect":"Allow","Action":"glue:ShareResource","Principal":{"Service":"ram.amazonaws.com"},"Resource":"*"}]}`
	list := `{"Statement":[{"Effect":"Allow","Action":["glue:ShareResource"],"Principal":{"Service":["ram.amazonaws.com"]},"Resource":"*"}]}`
	deny := `{"Statement":[{"Effect":"Deny","Action":"glue:*","Principal":"*","Resource":"*"}]}`
	for _, p := range []string{ok, list} {
		if !allowsRAMShare(p) {
			t.Errorf("RAM share statement not found in %s", p)
		}
	}
	for _, p := range []string{deny, "not json"} {
		if allowsRAMShare(p) {
			t.Errorf("no RAM share statement in %s", p)
		}
	}
}
