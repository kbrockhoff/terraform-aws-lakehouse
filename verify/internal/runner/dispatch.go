package runner

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lakeformation"
	lftypes "github.com/aws/aws-sdk-go-v2/service/lakeformation/types"
	"github.com/aws/aws-sdk-go-v2/service/s3tables"

	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/classify"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/manifest"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/matrix"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/ops"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/predicate"
)

// rowKey is a run-unique key for rows a writer case inserts, so concurrent runs and cases never
// touch each other's rows or the seeded ones (seeded keys are below 1000).
func rowKey(runID, salt string) int64 {
	h := fnv.New32a()
	h.Write([]byte(runID + "/" + salt))
	return 900000 + int64(h.Sum32()%90000)
}

func (r *Runner) cols(table string) ([]ops.Col, error) {
	if r.Manifest == nil {
		return nil, errors.New("write operations need the fixture manifest")
	}
	def, ok := r.Manifest.Tables[table]
	if !ok {
		return nil, fmt.Errorf("table %q is not a fixture table", table)
	}
	out := make([]ops.Col, len(def.Columns))
	for i, c := range def.Columns {
		out[i] = ops.Col{Name: c.Name, Type: c.Type}
	}
	return out, nil
}

func (r *Runner) dispatch(ctx context.Context, pc *ops.Clients, inst matrix.Instance, t manifest.Target) (ops.Result, Observation) {
	c := inst.Case
	exists := classify.Exists{Resource: t.Exists, Column: true}
	salt := inst.Persona.Key + "/" + string(inst.Style) + "/" + c.ID
	var obs Observation
	errResult := func(err error) ops.Result {
		return ops.Result{Outcome: classify.Error, Evidence: classify.Evidence{Message: err.Error()}}
	}

	switch c.Op {
	case matrix.ListCatalogs:
		obs.Name = t.AthenaCatalog()[strings.Index(t.AthenaCatalog(), "/")+1:]
		return pc.ListCatalogs(ctx, t), obs
	case matrix.ListDatabases:
		obs.Name = t.Database
		return pc.ListDatabases(ctx, t), obs
	case matrix.GetDatabase:
		return pc.GetDatabase(ctx, t), obs
	case matrix.ListTables:
		obs.Name = t.Table
		return pc.ListTables(ctx, t), obs
	case matrix.GetTable:
		return pc.GetTable(ctx, t), obs

	case matrix.Select, matrix.SelectColumns:
		limit := 0
		if r.Matrix.Mode == matrix.Probe {
			limit = r.Matrix.Defaults.QueryRowLimit
		}
		var columns []string
		if c.Op == matrix.SelectColumns {
			columns = t.Columns
		}
		opts := ops.QueryOpts{}
		if c.ExpectRows != nil && r.Matrix.Mode == matrix.Full {
			opts.KeyColumn = c.ExpectRows.KeyColumn
		}
		if p := c.RowPredicate; p != nil {
			opts.Visit = func(row map[string]string) {
				ok, err := p.Match(row)
				switch {
				case errors.Is(err, predicate.ErrColumnNotVisible):
					obs.PredicateErr = err
				case err != nil:
					obs.PredicateErr = err
				case !ok:
					obs.PredicateMisses++
				}
			}
		}
		return pc.Query(ctx, t, ops.SelectSQL(t.Database, t.Table, columns, limit), exists, opts), obs
	case matrix.Count:
		return pc.CountRows(ctx, t, exists), obs

	case matrix.Insert, matrix.Update, matrix.Delete:
		cols, err := r.cols(t.Table)
		if err != nil {
			return errResult(err), obs
		}
		key := rowKey(r.RunID, salt)
		undo := func(ctx context.Context) error {
			res := r.Runner.Query(ctx, t, ops.DeleteSQL(t.Database, t.Table, cols, key), classify.Exists{Resource: true}, ops.QueryOpts{})
			if res.Outcome != classify.Allowed {
				return fmt.Errorf("remove row %d: %s", key, res.Evidence.Message)
			}
			return nil
		}
		if c.Op != matrix.Insert {
			// Update and delete act on a row the runner inserts for the case
			if res := r.Runner.Query(ctx, t, ops.InsertSQL(t.Database, t.Table, cols, key), classify.Exists{Resource: true}, ops.QueryOpts{}); res.Outcome != classify.Allowed {
				res.Outcome = classify.Error
				res.Evidence.Message = "setup insert as runner: " + res.Evidence.Message
				return res, obs
			}
		}
		var sql string
		switch c.Op {
		case matrix.Insert:
			sql = ops.InsertSQL(t.Database, t.Table, cols, key)
		case matrix.Update:
			if sql, err = ops.UpdateSQL(t.Database, t.Table, cols, key); err != nil {
				return errResult(err), obs
			}
		case matrix.Delete:
			sql = ops.DeleteSQL(t.Database, t.Table, cols, key)
		}
		res := pc.Query(ctx, t, sql, exists, ops.QueryOpts{})
		res.Cleanup = undo
		return res, obs

	case matrix.CreateTable:
		name := "lfv_ct_" + strings.ReplaceAll(inst.Persona.Key, "-", "_")
		res := pc.DDL(ctx, t, ops.CreateTableSQL(t.Database, name, []ops.Col{{Name: "id", Type: "bigint"}}, ""))
		if res.Outcome == classify.Allowed {
			res.Cleanup = func(ctx context.Context) error { return r.deleteTable(ctx, t, name) }
		}
		return res, obs
	case matrix.AlterTable:
		column := "lfv_" + strings.ReplaceAll(inst.Persona.Key, "-", "_")
		res := pc.DDL(ctx, t, ops.AlterTableSQL(t.Database, t.Table, column))
		if res.Outcome == classify.Allowed {
			res.Cleanup = func(ctx context.Context) error {
				out := r.Runner.DDL(ctx, t, ops.DropColumnSQL(t.Database, t.Table, column))
				if out.Outcome != classify.Allowed {
					return fmt.Errorf("drop column %s: %s", column, out.Evidence.Message)
				}
				return nil
			}
		}
		return res, obs
	case matrix.DropTable:
		// Drop cases run last; a successful drop is undone by re-seeding the table, so terraform
		// can still refresh and destroy what references it
		res := pc.DDL(ctx, t, ops.DropTableSQL(t.Database, t.Table))
		if res.Outcome == classify.Allowed && r.Restore != nil {
			res.Cleanup = func(ctx context.Context) error { return r.Restore(ctx, t) }
		}
		return res, obs

	case matrix.Grant, matrix.Revoke, matrix.GrantColumnsWithWrite, matrix.GrantExcludePartition:
		grantee, ok := r.Manifest.Personas[c.Resource.Grantee]
		if !ok {
			return errResult(fmt.Errorf("grantee %q is not a persona", c.Resource.Grantee)), obs
		}
		switch c.Op {
		case matrix.Grant:
			return pc.Grant(ctx, r.Runner, t, grantee), obs
		case matrix.Revoke:
			return pc.Revoke(ctx, r.Runner, t, grantee), obs
		case matrix.GrantColumnsWithWrite:
			return pc.GrantColumnsWithWrite(ctx, r.Runner, t, grantee, t.Columns), obs
		default:
			part := r.Manifest.Tables[t.Table].PartitionColumn
			if part == nil {
				return errResult(fmt.Errorf("table %s has no partition column", t.Table)), obs
			}
			return pc.GrantExcludePartition(ctx, r.Runner, t, grantee, *part), obs
		}

	case matrix.AddLFTags:
		tags := r.Manifest.Styles.Label.TagKeys
		if tags == nil {
			return errResult(errors.New("manifest has no label tag keys")), obs
		}
		assigned := inst.Style == matrix.Label && t.Key == "alpha/curated" && t.Table == "scratch"
		return pc.AddLFTags(ctx, r.Runner, t, tags.MW, assigned), obs
	case matrix.CreateLFTag:
		return pc.CreateLFTag(ctx, r.Runner, fmt.Sprintf("%s-ct-%s", r.Manifest.Prefix, strings.ReplaceAll(inst.Persona.Key, "_", "-"))), obs
	case matrix.CreateFilter:
		return pc.CreateFilter(ctx, r.Runner, t, fmt.Sprintf("%s-f-%s-%s", r.Manifest.Prefix, inst.Persona.Key, inst.Style), "region = 'EU'"), obs
	case matrix.CreateFilterMapColumn:
		mc := r.Manifest.Tables[t.Table].MapColumn
		if mc == nil {
			return errResult(fmt.Errorf("table %s has no map column", t.Table)), obs
		}
		return pc.CreateFilterMapColumn(ctx, r.Runner, t, fmt.Sprintf("%s-fm-%s-%s", r.Manifest.Prefix, inst.Persona.Key, inst.Style), *mc), obs

	case matrix.ListPermissions:
		return pc.ListPermissionsOp(ctx), obs
	case matrix.GetSettings:
		_, res := pc.GetSettings(ctx)
		return res, obs
	case matrix.PutSettings:
		current, res := r.Runner.GetSettings(ctx)
		if res.Outcome != classify.Allowed {
			res.Outcome = classify.Error
			return res, obs
		}
		return pc.PutSettings(ctx, current), obs

	case matrix.StorageRead:
		return pc.StorageRead(ctx, t), obs
	}
	return errResult(fmt.Errorf("operation %s is not implemented", c.Op)), obs
}

