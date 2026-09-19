package postgres

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

const attributeOrderMigration = "000010_resource_type_attribute_order"

func orderMigrationSQL(t *testing.T, direction string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "migrations", attributeOrderMigration+"."+direction+".sql"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestAttributeOrderMigrationContract(t *testing.T) {
	for _, direction := range []string{"up", "down"} {
		t.Run(direction, func(t *testing.T) {
			sql := strings.Join(strings.Fields(orderMigrationSQL(t, direction)), " ")
			fragments := []string{"BEGIN;", "COMMIT;"}
			if direction == "up" {
				fragments = append(fragments,
					"CREATE TABLE public.resource_type_attribute_orders",
					"type_id BIGINT PRIMARY KEY REFERENCES public.resource_types(id) ON DELETE CASCADE",
					"revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0)",
					"updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
					"CREATE TABLE public.resource_type_attribute_order_items",
					"target_type_id BIGINT NOT NULL REFERENCES public.resource_type_attribute_orders(type_id) ON DELETE CASCADE",
					"resource_attribute_id BIGINT NOT NULL REFERENCES public.resource_attributes(id) ON DELETE CASCADE",
					"position INTEGER NOT NULL CHECK (position >= 0)",
					"PRIMARY KEY (target_type_id, resource_attribute_id)", "UNIQUE (target_type_id, position)",
					"CREATE INDEX resource_type_attribute_order_items_binding_idx ON public.resource_type_attribute_order_items (resource_attribute_id);",
					"ALTER TABLE public.resource_type_attribute_orders OWNER TO garfex_admin;",
					"ALTER TABLE public.resource_type_attribute_order_items OWNER TO garfex_admin;",
					"GRANT SELECT, INSERT, UPDATE, DELETE ON public.resource_type_attribute_orders, public.resource_type_attribute_order_items TO garfex_app;",
				)
			} else {
				fragments = append(fragments,
					"REVOKE SELECT, INSERT, UPDATE, DELETE ON public.resource_type_attribute_order_items, public.resource_type_attribute_orders FROM garfex_app;",
					"DROP TABLE public.resource_type_attribute_order_items; DROP TABLE public.resource_type_attribute_orders;",
				)
			}
			for _, fragment := range fragments {
				if !strings.Contains(sql, fragment) {
					t.Errorf("missing %q", fragment)
				}
			}
			for _, forbidden := range []string{"resource_type_presentation_fields", "CREATE SEQUENCE", "BIGSERIAL", "DROP TABLE public.resource_types"} {
				if strings.Contains(sql, forbidden) {
					t.Errorf("unexpected %q", forbidden)
				}
			}
		})
	}
}

// Explicit opt-in only: a disposable database with migrations 1–9 and normal
// roles must be supplied. The transaction always rolls back. Unlike the legacy
// migration window, this test never downs/reapplies unrelated migrations.
func TestAttributeOrderMigrationIntegration(t *testing.T) {
	dsn := os.Getenv("GARFEX_ORDER_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("requires authorized disposable GARFEX_ORDER_MIGRATION_TEST_DSN")
	}
	ctx := t.Context()
	pool := openUnitTestPool(t, dsn)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var clean bool
	err = tx.QueryRow(ctx, `SELECT to_regclass('public.resource_type_attribute_orders') IS NULL
  AND to_regclass('public.resource_type_attribute_order_items') IS NULL`).Scan(&clean)
	if err != nil || !clean {
		t.Fatalf("requires pre-migration disposable database: clean=%v err=%v", clean, err)
	}
	apply := func(direction string) {
		t.Helper()
		// Keep the migration body inside the rollback-only test transaction.
		sql := strings.ReplaceAll(orderMigrationSQL(t, direction), "BEGIN;", "")
		sql = strings.ReplaceAll(sql, "COMMIT;", "")
		if _, err := tx.Exec(ctx, sql); err != nil {
			t.Fatalf("%s migration: %v", direction, err)
		}
	}
	apply("up")
	for _, table := range []string{"resource_type_attribute_orders", "resource_type_attribute_order_items"} {
		var owner string
		err := tx.QueryRow(ctx, `SELECT pg_get_userbyid(relowner) FROM pg_class
   WHERE oid=to_regclass($1)`, "public."+table).Scan(&owner)
		if err != nil || owner != "garfex_admin" {
			t.Fatalf("owner %s = %q: %v", table, owner, err)
		}
		for _, privilege := range []string{"SELECT", "INSERT", "UPDATE", "DELETE"} {
			var granted bool
			err := tx.QueryRow(ctx, `SELECT has_table_privilege('garfex_app',$1,$2)`, "public."+table, privilege).Scan(&granted)
			if err != nil || !granted {
				t.Fatalf("grant %s/%s: %v", table, privilege, err)
			}
		}
	}
	for _, check := range []struct{ table, definition string }{
		{table: "resource_type_attribute_orders", definition: "PRIMARY KEY (type_id)"},
		{table: "resource_type_attribute_orders", definition: "FOREIGN KEY (type_id) REFERENCES resource_types(id) ON DELETE CASCADE"},
		{table: "resource_type_attribute_orders", definition: "CHECK ((revision > 0))"},
		{table: "resource_type_attribute_order_items", definition: "PRIMARY KEY (target_type_id, resource_attribute_id)"},
		{table: "resource_type_attribute_order_items", definition: `UNIQUE (target_type_id, "position")`},
		{table: "resource_type_attribute_order_items", definition: "FOREIGN KEY (target_type_id) REFERENCES resource_type_attribute_orders(type_id) ON DELETE CASCADE"},
		{table: "resource_type_attribute_order_items", definition: "FOREIGN KEY (resource_attribute_id) REFERENCES resource_attributes(id) ON DELETE CASCADE"},
		{table: "resource_type_attribute_order_items", definition: `CHECK (("position" >= 0))`},
	} {
		var found bool
		err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_constraint
   WHERE conrelid=to_regclass($1) AND pg_get_constraintdef(oid)=$2)`, "public."+check.table, check.definition).Scan(&found)
		if err != nil || !found {
			t.Fatalf("constraint %q: found=%v error=%v", check.definition, found, err)
		}
	}
	var defaults bool
	err = tx.QueryRow(ctx, `SELECT count(*)=2 FROM information_schema.columns
  WHERE table_schema='public' AND table_name='resource_type_attribute_orders' AND is_nullable='NO'
  AND ((column_name='revision' AND column_default='1')
   OR (column_name='updated_at' AND column_default='now()'))`).Scan(&defaults)
	if err != nil || !defaults {
		t.Fatalf("defaults: %v, %v", defaults, err)
	}
	assertOrderMigrationCascades(t, tx)
	apply("down")
	var gone bool
	err = tx.QueryRow(ctx, `SELECT to_regclass('public.resource_type_attribute_orders') IS NULL
  AND to_regclass('public.resource_type_attribute_order_items') IS NULL`).Scan(&gone)
	if err != nil || !gone {
		t.Fatalf("down: gone=%v err=%v", gone, err)
	}
	apply("up")
}

func assertOrderMigrationCascades(t *testing.T, tx pgx.Tx) {
	t.Helper()
	ctx := t.Context()
	// A fresh type and two direct bindings are transaction-local fixtures.
	var typeID int64
	err := tx.QueryRow(ctx, `INSERT INTO public.resource_types(class_id,family_id,code,name)
  SELECT class_id,id,'ORDER_TEST_' || txid_current(),'Order test' FROM public.resource_families ORDER BY id LIMIT 1
  RETURNING id`).Scan(&typeID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.resource_attributes(class_id,family_id,type_id,definition_id,mode,display_order)
  SELECT t.class_id,t.family_id,t.id,d.id,'OPTIONAL',(row_number() OVER (ORDER BY d.id)-1)::integer FROM public.resource_types t
  CROSS JOIN (SELECT id FROM public.attribute_definitions ORDER BY id LIMIT 2) d WHERE t.id=$1`, typeID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.resource_type_attribute_orders(type_id) VALUES($1)`, typeID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.resource_type_attribute_order_items(target_type_id,resource_attribute_id,position)
  SELECT $1,id,(row_number() OVER (ORDER BY id)-1)::integer FROM public.resource_attributes WHERE type_id=$1`, typeID)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM public.resource_type_attribute_order_items WHERE target_type_id=$1`, typeID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("requires two seeded definitions: count=%d err=%v", count, err)
	}
	_, err = tx.Exec(ctx, `DELETE FROM public.resource_attributes WHERE id=(SELECT min(resource_attribute_id)
  FROM public.resource_type_attribute_order_items WHERE target_type_id=$1)`, typeID)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM public.resource_type_attribute_order_items WHERE target_type_id=$1`, typeID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("binding cascade: count=%d err=%v", count, err)
	}
	_, err = tx.Exec(ctx, `DELETE FROM public.resource_type_attribute_orders WHERE type_id=$1`, typeID)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM public.resource_type_attribute_order_items WHERE target_type_id=$1`, typeID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("head cascade: count=%d err=%v", count, err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.resource_type_attribute_orders(type_id) VALUES($1)`, typeID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `DELETE FROM public.resource_attributes WHERE type_id=$1`, typeID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `DELETE FROM public.resource_types WHERE id=$1`, typeID)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM public.resource_type_attribute_orders WHERE type_id=$1`, typeID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("type cascade: count=%d err=%v", count, err)
	}
}
