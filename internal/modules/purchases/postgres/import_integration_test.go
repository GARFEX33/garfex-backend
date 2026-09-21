package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	coredomain "github.com/GARFEX33/garfex-costos-unitarios/internal/domain"
	"github.com/GARFEX33/garfex-costos-unitarios/internal/modules/purchases/domain"
	supplierdomain "github.com/GARFEX33/garfex-costos-unitarios/internal/modules/suppliers/domain"
	supplierpostgres "github.com/GARFEX33/garfex-costos-unitarios/internal/modules/suppliers/postgres"
	corepostgres "github.com/GARFEX33/garfex-costos-unitarios/internal/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// TestPurchaseImportIntegration proves against real PostgreSQL that: import
// is atomic and idempotent on the fiscal UUID, a purchase with the same UUID
// but different relevant content is rejected as a conflict rather than
// silently overwritten, a supplier product identified by (supplier, SKU) is
// reused across purchases, and linking/unlinking a supplier product to a
// Resource Master entry cascades to every PENDIENTE/VINCULADO line
// referencing it while leaving a manually set NO_APLICA line untouched.
func TestPurchaseImportIntegration(t *testing.T) {
	runtimeDSN := os.Getenv("GARFEX_TEST_DSN")
	adminDSN := os.Getenv("GARFEX_ADMIN_TEST_DSN")
	if runtimeDSN == "" || adminDSN == "" {
		t.Skip("GARFEX_TEST_DSN and GARFEX_ADMIN_TEST_DSN must target an isolated test database")
	}
	ctx := t.Context()

	runtimePool := openTestPool(t, runtimeDSN)
	adminPool := openTestPool(t, adminDSN)

	unique := fmt.Sprintf("%d", time.Now().UnixNano())
	resourceID := createTestResource(t, ctx, runtimePool, unique)
	supplierID := createTestSupplier(t, ctx, runtimePool, unique)
	t.Cleanup(func() { cleanupPurchaseFixtures(t, adminPool, resourceID, supplierID) })

	repo := NewRepository(runtimePool)

	uuid1 := "AAAAAAAA-1111-2222-3333-" + unique[len(unique)-12:]
	uuid2 := "BBBBBBBB-1111-2222-3333-" + unique[len(unique)-12:]
	sku := "SKU-" + unique

	draft1 := buildTestDraft(supplierID, uuid1, sku, "100.00")

	first, err := repo.Import(ctx, draft1)
	if err != nil {
		t.Fatalf("Import() first = %v", err)
	}
	if first.AlreadyExisted {
		t.Fatal("expected a fresh import")
	}
	if len(first.Lines) != 1 || first.Lines[0].SupplierProductID == nil {
		t.Fatalf("Lines = %+v, want one line with a resolved supplier product", first.Lines)
	}
	if first.Lines[0].DerivedStatus != domain.LinkPending {
		t.Fatalf("effective status = %q, want PENDIENTE for a brand new supplier product", first.Lines[0].DerivedStatus)
	}
	supplierProductID := *first.Lines[0].SupplierProductID

	t.Run("re-import is idempotent", func(t *testing.T) {
		again, err := repo.Import(ctx, draft1)
		if err != nil {
			t.Fatalf("Import() repeat = %v", err)
		}
		if !again.AlreadyExisted {
			t.Fatal("expected AlreadyExisted on re-import of the same document")
		}
		if again.Purchase.ID != first.Purchase.ID {
			t.Fatalf("Purchase.ID = %d, want %d (no duplicate)", again.Purchase.ID, first.Purchase.ID)
		}
	})

	t.Run("same uuid with different content is a conflict", func(t *testing.T) {
		conflicting := buildTestDraft(supplierID, uuid1, sku, "999.00")
		_, err := repo.Import(ctx, conflicting)
		if !errors.Is(err, domain.ErrPurchaseConflict) {
			t.Fatalf("error = %v, want ErrPurchaseConflict", err)
		}
		unchanged, err := repo.GetPurchase(ctx, first.Purchase.ID)
		if err != nil {
			t.Fatalf("GetPurchase() = %v", err)
		}
		if !unchanged.Total.Equal(decimal.RequireFromString("100.00")) {
			t.Fatalf("Total = %s, want the original 100.00 to survive the rejected conflict", unchanged.Total)
		}
	})

	draft2 := buildTestDraft(supplierID, uuid2, sku, "200.00")
	second, err := repo.Import(ctx, draft2)
	if err != nil {
		t.Fatalf("Import() second = %v", err)
	}
	if *second.Lines[0].SupplierProductID != supplierProductID {
		t.Fatal("expected the same supplier product reused across purchases sharing the same SKU")
	}

	confirmed, err := repo.ConfirmMapping(ctx, domain.ConfirmMappingCommand{SupplierProductID: supplierProductID, ResourceID: resourceID, Decision: mappingDecision()})
	if err != nil {
		t.Fatalf("ConfirmMapping() = %v", err)
	}
	t.Run("mapping derives linked status for every referencing pending line", func(t *testing.T) {
		for _, purchaseID := range []int64{first.Purchase.ID, second.Purchase.ID} {
			lines, err := repo.ListPurchaseLines(ctx, purchaseID)
			if err != nil {
				t.Fatalf("ListPurchaseLines(%d) = %v", purchaseID, err)
			}
			if lines[0].DerivedStatus != domain.LinkLinked {
				t.Fatalf("purchase %d line effective status = %q, want VINCULADO", purchaseID, lines[0].DerivedStatus)
			}
		}

		history, err := repo.ListPurchaseLinesByResource(ctx, resourceID, domain.ListCriteria{})
		if err != nil {
			t.Fatalf("ListPurchaseLinesByResource() = %v", err)
		}
		if len(history) != 2 {
			t.Fatalf("history entries = %d, want 2", len(history))
		}
	})

	t.Run("manual override survives a later relink and unlink reverts the rest", func(t *testing.T) {
		lines, err := repo.ListPurchaseLines(ctx, first.Purchase.ID)
		if err != nil {
			t.Fatalf("ListPurchaseLines() = %v", err)
		}
		overridden, err := repo.SetResolutionOverride(ctx, domain.SetResolutionOverrideCommand{
			LineID: lines[0].ID, Override: domain.LinkNotApplicable,
			ExpectedRevision: lines[0].ResolutionRevision, Decision: mappingDecision(),
		})
		if err != nil {
			t.Fatalf("SetResolutionOverride() = %v", err)
		}
		if overridden.DerivedStatus != domain.LinkNotApplicable || overridden.ResolutionOverride != domain.LinkNotApplicable {
			t.Fatalf("effective status = %q, override = %q, want NO_APLICA", overridden.DerivedStatus, overridden.ResolutionOverride)
		}

		if _, err := repo.ExceptionalUnlink(ctx, domain.ExceptionalUnlinkCommand{SupplierProductID: supplierProductID, ExpectedCurrentResource: resourceID, ExpectedRevision: confirmed.MappingRevision, Decision: mappingDecision()}); err != nil {
			t.Fatalf("ExceptionalUnlink() = %v", err)
		}
		unlinkedLines, err := repo.ListPurchaseLines(ctx, first.Purchase.ID)
		if err != nil {
			t.Fatalf("ListPurchaseLines() = %v", err)
		}
		if unlinkedLines[0].DerivedStatus != domain.LinkNotApplicable {
			t.Fatalf("effective status = %q, want the manual NO_APLICA override to survive unlink", unlinkedLines[0].DerivedStatus)
		}

		otherLines, err := repo.ListPurchaseLines(ctx, second.Purchase.ID)
		if err != nil {
			t.Fatalf("ListPurchaseLines() = %v", err)
		}
		if otherLines[0].DerivedStatus != domain.LinkPending {
			t.Fatalf("effective status = %q, want PENDIENTE after unlink", otherLines[0].DerivedStatus)
		}
	})
}

