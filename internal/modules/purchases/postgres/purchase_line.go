package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/GARFEX33/garfex-backend/internal/modules/purchases/domain"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

const purchaseLineColumns = `pl.id, pl.purchase_id, pl.line_number, pl.description, pl.supplier_sku, pl.sat_product_code,
	pl.quantity, pl.unit_code, pl.unit, pl.unit_price, pl.amount, pl.discount, pl.tax_transferred, pl.tax_withheld, pl.tax_object,
	pl.supplier_product_id, pl.resolution_override, pl.resolution_revision, sp.resource_id, COALESCE(sp.mapping_identity_conflict, FALSE), r.active`

const purchaseLineReturningColumns = `id, purchase_id, line_number, description, supplier_sku, sat_product_code,
	quantity, unit_code, unit, unit_price, amount, discount, tax_transferred, tax_withheld, tax_object,
	supplier_product_id, resolution_override, resolution_revision,
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

const purchaseLineWorkbenchBaseSQL = `WITH line_projection AS (
	SELECT
		pl.id AS line_id, pl.line_number, pl.purchase_id, p.issued_at, p.series, p.folio, p.cfdi_uuid,
		p.supplier_id, COALESCE(NULLIF(BTRIM(s.trade_name), ''), NULLIF(BTRIM(s.legal_name), ''),
			NULLIF(BTRIM(s.tax_identifier), ''), s.id::text) AS supplier_display_name,
		pl.description, pl.supplier_sku, sp.supplier_sku AS commercial_supplier_sku,
		pl.sat_product_code, pl.quantity, pl.unit_code, pl.unit, pl.unit_price, pl.amount,
		p.currency, pl.supplier_product_id, sp.resource_id, r.identity_key AS resource_identity,
		COALESCE(NULLIF(BTRIM(r.display_name), ''), r.identity_key) AS resource_display_name,
		sp.mapping_revision, pl.resolution_override, pl.resolution_revision,
		CASE
			WHEN pl.resolution_override = 'NO_APLICA' THEN 'NO_APLICA'
			WHEN pl.resolution_override = 'CONFLICTO' THEN 'CONFLICTO'
			WHEN COALESCE(sp.mapping_identity_conflict, FALSE) THEN 'CONFLICTO'
			WHEN sp.resource_id IS NOT NULL AND NOT COALESCE(r.active, TRUE) THEN 'SUSPENDIDO'
			WHEN sp.resource_id IS NULL THEN 'PENDIENTE'
			ELSE 'VINCULADO'
		END AS effective_status,
		CASE
			WHEN pl.resolution_override = 'NO_APLICA' THEN 'LINE_NOT_APPLICABLE'
			WHEN pl.resolution_override = 'CONFLICTO' THEN 'LINE_CONFLICT_OVERRIDE'
			WHEN COALESCE(sp.mapping_identity_conflict, FALSE) THEN 'IDENTITY_CONFLICT'
			WHEN sp.resource_id IS NOT NULL AND NOT COALESCE(r.active, TRUE) THEN 'RESOURCE_INACTIVE'
			WHEN sp.resource_id IS NULL THEN 'UNRESOLVED'
			ELSE 'NONE'
		END AS effective_cause
	FROM public.purchase_lines pl
	JOIN public.purchases p ON p.id = pl.purchase_id
	JOIN public.suppliers s ON s.id = p.supplier_id
	LEFT JOIN public.supplier_products sp ON sp.id = pl.supplier_product_id
	LEFT JOIN public.recursos r ON r.id = sp.resource_id
)`

func (r *repository) ListPurchaseLinesWorkbench(ctx context.Context, criteria domain.PurchaseLineWorkbenchCriteria) ([]domain.PurchaseLineWorkbenchRow, error) {
	if err := r.ready(); err != nil {
		return nil, err
	}
	query, args := buildPurchaseLineWorkbenchQuery(criteria)
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, wrapRead("list purchase lines workbench", err)
	}
	defer rows.Close()
	result := make([]domain.PurchaseLineWorkbenchRow, 0)
	for rows.Next() {
		row, err := scanPurchaseLineWorkbenchRow(rows)
		if err != nil {
			return nil, wrapRead("scan purchase line workbench row", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapRead("read purchase lines workbench", err)
	}
	return result, nil
}

func buildPurchaseLineWorkbenchQuery(criteria domain.PurchaseLineWorkbenchCriteria) (string, []any) {
	args := make([]any, 0, 10)
	conditions := make([]string, 0, 8)
	add := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}
	if criteria.SupplierID != nil {
		conditions = append(conditions, "lp.supplier_id = "+add(*criteria.SupplierID))
	}
	if criteria.EffectiveStatus != "" {
		conditions = append(conditions, "lp.effective_status = "+add(string(criteria.EffectiveStatus)))
	}
	if criteria.DateFrom != nil {
		conditions = append(conditions, "lp.issued_at >= "+add(*criteria.DateFrom))
	}
	if criteria.DateTo != nil {
		conditions = append(conditions, "lp.issued_at <= "+add(*criteria.DateTo))
	}
	if criteria.InvoiceText != "" {
		arg := add("%" + criteria.InvoiceText + "%")
		conditions = append(conditions, "(lp.series ILIKE "+arg+" OR lp.folio ILIKE "+arg+" OR lp.cfdi_uuid ILIKE "+arg+")")
	}
	if criteria.SupplierSKU != "" {
		conditions = append(conditions, "lp.supplier_sku ILIKE "+add("%"+criteria.SupplierSKU+"%"))
	}
	if criteria.Description != "" {
		conditions = append(conditions, "lp.description ILIKE "+add("%"+criteria.Description+"%"))
	}
	pageLimit := criteria.Limit
	if pageLimit <= 0 {
		pageLimit = 50
	} else if pageLimit > 51 {
		// Internal callers may request one extra row to derive HasNext, but
		// persistence never accepts an unbounded workbench page.
		pageLimit = 51
	}
	args = append(args, pageLimit, offset(criteria.Offset))
	limitArg := fmt.Sprintf("$%d", len(args)-1)
	offsetArg := fmt.Sprintf("$%d", len(args))
	where := "TRUE"
	if len(conditions) > 0 {
		where = strings.Join(conditions, " AND ")
	}
	return purchaseLineWorkbenchBaseSQL + `
