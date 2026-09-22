package app

import (
	"context"
	"errors"
	"testing"

	"github.com/GARFEX33/garfex-backend/internal/modules/suppliers/domain"
)

func TestBranchUseCasesAreSupplierScoped(t *testing.T) {
	ctx := context.Background()
	svc := NewService(newMemoryRepository())
	supplier, _ := svc.CreateSupplier(ctx, domain.SupplierDetails{TradeName: "Supplier One"})

	for _, name := range []string{"Downtown", "Warehouse"} {
		if _, err := svc.AddBranch(ctx, supplier.ID, domain.BranchDetails{Name: name, City: "Monterrey"}); err != nil {
			t.Fatalf("AddBranch(%s) error = %v", name, err)
		}
	}
	branches, err := svc.ListBranches(ctx, supplier.ID, domain.ListCriteria{})
	if err != nil || len(branches) != 2 {
		t.Fatalf("ListBranches() = %+v, %v; want two same-city branches", branches, err)
	}

	other, err := svc.CreateSupplier(ctx, domain.SupplierDetails{LegalName: "Other SA"})
	if err != nil {
		t.Fatalf("CreateSupplier(other) error = %v", err)
	}
	if got, err := svc.ListBranches(ctx, other.ID, domain.ListCriteria{}); err != nil || len(got) != 0 {
		t.Fatalf("ListBranches(other) = %+v, %v; want supplier-scoped empty list", got, err)
	}
	if _, err := svc.GetBranch(ctx, other.ID, branches[0].ID); !errors.Is(err, domain.ErrBranchNotFound) {
		t.Fatalf("GetBranch(cross supplier) error = %v, want ErrBranchNotFound", err)
	}
}
