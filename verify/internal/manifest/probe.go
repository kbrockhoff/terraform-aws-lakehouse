package manifest

import (
	"fmt"

	"github.com/kbrockhoff/terraform-aws-lakehouse/verify/internal/matrix"
)

// ProbeFromFull derives a probe-mode matrix from the full-mode matrix and a fixture manifest,
// for the full/probe cross-check: the same personas and expectations, limited to what probe mode
// can verify without changing anything.
//
//   - Only static cases with read operations are kept. Writes, scenarios, and boundaries need
//     attempts, and storage reads are blocked by the probe session policy itself.
//   - Resources are resolved to explicit catalogs, databases, and workgroups per style.
//   - Row key values are dropped (FR-030), and row counts are kept only for count, because
//     probe queries return at most query_row_limit rows.
//   - Inspection cases compare a reader's and a writer's effective permissions (FR-029).
func ProbeFromFull(full *matrix.Matrix, m *Manifest) (*matrix.Matrix, error) {
	out := &matrix.Matrix{Mode: matrix.Probe, Defaults: matrix.Defaults{QueryRowLimit: 10, ScanCapBytes: 1 << 30}}
	index := map[string]int{}
	persona := func(key string) (*matrix.Persona, error) {
		if i, ok := index[key]; ok {
			return &out.Personas[i], nil
		}
		arn, ok := m.Personas[key]
		if !ok {
			return nil, fmt.Errorf("persona %s is not in the manifest", key)
		}
		out.Personas = append(out.Personas, matrix.Persona{Key: key, RoleARN: arn})
		index[key] = len(out.Personas) - 1
		return &out.Personas[index[key]], nil
	}
	for _, inst := range full.Expand() {
		c := inst.Case
		if c.Phase != matrix.Static || c.Op.IsWrite() || c.Op == matrix.StorageRead || c.Setup != "" {
			continue
		}
		t, err := m.Resolve(inst.Style, c.Resource)
		if err != nil {
			return nil, err
		}
		pc := c
		pc.ID = inst.ID
		pc.Style = inst.Style
		pc.Resource = matrix.Resource{
			Table: c.Resource.Table, Columns: c.Resource.Columns,
			CatalogID: t.CatalogID, Database: t.Database, Workgroup: t.Workgroup,
		}
		if c.ExpectRows != nil {
			rows := *c.ExpectRows
			rows.Keys, rows.KeyColumn = nil, ""
			if c.Op != matrix.Count {
				rows.Count = nil
			}
			pc.ExpectRows = &rows
			if rows.Count == nil {
				pc.ExpectRows = nil
			}
		}
		p, err := persona(inst.Persona.Key)
		if err != nil {
			return nil, err
		}
		p.Cases = append(p.Cases, pc)
	}

	inspect := []struct {
		persona string
		label   []string
		named   []string
	}{
		{"reader_single", []string{"DESCRIBE", "SELECT"}, []string{"DESCRIBE", "SELECT"}},
		// Writers get DROP in Baseline environment types (the fixture is Ephemeral); the named
		// baseline grants no DROP
		{"writer_multi", []string{"ALTER", "DELETE", "DESCRIBE", "DROP", "INSERT", "SELECT"}, []string{"ALTER", "DELETE", "DESCRIBE", "INSERT", "SELECT"}},
	}
	for _, in := range inspect {
		for _, style := range []matrix.Style{matrix.Label, matrix.Named} {
			t, err := m.Resolve(style, matrix.Resource{Tenant: "alpha", Table: "customers"})
			if err != nil {
				return nil, err
			}
			perms := in.label
			if style == matrix.Named {
				perms = in.named
			}
			p, err := persona(in.persona)
			if err != nil {
				return nil, err
			}
			p.Cases = append(p.Cases, matrix.Case{
				ID: fmt.Sprintf("%s-inspect-customers/%s", in.persona, style), Style: style, Phase: matrix.Static,
				Op: matrix.Insert, Expect: matrix.Inspect, ExpectPermissions: perms,
				Resource: matrix.Resource{Table: t.Table, CatalogID: t.CatalogID, Database: t.Database},
				Rule:     "Effective permissions match the persona's grants (FR-029)",
			})
		}
	}
	return out, out.Validate()
}
