package postgres

import (
	"os"
	"strings"
	"testing"
)

func TestMappingMigrationContract(t *testing.T) {
	up, err := os.ReadFile("../../../../migrations/000013_supplier_product_mapping.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile("../../../../migrations/000013_supplier_product_mapping.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"mapping_revision", "mapping_identity_conflict", "supplier_product_mapping_audit",
		"resolution_override", "DROP COLUMN link_status", "garfex_admin", "garfex_app",
	} {
		if !strings.Contains(string(up), fragment) && !strings.Contains(string(down), fragment) {
			t.Fatalf("migration contract missing %q", fragment)
		}
	}
}
