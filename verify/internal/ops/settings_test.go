package ops

import (
	"slices"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	gluetypes "github.com/aws/aws-sdk-go-v2/service/glue/types"
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
	single := `{"Statement":{"Effect":"Allow","Action":"glue:ShareResource","Principal":{"Service":"ram.amazonaws.com"},"Resource":"*"}}`
	for _, p := range []string{ok, list, single} {
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

func TestGlueParentCatalog(t *testing.T) {
	for in, want := range map[string]string{
		"111122223333:s3tablescatalog/lfv-raw": "s3tablescatalog",
		"111122223333:s3tablescatalog":         "s3tablescatalog",
		"111122223333":                         "111122223333",
	} {
		if got := glueParentCatalog(in); got != want {
			t.Errorf("glueParentCatalog(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCatalogDefaultPermissions(t *testing.T) {
	if got := catalogDefaultPermissions(nil); got != "none" {
		t.Errorf("no default permissions = %q, want none", got)
	}
	perms := []gluetypes.PrincipalPermissions{
		{Principal: &gluetypes.DataLakePrincipal{DataLakePrincipalIdentifier: aws.String("IAM_ALLOWED_PRINCIPALS")}},
	}
	if got := catalogDefaultPermissions(perms); got != "IAM_ALLOWED_PRINCIPALS" {
		t.Errorf("default permissions = %q, want IAM_ALLOWED_PRINCIPALS", got)
	}
}

func TestIdentityCenterMismatches(t *testing.T) {
	const instance = "arn:aws:sso:::instance/ssoins-0123456789abcdef"
	cases := []struct {
		name, live, want string
		mismatch         bool
	}{
		{"expected and present", instance, instance, false},
		{"expected but missing", NotExpected, instance, true},
		{"expected, other instance", "arn:aws:sso:::instance/ssoins-fedcba9876543210", instance, true},
		{"not expected, missing", NotExpected, NotExpected, false},
		{"not expected, present: report only", instance, NotExpected, false},
	}
	for _, c := range cases {
		want := map[string]string{"instance_arn": c.want}
		got := identityCenterMismatches(c.live, want)
		if (len(got) > 0) != c.mismatch {
			t.Errorf("%s: mismatches %v, want mismatch %v", c.name, got, c.mismatch)
		}
		if want["instance_arn"] != c.want {
			t.Errorf("%s: want map was modified", c.name)
		}
	}
	if got := identityCenterMismatches(instance, map[string]string{"instance": instance}); len(got) != 1 {
		t.Errorf("unknown key: mismatches %v, want one", got)
	}
}
