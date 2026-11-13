package seed

import (
	"reflect"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
)

func TestRowsDeterministicAndCounted(t *testing.T) {
	a, err := Rows("orders")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Rows("orders")
	if !reflect.DeepEqual(a, b) {
		t.Error("rows must be deterministic")
	}
	counts := map[string]int{}
	for _, r := range a {
		counts[r["region"].(string)]++
	}
	if len(a) != 200 || counts["EU"] != 80 || counts["US"] != 70 || counts["APAC"] != 50 {
		t.Errorf("orders: %d rows, regions %v", len(a), counts)
	}
	if RegionCount("EU", "US") != 150 || RegionCount("EU") != 80 {
		t.Error("region counts")
	}
	for table, n := range map[string]int{"customers": 50, "scratch": 5} {
		r, err := Rows(table)
		if err != nil || len(r) != n {
			t.Errorf("%s: %d rows, %v", table, len(r), err)
		}
	}
	if _, err := Rows("nope"); err == nil {
		t.Error("unknown table must fail")
	}
}

func TestRecordConvertsFixtureTypes(t *testing.T) {
	schema := arrow.NewSchema([]arrow.Field{
		{Name: "order_id", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "region", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "amount", Type: &arrow.Decimal128Type{Precision: 12, Scale: 2}, Nullable: true},
		{Name: "ssn", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "attrs", Type: arrow.MapOf(arrow.BinaryTypes.String, arrow.BinaryTypes.String), Nullable: true},
		{Name: "order_date", Type: arrow.FixedWidthTypes.Date32, Nullable: true},
	}, nil)
	rows, _ := Rows("orders")
	rec, err := Record(schema, rows)
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Release()
	if rec.NumRows() != 200 || rec.NumCols() != 6 {
		t.Errorf("record shape %dx%d", rec.NumRows(), rec.NumCols())
	}
}