func mappingDecision() domain.MappingDecisionMetadata {
	return domain.MappingDecisionMetadata{Actor: "integration-test", Origin: domain.MappingOriginManual, Reason: "integration test decision", At: time.Now()}
}

func buildTestDraft(supplierID int64, uuid, sku, total string) domain.PurchaseDraft {
	amount := decimal.RequireFromString(total)
	draft, err := domain.NewPurchaseDraft(domain.PurchaseDraft{
		Supplier:    domain.PurchaseParty{SupplierID: supplierID},
		CFDIUUID:    uuid,
		Series:      "AB",
		Folio:       "1",
		IssuedAt:    time.Date(2026, 3, 5, 9, 30, 15, 0, time.UTC),
		Currency:    "MXN",
		Subtotal:    amount,
		Total:       amount,
		IssuerTaxID: "ABC010101AA1",
		IssuerName:  "PROVEEDOR DE PRUEBA",
		XML: domain.XMLDocument{
			Content:  []byte("<xml>" + uuid + total + "</xml>"),
			Hash:     fakeHash(uuid + total),
			Filename: "factura.xml",
		},
		Lines: []domain.PurchaseLineDraft{{
			LineNumber:  1,
			Description: "CABLE THHN 10 AWG",
			SupplierSKU: sku,
			Quantity:    decimal.NewFromInt(10),
			UnitPrice:   amount.Div(decimal.NewFromInt(10)),
			Amount:      amount,
		}},
	})
	if err != nil {
		panic(err)
	}
	return draft
}

