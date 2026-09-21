package postgres

import (
	"context"
	"fmt"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/modules/purchases/domain"
	"github.com/shopspring/decimal"
)

const listPurchaseLinesByResourceSQL = `
	SELECT
		pl.id, pl.purchase_id, pl.line_number, pl.description, pl.supplier_sku, pl.sat_product_code,
		pl.quantity, pl.unit_code, pl.unit, pl.unit_price, pl.amount, pl.discount, pl.tax_transferred, pl.tax_withheld, pl.tax_object,
		pl.supplier_product_id, pl.resolution_override, sp.resource_id, sp.mapping_identity_conflict, r.active,
		sp.id, sp.supplier_id, sp.supplier_sku, sp.description, sp.resource_id, sp.mapping_revision, sp.mapping_identity_conflict, r.active,
		sp.notes, sp.created_at, sp.updated_at,
		p.id, p.cfdi_uuid, p.supplier_id, p.branch_id, p.issued_at, p.currency
	FROM public.purchase_lines pl
	JOIN public.supplier_products sp ON sp.id = pl.supplier_product_id
	JOIN public.recursos r ON r.id = sp.resource_id
	JOIN public.purchases p ON p.id = pl.purchase_id
	WHERE sp.resource_id = $1
	ORDER BY p.issued_at DESC, pl.id DESC
	LIMIT $2 OFFSET $3`

func (r *repository) ListPurchaseLinesByResource(ctx context.Context, resourceID int64, criteria domain.ListCriteria) ([]domain.PurchaseLineHistory, error) {
	if err := r.ready(); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, listPurchaseLinesByResourceSQL, resourceID, limit(criteria.Limit), offset(criteria.Offset))
	if err != nil {
		return nil, wrapRead("list purchase lines by resource", err)
	}
	defer rows.Close()
	history := make([]domain.PurchaseLineHistory, 0)
	for rows.Next() {
		entry, err := scanPurchaseLineHistory(rows)
		if err != nil {
			return nil, wrapRead("scan purchase line history", err)
		}
		history = append(history, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapRead("read purchase line history", err)
	}
	return history, nil
}

func scanPurchaseLineHistory(row scanner) (domain.PurchaseLineHistory, error) {
	var entry domain.PurchaseLineHistory
	var quantity, unitPrice, amount, discount, taxTransferred, taxWithheld string
	var override string
	var lineResourceID *int64
	var lineIdentityConflict bool
	var lineResourceActive bool
	var supplierResourceID *int64
	var supplierResourceActive bool
	var supplierIdentityConflict bool
	err := row.Scan(
		&entry.Line.ID, &entry.Line.PurchaseID, &entry.Line.LineNumber, &entry.Line.Description, &entry.Line.SupplierSKU, &entry.Line.SATProductCode,
		&quantity, &entry.Line.UnitCode, &entry.Line.Unit, &unitPrice, &amount, &discount, &taxTransferred, &taxWithheld, &entry.Line.TaxObject,
		&entry.Line.SupplierProductID, &override, &lineResourceID, &lineIdentityConflict, &lineResourceActive,
		&entry.SupplierProduct.ID, &entry.SupplierProduct.SupplierID, &entry.SupplierProduct.SupplierSKU, &entry.SupplierProduct.Description,
		&supplierResourceID, &entry.SupplierProduct.MappingRevision, &supplierIdentityConflict, &supplierResourceActive,
		&entry.SupplierProduct.Notes, &entry.SupplierProduct.CreatedAt, &entry.SupplierProduct.UpdatedAt,
		&entry.PurchaseID, &entry.PurchaseUUID, &entry.SupplierID, &entry.BranchID, &entry.IssuedAt, &entry.Currency,
	)
	if err != nil {
		return domain.PurchaseLineHistory{}, err
	}
	entry.Line.ResolutionOverride = domain.LinkStatus(override)
	entry.SupplierProduct.CurrentMapping = domain.SupplierProductMapping{ResourceID: supplierResourceID, IdentityConflict: supplierIdentityConflict}
	entry.SupplierProduct.ResourceActive = &supplierResourceActive
	entry.Line.DerivedStatus, entry.Line.DerivedCause = entry.Line.EffectiveStatus(
		domain.SupplierProductMapping{ResourceID: lineResourceID, IdentityConflict: lineIdentityConflict}, lineResourceActive)

	if entry.Line.Quantity, err = decimal.NewFromString(quantity); err != nil {
		return domain.PurchaseLineHistory{}, fmt.Errorf("decode quantity: %w", err)
	}
	if entry.Line.UnitPrice, err = decimal.NewFromString(unitPrice); err != nil {
		return domain.PurchaseLineHistory{}, fmt.Errorf("decode unit price: %w", err)
	}
	if entry.Line.Amount, err = decimal.NewFromString(amount); err != nil {
		return domain.PurchaseLineHistory{}, fmt.Errorf("decode amount: %w", err)
	}
	if entry.Line.Discount, err = decimal.NewFromString(discount); err != nil {
		return domain.PurchaseLineHistory{}, fmt.Errorf("decode discount: %w", err)
	}
	if entry.Line.TaxTransferred, err = decimal.NewFromString(taxTransferred); err != nil {
		return domain.PurchaseLineHistory{}, fmt.Errorf("decode tax transferred: %w", err)
	}
	if entry.Line.TaxWithheld, err = decimal.NewFromString(taxWithheld); err != nil {
		return domain.PurchaseLineHistory{}, fmt.Errorf("decode tax withheld: %w", err)
	}
	return entry, nil
}
