package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/modules/purchases/domain"
)

func TestResolvePurchaseLineIntegration(t *testing.T) {
	runtimeDSN := os.Getenv("GARFEX_TEST_DSN")
	adminDSN := os.Getenv("GARFEX_ADMIN_TEST_DSN")
	if runtimeDSN == "" || adminDSN == "" {
		t.Skip("GARFEX_TEST_DSN and GARFEX_ADMIN_TEST_DSN must target an isolated test database")
	}
	ctx := context.Background()
	runtimePool := openTestPool(t, runtimeDSN)
	adminPool := openTestPool(t, adminDSN)
	unique := fmt.Sprintf("%d", time.Now().UnixNano())
	resourceID := createTestResource(t, ctx, runtimePool, unique)
	supplierID := createTestSupplier(t, ctx, runtimePool, unique)
	t.Cleanup(func() { cleanupPurchaseFixtures(t, adminPool, resourceID, supplierID) })
	repo := NewRepository(runtimePool)

	t.Run("existing supplier product", func(t *testing.T) {
		draft := buildTestDraft(supplierID, "DDDDDDDD-1111-2222-3333-"+unique, "SKU-RESOLVE-"+unique, "100.00")
		imported, err := repo.Import(ctx, draft)
		if err != nil {
			t.Fatalf("Import() = %v", err)
		}
		line := imported.Lines[0]
		revision := domain.MappingRevision(0)
		result, err := repo.ResolvePurchaseLine(ctx, domain.ResolvePurchaseLineCommand{
			LineID: line.ID, ResourceID: resourceID,
			ExpectedSupplierProductID: line.SupplierProductID,
			ExpectedMappingRevision:   &revision, ExpectedResolutionRevision: 0,
			Decision: mappingDecision(),
		})
		if err != nil {
			t.Fatalf("ResolvePurchaseLine() = %v", err)
		}
		if result.Line.DerivedStatus != domain.LinkLinked || result.SupplierProduct.MappingRevision != 1 || result.CommercialIdentityDisposition != domain.CommercialIdentityReused {
			t.Fatalf("result = %+v / %+v", result.Line, result.SupplierProduct)
		}
	})

	t.Run("posterior commercial identity preserves XML evidence", func(t *testing.T) {
		draft := buildTestDraft(supplierID, "EEEEEEEE-1111-2222-3333-"+unique, "TEMP-"+unique, "25.00")
		draft.Lines[0].SupplierSKU = ""
		imported, err := repo.Import(ctx, draft)
		if err != nil {
			t.Fatalf("Import() = %v", err)
		}
		line := imported.Lines[0]
		result, err := repo.ResolvePurchaseLine(ctx, domain.ResolvePurchaseLineCommand{
			LineID: line.ID, ResourceID: resourceID,
			CommercialSupplierSKU:      "POSTERIOR-" + unique,
			ExpectedResolutionRevision: 0, Decision: mappingDecision(),
		})
		if err != nil {
			t.Fatalf("ResolvePurchaseLine() = %v", err)
		}
		if result.Line.SupplierSKU != "" {
			t.Fatalf("XML supplier sku mutated to %q", result.Line.SupplierSKU)
		}
		if result.SupplierProduct.SupplierSKU != "POSTERIOR-"+unique || result.Line.SupplierProductID == nil || result.CommercialIdentityDisposition != domain.CommercialIdentityCreated {
			t.Fatalf("posterior identity result = %+v / %+v", result.Line, result.SupplierProduct)
		}

		decision := mappingDecision()
		if _, err := repo.SetResolutionOverride(ctx, domain.SetResolutionOverrideCommand{
			LineID: line.ID, Override: domain.LinkConflict, ExpectedRevision: 0, Decision: decision,
		}); err != nil {
			t.Fatalf("SetResolutionOverride(CONFLICTO) = %v", err)
		}
		if _, err := repo.SetResolutionOverride(ctx, domain.SetResolutionOverrideCommand{
			LineID: line.ID, Override: domain.LinkStatusNone, ExpectedRevision: 1, Decision: decision,
		}); err != nil {
			t.Fatalf("SetResolutionOverride(NONE) = %v", err)
		}
		var auditCount int
		if err := runtimePool.QueryRow(ctx, `SELECT count(*) FROM public.purchase_line_resolution_audit WHERE purchase_line_id = $1`, line.ID).Scan(&auditCount); err != nil {
			t.Fatalf("count resolution audit = %v", err)
		}
		if auditCount != 2 {
			t.Fatalf("resolution audit entries = %d, want 2", auditCount)
		}
		if _, err := repo.ResolvePurchaseLine(ctx, domain.ResolvePurchaseLineCommand{
			LineID: line.ID, ResourceID: resourceID,
			ExpectedSupplierProductID:  result.Line.SupplierProductID,
			ExpectedMappingRevision:    mappingRevisionPointer(result.SupplierProduct.MappingRevision),
			ExpectedResolutionRevision: 0, Decision: mappingDecision(),
		}); !errors.Is(err, domain.ErrStaleResolutionRevision) {
			t.Fatalf("stale resolution error = %v", err)
		}
	})
}

func mappingRevisionPointer(value domain.MappingRevision) *domain.MappingRevision { return &value }
