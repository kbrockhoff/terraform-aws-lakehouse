package ops

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lakeformation"
	lftypes "github.com/aws/aws-sdk-go-v2/service/lakeformation/types"

	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/classify"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/manifest"
)

// Every state-changing operation returns a Cleanup that undoes it when it was accepted, so an
// unexpected success never leaves a grant, tag, or filter behind. Cleanups run as the runner,
// not the persona, because the persona may lack the permission to undo.

func tableResource(t manifest.Target) *lftypes.Resource {
	return &lftypes.Resource{Table: &lftypes.TableResource{
		CatalogId: aws.String(t.CatalogID), DatabaseName: aws.String(t.Database), Name: aws.String(t.Table),
	}}
}

func principal(arn string) *lftypes.DataLakePrincipal {
	return &lftypes.DataLakePrincipal{DataLakePrincipalIdentifier: aws.String(arn)}
}

func perms(ps ...lftypes.Permission) []lftypes.Permission { return ps }

// accepted marks a successful mutation and attaches its undo.
func accepted(undo func(context.Context) error) Result {
	return Result{Outcome: classify.Allowed, Cleanup: undo}
}

// Grant grants SELECT on the target table to grantee. runner undoes it.
func (c *Clients) Grant(ctx context.Context, runner *Clients, t manifest.Target, grantee string) Result {
	in := &lakeformation.GrantPermissionsInput{Principal: principal(grantee), Resource: tableResource(t), Permissions: perms(lftypes.PermissionSelect)}
	if _, err := c.LF.GrantPermissions(ctx, in); err != nil {
		return fromErr(err, classify.Exists{Resource: t.Exists})
	}
	return accepted(func(ctx context.Context) error {
		_, err := runner.LF.RevokePermissions(ctx, &lakeformation.RevokePermissionsInput{
			Principal: in.Principal, Resource: in.Resource, Permissions: in.Permissions,
		})
		return err
	})
}

// Revoke first grants SELECT to grantee as the runner, then attempts the revoke as the persona.
// The runner's grant is always revoked afterwards.
func (c *Clients) Revoke(ctx context.Context, runner *Clients, t manifest.Target, grantee string) Result {
	setup := &lakeformation.GrantPermissionsInput{Principal: principal(grantee), Resource: tableResource(t), Permissions: perms(lftypes.PermissionSelect)}
	if _, err := runner.LF.GrantPermissions(ctx, setup); err != nil {
		return Result{Outcome: classify.Error, Evidence: classify.Evidence{Message: "revoke setup grant failed: " + err.Error()}}
	}
	undo := func(ctx context.Context) error {
		_, err := runner.LF.RevokePermissions(ctx, &lakeformation.RevokePermissionsInput{
			Principal: setup.Principal, Resource: setup.Resource, Permissions: setup.Permissions,
		})
		return ignoreNotGranted(err)
	}
	_, err := c.LF.RevokePermissions(ctx, &lakeformation.RevokePermissionsInput{
		Principal: setup.Principal, Resource: setup.Resource, Permissions: setup.Permissions,
	})
	res := fromErr(err, classify.Exists{Resource: t.Exists})
	res.Cleanup = undo
	return res
}

// AddLFTags assigns tagKey=yes to the target table. The fixture's metadata-writer tag is the
// target, so an accepted call on its own table changes nothing; anywhere else it is removed.
func (c *Clients) AddLFTags(ctx context.Context, runner *Clients, t manifest.Target, tagKey string, alreadyAssigned bool) Result {
	pair := []lftypes.LFTagPair{{TagKey: aws.String(tagKey), TagValues: []string{"yes"}}}
	out, err := c.LF.AddLFTagsToResource(ctx, &lakeformation.AddLFTagsToResourceInput{Resource: tableResource(t), LFTags: pair})
	if err == nil && out != nil && len(out.Failures) > 0 && out.Failures[0].Error != nil {
		e := out.Failures[0].Error
		return Result{Outcome: classify.Denied, Evidence: classify.Evidence{Code: aws.ToString(e.ErrorCode), Message: aws.ToString(e.ErrorMessage)}}
	}
	if err != nil {
		return fromErr(err, classify.Exists{Resource: t.Exists})
	}
	if alreadyAssigned {
		return Result{Outcome: classify.Allowed}
	}
	return accepted(func(ctx context.Context) error {
		_, err := runner.LF.RemoveLFTagsFromResource(ctx, &lakeformation.RemoveLFTagsFromResourceInput{Resource: tableResource(t), LFTags: pair})
		return err
	})
}

// CreateLFTag creates a throwaway tag key.
func (c *Clients) CreateLFTag(ctx context.Context, runner *Clients, key string) Result {
	if _, err := c.LF.CreateLFTag(ctx, &lakeformation.CreateLFTagInput{TagKey: aws.String(key), TagValues: []string{"x"}}); err != nil {
		return fromErr(err, classify.Exists{Resource: true})
	}
	return accepted(func(ctx context.Context) error {
		_, err := runner.LF.DeleteLFTag(ctx, &lakeformation.DeleteLFTagInput{TagKey: aws.String(key)})
		return err
	})
}

