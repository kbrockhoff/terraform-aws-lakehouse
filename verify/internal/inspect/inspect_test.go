package inspect

import (
	"slices"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	lftypes "github.com/aws/aws-sdk-go-v2/service/lakeformation/types"
)

var ref = TableRef{AccountID: "111", CatalogID: "111:s3tablescatalog/curated", Database: "alpha", Table: "orders"}

func tagGrant(perms []lftypes.Permission, expr map[string][]string) lftypes.PrincipalResourcePermissions {
	var e []lftypes.LFTag
	for k, v := range expr {
		e = append(e, lftypes.LFTag{TagKey: aws.String(k), TagValues: v})
	}
	return lftypes.PrincipalResourcePermissions{Permissions: perms, Resource: &lftypes.Resource{LFTagPolicy: &lftypes.LFTagPolicyResource{
		CatalogId: aws.String("111"), ResourceType: lftypes.ResourceTypeTable, Expression: e,
	}}}
}

func TestEffective(t *testing.T) {
	tags := map[string]string{"t-alpha": "member", "zone": "curated", "sensitivity": "restricted"}
	sel := []lftypes.Permission{lftypes.PermissionSelect}
	cases := []struct {
		name   string
		grants []lftypes.PrincipalResourcePermissions
		want   []string
	}{
		{"tag match", []lftypes.PrincipalResourcePermissions{tagGrant(sel, map[string][]string{"t-alpha": {"member"}, "zone": {"curated"}})}, []string{"SELECT"}},
		{"tag value miss", []lftypes.PrincipalResourcePermissions{tagGrant(sel, map[string][]string{"zone": {"raw"}})}, []string{}},
		{"missing key never matches", []lftypes.PrincipalResourcePermissions{tagGrant(sel, map[string][]string{"t-beta": {"member"}})}, []string{}},
		{"named table", []lftypes.PrincipalResourcePermissions{{Permissions: []lftypes.Permission{lftypes.PermissionInsert, lftypes.PermissionSelect},
			Resource: &lftypes.Resource{Table: &lftypes.TableResource{CatalogId: aws.String(ref.CatalogID), DatabaseName: aws.String("alpha"), Name: aws.String("orders")}}}},
			[]string{"INSERT", "SELECT"}},
		{"wildcard other database", []lftypes.PrincipalResourcePermissions{{Permissions: sel,
			Resource: &lftypes.Resource{Table: &lftypes.TableResource{CatalogId: aws.String(ref.CatalogID), DatabaseName: aws.String("beta"), TableWildcard: &lftypes.TableWildcard{}}}}},
			[]string{}},
		{"database grant is not a table grant", []lftypes.PrincipalResourcePermissions{{Permissions: []lftypes.Permission{lftypes.PermissionCreateTable},
			Resource: &lftypes.Resource{Database: &lftypes.DatabaseResource{CatalogId: aws.String(ref.CatalogID), Name: aws.String("alpha")}}}},
			[]string{}},
		{"filter grant", []lftypes.PrincipalResourcePermissions{{Permissions: sel,
			Resource: &lftypes.Resource{DataCellsFilter: &lftypes.DataCellsFilterResource{TableCatalogId: aws.String(ref.CatalogID), DatabaseName: aws.String("alpha"), TableName: aws.String("orders"), Name: aws.String("f")}}}},
			[]string{"SELECT"}},
	}
	for _, c := range cases {
		got := Effective(c.grants, ref, tags)
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestCovers(t *testing.T) {
	if !Covers([]string{"INSERT", "SELECT"}, []string{"SELECT"}) || Covers([]string{"SELECT"}, []string{"SELECT", "INSERT"}) {
		t.Error("covers subset logic")
	}
	if !Covers([]string{"ALL"}, []string{"DROP"}) {
		t.Error("ALL covers everything")
	}
}
