package postgres

import (
	"context"
	"errors"
	"os"
	"sort"
	"testing"

	"github.com/GARFEX33/garfex-backend/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

var conductoresScope = domain.ResourceScope{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"}
var canalizacionesScope = domain.ResourceScope{ClassCode: "MATERIAL", FamilyCode: "CANALIZACIONES", TypeCode: "TUBERIA"}

func TestResourceIntegrityMigrationIntegration(t *testing.T) {
	dsn := os.Getenv("GARFEX_ADMIN_TEST_DSN")
	if dsn == "" {
		t.Skip("GARFEX_ADMIN_TEST_DSN not set")
	}
	ctx := context.Background()
	p := openUnitTestPool(t, dsn)
	window := beginMigrationTestWindow(t, p)

	t.Run("blocks canonical collision", func(t *testing.T) {
		first := insertIntegrityResource(t, p, "TEST_RESOURCE_INTEGRITY_LEGACY_A")
		second := insertIntegrityResource(t, p, "TEST_RESOURCE_INTEGRITY_LEGACY_B")
		window.TrackResource(first)
		window.TrackResource(second)
		insertIntegrityOption(t, p, first, "conductor_material", "COBRE")
		insertIntegrityOption(t, p, second, "conductor_material", "COBRE")
		if _, err := p.Exec(ctx, `UPDATE public.recursos SET active=FALSE WHERE id=$1`, second); err != nil {
			t.Fatalf("deactivate collision fixture: %v", err)
		}
		if err := applyResourceIntegrityMigration(p, "000005_resource_integrity.up.sql"); err == nil {
			t.Fatal("migration accepted two resources with one canonical identity")
		}
		deleteIntegrityResources(t, p, first, second)
	})

	t.Run("blocks overlapping attribute applicability", func(t *testing.T) {
		var id int64
		err := p.QueryRow(ctx, `INSERT INTO public.resource_attributes
			(class_id, family_id, definition_id, mode, identity_participates)
			SELECT f.class_id, f.id, d.id, 'OPTIONAL', TRUE
			FROM public.resource_families f JOIN public.attribute_definitions d ON d.code='gauge'
			WHERE f.code='CONDUCTORES' RETURNING id`).Scan(&id)
		if err != nil {
			t.Fatalf("insert overlapping attribute: %v", err)
		}
		window.TrackAttribute(id)
		if err := applyResourceIntegrityMigration(p, "000005_resource_integrity.up.sql"); err == nil {
			t.Fatal("migration accepted family-wide and type-specific applicability")
		}
		if _, err := p.Exec(ctx, `DELETE FROM public.resource_attributes WHERE id=$1`, id); err != nil {
			t.Fatalf("delete overlapping attribute: %v", err)
		}
	})

	legacy := "TEST_RESOURCE_INTEGRITY_LEGACY_DRIFT"
	id := insertIntegrityResource(t, p, legacy)
	window.TrackResource(id)
	insertIntegrityOption(t, p, id, "conductor_material", "COBRE")
	if err := applyResourceIntegrityMigration(p, "000005_resource_integrity.up.sql"); err != nil {
		t.Fatalf("apply integrity migration: %v", err)
	}
	var key, oldKey, mappedKey string
	if err := p.QueryRow(ctx, `SELECT r.identity_key, m.legacy_identity_key, m.v1_identity_key
		FROM public.recursos r JOIN public.resource_integrity_identity_map m ON m.resource_id=r.id
		WHERE r.id=$1`, id).Scan(&key, &oldKey, &mappedKey); err != nil {
		t.Fatalf("read identity mapping: %v", err)
	}
	if key != legacy || oldKey != legacy || mappedKey != "v1|8:MATERIAL11:CONDUCTORES5:CABLE18:conductor_material17:CONTROLLED_OPTION5:COBRE" {
		t.Fatalf("identity mapping = %q/%q/%q, want unchanged legacy key %q and v1 mapping", key, oldKey, mappedKey, legacy)
	}
	if err := applyResourceIntegrityMigration(p, "000007_resource_identity_v1.up.sql"); err != nil {
		t.Fatalf("apply v1 migration: %v", err)
	}
	const admitted = "v1|8:MATERIAL11:CONDUCTORES5:CABLE18:conductor_material17:CONTROLLED_OPTION5:COBRE"
	if err := p.QueryRow(ctx, `SELECT identity_key FROM public.recursos WHERE id=$1`, id).Scan(&key); err != nil || key != admitted {
		t.Fatalf("rewritten identity = %q, error %v", key, err)
	}
	created := mustCreateResource(t, domain.SeedResourceCatalog(), conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "ALUMINIO"), domain.OptionValue("gauge", "12 AWG"), domain.OptionValue("insulation", "THW"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V")})
	if err := NewResourceRepository(p).Create(ctx, created); err != nil {
		t.Fatalf("post-up create: %v", err)
	}
	var createdID int64
	if err := p.QueryRow(ctx, `SELECT id FROM public.recursos WHERE identity_key=$1`, created.IdentityKey).Scan(&createdID); err != nil {
		t.Fatalf("read post-up resource id: %v", err)
	}
	window.TrackResource(createdID)
	if err := applyResourceIntegrityMigration(p, "000007_resource_identity_v1.down.sql"); err != nil {
		t.Fatalf("rollback v1 migration: %v", err)
	}
	var constraint bool
	if err := p.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='recursos_identity_key_v1')`).Scan(&constraint); err != nil || constraint {
		t.Fatalf("v1 constraint after rollback = %t, error %v", constraint, err)
	}
	if err := window.downMigration5(ctx); err != nil {
		t.Fatalf("rollback integrity migration: %v", err)
	}
	if err := p.QueryRow(ctx, `SELECT identity_key FROM public.recursos WHERE id=$1`, id).Scan(&key); err != nil || key != admitted {
		t.Fatalf("admitted key after rollback = %q, error %v", key, err)
	}
	if err := p.QueryRow(ctx, `SELECT identity_key FROM public.recursos WHERE identity_key=$1`, created.IdentityKey).Scan(&key); err != nil || key != created.IdentityKey {
		t.Fatalf("post-up key after rollback = %q, error %v", key, err)
	}
	deleteIntegrityResources(t, p, createdID)
	var mapExists, functionExists bool
	if err := p.QueryRow(ctx, `SELECT to_regclass('public.resource_integrity_identity_map') IS NOT NULL,
		to_regprocedure('public.resource_identity_component(text)') IS NOT NULL`).Scan(&mapExists, &functionExists); err != nil {
		t.Fatalf("inspect rolled-back integrity objects: %v", err)
	}
	if mapExists || functionExists {
		t.Fatalf("rolled-back integrity objects remain: map=%t function=%t", mapExists, functionExists)
	}
}

func applyResourceIntegrityMigration(pool *pgxpool.Pool, name string) error {
	return applySchemaMigration(pool, name)
}

func insertIntegrityResource(t *testing.T, pool *pgxpool.Pool, legacy string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(), `INSERT INTO public.recursos
		(class_id, family_id, type_id, natural_unit_id, display_name, identity_key)
		SELECT cl.id, f.id, ty.id, u.id, 'TEST_RESOURCE_INTEGRITY', $1
		FROM public.resource_classes cl JOIN public.resource_families f ON f.class_id=cl.id AND f.code='CONDUCTORES'
		JOIN public.resource_types ty ON ty.family_id=f.id AND ty.code='CABLE'
		JOIN public.unit_definitions u ON u.code='M' WHERE cl.code='MATERIAL' RETURNING id`, legacy).Scan(&id)
	if err != nil {
		t.Fatalf("insert integrity resource: %v", err)
	}
	return id
}

func insertIntegrityOption(t *testing.T, pool *pgxpool.Pool, resourceID int64, attribute, option string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `INSERT INTO public.resource_attribute_values
		(resource_id, family_id, resource_attribute_id, attribute_definition_id, option_code)
		SELECT r.id, r.family_id, ra.id, d.id, $2 FROM public.recursos r
		JOIN public.resource_attributes ra ON ra.family_id=r.family_id AND ra.type_id=r.type_id
		JOIN public.attribute_definitions d ON d.id=ra.definition_id AND d.code=$3
		WHERE r.id=$1`, resourceID, option, attribute)
	if err != nil {
		t.Fatalf("insert integrity option: %v", err)
	}
}

func deleteIntegrityResources(t *testing.T, pool *pgxpool.Pool, ids ...int64) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `DELETE FROM public.resource_attribute_values WHERE resource_id=ANY($1::bigint[])`, ids); err != nil {
		t.Fatalf("delete integrity resource values: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM public.recursos WHERE id=ANY($1::bigint[])`, ids); err != nil {
		t.Fatalf("delete integrity resources: %v", err)
	}
}

func TestResourceRepositoryIntegration(t *testing.T) {
	dsn := os.Getenv("GARFEX_TEST_DSN")
	if dsn == "" {
		t.Skip("GARFEX_TEST_DSN not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	defer pool.Close()

	repo := NewResourceRepository(pool)
	catalog := domain.SeedResourceCatalog()

	t.Run("insulated and desnudo round trip", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		defer cleanupResources(ctx, t, pool)
		insulated := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "10 AWG"), domain.OptionValue("insulation", "THW-LS"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V")})
		if err := repo.Create(ctx, insulated); err != nil {
			t.Fatalf("Create() insulated: %v", err)
		}
		assertRoundTrip(t, ctx, repo, insulated)
		desnudo, err := domain.HydrateResource(domain.ResourceSnapshot{ID: 1, ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE", NaturalUnit: "M", IdentityKey: "v1|8:MATERIAL11:CONDUCTORES5:CABLE18:conductor_material17:CONTROLLED_OPTION5:COBRE5:gauge17:CONTROLLED_OPTION6:12 AWG10:insulation17:CONTROLLED_OPTION7:DESNUDO"})
		if err != nil {
			t.Fatalf("hydrate desnudo: %v", err)
		}
		desnudo.Attributes = []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "12 AWG"), domain.OptionValue("insulation", "DESNUDO"), {AttributeCode: "color", Type: domain.ValueTypeControlledOption, Text: notApplicableState}, {AttributeCode: "voltage", Type: domain.ValueTypeControlledOption, Text: notApplicableState}}
		if err := repo.Create(ctx, desnudo); err != nil {
			t.Fatalf("Create() desnudo: %v", err)
		}
		assertRoundTrip(t, ctx, repo, desnudo)
	})

	t.Run("voltage identity canonicalizes duplicate", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		defer cleanupResources(ctx, t, pool)
		first := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "6 AWG"), domain.OptionValue("insulation", "THHN"), domain.OptionValue("color", "ROJO"), domain.OptionValue("voltage", "600 V")})
		second := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "6 AWG"), domain.OptionValue("insulation", "THHN"), domain.OptionValue("color", "ROJO"), domain.OptionValue("voltage", "600 V")})
		if err := repo.Create(ctx, first); err != nil {
			t.Fatalf("Create() first: %v", err)
		}
		if err := repo.Create(ctx, second); !errors.Is(err, domain.ErrDuplicateResource) {
			t.Fatalf("Create() duplicate error = %v, want ErrDuplicateResource", err)
		}
		assertRoundTrip(t, ctx, repo, first)
	})

	t.Run("natural unit persisted and excluded from identity", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		defer cleanupResources(ctx, t, pool)
		resource := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "ALUMINIO"), domain.OptionValue("gauge", "4 AWG"), domain.OptionValue("insulation", "XHHW-2"), domain.OptionValue("color", "AZUL"), domain.OptionValue("voltage", "600 V")})
		if err := repo.Create(ctx, resource); err != nil {
			t.Fatalf("Create() natural unit: %v", err)
		}
		got := assertRoundTrip(t, ctx, repo, resource)
		if got.NaturalUnit != "M" {
			t.Fatalf("NaturalUnit = %q, want M", got.NaturalUnit)
		}
		if got.IdentityKey != resource.IdentityKey {
			t.Fatalf("IdentityKey changed with NaturalUnit: %q", got.IdentityKey)
		}
	})

	t.Run("duplicate resource returns domain error", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		defer cleanupResources(ctx, t, pool)
		resource := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "2 AWG"), domain.OptionValue("insulation", "THW-LS"), domain.OptionValue("color", "VERDE"), domain.OptionValue("voltage", "600 V")})
		if err := repo.Create(ctx, resource); err != nil {
			t.Fatalf("Create() first: %v", err)
		}
		if err := repo.Create(ctx, resource); !errors.Is(err, domain.ErrDuplicateResource) {
			t.Fatalf("Create() duplicate error = %v, want ErrDuplicateResource", err)
		}
	})

	t.Run("invalid option returns reference error", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		defer cleanupResources(ctx, t, pool)
		resource := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "2 AWG"), domain.OptionValue("insulation", "THW-LS"), domain.OptionValue("color", "VERDE"), domain.OptionValue("voltage", "600 V")})
		resource.Attributes[1].OptionCode = "NOEXIST"
		if err := repo.Create(ctx, resource); !errors.Is(err, domain.ErrResourceReference) {
			t.Fatalf("Create() invalid option error = %v, want ErrResourceReference", err)
		}
	})

	t.Run("set without option returns reference error", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		defer cleanupResources(ctx, t, pool)
		resource := domain.Resource{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE", NaturalUnit: "M", IdentityKey: "MATERIAL|CONDUCTORES|CABLE|conductor_material=COBRE|gauge=2 AWG|insulation=THW-LS|color=|voltage=600 V", Attributes: []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), {AttributeCode: "gauge", Type: domain.ValueTypeControlledOption}, domain.OptionValue("insulation", "THW-LS"), {AttributeCode: "color", Type: domain.ValueTypeControlledOption}, domain.OptionValue("voltage", "600 V")}}
		if err := repo.Create(ctx, resource); !errors.Is(err, domain.ErrResourceReference) {
			t.Fatalf("Create() set without option error = %v, want ErrResourceReference", err)
		}
	})

	t.Run("not applicable with payload returns reference error", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		defer cleanupResources(ctx, t, pool)
		resource := domain.Resource{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE", NaturalUnit: "M", IdentityKey: "MATERIAL|CONDUCTORES|CABLE|conductor_material=COBRE|gauge=2 AWG|insulation=DESNUDO", Attributes: []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "2 AWG"), domain.OptionValue("insulation", "DESNUDO"), {AttributeCode: "color", Type: domain.ValueTypeControlledOption, OptionCode: "NEGRO", Text: notApplicableState}, {AttributeCode: "voltage", Type: domain.ValueTypeControlledOption, Text: notApplicableState}}}
		if err := repo.Create(ctx, resource); !errors.Is(err, domain.ErrResourceReference) {
			t.Fatalf("Create() NOT_APPLICABLE payload error = %v, want ErrResourceReference", err)
		}
	})

	t.Run("unknown family returns reference error", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		defer cleanupResources(ctx, t, pool)
		resource := domain.Resource{ClassCode: "MATERIAL", FamilyCode: "UNKNOWN", NaturalUnit: "M", IdentityKey: "MATERIAL|UNKNOWN|"}
		if err := repo.Create(ctx, resource); !errors.Is(err, domain.ErrResourceReference) {
			t.Fatalf("Create() unknown family error = %v, want ErrResourceReference", err)
		}
	})

	t.Run("canalizaciones round trip", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		defer cleanupResources(ctx, t, pool)
		resource := mustCreateResource(t, catalog, canalizacionesScope, "PZA", []domain.ResourceAttributeValue{domain.OptionValue("tipo", "CONDUIT PARED DELGADA"), domain.OptionValue("diameter_inch", `1/2"`), domain.OptionValue("diameter_mm", "13 mm")})
		if err := repo.Create(ctx, resource); err != nil {
			t.Fatalf("Create() canalizaciones: %v", err)
		}
		assertRoundTrip(t, ctx, repo, resource)
	})

	t.Run("canalizaciones search by inch and mm returns same resource", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		defer cleanupResources(ctx, t, pool)
		byInch := mustCreateResource(t, catalog, canalizacionesScope, "PZA", []domain.ResourceAttributeValue{domain.OptionValue("tipo", "CONDUIT PARED GRUESA"), domain.OptionValue("diameter_inch", `3/4"`), domain.OptionValue("diameter_mm", "19 mm")})
		if err := repo.Create(ctx, byInch); err != nil {
			t.Fatalf("Create() canalizaciones by inch: %v", err)
		}
		byMM := mustCreateResource(t, catalog, canalizacionesScope, "PZA", []domain.ResourceAttributeValue{domain.OptionValue("tipo", "CONDUIT PARED GRUESA"), domain.OptionValue("diameter_inch", `3/4"`), domain.OptionValue("diameter_mm", "19 mm")})
		if byInch.IdentityKey != byMM.IdentityKey {
			t.Fatalf("same technical identity expected: %q != %q", byInch.IdentityKey, byMM.IdentityKey)
		}
		got, err := repo.Get(ctx, byInch.ClassCode, byInch.IdentityKey)
		if err != nil {
			t.Fatalf("Get() canalizaciones: %v", err)
		}
		assertResourceEqual(t, got, byInch)
	})

	t.Run("search text matches identity key substring", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		defer cleanupResources(ctx, t, pool)
		match := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "10 AWG"), domain.OptionValue("insulation", "THW-LS"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V")})
		if err := repo.Create(ctx, match); err != nil {
			t.Fatalf("Create() match: %v", err)
		}
		other := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "8 AWG"), domain.OptionValue("insulation", "THHN"), domain.OptionValue("color", "BLANCO"), domain.OptionValue("voltage", "600 V")})
		if err := repo.Create(ctx, other); err != nil {
			t.Fatalf("Create() other: %v", err)
		}
		got, err := repo.Search(ctx, domain.SearchCriteria{Text: "THW-LS"})
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("Search() results = %d, want 1", len(got))
		}
		assertResourceEqual(t, got[0], match)
	})

	t.Run("search text matches family code or name", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		defer cleanupResources(ctx, t, pool)
		conductor := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "10 AWG"), domain.OptionValue("insulation", "THW"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V")})
		if err := repo.Create(ctx, conductor); err != nil {
			t.Fatalf("Create() conductor: %v", err)
		}
		canalizacion := mustCreateResource(t, catalog, canalizacionesScope, "PZA", []domain.ResourceAttributeValue{domain.OptionValue("tipo", "PVC CONDUIT"), domain.OptionValue("diameter_inch", `1"`), domain.OptionValue("diameter_mm", "25 mm")})
		if err := repo.Create(ctx, canalizacion); err != nil {
			t.Fatalf("Create() canalizacion: %v", err)
		}
		byCode, err := repo.Search(ctx, domain.SearchCriteria{Text: "CONDUCTORES"})
		if err != nil {
			t.Fatalf("Search() by code error = %v", err)
		}
		if len(byCode) != 1 || byCode[0].IdentityKey != conductor.IdentityKey {
			t.Fatalf("Search() by code = %+v, want only %+v", byCode, conductor)
		}
		byName, err := repo.Search(ctx, domain.SearchCriteria{Text: "Conductores"})
		if err != nil {
			t.Fatalf("Search() by name error = %v", err)
		}
		if len(byName) != 1 || byName[0].IdentityKey != conductor.IdentityKey {
			t.Fatalf("Search() by name = %+v, want only %+v", byName, conductor)
		}
	})

	t.Run("search text with no match returns empty result without error", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		defer cleanupResources(ctx, t, pool)
		resource := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "10 AWG"), domain.OptionValue("insulation", "THW"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V")})
		if err := repo.Create(ctx, resource); err != nil {
			t.Fatalf("Create() resource: %v", err)
		}
		got, err := repo.Search(ctx, domain.SearchCriteria{Text: "no-such-substring"})
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("Search() results = %d, want 0", len(got))
		}
	})

	t.Run("search filters narrow results to exact attribute match", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		defer cleanupResources(ctx, t, pool)
		black := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "10 AWG"), domain.OptionValue("insulation", "THW"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V")})
		if err := repo.Create(ctx, black); err != nil {
			t.Fatalf("Create() black: %v", err)
		}
		white := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "8 AWG"), domain.OptionValue("insulation", "THHN"), domain.OptionValue("color", "BLANCO"), domain.OptionValue("voltage", "600 V")})
		if err := repo.Create(ctx, white); err != nil {
			t.Fatalf("Create() white: %v", err)
		}
		got, err := repo.Search(ctx, domain.SearchCriteria{Filters: []domain.ResourceAttributeValue{domain.OptionValue("color", "NEGRO")}})
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("Search() results = %d, want 1", len(got))
		}
		assertResourceEqual(t, got[0], black)
	})

	t.Run("search family code narrows to one family", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		defer cleanupResources(ctx, t, pool)
		conductor := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "10 AWG"), domain.OptionValue("insulation", "THW"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V")})
		if err := repo.Create(ctx, conductor); err != nil {
			t.Fatalf("Create() conductor: %v", err)
		}
		canalizacion := mustCreateResource(t, catalog, canalizacionesScope, "PZA", []domain.ResourceAttributeValue{domain.OptionValue("tipo", "PVC CONDUIT"), domain.OptionValue("diameter_inch", `1"`), domain.OptionValue("diameter_mm", "25 mm")})
		if err := repo.Create(ctx, canalizacion); err != nil {
			t.Fatalf("Create() canalizacion: %v", err)
		}
		got, err := repo.Search(ctx, domain.SearchCriteria{FamilyCode: "CANALIZACIONES"})
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}
		if len(got) != 1 || got[0].IdentityKey != canalizacion.IdentityKey {
			t.Fatalf("Search() = %+v, want only %+v", got, canalizacion)
		}
	})

	// R1 carve-out (design §Open Risks R1, tasks 4.2): Get's first argument
	// is the owning CLASS code, not the family code — the compiler cannot
	// catch a miswired call site since both are plain strings, so this case
	// is the mandatory RED-before-GREEN proof that Get scopes by class.
	t.Run("Get scopes by class code, a family code must not match", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		defer cleanupResources(ctx, t, pool)
		resource := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "10 AWG"), domain.OptionValue("insulation", "THW"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V")})
		if err := repo.Create(ctx, resource); err != nil {
			t.Fatalf("Create() resource: %v", err)
		}
		if _, err := repo.Get(ctx, resource.ClassCode, resource.IdentityKey); err != nil {
			t.Fatalf("Get() with class code = %v, want nil", err)
		}
		// CONDUCTORES is a family code, not a class code — passing it as the
		// first argument must behave exactly like an unknown class: not found.
		if _, err := repo.Get(ctx, resource.FamilyCode, resource.IdentityKey); !errors.Is(err, domain.ErrResourceNotFound) {
			t.Fatalf("Get() with family code in place of class = %v, want ErrResourceNotFound", err)
		}
	})

	t.Run("search combines text family code and filters with and", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		defer cleanupResources(ctx, t, pool)
		match := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "10 AWG"), domain.OptionValue("insulation", "THW-LS"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V")})
		if err := repo.Create(ctx, match); err != nil {
			t.Fatalf("Create() match: %v", err)
		}
		wrongColor := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "8 AWG"), domain.OptionValue("insulation", "THW-LS"), domain.OptionValue("color", "BLANCO"), domain.OptionValue("voltage", "600 V")})
		if err := repo.Create(ctx, wrongColor); err != nil {
			t.Fatalf("Create() wrongColor: %v", err)
		}
		wrongText := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "6 AWG"), domain.OptionValue("insulation", "THHN"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V")})
		if err := repo.Create(ctx, wrongText); err != nil {
			t.Fatalf("Create() wrongText: %v", err)
		}
		got, err := repo.Search(ctx, domain.SearchCriteria{
			Text:       "THW-LS",
			FamilyCode: "CONDUCTORES",
			Filters:    []domain.ResourceAttributeValue{domain.OptionValue("color", "NEGRO")},
		})
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("Search() results = %d, want 1", len(got))
		}
		assertResourceEqual(t, got[0], match)
	})

	t.Run("search class code narrows to one class", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		defer cleanupResources(ctx, t, pool)
		conductor := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "10 AWG"), domain.OptionValue("insulation", "THW"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V")})
		if err := repo.Create(ctx, conductor); err != nil {
			t.Fatalf("Create() conductor: %v", err)
		}
		got, err := repo.Search(ctx, domain.SearchCriteria{ClassCode: "MATERIAL"})
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}
		if len(got) != 1 || got[0].IdentityKey != conductor.IdentityKey {
			t.Fatalf("Search() = %+v, want only %+v", got, conductor)
		}
		none, err := repo.Search(ctx, domain.SearchCriteria{ClassCode: "MANO_DE_OBRA"})
		if err != nil {
			t.Fatalf("Search() by unrelated class error = %v", err)
		}
		if len(none) != 0 {
			t.Fatalf("Search() by unrelated class = %+v, want empty", none)
		}
	})

	t.Run("search orders results deterministically by identity key", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		defer cleanupResources(ctx, t, pool)
		// Create in a deliberately "wrong" gauge order so insertion order
		// cannot be mistaken for the ordering guarantee under test.
		gauges := []string{"6 AWG", "10 AWG", "8 AWG"}
		for _, gauge := range gauges {
			resource := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", gauge), domain.OptionValue("insulation", "THW"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V")})
			if err := repo.Create(ctx, resource); err != nil {
				t.Fatalf("Create() gauge %s: %v", gauge, err)
			}
		}
		got, err := repo.Search(ctx, domain.SearchCriteria{FamilyCode: "CONDUCTORES"})
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("Search() results = %d, want 3", len(got))
		}
		for i := 1; i < len(got); i++ {
			if got[i-1].IdentityKey >= got[i].IdentityKey {
				t.Fatalf("Search() results not sorted: %q >= %q", got[i-1].IdentityKey, got[i].IdentityKey)
			}
		}
	})

	t.Run("search limit and offset paginate results", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		defer cleanupResources(ctx, t, pool)
		gauges := []string{"14 AWG", "12 AWG", "10 AWG"}
		var created []domain.Resource
		for _, gauge := range gauges {
			resource := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", gauge), domain.OptionValue("insulation", "THW"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V")})
			if err := repo.Create(ctx, resource); err != nil {
				t.Fatalf("Create() gauge %s: %v", gauge, err)
			}
			created = append(created, resource)
		}
		sort.Slice(created, func(i, j int) bool { return created[i].IdentityKey < created[j].IdentityKey })

		firstPage, err := repo.Search(ctx, domain.SearchCriteria{FamilyCode: "CONDUCTORES", Limit: 2})
		if err != nil {
			t.Fatalf("Search() first page error = %v", err)
		}
		if len(firstPage) != 2 || firstPage[0].IdentityKey != created[0].IdentityKey || firstPage[1].IdentityKey != created[1].IdentityKey {
			t.Fatalf("Search() first page = %+v, want %+v", firstPage, created[:2])
		}

		secondPage, err := repo.Search(ctx, domain.SearchCriteria{FamilyCode: "CONDUCTORES", Limit: 2, Offset: 2})
		if err != nil {
			t.Fatalf("Search() second page error = %v", err)
		}
		if len(secondPage) != 1 || secondPage[0].IdentityKey != created[2].IdentityKey {
			t.Fatalf("Search() second page = %+v, want %+v", secondPage, created[2:])
		}
	})

	t.Run("search non-positive limit falls back to default and still returns rows", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		defer cleanupResources(ctx, t, pool)
		resource := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "10 AWG"), domain.OptionValue("insulation", "THW"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V")})
		if err := repo.Create(ctx, resource); err != nil {
			t.Fatalf("Create() resource: %v", err)
		}
		got, err := repo.Search(ctx, domain.SearchCriteria{FamilyCode: "CONDUCTORES", Limit: 0})
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}
		// This is the case that would silently break if LIMIT 0 were
		// reintroduced: a non-positive limit must fall back to
		// defaultSearchLimit, not return zero rows.
		if len(got) != 1 {
			t.Fatalf("Search() results = %d, want 1 (defaultSearchLimit fallback)", len(got))
		}
	})

	t.Run("update changes attributes and identity key keeping same id, old identity gone", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		defer cleanupResources(ctx, t, pool)
		original := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "10 AWG"), domain.OptionValue("insulation", "THW-LS"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V")})
		if err := repo.Create(ctx, original); err != nil {
			t.Fatalf("Create() original: %v", err)
		}
		stored, err := repo.Get(ctx, original.ClassCode, original.IdentityKey)
		if err != nil {
			t.Fatalf("Get() original: %v", err)
		}
		if stored.ID == 0 {
			t.Fatalf("Get() original ID = 0, want non-zero")
		}
		candidate := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "10 AWG"), domain.OptionValue("insulation", "THW-LS"), domain.OptionValue("color", "BLANCO"), domain.OptionValue("voltage", "600 V")})
		candidate.ID = stored.ID
		if err := repo.Update(ctx, candidate); err != nil {
			t.Fatalf("Update() = %v, want nil", err)
		}
		got, err := repo.Get(ctx, candidate.ClassCode, candidate.IdentityKey)
		if err != nil {
			t.Fatalf("Get() after update by new identity: %v", err)
		}
		if got.ID != stored.ID {
			t.Fatalf("Get() after update ID = %d, want %d", got.ID, stored.ID)
		}
		assertResourceEqual(t, got, candidate)
		if _, err := repo.Get(ctx, original.ClassCode, original.IdentityKey); !errors.Is(err, domain.ErrResourceNotFound) {
			t.Fatalf("Get() by old identity error = %v, want ErrResourceNotFound", err)
		}
	})

	t.Run("update to a colliding identity returns ErrDuplicateResource and leaves original unchanged", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		defer cleanupResources(ctx, t, pool)
		black := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "10 AWG"), domain.OptionValue("insulation", "THW-LS"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V")})
		if err := repo.Create(ctx, black); err != nil {
			t.Fatalf("Create() black: %v", err)
		}
		white := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "10 AWG"), domain.OptionValue("insulation", "THW-LS"), domain.OptionValue("color", "BLANCO"), domain.OptionValue("voltage", "600 V")})
		if err := repo.Create(ctx, white); err != nil {
			t.Fatalf("Create() white: %v", err)
		}
		stored, err := repo.Get(ctx, black.ClassCode, black.IdentityKey)
		if err != nil {
			t.Fatalf("Get() black: %v", err)
		}
		collision := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "10 AWG"), domain.OptionValue("insulation", "THW-LS"), domain.OptionValue("color", "BLANCO"), domain.OptionValue("voltage", "600 V")})
		collision.ID = stored.ID
		if err := repo.Update(ctx, collision); !errors.Is(err, domain.ErrDuplicateResource) {
			t.Fatalf("Update() error = %v, want ErrDuplicateResource", err)
		}
		assertRoundTrip(t, ctx, repo, black)
	})

	t.Run("update with unknown type returns ErrResourceReference", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		defer cleanupResources(ctx, t, pool)
		original := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "10 AWG"), domain.OptionValue("insulation", "THW-LS"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V")})
		if err := repo.Create(ctx, original); err != nil {
			t.Fatalf("Create() original: %v", err)
		}
		stored, err := repo.Get(ctx, original.ClassCode, original.IdentityKey)
		if err != nil {
			t.Fatalf("Get() original: %v", err)
		}
		candidate := stored
		candidate.TypeCode = "UNKNOWN_TYPE"
		if err := repo.Update(ctx, candidate); !errors.Is(err, domain.ErrResourceReference) {
			t.Fatalf("Update() error = %v, want ErrResourceReference", err)
		}
	})

	t.Run("update with unknown id returns ErrResourceNotFound", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		defer cleanupResources(ctx, t, pool)
		candidate := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "10 AWG"), domain.OptionValue("insulation", "THW-LS"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V")})
		candidate.ID = 999999999
		if err := repo.Update(ctx, candidate); !errors.Is(err, domain.ErrResourceNotFound) {
			t.Fatalf("Update() error = %v, want ErrResourceNotFound", err)
		}
	})

	t.Run("SetActive false hides from Search but Get still finds it, SetActive true restores Search", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		defer cleanupResources(ctx, t, pool)
		resource := mustCreateResource(t, catalog, conductoresScope, "M", []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "10 AWG"), domain.OptionValue("insulation", "THW-LS"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V")})
		if err := repo.Create(ctx, resource); err != nil {
			t.Fatalf("Create() resource: %v", err)
		}
		stored, err := repo.Get(ctx, resource.ClassCode, resource.IdentityKey)
		if err != nil {
			t.Fatalf("Get() resource: %v", err)
		}
		criteria := domain.SearchCriteria{FamilyCode: "CONDUCTORES"}
		before, err := repo.Search(ctx, criteria)
		if err != nil {
			t.Fatalf("Search() before: %v", err)
		}
		if len(before) != 1 {
			t.Fatalf("Search() before = %d results, want 1", len(before))
		}

		if err := repo.SetActive(ctx, stored.ID, false); err != nil {
			t.Fatalf("SetActive(false) = %v, want nil", err)
		}
		afterDeactivate, err := repo.Search(ctx, criteria)
		if err != nil {
			t.Fatalf("Search() after deactivate: %v", err)
		}
		if len(afterDeactivate) != 0 {
			t.Fatalf("Search() after deactivate = %d results, want 0", len(afterDeactivate))
		}
		if _, err := repo.Get(ctx, resource.ClassCode, resource.IdentityKey); err != nil {
			t.Fatalf("Get() after deactivate = %v, want nil (Get ignores active)", err)
		}

		if err := repo.SetActive(ctx, stored.ID, true); err != nil {
			t.Fatalf("SetActive(true) = %v, want nil", err)
		}
		afterRestore, err := repo.Search(ctx, criteria)
		if err != nil {
			t.Fatalf("Search() after restore: %v", err)
		}
		if len(afterRestore) != 1 {
			t.Fatalf("Search() after restore = %d results, want 1", len(afterRestore))
		}
	})

	t.Run("SetActive on unknown id returns ErrResourceNotFound", func(t *testing.T) {
		cleanupResources(ctx, t, pool)
		defer cleanupResources(ctx, t, pool)
		if err := repo.SetActive(ctx, 999999999, false); !errors.Is(err, domain.ErrResourceNotFound) {
			t.Fatalf("SetActive() error = %v, want ErrResourceNotFound", err)
		}
	})
}

