package postgres

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

const codeUppercaseMigration = "000012_resource_code_uppercase"

func codeUppercaseMigrationSQL(t *testing.T, direction string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "migrations", codeUppercaseMigration+"."+direction+".sql"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestResourceCodeUppercaseMigrationContract(t *testing.T) {
	for _, direction := range []string{"up", "down"} {
		t.Run(direction, func(t *testing.T) {
			sql := strings.Join(strings.Fields(codeUppercaseMigrationSQL(t, direction)), " ")
			fragments := []string{"BEGIN;", "COMMIT;"}
			if direction == "up" {
				fragments = append(fragments,
					"ALTER TABLE public.resource_classes DROP CONSTRAINT resource_classes_code_key;",
					"CREATE UNIQUE INDEX resource_classes_code_upper_key ON public.resource_classes (UPPER(code));",
					"ALTER TABLE public.resource_families DROP CONSTRAINT resource_families_class_id_code_key;",
					"CREATE UNIQUE INDEX resource_families_class_id_code_upper_key ON public.resource_families (class_id, UPPER(code));",
					"ALTER TABLE public.resource_types DROP CONSTRAINT resource_types_family_id_code_key;",
					"CREATE UNIQUE INDEX resource_types_family_id_code_upper_key ON public.resource_types (family_id, UPPER(code));",
					"ALTER TABLE public.unit_definitions DROP CONSTRAINT unit_definitions_code_key;",
					"CREATE UNIQUE INDEX unit_definitions_code_upper_key ON public.unit_definitions (UPPER(code));",
					"CREATE UNIQUE INDEX resource_option_sets_code_upper_key ON public.resource_option_sets (UPPER(code));",
				)
			} else {
				fragments = append(fragments,
					"DROP INDEX public.resource_option_sets_code_upper_key;",
					"DROP INDEX public.unit_definitions_code_upper_key;",
					"ALTER TABLE public.unit_definitions ADD CONSTRAINT unit_definitions_code_key UNIQUE (code);",
					"DROP INDEX public.resource_types_family_id_code_upper_key;",
					"ALTER TABLE public.resource_types ADD CONSTRAINT resource_types_family_id_code_key UNIQUE (family_id, code);",
					"DROP INDEX public.resource_families_class_id_code_upper_key;",
					"ALTER TABLE public.resource_families ADD CONSTRAINT resource_families_class_id_code_key UNIQUE (class_id, code);",
					"DROP INDEX public.resource_classes_code_upper_key;",
					"ALTER TABLE public.resource_classes ADD CONSTRAINT resource_classes_code_key UNIQUE (code);",
				)
			}
			for _, fragment := range fragments {
				if !strings.Contains(sql, fragment) {
					t.Errorf("missing %q", fragment)
				}
			}
		})
	}
}

// Explicit opt-in only: a disposable database with migrations 1–10 and normal
// roles must be supplied. The transaction always rolls back.
func TestResourceCodeUppercaseMigrationIntegration(t *testing.T) {
	dsn := os.Getenv("GARFEX_CODE_UPPERCASE_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("requires authorized disposable GARFEX_CODE_UPPERCASE_MIGRATION_TEST_DSN")
	}
	ctx := t.Context()
	pool := openUnitTestPool(t, dsn)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	apply := func(direction string) {
		t.Helper()
		sql := strings.ReplaceAll(codeUppercaseMigrationSQL(t, direction), "BEGIN;", "")
		sql = strings.ReplaceAll(sql, "COMMIT;", "")
		if _, err := tx.Exec(ctx, sql); err != nil {
			t.Fatalf("%s migration: %v", direction, err)
		}
	}
	apply("up")

	requireCaseCollisionRejected(ctx, t, tx, "resource_classes", "INSERT INTO public.resource_classes (code, name, plural, slug) VALUES ('material', 'dup', 'dup', 'dup-slug-x')")

	var familyID, classID int64
	if err := tx.QueryRow(ctx, `SELECT id, class_id FROM public.resource_families ORDER BY id LIMIT 1`).Scan(&familyID, &classID); err != nil {
		t.Fatal(err)
	}
	var existingFamilyCode string
	if err := tx.QueryRow(ctx, `SELECT code FROM public.resource_families WHERE id=$1`, familyID).Scan(&existingFamilyCode); err != nil {
		t.Fatal(err)
	}
	requireCaseCollisionRejected(ctx, t, tx, "resource_families",
		"INSERT INTO public.resource_families (class_id, code, name) VALUES ("+
			strconv.FormatInt(classID, 10)+", '"+strings.ToLower(existingFamilyCode)+"', 'dup')")

	apply("down")
	var restored bool
	err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid=to_regclass('public.resource_classes') AND conname='resource_classes_code_key')`).Scan(&restored)
	if err != nil || !restored {
		t.Fatalf("down did not restore resource_classes_code_key: restored=%v err=%v", restored, err)
	}
	apply("up")
}

func requireCaseCollisionRejected(ctx context.Context, t *testing.T, tx pgx.Tx, label, insertSQL string) {
	t.Helper()
	savepoint, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = savepoint.Exec(ctx, insertSQL)
	rollbackErr := savepoint.Rollback(ctx)
	if rollbackErr != nil {
		t.Fatal(rollbackErr)
	}
	if err == nil {
		t.Fatalf("%s: case-variant duplicate was not rejected", label)
	}
}
