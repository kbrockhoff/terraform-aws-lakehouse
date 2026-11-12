package preflight

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/lakeformation"
	lftypes "github.com/aws/aws-sdk-go-v2/service/lakeformation/types"
)

type fakeLF struct {
	settings *lftypes.DataLakeSettings
	put      *lftypes.DataLakeSettings
	open     map[string]bool
}

func (f *fakeLF) GetDataLakeSettings(context.Context, *lakeformation.GetDataLakeSettingsInput, ...func(*lakeformation.Options)) (*lakeformation.GetDataLakeSettingsOutput, error) {
	return &lakeformation.GetDataLakeSettingsOutput{DataLakeSettings: f.settings}, nil
}

func (f *fakeLF) PutDataLakeSettings(_ context.Context, in *lakeformation.PutDataLakeSettingsInput, _ ...func(*lakeformation.Options)) (*lakeformation.PutDataLakeSettingsOutput, error) {
	f.put = in.DataLakeSettings
	f.settings = in.DataLakeSettings
	return &lakeformation.PutDataLakeSettingsOutput{}, nil
}

func (f *fakeLF) ListPermissions(_ context.Context, in *lakeformation.ListPermissionsInput, _ ...func(*lakeformation.Options)) (*lakeformation.ListPermissionsOutput, error) {
	out := &lakeformation.ListPermissionsOutput{}
	if f.open[aws.ToString(in.Resource.Database.Name)] {
		out.PrincipalResourcePermissions = []lftypes.PrincipalResourcePermissions{{Permissions: []lftypes.Permission{lftypes.PermissionAll}}}
	}
	return out, nil
}

func principals(arns ...string) []lftypes.DataLakePrincipal {
	var out []lftypes.DataLakePrincipal
	for _, a := range arns {
		out = append(out, lftypes.DataLakePrincipal{DataLakePrincipalIdentifier: aws.String(a)})
	}
	return out
}

const runner = "arn:aws:iam::111122223333:role/runner"

