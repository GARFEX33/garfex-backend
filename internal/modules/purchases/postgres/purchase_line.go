package postgres

import (
	"context"
	"fmt"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/modules/purchases/domain"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

const purchaseLineColumns = `pl.id, pl.purchase_id, pl.line_number, pl.description, pl.supplier_sku, pl.sat_product_code,
	pl.quantity, pl.unit_code, pl.unit, pl.unit_price, pl.amount, pl.discount, pl.tax_transferred, pl.tax_withheld, pl.tax_object,
	pl.supplier_product_id, pl.resolution_override, sp.resource_id, COALESCE(sp.mapping_identity_conflict, FALSE), r.active`

const purchaseLineReturningColumns = `id, purchase_id, line_number, description, supplier_sku, sat_product_code,
	quantity, unit_code, unit, unit_price, amount, discount, tax_transferred, tax_withheld, tax_object,
	supplier_product_id, resolution_override,
	(SELECT resource_id FROM public.supplier_products WHERE id = purchase_lines.supplier_product_id),
	COALESCE((SELECT mapping_identity_conflict FROM public.supplier_products WHERE id = purchase_lines.supplier_product_id), FALSE),
	(SELECT r.active FROM public.recursos r WHERE r.id = (SELECT resource_id FROM public.supplier_products WHERE id = purchase_lines.supplier_product_id))`

const listPurchaseLinesSQL = `SELECT ` + purchaseLineColumns + `
	FROM public.purchase_lines pl
	LEFT JOIN public.supplier_products sp ON sp.id = pl.supplier_product_id
	LEFT JOIN public.recursos r ON r.id = sp.resource_id
	WHERE pl.purchase_id = $1 ORDER BY pl.line_number`

func (r *repository) ListPurchaseLines(ctx context.Context, purchaseID int64) ([]domain.PurchaseLine, error) {
	if err := r.ready(); err != nil {
		return nil, err
	}
	return queryPurchaseLines(ctx, r.pool, listPurchaseLinesSQL, purchaseID)
}

func queryPurchaseLines(ctx context.Context, q querier, sql string, args ...any) ([]domain.PurchaseLine, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, wrapRead("list purchase lines", err)
	}
	defer rows.Close()
	lines := make([]domain.PurchaseLine, 0)
	for rows.Next() {
		line, err := scanPurchaseLine(rows)
		if err != nil {
			return nil, wrapRead("scan purchase line", err)
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapRead("read purchase lines", err)
	}
	return lines, nil
}

func scanPurchaseLine(row scanner) (domain.PurchaseLine, error) {
	var line domain.PurchaseLine
	var quantity, unitPrice, amount, discount, taxTransferred, taxWithheld string
	var resourceID *int64
	var identityConflict bool
	var resourceActive *bool
	var override string
	err := row.Scan(
		&line.ID, &line.PurchaseID, &line.LineNumber, &line.Description, &line.SupplierSKU, &line.SATProductCode,
		&quantity, &line.UnitCode, &line.Unit, &unitPrice, &amount, &discount, &taxTransferred, &taxWithheld, &line.TaxObject,
		&line.SupplierProductID, &override, &resourceID, &identityConflict, &resourceActive,
	)
	if err != nil {
		return domain.PurchaseLine{}, err
	}
	line.ResolutionOverride = domain.LinkStatus(override)
	active := true
	if resourceActive != nil {
		active = *resourceActive
	}
	line.DerivedStatus, line.DerivedCause = line.EffectiveStatus(domain.SupplierProductMapping{ResourceID: resourceID, IdentityConflict: identityConflict}, active)
	if line.Quantity, err = decimal.NewFromString(quantity); err != nil {
		return domain.PurchaseLine{}, fmt.Errorf("decode quantity: %w", err)
	}
	if line.UnitPrice, err = decimal.NewFromString(unitPrice); err != nil {
		return domain.PurchaseLine{}, fmt.Errorf("decode unit price: %w", err)
	}
	if line.Amount, err = decimal.NewFromString(amount); err != nil {
		return domain.PurchaseLine{}, fmt.Errorf("decode amount: %w", err)
	}
	if line.Discount, err = decimal.NewFromString(discount); err != nil {
		return domain.PurchaseLine{}, fmt.Errorf("decode discount: %w", err)
	}
	if line.TaxTransferred, err = decimal.NewFromString(taxTransferred); err != nil {
		return domain.PurchaseLine{}, fmt.Errorf("decode tax transferred: %w", err)
	}
	if line.TaxWithheld, err = decimal.NewFromString(taxWithheld); err != nil {
		return domain.PurchaseLine{}, fmt.Errorf("decode tax withheld: %w", err)
	}
	return line, nil
}

func insertPurchaseLine(ctx context.Context, tx pgx.Tx, purchaseID int64, line domain.PurchaseLineDraft, supplierProductID *int64) (domain.PurchaseLine, error) {
	const insertSQL = `INSERT INTO public.purchase_lines
		(purchase_id, line_number, description, supplier_sku, sat_product_code, quantity, unit_code, unit,
		 unit_price, amount, discount, tax_transferred, tax_withheld, tax_object, supplier_product_id, resolution_override)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		RETURNING ` + purchaseLineReturningColumns

	created, err := scanPurchaseLine(tx.QueryRow(ctx, insertSQL,
		purchaseID, line.LineNumber, line.Description, line.SupplierSKU, line.SATProductCode, line.Quantity, line.UnitCode, line.Unit,
		line.UnitPrice, line.Amount, line.Discount, line.TaxTransferred, line.TaxWithheld, line.TaxObject, supplierProductID, string(domain.LinkStatusNone),
	))
	if err != nil {
		return domain.PurchaseLine{}, mapWriteError("insert purchase line", err)
	}
	return created, nil
}
