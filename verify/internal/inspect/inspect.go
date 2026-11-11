package inspect

import (
	"context"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lakeformation"
	lftypes "github.com/aws/aws-sdk-go-v2/service/lakeformation/types"
)

// TableRef identifies a table in a catalog.
type TableRef struct {
	AccountID string
	CatalogID string // <account> or <account>:s3tablescatalog/<bucket>
	Database  string
	Table     string
}

// Effective returns the table-level permissions that grants give on ref, given the table's
// effective LF-tags (inherited ones included). Named table, all-tables (wildcard), column, and
// data cells filter grants count, and so do LF-tag policy grants on TABLE whose every
// expression term matches a tag on the table. A resource that lacks a key in an expression
// never matches. Database-level permissions are not table permissions and are ignored.
func Effective(grants []lftypes.PrincipalResourcePermissions, ref TableRef, tags map[string]string) []string {
	set := map[string]bool{}
	add := func(ps []lftypes.Permission) {
		for _, p := range ps {
			set[string(p)] = true
		}
	}
	for _, g := range grants {
		r := g.Resource
		if r == nil {
			continue
		}
		switch {
		case r.Table != nil:
			t := r.Table
			if sameCatalog(aws.ToString(t.CatalogId), ref) && aws.ToString(t.DatabaseName) == ref.Database &&
				(t.TableWildcard != nil || aws.ToString(t.Name) == ref.Table) {
				add(g.Permissions)
			}
		case r.TableWithColumns != nil:
			t := r.TableWithColumns
			if sameCatalog(aws.ToString(t.CatalogId), ref) && aws.ToString(t.DatabaseName) == ref.Database && aws.ToString(t.Name) == ref.Table {
				add(g.Permissions)
			}
		case r.DataCellsFilter != nil:
			f := r.DataCellsFilter
			if sameCatalog(aws.ToString(f.TableCatalogId), ref) && aws.ToString(f.DatabaseName) == ref.Database && aws.ToString(f.TableName) == ref.Table {
				add(g.Permissions)
			}
		case r.LFTagPolicy != nil:
			p := r.LFTagPolicy
			if p.ResourceType == lftypes.ResourceTypeTable && sameCatalog(aws.ToString(p.CatalogId), ref) && matches(p.Expression, tags) {
				add(g.Permissions)
			}
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}

// sameCatalog accepts an exact catalog match, and account-level catalog IDs, which LF-tag policy
// grants use and which cover every S3 Tables catalog in the account.
func sameCatalog(grant string, ref TableRef) bool {
	if grant == "" || grant == ref.CatalogID {
		return true
	}
	return grant == ref.AccountID && strings.HasPrefix(ref.CatalogID, ref.AccountID)
}

func matches(expr []lftypes.LFTag, tags map[string]string) bool {
	if len(expr) == 0 {
		return false
	}
	for _, term := range expr {
		v, ok := tags[aws.ToString(term.TagKey)]
		if !ok || !(slices.Contains(term.TagValues, v) || slices.Contains(term.TagValues, "*")) {
			return false
		}
	}
	return true
}

// Covers reports whether every expected permission is in the effective set.
func Covers(effective, expected []string) bool {
	for _, e := range expected {
		if !slices.Contains(effective, e) && !slices.Contains(effective, "ALL") {
			return false
		}
	}
	return true
}

// LFAPI is the Lake Formation subset inspection needs.
type LFAPI interface {
	lakeformation.ListPermissionsAPIClient
	GetResourceLFTags(ctx context.Context, in *lakeformation.GetResourceLFTagsInput, optFns ...func(*lakeformation.Options)) (*lakeformation.GetResourceLFTagsOutput, error)
}

// Inspector resolves effective permissions as the runner, a read-only admin.
type Inspector struct{ LF LFAPI }

// Permissions returns principal's effective table permissions on ref.
func (i Inspector) Permissions(ctx context.Context, principal string, ref TableRef) ([]string, error) {
	var grants []lftypes.PrincipalResourcePermissions
	p := lakeformation.NewListPermissionsPaginator(i.LF, &lakeformation.ListPermissionsInput{
		Principal: &lftypes.DataLakePrincipal{DataLakePrincipalIdentifier: aws.String(principal)},
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		grants = append(grants, page.PrincipalResourcePermissions...)
	}
	out, err := i.LF.GetResourceLFTags(ctx, &lakeformation.GetResourceLFTagsInput{
		Resource: &lftypes.Resource{Table: &lftypes.TableResource{
			CatalogId: aws.String(ref.CatalogID), DatabaseName: aws.String(ref.Database), Name: aws.String(ref.Table),
		}},
		ShowAssignedLFTags: aws.Bool(false), // include inherited tags
	})
	if err != nil {
		return nil, err
	}
	tags := map[string]string{}
	for _, t := range append(out.LFTagOnDatabase, out.LFTagsOnTable...) {
		if len(t.TagValues) > 0 {
			tags[aws.ToString(t.TagKey)] = t.TagValues[0]
		}
	}
	return Effective(grants, ref, tags), nil
}
