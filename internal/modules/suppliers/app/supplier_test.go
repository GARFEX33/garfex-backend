package app

import (
	"context"
	"errors"
	"testing"

	"github.com/GARFEX33/garfex-backend/internal/modules/suppliers/domain"
)

func TestSupplierUseCasesSupportProgressiveEnrichment(t *testing.T) {
	ctx := context.Background()
	svc := NewService(newMemoryRepository())

	supplier, err := svc.CreateSupplier(ctx, domain.SupplierDetails{TradeName: "Supplier One"})
	if err != nil {
		t.Fatalf("CreateSupplier() error = %v", err)
	}
	if supplier.LegalName != "" || supplier.Website != "" || supplier.Notes != "" {
		t.Fatalf("progressive supplier = %+v, want omitted optional fields", supplier)
	}
}

func TestSupplierLifecycleKeepsInactiveRecordsQueryable(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryRepository()
	svc := NewService(repo)
	supplier, _ := svc.CreateSupplier(ctx, domain.SupplierDetails{TradeName: "Supplier"})

	deactivated, err := svc.DeactivateSupplier(ctx, supplier.ID)
	if err != nil || deactivated.Active {
		t.Fatalf("DeactivateSupplier() = %+v, %v", deactivated, err)
	}
	if repo.setSupplierActiveCalls != 1 {
		t.Fatalf("SetSupplierActive calls = %d, want 1", repo.setSupplierActiveCalls)
	}
	if _, err := svc.DeactivateSupplier(ctx, supplier.ID); err != nil {
		t.Fatalf("idempotent DeactivateSupplier() error = %v", err)
	}
	if repo.setSupplierActiveCalls != 1 {
		t.Fatal("idempotent deactivation must not persist twice")
	}

	got, err := svc.GetSupplier(ctx, supplier.ID)
	if err != nil || got.Active {
		t.Fatalf("GetSupplier(inactive) = %+v, %v", got, err)
	}
	all, err := svc.SearchSuppliers(ctx, domain.SupplierSearch{})
	if err != nil || len(all) != 1 || all[0].Active {
		t.Fatalf("SearchSuppliers(all) = %+v, %v; inactive must remain queryable", all, err)
	}

	reactivated, err := svc.ReactivateSupplier(ctx, supplier.ID)
	if err != nil || !reactivated.Active {
		t.Fatalf("ReactivateSupplier() = %+v, %v", reactivated, err)
	}
	if _, err := svc.ReactivateSupplier(ctx, supplier.ID); err != nil {
		t.Fatalf("idempotent ReactivateSupplier() error = %v", err)
	}
	if repo.setSupplierActiveCalls != 2 {
		t.Fatalf("SetSupplierActive calls = %d, want 2 after one deactivate and one reactivate", repo.setSupplierActiveCalls)
	}
}

func TestSupplierValidationHappensBeforePersistence(t *testing.T) {
	svc := NewService(newMemoryRepository())
	ctx := context.Background()
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "empty supplier", run: func() error { _, err := svc.CreateSupplier(ctx, domain.SupplierDetails{}); return err }},
		{name: "invalid supplier id", run: func() error { _, err := svc.GetSupplier(ctx, 0); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("error = %v, want ErrValidation", err)
			}
		})
	}
}
