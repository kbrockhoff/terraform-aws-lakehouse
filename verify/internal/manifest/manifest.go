// Package manifest loads the fixture manifest (verify/fixture output "manifest") and resolves
// symbolic matrix resources to concrete catalog, database, and workgroup targets.
package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/matrix"
)

// Namespace is one database in one grant style.
type Namespace struct {
	CatalogID      string `json:"catalog_id"`
	Database       string `json:"database"`
	TableBucketARN string `json:"table_bucket_arn"`
	Workgroup      string `json:"workgroup"`
	// DMLWorkgroup runs INSERT, UPDATE, and DELETE: a result-encrypted workgroup cannot run
	// them against S3 Tables (awslkhs-cdq). Empty means use Workgroup.
	DMLWorkgroup    string `json:"dml_workgroup,omitempty"`
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

// Share is one cross-account share: the consumer account and the table grant's LF-tag
// expression.
type Share struct {
	AccountID  string    `json:"account_id"`
	Expression []TagTerm `json:"expression"`
}

// TagTerm is one LF-tag expression term.
type TagTerm struct {
	Key    string   `json:"key"`
	Values []string `json:"values"`
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
	// AccountSettings are the values the spec 003 check cases expect (absent in older fixtures)
	AccountSettings map[string]string `json:"account_settings,omitempty"`
	// Share is the producer grant to the consumer account, which the revoke_share setup revokes
	// and restores (spec 003; absent without a consumer)
	Share *Share `json:"share,omitempty"`
	// Consumer is the consumer account's resource link to the shared alpha namespace. It is kept
	// out of Styles, so seeding and cleanup never touch it; resource tenant "consumer" resolves here.
	Consumer *Namespace `json:"consumer,omitempty"`
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

// ForDML returns the target with its workgroup switched to the DML workgroup, when there is one.
func (t Target) ForDML() Target {
	if t.DMLWorkgroup != "" {
		t.Workgroup = t.DMLWorkgroup
	}
	return t
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
	// A bare account ID is that account's default catalog, such as the consumer's resource link
	// database (spec 003)
	return "AwsDataCatalog"
}

// Resolve maps a symbolic matrix resource to a target in one style.
func (m *Manifest) Resolve(style matrix.Style, r matrix.Resource) (Target, error) {
	var key string
	var ns Namespace
	var ok bool
	if r.Tenant == matrix.Consumer {
		if m.Consumer == nil || style != matrix.Label {
			return Target{}, errors.New("tenant consumer needs a fixture consumer account and style label")
		}
		return Target{Namespace: *m.Consumer, Key: "consumer", Table: r.Table, Columns: r.Columns, Exists: true}, nil
	}
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
	if r.Via == matrix.Named {
		// Every named namespace shares the baseline workgroups and results location
		via, ok := m.Styles.Named.Namespaces["n_alpha"]
		if !ok {
			return Target{}, fmt.Errorf("no named namespace %q in manifest for via", "n_alpha")
		}
		ns.Workgroup, ns.DMLWorkgroup, ns.ResultsLocation = via.Workgroup, via.DMLWorkgroup, via.ResultsLocation
		key += " via " + string(matrix.Named)
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

// FromManifest is the expect_values sentinel for a value taken from the fixture manifest.
const FromManifest = "manifest"

// settingKeys maps a check case's expect_values key to the manifest's account_settings key, for
// values written as FromManifest.
var settingKeys = map[string]string{
	"cross_account_version": "cross_account_version",
	"mode":                  "catalog_encryption_mode",
	"kms_key_arn":           "catalog_kms_key_arn",
	"federated_identifier":  "s3tables_catalog_identifier",
}

// ExpectValues resolves a check case's expectations, replacing FromManifest with the fixture's
// expected value. Probe matrices generated from the full matrix use it too, because probe runs
// have no manifest.
func (m *Manifest) ExpectValues(values map[string]string) (map[string]string, error) {
	if values == nil {
		return nil, nil
	}
	out := map[string]string{}
	for k, v := range values {
		if v != FromManifest {
			out[k] = v
			continue
		}
		mk, ok := settingKeys[k]
		if !ok {
			return nil, fmt.Errorf("expect_values %s cannot come from the manifest", k)
		}
		mv, ok := m.AccountSettings[mk]
		if !ok || mv == "" {
			return nil, fmt.Errorf("manifest has no account_settings.%s; re-apply the fixture", mk)
		}
		out[k] = mv
	}
	return out, nil
}
