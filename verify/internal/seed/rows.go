package seed

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
)

// Regions of the 200 fixture orders: 80 EU, 70 US, and 50 APAC, so row filters select known
// subsets (EU 80, EU or US 150).
var regionCounts = []struct {
	Region string
	N      int
}{{"EU", 80}, {"US", 70}, {"APAC", 50}}

// Rows returns the deterministic synthetic rows for a fixture table. Values are obviously fake
// (FR-005); ssn values use the never-issued 900 area.
func Rows(table string) ([]map[string]any, error) {
	var out []map[string]any
	switch table {
	case "orders":
		id := 0
		for _, rc := range regionCounts {
			for i := 0; i < rc.N; i++ {
				id++
				out = append(out, map[string]any{
					"order_id":   id,
					"region":     rc.Region,
					"amount":     fmt.Sprintf("%d.%02d", 10+id, id%100),
					"ssn":        fmt.Sprintf("900-00-%04d", id),
					"attrs":      []map[string]string{{"key": "source", "value": "lfverify"}},
					"order_date": fmt.Sprintf("2026-01-%02d", 1+id%28),
				})
			}
		}
	case "customers":
		tiers := []string{"gold", "silver", "bronze"}
		for id := 1; id <= 50; id++ {
			out = append(out, map[string]any{"customer_id": id, "name": fmt.Sprintf("customer-%02d", id), "tier": tiers[id%3]})
		}
	case "scratch":
		for id := 1; id <= 5; id++ {
			out = append(out, map[string]any{"id": id, "note": fmt.Sprintf("note-%d", id)})
		}
	default:
		return nil, fmt.Errorf("no fixture rows for table %q", table)
	}
	return out, nil
}

// RegionCount returns how many orders carry one of the regions.
func RegionCount(regions ...string) int {
	n := 0
	for _, rc := range regionCounts {
		for _, r := range regions {
			if rc.Region == r {
				n += rc.N
			}
		}
	}
	return n
}

// Record converts rows to an Arrow record in the given schema, through the Arrow JSON reader so
// decimal, date, and map columns use their standard JSON forms.
func Record(schema *arrow.Schema, rows []map[string]any) (arrow.RecordBatch, error) {
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, r := range rows {
		if i > 0 {
			buf.WriteByte(',')
		}
		b, err := json.Marshal(r)
		if err != nil {
			return nil, err
		}
		buf.Write(b)
	}
	buf.WriteByte(']')
	rec, _, err := array.RecordFromJSON(memory.DefaultAllocator, schema, &buf)
	if err != nil {
		return nil, fmt.Errorf("rows to arrow: %w", err)
	}
	return rec, nil
}
