package seed

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/iceberg-go/catalog/rest"
	_ "github.com/apache/iceberg-go/io/gocloud/s3" // registers the s3:// file IO
	"github.com/apache/iceberg-go/table"
	"github.com/apache/iceberg-go/utils"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lakeformation"
	lftypes "github.com/aws/aws-sdk-go-v2/service/lakeformation/types"

	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/classify"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/manifest"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/ops"
)

// Seeder creates fixture tables and rows as the fixture owner (the runner), so creator
// permissions never reach a persona (FR-006).
type Seeder struct {
	Cfg      aws.Config
	Runner   *ops.Clients
	Manifest *manifest.Manifest
	Log      func(format string, a ...any)
}

func (s *Seeder) logf(format string, a ...any) {
	if s.Log != nil {
		s.Log(format, a...)
	}
}

// CatalogDescribe makes the module's required one-time grant (README, "Required One-Time
// Grants"): DESCRIBE on each bucket catalog to the account's IAM principals. The query engine
// resolves a bucket catalog only for principals that can describe it; the grant reveals only the
// catalog's existence, and databases and tables stay gated by their own grants. The provider
// cannot express it yet (awslkhs-qrp). Deleting the bucket removes the grant.
func (s *Seeder) CatalogDescribe(ctx context.Context) error {
	done := map[string]bool{}
	for _, ns := range s.Manifest.AllNamespaces() {
		if done[ns.CatalogID] {
			continue
		}
		done[ns.CatalogID] = true
		s.logf("grant DESCRIBE on catalog %s to %s:IAMPrincipals", ns.CatalogID, s.Manifest.AccountID)
		_, err := s.Runner.LF.GrantPermissions(ctx, &lakeformation.GrantPermissionsInput{
			Principal:   &lftypes.DataLakePrincipal{DataLakePrincipalIdentifier: aws.String(s.Manifest.AccountID + ":IAMPrincipals")},
			Permissions: []lftypes.Permission{lftypes.PermissionDescribe},
			Resource:    &lftypes.Resource{Catalog: &lftypes.CatalogResource{Id: aws.String(ns.CatalogID)}},
		})
		if err != nil {
			return fmt.Errorf("catalog DESCRIBE on %s: %w", ns.CatalogID, err)
		}
	}
	return nil
}

// Seed creates every fixture table in every namespace of both styles, appends its rows, and
// verifies the counts. It is idempotent: existing tables with the right count are left alone.
func (s *Seeder) Seed(ctx context.Context) error {
	if err := s.CatalogDescribe(ctx); err != nil {
		return err
	}
	ctx = utils.WithAwsConfig(ctx, &s.Cfg)
	catalogs := map[string]*rest.Catalog{}
	keys := make([]string, 0)
	all := s.Manifest.AllNamespaces()
	for k := range all {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		ns := all[k]
		cat, ok := catalogs[ns.TableBucketARN]
		if !ok {
			c, err := rest.NewCatalog(ctx, "s3tables", "https://s3tables."+s.Cfg.Region+".amazonaws.com/iceberg",
				rest.WithSigV4RegionSvc(s.Cfg.Region, "s3tables"),
				rest.WithWarehouseLocation(ns.TableBucketARN),
				rest.WithAwsConfig(s.Cfg),
			)
			if err != nil {
				return fmt.Errorf("iceberg REST catalog for %s: %w", ns.TableBucketARN, err)
			}
			catalogs[ns.TableBucketARN], cat = c, c
		}
		for _, name := range []string{"orders", "customers", "scratch"} {
			if err := s.seedTable(ctx, cat, k, ns, name); err != nil {
				return err
			}
		}
	}
	return nil
}

// Restore re-creates and re-seeds one fixture table, after a drop case removed it, so the
// fixture stays whole for teardown and later runs.
func (s *Seeder) Restore(ctx context.Context, t manifest.Target) error {
	ctx = utils.WithAwsConfig(ctx, &s.Cfg)
	cat, err := rest.NewCatalog(ctx, "s3tables", "https://s3tables."+s.Cfg.Region+".amazonaws.com/iceberg",
		rest.WithSigV4RegionSvc(s.Cfg.Region, "s3tables"),
		rest.WithWarehouseLocation(t.TableBucketARN),
		rest.WithAwsConfig(s.Cfg),
	)
	if err != nil {
		return err
	}
	return s.seedTable(ctx, cat, t.Key, t.Namespace, t.Table)
}

func (s *Seeder) seedTable(ctx context.Context, cat *rest.Catalog, key string, ns manifest.Namespace, name string) error {
	def, ok := s.Manifest.Tables[name]
	if !ok {
		return fmt.Errorf("manifest has no table %q", name)
	}
	t := manifest.Target{Namespace: ns, Key: key, Table: name, Exists: true}
	want := def.Rows

	count := s.Runner.CountRows(ctx, t, classify.Exists{Resource: true})
	if count.Outcome == classify.Allowed && count.RowCount != nil && *count.RowCount == want {
		s.logf("%s.%s already seeded (%d rows)", key, name, want)
		return nil
	}
	if count.Outcome != classify.Allowed {
		cols := make([]ops.Col, len(def.Columns))
		for i, c := range def.Columns {
			cols[i] = ops.Col{Name: c.Name, Type: c.Type}
		}
		partition := ""
		if def.PartitionColumn != nil {
			partition = *def.PartitionColumn
		}
		s.logf("create %s.%s", key, name)
		if res := s.Runner.DDL(ctx, t, ops.CreateTableSQL(ns.Database, name, cols, partition)); res.Outcome != classify.Allowed {
			return fmt.Errorf("create %s.%s: %s %s", key, name, res.Evidence.Code, res.Evidence.Message)
		}
	} else if count.RowCount != nil && *count.RowCount != 0 {
		return fmt.Errorf("%s.%s holds %d rows, want 0 or %d; run cleanup and reseed", key, name, *count.RowCount, want)
	}

	tbl, err := cat.LoadTable(ctx, table.Identifier{ns.Database, name})
	if err != nil {
		return fmt.Errorf("load %s.%s through the Iceberg REST endpoint: %w", key, name, err)
	}
	schema, err := table.SchemaToArrowSchema(tbl.Schema(), nil, false, false)
	if err != nil {
		return err
	}
	rows, err := Rows(name)
	if err != nil {
		return err
	}
	rec, err := Record(schema, rows)
	if err != nil {
		return err
	}
	defer rec.Release()
	at := arrowTable(schema, rec)
	defer at.Release()
	s.logf("append %d rows to %s.%s", len(rows), key, name)
	if _, err := tbl.AppendTable(ctx, at, int64(len(rows)), nil); err != nil {
		return fmt.Errorf("append to %s.%s (plan Risk 3: decide the fallback, do not retry silently): %w", key, name, err)
	}

	count = s.Runner.CountRows(ctx, t, classify.Exists{Resource: true})
	if count.Outcome != classify.Allowed || count.RowCount == nil || *count.RowCount != want {
		got := "no count"
		if count.RowCount != nil {
			got = fmt.Sprint(*count.RowCount)
		}
		return fmt.Errorf("%s.%s after seeding: %s rows, want %d (%s)", key, name, got, want, strings.TrimSpace(count.Evidence.Message))
	}
	return nil
}

func arrowTable(schema *arrow.Schema, rec arrow.RecordBatch) arrow.Table {
	return array.NewTableFromRecords(schema, []arrow.RecordBatch{rec})
}
