// Package app implements UI-independent Purchase and Price History use cases.
package app

import (
	"context"
	"fmt"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/modules/purchases/domain"
	supplierdomain "github.com/GARFEX33/garfex-costos-unitarios/internal/modules/suppliers/domain"
)

// SupplierDirectory is the subset of the Supplier Master this module reuses:
// resolving the supplier that issued a purchase document by its tax
// identifier, minimally registering one when it is not yet known, and
// validating a claimed branch belongs to that supplier. It is satisfied
// structurally by *suppliers/app.Service; this module never depends on the
// Supplier Master's storage.
type SupplierDirectory interface {
	GetSupplierByTaxIdentifier(ctx context.Context, taxID string) (supplierdomain.Supplier, error)
	CreateSupplier(ctx context.Context, details supplierdomain.SupplierDetails) (supplierdomain.Supplier, error)
	GetBranch(ctx context.Context, supplierID, branchID int64) (supplierdomain.Branch, error)
}

type Service struct {
	repo      domain.Repository
	suppliers SupplierDirectory
}

func NewService(repo domain.Repository, suppliers SupplierDirectory) *Service {
	return &Service{repo: repo, suppliers: suppliers}
}

func validID(field string, id int64) error {
	if id <= 0 {
		return domain.NewValidationError(field, "must be positive")
	}
	return nil
}

func wrap(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}
