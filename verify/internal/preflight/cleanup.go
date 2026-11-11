package preflight

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/athena"
	"github.com/aws/aws-sdk-go-v2/service/glue"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/aws/aws-sdk-go-v2/service/lakeformation"
	lftypes "github.com/aws/aws-sdk-go-v2/service/lakeformation/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/s3tables"
)

// Sweeper removes everything a run left behind, found by its lfv-<run-id> name prefix, so an
// interrupted run or lost state is recoverable (FR-004, Cleanup SLO). Every step tolerates
// resources that are already gone, so cleanup can rerun.
type Sweeper struct {
	LF       *lakeformation.Client
	S3Tables *s3tables.Client
	Athena   *athena.Client
	S3       *s3.Client
	IAM      *iam.Client
	Glue     *glue.Client
	KMS      *kms.Client
	Prefix   string // lfv-<run-id>
	Log      func(format string, a ...any)
}

func (s *Sweeper) logf(format string, a ...any) {
	if s.Log != nil {
		s.Log(format, a...)
	}
}

// DropTables deletes the tables in the run's table buckets and keeps the buckets and namespaces,
// so terraform destroy can delete namespaces (it cannot delete one that holds tables).
func (s *Sweeper) DropTables(ctx context.Context) error {
	if err := s.checkPrefix(); err != nil {
		return err
	}
	p := s3tables.NewListTableBucketsPaginator(s.S3Tables, &s3tables.ListTableBucketsInput{Prefix: aws.String(s.Prefix)})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, b := range page.TableBuckets {
			if err := s.deleteTables(ctx, aws.ToString(b.Arn)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Sweeper) checkPrefix() error {
	if !strings.HasPrefix(s.Prefix, "lfv-") || len(s.Prefix) < len("lfv-000000") {
		return fmt.Errorf("refusing to sweep with prefix %q", s.Prefix)
	}
	return nil
}

// Sweep deletes, in dependency order: data cells filters, grants to persona roles, LF-tags,
// table buckets (with their tables and namespaces), workgroups, results buckets, and persona
// roles. It returns every error rather than stopping at the first.
func (s *Sweeper) Sweep(ctx context.Context) error {
	if err := s.checkPrefix(); err != nil {
		return err
	}
	var errs []error
	for _, step := range []struct {
		name string
		fn   func(context.Context) error
	}{
		{"data cells filters", s.filters},
		{"persona grants", s.grants},
		{"LF-tags", s.tags},
		{"table buckets", s.tableBuckets},
		{"workgroups", s.workgroups},
		{"results buckets", s.resultsBuckets},
		{"account integration", s.accountIntegration},
		{"persona roles", s.roles},
		{"KMS keys", s.keys},
	} {
		if err := step.fn(ctx); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", step.name, err))
		}
	}
	return errors.Join(errs...)
}

func (s *Sweeper) filters(ctx context.Context) error {
	p := lakeformation.NewListDataCellsFilterPaginator(s.LF, &lakeformation.ListDataCellsFilterInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, f := range page.DataCellsFilters {
			// Fixture filters carry the prefix; module filters are named <tenant>-<key> and live
			// in fixture table buckets, which carry the prefix
			if !strings.HasPrefix(aws.ToString(f.Name), s.Prefix) && !strings.Contains(aws.ToString(f.TableCatalogId), "/"+s.Prefix) {
				continue
			}
			s.logf("delete filter %s", aws.ToString(f.Name))
			if _, err := s.LF.DeleteDataCellsFilter(ctx, &lakeformation.DeleteDataCellsFilterInput{
				TableCatalogId: f.TableCatalogId, DatabaseName: f.DatabaseName, TableName: f.TableName, Name: f.Name,
			}); err != nil && !notFound(err) {
				return err
			}
		}
	}
	return nil
}

func (s *Sweeper) grants(ctx context.Context) error {
	p := lakeformation.NewListPermissionsPaginator(s.LF, &lakeformation.ListPermissionsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, g := range page.PrincipalResourcePermissions {
			id := aws.ToString(g.Principal.DataLakePrincipalIdentifier)
			if !strings.Contains(id, ":role/"+s.Prefix+"-") {
				continue
			}
			s.logf("revoke %v from %s", g.Permissions, id)
			if _, err := s.LF.RevokePermissions(ctx, &lakeformation.RevokePermissionsInput{
				Principal: g.Principal, Resource: revocable(g.Resource), Permissions: g.Permissions,
				PermissionsWithGrantOption: g.PermissionsWithGrantOption,
			}); err != nil && !notFound(err) {
				return err
			}
		}
	}
	return nil
}

// revocable adapts a listed resource for RevokePermissions: ListPermissions reports all-tables
// grants with both a table name and a wildcard, which RevokePermissions rejects.
func revocable(r *lftypes.Resource) *lftypes.Resource {
	if r == nil || r.Table == nil || r.Table.TableWildcard == nil {
		return r
	}
	out := *r
	t := *r.Table
	t.Name = nil
	out.Table = &t
	return &out
}

func (s *Sweeper) tags(ctx context.Context) error {
	p := lakeformation.NewListLFTagsPaginator(s.LF, &lakeformation.ListLFTagsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, t := range page.LFTags {
			if !strings.HasPrefix(aws.ToString(t.TagKey), s.Prefix+"-") {
				continue
			}
			s.logf("delete LF-tag %s", aws.ToString(t.TagKey))
			if _, err := s.LF.DeleteLFTag(ctx, &lakeformation.DeleteLFTagInput{TagKey: t.TagKey}); err != nil && !notFound(err) {
				return err
			}
		}
	}
	return nil
}

func (s *Sweeper) tableBuckets(ctx context.Context) error {
	p := s3tables.NewListTableBucketsPaginator(s.S3Tables, &s3tables.ListTableBucketsInput{Prefix: aws.String(s.Prefix)})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, b := range page.TableBuckets {
			if err := s.deleteTableBucket(ctx, aws.ToString(b.Arn)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Sweeper) deleteTables(ctx context.Context, arn string) error {
	tp := s3tables.NewListTablesPaginator(s.S3Tables, &s3tables.ListTablesInput{TableBucketARN: aws.String(arn)})
	for tp.HasMorePages() {
		page, err := tp.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, t := range page.Tables {
			ns := t.Namespace[0]
			s.logf("delete table %s.%s", ns, aws.ToString(t.Name))
			if _, err := s.S3Tables.DeleteTable(ctx, &s3tables.DeleteTableInput{TableBucketARN: aws.String(arn), Namespace: aws.String(ns), Name: t.Name}); err != nil && !notFound(err) {
				return err
			}
		}
	}
	return nil
}

func (s *Sweeper) deleteTableBucket(ctx context.Context, arn string) error {
	if err := s.deleteTables(ctx, arn); err != nil {
		return err
	}
	np := s3tables.NewListNamespacesPaginator(s.S3Tables, &s3tables.ListNamespacesInput{TableBucketARN: aws.String(arn)})
	for np.HasMorePages() {
		page, err := np.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, n := range page.Namespaces {
			if _, err := s.S3Tables.DeleteNamespace(ctx, &s3tables.DeleteNamespaceInput{TableBucketARN: aws.String(arn), Namespace: aws.String(n.Namespace[0])}); err != nil && !notFound(err) {
				return err
			}
		}
	}
	s.logf("delete table bucket %s", arn)
	if _, err := s.S3Tables.DeleteTableBucket(ctx, &s3tables.DeleteTableBucketInput{TableBucketARN: aws.String(arn)}); err != nil && !notFound(err) {
		return err
	}
	return nil
}

func (s *Sweeper) workgroups(ctx context.Context) error {
	p := athena.NewListWorkGroupsPaginator(s.Athena, &athena.ListWorkGroupsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, w := range page.WorkGroups {
			if !strings.HasPrefix(aws.ToString(w.Name), s.Prefix+"-") {
				continue
			}
			s.logf("delete workgroup %s", aws.ToString(w.Name))
			if _, err := s.Athena.DeleteWorkGroup(ctx, &athena.DeleteWorkGroupInput{WorkGroup: w.Name, RecursiveDeleteOption: aws.Bool(true)}); err != nil && !notFound(err) {
				return err
			}
		}
	}
	return nil
}

func (s *Sweeper) resultsBuckets(ctx context.Context) error {
	p := s3.NewListBucketsPaginator(s.S3, &s3.ListBucketsInput{Prefix: aws.String(s.Prefix + "-")})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, b := range page.Buckets {
			if err := s.emptyBucket(ctx, aws.ToString(b.Name)); err != nil {
				return err
			}
			s.logf("delete bucket %s", aws.ToString(b.Name))
			if _, err := s.S3.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: b.Name}); err != nil && !notFound(err) {
				return err
			}
		}
	}
	return nil
}

