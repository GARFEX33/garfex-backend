package postgres

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/GARFEX33/garfex-backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

// Requires a separately authorized disposable database seeded through 000010.
// Never enable against a shared database: fixture writes are committed so the
// reader's independent transaction can observe them.
func TestAttributeOrderReadIntegration(t *testing.T) {
	dsn := os.Getenv("GARFEX_ORDER_READ_TEST_DSN")
	if dsn == "" {
		t.Skip("requires authorized GARFEX_ORDER_READ_TEST_DSN")
	}
	pool := openUnitTestPool(t, dsn)
	ctx := t.Context()
	var database string
	if err := pool.QueryRow(ctx, `SELECT current_database()`).Scan(&database); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(database, "garfex_c3a_disposable_") {
		t.Fatal("refusing non-disposable reader test database")
	}
	repo := NewAttributeOrderRepository(pool)
	scope := domain.ResourceScope{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"}
	read := func() domain.AttributeOrderReadResult {
		t.Helper()
		result, err := repo.ReadAttributeOrder(ctx, scope)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	before := read()
	effective, err := before.Catalog.EffectiveAttributesFor(scope, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i, a := range effective {
		if before.Order.OrderedAttributes[i].CharacteristicCode != a.Attribute.Definition.Code {
			t.Fatal("baseline differs")
		}
	}
	var typeID, familyID, definitionID, lastID int64
	err = pool.QueryRow(ctx, `SELECT t.id,t.family_id,d.id,a.id FROM public.resource_types t
  JOIN public.resource_families f ON f.id=t.family_id JOIN public.resource_classes c ON c.id=f.class_id
  JOIN public.attribute_definitions d ON d.code='insulation'
  JOIN public.resource_attributes a ON a.type_id=t.id
  JOIN public.attribute_definitions last ON last.id=a.definition_id AND last.code='voltage'
  WHERE c.code='MATERIAL' AND f.code='CONDUCTORES' AND t.code='CABLE'`).Scan(&typeID, &familyID, &definitionID, &lastID)
	if err != nil {
		t.Fatal(err)
	}
	var clean bool
	err = pool.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM public.resource_type_attribute_orders WHERE type_id=$1)
  AND NOT EXISTS(SELECT 1 FROM public.resource_attributes WHERE family_id=$2 AND type_id IS NULL AND definition_id=$3)`, typeID, familyID, definitionID).Scan(&clean)
	if err != nil || !clean {
		t.Fatalf("requires clean seed: %v", err)
	}
	// Cleanup targets only the fixture identities checked absent above.
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM public.resource_attributes WHERE family_id=$1 AND type_id IS NULL AND definition_id=$2`, familyID, definitionID); err != nil {
			t.Error(err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM public.resource_type_attribute_orders WHERE type_id=$1`, typeID); err != nil {
			t.Error(err)
		}
	})
	if _, err := pool.Exec(ctx, `INSERT INTO public.resource_type_attribute_orders(type_id) VALUES($1)`, typeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO public.resource_type_attribute_order_items(target_type_id,resource_attribute_id,position)
  SELECT $1,id,(row_number() OVER (ORDER BY (id=$2) DESC,id)-1)::integer
  FROM public.resource_attributes WHERE family_id=$3 AND (type_id IS NULL OR type_id=$1)`, typeID, lastID, familyID); err != nil {
		t.Fatal(err)
	}
	saved := read()
	if saved.Order.OrderedAttributes[0].CharacteristicCode != "voltage" {
		t.Fatal("saved overlay missing")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	frozen, err := readAttributeOrderTx(ctx, tx, scope)
	if err != nil {
		t.Fatal(err)
	}
	insert := func() int64 {
		t.Helper()
		var id int64
		err := pool.QueryRow(ctx, `INSERT INTO public.resource_attributes(class_id,family_id,definition_id,mode,display_order)
   SELECT f.class_id,f.id,$2,'OPTIONAL',COALESCE((SELECT max(display_order)+1 FROM public.resource_attributes WHERE family_id=f.id AND type_id IS NULL),0)
   FROM public.resource_families f WHERE f.id=$1 RETURNING id`, familyID, definitionID).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	newID := insert()
	stillFrozen, err := readAttributeOrderTx(ctx, tx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(frozen, stillFrozen) {
		t.Fatal("repeatable snapshot torn")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	added := read()
	last := added.Order.OrderedAttributes[len(added.Order.OrderedAttributes)-1]
	if last.SourceLevel != domain.SourceLevelFamily || last.CharacteristicCode != "insulation" {
		t.Fatal("new inherited occurrence did not append")
	}
	if len(added.Order.OrderedAttributes) != len(saved.Order.OrderedAttributes)+1 {
		t.Fatal("same-characteristic occurrence lost")
	}
	// Persist the new incarnation first, then delete it: FK cleanup must not
	// transfer its saved position to a newly created binding with the same key.
	if _, err := pool.Exec(ctx, `DELETE FROM public.resource_type_attribute_order_items WHERE target_type_id=$1`, typeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO public.resource_type_attribute_order_items(target_type_id,resource_attribute_id,position)
  SELECT $1,id,(row_number() OVER (ORDER BY (id=$2) DESC,id)-1)::integer
  FROM public.resource_attributes WHERE family_id=$3 AND (type_id IS NULL OR type_id=$1)`, typeID, newID, familyID); err != nil {
		t.Fatal(err)
	}
	persisted := read()
	if persisted.Order.OrderedAttributes[0] != last || len(persisted.Order.OrderedAttributes) != len(added.Order.OrderedAttributes) {
		t.Fatal("fixture must save the old incarnation FIRST in a full order")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM public.resource_attributes WHERE id=$1`, newID); err != nil {
		t.Fatal(err)
	}
	removed := read()
	if len(removed.Order.OrderedAttributes) != len(saved.Order.OrderedAttributes) {
		t.Fatal("deleted occurrence retained")
	}
	recreatedID := insert()
	recreated := read()
	if recreatedID == newID || recreated.Order.OrderRevision == persisted.Order.OrderRevision {
		t.Fatal("recreated incarnation not detected")
	}
	if recreated.Order.OrderedAttributes[len(recreated.Order.OrderedAttributes)-1] != last {
		t.Fatal("recreated binding did not append")
	}
	// A second real family reuses CABLE; a type-code-only lookup would return
	// the populated original instead of this family's empty, distinct snapshot.
	var otherFamilyID int64
	err = pool.QueryRow(ctx, `INSERT INTO public.resource_families(class_id,code,name)
  SELECT class_id,'ORDER_READ_OTHER','Order reader other' FROM public.resource_families WHERE id=$1 RETURNING id`, familyID).Scan(&otherFamilyID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM public.resource_types WHERE family_id=$1`, otherFamilyID); err != nil {
			t.Error(err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM public.resource_families WHERE id=$1`, otherFamilyID); err != nil {
			t.Error(err)
		}
	})
	if _, err := pool.Exec(ctx, `INSERT INTO public.resource_types(class_id,family_id,code,name)
  SELECT class_id,id,'CABLE','Other cable' FROM public.resource_families WHERE id=$1`, otherFamilyID); err != nil {
		t.Fatal(err)
	}
	otherScope := domain.ResourceScope{ClassCode: scope.ClassCode, FamilyCode: "ORDER_READ_OTHER", TypeCode: scope.TypeCode}
	other, err := repo.ReadAttributeOrder(ctx, otherScope)
	if err != nil {
		t.Fatal(err)
	}
	if other.Order.Scope != otherScope || other.Order.OrderedAttributes == nil || len(other.Order.OrderedAttributes) != 0 {
		t.Fatal("type code escaped its requested family")
	}
	if other.Order.OrderRevision == recreated.Order.OrderRevision || read().Order.OrderRevision != recreated.Order.OrderRevision {
		t.Fatal("cross-family scope changed original order")
	}
	for _, missing := range []domain.ResourceScope{
		{ClassCode: "MISSING", FamilyCode: scope.FamilyCode, TypeCode: scope.TypeCode},
		{ClassCode: scope.ClassCode, FamilyCode: "MISSING", TypeCode: scope.TypeCode},
	} {
		if _, err := repo.ReadAttributeOrder(ctx, missing); !errors.Is(err, domain.ErrCatalogRecordNotFound) {
			t.Fatalf("missing scope: %v", err)
		}
	}
}
