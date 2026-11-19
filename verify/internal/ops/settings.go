package ops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/glue"
	gluetypes "github.com/aws/aws-sdk-go-v2/service/glue/types"
	"github.com/aws/aws-sdk-go-v2/service/lakeformation"
	lftypes "github.com/aws/aws-sdk-go-v2/service/lakeformation/types"

	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/classify"
)

// Check ops (spec 003 FR-020 cases 3 to 5) read account-wide settings and report every expected
// value that differs. They change nothing, so probe mode runs them too.

// compare records a mismatch for each expected key whose actual value differs. Unknown keys are
// mismatches, so a typo never passes silently.
func compare(actual, want map[string]string) []string {
	var out []string
	for _, k := range sortedKeys(want) {
		got, ok := actual[k]
		switch {
		case !ok:
			out = append(out, fmt.Sprintf("%s is not a known setting", k))
		case got != want[k]:
			out = append(out, fmt.Sprintf("%s is %q, want %q", k, got, want[k]))
		}
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// joined renders a list setting as sorted, comma-separated values; "none" when empty.
func joined(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	v := slices.Clone(values)
	sort.Strings(v)
	return strings.Join(v, ",")
}

func defaultPermissions(perms []lftypes.PrincipalPermissions) string {
	var principals []string
	for _, p := range perms {
		if p.Principal != nil {
			principals = append(principals, aws.ToString(p.Principal.DataLakePrincipalIdentifier))
		}
	}
	return joined(principals)
}

func checkResult(mismatches []string) Result {
	return Result{Outcome: classify.Allowed, Mismatches: mismatches}
}

// AccountSettings compares the live data-lake settings with want (FR-003, FR-005). Keys:
// cross_account_version, set_context, allow_external_data_filtering,
// allow_full_table_external_data_access, external_data_filtering_allow_list,
// authorized_session_tag_values, trusted_resource_owners, create_database_default_permissions,
// and create_table_default_permissions. Lists are sorted and comma-joined, "none" when empty.
func (c *Clients) AccountSettings(ctx context.Context, want map[string]string) Result {
	s, res := c.GetSettings(ctx)
	if res.Outcome != classify.Allowed {
		return res
	}
	var allowList []string
	for _, p := range s.ExternalDataFilteringAllowList {
		allowList = append(allowList, aws.ToString(p.DataLakePrincipalIdentifier))
	}
	actual := map[string]string{
		"cross_account_version":                 s.Parameters["CROSS_ACCOUNT_VERSION"],
		"set_context":                           s.Parameters["SET_CONTEXT"],
		"allow_external_data_filtering":         strconv.FormatBool(aws.ToBool(s.AllowExternalDataFiltering)),
		"allow_full_table_external_data_access": strconv.FormatBool(aws.ToBool(s.AllowFullTableExternalDataAccess)),
		"external_data_filtering_allow_list":    joined(allowList),
		"authorized_session_tag_values":         joined(s.AuthorizedSessionTagValueList),
		"trusted_resource_owners":               joined(s.TrustedResourceOwners),
		"create_database_default_permissions":   defaultPermissions(s.CreateDatabaseDefaultPermissions),
		"create_table_default_permissions":      defaultPermissions(s.CreateTableDefaultPermissions),
	}
	return checkResult(compare(actual, want))
}

// CatalogEncryption compares the live Glue Data Catalog encryption with want (FR-009, FR-010).
// Keys: mode, kms_key_arn, service_role ("set" or "none"), and connection_passwords_encrypted.
// An empty catalogID reads the caller's own account.
func (c *Clients) CatalogEncryption(ctx context.Context, catalogID string, want map[string]string) Result {
	in := &glue.GetDataCatalogEncryptionSettingsInput{}
	if catalogID != "" {
		in.CatalogId = aws.String(catalogID)
	}
	out, err := c.Glue.GetDataCatalogEncryptionSettings(ctx, in)
	if err != nil {
		return fromErr(err, classify.Exists{Resource: true})
	}
	actual := map[string]string{"mode": "DISABLED", "kms_key_arn": "none", "service_role": "none", "connection_passwords_encrypted": "false"}
	if s := out.DataCatalogEncryptionSettings; s != nil {
		if e := s.EncryptionAtRest; e != nil {
			actual["mode"] = string(e.CatalogEncryptionMode)
			if k := aws.ToString(e.SseAwsKmsKeyId); k != "" {
				actual["kms_key_arn"] = k
			}
			if aws.ToString(e.CatalogEncryptionServiceRole) != "" {
				actual["service_role"] = "set"
			}
		}
		if p := s.ConnectionPasswordEncryption; p != nil {
			actual["connection_passwords_encrypted"] = strconv.FormatBool(p.ReturnConnectionPasswordEncrypted)
		}
	}
	return checkResult(compare(actual, want))
}

// CatalogResourcePolicy reports an account catalog resource policy that would break Lake
// Formation's RAM shares: one that exists without glue:ShareResource for ram.amazonaws.com
// (FR-006, D1). No policy is the expected default.
func (c *Clients) CatalogResourcePolicy(ctx context.Context) Result {
	out, err := c.Glue.GetResourcePolicy(ctx, &glue.GetResourcePolicyInput{})
	var notFound *gluetypes.EntityNotFoundException
	if errors.As(err, &notFound) {
		return checkResult(nil)
	}
	if err != nil {
		return fromErr(err, classify.Exists{Resource: true})
	}
	if !allowsRAMShare(aws.ToString(out.PolicyInJson)) {
		return checkResult([]string{"the catalog resource policy lacks glue:ShareResource for ram.amazonaws.com, which breaks every cross-account share"})
	}
	return checkResult(nil)
}

// allowsRAMShare reports whether a policy allows glue:ShareResource to ram.amazonaws.com.
func allowsRAMShare(policy string) bool {
	type statement struct {
		Effect    string          `json:"Effect"`
		Action    json.RawMessage `json:"Action"`
		Principal json.RawMessage `json:"Principal"`
	}
	var doc struct {
		Statement json.RawMessage `json:"Statement"`
	}
	if json.Unmarshal([]byte(policy), &doc) != nil {
		return false
	}
	// Statement may be a single object or an array of them
	var stmts []statement
	if json.Unmarshal(doc.Statement, &stmts) != nil {
		var one statement
		if json.Unmarshal(doc.Statement, &one) != nil {
			return false
		}
		stmts = []statement{one}
	}
	for _, s := range stmts {
		if s.Effect == "Allow" && strings.Contains(string(s.Action), "glue:ShareResource") && strings.Contains(string(s.Principal), "ram.amazonaws.com") {
			return true
		}
	}
	return false
}

// Database names one database to check.
type Database struct {
	CatalogID string
	Name      string
}

// NoIAMAllowedPrincipals reports any IAM_ALLOWED_PRINCIPALS grant on the given databases or their
// tables (FR-020 case 5): such a grant gives identity-policy-only access. ListPermissions needs a
// resource whenever it filters by principal, so each database is checked on its own.
func (c *Clients) NoIAMAllowedPrincipals(ctx context.Context, dbs []Database) Result {
	var mismatches []string
	principal := &lftypes.DataLakePrincipal{DataLakePrincipalIdentifier: aws.String("IAM_ALLOWED_PRINCIPALS")}
	for _, db := range dbs {
		for _, res := range []*lftypes.Resource{
			{Database: &lftypes.DatabaseResource{CatalogId: aws.String(db.CatalogID), Name: aws.String(db.Name)}},
			{Table: &lftypes.TableResource{CatalogId: aws.String(db.CatalogID), DatabaseName: aws.String(db.Name), TableWildcard: &lftypes.TableWildcard{}}},
		} {
			in := &lakeformation.ListPermissionsInput{Principal: principal, Resource: res}
			for {
				out, err := c.LF.ListPermissions(ctx, in)
				if err != nil {
					return fromErr(err, classify.Exists{Resource: true})
				}
				for _, p := range out.PrincipalResourcePermissions {
					mismatches = append(mismatches, "IAM_ALLOWED_PRINCIPALS holds "+joinPermissions(p.Permissions)+" on "+catalogOf(p.Resource)+resourceName(p.Resource))
				}
				if out.NextToken == nil {
					break
				}
				in.NextToken = out.NextToken
			}
		}
	}
	return checkResult(mismatches)
}

func catalogOf(r *lftypes.Resource) string {
	switch {
	case r == nil:
		return ""
	case r.Database != nil:
		return aws.ToString(r.Database.CatalogId)
	case r.Table != nil:
		return aws.ToString(r.Table.CatalogId)
	case r.TableWithColumns != nil:
		return aws.ToString(r.TableWithColumns.CatalogId)
	case r.Catalog != nil:
		return aws.ToString(r.Catalog.Id)
	}
	return ""
}

func resourceName(r *lftypes.Resource) string {
	switch {
	case r == nil:
		return ""
	case r.Database != nil:
		return "/" + aws.ToString(r.Database.Name)
	case r.Table != nil:
		return "/" + aws.ToString(r.Table.DatabaseName) + "." + aws.ToString(r.Table.Name)
	case r.TableWithColumns != nil:
		return "/" + aws.ToString(r.TableWithColumns.DatabaseName) + "." + aws.ToString(r.TableWithColumns.Name)
	}
	return ""
}

func joinPermissions(p []lftypes.Permission) string {
	out := make([]string, len(p))
	for i, x := range p {
		out[i] = string(x)
	}
	return strings.Join(out, ",")
}