func filterInput(t manifest.Target, name, expression string, excluded []string) *lakeformation.CreateDataCellsFilterInput {
	return &lakeformation.CreateDataCellsFilterInput{TableData: &lftypes.DataCellsFilter{
		TableCatalogId: aws.String(t.CatalogID),
		DatabaseName:   aws.String(t.Database),
		TableName:      aws.String(t.Table),
		Name:           aws.String(name),
		RowFilter:      &lftypes.RowFilter{FilterExpression: aws.String(expression)},
		ColumnWildcard: &lftypes.ColumnWildcard{ExcludedColumnNames: excluded},
	}}
}

// CreateFilter creates a throwaway data cells filter on the target table.
func (c *Clients) CreateFilter(ctx context.Context, runner *Clients, t manifest.Target, name, expression string) Result {
	if _, err := c.LF.CreateDataCellsFilter(ctx, filterInput(t, name, expression, nil)); err != nil {
		return fromErr(err, classify.Exists{Resource: t.Exists})
	}
	return accepted(func(ctx context.Context) error {
		_, err := runner.LF.DeleteDataCellsFilter(ctx, &lakeformation.DeleteDataCellsFilterInput{
			TableCatalogId: aws.String(t.CatalogID), DatabaseName: aws.String(t.Database), TableName: aws.String(t.Table), Name: aws.String(name),
		})
		return err
	})
}

// GrantColumnsWithWrite attempts SELECT on a column subset together with INSERT, which the
// permission service must reject (FR-021).
func (c *Clients) GrantColumnsWithWrite(ctx context.Context, runner *Clients, t manifest.Target, grantee string, columns []string) Result {
	return c.grantColumns(ctx, runner, t, grantee, &lftypes.TableWithColumnsResource{
		CatalogId: aws.String(t.CatalogID), DatabaseName: aws.String(t.Database), Name: aws.String(t.Table), ColumnNames: columns,
	}, perms(lftypes.PermissionSelect, lftypes.PermissionInsert))
}

// GrantExcludePartition attempts column-restricted SELECT that excludes a partition column, which
// the permission service must reject (FR-021).
func (c *Clients) GrantExcludePartition(ctx context.Context, runner *Clients, t manifest.Target, grantee, partition string) Result {
	return c.grantColumns(ctx, runner, t, grantee, &lftypes.TableWithColumnsResource{
		CatalogId: aws.String(t.CatalogID), DatabaseName: aws.String(t.Database), Name: aws.String(t.Table),
		ColumnWildcard: &lftypes.ColumnWildcard{ExcludedColumnNames: []string{partition}},
	}, perms(lftypes.PermissionSelect))
}

func (c *Clients) grantColumns(ctx context.Context, runner *Clients, t manifest.Target, grantee string, r *lftypes.TableWithColumnsResource, ps []lftypes.Permission) Result {
	in := &lakeformation.GrantPermissionsInput{Principal: principal(grantee), Resource: &lftypes.Resource{TableWithColumns: r}, Permissions: ps}
	if _, err := c.LF.GrantPermissions(ctx, in); err != nil {
		return fromErr(err, classify.Exists{Resource: t.Exists})
	}
	return accepted(func(ctx context.Context) error {
		_, err := runner.LF.RevokePermissions(ctx, &lakeformation.RevokePermissionsInput{Principal: in.Principal, Resource: in.Resource, Permissions: ps})
		return err
	})
}

// CreateFilterMapColumn attempts a row filter on a map column, which the permission service must
// reject (FR-021).
func (c *Clients) CreateFilterMapColumn(ctx context.Context, runner *Clients, t manifest.Target, name, mapColumn string) Result {
	return c.CreateFilter(ctx, runner, t, name, mapColumn+"['k'] = 'v'")
}

// ListPermissions lists every grant visible to the caller.
func (c *Clients) ListPermissions(ctx context.Context, in *lakeformation.ListPermissionsInput) ([]lftypes.PrincipalResourcePermissions, error) {
	var out []lftypes.PrincipalResourcePermissions
	p := lakeformation.NewListPermissionsPaginator(c.LF, in)
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, page.PrincipalResourcePermissions...)
	}
	return out, nil
}

// ListPermissionsOp is the matrix operation: can the caller list grants at all.
func (c *Clients) ListPermissionsOp(ctx context.Context) Result {
	if _, err := c.ListPermissions(ctx, &lakeformation.ListPermissionsInput{MaxResults: aws.Int32(100)}); err != nil {
		return fromErr(err, classify.Exists{Resource: true})
	}
	return Result{Outcome: classify.Allowed}
}

// GetSettings reads the data-lake settings.
func (c *Clients) GetSettings(ctx context.Context) (*lftypes.DataLakeSettings, Result) {
	out, err := c.LF.GetDataLakeSettings(ctx, &lakeformation.GetDataLakeSettingsInput{})
	if err != nil {
		return nil, fromErr(err, classify.Exists{Resource: true})
	}
	return out.DataLakeSettings, Result{Outcome: classify.Allowed}
}

// PutSettings writes back the settings the runner just read, so an accepted call changes
// nothing.
func (c *Clients) PutSettings(ctx context.Context, current *lftypes.DataLakeSettings) Result {
	_, err := c.LF.PutDataLakeSettings(ctx, &lakeformation.PutDataLakeSettingsInput{DataLakeSettings: current})
	return fromErr(err, classify.Exists{Resource: true})
}

func ignoreNotGranted(err error) error {
	if err == nil {
		return nil
	}
	if o, _ := classify.Classify(err, classify.Exists{}); o == classify.Rejected {
		return nil // already revoked by the persona
	}
	return err
}
