package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/GARFEX33/garfex-backend/internal/modules/purchases/domain"
)

func TestSupplierProductMappingIntegration(t *testing.T) {
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
	draft := buildTestDraft(supplierID, "CCCCCCCC-1111-2222-3333-"+unique, "SKU-MAPPING-"+unique, "100.00")
	imported, err := repo.Import(ctx, draft)
	if err != nil {
		t.Fatalf("Import() = %v", err)
	}
	supplierProductID := *imported.Lines[0].SupplierProductID

	var auditTable string
	if err := runtimePool.QueryRow(ctx, `SELECT to_regclass('public.supplier_product_mapping_audit')`).Scan(&auditTable); err != nil {
		t.Fatalf("migration audit table probe = %v", err)
	}
	if auditTable != "supplier_product_mapping_audit" {
		t.Fatalf("audit table = %q, want supplier_product_mapping_audit", auditTable)
	}

	confirmed, err := repo.ConfirmMapping(ctx, domain.ConfirmMappingCommand{
		SupplierProductID: supplierProductID,
		ResourceID:        resourceID,
		Decision:          mappingDecision(),
	})
	if err != nil {
		t.Fatalf("ConfirmMapping() = %v", err)
	}
	if confirmed.MappingRevision != 1 || confirmed.CurrentMapping.ResourceID == nil || *confirmed.CurrentMapping.ResourceID != resourceID {
		t.Fatalf("confirmed mapping = %+v revision %d", confirmed.CurrentMapping, confirmed.MappingRevision)
	}

	if _, err := repo.ConfirmMapping(ctx, domain.ConfirmMappingCommand{
		SupplierProductID: supplierProductID,
		ResourceID:        resourceID,
		ExpectedRevision:  0,
		Decision:          mappingDecision(),
	}); !errors.Is(err, domain.ErrStaleMappingRevision) {
		t.Fatalf("stale ConfirmMapping() = %v, want ErrStaleMappingRevision", err)
	}
	unchanged, err := repo.GetSupplierProduct(ctx, supplierProductID)
	if err != nil {
		t.Fatalf("GetSupplierProduct() after stale transition = %v", err)
	}
	if unchanged.MappingRevision != 1 || unchanged.CurrentMapping.ResourceID == nil || *unchanged.CurrentMapping.ResourceID != resourceID {
		t.Fatalf("mapping after stale transition = %+v revision %d", unchanged.CurrentMapping, unchanged.MappingRevision)
	}

	entries, err := repo.ListMappingAudit(ctx, supplierProductID, domain.ListCriteria{})
	if err != nil {
		t.Fatalf("ListMappingAudit() = %v", err)
	}
	if len(entries) != 1 || entries[0].Operation != domain.MappingOperationConfirm {
		t.Fatalf("mapping audit = %+v, want one confirm entry", entries)
	}

	if _, err := adminPool.Exec(ctx, `UPDATE public.recursos SET active = FALSE WHERE id = $1`, resourceID); err != nil {
		t.Fatalf("deactivate resource: %v", err)
	}
	reconfirmed, err := repo.ConfirmMapping(ctx, domain.ConfirmMappingCommand{
		SupplierProductID: supplierProductID,
		ResourceID:        resourceID,
		ExpectedRevision:  1,
		Decision:          mappingDecision(),
	})
	if err != nil {
		t.Fatalf("inactive idempotent ConfirmMapping() = %v, want success", err)
	}
	if reconfirmed.MappingRevision != 1 || reconfirmed.CurrentMapping.ResourceID == nil || *reconfirmed.CurrentMapping.ResourceID != resourceID {
		t.Fatalf("inactive idempotent mapping = %+v revision %d", reconfirmed.CurrentMapping, reconfirmed.MappingRevision)
	}
	if reconfirmed.ResourceActive == nil || *reconfirmed.ResourceActive {
		t.Fatal("inactive idempotent confirmation must preserve ResourceActive=false")
	}
	rolledBack, err := repo.GetSupplierProduct(ctx, supplierProductID)
	if err != nil {
		t.Fatalf("GetSupplierProduct() after inactive rollback = %v", err)
	}
	if rolledBack.MappingRevision != 1 || rolledBack.CurrentMapping.ResourceID == nil || *rolledBack.CurrentMapping.ResourceID != resourceID {
		t.Fatalf("mapping after inactive idempotent confirmation = %+v revision %d", rolledBack.CurrentMapping, rolledBack.MappingRevision)
	}
	entries, err = repo.ListMappingAudit(ctx, supplierProductID, domain.ListCriteria{})
	if err != nil {
		t.Fatalf("ListMappingAudit() after inactive idempotent confirmation = %v", err)
	}
	if len(entries) != 1 || entries[0].Operation != domain.MappingOperationConfirm {
		t.Fatalf("mapping audit after inactive idempotent confirmation = %+v, want unchanged one-entry audit", entries)
	}

	if _, err := adminPool.Exec(ctx, `UPDATE public.recursos SET active = TRUE WHERE id = $1`, resourceID); err != nil {
		t.Fatalf("reactivate resource: %v", err)
	}
	unlinked, err := repo.ExceptionalUnlink(ctx, domain.ExceptionalUnlinkCommand{
		SupplierProductID:       supplierProductID,
		ExpectedCurrentResource: resourceID,
		ExpectedRevision:        1,
		Decision:                mappingDecision(),
	})
	if err != nil {
		t.Fatalf("ExceptionalUnlink() = %v", err)
	}
	if unlinked.MappingRevision != 2 || unlinked.CurrentMapping.ResourceID != nil {
		t.Fatalf("unlinked mapping = %+v revision %d", unlinked.CurrentMapping, unlinked.MappingRevision)
	}
	entries, err = repo.ListMappingAudit(ctx, supplierProductID, domain.ListCriteria{})
	if err != nil {
		t.Fatalf("ListMappingAudit() after unlink = %v", err)
	}
	if len(entries) != 2 || entries[1].Operation != domain.MappingOperationExceptionalUnlink {
		t.Fatalf("mapping audit after unlink = %+v, want confirm and exceptional unlink", entries)
	}
}
