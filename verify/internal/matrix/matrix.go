package matrix

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/predicate"
	"go.yaml.in/yaml/v3"
)

// Mode selects how the matrix runs: full mode attempts every case against fixtures, probe mode
// observes an existing deployment without changing it (FR-002).
type Mode string

const (
	Full  Mode = "full"
	Probe Mode = "probe"
)

// Style is the grant style a case targets. Both expands into one case per style.
type Style string

const (
	Label Style = "label"
	Named Style = "named"
	Both  Style = "both"
)

// Phase orders cases: static cases run in parallel, scenario cases mutate and revert one at a
// time, and boundary cases attempt invalid grants (FR-021).
type Phase string

const (
	Static   Phase = "static"
	Scenario Phase = "scenario"
	Boundary Phase = "boundary"
)

// Expect is the expected outcome of a case.
type Expect string

const (
	Allowed  Expect = "allowed"
	Denied   Expect = "denied"
	Rejected Expect = "rejected"
	// Absent means the call succeeds but the resource or the listed columns are not returned
	Absent Expect = "absent"
	// Inspect compares the persona's effective permissions instead of attempting the operation
	// (probe mode, FR-029)
	Inspect Expect = "inspect"
)

// Op is an operation a persona attempts.
type Op string

const (
	ListCatalogs    Op = "list_catalogs"
	ListDatabases   Op = "list_databases"
	GetDatabase     Op = "get_database"
	ListTables      Op = "list_tables"
	GetTable        Op = "get_table"
	Select          Op = "select"
	SelectColumns   Op = "select_columns"
	Count           Op = "count"
	Insert          Op = "insert"
	Update          Op = "update"
	Delete          Op = "delete"
	CreateTable     Op = "create_table"
	AlterTable      Op = "alter_table"
	DropTable       Op = "drop_table"
	Grant           Op = "grant"
	Revoke          Op = "revoke"
	AddLFTags       Op = "add_lf_tags"
	CreateLFTag     Op = "create_lf_tag"
	CreateFilter    Op = "create_filter"
	ListPermissions Op = "list_permissions"
	GetSettings     Op = "get_settings"
	PutSettings     Op = "put_settings"
	StorageRead     Op = "storage_read"

	// Rule-boundary operations (FR-021): each attempts a grant the permission service must reject
	GrantColumnsWithWrite Op = "grant_columns_with_write"
	GrantExcludePartition Op = "grant_exclude_partition"
	CreateFilterMapColumn Op = "create_filter_map_column"
)

var readOps = []Op{
	ListCatalogs, ListDatabases, GetDatabase, ListTables, GetTable, Select, SelectColumns, Count,
	ListPermissions, GetSettings,
}

var writeOps = []Op{
	Insert, Update, Delete, CreateTable, AlterTable, DropTable, Grant, Revoke, AddLFTags,
	CreateLFTag, CreateFilter, PutSettings, GrantColumnsWithWrite, GrantExcludePartition,
	CreateFilterMapColumn,
}

// QueryOps run through the query engine and need a workgroup.
var QueryOps = []Op{Select, SelectColumns, Count, Insert, Update, Delete}

// IsWrite reports whether op changes the target. Probe mode never attempts these (FR-027).
func (o Op) IsWrite() bool { return slices.Contains(writeOps, o) }

func (o Op) known() bool {
	return slices.Contains(readOps, o) || slices.Contains(writeOps, o) || o == StorageRead
}

// Setup is a mutation the runner applies before one scenario case.
type Setup string

const (
	// RelabelColumnRestricted labels orders.amount restricted (FR-017)
	RelabelColumnRestricted Setup = "relabel_column_restricted"
	// RelabelTableRestricted labels the customers table restricted (FR-023)
	RelabelTableRestricted Setup = "relabel_table_restricted"
	// LateTable creates late_table after the grants exist (FR-013)
	LateTable Setup = "late_table"
	// AllRowsFilter grants reader_rows an all-rows filter on orders (FR-018)
	AllRowsFilter Setup = "all_rows_filter"
)

// Setups lists the known setup mutations.
var Setups = []Setup{RelabelColumnRestricted, RelabelTableRestricted, LateTable, AllRowsFilter}

// Defaults are run-wide settings; zero values in the file take these defaults.
type Defaults struct {
	SettleSeconds int `yaml:"settle_seconds"`
	// ScenarioSettleSeconds is the wait after each scenario mutation before a negative check
	ScenarioSettleSeconds int   `yaml:"scenario_settle_seconds"`
	PositiveRetrySeconds  int   `yaml:"positive_retry_seconds"`
	QueryRowLimit         int   `yaml:"query_row_limit"`
	ScanCapBytes          int64 `yaml:"scan_cap_bytes"`
}

