package ops

import "testing"

func TestSQL(t *testing.T) {
	cols := []Col{{"customer_id", "bigint"}, {"name", "string"}, {"tier", "string"}}
	cases := map[string]string{
		SelectSQL("alpha", "orders", nil, 10):                                          `SELECT * FROM "alpha"."orders" LIMIT 10`,
		SelectSQL("alpha", "orders", []string{"order_id", `we"ird`}, 0):                `SELECT "order_id", "we""ird" FROM "alpha"."orders"`,
		CountSQL("n_alpha", "orders"):                                                  `SELECT count(*) AS n FROM "n_alpha"."orders"`,
		InsertSQL("alpha", "customers", cols, 900123):                                  `INSERT INTO "alpha"."customers" ("customer_id", "name", "tier") VALUES (900123, 'lfverify', 'lfverify')`,
		DeleteSQL("alpha", "customers", cols, 900123):                                  `DELETE FROM "alpha"."customers" WHERE "customer_id" = 900123`,
		CreateTableSQL("alpha", "late_table", cols[:2], ""):                            "CREATE TABLE `alpha`.`late_table` (`customer_id` bigint, `name` string) TBLPROPERTIES ('table_type' = 'iceberg')",
		CreateTableSQL("alpha", "orders", []Col{{"id", "bigint"}, {"d", "date"}}, "d"): "CREATE TABLE `alpha`.`orders` (`id` bigint, `d` date) PARTITIONED BY (`d`) TBLPROPERTIES ('table_type' = 'iceberg')",
		AlterTableSQL("alpha", "scratch", "note2"):                                     "ALTER TABLE `alpha`.`scratch` ADD COLUMNS (`note2` string)",
		DropTableSQL("alpha", "scratch"):                                               "DROP TABLE `alpha`.`scratch`",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got  %s\nwant %s", got, want)
		}
	}
	u, err := UpdateSQL("alpha", "customers", cols, 7)
	if err != nil || u != `UPDATE "alpha"."customers" SET "name" = 'lfverify-updated' WHERE "customer_id" = 7` {
		t.Errorf("update: %s %v", u, err)
	}
	if _, err := UpdateSQL("t", "x", []Col{{"id", "bigint"}, {"n", "bigint"}}, 1); err == nil {
		t.Error("update without a string column must fail")
	}
	if got := literal("it's"); got != "'it''s'" {
		t.Errorf("literal escaping: %s", got)
	}
}