func mustCreateResource(t *testing.T, catalog domain.ResourceCatalog, scope domain.ResourceScope, unit string, values []domain.ResourceAttributeValue) domain.Resource {
	t.Helper()
	resource, err := domain.NewResource(catalog, scope, unit, values)
	if err != nil {
		t.Fatalf("build resource: %v", err)
	}
	return resource
}

func cleanupResources(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, "DELETE FROM public.recursos"); err != nil {
		t.Fatalf("cleanup resources: %v", err)
	}
}

func assertRoundTrip(t *testing.T, ctx context.Context, repo domain.ResourceRepository, want domain.Resource) domain.Resource {
	t.Helper()
	got, err := repo.Get(ctx, want.ClassCode, want.IdentityKey)
	if err != nil {
		t.Fatalf("Get() round trip: %v", err)
	}
	assertResourceEqual(t, got, want)
	return got
}

func assertResourceEqual(t *testing.T, got, want domain.Resource) {
	t.Helper()
	if got.ClassCode != want.ClassCode || got.FamilyCode != want.FamilyCode || got.NaturalUnit != want.NaturalUnit || got.IdentityKey != want.IdentityKey {
		t.Fatalf("resource identity = %#v, want %#v", got, want)
	}
	if len(got.Attributes) != len(want.Attributes) {
		t.Fatalf("attribute count = %d, want %d", len(got.Attributes), len(want.Attributes))
	}
	gotAttrs := append([]domain.ResourceAttributeValue(nil), got.Attributes...)
	wantAttrs := append([]domain.ResourceAttributeValue(nil), want.Attributes...)
	sort.Slice(gotAttrs, func(i, j int) bool { return gotAttrs[i].AttributeCode < gotAttrs[j].AttributeCode })
	sort.Slice(wantAttrs, func(i, j int) bool { return wantAttrs[i].AttributeCode < wantAttrs[j].AttributeCode })
	for i := range wantAttrs {
		if gotAttrs[i].AttributeCode != wantAttrs[i].AttributeCode || gotAttrs[i].Type != wantAttrs[i].Type || gotAttrs[i].OptionCode != wantAttrs[i].OptionCode || gotAttrs[i].Text != wantAttrs[i].Text {
			t.Fatalf("attribute %d = %#v, want %#v", i, gotAttrs[i], wantAttrs[i])
		}
		if wantAttrs[i].Quantity != nil && (!gotAttrs[i].Quantity.Value.Equal(wantAttrs[i].Quantity.Value) || gotAttrs[i].Quantity.UnitCode != wantAttrs[i].Quantity.UnitCode) {
			t.Fatalf("quantity %d = %#v, want %#v", i, gotAttrs[i].Quantity, wantAttrs[i].Quantity)
		}
	}
}

