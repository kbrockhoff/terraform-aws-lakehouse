package preflight

import (
	"bufio"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
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
		f, err := os.Open(path) // #nosec G304 -- reads a file the operator names on the command line
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

// Target is the account and Region a full-mode preflight checked. Teardown and cleanup compare
// it with the current credentials, because a settings snapshot alone does not say which account
// it belongs to: restoring it elsewhere would replace that account's data-lake admins.
type Target struct {
	Account string `json:"account"`
	Region  string `json:"region"`
}

// RecordTarget writes the checked target on the first preflight of a run. Later preflights of the
// same run must see the same target.
func RecordTarget(path string, t Target) error {
	if _, err := os.Stat(path); err == nil { // #nosec G703 -- the run's own output directory
		return CheckTarget(path, t)
	}
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600) // #nosec G703 -- the run's own output directory
}

// CheckTarget refuses unless the run's recorded target equals current.
func CheckTarget(path string, current Target) error {
	b, err := os.ReadFile(path) // #nosec G304 -- reads the run's own target record
	if err != nil {
		return refuse("no target record (%s): preflight never passed for this run, so nothing may be torn down or restored", path)
	}
	var t Target
	if err := json.Unmarshal(b, &t); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if t != current {
		return refuse("run belongs to account %s in %s, but the credentials point at account %s in %s", t.Account, t.Region, current.Account, current.Region)
	}
	return nil
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
	b, err := os.ReadFile(path) // #nosec G304 -- reads the run's own settings snapshot
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
	"lakeformation:RegisterResource", "lakeformation:DeregisterResource", "lakeformation:UpdateDataCellsFilter",
	"lakeformation:CreateLFTagExpression", "lakeformation:UpdateLFTagExpression", "lakeformation:DeleteLFTagExpression",
	"glue:CreateTable", "glue:UpdateTable", "glue:DeleteTable", "glue:BatchDeleteTable", "glue:CreateDatabase",
	"glue:UpdateDatabase", "glue:DeleteDatabase", "glue:CreateCatalog", "glue:UpdateCatalog", "glue:DeleteCatalog",
	// UpdateTableMetadataLocation commits Iceberg metadata; the module's data-access role uses it
	"s3tables:PutTableData", "s3tables:UpdateTableMetadataLocation", "s3tables:CreateTable", "s3tables:DeleteTable",
	"s3tables:RenameTable", "s3tables:CreateNamespace", "s3tables:DeleteNamespace", "s3tables:DeleteTableBucket",
	"s3tables:PutTableBucketPolicy", "s3tables:DeleteTableBucketPolicy", "s3tables:PutTablePolicy",
	"s3tables:DeleteTablePolicy", "s3tables:PutTableMaintenanceConfiguration",
	"iam:PutRolePolicy", "iam:AttachRolePolicy", "iam:UpdateAssumeRolePolicy", "iam:CreateRole", "iam:DeleteRole",
	"athena:UpdateWorkGroup", "athena:DeleteWorkGroup",
	"s3:PutBucketPolicy", "s3:DeleteObject", "s3:DeleteBucket",
	"kms:ScheduleKeyDeletion", "kms:PutKeyPolicy", "kms:DisableKey",
}

// ProbeInput is what probe-mode preflight checks.
type ProbeInput struct {
	Runner string // role ARN of the runner
	// Resources are the target's ARNs: persona roles, catalogs, databases, tables, table
	// buckets, workgroups, results buckets, and keys. Each mutating action is simulated against
	// "*" and, separately (IAM rejects a list mixing both), the ARNs of its own service, because
	// a write allowed only on a named resource is an implicit deny for "*".
	Resources []string
}

// Probe checks that the runner is read-only by construction: IAM simulation denies every
// mutating action on every target resource, and the runner is a Lake Formation read-only admin.
func Probe(ctx context.Context, iamc IAMAPI, lf LFAPI, in ProbeInput) error {
	var allowed []string
	byService := map[string][]string{}
	for _, a := range MutatingActions {
		svc, _, _ := strings.Cut(a, ":")
		byService[svc] = append(byService[svc], a)
	}
	for _, svc := range slices.Sorted(maps.Keys(byService)) {
		sets := [][]string{nil} // nil simulates "*"
		if arns := serviceARNs(svc, in.Resources); len(arns) > 0 {
			sets = append(sets, arns)
		}
		for chunk := range slices.Chunk(byService[svc], 20) {
			for _, resources := range sets {
				found, err := simulate(ctx, iamc, in.Runner, chunk, resources)
				if err != nil {
					return err
				}
				allowed = append(allowed, found...)
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

// simulate returns "action on resource" for every allowed pair. It fails closed: an action the
// simulation returns no result for is an error, not a pass.
func simulate(ctx context.Context, iamc IAMAPI, runner string, actions, resources []string) ([]string, error) {
	var allowed []string
	seen := map[string]bool{}
	in := &iam.SimulatePrincipalPolicyInput{PolicySourceArn: aws.String(runner), ActionNames: actions, ResourceArns: resources}
	for {
		out, err := iamc.SimulatePrincipalPolicy(ctx, in)
		if err != nil {
			return nil, fmt.Errorf("simulate runner policy: %w", err)
		}
		// With resource ARNs, IAM returns one result per action, with a decision per resource
		// in ResourceSpecificResults and a resource-name template at the top level
		for _, r := range out.EvaluationResults {
			action := aws.ToString(r.EvalActionName)
			seen[action] = true
			if len(r.ResourceSpecificResults) == 0 {
				if r.EvalDecision == iamtypes.PolicyEvaluationDecisionTypeAllowed {
					allowed = append(allowed, action+" on "+cmp.Or(strings.Join(resources, ", "), "*"))
				}
				continue
			}
			for _, rs := range r.ResourceSpecificResults {
				if rs.EvalResourceDecision == iamtypes.PolicyEvaluationDecisionTypeAllowed {
					allowed = append(allowed, action+" on "+aws.ToString(rs.EvalResourceName))
				}
			}
		}
		if !out.IsTruncated {
			break
		}
		in.Marker = out.Marker
	}
	for _, a := range actions {
		if !seen[a] {
			return nil, fmt.Errorf("simulate runner policy: no result for %s, so read-only access is not established", a)
		}
	}
	return slices.Compact(slices.Sorted(slices.Values(allowed))), nil
}

// serviceARNs returns the ARNs that belong to an IAM service prefix (s3 ARNs have no region or
// account, so arn:<partition>:s3::: matches only S3).
func serviceARNs(service string, arns []string) []string {
	var out []string
	for _, a := range arns {
		if parts := strings.SplitN(a, ":", 4); len(parts) == 4 && parts[2] == service {
			out = append(out, a)
		}
	}
	return out
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
