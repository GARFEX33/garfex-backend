package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/GARFEX33/garfex-backend/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestResourceRepositoryAttributeCardinalityIntegration(t *testing.T) {
	adminDSN, appDSN := os.Getenv("GARFEX_ADMIN_TEST_DSN"), os.Getenv("GARFEX_TEST_DSN")
	if adminDSN == "" || appDSN == "" {
		t.Skip("GARFEX_ADMIN_TEST_DSN and GARFEX_TEST_DSN are required")
	}
	ctx := context.Background()
	admin := openUnitTestPool(t, adminDSN)
	app := openUnitTestPool(t, appDSN)
	classID, familyID, typeID, quantityDefinitionID := setupCardinalityFixture(t, admin)
	defer teardownCardinalityFixture(t, admin, classID, familyID, typeID, quantityDefinitionID)
	repo := NewResourceRepository(app)
	clearCardinalityResources(t, admin)

	t.Run("family-wide target and quantity unit persist once", func(t *testing.T) {
		clearCardinalityResources(t, admin)
		bindingIDs := []int64{insertCardinalityBinding(t, admin, classID, familyID, nil, "gauge"), insertCardinalityBinding(t, admin, classID, familyID, nil, "cardinality_quantity")}
		defer deleteCardinalityBindings(t, admin, bindingIDs...)
		want := cardinalityResource(t, "family-wide", domain.OptionValue("gauge", "10 AWG"), domain.QuantityValue("cardinality_quantity", "2.5", "M"))
		if err := repo.Create(ctx, want); err != nil {
			t.Fatalf("Create() = %v", err)
		}
		got, err := repo.Get(ctx, want.ClassCode, want.IdentityKey)
		if err != nil || len(got.Attributes) != 2 {
			t.Fatalf("Get() = %#v, error %v; want two attributes", got, err)
		}
		var quantity *domain.Quantity
		for _, attribute := range got.Attributes {
			if attribute.AttributeCode == "cardinality_quantity" {
				quantity = attribute.Quantity
			}
		}
		if quantity == nil || !quantity.Value.Equal(domain.QuantityValue("x", "2.5", "M").Quantity.Value) || quantity.UnitCode != "M" {
			t.Fatalf("quantity = %#v, want 2.5 M", quantity)
		}
	})

	t.Run("reused family code cannot cross class", func(t *testing.T) {
		clearCardinalityResources(t, admin)
		want := cardinalityResource(t, "wrong-class", domain.OptionValue("gauge", "10 AWG"))
		if err := repo.Create(ctx, want); !errors.Is(err, domain.ErrResourceIntegrity) {
			t.Fatalf("Create() error = %v, want ErrResourceIntegrity", err)
		}
		assertNoCardinalityResource(t, admin, want.IdentityKey)
	})

	t.Run("type-specific target resolves exactly", func(t *testing.T) {
		clearCardinalityResources(t, admin)
		binding := insertCardinalityBinding(t, admin, classID, familyID, cardinalityType(typeID), "color")
		defer deleteCardinalityBindings(t, admin, binding)
		want := cardinalityResource(t, "type-specific", domain.OptionValue("color", "NEGRO"))
		if err := repo.Create(ctx, want); err != nil {
			t.Fatalf("Create() = %v", err)
		}
	})

	t.Run("zero target and missing definition are integrity errors", func(t *testing.T) {
		clearCardinalityResources(t, admin)
		for _, value := range []domain.ResourceAttributeValue{domain.OptionValue("gauge", "10 AWG"), domain.OptionValue("missing_definition", "VALUE")} {
			want := cardinalityResource(t, value.AttributeCode, value)
			if err := repo.Create(ctx, want); !errors.Is(err, domain.ErrResourceIntegrity) {
				t.Fatalf("Create(%s) error = %v, want ErrResourceIntegrity", value.AttributeCode, err)
			}
			assertNoCardinalityResource(t, admin, want.IdentityKey)
		}
	})

	t.Run("family-wide and type-specific overlap is ambiguous and rolls back create", func(t *testing.T) {
		clearCardinalityResources(t, admin)
		bindingIDs := []int64{insertCardinalityBinding(t, admin, classID, familyID, nil, "gauge"), insertCardinalityBinding(t, admin, classID, familyID, cardinalityType(typeID), "gauge")}
		defer deleteCardinalityBindings(t, admin, bindingIDs...)
		want := cardinalityResource(t, "ambiguous-create", domain.OptionValue("gauge", "10 AWG"))
		if err := repo.Create(ctx, want); !errors.Is(err, domain.ErrResourceIntegrity) {
			t.Fatalf("Create() error = %v, want ErrResourceIntegrity", err)
		}
		assertNoCardinalityResource(t, admin, want.IdentityKey)
	})

	t.Run("ambiguous update rolls back resource and attributes", func(t *testing.T) {
		clearCardinalityResources(t, admin)
		familyBinding := insertCardinalityBinding(t, admin, classID, familyID, nil, "gauge")
		defer deleteCardinalityBindings(t, admin, familyBinding)
		original := cardinalityResource(t, "update-original", domain.OptionValue("gauge", "10 AWG"))
		if err := repo.Create(ctx, original); err != nil {
			t.Fatalf("Create() = %v", err)
		}
		stored, err := repo.Get(ctx, original.ClassCode, original.IdentityKey)
		if err != nil {
			t.Fatalf("Get() = %v", err)
		}
		ambiguous := insertCardinalityBinding(t, admin, classID, familyID, cardinalityType(typeID), "gauge")
		defer deleteCardinalityBindings(t, admin, ambiguous)
		candidate := cardinalityResource(t, "update-rejected", domain.OptionValue("gauge", "10 AWG"))
		candidate.ID = stored.ID
		if err := repo.Update(ctx, candidate); !errors.Is(err, domain.ErrResourceIntegrity) {
			t.Fatalf("Update() error = %v, want ErrResourceIntegrity", err)
		}
		assertRoundTrip(t, ctx, repo, original)
		assertNoCardinalityResource(t, admin, candidate.IdentityKey)
	})
}

