package postgres

import (
	"errors"
	"reflect"
	"testing"

	"github.com/GARFEX33/garfex-backend/internal/domain"
)

// TestClassifyLoadedCatalog is the RED+GREEN pair for LoadResourceCatalog's
// ErrCatalogEmpty branch (design §2): a genuinely empty catalog (migrations
// applied but never seeded) must be distinguishable from a load failure.
// This decision is deliberately pure/DB-free — this session's dev DB is
// always seeded by migration 000002, so it can never exercise the empty
// branch safely; see catalog_loader_integration_test.go for the real,
// non-empty hydration proof against that same DB.
func TestClassifyLoadedCatalog(t *testing.T) {
	tests := []struct {
		name    string
		catalog domain.ResourceCatalog
		wantErr error
	}{
		{name: "zero classes is empty", catalog: domain.ResourceCatalog{}, wantErr: ErrCatalogEmpty},
		{name: "one class is not empty", catalog: domain.ResourceCatalog{Classes: []domain.ResourceClass{{Code: "MATERIAL"}}}, wantErr: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := classifyLoadedCatalog(tt.catalog)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("classifyLoadedCatalog() = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("classifyLoadedCatalog() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// TestNormalizeCatalogForParity proves normalizeCatalogForParity resolves
// exactly the representational differences design D6 flagged between
// SeedResourceCatalog()'s Go literal and LoadResourceCatalog's DB-hydrated
// result: OptionSet "" vs "DEFAULT", and nil vs empty []string for
// Aliases/Keywords. It is a pure function, deliberately unit-tested without
// a real catalog or DB.
func TestNormalizeCatalogForParity(t *testing.T) {
	seedLike := domain.ResourceCatalog{
		Classes:    []domain.ResourceClass{{Code: "MATERIAL", Aliases: nil, Keywords: nil}},
		Attributes: []domain.ResourceAttribute{{Definition: domain.AttributeDefinition{Code: "gauge"}, OptionSet: ""}},
		Options:    []domain.AttributeOption{{AttributeCode: "gauge", Code: "10 AWG", OptionSet: ""}},
		Relations:  []domain.AttributeOptionRelation{{FromAttribute: "diameter_inch", OptionSet: ""}},
	}
	dbLike := domain.ResourceCatalog{
		Classes:    []domain.ResourceClass{{Code: "MATERIAL", Aliases: []string{}, Keywords: []string{}}},
		Attributes: []domain.ResourceAttribute{{Definition: domain.AttributeDefinition{Code: "gauge"}, OptionSet: "DEFAULT"}},
		Options:    []domain.AttributeOption{{AttributeCode: "gauge", Code: "10 AWG", OptionSet: "DEFAULT"}},
		Relations:  []domain.AttributeOptionRelation{{FromAttribute: "diameter_inch", OptionSet: "DEFAULT"}},
	}

	got, want := normalizeCatalogForParity(dbLike), normalizeCatalogForParity(seedLike)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeCatalogForParity() mismatch:\n got  = %+v\n want = %+v", got, want)
	}

	// Triangulation: a REAL difference (a different option code) must still
	// be caught after normalization — proves this isn't a trivial
	// always-equal normalizer.
	different := dbLike
	different.Options = []domain.AttributeOption{{AttributeCode: "gauge", Code: "12 AWG", OptionSet: "DEFAULT"}}
	if reflect.DeepEqual(normalizeCatalogForParity(different), want) {
		t.Fatalf("normalizeCatalogForParity() hid a real difference in Options[0].Code")
	}
}
