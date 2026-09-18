package postgres

import (
	"strings"
	"testing"
)

// The lookup must use the exact expression of the unique index
// suppliers_tax_identifier_key so it is case/space-insensitive like the
// constraint, can use the index, and never hides inactive suppliers.
func TestGetSupplierByTaxIdentifierSQLContract(t *testing.T) {
	for _, want := range []string{
		"upper(btrim(tax_identifier)) = upper(btrim($1))",
		"tax_identifier IS NOT NULL",
	} {
		if !strings.Contains(getSupplierByTaxIdentifierSQL, want) {
			t.Errorf("SQL does not contain %q:\n%s", want, getSupplierByTaxIdentifierSQL)
		}
	}
	_, where, _ := strings.Cut(getSupplierByTaxIdentifierSQL, "WHERE")
	if strings.Contains(where, "active") {
		t.Errorf("lookup must include inactive suppliers, WHERE filters on active:\n%s", where)
	}
	if strings.Contains(getSupplierByTaxIdentifierSQL, "ILIKE") {
		t.Errorf("lookup must be exact, not a partial match:\n%s", getSupplierByTaxIdentifierSQL)
	}
}
