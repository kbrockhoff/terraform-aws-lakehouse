package runner

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/classify"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/identity"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/inspect"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/manifest"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/matrix"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/ops"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/report"
)

// Runner executes a matrix. Static cases run in parallel; scenario and boundary cases run one
// at a time because they mutate shared state.
type Runner struct {
	Matrix   *matrix.Matrix
	Manifest *manifest.Manifest // full mode only
	Assumer  *identity.Assumer
	// Runner is the runner identity's clients: fixture owner in full mode, read-only admin in
	// probe mode. Cleanups and setup mutations run as the runner.
	Runner    *ops.Clients
	Inspector inspect.Inspector
	AccountID string
	RunID     string
	Workers   int
	// SettledSince is the time of the last grant change before the run (the fixture apply);
	// negative static cases wait until SettleSeconds after it
	SettledSince time.Time
	// Only, when set, limits the run to these persona keys (detect reruns)
	Only []string
	// StaticOnly skips scenario and boundary cases (detect reruns)
	StaticOnly bool
	// Restore re-creates a fixture table after a drop case removed it (full mode)
	Restore func(ctx context.Context, t manifest.Target) error
	Log     func(format string, a ...any)

	scanned atomic.Int64
	mu      sync.Mutex
	clients map[string]*ops.Clients
}

func (r *Runner) logf(format string, a ...any) {
	if r.Log != nil {
		r.Log(format, a...)
	}
}

// Run executes every selected case and returns the results in matrix order.
func (r *Runner) Run(ctx context.Context) *report.Run {
	run := &report.Run{RunID: r.RunID, Mode: string(r.Matrix.Mode), Started: time.Now()}
	all := r.Matrix.Expand()
	results := make([]report.Result, len(all))
	var static, sequential []int
	for i, inst := range all {
		if !r.selected(inst) {
			continue
		}
		if inst.Case.Phase == matrix.Static {
			static = append(static, i)
		} else {
			sequential = append(sequential, i)
		}
	}

	// Static phase: wait out the settle window once, so no denial is observed before the
	// fixture's grants have propagated; positive cases retry on their own
	if wait := time.Until(r.SettledSince.Add(time.Duration(r.Matrix.Defaults.SettleSeconds) * time.Second)); wait > 0 {
		r.logf("waiting %s for grants to settle", wait.Round(time.Second))
		sleep(ctx, wait)
	}
	workers := max(r.Workers, 1)
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				results[i] = r.execute(ctx, all[i])
			}
		}()
	}
	for _, i := range static {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	// Scenario cases first, then rule-boundary cases, and drops last because they remove
	// fixture tables
	sort.SliceStable(sequential, func(a, b int) bool { return rank(all[sequential[a]].Case) < rank(all[sequential[b]].Case) })
	for _, i := range sequential {
		results[i] = r.execute(ctx, all[i])
	}

	for i, res := range results {
		if r.selected(all[i]) {
			run.Results = append(run.Results, res)
		}
	}
	run.Finished = time.Now()
	return run
}

func rank(c matrix.Case) int {
	switch {
	case c.Op == matrix.DropTable:
		return 2
	case c.Phase == matrix.Boundary:
		return 1
	}
	return 0
}

func (r *Runner) selected(inst matrix.Instance) bool {
	if r.StaticOnly && inst.Case.Phase != matrix.Static {
		return false
	}
	if len(r.Only) == 0 {
		return true
	}
	for _, k := range r.Only {
		if k == inst.Persona.Key {
			return true
		}
	}
	return false
}

func (r *Runner) roleARN(p matrix.Persona) (string, error) {
	if r.Matrix.Mode == matrix.Probe {
		return p.RoleARN, nil
	}
	arn, ok := r.Manifest.Personas[p.Key]
	if !ok {
		return "", fmt.Errorf("persona %s is not in the manifest", p.Key)
	}
	return arn, nil
}

func (r *Runner) personaClients(ctx context.Context, key, roleARN string) (*ops.Clients, error) {
	cfg, err := r.Assumer.Config(ctx, key, roleARN)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.clients == nil {
		r.clients = map[string]*ops.Clients{}
	}
	// Clients share the assumer's refreshing credentials cache, so building them once is enough
	if c, ok := r.clients[key]; ok {
		return c, nil
	}
	c := ops.NewClients(cfg)
	r.clients[key] = c
	return c, nil
}