SELECT line_id, line_number, purchase_id, issued_at, series, folio, cfdi_uuid, supplier_id, supplier_display_name,
	description, supplier_sku, commercial_supplier_sku, sat_product_code, quantity, unit_code, unit,
	unit_price, amount, currency, supplier_product_id, resource_id, resource_identity, resource_display_name,
	mapping_revision, resolution_override, resolution_revision, effective_status, effective_cause
FROM line_projection lp
WHERE ` + where + `
ORDER BY issued_at DESC, line_id DESC
LIMIT ` + limitArg + ` OFFSET ` + offsetArg, args
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
		&line.SupplierProductID, &override, &line.ResolutionRevision, &resourceID, &identityConflict, &resourceActive,
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

func scanPurchaseLineWorkbenchRow(row scanner) (domain.PurchaseLineWorkbenchRow, error) {
	var result domain.PurchaseLineWorkbenchRow
	var quantity, unitPrice, amount string
	var commercialSupplierSKU, resourceIdentity, resourceDisplayName *string
	var mappingRevision *uint64
	var override, effectiveStatus, effectiveCause string
	err := row.Scan(
		&result.LineID, &result.LineNumber, &result.PurchaseID, &result.IssuedAt, &result.Series, &result.Folio, &result.CFDIUUID,
		&result.SupplierID, &result.SupplierDisplayName, &result.Description, &result.SupplierSKU,
		&commercialSupplierSKU, &result.SATProductCode, &quantity, &result.UnitCode, &result.Unit,
		&unitPrice, &amount, &result.Currency, &result.SupplierProductID, &result.ResourceID,
		&resourceIdentity, &resourceDisplayName, &mappingRevision, &override, &result.ResolutionRevision, &effectiveStatus, &effectiveCause,
	)
	if err != nil {
		return domain.PurchaseLineWorkbenchRow{}, err
	}
	result.CommercialSupplierSKU = commercialSupplierSKU
	result.ResourceIdentity = resourceIdentity
	result.ResourceDisplayName = resourceDisplayName
	if mappingRevision != nil {
		revision := domain.MappingRevision(*mappingRevision)
		result.MappingRevision = &revision
	}
	result.ResolutionOverride = domain.LinkStatus(override)
	result.EffectiveStatus = domain.LinkStatus(effectiveStatus)
	result.EffectiveCause = domain.MappingCause(effectiveCause)
	if result.Quantity, err = decimal.NewFromString(quantity); err != nil {
		return domain.PurchaseLineWorkbenchRow{}, fmt.Errorf("decode quantity: %w", err)
	}
	if result.UnitPrice, err = decimal.NewFromString(unitPrice); err != nil {
		return domain.PurchaseLineWorkbenchRow{}, fmt.Errorf("decode unit price: %w", err)
	}
	if result.Amount, err = decimal.NewFromString(amount); err != nil {
		return domain.PurchaseLineWorkbenchRow{}, fmt.Errorf("decode amount: %w", err)
	}
	return result, nil
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
