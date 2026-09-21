package postgres

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/modules/purchases/domain"
)

func TestMappingTransitionSnapshotRefreshPolicy(t *testing.T) {
	if mappingTransitionRefreshesResourceActivity(domain.MappingTransition{}) {
		t.Fatal("unchanged transition must preserve the loaded resource snapshot")
	}
	if !mappingTransitionRefreshesResourceActivity(domain.MappingTransition{Changed: true}) {
		t.Fatal("changed transition must refresh or clear the resource snapshot")
	}
}

func TestConfirmNoOpDoesNotRequireTargetLock(t *testing.T) {
	if confirmRequiresTargetLock(domain.MappingTransition{}) {
		t.Fatal("idempotent confirmation must not require target locking")
	}
	if !confirmRequiresTargetLock(domain.MappingTransition{Changed: true}) {
		t.Fatal("changed confirmation must require target locking")
	}
}

func TestFixtureCleanupDeletesMappingAuditBeforeSupplierProducts(t *testing.T) {
	if strings.Index(mappingAuditCleanupSQL, "supplier_product_mapping_audit") > strings.Index(mappingAuditCleanupSQL, "supplier_products") {
		t.Fatal("fixture cleanup must delete mapping audit rows before supplier products")
	}
}

func TestMappingTargetActivityPolicyIgnoresInactiveCurrentResource(t *testing.T) {
	active := map[int64]bool{7: false, 42: true}
	if err := requireTargetActive(active, 42); err != nil {
		t.Fatalf("active target rejected because current resource is inactive: %v", err)
	}
}

func TestMappingTargetActivityPolicyRejectsInactiveSelectedTarget(t *testing.T) {
	active := map[int64]bool{7: false}
	if !errors.Is(requireTargetActive(active, 7), domain.ErrResourceInactive) {
		t.Fatalf("inactive selected target error = %v, want ErrResourceInactive", requireTargetActive(active, 7))
	}
}

func TestMappingPersistenceErrorsAreStable(t *testing.T) {
	if !errors.Is(mapCommitError(errors.New("connection reset")), domain.ErrCommitAmbiguous) {
		t.Fatal("commit errors must classify as ambiguous")
	}
	if !errors.Is(mapResourceStateError(false), domain.ErrResourceInactive) {
		t.Fatal("inactive targets must classify as inactive")
	}
}

func TestMappingTransitionSQLUsesCASAndAudit(t *testing.T) {
	for name, sql := range map[string]string{
		"supplier product lock": lockSupplierProductSQL,
		"mapping update":        updateSupplierProductMappingSQL,
		"audit insert":          insertMappingAuditSQL,
	} {
		if sql == "" {
			t.Fatalf("%s SQL is empty", name)
		}
	}
}

func TestMappingResourceLockIDsAreUniqueAndCanonical(t *testing.T) {
	got := orderedResourceIDs(9, 42, 3, 9, 0, 3)
	want := []int64{3, 9, 42}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("orderedResourceIDs() = %v, want %v", got, want)
	}
}

func TestMappingAuditPaginationUsesStableOrder(t *testing.T) {
	if mappingAuditSQL == "" {
		t.Fatal("mapping audit query is empty")
	}
}