// deleteTable removes a table through the storage API as the runner, which does not depend on
// Lake Formation grants.
func (r *Runner) deleteTable(ctx context.Context, t manifest.Target, name string) error {
	_, err := r.Runner.S3Tables.DeleteTable(ctx, &s3tables.DeleteTableInput{
		TableBucketARN: aws.String(t.TableBucketARN), Namespace: aws.String(t.Database), Name: aws.String(name),
	})
	return err
}

// setup applies a scenario mutation as the runner and returns its undo, which the caller always
// runs, even when setup fails part way.
func (r *Runner) setup(ctx context.Context, s matrix.Setup, style matrix.Style, t manifest.Target) (func(context.Context) error, error) {
	switch s {
	case matrix.RelabelColumnRestricted, matrix.RelabelTableRestricted:
		if style != matrix.Label {
			return nil, errors.New("relabel setups apply to the label style only")
		}
		key := r.Manifest.Styles.Label.TagKeys.Sensitivity
		res := &lftypes.Resource{Table: &lftypes.TableResource{CatalogId: aws.String(t.CatalogID), DatabaseName: aws.String(t.Database), Name: aws.String(t.Table)}}
		// The fixture labels orders columns and the customers and scratch tables internal
		// (labels.tf), so undo restores internal
		if s == matrix.RelabelColumnRestricted {
			res = &lftypes.Resource{TableWithColumns: &lftypes.TableWithColumnsResource{
				CatalogId: aws.String(t.CatalogID), DatabaseName: aws.String(t.Database), Name: aws.String(t.Table), ColumnNames: []string{"amount"},
			}}
		}
		undo := func(ctx context.Context) error {
			_, err := r.Runner.LF.AddLFTagsToResource(ctx, &lakeformation.AddLFTagsToResourceInput{
				Resource: res, LFTags: []lftypes.LFTagPair{{TagKey: aws.String(key), TagValues: []string{"internal"}}},
			})
			return err
		}
		_, err := r.Runner.LF.AddLFTagsToResource(ctx, &lakeformation.AddLFTagsToResourceInput{
			Resource: res, LFTags: []lftypes.LFTagPair{{TagKey: aws.String(key), TagValues: []string{"restricted"}}},
		})
		return undo, err

	case matrix.LateTable:
		undo := func(ctx context.Context) error { return r.deleteTable(ctx, t, t.Table) }
		res := r.Runner.DDL(ctx, t, ops.CreateTableSQL(t.Database, t.Table, []ops.Col{{Name: "id", Type: "bigint"}}, ""))
		if res.Outcome != classify.Allowed {
			return undo, fmt.Errorf("create %s: %s", t.Table, res.Evidence.Message)
		}
		return undo, nil

	case matrix.AllRowsFilter:
		grantee := r.Manifest.Personas["reader_rows"]
		name := fmt.Sprintf("%s-allrows-%s", r.Manifest.Prefix, style)
		filter := &lakeformation.CreateDataCellsFilterInput{TableData: &lftypes.DataCellsFilter{
			TableCatalogId: aws.String(t.CatalogID), DatabaseName: aws.String(t.Database), TableName: aws.String(t.Table), Name: aws.String(name),
			RowFilter:      &lftypes.RowFilter{AllRowsWildcard: &lftypes.AllRowsWildcard{}},
			ColumnWildcard: &lftypes.ColumnWildcard{},
		}}
		grant := &lftypes.Resource{DataCellsFilter: &lftypes.DataCellsFilterResource{
			TableCatalogId: aws.String(t.CatalogID), DatabaseName: aws.String(t.Database), TableName: aws.String(t.Table), Name: aws.String(name),
		}}
		principal := &lftypes.DataLakePrincipal{DataLakePrincipalIdentifier: aws.String(grantee)}
		undo := func(ctx context.Context) error {
			_, errRevoke := r.Runner.LF.RevokePermissions(ctx, &lakeformation.RevokePermissionsInput{
				Principal: principal, Resource: grant, Permissions: []lftypes.Permission{lftypes.PermissionSelect},
			})
			_, errDelete := r.Runner.LF.DeleteDataCellsFilter(ctx, &lakeformation.DeleteDataCellsFilterInput{
				TableCatalogId: aws.String(t.CatalogID), DatabaseName: aws.String(t.Database), TableName: aws.String(t.Table), Name: aws.String(name),
			})
			return errors.Join(ignoreGone(errRevoke), ignoreGone(errDelete))
		}
		if _, err := r.Runner.LF.CreateDataCellsFilter(ctx, filter); err != nil {
			return undo, err
		}
		_, err := r.Runner.LF.GrantPermissions(ctx, &lakeformation.GrantPermissionsInput{
			Principal: principal, Resource: grant, Permissions: []lftypes.Permission{lftypes.PermissionSelect},
		})
		return undo, err
	}
	return nil, fmt.Errorf("unknown setup %q", s)
}

func ignoreGone(err error) error {
	if err == nil {
		return nil
	}
	if o, _ := classify.Classify(err, classify.Exists{}); o == classify.Rejected {
		return nil
	}
	return err
}
