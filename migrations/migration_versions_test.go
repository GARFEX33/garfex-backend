package migrations_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var migrationName = regexp.MustCompile(`^([0-9]+)_.+\.(up|down)\.sql$`)

func TestMigrationVersionDirectionsAreUnique(t *testing.T) {
	if err := validateUniqueMigrationVersionDirections("."); err != nil {
		t.Fatal(err)
	}
}

func TestMigration013SupplierProductMappingContract(t *testing.T) {
	up, err := os.ReadFile("000013_supplier_product_mapping.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile("000013_supplier_product_mapping.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	upSQL, downSQL := string(up), string(down)
	for _, fragment := range []string{
		"mapping_revision", "mapping_identity_conflict", "supplier_product_mapping_audit",
		"resolution_override", "DROP COLUMN link_status", "garfex_admin", "garfex_app",
	} {
		if !strings.Contains(upSQL, fragment) && !strings.Contains(downSQL, fragment) {
			t.Fatalf("migration 000013 missing %q", fragment)
		}
	}
	if strings.Contains(upSQL, "purchase_lines SET link_status") || strings.Contains(upSQL, "UPDATE public.purchase_lines SET link_status") {
		t.Fatal("migration 000013 must not write derived link_status authority")
	}
}

func TestMigration014PurchaseLineResolutionAuditContract(t *testing.T) {
	up, err := os.ReadFile("000014_purchase_line_resolution_audit.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile("000014_purchase_line_resolution_audit.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"resolution_revision", "purchase_line_resolution_audit", "previous_override",
		"new_override", "previous_revision", "new_revision", "origin = 'manual'",
		"garfex_admin", "garfex_app",
	} {
		if !strings.Contains(string(up), fragment) {
			t.Fatalf("migration 000014 up missing %q", fragment)
		}
	}
	for _, fragment := range []string{"DROP TABLE IF EXISTS public.purchase_line_resolution_audit", "DROP COLUMN IF EXISTS resolution_revision"} {
		if !strings.Contains(string(down), fragment) {
			t.Fatalf("migration 000014 down missing %q", fragment)
		}
	}
}

func TestMigration013DownDropsSupplierProductMappingResourceIndex(t *testing.T) {
	down, err := os.ReadFile("000013_supplier_product_mapping.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(down), "DROP INDEX IF EXISTS public.supplier_products_mapping_resource_idx") {
		t.Fatal("down migration must drop supplier_products_mapping_resource_idx")
	}
}

func TestMigration013DownProjectsInactiveMappingsToPending(t *testing.T) {
	down, err := os.ReadFile("000013_supplier_product_mapping.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(down)
	if !strings.Contains(sql, "WHEN sp.resource_id IS NOT NULL AND r.active = FALSE THEN 'PENDIENTE'") {
		t.Fatal("down migration must project inactive mapped resources to PENDIENTE")
	}
	if !strings.Contains(sql, "WHEN pl.resolution_override IN ('NO_APLICA', 'CONFLICTO')") {
		t.Fatal("down migration must preserve explicit override precedence")
	}
}

func TestValidateUniqueMigrationVersionDirections(t *testing.T) {
	tests := []struct {
		name      string
		files     []string
		wantError bool
	}{
		{name: "unique pairs", files: []string{"000001_first.up.sql", "000001_first.down.sql", "000002_second.up.sql"}},
		{name: "duplicate version and direction", files: []string{"000006_supplier.up.sql", "000006_resource.up.sql"}, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
					t.Fatalf("write migration fixture: %v", err)
				}
			}
			err := validateUniqueMigrationVersionDirections(dir)
			if (err != nil) != tt.wantError {
				t.Fatalf("validateUniqueMigrationVersionDirections() error = %v, wantError %t", err, tt.wantError)
			}
		})
	}
}

func validateUniqueMigrationVersionDirections(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	type key struct {
		version   uint64
		direction string
	}
	seen := make(map[key]string)
	for _, entry := range entries {
		match := migrationName.FindStringSubmatch(entry.Name())
		if entry.IsDir() || match == nil {
			continue
		}
		version, err := strconv.ParseUint(match[1], 10, 64)
		if err != nil {
			return fmt.Errorf("parse migration version %q: %w", match[1], err)
		}
		pair := key{version: version, direction: match[2]}
		if previous, exists := seen[pair]; exists {
			return fmt.Errorf("duplicate migration version %d %s: %s and %s", version, pair.direction, previous, entry.Name())
		}
		seen[pair] = entry.Name()
	}
	return nil
}
