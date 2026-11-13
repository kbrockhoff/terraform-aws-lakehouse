package report

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/classify"
)

// Status is a case result. NotRun and Fail both fail the run (FR-025).
type Status string

const (
	Pass                 Status = "pass"
	Fail                 Status = "fail"
	NotRun               Status = "not-run"
	VerifiedByInspection Status = "verified-by-inspection"
)

// Observed is what the case saw. It holds column names and counts only: there is deliberately
// no field that could carry a row value (FR-030).
type Observed struct {
	Outcome     classify.Outcome `json:"outcome,omitempty"`
	Columns     []string         `json:"columns,omitempty"`
	RowCount    *int             `json:"row_count,omitempty"`
	Permissions []string         `json:"permissions,omitempty"`
	ScanBytes   int64            `json:"scan_bytes,omitempty"`
}

// Result is one executed case instance.
type Result struct {
	ID       string            `json:"id"`
	Persona  string            `json:"persona"`
	Style    string            `json:"style"`
	Op       string            `json:"op"`
	Resource string            `json:"resource"`
	Expected string            `json:"expected"`
	Observed Observed          `json:"observed"`
	Status   Status            `json:"status"`
	Reason   string            `json:"reason,omitempty"`
	Rule     string            `json:"rule,omitempty"`
	Evidence classify.Evidence `json:"evidence,omitempty"`
	Duration time.Duration     `json:"duration_ns"`
}

// Failed reports whether the result fails the run.
func (r Result) Failed() bool { return r.Status == Fail || r.Status == NotRun }

// Run is a whole run's results.
type Run struct {
	RunID    string    `json:"run_id"`
	Mode     string    `json:"mode"`
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished"`
	Results  []Result  `json:"results"`
}

// Totals counts results by status.
func (r *Run) Totals() map[Status]int {
	t := map[Status]int{}
	for _, res := range r.Results {
		t[res.Status]++
	}
	return t
}

// Failed reports whether any result fails the run.
func (r *Run) Failed() bool {
	for _, res := range r.Results {
		if res.Failed() {
			return true
		}
	}
	return false
}

// WriteAll writes report.json, junit.xml, and report.md into dir.
func (r *Run) WriteAll(dir string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	for name, write := range map[string]func(io.Writer) error{
		"report.json": r.WriteJSON,
		"junit.xml":   r.WriteJUnit,
		"report.md":   r.WriteMarkdown,
	} {
		f, err := os.Create(filepath.Join(dir, name)) // #nosec G304 -- fixed report names inside the run's output directory
		if err != nil {
			return err
		}
		if err := write(f); err != nil {
			_ = f.Close() // the write error is the one to report
			return fmt.Errorf("%s: %w", name, err)
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return nil
}

// WriteJSON writes the full case list.
func (r *Run) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

type junitSuites struct {
	XMLName xml.Name     `xml:"testsuites"`
	Suites  []junitSuite `xml:"testsuite"`
}

type junitSuite struct {
	Name     string      `xml:"name,attr"`
	Tests    int         `xml:"tests,attr"`
	Failures int         `xml:"failures,attr"`
	Errors   int         `xml:"errors,attr"`
	Cases    []junitCase `xml:"testcase"`
}

type junitCase struct {
	Name      string        `xml:"name,attr"`
	ClassName string        `xml:"classname,attr"`
	Time      float64       `xml:"time,attr"`
	Failure   *junitMessage `xml:"failure,omitempty"`
	Error     *junitMessage `xml:"error,omitempty"`
}

type junitMessage struct {
	Message string `xml:"message,attr"`
	Body    string `xml:",chardata"`
}

// WriteJUnit writes one testsuite per persona and style. Fail is a failure and not-run is an
// error, so CI shows both.
func (r *Run) WriteJUnit(w io.Writer) error {
	suites := map[string]*junitSuite{}
	for _, res := range r.Results {
		name := res.Persona + "/" + res.Style
		s := suites[name]
		if s == nil {
			s = &junitSuite{Name: name}
			suites[name] = s
		}
		c := junitCase{Name: res.ID, ClassName: name, Time: res.Duration.Seconds()}
		msg := fmt.Sprintf("expected %s, observed %s", res.Expected, res.Observed.Outcome)
		body := res.Reason + "\n" + res.Evidence.Code + " " + res.Evidence.Message + " " + res.Evidence.RequestID
		switch res.Status {
		case Fail:
			c.Failure = &junitMessage{Message: msg, Body: body}
			s.Failures++
		case NotRun:
			c.Error = &junitMessage{Message: "not run: " + res.Reason, Body: body}
			s.Errors++
		}
		s.Tests++
		s.Cases = append(s.Cases, c)
	}
	out := junitSuites{}
	for _, name := range sortedKeys(suites) {
		out.Suites = append(out.Suites, *suites[name])
	}
	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	return enc.Encode(out)
}

// WriteMarkdown writes a summary and one table per persona.
func (r *Run) WriteMarkdown(w io.Writer) error {
	var b strings.Builder
	t := r.Totals()
	fmt.Fprintf(&b, "# lfverify %s run %s\n\n", r.Mode, r.RunID)
	fmt.Fprintf(&b, "%d cases: %d pass, %d verified by inspection, %d fail, %d not run. Duration %s.\n\n",
		len(r.Results), t[Pass], t[VerifiedByInspection], t[Fail], t[NotRun], r.Finished.Sub(r.Started).Round(time.Second))
	byPersona := map[string][]Result{}
	for _, res := range r.Results {
		byPersona[res.Persona] = append(byPersona[res.Persona], res)
	}
	for _, p := range sortedKeys(byPersona) {
		fmt.Fprintf(&b, "## %s\n\n| Case | Style | Op | Resource | Expected | Observed | Status | Reason |\n|---|---|---|---|---|---|---|---|\n", p)
		for _, res := range byPersona[p] {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s |\n",
				res.ID, res.Style, res.Op, cell(res.Resource), res.Expected, observedCell(res.Observed), res.Status, cell(res.Reason))
		}
		b.WriteString("\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func observedCell(o Observed) string {
	s := string(o.Outcome)
	if o.RowCount != nil {
		s += fmt.Sprintf(", %d rows", *o.RowCount)
	}
	if len(o.Columns) > 0 {
		s += ", columns " + strings.Join(o.Columns, " ")
	}
	return cell(s)
}

func cell(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "|", `\|`), "\n", " ") }

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