func (s *Sweeper) emptyBucket(ctx context.Context, bucket string) error {
	p := s3.NewListObjectsV2Paginator(s.S3, &s3.ListObjectsV2Input{Bucket: aws.String(bucket)})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return err
		}
		if len(page.Contents) == 0 {
			continue
		}
		var ids []s3types.ObjectIdentifier
		for _, o := range page.Contents {
			ids = append(ids, s3types.ObjectIdentifier{Key: o.Key})
		}
		if _, err := s.S3.DeleteObjects(ctx, &s3.DeleteObjectsInput{Bucket: aws.String(bucket), Delete: &s3types.Delete{Objects: ids, Quiet: aws.Bool(true)}}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Sweeper) roles(ctx context.Context) error {
	p := iam.NewListRolesPaginator(s.IAM, &iam.ListRolesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, r := range page.Roles {
			name := aws.ToString(r.RoleName)
			if !strings.HasPrefix(name, s.Prefix+"-") {
				continue
			}
			if err := s.deleteRole(ctx, name); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Sweeper) deleteRole(ctx context.Context, name string) error {
	inline, err := s.IAM.ListRolePolicies(ctx, &iam.ListRolePoliciesInput{RoleName: aws.String(name)})
	if err != nil {
		return err
	}
	for _, pn := range inline.PolicyNames {
		if _, err := s.IAM.DeleteRolePolicy(ctx, &iam.DeleteRolePolicyInput{RoleName: aws.String(name), PolicyName: aws.String(pn)}); err != nil && !notFound(err) {
			return err
		}
	}
	attached, err := s.IAM.ListAttachedRolePolicies(ctx, &iam.ListAttachedRolePoliciesInput{RoleName: aws.String(name)})
	if err != nil {
		return err
	}
	for _, ap := range attached.AttachedPolicies {
		if _, err := s.IAM.DetachRolePolicy(ctx, &iam.DetachRolePolicyInput{RoleName: aws.String(name), PolicyArn: ap.PolicyArn}); err != nil && !notFound(err) {
			return err
		}
	}
	s.logf("delete role %s", name)
	if _, err := s.IAM.DeleteRole(ctx, &iam.DeleteRoleInput{RoleName: aws.String(name)}); err != nil && !notFound(err) {
		return err
	}
	return nil
}

// RestoreSettings writes back the settings saved before the run and verifies them. Destroying
// the fixture's settings resource does not restore earlier values.
func RestoreSettings(ctx context.Context, lf LFAPI, saved *lftypes.DataLakeSettings) error {
	if _, err := lf.PutDataLakeSettings(ctx, &lakeformation.PutDataLakeSettingsInput{DataLakeSettings: saved}); err != nil {
		return fmt.Errorf("restore data-lake settings: %w", err)
	}
	out, err := lf.GetDataLakeSettings(ctx, &lakeformation.GetDataLakeSettingsInput{})
	if err != nil {
		return fmt.Errorf("verify restored settings: %w", err)
	}
	got := out.DataLakeSettings
	if d := principalDiff(saved.DataLakeAdmins, got.DataLakeAdmins); d != "" {
		return fmt.Errorf("admins differ after restore: %s", d)
	}
	if d := principalDiff(saved.ReadOnlyAdmins, got.ReadOnlyAdmins); d != "" {
		return fmt.Errorf("read-only admins differ after restore: %s", d)
	}
	return nil
}

func principalDiff(want, got []lftypes.DataLakePrincipal) string {
	ids := func(ps []lftypes.DataLakePrincipal) []string {
		var out []string
		for _, p := range ps {
			out = append(out, aws.ToString(p.DataLakePrincipalIdentifier))
		}
		slices.Sort(out)
		return out
	}
	w, g := ids(want), ids(got)
	if slices.Equal(w, g) {
		return ""
	}
	return fmt.Sprintf("want %v, got %v", w, g)
}

func notFound(err error) bool {
	s := err.Error()
	return strings.Contains(s, "NotFound") || strings.Contains(s, "NoSuchEntity") || strings.Contains(s, "NoSuchBucket") || strings.Contains(s, "does not exist")
}

// accountIntegration removes the federated catalog and the bucket registration, but only when
// the registration's role carries the run prefix: that proves this run's fixture created them,
// so another deployment's integration is never touched.
func (s *Sweeper) accountIntegration(ctx context.Context) error {
	out, err := s.LF.ListResources(ctx, &lakeformation.ListResourcesInput{})
	if err != nil {
		return err
	}
	for _, r := range out.ResourceInfoList {
		if !strings.Contains(aws.ToString(r.RoleArn), ":role/"+s.Prefix+"-") {
			continue
		}
		s.logf("deregister %s (role %s)", aws.ToString(r.ResourceArn), aws.ToString(r.RoleArn))
		if _, err := s.LF.DeregisterResource(ctx, &lakeformation.DeregisterResourceInput{ResourceArn: r.ResourceArn}); err != nil && !notFound(err) {
			return err
		}
		if s.Glue == nil {
			continue
		}
		s.logf("delete federated catalog s3tablescatalog")
		if _, err := s.Glue.DeleteCatalog(ctx, &glue.DeleteCatalogInput{CatalogId: aws.String("s3tablescatalog")}); err != nil && !notFound(err) {
			return err
		}
	}
	return nil
}

// keys schedules deletion of KMS keys tagged with the run (7-day window, the minimum).
func (s *Sweeper) keys(ctx context.Context) error {
	if s.KMS == nil {
		return nil
	}
	run := strings.TrimPrefix(s.Prefix, "lfv-")
	p := kms.NewListKeysPaginator(s.KMS, &kms.ListKeysInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, k := range page.Keys {
			tags, err := s.KMS.ListResourceTags(ctx, &kms.ListResourceTagsInput{KeyId: k.KeyId})
			if err != nil {
				continue // AWS managed keys refuse tag reads
			}
			tagged := false
			for _, t := range tags.Tags {
				if aws.ToString(t.TagKey) == "LfverifyRun" && aws.ToString(t.TagValue) == run {
					tagged = true
				}
			}
			if !tagged {
				continue
			}
			d, err := s.KMS.DescribeKey(ctx, &kms.DescribeKeyInput{KeyId: k.KeyId})
			if err != nil || d.KeyMetadata.KeyState == kmstypes.KeyStatePendingDeletion {
				continue
			}
			s.logf("schedule deletion of KMS key %s", aws.ToString(k.KeyId))
			if _, err := s.KMS.ScheduleKeyDeletion(ctx, &kms.ScheduleKeyDeletionInput{KeyId: k.KeyId, PendingWindowInDays: aws.Int32(7)}); err != nil {
				return err
			}
		}
	}
	return nil
}
