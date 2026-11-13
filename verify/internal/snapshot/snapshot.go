package snapshot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/glue"
	"github.com/aws/aws-sdk-go-v2/service/lakeformation"
	lftypes "github.com/aws/aws-sdk-go-v2/service/lakeformation/types"
)

// State is the part of a target that probe mode must never change (Non-interference SLO).
// Every field is a sorted list of canonical strings, so two snapshots compare line by line.
type State struct {
	Databases   []string `json:"databases"`
	Tables      []string `json:"tables"`
	LFTags      []string `json:"lf_tags"`
	Assignments []string `json:"tag_assignments"`
	Filters     []string `json:"data_cells_filters"`
	Permissions []string `json:"permissions"`
	Settings    []string `json:"settings"`
}

// Catalog is one catalog and the databases in it to capture.
type Catalog struct {
	CatalogID string
	Databases []string
}

// Capture reads the target state as the runner (a read-only admin).
func Capture(ctx context.Context, g *glue.Client, lf *lakeformation.Client, catalogs []Catalog) (*State, error) {
	s := &State{}
	for _, c := range catalogs {
		for _, db := range c.Databases {
			d, err := g.GetDatabase(ctx, &glue.GetDatabaseInput{CatalogId: aws.String(c.CatalogID), Name: aws.String(db)})
			if err != nil {
				return nil, fmt.Errorf("database %s/%s: %w", c.CatalogID, db, err)
			}
			s.Databases = append(s.Databases, c.CatalogID+"/"+aws.ToString(d.Database.Name)+" "+canon(d.Database.Parameters))
			// The database's own labels and grants: a relabel or grant on a database with no
			// tables would otherwise go unseen
			dbRes := &lftypes.Resource{Database: &lftypes.DatabaseResource{CatalogId: aws.String(c.CatalogID), Name: aws.String(db)}}
			dbTags, err := lf.GetResourceLFTags(ctx, &lakeformation.GetResourceLFTagsInput{Resource: dbRes, ShowAssignedLFTags: aws.Bool(true)})
			if err != nil {
				return nil, fmt.Errorf("database %s/%s labels: %w", c.CatalogID, db, err)
			}
			s.Assignments = append(s.Assignments, fmt.Sprintf("%s/%s %s", c.CatalogID, db, canon(normalTags(dbTags))))
			if err := s.addPermissions(ctx, lf, dbRes); err != nil {
				return nil, err
			}
			p := glue.NewGetTablesPaginator(g, &glue.GetTablesInput{CatalogId: aws.String(c.CatalogID), DatabaseName: aws.String(db)})
			for p.HasMorePages() {
				page, err := p.NextPage(ctx)
				if err != nil {
					return nil, err
				}
				for _, t := range page.TableList {
					var cols []string
					if t.StorageDescriptor != nil {
						for _, col := range t.StorageDescriptor.Columns {
							cols = append(cols, aws.ToString(col.Name)+":"+aws.ToString(col.Type))
						}
					}
					// metadata_location changes on every commit, so it reveals any write
					s.Tables = append(s.Tables, fmt.Sprintf("%s/%s.%s cols=%v metadata=%s",
						c.CatalogID, db, aws.ToString(t.Name), cols, t.Parameters["metadata_location"]))
					tags, err := lf.GetResourceLFTags(ctx, &lakeformation.GetResourceLFTagsInput{
						Resource: &lftypes.Resource{Table: &lftypes.TableResource{
							CatalogId: aws.String(c.CatalogID), DatabaseName: aws.String(db), Name: t.Name,
						}},
						ShowAssignedLFTags: aws.Bool(true),
					})
					if err != nil {
						return nil, err
					}
					s.Assignments = append(s.Assignments, fmt.Sprintf("%s/%s.%s %s", c.CatalogID, db, aws.ToString(t.Name), canon(normalTags(tags))))
					// The unfiltered listing below omits table grants in federated catalogs
					if err := s.addPermissions(ctx, lf, &lftypes.Resource{Table: &lftypes.TableResource{
						CatalogId: aws.String(c.CatalogID), DatabaseName: aws.String(db), Name: t.Name,
					}}); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	tp := lakeformation.NewListLFTagsPaginator(lf, &lakeformation.ListLFTagsInput{})
	for tp.HasMorePages() {
		page, err := tp.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, t := range page.LFTags {
			t.TagValues = sortedStrings(t.TagValues)
			s.LFTags = append(s.LFTags, canon(t))
		}
	}
	fp := lakeformation.NewListDataCellsFilterPaginator(lf, &lakeformation.ListDataCellsFilterInput{})
	for fp.HasMorePages() {
		page, err := fp.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, f := range page.DataCellsFilters {
			f.ColumnNames = sortedStrings(f.ColumnNames)
			if f.ColumnWildcard != nil {
				f.ColumnWildcard = &lftypes.ColumnWildcard{ExcludedColumnNames: sortedStrings(f.ColumnWildcard.ExcludedColumnNames)}
			}
			s.Filters = append(s.Filters, canon(f))
		}
	}
	if err := s.addPermissions(ctx, lf, nil); err != nil {
		return nil, err
	}
	set, err := lf.GetDataLakeSettings(ctx, &lakeformation.GetDataLakeSettingsInput{})
	if err != nil {
		return nil, err
	}
	s.Settings = []string{canon(sortedSettings(set.DataLakeSettings))}
	s.sort()
	return s, nil
}

// sortedSettings orders the principal lists, which Lake Formation returns in no stable order
// (observed in probe run 2651ed), so only real changes show up as differences.
func sortedSettings(in *lftypes.DataLakeSettings) *lftypes.DataLakeSettings {
	if in == nil {
		return nil
	}
	out := *in
	byID := func(ps []lftypes.DataLakePrincipal) []lftypes.DataLakePrincipal {
		ps = slices.Clone(ps)
		sort.Slice(ps, func(i, j int) bool {
			return aws.ToString(ps[i].DataLakePrincipalIdentifier) < aws.ToString(ps[j].DataLakePrincipalIdentifier)
		})
		return ps
	}
	out.DataLakeAdmins = byID(in.DataLakeAdmins)
	out.ReadOnlyAdmins = byID(in.ReadOnlyAdmins)
	out.TrustedResourceOwners = slices.Sorted(slices.Values(in.TrustedResourceOwners))
	return &out
}

// addPermissions lists the grants on resource (every grant when nil) in canonical form.
func (s *State) addPermissions(ctx context.Context, lf *lakeformation.Client, resource *lftypes.Resource) error {
	p := lakeformation.NewListPermissionsPaginator(lf, &lakeformation.ListPermissionsInput{Resource: resource})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, g := range page.PrincipalResourcePermissions {
			s.Permissions = append(s.Permissions, canon(NormalGrant(g)))
		}
	}
	return nil
}

// NormalGrant puts a grant in canonical form: no timestamps, and every set-valued list sorted
// (permissions, tag expressions and values, column sets), so the same grant returned in another
// order is not a change. Nothing order-sensitive is touched.
func NormalGrant(g lftypes.PrincipalResourcePermissions) lftypes.PrincipalResourcePermissions {
	g.LastUpdated, g.LastUpdatedBy = nil, nil
	g.Permissions = sortedPerms(g.Permissions)
	g.PermissionsWithGrantOption = sortedPerms(g.PermissionsWithGrantOption)
	if g.Resource == nil {
		return g
	}
	r := *g.Resource
	if r.LFTagPolicy != nil {
		p := *r.LFTagPolicy
		p.Expression = normalExpression(p.Expression)
		r.LFTagPolicy = &p
	}
	if r.LFTag != nil {
		t := *r.LFTag
		t.TagValues = sortedStrings(t.TagValues)
		r.LFTag = &t
	}
	if r.TableWithColumns != nil {
		t := *r.TableWithColumns
		t.ColumnNames = sortedStrings(t.ColumnNames)
		if t.ColumnWildcard != nil {
			t.ColumnWildcard = &lftypes.ColumnWildcard{ExcludedColumnNames: sortedStrings(t.ColumnWildcard.ExcludedColumnNames)}
		}
		r.TableWithColumns = &t
	}
	g.Resource = &r
	return g
}

// normalTags sorts a GetResourceLFTags result: tags by key, values, and columns by name.
func normalTags(o *lakeformation.GetResourceLFTagsOutput) *lakeformation.GetResourceLFTagsOutput {
	if o == nil {
		return nil
	}
	out := *o
	out.LFTagOnDatabase = normalPairs(o.LFTagOnDatabase)
	out.LFTagsOnTable = normalPairs(o.LFTagsOnTable)
	out.LFTagsOnColumns = nil
	for _, c := range o.LFTagsOnColumns {
		c.LFTags = normalPairs(c.LFTags)
		out.LFTagsOnColumns = append(out.LFTagsOnColumns, c)
	}
	sort.Slice(out.LFTagsOnColumns, func(i, j int) bool {
		return aws.ToString(out.LFTagsOnColumns[i].Name) < aws.ToString(out.LFTagsOnColumns[j].Name)
	})
	return &out
}

func normalPairs(in []lftypes.LFTagPair) []lftypes.LFTagPair {
	out := make([]lftypes.LFTagPair, 0, len(in))
	for _, t := range in {
		t.TagValues = sortedStrings(t.TagValues)
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return aws.ToString(out[i].TagKey) < aws.ToString(out[j].TagKey) })
	return out
}

func normalExpression(in []lftypes.LFTag) []lftypes.LFTag {
	out := make([]lftypes.LFTag, 0, len(in))
	for _, t := range in {
		t.TagValues = sortedStrings(t.TagValues)
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return aws.ToString(out[i].TagKey) < aws.ToString(out[j].TagKey) })
	return out
}

func sortedStrings(in []string) []string {
	if in == nil {
		return nil
	}
	return slices.Sorted(slices.Values(in))
}

func sortedPerms(in []lftypes.Permission) []lftypes.Permission {
	if in == nil {
		return nil
	}
	return slices.Sorted(slices.Values(in))
}

func canon(v any) string {
	b, _ := json.Marshal(v) // map keys are sorted by encoding/json
	return string(b)
}

func (s *State) sort() {
	for _, l := range []*[]string{&s.Databases, &s.Tables, &s.LFTags, &s.Assignments, &s.Filters, &s.Permissions, &s.Settings} {
		sort.Strings(*l)
	}
}

// Diff lists every entry added or removed between two snapshots, prefixed + or -.
func Diff(before, after *State) []string {
	var out []string
	pairs := []struct {
		name string
		a, b []string
	}{
		{"database", before.Databases, after.Databases},
		{"table", before.Tables, after.Tables},
		{"lf-tag", before.LFTags, after.LFTags},
		{"tag-assignment", before.Assignments, after.Assignments},
		{"filter", before.Filters, after.Filters},
		{"permission", before.Permissions, after.Permissions},
		{"settings", before.Settings, after.Settings},
	}
	for _, p := range pairs {
		for _, x := range p.a {
			if !slices.Contains(p.b, x) {
				out = append(out, "- "+p.name+" "+x)
			}
		}
		for _, x := range p.b {
			if !slices.Contains(p.a, x) {
				out = append(out, "+ "+p.name+" "+x)
			}
		}
	}
	return out
}

// Save writes a snapshot as JSON.
func (s *State) Save(path string) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// Load reads a snapshot written by Save.
func Load(path string) (*State, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- reads a file the operator names on the command line
	if err != nil {
		return nil, err
	}
	var s State
	return &s, json.Unmarshal(b, &s)
}
