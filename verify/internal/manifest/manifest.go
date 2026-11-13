// Package manifest loads the fixture manifest (verify/fixture output "manifest") and resolves
// symbolic matrix resources to concrete catalog, database, and workgroup targets.
package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/matrix"
)

// Namespace is one database in one grant style.
type Namespace struct {
	CatalogID       string `json:"catalog_id"`
	Database        string `json:"database"`
	TableBucketARN  string `json:"table_bucket_arn"`
	Workgroup       string `json:"workgroup"`
	ResultsLocation string `json:"results_location"`
}

// Column is a fixture table column.
type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Table is a fixture table definition.
type Table struct {
	Columns          []Column `json:"columns"`
	PartitionColumn  *string  `json:"partition_column"`
	RestrictedColumn *string  `json:"restricted_column"`
	MapColumn        *string  `json:"map_column"`
	Rows             int      `json:"rows"`
}

// ColumnNames returns the table's column names in order.
func (t Table) ColumnNames() []string {
	out := make([]string, len(t.Columns))
	for i, c := range t.Columns {
		out[i] = c.Name
	}
	return out
}

// Style is one grant style's namespaces and filters.
type Style struct {
	Namespaces map[string]Namespace `json:"namespaces"`
	TagKeys    *TagKeys             `json:"tag_keys,omitempty"`
	Filters    map[string]string    `json:"filters"`
}

// TagKeys are the label-style LF-tag keys.
type TagKeys struct {
	Tenant      map[string]string `json:"tenant"`
	Zone        string            `json:"zone"`
	Sensitivity string            `json:"sensitivity"`
	Domain      string            `json:"domain"`
	Regulations map[string]string `json:"regulations"`
	MW          string            `json:"mw"`
}

// Manifest is the fixture's identifiers.
type Manifest struct {
	RunID     string            `json:"run_id"`
	AccountID string            `json:"account_id"`
	Region    string            `json:"region"`
	Prefix    string            `json:"prefix"`
	Stage     int               `json:"stage"`
	Inject    string            `json:"inject"`
	Runner    string            `json:"runner"`
	Personas  map[string]string `json:"personas"`
	Styles    struct {
		Label Style `json:"label"`
		Named Style `json:"named"`
	} `json:"styles"`
	Tables    map[string]Table `json:"tables"`
	KMSKeyARN string           `json:"kms_key_arn"`
}

// Load reads a manifest written by `terraform output -json manifest`.
func Load(path string) (*Manifest, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- reads a file the operator names on the command line
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	if m.RunID == "" || len(m.Personas) == 0 {
		return nil, fmt.Errorf("manifest %s is missing run_id or personas", path)
	}
	return &m, nil
}

// Target is a resolved resource.
type Target struct {
	Namespace
	Key     string // namespace key in the manifest, for reports
	Table   string
	Columns []string
	// Exists reports whether the manifest says the database and table exist, so "not found"
	// responses can be classified as denials
	Exists bool
}

// Label returns a short description for reports.
func (t Target) Label() string {
	s := t.Key
	if t.Table != "" {
		s += "." + t.Table
	}
	if len(t.Columns) > 0 {
		s += "(" + strings.Join(t.Columns, ",") + ")"
	}
	return s
}

// AthenaCatalog is the query engine catalog name for an S3 Tables catalog ID
// (<account>:s3tablescatalog/<bucket> becomes s3tablescatalog/<bucket>).
func (t Target) AthenaCatalog() string {
	if i := strings.Index(t.CatalogID, ":"); i >= 0 {
		return t.CatalogID[i+1:]
	}
	return t.CatalogID
}

// Resolve maps a symbolic matrix resource to a target in one style.
func (m *Manifest) Resolve(style matrix.Style, r matrix.Resource) (Target, error) {
	var key string
	var ns Namespace
	var ok bool
	switch style {
	case matrix.Label:
		zone := r.Zone
		if zone == "" {
			zone = "curated"
		}
		key = r.Tenant + "/" + zone
		ns, ok = m.Styles.Label.Namespaces[key]
	case matrix.Named:
		key = "n_" + r.Tenant
		ns, ok = m.Styles.Named.Namespaces[key]
	default:
		return Target{}, fmt.Errorf("resolve needs label or named, got %q", style)
	}
	if r.Tenant == "" {
		// Account-level operations (settings, catalogs) still need a workgroup and catalog;
		// use the style's alpha namespace
		key = map[matrix.Style]string{matrix.Label: "alpha/curated", matrix.Named: "n_alpha"}[style]
		ns, ok = m.namespaces(style)[key]
	}
	if !ok {
		return Target{}, fmt.Errorf("no %s namespace %q in manifest", style, key)
	}
	_, known := m.Tables[r.Table]
	return Target{
		Namespace: ns,
		Key:       key,
		Table:     r.Table,
		Columns:   r.Columns,
		Exists:    r.Table == "" || known,
	}, nil
}

func (m *Manifest) namespaces(style matrix.Style) map[string]Namespace {
	if style == matrix.Label {
		return m.Styles.Label.Namespaces
	}
	return m.Styles.Named.Namespaces
}

// AllNamespaces returns every namespace in both styles, keyed "<style>:<key>".
func (m *Manifest) AllNamespaces() map[string]Namespace {
	out := map[string]Namespace{}
	for k, v := range m.Styles.Label.Namespaces {
		out["label:"+k] = v
	}
	for k, v := range m.Styles.Named.Namespaces {
		out["named:"+k] = v
	}
	return out
}
