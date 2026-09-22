package app

import (
	"context"

	"github.com/GARFEX33/garfex-backend/internal/modules/purchases/domain"
)

func (s *Service) GetPurchase(ctx context.Context, id int64) (domain.Purchase, error) {
	if err := validID("purchase_id", id); err != nil {
		return domain.Purchase{}, err
	}
	purchase, err := s.repo.GetPurchase(ctx, id)
	return purchase, wrap("get purchase", err)
}

func (s *Service) GetPurchaseByUUID(ctx context.Context, uuid string) (domain.Purchase, error) {
	purchase, err := s.repo.GetPurchaseByUUID(ctx, uuid)
	return purchase, wrap("get purchase by uuid", err)
}

func (s *Service) ListPurchaseLines(ctx context.Context, purchaseID int64) ([]domain.PurchaseLine, error) {
	if err := validID("purchase_id", purchaseID); err != nil {
		return nil, err
	}
	lines, err := s.repo.ListPurchaseLines(ctx, purchaseID)
	return lines, wrap("list purchase lines", err)
}

// ListPurchaseLinesWorkbench returns the global, derived PurchaseLine read
// projection for the workbench.
func (s *Service) ListPurchaseLinesWorkbench(ctx context.Context, criteria domain.PurchaseLineWorkbenchCriteria) ([]domain.PurchaseLineWorkbenchRow, error) {
	rows, err := s.repo.ListPurchaseLinesWorkbench(ctx, criteria)
	return rows, wrap("list purchase lines workbench", err)
}

// ListPurchasesBySupplier answers "what have we bought from this supplier",
// most recent purchase first.
func (s *Service) ListPurchasesBySupplier(ctx context.Context, supplierID int64, criteria domain.ListCriteria) ([]domain.Purchase, error) {
	if err := validID("supplier_id", supplierID); err != nil {
		return nil, err
	}
	purchases, err := s.repo.ListPurchasesBySupplier(ctx, supplierID, criteria)
	return purchases, wrap("list purchases by supplier", err)
}

func (s *Service) GetSupplierProduct(ctx context.Context, id int64) (domain.SupplierProduct, error) {
	if err := validID("supplier_product_id", id); err != nil {
		return domain.SupplierProduct{}, err
	}
	product, err := s.repo.GetSupplierProduct(ctx, id)
	return product, wrap("get supplier product", err)
}

func (s *Service) FindSupplierProduct(ctx context.Context, supplierID int64, sku string) (domain.SupplierProduct, error) {
	if err := validID("supplier_id", supplierID); err != nil {
		return domain.SupplierProduct{}, err
	}
	product, err := s.repo.FindSupplierProduct(ctx, supplierID, sku)
	return product, wrap("find supplier product", err)
}

func (s *Service) ListSupplierProducts(ctx context.Context, supplierID int64, criteria domain.ListCriteria) ([]domain.SupplierProduct, error) {
	if err := validID("supplier_id", supplierID); err != nil {
		return nil, err
	}
	products, err := s.repo.ListSupplierProducts(ctx, supplierID, criteria)
	return products, wrap("list supplier products", err)
}

// ListPurchaseLinesByResource answers, from a Resource Master entry, its
// full purchase history: which suppliers and supplier products have sold
// it, when, at what price and quantity, in what currency, and from which
// branch and source document. Most recent purchase first.
func (s *Service) ListPurchaseLinesByResource(ctx context.Context, resourceID int64, criteria domain.ListCriteria) ([]domain.PurchaseLineHistory, error) {
	if err := validID("resource_id", resourceID); err != nil {
		return nil, err
	}
	history, err := s.repo.ListPurchaseLinesByResource(ctx, resourceID, criteria)
	return history, wrap("list purchase lines by resource", err)
}