// Resource names what a case acts on. Full mode uses symbolic names that the fixture manifest
// resolves: Tenant (alpha, beta, gamma) maps to the label namespace <tenant>/<zone> and the
// named namespace n_<tenant>. Probe mode names catalogs, databases, and workgroups directly.
type Resource struct {
	Tenant  string   `yaml:"tenant,omitempty"`
	Zone    string   `yaml:"zone,omitempty"`
	Table   string   `yaml:"table,omitempty"`
	Columns []string `yaml:"columns,omitempty"`

	CatalogID string `yaml:"catalog_id,omitempty"`
	Database  string `yaml:"database,omitempty"`
	Workgroup string `yaml:"workgroup,omitempty"`
	// Grantee is the persona key that grant and revoke cases target
	Grantee string `yaml:"grantee,omitempty"`
}

// Rows is the expected query result size and, in full mode only, the expected key values.
type Rows struct {
	Count     *int     `yaml:"count,omitempty"`
	KeyColumn string   `yaml:"key_column,omitempty"`
	Keys      []string `yaml:"keys,omitempty"`
}

// Case is one operation a persona attempts, with its expected outcome.
type Case struct {
	ID       string   `yaml:"id"`
	Style    Style    `yaml:"style"`
	Phase    Phase    `yaml:"phase"`
	Op       Op       `yaml:"op"`
	Resource Resource `yaml:"resource"`
	Expect   Expect   `yaml:"expect"`

	ExpectColumns     []string             `yaml:"expect_columns,omitempty"`
	AbsentColumns     []string             `yaml:"absent_columns,omitempty"`
	ExpectPermissions []string             `yaml:"expect_permissions,omitempty"`
	ExpectRows        *Rows                `yaml:"expect_rows,omitempty"`
	RowPredicate      *predicate.Predicate `yaml:"row_predicate,omitempty"`
	MaxRows           *int                 `yaml:"max_rows,omitempty"`

	// Setup names a mutation the runner applies before the case and reverts after it (full
	// mode, phase scenario only)
	Setup Setup `yaml:"setup,omitempty"`

	// Rule cites the documented permission rule the expectation encodes
	Rule string `yaml:"rule,omitempty"`
	Note string `yaml:"note,omitempty"`
}

// Persona is one identity under test.
type Persona struct {
	Key string `yaml:"key"`
	// RoleARN is required in probe mode; full mode resolves it from the manifest
	RoleARN string `yaml:"role_arn,omitempty"`
	Cases   []Case `yaml:"cases"`
}

// Matrix is the declared expectation set (FR-026).
type Matrix struct {
	Mode     Mode      `yaml:"mode"`
	Defaults Defaults  `yaml:"defaults"`
	Personas []Persona `yaml:"personas"`
}

