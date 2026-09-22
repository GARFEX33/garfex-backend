package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/GARFEX33/garfex-backend/internal/modules/purchases/domain"
	"github.com/jackc/pgx/v5"
)

const lockPurchaseLineForResolveSQL = `SELECT pl.supplier_product_id, pl.resolution_override, pl.resolution_revision, p.supplier_id, pl.description
	FROM public.purchase_lines pl
	JOIN public.purchases p ON p.id = pl.purchase_id
	WHERE pl.id = $1
	FOR UPDATE OF pl`

const getPurchaseLineForResolveSQL = `SELECT ` + purchaseLineColumns + `
	FROM public.purchase_lines pl
	LEFT JOIN public.supplier_products sp ON sp.id = pl.supplier_product_id
	LEFT JOIN public.recursos r ON r.id = sp.resource_id
	WHERE pl.id = $1`

func (r *repository) ResolvePurchaseLine(ctx context.Context, command domain.ResolvePurchaseLineCommand) (domain.ResolvePurchaseLineResult, error) {
	if err := r.ready(); err != nil {
		return domain.ResolvePurchaseLineResult{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.ResolvePurchaseLineResult{}, wrapRead("begin purchase line resolution", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var actualSupplierProductID *int64
	var override string
	var resolutionRevision domain.ResolutionRevision
	var supplierID int64
	var description string
	if err := tx.QueryRow(ctx, lockPurchaseLineForResolveSQL, command.LineID).Scan(
		&actualSupplierProductID, &override, &resolutionRevision, &supplierID, &description,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ResolvePurchaseLineResult{}, domain.ErrPurchaseLineNotFound
		}
		return domain.ResolvePurchaseLineResult{}, wrapRead("lock purchase line", err)
	}
	if resolutionRevision != command.ExpectedResolutionRevision {
		return domain.ResolvePurchaseLineResult{}, domain.ErrStaleResolutionRevision
	}
	if !sameOptionalID(actualSupplierProductID, command.ExpectedSupplierProductID) {
		return domain.ResolvePurchaseLineResult{}, domain.ErrPurchaseLineStateConflict
	}
	if domain.LinkStatus(override) != domain.LinkStatusNone {
		return domain.ResolvePurchaseLineResult{}, domain.ErrPurchaseLineStateConflict
	}

	product, newlyAssociated, identityCreated, err := resolveSupplierProductIdentity(ctx, tx, supplierID, description, actualSupplierProductID, command)
	if err != nil {
		return domain.ResolvePurchaseLineResult{}, err
	}
	active := true
	if product.CurrentMapping.ResourceID != nil && product.ResourceActive != nil {
		active = *product.ResourceActive
	}
	status, _ := domain.EffectiveLineStatus(domain.LinkStatusNone, product.CurrentMapping, active)
	if actualSupplierProductID != nil && status != domain.LinkPending {
		return domain.ResolvePurchaseLineResult{}, domain.ErrPurchaseLineStateConflict
	}

	resourceStates, err := lockResources(ctx, tx, command.ResourceID)
	if err != nil {
		return domain.ResolvePurchaseLineResult{}, err
	}
	if err := requireTargetActive(resourceStates, command.ResourceID); err != nil {
		return domain.ResolvePurchaseLineResult{}, err
	}

	expectedRevision := product.MappingRevision
	if command.ExpectedMappingRevision != nil {
		expectedRevision = *command.ExpectedMappingRevision
	}
	transition, err := product.ConfirmMapping(command.ResourceID, expectedRevision, command.Decision)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidMappingTransition) && newlyAssociated {
			return domain.ResolvePurchaseLineResult{}, domain.ErrMappingTargetConflict
		}
		return domain.ResolvePurchaseLineResult{}, err
	}
	if err := persistMappingTransition(ctx, tx, product, transition, expectedRevision); err != nil {
		return domain.ResolvePurchaseLineResult{}, err
	}
	product.ResourceActive = boolPointer(true)

	if newlyAssociated {
		tag, err := tx.Exec(ctx, `UPDATE public.purchase_lines SET supplier_product_id = $2 WHERE id = $1 AND supplier_product_id IS NULL`, command.LineID, product.ID)
		if err != nil {
			return domain.ResolvePurchaseLineResult{}, mapWriteError("associate purchase line supplier product", err)
		}
		if tag.RowsAffected() != 1 {
			return domain.ResolvePurchaseLineResult{}, domain.ErrPurchaseLineStateConflict
		}
	}

	line, err := scanPurchaseLine(tx.QueryRow(ctx, getPurchaseLineForResolveSQL, command.LineID))
	if err != nil {
		return domain.ResolvePurchaseLineResult{}, wrapRead("read resolved purchase line", err)
	}
	if line.SupplierProductID == nil || *line.SupplierProductID != product.ID {
		return domain.ResolvePurchaseLineResult{}, fmt.Errorf("%w: resolved line association", domain.ErrPurchaseIntegrityConflict)
	}
	disposition := commercialIdentityDisposition(identityCreated, transition)
	if err := tx.Commit(ctx); err != nil {
		return domain.ResolvePurchaseLineResult{}, mapCommitError(err)
	}
	return domain.ResolvePurchaseLineResult{
		Line: line, SupplierProduct: product, CommercialIdentityDisposition: disposition,
	}, nil
}

