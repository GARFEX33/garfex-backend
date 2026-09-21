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
		pl.supplier_product_id, pl.link_status,
		sp.id, sp.supplier_id, sp.supplier_sku, sp.description, sp.resource_id, sp.notes, sp.created_at, sp.updated_at,
		p.id, p.cfdi_uuid, p.supplier_id, p.branch_id, p.issued_at, p.currency
	FROM public.purchase_lines pl
	JOIN public.supplier_products sp ON sp.id = pl.supplier_product_id
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
	var quantity, unitPrice, amount, discount, taxTransferred, taxWithheld, linkStatus string
	var supplierProductID int64
	var resourceID *int64

	err := row.Scan(
		&entry.Line.ID, &entry.Line.PurchaseID, &entry.Line.LineNumber, &entry.Line.Description, &entry.Line.SupplierSKU, &entry.Line.SATProductCode,
		&quantity, &entry.Line.UnitCode, &entry.Line.Unit, &unitPrice, &amount, &discount, &taxTransferred, &taxWithheld, &entry.Line.TaxObject,
		&supplierProductID, &linkStatus,
		&entry.SupplierProduct.ID, &entry.SupplierProduct.SupplierID, &entry.SupplierProduct.SupplierSKU, &entry.SupplierProduct.Description,
		&resourceID, &entry.SupplierProduct.Notes, &entry.SupplierProduct.CreatedAt, &entry.SupplierProduct.UpdatedAt,
		&entry.PurchaseID, &entry.PurchaseUUID, &entry.SupplierID, &entry.BranchID, &entry.IssuedAt, &entry.Currency,
	)
	if err != nil {
		return domain.PurchaseLineHistory{}, err
	}

	entry.Line.SupplierProductID = &supplierProductID
	entry.Line.LinkStatus = domain.LinkStatus(linkStatus)
	entry.SupplierProduct.CurrentMapping = domain.SupplierProductMapping{ResourceID: resourceID}

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
