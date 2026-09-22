package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/GARFEX33/garfex-backend/internal/modules/suppliers/domain"
	"github.com/jackc/pgx/v5"
)

const supplierColumns = `id, trade_name, legal_name, COALESCE(tax_identifier, ''), website, notes, active, created_at, updated_at`

const createSupplierSQL = `
	INSERT INTO public.suppliers (trade_name, legal_name, tax_identifier, website, notes, active)
	VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6)
	RETURNING ` + supplierColumns

const getSupplierSQL = `SELECT ` + supplierColumns + ` FROM public.suppliers WHERE id = $1`

// getSupplierByTaxIdentifierSQL repeats the expression of the unique index
// suppliers_tax_identifier_key so the lookup can use it. It deliberately does
// not filter on active: an inactive supplier still owns its tax identifier.
const getSupplierByTaxIdentifierSQL = `SELECT ` + supplierColumns + ` FROM public.suppliers
	WHERE tax_identifier IS NOT NULL AND upper(btrim(tax_identifier)) = upper(btrim($1))`

const searchSuppliersSQL = `
	SELECT ` + supplierColumns + ` FROM public.suppliers
	WHERE ($1 = '' OR trade_name ILIKE '%' || $1 || '%' OR legal_name ILIKE '%' || $1 || '%' OR tax_identifier ILIKE '%' || $1 || '%')
	  AND ($2::BOOLEAN IS NULL OR active = $2)
	ORDER BY lower(COALESCE(NULLIF(trade_name, ''), NULLIF(legal_name, ''), tax_identifier)), id
	LIMIT $3 OFFSET $4`

func (r *repository) CreateSupplier(ctx context.Context, value domain.Supplier) (domain.Supplier, error) {
	if err := r.ready(); err != nil {
		return domain.Supplier{}, err
	}
	created, err := scanSupplier(r.pool.QueryRow(ctx, createSupplierSQL, value.TradeName, value.LegalName, value.TaxIdentifier, value.Website, value.Notes, value.Active))
	return created, mapWriteError("create supplier", err)
}

func (r *repository) GetSupplier(ctx context.Context, id int64) (domain.Supplier, error) {
	if err := r.ready(); err != nil {
		return domain.Supplier{}, err
	}
	value, err := scanSupplier(r.pool.QueryRow(ctx, getSupplierSQL, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Supplier{}, fmt.Errorf("%w: id %d", domain.ErrSupplierNotFound, id)
	}
	return value, wrapRead("get supplier", err)
}

func (r *repository) GetSupplierByTaxIdentifier(ctx context.Context, taxID string) (domain.Supplier, error) {
	if err := r.ready(); err != nil {
		return domain.Supplier{}, err
	}
	value, err := scanSupplier(r.pool.QueryRow(ctx, getSupplierByTaxIdentifierSQL, taxID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Supplier{}, domain.ErrSupplierNotFound
	}
	return value, wrapRead("get supplier by tax identifier", err)
}

func (r *repository) SearchSuppliers(ctx context.Context, criteria domain.SupplierSearch) ([]domain.Supplier, error) {
	if err := r.ready(); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, searchSuppliersSQL, criteria.Text, nullableBool(criteria.Active), limit(criteria.Limit), offset(criteria.Offset))
	if err != nil {
		return nil, fmt.Errorf("search suppliers: %w", err)
	}
	defer rows.Close()
	values := make([]domain.Supplier, 0)
	for rows.Next() {
		value, err := scanSupplier(rows)
		if err != nil {
			return nil, fmt.Errorf("scan supplier: %w", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read suppliers: %w", err)
	}
	return values, nil
}

func (r *repository) UpdateSupplier(ctx context.Context, value domain.Supplier) (domain.Supplier, error) {
	if err := r.ready(); err != nil {
		return domain.Supplier{}, err
	}
	const query = `UPDATE public.suppliers SET trade_name=$2, legal_name=$3, tax_identifier=NULLIF($4, ''), website=$5, notes=$6 WHERE id=$1 RETURNING ` + supplierColumns
	updated, err := scanSupplier(r.pool.QueryRow(ctx, query, value.ID, value.TradeName, value.LegalName, value.TaxIdentifier, value.Website, value.Notes))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Supplier{}, fmt.Errorf("%w: id %d", domain.ErrSupplierNotFound, value.ID)
	}
	return updated, mapWriteError("update supplier", err)
}

func (r *repository) SetSupplierActive(ctx context.Context, id int64, active bool) (domain.Supplier, error) {
	if err := r.ready(); err != nil {
		return domain.Supplier{}, err
	}
	const query = `UPDATE public.suppliers SET active=$2 WHERE id=$1 RETURNING ` + supplierColumns
	updated, err := scanSupplier(r.pool.QueryRow(ctx, query, id, active))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Supplier{}, fmt.Errorf("%w: id %d", domain.ErrSupplierNotFound, id)
	}
	return updated, mapWriteError("set supplier active", err)
}

func scanSupplier(row scanner) (domain.Supplier, error) {
	var value domain.Supplier
	err := row.Scan(&value.ID, &value.TradeName, &value.LegalName, &value.TaxIdentifier, &value.Website, &value.Notes, &value.Active, &value.CreatedAt, &value.UpdatedAt)
	return value, err
}
