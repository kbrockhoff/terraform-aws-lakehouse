package ops

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/glue"

	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/classify"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/manifest"
)

// ListCatalogs lists the S3 Tables bucket catalogs under the federated catalog.
func (c *Clients) ListCatalogs(ctx context.Context, t manifest.Target) Result {
	parent := t.AthenaCatalog()
	if i := strings.Index(parent, "/"); i >= 0 {
		parent = parent[:i]
	}
	var names []string
	in := &glue.GetCatalogsInput{ParentCatalogId: aws.String(parent)}
	for {
		out, err := c.Glue.GetCatalogs(ctx, in)
		if err != nil {
			return fromErr(err, classify.Exists{Resource: true})
		}
		for _, cat := range out.CatalogList {
			names = append(names, aws.ToString(cat.Name))
		}
		if out.NextToken == nil {
			break
		}
		in.NextToken = out.NextToken
	}
	return Result{Outcome: classify.Allowed, Names: names}
}

// ListDatabases lists the databases visible in the target's catalog.
func (c *Clients) ListDatabases(ctx context.Context, t manifest.Target) Result {
	var names []string
	p := glue.NewGetDatabasesPaginator(c.Glue, &glue.GetDatabasesInput{CatalogId: aws.String(t.CatalogID)})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return fromErr(err, classify.Exists{Resource: true})
		}
		for _, d := range page.DatabaseList {
			names = append(names, aws.ToString(d.Name))
		}
	}
	return Result{Outcome: classify.Allowed, Names: names}
}

// GetDatabase reads one database's metadata.
func (c *Clients) GetDatabase(ctx context.Context, t manifest.Target) Result {
	_, err := c.Glue.GetDatabase(ctx, &glue.GetDatabaseInput{CatalogId: aws.String(t.CatalogID), Name: aws.String(t.Database)})
	if err != nil {
		return fromErr(err, classify.Exists{Resource: true})
	}
	return Result{Outcome: classify.Allowed}
}

// ListTables lists the tables visible in the target database.
func (c *Clients) ListTables(ctx context.Context, t manifest.Target) Result {
	var names []string
	p := glue.NewGetTablesPaginator(c.Glue, &glue.GetTablesInput{CatalogId: aws.String(t.CatalogID), DatabaseName: aws.String(t.Database)})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return fromErr(err, classify.Exists{Resource: true})
		}
		for _, tb := range page.TableList {
			names = append(names, aws.ToString(tb.Name))
		}
	}
	return Result{Outcome: classify.Allowed, Names: names}
}

// GetTable reads a table's schema. Lake Formation returns only the columns the caller may see,
// so Columns is the visible schema (FR-011, FR-017).
func (c *Clients) GetTable(ctx context.Context, t manifest.Target) Result {
	out, err := c.Glue.GetTable(ctx, &glue.GetTableInput{
		CatalogId: aws.String(t.CatalogID), DatabaseName: aws.String(t.Database), Name: aws.String(t.Table),
	})
	if err != nil {
		return fromErr(err, classify.Exists{Resource: t.Exists})
	}
	res := Result{Outcome: classify.Allowed}
	if sd := out.Table.StorageDescriptor; sd != nil {
		for _, col := range sd.Columns {
			res.Columns = append(res.Columns, aws.ToString(col.Name))
		}
	}
	for _, col := range out.Table.PartitionKeys {
		res.Columns = append(res.Columns, aws.ToString(col.Name))
	}
	return res
}

// DDL runs a DDL statement through the query engine, the path metadata writers use.
func (c *Clients) DDL(ctx context.Context, t manifest.Target, sql string) Result {
	return c.Query(ctx, t, sql, classify.Exists{Resource: t.Exists}, QueryOpts{})
}
