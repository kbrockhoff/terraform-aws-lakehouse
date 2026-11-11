package classify

import (
	"errors"
	"testing"

	"github.com/aws/smithy-go"
)

func apiErr(code, msg string) error {
	return &smithy.GenericAPIError{Code: code, Message: msg}
}

func TestClassify(t *testing.T) {
	both := Exists{Resource: true, Column: true}
	cases := []struct {
		name   string
		err    error
		exists Exists
		want   Outcome
	}{
		{"nil is allowed", nil, both, Allowed},
		{"access denied exception", apiErr("AccessDeniedException", "Insufficient Lake Formation permission(s) on orders"), both, Denied},
		{"s3 access denied", apiErr("AccessDenied", "Access Denied"), both, Denied},
		{"invalid input is a rejection", apiErr("InvalidInputException", "Column filtering is not allowed on partition columns"), both, Rejected},
		{"invalid input about permissions is a denial", apiErr("InvalidInputException", "Insufficient Lake Formation permission(s): Required Grantable"), both, Denied},
		{"not found on existing table is denial", apiErr("EntityNotFoundException", "Table orders not found"), both, Denied},
		{"not found on missing table is error", apiErr("EntityNotFoundException", "Table orders not found"), Exists{}, Error},
		{"other api error", apiErr("ThrottlingException", "Rate exceeded"), both, Error},
		{"plain error", errors.New("dial tcp: timeout"), both, Error},
	}
	for _, c := range cases {
		got, ev := Classify(c.err, c.exists)
		if got != c.want {
			t.Errorf("%s: got %s, want %s (evidence %+v)", c.name, got, c.want, ev)
		}
	}
}

func TestQueryFailure(t *testing.T) {
	both := Exists{Resource: true, Column: true}
	cases := []struct {
		name   string
		reason string
		exists Exists
		want   Outcome
		code   string
	}{
		{"lf denial", "Insufficient Lake Formation permission(s) on orders (Service: LakeFormation)", both, Denied, ""},
		{"table hidden", "TABLE_NOT_FOUND: line 1:15: Table 'awsdatacatalog.alpha.orders' does not exist", both, Denied, "TABLE_NOT_FOUND"},
		{"schema hidden", "SCHEMA_NOT_FOUND: line 1:15: Schema 'gamma' does not exist", both, Denied, "SCHEMA_NOT_FOUND"},
		{"column hidden", "COLUMN_NOT_FOUND: line 1:8: Column 'ssn' cannot be resolved", both, Denied, "COLUMN_NOT_FOUND"},
		{"column truly missing", "COLUMN_NOT_FOUND: line 1:8: Column 'nope' cannot be resolved", Exists{Resource: true}, Error, "COLUMN_NOT_FOUND"},
		{"catalog hidden", "CATALOG_NOT_FOUND: line 1:15: Catalog 's3tablescatalog/lfv-1-curated' not found", both, Denied, "CATALOG_NOT_FOUND"},
		{"no privilege", "Insufficient permissions to execute the query. Principal does not have any privilege on specified resource", both, Denied, ""},
		{"table not found in database", "Table customers not found in database alpha", both, Denied, ""},
		{"hive database missing", "FAILED: SemanticException [Error 10072]: Database does not exist: alpha", both, Denied, ""},
		{"engine error", "INTERNAL_ERROR_QUERY_ENGINE: Amazon Athena experienced an internal error", both, Error, "INTERNAL_ERROR_QUERY_ENGINE"},
	}
	for _, c := range cases {
		got, ev := QueryFailure(c.reason, c.exists)
		if got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
		if ev.Code != c.code {
			t.Errorf("%s: code %q, want %q", c.name, ev.Code, c.code)
		}
	}
}
