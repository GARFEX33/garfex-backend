package app

import (
	"context"
	"errors"
	"testing"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/modules/suppliers/domain"
)

func TestGetSupplierByTaxIdentifier(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryRepository()
	svc := NewService(repo)
	active, err := svc.CreateSupplier(ctx, domain.SupplierDetails{LegalName: "ACME SA", TaxIdentifier: "ACM010101AA1"})
	if err != nil {
		t.Fatal(err)
	}
	inactive, err := svc.CreateSupplier(ctx, domain.SupplierDetails{LegalName: "OLD SA", TaxIdentifier: "OLD010101AA1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeactivateSupplier(ctx, inactive.ID); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		taxID      string
		wantID     int64
		wantErr    error
		wantLookup string
	}{
		{"exact", "ACM010101AA1", active.ID, nil, "ACM010101AA1"},
		{"lowercase and padded are normalized", "  acm010101aa1 \n", active.ID, nil, "ACM010101AA1"},
		{"inactive suppliers are found too", "OLD010101AA1", inactive.ID, nil, "OLD010101AA1"},
		{"unknown", "ZZZ010101AA1", 0, domain.ErrSupplierNotFound, "ZZZ010101AA1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo.getByTaxIDCalls = nil
			got, err := svc.GetSupplierByTaxIdentifier(ctx, tt.taxID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if got.ID != tt.wantID {
				t.Fatalf("id = %d, want %d", got.ID, tt.wantID)
			}
			if len(repo.getByTaxIDCalls) != 1 || repo.getByTaxIDCalls[0] != tt.wantLookup {
				t.Fatalf("repository lookups = %v, want [%s] (service must normalize)", repo.getByTaxIDCalls, tt.wantLookup)
			}
		})
	}
}

func TestGetSupplierByTaxIdentifierRejectsBlank(t *testing.T) {
	repo := newMemoryRepository()
	svc := NewService(repo)
	for _, taxID := range []string{"", "   ", "\t"} {
		_, err := svc.GetSupplierByTaxIdentifier(context.Background(), taxID)
		if !errors.Is(err, domain.ErrValidation) {
			t.Errorf("GetSupplierByTaxIdentifier(%q) error = %v, want validation", taxID, err)
		}
	}
	if len(repo.getByTaxIDCalls) != 0 {
		t.Fatalf("blank input reached the repository: %v", repo.getByTaxIDCalls)
	}
}
