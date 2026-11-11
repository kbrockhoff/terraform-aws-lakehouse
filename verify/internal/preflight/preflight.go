package preflight

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/lakeformation"
	lftypes "github.com/aws/aws-sdk-go-v2/service/lakeformation/types"
)

// ErrRefused means a safety rule stopped the run. The CLI exits 2 for it.
var ErrRefused = errors.New("preflight refused")

func refuse(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrRefused, fmt.Sprintf(format, a...))
}

// LFAPI is the Lake Formation subset preflight and cleanup need.
type LFAPI interface {
	GetDataLakeSettings(ctx context.Context, in *lakeformation.GetDataLakeSettingsInput, optFns ...func(*lakeformation.Options)) (*lakeformation.GetDataLakeSettingsOutput, error)
	PutDataLakeSettings(ctx context.Context, in *lakeformation.PutDataLakeSettingsInput, optFns ...func(*lakeformation.Options)) (*lakeformation.PutDataLakeSettingsOutput, error)
	ListPermissions(ctx context.Context, in *lakeformation.ListPermissionsInput, optFns ...func(*lakeformation.Options)) (*lakeformation.ListPermissionsOutput, error)
}

// IAMAPI is the IAM subset the probe runner check needs.
type IAMAPI interface {
	SimulatePrincipalPolicy(ctx context.Context, in *iam.SimulatePrincipalPolicyInput, optFns ...func(*iam.Options)) (*iam.SimulatePrincipalPolicyOutput, error)
}

// LoadAllowList reads "<account> <region>" lines from path plus comma-separated entries from
// LFVERIFY_SANDBOX_ACCOUNTS.
func LoadAllowList(path string) (map[string]bool, error) {
	out := map[string]bool{}
	if path != "" {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if e := parseEntry(sc.Text()); e != "" {
				out[e] = true
			}
		}
		if err := sc.Err(); err != nil {
			return nil, err
		}
	}
	for _, e := range strings.Split(os.Getenv("LFVERIFY_SANDBOX_ACCOUNTS"), ",") {
		if e := parseEntry(e); e != "" {
			out[e] = true
		}
	}
	return out, nil
}

func parseEntry(line string) string {
	if i := strings.Index(line, "#"); i >= 0 {
		line = line[:i]
	}
	f := strings.Fields(line)
	if len(f) != 2 {
		return ""
	}
	return f[0] + " " + f[1]
}

// productionEnvs are environment types full mode refuses (FR-003).
var productionEnvs = []string{"Production", "MissionCritical"}

// FullInput is what full-mode preflight checks.
type FullInput struct {
	Account, Region string
	Runner          string // role ARN of the runner
	Env             string // LFVERIFY_ENV, empty when unset
	AllowList       map[string]bool
	// FixtureAdmins are admins the fixture itself manages (runner and the admin persona); they
	// may already be present when preflight reruns against an applied fixture
	FixtureAdmins []string
}

// Full checks that full mode may run and returns the current settings, which the caller saves
// so cleanup can restore them.
func Full(ctx context.Context, lf LFAPI, in FullInput) (*lftypes.DataLakeSettings, error) {
	if !in.AllowList[in.Account+" "+in.Region] {
		return nil, refuse("account %s in %s is not in the full-mode allow-list (verify/sandbox-accounts.txt)", in.Account, in.Region)
	}
	if slices.Contains(productionEnvs, in.Env) {
		return nil, refuse("LFVERIFY_ENV=%s; full mode never runs against production", in.Env)
	}
	out, err := lf.GetDataLakeSettings(ctx, &lakeformation.GetDataLakeSettingsInput{})
	if err != nil {
		return nil, fmt.Errorf("read data-lake settings: %w", err)
	}
	allowed := append([]string{in.Runner}, in.FixtureAdmins...)
	var foreign []string
	for _, a := range out.DataLakeSettings.DataLakeAdmins {
		if id := aws.ToString(a.DataLakePrincipalIdentifier); !slices.Contains(allowed, id) {
			foreign = append(foreign, id)
		}
	}
	if len(foreign) > 0 {
		return nil, refuse("data-lake admins include %v: another deployment owns the settings in this account and Region", foreign)
	}
	return out.DataLakeSettings, nil
}

