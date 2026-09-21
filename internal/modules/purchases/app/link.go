package app

import (
	"context"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/modules/purchases/domain"
)

func (s *Service) ConfirmMapping(ctx context.Context, command domain.ConfirmMappingCommand) (domain.SupplierProduct, error) {
	if err := validID("supplier_product_id", command.SupplierProductID); err != nil {
		return domain.SupplierProduct{}, err
	}
	if err := validID("resource_id", command.ResourceID); err != nil {
		return domain.SupplierProduct{}, err
	}
	product, err := s.repo.ConfirmMapping(ctx, command)
	return product, wrap("confirm supplier product mapping", err)
}

func (s *Service) CorrectMapping(ctx context.Context, command domain.CorrectMappingCommand) (domain.SupplierProduct, error) {
	if err := validID("supplier_product_id", command.SupplierProductID); err != nil {
		return domain.SupplierProduct{}, err
	}
	if err := validID("resource_id", command.ResourceID); err != nil {
		return domain.SupplierProduct{}, err
	}
	product, err := s.repo.CorrectMapping(ctx, command)
	return product, wrap("correct supplier product mapping", err)
}

func (s *Service) ExceptionalUnlink(ctx context.Context, command domain.ExceptionalUnlinkCommand) (domain.SupplierProduct, error) {
	if err := validID("supplier_product_id", command.SupplierProductID); err != nil {
		return domain.SupplierProduct{}, err
	}
	product, err := s.repo.ExceptionalUnlink(ctx, command)
	return product, wrap("exceptionally unlink supplier product mapping", err)
}

func (s *Service) ReportIdentityConflict(ctx context.Context, command domain.ReportIdentityConflictCommand) (domain.SupplierProduct, error) {
	if err := validID("supplier_product_id", command.SupplierProductID); err != nil {
		return domain.SupplierProduct{}, err
	}
	product, err := s.repo.ReportIdentityConflict(ctx, command)
	return product, wrap("report supplier product identity conflict", err)
}

func (s *Service) ResolveIdentityConflict(ctx context.Context, command domain.ResolveIdentityConflictCommand) (domain.SupplierProduct, error) {
	if err := validID("supplier_product_id", command.SupplierProductID); err != nil {
		return domain.SupplierProduct{}, err
	}
	if err := validID("resource_id", command.ResourceID); err != nil {
		return domain.SupplierProduct{}, err
	}
	product, err := s.repo.ResolveIdentityConflict(ctx, command)
	return product, wrap("resolve supplier product identity conflict", err)
}

func (s *Service) MarkNotApplicable(ctx context.Context, lineID int64) (domain.PurchaseLine, error) {
	if err := validID("purchase_line_id", lineID); err != nil {
		return domain.PurchaseLine{}, err
	}
	line, err := s.repo.MarkNotApplicable(ctx, lineID)
	return line, wrap("mark purchase line not applicable", err)
}

func (s *Service) MarkConflict(ctx context.Context, lineID int64) (domain.PurchaseLine, error) {
	if err := validID("purchase_line_id", lineID); err != nil {
		return domain.PurchaseLine{}, err
	}
	line, err := s.repo.MarkConflict(ctx, lineID)
	return line, wrap("mark purchase line conflict", err)
}

func (s *Service) ClearOverride(ctx context.Context, lineID int64) (domain.PurchaseLine, error) {
	if err := validID("purchase_line_id", lineID); err != nil {
		return domain.PurchaseLine{}, err
	}
	line, err := s.repo.ClearOverride(ctx, lineID)
	return line, wrap("clear purchase line override", err)
}

func (s *Service) ListMappingAudit(ctx context.Context, supplierProductID int64, criteria domain.ListCriteria) ([]domain.MappingAuditEntry, error) {
	if err := validID("supplier_product_id", supplierProductID); err != nil {
		return nil, err
	}
	entries, err := s.repo.ListMappingAudit(ctx, supplierProductID, criteria)
	return entries, wrap("list supplier product mapping audit", err)
}
