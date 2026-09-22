package postgres

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/GARFEX33/garfex-backend/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestLoadResourceCatalogIntegration proves LoadResourceCatalog fully
// hydrates the real, seeded catalog (migrations 000002+000003) — including
// nested Rules from the new resource_attribute_rules table — and that its
// declaration order matches domain.SeedResourceCatalog()'s Go literal via
// the new display_order columns (design §1d/§2). Skips per this project's
// existing GARFEX_TEST_DSN convention.
func TestLoadResourceCatalogIntegration(t *testing.T) {
	dsn := os.Getenv("GARFEX_TEST_DSN")
	if dsn == "" {
		t.Skip("GARFEX_TEST_DSN not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	t.Cleanup(pool.Close)

	catalog, err := LoadResourceCatalog(ctx, pool)
	if err != nil {
		t.Fatalf("LoadResourceCatalog() error = %v, want nil", err)
	}

	t.Run("classes hydrated in declaration order", func(t *testing.T) {
		var codes []string
		for _, class := range catalog.Classes {
			codes = append(codes, class.Code)
		}
		want := []string{"MATERIAL", "MANO_DE_OBRA", "EQUIPO_HERRAMIENTA"}
		if !reflect.DeepEqual(codes, want) {
			t.Fatalf("class codes = %v, want %v", codes, want)
		}
	})

	t.Run("gauge options preserve business order, not alphabetical", func(t *testing.T) {
		var found []string
		for _, option := range catalog.Options {
			if option.AttributeCode == "gauge" {
				found = append(found, option.Code)
			}
		}
		want := []string{"14 AWG", "12 AWG", "10 AWG", "8 AWG", "6 AWG", "4 AWG", "2 AWG", "1 AWG", "1/0 AWG", "2/0 AWG", "3/0 AWG", "4/0 AWG"}
		if !reflect.DeepEqual(found, want) {
			t.Fatalf("gauge option order = %v, want %v (must be business order 14->12->10 AWG, not alphabetical)", found, want)
		}
	})

	t.Run("conditional rules hydrate nested from resource_attribute_rules", func(t *testing.T) {
		if err := catalog.Validate(); err != nil {
			t.Fatalf("LoadResourceCatalog() produced an invalid catalog: %v", err)
		}
		var color, voltage *domain.ResourceAttribute
		for i := range catalog.Attributes {
			switch catalog.Attributes[i].Definition.Code {
			case "color":
				color = &catalog.Attributes[i]
			case "voltage":
				voltage = &catalog.Attributes[i]
			}
		}
		if color == nil || voltage == nil {
			t.Fatalf("color/voltage attributes not found in hydrated catalog")
		}
		for _, attribute := range []*domain.ResourceAttribute{color, voltage} {
			if attribute.Mode != domain.ModeConditional {
				t.Fatalf("%s Mode = %v, want ModeConditional", attribute.Definition.Code, attribute.Mode)
			}
			if len(attribute.Rules) != 1 {
				t.Fatalf("%s Rules = %+v, want exactly 1 rule", attribute.Definition.Code, attribute.Rules)
			}
			rule := attribute.Rules[0]
			if rule.When.AttributeCode != "insulation" || rule.When.Equals != "DESNUDO" {
				t.Fatalf("%s Rules[0].When = %+v, want insulation=DESNUDO", attribute.Definition.Code, rule.When)
			}
			if rule.Mode != domain.ModeForbidden || !rule.NotApplicable {
				t.Fatalf("%s Rules[0] = %+v, want Mode=FORBIDDEN NotApplicable=true", attribute.Definition.Code, rule)
			}
		}
		for _, tt := range []struct {
			name, insulation string
			wantMode         domain.AttributeMode
			wantNA           bool
		}{
			{"matching rule", "DESNUDO", domain.ModeForbidden, true},
			{"non-matching rule", "THW", domain.ModeRequired, false},
		} {
			t.Run(tt.name, func(t *testing.T) {
				mode, _, notApplicable := color.Effective([]domain.ResourceAttributeValue{domain.OptionValue("insulation", tt.insulation)})
				if mode != tt.wantMode || notApplicable != tt.wantNA {
					t.Fatalf("color.Effective(%s) = %v, %v; want %v, %v", tt.insulation, mode, notApplicable, tt.wantMode, tt.wantNA)
				}
			})
		}
	})

	t.Run("unit names hydrate without changing stable fields", func(t *testing.T) {
		want := map[string]string{"M": "Metro", "PZA": "Pieza"}
		seen := 0
		for _, unit := range catalog.Units {
			if expected, ok := want[unit.Code]; ok && (unit.Name != expected || unit.Symbol != unit.Code) {
				t.Errorf("unit %q = %+v, want name %q and stable symbol", unit.Code, unit, expected)
			} else if ok {
				seen++
			}
		}
		if seen != len(want) {
			t.Fatalf("hydrated unit count = %d, want %d approved units", seen, len(want))
		}
	})

	t.Run("parity with SeedResourceCatalog once normalized", func(t *testing.T) {
		got := normalizeCatalogForParity(catalog)
		want := normalizeCatalogForParity(domain.SeedResourceCatalog())
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("LoadResourceCatalog() does not match SeedResourceCatalog() after normalization\n got  = %+v\n want = %+v", got, want)
		}
	})
}

func TestLoadResourceCatalogPreservesEveryActiveFlag(t *testing.T) {
	dsn := os.Getenv("GARFEX_ADMIN_TEST_DSN")
	if dsn == "" {
		t.Skip("GARFEX_ADMIN_TEST_DSN not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	t.Cleanup(pool.Close)
	updates := []string{
		`UPDATE resource_classes SET active=false WHERE code='MATERIAL'`,
		`UPDATE resource_option_sets SET active=false WHERE code='DEFAULT'`,
		`UPDATE resource_families SET active=false WHERE code='CONDUCTORES'`,
		`UPDATE resource_types SET active=false WHERE code='CABLE'`,
		`UPDATE unit_definitions SET active=false WHERE code='M'`,
		`UPDATE resource_unit_policies p SET active=false FROM resource_families f, unit_definitions u WHERE p.family_id=f.id AND p.unit_id=u.id AND f.code='CONDUCTORES' AND u.code='M'`,
		`UPDATE attribute_definitions SET active=false WHERE code='gauge'`,
		`UPDATE resource_attributes ra SET active=false FROM attribute_definitions d WHERE ra.definition_id=d.id AND d.code='gauge'`,
		`UPDATE resource_attribute_rules rr SET active=false FROM resource_attributes ra JOIN attribute_definitions d ON d.id=ra.definition_id WHERE rr.resource_attribute_id=ra.id AND d.code='color'`,
		`UPDATE attribute_options SET active=false WHERE attribute_definition_id=(SELECT id FROM attribute_definitions WHERE code='conductor_material') AND code='COBRE'`,
		`UPDATE attribute_option_relations SET active=false WHERE id=(SELECT min(id) FROM attribute_option_relations)`,
		`UPDATE resource_type_presentation_fields pf SET active=false FROM resource_types t WHERE pf.type_id=t.id AND t.code='CABLE' AND pf.position=1`,
	}
	for _, query := range updates {
		if _, err := pool.Exec(ctx, query); err != nil {
			t.Fatalf("deactivate catalog flag with %q: %v", query, err)
		}
	}
	t.Cleanup(func() {
		for _, table := range []string{"resource_classes", "resource_option_sets", "resource_families", "resource_types", "unit_definitions", "resource_unit_policies", "attribute_definitions", "resource_attributes", "resource_attribute_rules", "attribute_options", "attribute_option_relations", "resource_type_presentation_fields"} {
			if _, err := pool.Exec(ctx, "UPDATE "+table+" SET active=true"); err != nil {
				t.Errorf("restore %s active flags: %v", table, err)
			}
		}
	})
	catalog, err := LoadResourceCatalog(ctx, pool)
	if err != nil {
		t.Fatalf("LoadResourceCatalog() error = %v", err)
	}
	if catalog.Classes[0].Active || catalog.OptionSets[0].Active || catalog.Families[0].Active || catalog.Types[0].Active || catalog.Units[0].Active {
		t.Fatalf("parent active flags were not preserved: class=%t option-set=%t family=%t type=%t unit=%t", catalog.Classes[0].Active, catalog.OptionSets[0].Active, catalog.Families[0].Active, catalog.Types[0].Active, catalog.Units[0].Active)
	}
	for _, policy := range catalog.UnitPolicies {
		if policy.FamilyCode == "CONDUCTORES" && policy.UnitCode == "M" && policy.Active {
			t.Fatal("unit policy active flag was not preserved")
		}
	}
	for _, definition := range catalog.Definitions {
		if definition.Code == "gauge" && definition.Active {
			t.Fatal("definition active flag was not preserved")
		}
	}
	for _, attribute := range catalog.Attributes {
		if attribute.Definition.Code == "gauge" && attribute.Active {
			t.Fatal("attribute binding active flag was not preserved")
		}
		if attribute.Definition.Code == "color" && attribute.Rules[0].Active {
			t.Fatal("rule active flag was not preserved")
		}
	}
	for _, option := range catalog.Options {
		if option.AttributeCode == "conductor_material" && option.Code == "COBRE" && option.Active {
			t.Fatal("option active flag was not preserved")
		}
	}
	if catalog.Relations[0].Active || catalog.PresentationFields[0].Active {
		t.Fatal("relation or presentation active flag was not preserved")
	}
}

// TestLoaderRevisionReadsBackfilledDefault proves loadRevision — the dormant
// read half of migration 8's CAS revision handling (design "Resource
// revisions and identity") — reads back the migration-8 backfilled default
// of 1 for an existing seeded catalog row, using the ordinary garfex_app
// role (no elevated privilege is required to read the column).
func TestLoaderRevisionReadsBackfilledDefault(t *testing.T) {
	dsn := os.Getenv("GARFEX_TEST_DSN")
	if dsn == "" {
		t.Skip("GARFEX_TEST_DSN not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	t.Cleanup(pool.Close)

	var classID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM public.resource_classes WHERE code = 'MATERIAL'`).Scan(&classID); err != nil {
		t.Fatalf("read seeded class id: %v", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })

	revision, err := loadRevision(ctx, tx, "resource_classes", classID)
	if err != nil {
		t.Fatalf("loadRevision() error = %v, want nil", err)
	}
	if revision != 1 {
		t.Fatalf("loadRevision() = %d, want 1 (migration-8 backfilled default)", revision)
	}
}

// TestLoaderRevisionRejectsUnknownTable proves loadRevision fails closed for
// any table migration 8 does not add a revision column to — most notably
// resource_attribute_rules, which deliberately has no independent revision —
// without needing a live database connection to prove it.
func TestLoaderRevisionRejectsUnknownTable(t *testing.T) {
	if _, err := loadRevision(context.Background(), nil, "resource_attribute_rules", 1); err == nil {
		t.Fatal("loadRevision() error = nil, want error for a table with no revision column")
	}
}