func TestLoadAllowList(t *testing.T) {
	p := filepath.Join(t.TempDir(), "accounts.txt")
	os.WriteFile(p, []byte("# comment\n111122223333 us-east-1 # sandbox\n\nbad line here\n"), 0o600)
	t.Setenv("LFVERIFY_SANDBOX_ACCOUNTS", "444455556666 us-west-2, ")
	got, err := LoadAllowList(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got["111122223333 us-east-1"] || !got["444455556666 us-west-2"] {
		t.Errorf("allow-list: %v", got)
	}
}

func TestFull(t *testing.T) {
	allow := map[string]bool{"111122223333 us-east-1": true}
	base := FullInput{Account: "111122223333", Region: "us-east-1", Runner: runner, AllowList: allow}
	ctx := context.Background()

	lf := &fakeLF{settings: &lftypes.DataLakeSettings{DataLakeAdmins: principals(runner)}}
	if _, err := Full(ctx, lf, base); err != nil {
		t.Fatalf("clean sandbox must pass: %v", err)
	}

	cases := map[string]func(*FullInput, *fakeLF){
		"not allow-listed": func(in *FullInput, _ *fakeLF) { in.Region = "eu-west-1" },
		"production":       func(in *FullInput, _ *fakeLF) { in.Env = "Production" },
		"foreign admin": func(_ *FullInput, f *fakeLF) {
			f.settings = &lftypes.DataLakeSettings{DataLakeAdmins: principals(runner, "arn:aws:iam::111122223333:role/other-deployer")}
		},
	}
	for name, mutate := range cases {
		in := base
		f := &fakeLF{settings: &lftypes.DataLakeSettings{DataLakeAdmins: principals(runner)}}
		mutate(&in, f)
		if _, err := Full(ctx, f, in); !errors.Is(err, ErrRefused) {
			t.Errorf("%s: want refusal, got %v", name, err)
		}
	}

	in := base
	in.FixtureAdmins = []string{"arn:aws:iam::111122223333:role/lfv-abc123-admin"}
	lf = &fakeLF{settings: &lftypes.DataLakeSettings{DataLakeAdmins: principals(runner, in.FixtureAdmins[0])}}
	if _, err := Full(ctx, lf, in); err != nil {
		t.Errorf("fixture admins already applied must pass: %v", err)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	s := &lftypes.DataLakeSettings{DataLakeAdmins: principals(runner), ReadOnlyAdmins: principals("arn:aws:iam::111122223333:role/auditor")}
	if err := SaveSettings(p, s); err != nil {
		t.Fatal(err)
	}
	back, err := LoadSettings(p)
	if err != nil {
		t.Fatal(err)
	}
	if aws.ToString(back.ReadOnlyAdmins[0].DataLakePrincipalIdentifier) != "arn:aws:iam::111122223333:role/auditor" {
		t.Errorf("round trip lost read-only admins: %+v", back)
	}
}

type fakeIAM struct{ allow map[string]bool }

func (f *fakeIAM) SimulatePrincipalPolicy(_ context.Context, in *iam.SimulatePrincipalPolicyInput, _ ...func(*iam.Options)) (*iam.SimulatePrincipalPolicyOutput, error) {
	out := &iam.SimulatePrincipalPolicyOutput{}
	for _, a := range in.ActionNames {
		d := iamtypes.PolicyEvaluationDecisionTypeImplicitDeny
		if f.allow[a] {
			d = iamtypes.PolicyEvaluationDecisionTypeAllowed
		}
		out.EvaluationResults = append(out.EvaluationResults, iamtypes.EvaluationResult{EvalActionName: aws.String(a), EvalDecision: d})
	}
	return out, nil
}

func TestProbe(t *testing.T) {
	ctx := context.Background()
	readOnly := &fakeLF{settings: &lftypes.DataLakeSettings{ReadOnlyAdmins: principals(runner)}}
	if err := Probe(ctx, &fakeIAM{}, readOnly, ProbeInput{Runner: runner}); err != nil {
		t.Fatalf("read-only runner must pass: %v", err)
	}
	err := Probe(ctx, &fakeIAM{allow: map[string]bool{"lakeformation:GrantPermissions": true}}, readOnly, ProbeInput{Runner: runner})
	if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "lakeformation:GrantPermissions") {
		t.Errorf("runner that can grant must be refused with the action named, got %v", err)
	}
	notRO := &fakeLF{settings: &lftypes.DataLakeSettings{}}
	if err := Probe(ctx, &fakeIAM{}, notRO, ProbeInput{Runner: runner}); !errors.Is(err, ErrRefused) {
		t.Errorf("runner outside ReadOnlyAdmins must be refused, got %v", err)
	}
}

func TestNoOpenDefaults(t *testing.T) {
	ctx := context.Background()
	dbs := []Database{{"1:s3tablescatalog/b", "n_alpha"}, {"1:s3tablescatalog/b", "n_beta"}}
	if err := NoOpenDefaults(ctx, &fakeLF{}, dbs); err != nil {
		t.Errorf("no open defaults: %v", err)
	}
	if err := NoOpenDefaults(ctx, &fakeLF{open: map[string]bool{"n_alpha": true}}, dbs); err == nil || !strings.Contains(err.Error(), "n_alpha") {
		t.Errorf("open default must fail naming the database, got %v", err)
	}
}

func TestRestoreSettings(t *testing.T) {
	saved := &lftypes.DataLakeSettings{DataLakeAdmins: principals(runner), ReadOnlyAdmins: principals("arn:aws:iam::111122223333:role/auditor")}
	lf := &fakeLF{settings: &lftypes.DataLakeSettings{DataLakeAdmins: principals(runner, "arn:aws:iam::111122223333:role/lfv-abc123-admin")}}
	if err := RestoreSettings(context.Background(), lf, saved); err != nil {
		t.Fatal(err)
	}
	if lf.put != saved {
		t.Error("saved settings were not written back")
	}
	if d := principalDiff(principals("a", "b"), principals("b", "a")); d != "" {
		t.Errorf("order must not matter: %s", d)
	}
	if d := principalDiff(principals("a"), principals("a", "x")); d == "" {
		t.Error("an extra admin must be a difference")
	}
}

func TestSweepRefusesBadPrefix(t *testing.T) {
	for _, p := range []string{"", "lfv-", "prod-abc123", "lfv-ab"} {
		if err := (&Sweeper{Prefix: p}).Sweep(context.Background()); err == nil || !strings.Contains(err.Error(), "refusing") {
			t.Errorf("prefix %q: want refusal, got %v", p, err)
		}
	}
}

func TestRevocableDropsNameFromWildcard(t *testing.T) {
	r := &lftypes.Resource{Table: &lftypes.TableResource{Name: aws.String("ALL_TABLES"), DatabaseName: aws.String("alpha"), TableWildcard: &lftypes.TableWildcard{}}}
	got := revocable(r)
	if got.Table.Name != nil || got.Table.TableWildcard == nil || aws.ToString(got.Table.DatabaseName) != "alpha" {
		t.Errorf("got %+v", got.Table)
	}
	if aws.ToString(r.Table.Name) != "ALL_TABLES" {
		t.Error("the listed resource must not be modified")
	}
	named := &lftypes.Resource{Table: &lftypes.TableResource{Name: aws.String("orders")}}
	if revocable(named) != named {
		t.Error("named table grants pass through")
	}
}

func TestRevocableAllTablesColumnGrant(t *testing.T) {
	r := &lftypes.Resource{TableWithColumns: &lftypes.TableWithColumnsResource{
		CatalogId: aws.String("123:s3tablescatalog/b"), DatabaseName: aws.String("n_alpha"),
		Name: aws.String("ALL_TABLES"), ColumnWildcard: &lftypes.ColumnWildcard{},
	}}
	got := revocable(r)
	if got.TableWithColumns != nil || got.Table == nil || got.Table.TableWildcard == nil || got.Table.Name != nil {
		t.Fatalf("want a table wildcard, got %+v", got)
	}
	if aws.ToString(got.Table.CatalogId) != "123:s3tablescatalog/b" || aws.ToString(got.Table.DatabaseName) != "n_alpha" {
		t.Errorf("catalog or database lost: %+v", got.Table)
	}
	cols := &lftypes.Resource{TableWithColumns: &lftypes.TableWithColumnsResource{Name: aws.String("orders")}}
	if revocable(cols) != cols {
		t.Error("column grants on a named table pass through")
	}
}
