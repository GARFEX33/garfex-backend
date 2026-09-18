package app

import (
	"context"
	"strings"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/modules/suppliers/domain"
)

func (s *Service) CreateSupplier(ctx context.Context, details domain.SupplierDetails) (domain.Supplier, error) {
	supplier, err := domain.NewSupplier(details)
	if err != nil {
		return domain.Supplier{}, err
	}
	created, err := s.repo.CreateSupplier(ctx, supplier)
	return created, wrap("create supplier", err)
}

func (s *Service) GetSupplier(ctx context.Context, id int64) (domain.Supplier, error) {
	if err := validID("supplier_id", id); err != nil {
		return domain.Supplier{}, err
	}
	supplier, err := s.repo.GetSupplier(ctx, id)
	return supplier, wrap("get supplier", err)
}

// GetSupplierByTaxIdentifier finds a supplier, active or not, by its tax
// identifier. The value is trimmed and upper-cased here so callers cannot
// bypass the normalization the unique index relies on.
func (s *Service) GetSupplierByTaxIdentifier(ctx context.Context, taxID string) (domain.Supplier, error) {
	taxID = strings.ToUpper(strings.TrimSpace(taxID))
	if taxID == "" {
		return domain.Supplier{}, domain.NewValidationError("tax_identifier", "must not be blank")
	}
	supplier, err := s.repo.GetSupplierByTaxIdentifier(ctx, taxID)
	return supplier, wrap("get supplier by tax identifier", err)
}

func (s *Service) SearchSuppliers(ctx context.Context, criteria domain.SupplierSearch) ([]domain.Supplier, error) {
	suppliers, err := s.repo.SearchSuppliers(ctx, criteria)
	return suppliers, wrap("search suppliers", err)
}

func (s *Service) UpdateSupplier(ctx context.Context, id int64, details domain.SupplierDetails) (domain.Supplier, error) {
	current, err := s.GetSupplier(ctx, id)
	if err != nil {
		return domain.Supplier{}, err
	}
	next, err := current.WithDetails(details)
	if err != nil {
		return domain.Supplier{}, err
	}
	updated, err := s.repo.UpdateSupplier(ctx, next)
	return updated, wrap("update supplier", err)
}

func (s *Service) DeactivateSupplier(ctx context.Context, id int64) (domain.Supplier, error) {
	return s.setSupplierActive(ctx, id, false)
}

func (s *Service) ReactivateSupplier(ctx context.Context, id int64) (domain.Supplier, error) {
	return s.setSupplierActive(ctx, id, true)
}

func (s *Service) setSupplierActive(ctx context.Context, id int64, active bool) (domain.Supplier, error) {
	current, err := s.GetSupplier(ctx, id)
	if err != nil {
		return domain.Supplier{}, err
	}
	var changed bool
	if active {
		changed = current.Activate()
	} else {
		changed = current.Deactivate()
	}
	if !changed {
		return current, nil
	}
	updated, err := s.repo.SetSupplierActive(ctx, id, active)
	return updated, wrap("set supplier active", err)
}
