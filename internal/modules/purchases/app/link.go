package app

import (
	"context"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/modules/purchases/domain"
)

// LinkSupplierProductToResource relates an existing supplier product to a
// Resource Master entry, either because a matching resource already exists
// or because one was just created through the normal resource flow. It
// never modifies any PurchaseLine's original data: every PENDIENTE line
// already referencing the supplier product becomes VINCULADO, and every
// future purchase line resolving to the same supplier product reuses this
// relation automatically.
func (s *Service) LinkSupplierProductToResource(ctx context.Context, supplierProductID, resourceID int64) (domain.SupplierProduct, error) {
	if err := validID("supplier_product_id", supplierProductID); err != nil {
		return domain.SupplierProduct{}, err
	}
	if err := validID("resource_id", resourceID); err != nil {
		return domain.SupplierProduct{}, err
	}
	product, err := s.repo.LinkSupplierProductToResource(ctx, supplierProductID, resourceID)
	return product, wrap("link supplier product to resource", err)
}

// UnlinkSupplierProduct removes a supplier product's Resource Master
// relation, correcting a previous assignment without touching any purchase
// line's original data. Every VINCULADO line referencing it reverts to
// PENDIENTE.
func (s *Service) UnlinkSupplierProduct(ctx context.Context, supplierProductID int64) (domain.SupplierProduct, error) {
	if err := validID("supplier_product_id", supplierProductID); err != nil {
		return domain.SupplierProduct{}, err
	}
	product, err := s.repo.UnlinkSupplierProduct(ctx, supplierProductID)
	return product, wrap("unlink supplier product", err)
}

// SetPurchaseLineLinkStatus manually overrides one line's linking state,
// for the cases automatic resolution must not decide on its own: marking a
// line NO_APLICA (it will never map to a Resource Master entry), or
// resolving a CONFLICTO a human has reviewed.
func (s *Service) SetPurchaseLineLinkStatus(ctx context.Context, lineID int64, status domain.LinkStatus) (domain.PurchaseLine, error) {
	if err := validID("purchase_line_id", lineID); err != nil {
		return domain.PurchaseLine{}, err
	}
	if !status.Valid() {
		return domain.PurchaseLine{}, domain.NewValidationError("link_status", "is not a recognized value")
	}
	line, err := s.repo.SetPurchaseLineLinkStatus(ctx, lineID, status)
	return line, wrap("set purchase line link status", err)
}