func fakeHash(seed string) string {
	const hex = "0123456789abcdef"
	out := make([]byte, 64)
	for i := range out {
		out[i] = hex[(int(seed[i%len(seed)])+i)%16]
	}
	return string(out)
}

func openTestPool(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect test PostgreSQL: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func createTestSupplier(t *testing.T, ctx context.Context, pool *pgxpool.Pool, unique string) int64 {
	t.Helper()
	repo := supplierpostgres.NewRepository(pool)
	supplier, err := supplierdomain.NewSupplier(supplierdomain.SupplierDetails{
		LegalName:     "PROVEEDOR DE PRUEBA " + unique,
		TaxIdentifier: "ABC010101AA1",
	})
	if err != nil {
		t.Fatalf("build supplier: %v", err)
	}
	created, err := repo.CreateSupplier(ctx, supplier)
	if err != nil {
		t.Fatalf("create supplier: %v", err)
	}
	return created.ID
}

var conductoresScope = coredomain.ResourceScope{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"}

func createTestResource(t *testing.T, ctx context.Context, pool *pgxpool.Pool, unique string) int64 {
	t.Helper()
	catalog := coredomain.SeedResourceCatalog()
	resource, err := coredomain.NewResource(catalog, conductoresScope, "M", []coredomain.ResourceAttributeValue{
		coredomain.OptionValue("conductor_material", "COBRE"),
		coredomain.OptionValue("gauge", "10 AWG"),
		coredomain.OptionValue("insulation", "THW"),
		coredomain.OptionValue("color", "NEGRO"),
		coredomain.OptionValue("voltage", "600 V"),
	})
	if err != nil {
		t.Fatalf("build resource: %v", err)
	}
	repo := corepostgres.NewResourceRepository(pool)
	if err := repo.Create(ctx, resource); err != nil {
		t.Fatalf("create resource: %v", err)
	}
	stored, err := repo.Get(ctx, resource.ClassCode, resource.IdentityKey)
	if err != nil {
		t.Fatalf("get created resource: %v", err)
	}
	return stored.ID
}

const mappingAuditCleanupSQL = `DELETE FROM public.supplier_product_mapping_audit
WHERE supplier_product_id IN (
	SELECT id FROM public.supplier_products WHERE supplier_id = $1
)`

const resolutionAuditCleanupSQL = `DELETE FROM public.purchase_line_resolution_audit
WHERE purchase_line_id IN (
	SELECT pl.id FROM public.purchase_lines pl
	JOIN public.purchases p ON p.id = pl.purchase_id
	WHERE p.supplier_id = $1
)`

func cleanupPurchaseFixtures(t *testing.T, adminPool *pgxpool.Pool, resourceID, supplierID int64) {
	t.Helper()
	ctx := context.Background()
	var auditTable *string
	if err := adminPool.QueryRow(ctx, `SELECT to_regclass('public.supplier_product_mapping_audit')`).Scan(&auditTable); err != nil {
		t.Errorf("probe mapping audit table: %v", err)
	} else if auditTable != nil {
		if _, err := adminPool.Exec(ctx, mappingAuditCleanupSQL, supplierID); err != nil {
			t.Errorf("cleanup mapping audit: %v", err)
		}
	}
	var resolutionAuditTable *string
	if err := adminPool.QueryRow(ctx, `SELECT to_regclass('public.purchase_line_resolution_audit')`).Scan(&resolutionAuditTable); err != nil {
		t.Errorf("probe resolution audit table: %v", err)
	} else if resolutionAuditTable != nil {
		if _, err := adminPool.Exec(ctx, resolutionAuditCleanupSQL, supplierID); err != nil {
			t.Errorf("cleanup resolution audit: %v", err)
		}
	}
	if _, err := adminPool.Exec(ctx, `DELETE FROM public.purchases WHERE supplier_id = $1`, supplierID); err != nil {
		t.Errorf("cleanup purchases: %v", err)
	}
	if _, err := adminPool.Exec(ctx, `DELETE FROM public.supplier_products WHERE supplier_id = $1`, supplierID); err != nil {
		t.Errorf("cleanup supplier products: %v", err)
	}
	if _, err := adminPool.Exec(ctx, `DELETE FROM public.suppliers WHERE id = $1`, supplierID); err != nil {
		t.Errorf("cleanup supplier: %v", err)
	}
	if _, err := adminPool.Exec(ctx, `DELETE FROM public.recursos WHERE id = $1`, resourceID); err != nil {
		t.Errorf("cleanup resource: %v", err)
	}
}
