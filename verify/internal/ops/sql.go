package ops

import (
	"fmt"
	"strings"
)

// Fixture DML uses one run-unique key so writer cases touch only the row they insert.

// quote quotes an identifier for query (DML) statements.
func quote(id string) string { return `"` + strings.ReplaceAll(id, `"`, `""`) + `"` }

// bquote quotes an identifier for DDL statements.
func bquote(id string) string { return "`" + strings.ReplaceAll(id, "`", "``") + "`" }

// literal quotes a string literal.
func literal(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// SelectSQL selects the given columns (all when empty) with a row limit (none when 0).
func SelectSQL(db, table string, columns []string, limit int) string {
	cols := "*"
	if len(columns) > 0 {
		q := make([]string, len(columns))
		for i, c := range columns {
			q[i] = quote(c)
		}
		cols = strings.Join(q, ", ")
	}
	s := fmt.Sprintf("SELECT %s FROM %s.%s", cols, quote(db), quote(table))
	if limit > 0 {
		s += fmt.Sprintf(" LIMIT %d", limit)
	}
	return s
}

// CountSQL counts rows.
func CountSQL(db, table string) string {
	return fmt.Sprintf("SELECT count(*) AS n FROM %s.%s", quote(db), quote(table))
}

// Col is a column name and type for DML and DDL generation.
type Col struct{ Name, Type string }

// InsertSQL inserts one row: the key in the first (bigint) column, 'lfverify' in string columns,
// and NULL elsewhere.
func InsertSQL(db, table string, cols []Col, key int64) string {
	names := make([]string, len(cols))
	vals := make([]string, len(cols))
	for i, c := range cols {
		names[i] = quote(c.Name)
		switch {
		case i == 0:
			vals[i] = fmt.Sprintf("%d", key)
		case c.Type == "string":
			vals[i] = literal("lfverify")
		default:
			vals[i] = "NULL"
		}
	}
	return fmt.Sprintf("INSERT INTO %s.%s (%s) VALUES (%s)", quote(db), quote(table), strings.Join(names, ", "), strings.Join(vals, ", "))
}

// UpdateSQL changes the first string column of the keyed row.
func UpdateSQL(db, table string, cols []Col, key int64) (string, error) {
	for _, c := range cols[1:] {
		if c.Type == "string" {
			return fmt.Sprintf("UPDATE %s.%s SET %s = %s WHERE %s = %d",
				quote(db), quote(table), quote(c.Name), literal("lfverify-updated"), quote(cols[0].Name), key), nil
		}
	}
	return "", fmt.Errorf("table %s has no string column to update", table)
}

// DeleteSQL deletes the keyed row.
func DeleteSQL(db, table string, cols []Col, key int64) string {
	return fmt.Sprintf("DELETE FROM %s.%s WHERE %s = %d", quote(db), quote(table), quote(cols[0].Name), key)
}

// CreateTableSQL creates an Iceberg table, partitioned when partition is set.
func CreateTableSQL(db, table string, cols []Col, partition string) string {
	defs := make([]string, len(cols))
	for i, c := range cols {
		defs[i] = bquote(c.Name) + " " + c.Type
	}
	s := fmt.Sprintf("CREATE TABLE %s.%s (%s)", bquote(db), bquote(table), strings.Join(defs, ", "))
	if partition != "" {
		s += fmt.Sprintf(" PARTITIONED BY (%s)", bquote(partition))
	}
	return s + " TBLPROPERTIES ('table_type' = 'iceberg')"
}

// AlterTableSQL adds a nullable column, the metadata change metadata writers attempt.
func AlterTableSQL(db, table, column string) string {
	return fmt.Sprintf("ALTER TABLE %s.%s ADD COLUMNS (%s string)", bquote(db), bquote(table), bquote(column))
}

// DropTableSQL drops a table.
func DropTableSQL(db, table string) string {
	return fmt.Sprintf("DROP TABLE %s.%s", bquote(db), bquote(table))
}

// DropColumnSQL removes a column added by AlterTableSQL.
func DropColumnSQL(db, table, column string) string {
	return fmt.Sprintf("ALTER TABLE %s.%s DROP COLUMN %s", bquote(db), bquote(table), bquote(column))
}
