package ops

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/athena"
	"github.com/aws/aws-sdk-go-v2/service/athena/types"

	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/classify"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/manifest"
)

// QueryTimeout bounds one query, including queueing.
const QueryTimeout = 5 * time.Minute

// RowVisitor sees each result row and must not retain or log it (FR-030).
type RowVisitor func(row map[string]string)

// QueryOpts controls what a query returns.
type QueryOpts struct {
	// KeyColumn, when set, collects that column's values into Result.Keys (full mode only)
	KeyColumn string
	// Visit, when set, receives every row (probe-mode predicate checks)
	Visit RowVisitor
}

// Query runs one statement in the target's workgroup and catalog and waits for it.
func (c *Clients) Query(ctx context.Context, t manifest.Target, sql string, exists classify.Exists, opts QueryOpts) Result {
	start, err := c.Athena.StartQueryExecution(ctx, &athena.StartQueryExecutionInput{
		QueryString: aws.String(sql),
		WorkGroup:   aws.String(t.Workgroup),
		QueryExecutionContext: &types.QueryExecutionContext{
			Catalog:  aws.String(t.AthenaCatalog()),
			Database: aws.String(t.Database),
		},
	})
	if err != nil {
		return fromErr(err, exists)
	}
	id := aws.ToString(start.QueryExecutionId)
	exec, err := c.wait(ctx, id)
	if err != nil {
		return fromErr(err, exists)
	}
	res := Result{Evidence: classify.Evidence{RequestID: id}}
	if exec.Statistics != nil {
		res.ScanBytes = aws.ToInt64(exec.Statistics.DataScannedInBytes)
	}
	if exec.Status.State != types.QueryExecutionStateSucceeded {
		reason := aws.ToString(exec.Status.StateChangeReason)
		if exec.Status.AthenaError != nil && reason == "" {
			reason = aws.ToString(exec.Status.AthenaError.ErrorMessage)
		}
		res.Outcome, res.Evidence = classify.QueryFailure(reason, exists)
		res.Evidence.RequestID = id
		return res
	}
	res.Outcome = classify.Allowed
	if exec.StatementType != types.StatementTypeDml {
		return res
	}
	if err := c.readResults(ctx, id, &res, opts); err != nil {
		res.Outcome, res.Evidence = classify.Error, classify.Evidence{Message: err.Error(), RequestID: id}
	}
	return res
}

func (c *Clients) wait(ctx context.Context, id string) (*types.QueryExecution, error) {
	ctx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()
	delay := 500 * time.Millisecond
	for {
		out, err := c.Athena.GetQueryExecution(ctx, &athena.GetQueryExecutionInput{QueryExecutionId: aws.String(id)})
		if err != nil {
			return nil, err
		}
		switch out.QueryExecution.Status.State {
		case types.QueryExecutionStateSucceeded, types.QueryExecutionStateFailed, types.QueryExecutionStateCancelled:
			return out.QueryExecution, nil
		}
		select {
		case <-ctx.Done():
			_, _ = c.Athena.StopQueryExecution(context.WithoutCancel(ctx), &athena.StopQueryExecutionInput{QueryExecutionId: aws.String(id)})
			return nil, fmt.Errorf("query %s did not finish within %s", id, QueryTimeout)
		case <-time.After(delay):
		}
		delay = min(delay*2, 5*time.Second)
	}
}

func (c *Clients) readResults(ctx context.Context, id string, res *Result, opts QueryOpts) error {
	p := athena.NewGetQueryResultsPaginator(c.Athena, &athena.GetQueryResultsInput{QueryExecutionId: aws.String(id)})
	rows := 0
	first := true
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return err
		}
		if res.Columns == nil && page.ResultSet.ResultSetMetadata != nil {
			for _, ci := range page.ResultSet.ResultSetMetadata.ColumnInfo {
				res.Columns = append(res.Columns, aws.ToString(ci.Name))
			}
		}
		for _, r := range page.ResultSet.Rows {
			vals := make([]string, len(r.Data))
			for i, d := range r.Data {
				vals[i] = aws.ToString(d.VarCharValue)
			}
			// SELECT results repeat the header as the first row
			if first {
				first = false
				if equal(vals, res.Columns) {
					continue
				}
			}
			rows++
			row := make(map[string]string, len(vals))
			for i, v := range vals {
				if i < len(res.Columns) {
					row[res.Columns[i]] = v
				}
			}
			if opts.KeyColumn != "" {
				if v, ok := row[opts.KeyColumn]; ok {
					res.Keys = append(res.Keys, v)
				}
			}
			if opts.Visit != nil {
				opts.Visit(row)
			}
		}
	}
	res.RowCount = intPtr(rows)
	return nil
}

// CountRows runs count(*) and returns the count as RowCount.
func (c *Clients) CountRows(ctx context.Context, t manifest.Target, exists classify.Exists) Result {
	var n *int
	res := c.Query(ctx, t, CountSQL(t.Database, t.Table), exists, QueryOpts{Visit: func(row map[string]string) {
		var v int
		if _, err := fmt.Sscan(row["n"], &v); err == nil {
			n = &v
		}
	}})
	if res.Outcome == classify.Allowed {
		if n == nil {
			res.Outcome, res.Evidence.Message = classify.Error, "count(*) returned no value"
		}
		res.RowCount = n
	}
	return res
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