// SaveSettings writes settings as JSON for cleanup.
func SaveSettings(path string, s *lftypes.DataLakeSettings) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// LoadSettings reads settings saved by SaveSettings.
func LoadSettings(path string) (*lftypes.DataLakeSettings, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s lftypes.DataLakeSettings
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// MutatingActions are the actions a probe runner must not be able to call (FR-027).
var MutatingActions = []string{
	"lakeformation:GrantPermissions", "lakeformation:RevokePermissions", "lakeformation:BatchGrantPermissions",
	"lakeformation:BatchRevokePermissions", "lakeformation:PutDataLakeSettings", "lakeformation:AddLFTagsToResource",
	"lakeformation:RemoveLFTagsFromResource", "lakeformation:CreateLFTag", "lakeformation:UpdateLFTag",
	"lakeformation:DeleteLFTag", "lakeformation:CreateDataCellsFilter", "lakeformation:DeleteDataCellsFilter",
	"lakeformation:RegisterResource", "lakeformation:DeregisterResource",
	"glue:CreateTable", "glue:UpdateTable", "glue:DeleteTable", "glue:CreateDatabase", "glue:UpdateDatabase",
	"glue:DeleteDatabase", "glue:UpdateCatalog", "glue:DeleteCatalog",
	"s3tables:PutTableData", "s3tables:CreateTable", "s3tables:DeleteTable", "s3tables:CreateNamespace",
	"s3tables:DeleteNamespace", "s3tables:PutTableBucketPolicy",
	"iam:PutRolePolicy", "iam:AttachRolePolicy", "iam:UpdateAssumeRolePolicy", "iam:CreateRole", "iam:DeleteRole",
	"athena:UpdateWorkGroup", "athena:DeleteWorkGroup",
	"s3:PutBucketPolicy", "s3:DeleteObject", "s3:DeleteBucket",
	"kms:ScheduleKeyDeletion", "kms:PutKeyPolicy",
}

// ProbeInput is what probe-mode preflight checks.
type ProbeInput struct {
	Runner string // role ARN of the runner
}

// Probe checks that the runner is read-only by construction: IAM simulation denies every
// mutating action, and the runner is a Lake Formation read-only admin.
func Probe(ctx context.Context, iamc IAMAPI, lf LFAPI, in ProbeInput) error {
	var allowed []string
	for chunk := range slices.Chunk(MutatingActions, 20) {
		out, err := iamc.SimulatePrincipalPolicy(ctx, &iam.SimulatePrincipalPolicyInput{
			PolicySourceArn: aws.String(in.Runner), ActionNames: chunk,
		})
		if err != nil {
			return fmt.Errorf("simulate runner policy: %w", err)
		}
		for _, r := range out.EvaluationResults {
			if r.EvalDecision == iamtypes.PolicyEvaluationDecisionTypeAllowed {
				allowed = append(allowed, aws.ToString(r.EvalActionName))
			}
		}
	}
	if len(allowed) > 0 {
		return refuse("probe runner %s can call mutating actions %v; probe mode needs a read-only runner", in.Runner, allowed)
	}
	out, err := lf.GetDataLakeSettings(ctx, &lakeformation.GetDataLakeSettingsInput{})
	if err != nil {
		return fmt.Errorf("read data-lake settings: %w", err)
	}
	for _, a := range out.DataLakeSettings.ReadOnlyAdmins {
		if aws.ToString(a.DataLakePrincipalIdentifier) == in.Runner {
			return nil
		}
	}
	return refuse("probe runner %s is not a Lake Formation read-only admin (governance_config.read_only_admin_principal_arns)", in.Runner)
}

// Database is a catalog and database pair checked for open defaults.
type Database struct{ CatalogID, Name string }

// NoOpenDefaults fails when IAM_ALLOWED_PRINCIPALS holds any permission on a database, because
// identity policies alone would then decide access and every case would be meaningless (FR-008).
func NoOpenDefaults(ctx context.Context, lf LFAPI, dbs []Database) error {
	var open []string
	for _, db := range dbs {
		in := &lakeformation.ListPermissionsInput{
			Principal: &lftypes.DataLakePrincipal{DataLakePrincipalIdentifier: aws.String("IAM_ALLOWED_PRINCIPALS")},
			Resource: &lftypes.Resource{Database: &lftypes.DatabaseResource{
				CatalogId: aws.String(db.CatalogID), Name: aws.String(db.Name),
			}},
		}
		for {
			out, err := lf.ListPermissions(ctx, in)
			if err != nil {
				return fmt.Errorf("list open defaults on %s/%s: %w", db.CatalogID, db.Name, err)
			}
			if len(out.PrincipalResourcePermissions) > 0 {
				open = append(open, db.CatalogID+"/"+db.Name)
				break
			}
			if out.NextToken == nil {
				break
			}
			in.NextToken = out.NextToken
		}
	}
	if len(open) > 0 {
		return fmt.Errorf("IAM_ALLOWED_PRINCIPALS holds permissions on %v; identity policies alone would decide access (FR-008)", open)
	}
	return nil
}
