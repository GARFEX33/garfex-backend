package postgres

import (
	"strings"
	"testing"
	"time"

	"github.com/GARFEX33/garfex-backend/internal/modules/purchases/domain"
)

func TestBuildPurchaseLineWorkbenchQuery_IsParameterizedAndDeterministic(t *testing.T) {
	supplierID := int64(7)
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	query, args := buildPurchaseLineWorkbenchQuery(domain.PurchaseLineWorkbenchCriteria{
		Limit: 11, Offset: 3, SupplierID: &supplierID, EffectiveStatus: domain.LinkSuspended,
		DateFrom: &from, DateTo: &to, InvoiceText: "A'1", SupplierSKU: "SKU", Description: "Cable",
	})
	for _, value := range []string{"A'1", "SKU", "Cable"} {
		if strings.Contains(query, value) {
			t.Fatalf("query contains raw filter value %q: %s", value, query)
		}
	}
	for _, want := range []string{
		"pl.id AS line_id, pl.line_number, pl.purchase_id",
		"JOIN public.suppliers s ON s.id = p.supplier_id",
		"WHEN pl.resolution_override = 'NO_APLICA' THEN 'NO_APLICA'",
		"WHEN COALESCE(sp.mapping_identity_conflict, FALSE) THEN 'CONFLICTO'",
		"WHEN sp.resource_id IS NOT NULL AND NOT COALESCE(r.active, TRUE) THEN 'SUSPENDIDO'",
		"pl.resolution_revision",
		"COALESCE(NULLIF(BTRIM(r.display_name), ''), r.identity_key) AS resource_display_name",
		"ORDER BY issued_at DESC, line_id DESC",
	} {
		if !strings.Contains(query, want) {
			t.Errorf("query missing %q:\n%s", want, query)
		}
	}
	if got, want := len(args), 9; got != want {
		t.Fatalf("args = %d, want %d", got, want)
	}
	if args[len(args)-2] != 11 || args[len(args)-1] != 3 {
		t.Fatalf("pagination args = %#v, want limit/offset at end", args[len(args)-2:])
	}
}

func TestBuildPurchaseLineWorkbenchQuery_CapsInternalOverfetch(t *testing.T) {
	_, args := buildPurchaseLineWorkbenchQuery(domain.PurchaseLineWorkbenchCriteria{Limit: 500})
	if args[len(args)-2] != 51 {
		t.Fatalf("internal page limit = %v, want 51", args[len(args)-2])
	}
}

func TestBuildPurchaseLineWorkbenchQuery_DefaultsPaginationWithoutFilters(t *testing.T) {
	query, args := buildPurchaseLineWorkbenchQuery(domain.PurchaseLineWorkbenchCriteria{})
	if !strings.Contains(query, "WHERE TRUE") {
		t.Fatalf("unfiltered query = %s, want WHERE TRUE", query)
	}
	if len(args) != 2 || args[0] != 50 || args[1] != 0 {
		t.Fatalf("args = %#v, want default limit and offset", args)
	}
}