func cardinalityResource(t *testing.T, key string, values ...domain.ResourceAttributeValue) domain.Resource {
	t.Helper()
	resource, err := domain.HydrateResource(domain.ResourceSnapshot{ID: 1, ClassCode: "MANO_DE_OBRA", FamilyCode: "CONDUCTORES", TypeCode: "CABLE", NaturalUnit: "M", Attributes: values, IdentityKey: fmt.Sprintf("v1|cardinality|%s", key), Active: true})
	if err != nil {
		t.Fatalf("hydrate cardinality resource: %v", err)
	}
	return resource
}

func setupCardinalityFixture(t *testing.T, pool *pgxpool.Pool) (int64, int64, int64, int64) {
	t.Helper()
	ctx := context.Background()
	var classID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM public.resource_classes WHERE code='MANO_DE_OBRA'`).Scan(&classID); err != nil {
		t.Fatalf("find cardinality class: %v", err)
	}
	_, _ = pool.Exec(ctx, `DELETE FROM public.recursos WHERE identity_key LIKE 'v1|cardinality|%'`)
	_, _ = pool.Exec(ctx, `DELETE FROM public.resource_attributes WHERE family_id IN (SELECT id FROM public.resource_families WHERE class_id=$1 AND code='CONDUCTORES')`, classID)
	_, _ = pool.Exec(ctx, `DELETE FROM public.resource_unit_policies WHERE family_id IN (SELECT id FROM public.resource_families WHERE class_id=$1 AND code='CONDUCTORES')`, classID)
	_, _ = pool.Exec(ctx, `DELETE FROM public.resource_types WHERE family_id IN (SELECT id FROM public.resource_families WHERE class_id=$1 AND code='CONDUCTORES')`, classID)
	_, _ = pool.Exec(ctx, `DELETE FROM public.resource_families WHERE class_id=$1 AND code='CONDUCTORES'`, classID)
	var familyID, typeID, quantityDefinitionID int64
	if err := pool.QueryRow(ctx, `INSERT INTO public.resource_families (class_id, code, name, description) VALUES ($1,'CONDUCTORES','Test conductors','cardinality') RETURNING id`, classID).Scan(&familyID); err != nil {
		t.Fatalf("insert cardinality family: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO public.resource_types (class_id, family_id, code, name) VALUES ($1,$2,'CABLE','Test cable') RETURNING id`, classID, familyID).Scan(&typeID); err != nil {
		t.Fatalf("insert cardinality type: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO public.resource_unit_policies (family_id, unit_id) SELECT $1,id FROM public.unit_definitions WHERE code='M'`, familyID); err != nil {
		t.Fatalf("insert cardinality unit policy: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO public.attribute_definitions (code,name,value_type,dimension) VALUES ('cardinality_quantity','Cardinality quantity','QUANTITY','LENGTH') RETURNING id`).Scan(&quantityDefinitionID); err != nil {
		t.Fatalf("insert cardinality definition: %v", err)
	}
	return classID, familyID, typeID, quantityDefinitionID
}

func insertCardinalityBinding(t *testing.T, pool *pgxpool.Pool, classID, familyID int64, typeID *int64, definition string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(), `INSERT INTO public.resource_attributes (class_id,family_id,type_id,definition_id,mode,display_order)
		VALUES ($1,$2,$3,(SELECT id FROM public.attribute_definitions WHERE code=$4),'REQUIRED',
		COALESCE((SELECT max(display_order)+1 FROM public.resource_attributes WHERE family_id=$2 AND type_id IS NOT DISTINCT FROM $3),0)) RETURNING id`, classID, familyID, typeID, definition).Scan(&id); err != nil {
		t.Fatalf("insert cardinality binding %s: %v", definition, err)
	}
	return id
}

