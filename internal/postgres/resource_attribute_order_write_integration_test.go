package postgres

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GARFEX33/garfex-backend/internal/domain"
)

// Requires a separately authorized disposable database seeded through 000010,
// mirroring resource_attribute_order_read_integration_test.go's protocol.
// Never enable against a shared database: this test writes and mutates
// resource_type_attribute_orders/_items rows and briefly holds real
// table-level locks.
func TestAttributeOrderWriteIntegration(t *testing.T) {
	dsn := os.Getenv("GARFEX_ORDER_WRITE_TEST_DSN")
	if dsn == "" {
		t.Skip("requires authorized GARFEX_ORDER_WRITE_TEST_DSN")
	}
	pool := openUnitTestPool(t, dsn)
	ctx := t.Context()
	var database string
	if err := pool.QueryRow(ctx, `SELECT current_database()`).Scan(&database); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(database, "garfex_c3b_disposable_") {
		t.Fatal("refusing non-disposable writer test database")
	}
	repo := NewAttributeOrderRepositoryFull(pool)
	scope := domain.ResourceScope{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"}

	var typeID, familyID, classID int64
	err := pool.QueryRow(ctx, `SELECT t.id,t.family_id,t.class_id FROM public.resource_types t
  JOIN public.resource_families f ON f.id=t.family_id JOIN public.resource_classes c ON c.id=f.class_id
  WHERE c.code='MATERIAL' AND f.code='CONDUCTORES' AND t.code='CABLE'`).Scan(&typeID, &familyID, &classID)
	if err != nil {
		t.Fatal(err)
	}
	var clean bool
	if err := pool.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM public.resource_type_attribute_orders WHERE type_id=$1)`, typeID).Scan(&clean); err != nil || !clean {
		t.Fatalf("requires clean seed: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM public.resource_type_attribute_orders WHERE type_id=$1`, typeID); err != nil {
			t.Error(err)
		}
	})

	before, err := repo.ReadAttributeOrder(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}

	// Missing scope must never touch the locked write path.
	missing := domain.ResourceScope{ClassCode: "MISSING", FamilyCode: scope.FamilyCode, TypeCode: scope.TypeCode}
	if _, err := repo.WriteAttributeOrder(ctx, domain.AttributeOrderWriteRequest{Scope: missing, ExpectedOrderRevision: "v1:x", OrderedAttributes: before.Order.OrderedAttributes}); !errors.Is(err, domain.ErrCatalogRecordNotFound) {
		t.Fatalf("missing scope: %v", err)
	}

	// A stale ExpectedOrderRevision must be rejected and leave state untouched.
	if _, err := repo.WriteAttributeOrder(ctx, domain.AttributeOrderWriteRequest{Scope: scope, ExpectedOrderRevision: "v1:stale", OrderedAttributes: before.Order.OrderedAttributes}); !errors.Is(err, domain.ErrAttributeOrderRevisionConflict) {
		t.Fatalf("stale revision: %v", err)
	}
	stillBefore, err := repo.ReadAttributeOrder(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if stillBefore.Order.OrderRevision != before.Order.OrderRevision {
		t.Fatal("rejected write mutated state")
	}

	// Full reverse: first-ever write, head revision must land at 1.
	reversed := slices.Clone(before.Order.OrderedAttributes)
	slices.Reverse(reversed)
	written, err := repo.WriteAttributeOrder(ctx, domain.AttributeOrderWriteRequest{Scope: scope, ExpectedOrderRevision: before.Order.OrderRevision, OrderedAttributes: reversed})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(written.Order.OrderedAttributes, reversed) {
		t.Fatalf("reversed order not applied: %+v", written.Order.OrderedAttributes)
	}
	if written.Order.OrderRevision == before.Order.OrderRevision {
		t.Fatal("head revision did not change")
	}
	var persistedRevision int64
	if err := pool.QueryRow(ctx, `SELECT revision FROM public.resource_type_attribute_orders WHERE type_id=$1`, typeID).Scan(&persistedRevision); err != nil {
		t.Fatal(err)
	}
	if persistedRevision != 1 {
		t.Fatalf("first write revision = %d, want 1", persistedRevision)
	}
	var persistedCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM public.resource_type_attribute_order_items WHERE target_type_id=$1`, typeID).Scan(&persistedCount); err != nil {
		t.Fatal(err)
	}
	if persistedCount != len(reversed) {
		t.Fatalf("persisted item count = %d, want %d", persistedCount, len(reversed))
	}

	// No-op reorder (same accepted order) must still bump the head revision.
	noop, err := repo.WriteAttributeOrder(ctx, domain.AttributeOrderWriteRequest{Scope: scope, ExpectedOrderRevision: written.Order.OrderRevision, OrderedAttributes: reversed})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(noop.Order.OrderedAttributes, reversed) {
		t.Fatal("no-op order changed")
	}
	if noop.Order.OrderRevision == written.Order.OrderRevision {
		t.Fatal("no-op write did not bump head revision")
	}
	if err := pool.QueryRow(ctx, `SELECT revision FROM public.resource_type_attribute_orders WHERE type_id=$1`, typeID).Scan(&persistedRevision); err != nil {
		t.Fatal(err)
	}
	if persistedRevision != 2 {
		t.Fatalf("no-op write revision = %d, want 2", persistedRevision)
	}

	// The now-stale pre-write revision must also be rejected post-write.
	if _, err := repo.WriteAttributeOrder(ctx, domain.AttributeOrderWriteRequest{Scope: scope, ExpectedOrderRevision: written.Order.OrderRevision, OrderedAttributes: before.Order.OrderedAttributes}); !errors.Is(err, domain.ErrAttributeOrderRevisionConflict) {
		t.Fatalf("stale replay after write: %v", err)
	}

	// Concurrent structural lock proof: hold the writer's own locks open in a
	// manually managed transaction (reusing the exact production lock SQL),
	// and confirm a concurrent structural insert against the SHARE-locked
	// resource_attributes table blocks until that transaction ends, then
	// proceeds once released.
	lockTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var lockedTypeID int64
	if err := lockTx.QueryRow(ctx, attributeOrderLockTypeSQL, scope.ClassCode, scope.FamilyCode, scope.TypeCode).Scan(&lockedTypeID); err != nil {
		t.Fatal(err)
	}
	if _, err := lockTx.Exec(ctx, attributeOrderLockStructuralSQL); err != nil {
		t.Fatal(err)
	}

	type insertResult struct {
		id  int64
		err error
	}
	insertDone := make(chan insertResult, 1)
	go func() {
		var newID int64
		err := pool.QueryRow(ctx, `INSERT INTO public.resource_attributes(class_id,family_id,definition_id,mode,display_order)
   SELECT $1,$2,d.id,'OPTIONAL',COALESCE((SELECT max(display_order)+1 FROM public.resource_attributes WHERE family_id=$2 AND type_id IS NULL),0)
   FROM public.attribute_definitions d WHERE d.code='insulation' RETURNING id`, classID, familyID).Scan(&newID)
		insertDone <- insertResult{id: newID, err: err}
	}()
	select {
	case res := <-insertDone:
		t.Fatalf("concurrent structural insert completed while locks were held: id=%d err=%v", res.id, res.err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := lockTx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var res insertResult
	select {
	case res = <-insertDone:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent insert did not proceed after locks released")
	}
	if res.err != nil {
		t.Fatal(res.err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM public.resource_attributes WHERE id=$1`, res.id); err != nil {
			t.Error(err)
		}
	})

	// The concurrently inserted family-level occurrence must be visible (and
	// appended) on the next coherent read, mirroring C3a's own "new inherited
	// occurrence appended after commit" evidence.
	after, err := repo.ReadAttributeOrder(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Order.OrderedAttributes) != len(reversed)+1 {
		t.Fatalf("concurrent occurrence not observed: %+v", after.Order.OrderedAttributes)
	}
}
