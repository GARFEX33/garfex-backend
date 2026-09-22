package postgres

import (
	"context"
	"fmt"

	"github.com/GARFEX33/garfex-backend/internal/modules/purchases/domain"
	"github.com/jackc/pgx/v5"
)

const supplierProductColumns = `sp.id, sp.supplier_id, sp.supplier_sku, sp.description,
	sp.resource_id, sp.mapping_revision, sp.mapping_identity_conflict, r.active,
	sp.notes, sp.created_at, sp.updated_at`

const getSupplierProductSQL = `SELECT ` + supplierProductColumns + `
	FROM public.supplier_products sp LEFT JOIN public.recursos r ON r.id = sp.resource_id WHERE sp.id = $1`

const findSupplierProductSQL = `SELECT ` + supplierProductColumns + `
	FROM public.supplier_products sp LEFT JOIN public.recursos r ON r.id = sp.resource_id
	WHERE sp.supplier_id = $1 AND sp.supplier_sku = $2`

const listSupplierProductsSQL = `SELECT ` + supplierProductColumns + `
	FROM public.supplier_products sp LEFT JOIN public.recursos r ON r.id = sp.resource_id
	WHERE sp.supplier_id = $1 ORDER BY sp.supplier_sku LIMIT $2 OFFSET $3`

func (r *repository) GetSupplierProduct(ctx context.Context, id int64) (domain.SupplierProduct, error) {
	if err := r.ready(); err != nil {
		return domain.SupplierProduct{}, err
	}
	product, err := scanSupplierProduct(r.pool.QueryRow(ctx, getSupplierProductSQL, id))
	if err != nil {
		return domain.SupplierProduct{}, wrapRead("get supplier product", notFound(err, fmt.Errorf("%w: id %d", domain.ErrSupplierProductNotFound, id)))
	}
	return product, nil
}

func (r *repository) FindSupplierProduct(ctx context.Context, supplierID int64, sku string) (domain.SupplierProduct, error) {
	if err := r.ready(); err != nil {
		return domain.SupplierProduct{}, err
	}
	product, err := scanSupplierProduct(r.pool.QueryRow(ctx, findSupplierProductSQL, supplierID, sku))
	if err != nil {
		return domain.SupplierProduct{}, wrapRead("find supplier product", notFound(err, domain.ErrSupplierProductNotFound))
	}
	return product, nil
}

func (r *repository) ListSupplierProducts(ctx context.Context, supplierID int64, criteria domain.ListCriteria) ([]domain.SupplierProduct, error) {
	if err := r.ready(); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, listSupplierProductsSQL, supplierID, limit(criteria.Limit), offset(criteria.Offset))
	if err != nil {
		return nil, wrapRead("list supplier products", err)
	}
	defer rows.Close()
	products := make([]domain.SupplierProduct, 0)
	for rows.Next() {
		product, err := scanSupplierProduct(rows)
		if err != nil {
			return nil, wrapRead("scan supplier product", err)
		}
		products = append(products, product)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapRead("read supplier products", err)
	}
	return products, nil
}

func scanSupplierProduct(row scanner) (domain.SupplierProduct, error) {
	var product domain.SupplierProduct
	var resourceID *int64
	var resourceActive *bool
	err := row.Scan(
		&product.ID, &product.SupplierID, &product.SupplierSKU, &product.Description,
		&resourceID, &product.MappingRevision, &product.CurrentMapping.IdentityConflict, &resourceActive,
		&product.Notes, &product.CreatedAt, &product.UpdatedAt,
	)
	product.CurrentMapping.ResourceID = resourceID
	product.ResourceActive = resourceActive
	return product, err
}

// upsertSupplierProduct returns the complete current mapping projection for
// the identity. Import updates only last-seen description; mapping authority
// and revision are never changed by this path.
func upsertSupplierProduct(ctx context.Context, tx pgx.Tx, supplierID int64, sku, description string) (domain.SupplierProduct, error) {
	const upsertSQL = `INSERT INTO public.supplier_products (supplier_id, supplier_sku, description)
		VALUES ($1, $2, $3)
		ON CONFLICT (supplier_id, supplier_sku) DO UPDATE SET description = EXCLUDED.description
		RETURNING id, supplier_id, supplier_sku, description, resource_id, mapping_revision,
			mapping_identity_conflict, (SELECT r.active FROM public.recursos r WHERE r.id = supplier_products.resource_id),
			notes, created_at, updated_at`
	product, err := scanSupplierProduct(tx.QueryRow(ctx, upsertSQL, supplierID, sku, description))
	if err != nil {
		return domain.SupplierProduct{}, mapWriteError("upsert supplier product", err)
	}
	return product, nil
}