// Load reads, defaults, and validates a matrix file. Unknown fields are errors, so a typo never
// silently drops an expectation.
func Load(path string) (*Matrix, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

// Parse decodes and validates matrix YAML.
func Parse(b []byte) (*Matrix, error) {
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	var m Matrix
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("decode matrix: %w", err)
	}
	m.applyDefaults()
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

func (m *Matrix) applyDefaults() {
	d := &m.Defaults
	if d.SettleSeconds == 0 {
		d.SettleSeconds = 120
	}
	if d.ScenarioSettleSeconds == 0 {
		d.ScenarioSettleSeconds = 30
	}
	if d.PositiveRetrySeconds == 0 {
		d.PositiveRetrySeconds = 120
	}
	if d.QueryRowLimit == 0 {
		d.QueryRowLimit = 10
	}
	if d.ScanCapBytes == 0 {
		d.ScanCapBytes = 1 << 30
	}
	for i := range m.Personas {
		for j := range m.Personas[i].Cases {
			c := &m.Personas[i].Cases[j]
			if c.Style == "" {
				c.Style = Both
			}
			if c.Phase == "" {
				c.Phase = Static
			}
			if m.Mode == Full && c.Resource.Tenant != "" && c.Resource.Zone == "" {
				c.Resource.Zone = "curated"
			}
		}
	}
}

// Validate checks every rule that keeps a matrix meaningful and, in probe mode, harmless.
func (m *Matrix) Validate() error {
	var errs []error
	if m.Mode != Full && m.Mode != Probe {
		errs = append(errs, fmt.Errorf("mode must be full or probe, got %q", m.Mode))
	}
	if len(m.Personas) == 0 {
		errs = append(errs, errors.New("matrix has no personas"))
	}
	ids := map[string]bool{}
	keys := map[string]bool{}
	for _, p := range m.Personas {
		if p.Key == "" {
			errs = append(errs, errors.New("persona with empty key"))
		}
		if keys[p.Key] {
			errs = append(errs, fmt.Errorf("duplicate persona %q", p.Key))
		}
		keys[p.Key] = true
		if m.Mode == Probe && p.RoleARN == "" {
			errs = append(errs, fmt.Errorf("persona %s: role_arn is required in probe mode", p.Key))
		}
		if m.Mode == Full && p.RoleARN != "" {
			errs = append(errs, fmt.Errorf("persona %s: role_arn comes from the manifest in full mode", p.Key))
		}
		for _, c := range p.Cases {
			if ids[c.ID] {
				errs = append(errs, fmt.Errorf("duplicate case id %q", c.ID))
			}
			ids[c.ID] = true
			for _, err := range m.validateCase(c) {
				errs = append(errs, fmt.Errorf("persona %s case %s: %w", p.Key, c.ID, err))
			}
		}
	}
	return errors.Join(errs...)
}

func (m *Matrix) validateCase(c Case) []error {
	var errs []error
	add := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }
	if c.ID == "" {
		add("empty id")
	}
	if !c.Op.known() {
		add("unknown op %q", c.Op)
	}
	if !slices.Contains([]Style{Label, Named, Both}, c.Style) {
		add("unknown style %q", c.Style)
	}
	if !slices.Contains([]Phase{Static, Scenario, Boundary}, c.Phase) {
		add("unknown phase %q", c.Phase)
	}
	if !slices.Contains([]Expect{Allowed, Denied, Rejected, Absent, Inspect}, c.Expect) {
		add("unknown expect %q", c.Expect)
	}
	if c.Expect == Absent && len(c.AbsentColumns) == 0 && !slices.Contains([]Op{ListCatalogs, ListDatabases, ListTables}, c.Op) {
		add("expect absent needs absent_columns, or a list op")
	}
	if c.Setup != "" {
		if !slices.Contains(Setups, c.Setup) {
			add("unknown setup %q", c.Setup)
		}
		if c.Phase != Scenario {
			add("a setup case needs phase: scenario")
		}
	}
	if c.RowPredicate != nil {
		if err := c.RowPredicate.Validate(); err != nil {
			add("row_predicate: %w", err)
		}
	}
	switch m.Mode {
	case Probe:
		if c.Op.IsWrite() && c.Expect != Inspect {
			add("probe mode never attempts %s; use expect: inspect (FR-027, FR-029)", c.Op)
		}
		if c.Phase != Static {
			add("probe mode runs static cases only")
		}
		if c.ExpectRows != nil && len(c.ExpectRows.Keys) > 0 {
			add("probe mode cannot declare row key values (FR-030)")
		}
		if c.Expect == Inspect && len(c.ExpectPermissions) == 0 {
			add("expect inspect needs expect_permissions")
		}
		if c.Op != GetSettings && c.Op != ListCatalogs && (c.Resource.CatalogID == "" || c.Resource.Database == "") {
			add("probe resources need catalog_id and database")
		}
		if slices.Contains(QueryOps, c.Op) && c.Expect != Inspect && c.Resource.Workgroup == "" {
			add("probe query cases need a workgroup")
		}
	case Full:
		if c.Expect == Inspect {
			add("expect inspect is for probe mode; full mode attempts the operation")
		}
		if c.Resource.CatalogID != "" || c.Resource.Database != "" || c.Resource.Workgroup != "" {
			add("full mode resources use symbolic tenant names, not catalog_id, database, or workgroup")
		}
		if c.Style == Named && c.Resource.Zone != "" && c.Resource.Zone != "curated" {
			add("the named baseline mirrors curated namespaces only")
		}
	}
	return errs
}

// Instance is one case bound to one style. Cases with style both expand into two instances.
type Instance struct {
	ID      string
	Persona Persona
	Case    Case
	Style   Style
}

// Expand returns every case instance, label before named, in file order.
func (m *Matrix) Expand() []Instance {
	var out []Instance
	for _, p := range m.Personas {
		for _, c := range p.Cases {
			styles := []Style{c.Style}
			if c.Style == Both {
				styles = []Style{Label, Named}
			}
			if m.Mode == Probe {
				styles = []Style{c.Style}
			}
			for _, s := range styles {
				id := c.ID
				if c.Style == Both && m.Mode == Full {
					id = c.ID + "/" + string(s)
				}
				out = append(out, Instance{ID: id, Persona: p, Case: c, Style: s})
			}
		}
	}
	return out
}
