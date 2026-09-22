package app

import (
	"context"
	"strings"
	"time"

	"github.com/GARFEX33/garfex-backend/internal/modules/purchases/domain"
)

func (s *Service) ResolvePurchaseLine(ctx context.Context, command domain.ResolvePurchaseLineCommand) (domain.ResolvePurchaseLineResult, error) {
	if err := validID("purchase_line_id", command.LineID); err != nil {
		return domain.ResolvePurchaseLineResult{}, err
	}
	if err := validID("resource_id", command.ResourceID); err != nil {
		return domain.ResolvePurchaseLineResult{}, err
	}
	commercialSKU := strings.TrimSpace(command.CommercialSupplierSKU)
	if command.ExpectedSupplierProductID == nil {
		if command.ExpectedMappingRevision != nil {
			return domain.ResolvePurchaseLineResult{}, domain.ErrPurchaseLineStateConflict
		}
		if commercialSKU == "" {
			return domain.ResolvePurchaseLineResult{}, domain.ErrCommercialSupplierSKURequired
		}
	} else {
		if err := validID("expected_supplier_product_id", *command.ExpectedSupplierProductID); err != nil {
			return domain.ResolvePurchaseLineResult{}, err
		}
		if command.ExpectedMappingRevision == nil {
			return domain.ResolvePurchaseLineResult{}, domain.ErrPurchaseLineStateConflict
		}
		if commercialSKU != "" {
			return domain.ResolvePurchaseLineResult{}, domain.ErrCommercialSupplierSKUForbidden
		}
	}
	if strings.TrimSpace(command.Actor) == "" {
		return domain.ResolvePurchaseLineResult{}, domain.NewValidationError("actor", "is required")
	}
	command.CommercialSupplierSKU = commercialSKU
	command.Decision = domain.MappingDecisionMetadata{
		Actor: command.Actor, Origin: domain.MappingOriginManual,
		Reason: strings.TrimSpace(command.Reason), At: time.Now().UTC(),
	}
	result, err := s.repo.ResolvePurchaseLine(ctx, command)
	return result, wrap("resolve purchase line", err)
}

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

func (s *Service) SetResolutionOverride(ctx context.Context, command domain.SetResolutionOverrideCommand) (domain.PurchaseLine, error) {
	if err := validID("purchase_line_id", command.LineID); err != nil {
		return domain.PurchaseLine{}, err
	}
	if !command.Override.ValidOverride() {
		return domain.PurchaseLine{}, domain.NewValidationError("resolution_override", "must be NONE, NO_APLICA, or CONFLICTO")
	}
	if strings.TrimSpace(command.Actor) == "" {
		return domain.PurchaseLine{}, domain.NewValidationError("actor", "is required")
	}
	if strings.TrimSpace(command.Reason) == "" {
		return domain.PurchaseLine{}, domain.NewValidationError("reason", "is required")
	}
	command.Decision = domain.MappingDecisionMetadata{
		Actor: strings.TrimSpace(command.Actor), Origin: domain.MappingOriginManual,
		Reason: strings.TrimSpace(command.Reason), At: time.Now().UTC(),
	}
	line, err := s.repo.SetResolutionOverride(ctx, command)
	return line, wrap("set purchase line resolution override", err)
}

func (s *Service) ListMappingAudit(ctx context.Context, supplierProductID int64, criteria domain.ListCriteria) ([]domain.MappingAuditEntry, error) {
	if err := validID("supplier_product_id", supplierProductID); err != nil {
		return nil, err
	}
	entries, err := s.repo.ListMappingAudit(ctx, supplierProductID, criteria)
	return entries, wrap("list supplier product mapping audit", err)
}
