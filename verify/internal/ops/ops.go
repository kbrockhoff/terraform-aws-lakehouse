package ops

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/athena"
	"github.com/aws/aws-sdk-go-v2/service/glue"
	"github.com/aws/aws-sdk-go-v2/service/lakeformation"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3tables"

	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/classify"
)

// Clients holds the service clients for one identity.
type Clients struct {
	Glue     *glue.Client
	LF       *lakeformation.Client
	Athena   *athena.Client
	S3       *s3.Client
	S3Tables *s3tables.Client
}

// NewClients builds clients from one config, typically an assumed persona session.
func NewClients(cfg aws.Config) *Clients {
	return &Clients{
		Glue:     glue.NewFromConfig(cfg),
		LF:       lakeformation.NewFromConfig(cfg),
		Athena:   athena.NewFromConfig(cfg),
		S3:       s3.NewFromConfig(cfg),
		S3Tables: s3tables.NewFromConfig(cfg),
	}
}

// Result is what an attempted operation returned. Keys carries fixture key values for full-mode
// comparison only; it is never written to reports.
type Result struct {
	Outcome   classify.Outcome
	Evidence  classify.Evidence
	Columns   []string
	Names     []string
	RowCount  *int
	Keys      []string
	ScanBytes int64
	// Permissions is set by permission inspection
	Permissions []string
	// Cleanup undoes an operation that changed state, such as an unexpectedly accepted grant.
	// The runner always calls it when it is set.
	Cleanup func(context.Context) error
}

func fromErr(err error, exists classify.Exists) Result {
	o, ev := classify.Classify(err, exists)
	return Result{Outcome: o, Evidence: ev}
}

func intPtr(n int) *int { return &n }