func (r *Runner) target(inst matrix.Instance) (manifest.Target, error) {
	if r.Matrix.Mode == matrix.Probe {
		res := inst.Case.Resource
		return manifest.Target{
			Namespace: manifest.Namespace{CatalogID: res.CatalogID, Database: res.Database, Workgroup: res.Workgroup},
			Key:       res.CatalogID + "/" + res.Database,
			Table:     res.Table,
			Columns:   res.Columns,
			Exists:    true,
		}, nil
	}
	return r.Manifest.Resolve(inst.Style, inst.Case.Resource)
}

// execute names its result so the deferred duration lands on the value returned; with an
// unnamed result, every case reported a duration of 0 (observed in T049)
func (r *Runner) execute(ctx context.Context, inst matrix.Instance) (res report.Result) {
	c := inst.Case
	start := time.Now()
	res = report.Result{
		ID: inst.ID, Persona: inst.Persona.Key, Style: string(inst.Style), Op: string(c.Op),
		Expected: string(c.Expect), Rule: c.Rule,
	}
	defer func() { res.Duration = time.Since(start) }()
	notRun := func(reason string) report.Result {
		res.Status, res.Reason = report.NotRun, reason
		return res
	}

	t, err := r.target(inst)
	if err != nil {
		return notRun(err.Error())
	}
	res.Resource = t.Label()
	roleARN, err := r.roleARN(inst.Persona)
	if err != nil {
		return notRun(err.Error())
	}

	if c.Expect == matrix.Inspect {
		eff, err := r.Inspector.Permissions(ctx, roleARN, inspect.TableRef{AccountID: r.AccountID, CatalogID: t.CatalogID, Database: t.Database, Table: t.Table})
		if err != nil {
			return notRun("inspect: " + err.Error())
		}
		res.Observed.Permissions = eff
		res.Status, res.Reason = EvaluateInspection(c, eff)
		return res
	}

	if r.Matrix.Mode == matrix.Probe && slicesContainsOp(matrix.QueryOps, c.Op) && r.scanned.Load() >= r.Matrix.Defaults.ScanCapBytes {
		return notRun(fmt.Sprintf("probe scan cap of %d bytes reached (FR-031)", r.Matrix.Defaults.ScanCapBytes))
	}

	pc, err := r.personaClients(ctx, inst.Persona.Key, roleARN)
	if err != nil {
		return notRun(err.Error())
	}

	if c.Setup != "" {
		undo, err := r.setup(ctx, c.Setup, inst.Style, t)
		if undo != nil {
			defer func() {
				if err := undo(context.WithoutCancel(ctx)); err != nil {
					r.logf("WARNING: undo %s for %s failed: %v", c.Setup, inst.ID, err)
				}
			}()
		}
		if err != nil {
			return notRun(fmt.Sprintf("setup %s: %v", c.Setup, err))
		}
		if c.Expect != matrix.Allowed {
			sleep(ctx, time.Duration(r.Matrix.Defaults.ScenarioSettleSeconds)*time.Second)
		}
	}

	deadline := time.Now().Add(time.Duration(r.Matrix.Defaults.PositiveRetrySeconds) * time.Second)
	delay := 2 * time.Second
	var out ops.Result
	var obs Observation
	for {
		out, obs = r.dispatch(ctx, pc, inst, t)
		if out.Cleanup != nil {
			if err := out.Cleanup(context.WithoutCancel(ctx)); err != nil {
				r.logf("WARNING: cleanup for %s failed: %v", inst.ID, err)
			}
		}
		if !Retryable(c, out) || time.Now().Add(delay).After(deadline) {
			break
		}
		sleep(ctx, delay)
		delay = min(delay*2, 30*time.Second)
	}
	r.scanned.Add(out.ScanBytes)
	if r.Matrix.Mode == matrix.Probe && slicesContainsOp(matrix.QueryOps, c.Op) {
		out.Evidence = RedactQueryEvidence(out.Evidence)
	}
	res.Observed = report.Observed{Outcome: out.Outcome, Columns: out.Columns, RowCount: out.RowCount, ScanBytes: out.ScanBytes}
	res.Evidence = out.Evidence
	res.Status, res.Reason = Evaluate(c, out, obs)
	r.logf("%-10s %s", res.Status, inst.ID)
	return res
}

// RedactQueryEvidence keeps only the error code and request ID of a query result. Query engine
// errors can quote field values (cast and parse failures), and probe reports must carry no data
// values (FR-030), so in probe mode the message text never reaches a report or a reason. The
// outcome was already classified from the full text.
func RedactQueryEvidence(ev classify.Evidence) classify.Evidence {
	if ev.Message != "" {
		ev.Message = "(query error text withheld in probe mode)"
	}
	return ev
}

func slicesContainsOp(ops []matrix.Op, op matrix.Op) bool {
	for _, o := range ops {
		if o == op {
			return true
		}
	}
	return false
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