func cardinalityType(id int64) *int64 { return &id }

func deleteCardinalityBindings(t *testing.T, pool *pgxpool.Pool, ids ...int64) {
	t.Helper()
	clearCardinalityResources(t, pool)
	for _, id := range ids {
		if _, err := pool.Exec(context.Background(), `DELETE FROM public.resource_attributes WHERE id=$1`, id); err != nil {
			t.Fatalf("delete cardinality binding %d: %v", id, err)
		}
	}
}

func clearCardinalityResources(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `DELETE FROM public.recursos WHERE identity_key LIKE 'v1|cardinality|%'`); err != nil {
		t.Fatalf("clear cardinality resources: %v", err)
	}
}

func assertNoCardinalityResource(t *testing.T, pool *pgxpool.Pool, identity string) {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(), `SELECT id FROM public.recursos WHERE identity_key=$1`, identity).Scan(&id)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("resource %q after rollback = %d, error %v; want no row", identity, id, err)
	}
}

func teardownCardinalityFixture(t *testing.T, pool *pgxpool.Pool, classID, familyID, typeID, quantityDefinitionID int64) {
	t.Helper()
	clearCardinalityResources(t, pool)
	_, _ = pool.Exec(context.Background(), `DELETE FROM public.resource_attributes WHERE family_id=$1`, familyID)
	_, _ = pool.Exec(context.Background(), `DELETE FROM public.resource_unit_policies WHERE family_id=$1`, familyID)
	_, _ = pool.Exec(context.Background(), `DELETE FROM public.resource_types WHERE id=$1`, typeID)
	_, _ = pool.Exec(context.Background(), `DELETE FROM public.resource_families WHERE id=$1 AND class_id=$2`, familyID, classID)
	_, _ = pool.Exec(context.Background(), `DELETE FROM public.attribute_definitions WHERE id=$1`, quantityDefinitionID)
}