// TestRevisionColumnDefaultsToOneOnResourceCreateIntegration proves a resource
// created through the real NewResourceRepository(...).Create() path — which
// never references the revision column — receives migration 8's backfilled
// default of 1, and that domain.Resource/domain.ResourceSnapshot gaining a
// Revision field left Create's SQL and callers unchanged (design "Resource
// revisions and identity").
func TestRevisionColumnDefaultsToOneOnResourceCreateIntegration(t *testing.T) {
	dsn := os.Getenv("GARFEX_TEST_DSN")
	if dsn == "" {
		t.Skip("GARFEX_TEST_DSN not set")
	}
	ctx := context.Background()
	pool := openUnitTestPool(t, dsn)
	created := mustCreateResource(t, domain.SeedResourceCatalog(), canalizacionesScope, "PZA", []domain.ResourceAttributeValue{domain.OptionValue("tipo", "PVC CONDUIT"), domain.OptionValue("diameter_inch", `1"`), domain.OptionValue("diameter_mm", "25 mm")})
	if err := NewResourceRepository(pool).Create(ctx, created); err != nil {
		t.Fatalf("create resource: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, `DELETE FROM public.resource_attribute_values WHERE resource_id IN (SELECT id FROM public.recursos WHERE identity_key=$1)`, created.IdentityKey); err != nil {
			t.Errorf("cleanup created resource values: %v", err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM public.recursos WHERE identity_key=$1`, created.IdentityKey); err != nil {
			t.Errorf("cleanup created resource: %v", err)
		}
	})
	var revision int64
	if err := pool.QueryRow(ctx, `SELECT revision FROM public.recursos WHERE identity_key=$1`, created.IdentityKey).Scan(&revision); err != nil {
		t.Fatalf("read created resource revision: %v", err)
	}
	if revision != 1 {
		t.Fatalf("created resource revision = %d, want 1", revision)
	}
}