func resolveSupplierProductIdentity(
	ctx context.Context,
	tx pgx.Tx,
	supplierID int64,
	description string,
	actualSupplierProductID *int64,
	command domain.ResolvePurchaseLineCommand,
) (domain.SupplierProduct, bool, bool, error) {
	if actualSupplierProductID != nil {
		product, err := scanSupplierProduct(tx.QueryRow(ctx, lockSupplierProductSQL, *actualSupplierProductID))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.SupplierProduct{}, false, false, domain.ErrPurchaseIntegrityConflict
			}
			return domain.SupplierProduct{}, false, false, wrapRead("lock purchase line supplier product", err)
		}
		if product.SupplierID != supplierID {
			return domain.SupplierProduct{}, false, false, domain.ErrPurchaseIntegrityConflict
		}
		return product, false, false, nil
	}

	product, created, err := resolveSupplierProduct(ctx, tx, supplierID, command.CommercialSupplierSKU, description)
	if err != nil {
		return domain.SupplierProduct{}, false, false, err
	}
	state := product.CurrentMapping.KnowledgeState()
	if state == domain.MappingStateConfirmed {
		if product.CurrentMapping.ResourceID == nil || *product.CurrentMapping.ResourceID != command.ResourceID {
			return domain.SupplierProduct{}, false, false, domain.ErrMappingTargetConflict
		}
	} else if state != domain.MappingStateUnresolved {
		return domain.SupplierProduct{}, false, false, domain.ErrMappingTargetConflict
	}
	return product, true, created, nil
}

func resolveSupplierProduct(ctx context.Context, tx pgx.Tx, supplierID int64, sku, description string) (domain.SupplierProduct, bool, error) {
	const insertSQL = `INSERT INTO public.supplier_products (supplier_id, supplier_sku, description)
		VALUES ($1, $2, $3)
		ON CONFLICT (supplier_id, supplier_sku) DO NOTHING
		RETURNING id, supplier_id, supplier_sku, description, resource_id, mapping_revision,
			mapping_identity_conflict, (SELECT r.active FROM public.recursos r WHERE r.id = supplier_products.resource_id),
			notes, created_at, updated_at`
	product, err := scanSupplierProduct(tx.QueryRow(ctx, insertSQL, supplierID, sku, description))
	if err == nil {
		return product, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.SupplierProduct{}, false, mapWriteError("insert resolved supplier product", err)
	}

	const reuseSQL = `UPDATE public.supplier_products sp
		SET description = $3
		WHERE sp.supplier_id = $1 AND sp.supplier_sku = $2
		RETURNING sp.id, sp.supplier_id, sp.supplier_sku, sp.description,
			sp.resource_id, sp.mapping_revision, sp.mapping_identity_conflict,
			(SELECT r.active FROM public.recursos r WHERE r.id = sp.resource_id),
			sp.notes, sp.created_at, sp.updated_at`
	product, err = scanSupplierProduct(tx.QueryRow(ctx, reuseSQL, supplierID, sku, description))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.SupplierProduct{}, false, domain.ErrPurchaseIntegrityConflict
		}
		return domain.SupplierProduct{}, false, mapWriteError("reuse resolved supplier product", err)
	}
	return product, false, nil
}

func commercialIdentityDisposition(identityCreated bool, transition domain.MappingTransition) domain.CommercialIdentityDisposition {
	if identityCreated {
		return domain.CommercialIdentityCreated
	}
	if transition.Changed {
		return domain.CommercialIdentityReused
	}
	return domain.CommercialIdentityAlreadyMapped
}

func sameOptionalID(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func boolPointer(value bool) *bool { return &value }
