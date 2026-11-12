package runner

import (
	"context"
	"testing"

	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/manifest"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/matrix"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/report"
)

// TestExecuteRecordsDuration covers an early return: the deferred duration must reach the result
func TestExecuteRecordsDuration(t *testing.T) {
	r := &Runner{Matrix: &matrix.Matrix{Mode: matrix.Full}, Manifest: &manifest.Manifest{}}
	res := r.execute(context.Background(), matrix.Instance{
		ID: "x/label", Persona: matrix.Persona{Key: "admin"}, Style: matrix.Label,
		Case: matrix.Case{Op: matrix.ListDatabases, Resource: matrix.Resource{Tenant: "alpha"}},
	})
	if res.Status != report.NotRun {
		t.Fatalf("status %s, want not-run for a namespace missing from the manifest", res.Status)
	}
	if res.Duration <= 0 {
		t.Errorf("duration %v, want > 0", res.Duration)
	}
}
