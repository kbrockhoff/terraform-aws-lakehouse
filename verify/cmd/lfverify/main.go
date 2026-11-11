// Command lfverify verifies Lake Formation access by acting as each persona (spec 002).
//
// Full mode runs against a fixture the verify/fixture Terraform root deployed; probe mode
// observes an existing deployment without changing it. Exit status: 0 when every case passes,
// 1 when any case fails or does not run, 2 for usage errors and preflight refusals.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/athena"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	cttypes "github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	"github.com/aws/aws-sdk-go-v2/service/glue"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/lakeformation"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3tables"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/identity"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/inspect"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/manifest"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/matrix"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/ops"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/preflight"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/report"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/runner"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/seed"
	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/snapshot"
	"go.yaml.in/yaml/v3"
)

const usage = `usage: lfverify <command> [flags]

commands:
  preflight  check that a run may start (full: allow-list, settings owner; probe: read-only runner)
  seed       create fixture tables and rows as the fixture owner (full mode)
  run        execute a matrix and write report.json, junit.xml, report.md
  detect     apply each misconfiguration overlay and require a failing case (full mode)
  snapshot   capture probe target state (--name before|after)
  diff       compare the before and after snapshots; fails on any difference
  audit      list non-read-only CloudTrail events from the run's sessions (probe mode)
  cleanup    sweep a run's resources by prefix and restore the data-lake settings
  probe-matrix  derive a probe matrix from --matrix (full) and --manifest, written to --out
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	ctx := context.Background()
	err := dispatch(ctx, os.Args[1], os.Args[2:])
	switch {
	case err == nil:
	case errors.Is(err, errUsage), errors.Is(err, preflight.ErrRefused):
		fmt.Fprintln(os.Stderr, "lfverify:", err)
		os.Exit(2)
	default:
		fmt.Fprintln(os.Stderr, "lfverify:", err)
		os.Exit(1)
	}
}

var errUsage = errors.New("usage")

func logf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, time.Now().Format("15:04:05")+" "+format+"\n", a...)
}

type flags struct {
	fs                                        *flag.FlagSet
	mode, manifest, matrix, out, runID, allow string
	name, since, only, applyCmd               string
	workers                                   int
	staticOnly, tablesOnly                    bool
}

func parse(cmd string, args []string) (*flags, error) {
	f := &flags{fs: flag.NewFlagSet(cmd, flag.ContinueOnError)}
	f.fs.StringVar(&f.mode, "mode", "full", "full or probe")
	f.fs.StringVar(&f.manifest, "manifest", "", "fixture manifest JSON (full mode)")
	f.fs.StringVar(&f.matrix, "matrix", "", "expectation matrix YAML")
	f.fs.StringVar(&f.out, "out", "", "output directory (default verify/out/<run-id>)")
	f.fs.StringVar(&f.runID, "run-id", "", "run identifier (full mode reads it from the manifest)")
	f.fs.StringVar(&f.allow, "allow-list", "sandbox-accounts.txt", "full-mode account allow-list")
	f.fs.StringVar(&f.name, "name", "", "snapshot name: before or after")
	f.fs.StringVar(&f.since, "since", "", "audit start time (RFC 3339); default: 1 hour ago")
	f.fs.StringVar(&f.only, "only", "", "comma-separated persona keys to run")
	f.fs.StringVar(&f.applyCmd, "apply-cmd", "make -s verify-apply RUN_ID={run} INJECT={inject} INJECT_PERSONA={persona}", "command detect uses to apply an overlay, run from the repository root")
	f.fs.IntVar(&f.workers, "workers", 8, "parallel static cases")
	f.fs.BoolVar(&f.staticOnly, "static-only", false, "skip scenario and boundary cases")
	f.fs.BoolVar(&f.tablesOnly, "tables-only", false, "cleanup: drop the run's tables only, before terraform destroy")
	if err := f.fs.Parse(args); err != nil {
		return nil, fmt.Errorf("%w: %v", errUsage, err)
	}
	if f.mode != "full" && f.mode != "probe" {
		return nil, fmt.Errorf("%w: --mode must be full or probe", errUsage)
	}
	return f, nil
}

func (f *flags) loadManifest() (*manifest.Manifest, error) {
	if f.manifest == "" {
		return nil, fmt.Errorf("%w: --manifest is required in full mode", errUsage)
	}
	m, err := manifest.Load(f.manifest)
	if err != nil {
		return nil, err
	}
	if f.runID == "" {
		f.runID = m.RunID
	}
	return m, nil
}

func (f *flags) outDir() (string, error) {
	if f.runID == "" {
		return "", fmt.Errorf("%w: --run-id is required", errUsage)
	}
	dir := f.out
	if dir == "" {
		dir = filepath.Join("out", f.runID)
	}
	return dir, os.MkdirAll(dir, 0o750)
}

// env is the runner identity: its config, account, region, and role ARN.
type env struct {
	cfg     aws.Config
	account string
	role    string
}

func load(ctx context.Context) (*env, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	id, err := sts.NewFromConfig(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, fmt.Errorf("no valid AWS session: %w", err)
	}
	e := &env{cfg: cfg, account: aws.ToString(id.Account), role: aws.ToString(id.Arn)}
	// arn:aws:sts::<account>:assumed-role/<name>/<session> -> the role's IAM ARN, path included
	if parts := strings.Split(e.role, "/"); len(parts) == 3 && strings.Contains(parts[0], ":assumed-role") {
		out, err := iam.NewFromConfig(cfg).GetRole(ctx, &iam.GetRoleInput{RoleName: aws.String(parts[1])})
		if err != nil {
			return nil, fmt.Errorf("resolve runner role %s: %w", parts[1], err)
		}
		e.role = aws.ToString(out.Role.Arn)
	}
	return e, nil
}

func dispatch(ctx context.Context, cmd string, args []string) error {
	f, err := parse(cmd, args)
	if err != nil {
		return err
	}
	switch cmd {
	case "preflight":
		return cmdPreflight(ctx, f)
	case "seed":
		return cmdSeed(ctx, f)
	case "run":
		return cmdRun(ctx, f)
	case "detect":
		return cmdDetect(ctx, f)
	case "snapshot":
		return cmdSnapshot(ctx, f)
	case "diff":
		return cmdDiff(f)
	case "audit":
		return cmdAudit(ctx, f)
	case "cleanup":
		return cmdCleanup(ctx, f)
	case "probe-matrix":
		return cmdProbeMatrix(f)
	}
	return fmt.Errorf("%w: unknown command %q\n%s", errUsage, cmd, usage)
}

func cmdPreflight(ctx context.Context, f *flags) error {
	e, err := load(ctx)
	if err != nil {
		return err
	}
	lf := lakeformation.NewFromConfig(e.cfg)
	if f.mode == "probe" {
		if err := preflight.Probe(ctx, iam.NewFromConfig(e.cfg), lf, preflight.ProbeInput{Runner: e.role}); err != nil {
			return err
		}
		m, err := matrix.Load(f.matrix)
		if err != nil {
			return err
		}
		if err := preflight.NoOpenDefaults(ctx, lf, probeDatabases(m)); err != nil {
			return err
		}
		logf("probe preflight passed: runner %s is read-only", e.role)
		return nil
	}
	allow, err := preflight.LoadAllowList(f.allow)
	if err != nil {
		return err
	}
	if f.runID == "" {
		return fmt.Errorf("%w: --run-id is required", errUsage)
	}
	dir, err := f.outDir()
	if err != nil {
		return err
	}
	saved := filepath.Join(dir, "settings.before.json")
	if _, err := os.Stat(saved); err == nil {
		logf("settings snapshot %s already exists; keeping the original", saved)
	}
	var fixtureAdmins []string
	if f.manifest != "" {
		if m, err := manifest.Load(f.manifest); err == nil {
			fixtureAdmins = append(fixtureAdmins, m.Personas["admin"])
		}
	}
	settings, err := preflight.Full(ctx, lf, preflight.FullInput{
		Account: e.account, Region: e.cfg.Region, Runner: e.role, Env: os.Getenv("LFVERIFY_ENV"),
		AllowList: allow, FixtureAdmins: fixtureAdmins,
	})
	if err != nil {
		return err
	}
	if _, err := os.Stat(saved); errors.Is(err, os.ErrNotExist) {
		if err := preflight.SaveSettings(saved, settings); err != nil {
			return err
		}
	}
	if f.manifest != "" {
		m, err := f.loadManifest()
		if err != nil {
			return err
		}
		var dbs []preflight.Database
		for _, ns := range m.AllNamespaces() {
			dbs = append(dbs, preflight.Database{CatalogID: ns.CatalogID, Name: ns.Database})
		}
		if err := preflight.NoOpenDefaults(ctx, lf, dbs); err != nil {
			return err
		}
	}
	logf("full preflight passed for %s %s; settings saved to %s", e.account, e.cfg.Region, saved)
	return nil
}

func probeDatabases(m *matrix.Matrix) []preflight.Database {
	seen := map[preflight.Database]bool{}
	var out []preflight.Database
	for _, p := range m.Personas {
		for _, c := range p.Cases {
			db := preflight.Database{CatalogID: c.Resource.CatalogID, Name: c.Resource.Database}
			if db.Name != "" && !seen[db] {
				seen[db] = true
				out = append(out, db)
			}
		}
	}
	return out
}

func cmdSeed(ctx context.Context, f *flags) error {
	m, err := f.loadManifest()
	if err != nil {
		return err
	}
	e, err := load(ctx)
	if err != nil {
		return err
	}
	s := &seed.Seeder{Cfg: e.cfg, Runner: ops.NewClients(e.cfg), Manifest: m, Log: logf}
	return s.Seed(ctx)
}

func newRunner(ctx context.Context, f *flags, e *env, m *matrix.Matrix, man *manifest.Manifest) (*runner.Runner, error) {
	as := &identity.Assumer{STS: sts.NewFromConfig(e.cfg), Base: e.cfg, RunID: f.runID}
	if m.Mode == matrix.Probe {
		policy, err := probePolicy(m)
		if err != nil {
			return nil, err
		}
		as.Policy = policy
	}
	rc := ops.NewClients(e.cfg)
	r := &runner.Runner{
		Matrix: m, Manifest: man, Assumer: as, Runner: rc, Inspector: inspect.Inspector{LF: rc.LF},
		AccountID: e.account, RunID: f.runID, Workers: f.workers, StaticOnly: f.staticOnly, Log: logf,
	}
	if f.only != "" {
		r.Only = strings.Split(f.only, ",")
	}
	if man != nil {
		s := &seed.Seeder{Cfg: e.cfg, Runner: rc, Manifest: man, Log: logf}
		r.Restore = s.Restore
	}
	if f.manifest != "" {
		// The manifest is written right after the last fixture apply, so its age is the settle
		// reference
		if st, err := os.Stat(f.manifest); err == nil {
			r.SettledSince = st.ModTime()
		}
	}
	return r, nil
}

// probePolicy scopes probe sessions to read-only calls plus result writes in the matrix's
// workgroups' result locations. Athena results go to the workgroup's enforced location, which
// probe matrices do not name, so writes are limited to buckets by name pattern instead.
func probePolicy(m *matrix.Matrix) (string, error) {
	var buckets, objects []string
	for _, b := range strings.Split(os.Getenv("LFVERIFY_PROBE_RESULTS_BUCKETS"), ",") {
		if b = strings.TrimSpace(b); b != "" {
			buckets = append(buckets, "arn:aws:s3:::"+b)
			objects = append(objects, "arn:aws:s3:::"+b+"/*")
		}
	}
	var keys []string
	for _, k := range strings.Split(os.Getenv("LFVERIFY_PROBE_KMS_KEYS"), ",") {
		if k = strings.TrimSpace(k); k != "" {
			keys = append(keys, k)
		}
	}
	return identity.ProbeSessionPolicy(objects, buckets, keys)
}

func cmdRun(ctx context.Context, f *flags) error {
	m, err := matrix.Load(f.matrix)
	if err != nil {
		return err
	}
	if string(m.Mode) != f.mode {
		return fmt.Errorf("%w: matrix mode %s does not match --mode %s", errUsage, m.Mode, f.mode)
	}
	var man *manifest.Manifest
	if m.Mode == matrix.Full {
		if man, err = f.loadManifest(); err != nil {
			return err
		}
	} else if f.runID == "" {
		f.runID = fmt.Sprintf("%06x", time.Now().UnixNano()&0xffffff)
	}
	dir, err := f.outDir()
	if err != nil {
		return err
	}
	e, err := load(ctx)
	if err != nil {
		return err
	}
	r, err := newRunner(ctx, f, e, m, man)
	if err != nil {
		return err
	}
	run := r.Run(ctx)
	if err := run.WriteAll(dir); err != nil {
		return err
	}
	t := run.Totals()
	logf("%d cases: %d pass, %d verified by inspection, %d fail, %d not run; reports in %s",
		len(run.Results), t[report.Pass], t[report.VerifiedByInspection], t[report.Fail], t[report.NotRun], dir)
	if run.Failed() {
		return errors.New("some cases failed or did not run")
	}
	return nil
}

// detectScenarios returns each overlay: one extra and one missing grant per reader or writer
// persona, plus the column, row, and open-default overlays (Detection SLO).
func detectScenarios(personas []string) [][2]string {
	var out [][2]string
	for _, p := range personas {
		if strings.HasPrefix(p, "reader_") || strings.HasPrefix(p, "writer_") {
			out = append(out, [2]string{"extra_grant", p}, [2]string{"missing_grant", p})
		}
	}
	return append(out, [2]string{"drop_column_restriction", "reader_cols"}, [2]string{"drop_row_filter", "reader_rows"}, [2]string{"open_default", ""})
}

func cmdDetect(ctx context.Context, f *flags) error {
	man, err := f.loadManifest()
	if err != nil {
		return err
	}
	m, err := matrix.Load(f.matrix)
	if err != nil {
		return err
	}
	dir, err := f.outDir()
	if err != nil {
		return err
	}
	e, err := load(ctx)
	if err != nil {
		return err
	}
	var keys []string
	for _, p := range m.Personas {
		keys = append(keys, p.Key)
	}
	type outcome struct {
		Inject, Persona string
		Failing         []string
		Detected        bool
		Error           string `json:",omitempty"`
	}
	var results []outcome
	apply := func(inject, persona string) error {
		c := strings.NewReplacer("{run}", f.runID, "{inject}", inject, "{persona}", persona).Replace(f.applyCmd)
		logf("apply overlay: %s", c)
		cmd := exec.CommandContext(ctx, "sh", "-c", c)
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		return cmd.Run()
	}
	missed := 0
	for _, sc := range detectScenarios(keys) {
		o := outcome{Inject: sc[0], Persona: sc[1]}
		if err := apply(sc[0], sc[1]); err != nil {
			o.Error = err.Error()
			results = append(results, o)
			missed++
			continue
		}
		only := []string{sc[1]}
		if sc[0] == "open_default" {
			only = nil
		}
		sub := *f
		sub.staticOnly = true
		r, err := newRunner(ctx, &sub, e, m, man)
		if err != nil {
			return err
		}
		r.Only = only
		if sc[0] == "open_default" {
			// The overlay is caught by the open-default preflight check (FR-008)
			var dbs []preflight.Database
			for _, ns := range man.AllNamespaces() {
				dbs = append(dbs, preflight.Database{CatalogID: ns.CatalogID, Name: ns.Database})
			}
			if err := preflight.NoOpenDefaults(ctx, lakeformation.NewFromConfig(e.cfg), dbs); err != nil {
				o.Failing = []string{"preflight: " + err.Error()}
			}
		} else {
			for _, res := range r.Run(ctx).Results {
				if res.Status == report.Fail {
					o.Failing = append(o.Failing, res.ID)
				}
			}
		}
		o.Detected = len(o.Failing) > 0
		if !o.Detected {
			missed++
		}
		results = append(results, o)
		logf("%s %s: detected=%v (%d failing cases)", sc[0], sc[1], o.Detected, len(o.Failing))
	}
	if err := apply("", ""); err != nil {
		logf("WARNING: reverting overlays failed: %v", err)
	}
	if err := writeJSON(filepath.Join(dir, "detect.json"), results); err != nil {
		return err
	}
	if missed > 0 {
		return fmt.Errorf("%d of %d overlays were not detected (see detect.json)", missed, len(results))
	}
	return nil
}

func cmdSnapshot(ctx context.Context, f *flags) error {
	if f.name != "before" && f.name != "after" {
		return fmt.Errorf("%w: --name must be before or after", errUsage)
	}
	m, err := matrix.Load(f.matrix)
	if err != nil {
		return err
	}
	dir, err := f.outDir()
	if err != nil {
		return err
	}
	e, err := load(ctx)
	if err != nil {
		return err
	}
	byCatalog := map[string][]string{}
	for _, db := range probeDatabases(m) {
		if !slices.Contains(byCatalog[db.CatalogID], db.Name) {
			byCatalog[db.CatalogID] = append(byCatalog[db.CatalogID], db.Name)
		}
	}
	var cats []snapshot.Catalog
	for c, dbs := range byCatalog {
		cats = append(cats, snapshot.Catalog{CatalogID: c, Databases: dbs})
	}
	rc := ops.NewClients(e.cfg)
	s, err := snapshot.Capture(ctx, rc.Glue, rc.LF, cats)
	if err != nil {
		return err
	}
	return s.Save(filepath.Join(dir, "snapshot."+f.name+".json"))
}

func cmdDiff(f *flags) error {
	dir, err := f.outDir()
	if err != nil {
		return err
	}
	before, err := snapshot.Load(filepath.Join(dir, "snapshot.before.json"))
	if err != nil {
		return err
	}
	after, err := snapshot.Load(filepath.Join(dir, "snapshot.after.json"))
	if err != nil {
		return err
	}
	d := snapshot.Diff(before, after)
	if err := os.WriteFile(filepath.Join(dir, "diff.txt"), []byte(strings.Join(d, "\n")+"\n"), 0o600); err != nil {
		return err
	}
	if len(d) > 0 {
		for _, l := range d {
			fmt.Println(l)
		}
		return fmt.Errorf("probe target changed: %d differences", len(d))
	}
	logf("no differences between before and after snapshots")
	return nil
}

func cmdAudit(ctx context.Context, f *flags) error {
	if f.runID == "" {
		return fmt.Errorf("%w: --run-id is required", errUsage)
	}
	since := time.Now().Add(-time.Hour)
	if f.since != "" {
		t, err := time.Parse(time.RFC3339, f.since)
		if err != nil {
			return fmt.Errorf("%w: --since: %v", errUsage, err)
		}
		since = t
	}
	if time.Since(since) < 15*time.Minute {
		logf("WARNING: CloudTrail delivers events up to 15 minutes late; rerun audit later for a final answer")
	}
	e, err := load(ctx)
	if err != nil {
		return err
	}
	ct := cloudtrail.NewFromConfig(e.cfg)
	prefix := identity.SessionName(f.runID, "")
	var changes []string
	p := cloudtrail.NewLookupEventsPaginator(ct, &cloudtrail.LookupEventsInput{
		StartTime:        aws.Time(since),
		LookupAttributes: []cttypes.LookupAttribute{{AttributeKey: cttypes.LookupAttributeKeyReadOnly, AttributeValue: aws.String("false")}},
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, ev := range page.Events {
			if strings.HasPrefix(aws.ToString(ev.Username), prefix) {
				changes = append(changes, fmt.Sprintf("%s %s %s", aws.ToTime(ev.EventTime).Format(time.RFC3339), aws.ToString(ev.Username), aws.ToString(ev.EventName)))
			}
		}
	}
	if len(changes) > 0 {
		for _, c := range changes {
			fmt.Println(c)
		}
		return fmt.Errorf("%d change events from run %s sessions", len(changes), f.runID)
	}
	logf("no change events from sessions %s* since %s", prefix, since.Format(time.RFC3339))
	return nil
}

func cmdCleanup(ctx context.Context, f *flags) error {
	if f.runID == "" {
		return fmt.Errorf("%w: --run-id is required", errUsage)
	}
	dir, err := f.outDir()
	if err != nil {
		return err
	}
	e, err := load(ctx)
	if err != nil {
		return err
	}
	sw := &preflight.Sweeper{
		LF: lakeformation.NewFromConfig(e.cfg), S3Tables: s3tables.NewFromConfig(e.cfg), Athena: athena.NewFromConfig(e.cfg),
		S3: s3.NewFromConfig(e.cfg), IAM: iam.NewFromConfig(e.cfg), Glue: glue.NewFromConfig(e.cfg), KMS: kms.NewFromConfig(e.cfg),
		Prefix: "lfv-" + f.runID, Log: logf,
	}
	if f.tablesOnly {
		return sw.DropTables(ctx)
	}
	sweepErr := sw.Sweep(ctx)
	saved := filepath.Join(dir, "settings.before.json")
	s, err := preflight.LoadSettings(saved)
	if err != nil {
		return errors.Join(sweepErr, fmt.Errorf("no settings snapshot to restore (%s): %w", saved, err))
	}
	if err := preflight.RestoreSettings(ctx, sw.LF, s); err != nil {
		return errors.Join(sweepErr, err)
	}
	logf("data-lake settings restored from %s", saved)
	return sweepErr
}

func cmdProbeMatrix(f *flags) error {
	man, err := f.loadManifest()
	if err != nil {
		return err
	}
	full, err := matrix.Load(f.matrix)
	if err != nil {
		return err
	}
	p, err := manifest.ProbeFromFull(full, man)
	if err != nil {
		return err
	}
	if f.out == "" {
		return fmt.Errorf("%w: --out is the probe matrix file to write", errUsage)
	}
	b, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	header := "# Generated by lfverify probe-matrix from " + filepath.Base(f.matrix) + " and run " + man.RunID + "; do not edit.\n"
	return os.WriteFile(f.out, append([]byte(header), b...), 0o600)
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}
