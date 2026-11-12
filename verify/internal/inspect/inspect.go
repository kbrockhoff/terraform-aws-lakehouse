package inspect

import (
	"context"
	"maps"
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

// Tags are a table's effective LF-tags (inherited ones included) and, per column, the tags
// assigned to that column, which override the table's for the same key.
type Tags struct {
	Table   map[string]string
	Columns map[string]map[string]string
}

// Effective returns the table-level permissions that grants give on ref. Named table,
// all-tables (wildcard), column, and data cells filter grants in ref's own catalog count. So do
// LF-tag policy grants on TABLE whose every expression term matches the table's tags or one
// column's effective tags: Lake Formation then grants the permission on the matching columns,
// so a column-restricted reader still holds SELECT. A resource that lacks a key in an
// expression never matches. Database-level permissions are not table permissions and are
// ignored.
func Effective(grants []lftypes.PrincipalResourcePermissions, ref TableRef, tags Tags) []string {
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
			// All-tables SELECT is listed as a column grant on the table name ALL_TABLES
			name := aws.ToString(t.Name)
			if sameCatalog(aws.ToString(t.CatalogId), ref) && aws.ToString(t.DatabaseName) == ref.Database && (name == ref.Table || name == "ALL_TABLES") {
				add(g.Permissions)
			}
		case r.DataCellsFilter != nil:
			f := r.DataCellsFilter
			if sameCatalog(aws.ToString(f.TableCatalogId), ref) && aws.ToString(f.DatabaseName) == ref.Database && aws.ToString(f.TableName) == ref.Table {
				add(g.Permissions)
			}
		case r.LFTagPolicy != nil:
			p := r.LFTagPolicy
			if p.ResourceType == lftypes.ResourceTypeTable && tagPolicyCatalog(aws.ToString(p.CatalogId), ref) && tags.match(p.Expression) {
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

// sameCatalog is for named table, column, and filter grants: the catalogs must be the same,
// with an empty ID meaning the account's default catalog. A default-catalog alpha.orders is a
// different table from alpha.orders in an S3 Tables catalog.
func sameCatalog(grant string, ref TableRef) bool {
	resolve := func(id string) string {
		if id == "" {
			return ref.AccountID
		}
		return id
	}
	return resolve(grant) == resolve(ref.CatalogID)
}

// tagPolicyCatalog is for LF-tag policy grants, which use the account-level catalog ID and cover
// every catalog in the account, S3 Tables catalogs included.
func tagPolicyCatalog(grant string, ref TableRef) bool {
	return grant == "" || grant == ref.CatalogID ||
		(grant == ref.AccountID && strings.HasPrefix(ref.CatalogID, ref.AccountID))
}

// match reports whether expr matches the table or any one column.
func (t Tags) match(expr []lftypes.LFTag) bool {
	if matches(expr, t.Table) {
		return true
	}
	for _, col := range t.Columns {
		eff := maps.Clone(t.Table)
		if eff == nil {
			eff = map[string]string{}
		}
		maps.Copy(eff, col)
		if matches(expr, eff) {
			return true
		}
	}
	return false
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
	// Two listings, because neither alone is complete (observed in probe run 043438):
	// - the unfiltered listing has the LF-tag policy grants but omits table-level grants in
	//   federated S3 Tables catalogs, and accepts no principal filter without a resource;
	// - the table listing has the principal's named and all-tables grants on the table.
	var grants []lftypes.PrincipalResourcePermissions
	keep := func(in *lakeformation.ListPermissionsInput) error {
		p := lakeformation.NewListPermissionsPaginator(i.LF, in)
		for p.HasMorePages() {
			page, err := p.NextPage(ctx)
			if err != nil {
				return err
			}
			for _, g := range page.PrincipalResourcePermissions {
				if g.Principal != nil && aws.ToString(g.Principal.DataLakePrincipalIdentifier) == principal {
					grants = append(grants, g)
				}
			}
		}
		return nil
	}
	if err := keep(&lakeformation.ListPermissionsInput{}); err != nil {
		return nil, err
	}
	if err := keep(&lakeformation.ListPermissionsInput{
		Principal: &lftypes.DataLakePrincipal{DataLakePrincipalIdentifier: aws.String(principal)},
		Resource: &lftypes.Resource{Table: &lftypes.TableResource{
			CatalogId: aws.String(ref.CatalogID), DatabaseName: aws.String(ref.Database), Name: aws.String(ref.Table),
		}},
	}); err != nil {
		return nil, err
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
	tags := Tags{Table: map[string]string{}, Columns: map[string]map[string]string{}}
	for _, t := range append(out.LFTagOnDatabase, out.LFTagsOnTable...) {
		if len(t.TagValues) > 0 {
			tags.Table[aws.ToString(t.TagKey)] = t.TagValues[0]
		}
	}
	for _, c := range out.LFTagsOnColumns {
		col := map[string]string{}
		for _, t := range c.LFTags {
			if len(t.TagValues) > 0 {
				col[aws.ToString(t.TagKey)] = t.TagValues[0]
			}
		}
		tags.Columns[aws.ToString(c.Name)] = col
	}
	return Effective(grants, ref, tags), nil
}
