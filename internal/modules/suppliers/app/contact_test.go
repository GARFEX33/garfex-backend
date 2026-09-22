package app

import (
	"context"
	"errors"
	"testing"

	"github.com/GARFEX33/garfex-backend/internal/modules/suppliers/domain"
)

func TestContactUseCasesEnforceBranchOwnership(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryRepository()
	svc := NewService(repo)
	supplier, _ := svc.CreateSupplier(ctx, domain.SupplierDetails{TradeName: "Supplier One"})
	branch, _ := svc.AddBranch(ctx, supplier.ID, domain.BranchDetails{Name: "Downtown"})
	other, _ := svc.CreateSupplier(ctx, domain.SupplierDetails{LegalName: "Other SA"})

	supplierContact, err := svc.AddContact(ctx, supplier.ID, domain.ContactDetails{Name: "Ana"})
	if err != nil || supplierContact.BranchID != nil {
		t.Fatalf("AddContact(supplier level) = %+v, %v", supplierContact, err)
	}
	branchContact, err := svc.AddContact(ctx, supplier.ID, domain.ContactDetails{Name: "Luis", BranchID: &branch.ID})
	if err != nil || branchContact.BranchID == nil || *branchContact.BranchID != branch.ID {
		t.Fatalf("AddContact(branch level) = %+v, %v", branchContact, err)
	}

	before := repo.createContactCalls
	if _, err := svc.AddContact(ctx, other.ID, domain.ContactDetails{Name: "Cross", BranchID: &branch.ID}); !errors.Is(err, domain.ErrBranchOwnership) {
		t.Fatalf("AddContact(cross supplier) error = %v, want ErrBranchOwnership", err)
	}
	if repo.createContactCalls != before {
		t.Fatal("cross-supplier contact reached persistence")
	}
}

func TestChildLifecycleDoesNotCascade(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryRepository()
	svc := NewService(repo)
	supplier, _ := svc.CreateSupplier(ctx, domain.SupplierDetails{TradeName: "Supplier"})
	branch, _ := svc.AddBranch(ctx, supplier.ID, domain.BranchDetails{City: "Puebla"})
	contact, _ := svc.AddContact(ctx, supplier.ID, domain.ContactDetails{Name: "Ana", BranchID: &branch.ID})

	if _, err := svc.DeactivateSupplier(ctx, supplier.ID); err != nil {
		t.Fatalf("DeactivateSupplier() error = %v", err)
	}
	storedBranch, _ := svc.GetBranch(ctx, supplier.ID, branch.ID)
	storedContact, _ := svc.GetContact(ctx, supplier.ID, contact.ID)
	if !storedBranch.Active || !storedContact.Active {
		t.Fatal("supplier deactivation cascaded domain state")
	}
	if _, err := svc.DeactivateBranch(ctx, supplier.ID, branch.ID); err != nil {
		t.Fatalf("DeactivateBranch() error = %v", err)
	}
	if got, _ := svc.GetContact(ctx, supplier.ID, contact.ID); !got.Active {
		t.Fatal("branch deactivation cascaded to contact")
	}
	if _, err := svc.ReactivateBranch(ctx, supplier.ID, branch.ID); err != nil {
		t.Fatalf("ReactivateBranch() error = %v", err)
	}
	if _, err := svc.DeactivateContact(ctx, supplier.ID, contact.ID); err != nil {
		t.Fatalf("DeactivateContact() error = %v", err)
	}
	if got, _ := svc.GetContact(ctx, supplier.ID, contact.ID); got.Active {
		t.Fatal("inactive contact was not queryable with inactive state")
	}
	if _, err := svc.ReactivateContact(ctx, supplier.ID, contact.ID); err != nil {
		t.Fatalf("ReactivateContact() error = %v", err)
	}
}

func TestContactValidationHappensBeforePersistence(t *testing.T) {
	repo := newMemoryRepository()
	svc := NewService(repo)
	ctx := context.Background()
	supplier, _ := svc.CreateSupplier(ctx, domain.SupplierDetails{TradeName: "Valid"})
	if _, err := svc.AddContact(ctx, supplier.ID, domain.ContactDetails{}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("AddContact() error = %v, want ErrValidation", err)
	}
}
