package postgres

import (
	"context"
	"fmt"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/modules/purchases/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const supplierProductColumns = `id, supplier_id, supplier_sku, description, resource_id, notes, created_at, updated_at`

const getSupplierProductSQL = `SELECT ` + supplierProductColumns + ` FROM public.supplier_products WHERE id = $1`

const findSupplierProductSQL = `SELECT ` + supplierProductColumns + `
	FROM public.supplier_products WHERE supplier_id = $1 AND supplier_sku = $2`

const listSupplierProductsSQL = `SELECT ` + supplierProductColumns + `
	FROM public.supplier_products WHERE supplier_id = $1 ORDER BY supplier_sku LIMIT $2 OFFSET $3`

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

func (r *repository) LinkSupplierProductToResource(ctx context.Context, supplierProductID, resourceID int64) (domain.SupplierProduct, error) {
	if err := r.ready(); err != nil {
		return domain.SupplierProduct{}, err
	}
	const updateSQL = `UPDATE public.supplier_products SET resource_id = $2 WHERE id = $1 RETURNING ` + supplierProductColumns
	product, err := scanSupplierProduct(r.pool.QueryRow(ctx, updateSQL, supplierProductID, resourceID))
	if err != nil {
		return domain.SupplierProduct{}, mapWriteError("link supplier product to resource", notFound(err, fmt.Errorf("%w: id %d", domain.ErrSupplierProductNotFound, supplierProductID)))
	}
	if err := cascadeLinkStatus(ctx, r.pool, supplierProductID, domain.LinkPending, domain.LinkLinked); err != nil {
		return domain.SupplierProduct{}, err
	}
	return product, nil
}

func (r *repository) UnlinkSupplierProduct(ctx context.Context, supplierProductID int64) (domain.SupplierProduct, error) {
	if err := r.ready(); err != nil {
		return domain.SupplierProduct{}, err
	}
	const updateSQL = `UPDATE public.supplier_products SET resource_id = NULL WHERE id = $1 RETURNING ` + supplierProductColumns
	product, err := scanSupplierProduct(r.pool.QueryRow(ctx, updateSQL, supplierProductID))
	if err != nil {
		return domain.SupplierProduct{}, wrapRead("unlink supplier product", notFound(err, fmt.Errorf("%w: id %d", domain.ErrSupplierProductNotFound, supplierProductID)))
	}
	if err := cascadeLinkStatus(ctx, r.pool, supplierProductID, domain.LinkLinked, domain.LinkPending); err != nil {
		return domain.SupplierProduct{}, err
	}
	return product, nil
}

// cascadeLinkStatus updates every purchase line referencing
// supplierProductID currently in from status to to, leaving manually set
// NO_APLICA/CONFLICTO lines untouched.
func cascadeLinkStatus(ctx context.Context, pool *pgxpool.Pool, supplierProductID int64, from, to domain.LinkStatus) error {
	const updateSQL = `UPDATE public.purchase_lines SET link_status = $3 WHERE supplier_product_id = $1 AND link_status = $2`
	if _, err := pool.Exec(ctx, updateSQL, supplierProductID, string(from), string(to)); err != nil {
		return wrapRead("cascade purchase line link status", err)
	}
	return nil
}

func scanSupplierProduct(row scanner) (domain.SupplierProduct, error) {
	var product domain.SupplierProduct
	err := row.Scan(&product.ID, &product.SupplierID, &product.SupplierSKU, &product.Description, &product.ResourceID, &product.Notes, &product.CreatedAt, &product.UpdatedAt)
	return product, err
}

// upsertSupplierProduct returns the id and current resource_id of the
// supplier product identified by (supplierID, sku) inside tx, creating it
// with description when it does not yet exist. It never overwrites an
// existing description or resource relation.
func upsertSupplierProduct(ctx context.Context, tx pgx.Tx, supplierID int64, sku, description string) (int64, *int64, error) {
	const upsertSQL = `INSERT INTO public.supplier_products (supplier_id, supplier_sku, description)
		VALUES ($1, $2, $3)
		ON CONFLICT (supplier_id, supplier_sku) DO UPDATE SET supplier_id = EXCLUDED.supplier_id
		RETURNING id, resource_id`
	var id int64
	var resourceID *int64
	if err := tx.QueryRow(ctx, upsertSQL, supplierID, sku, description).Scan(&id, &resourceID); err != nil {
		return 0, nil, mapWriteError("upsert supplier product", err)
	}
	return id, resourceID, nil
}
