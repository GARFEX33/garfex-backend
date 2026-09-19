package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/modules/purchases/domain"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

const purchaseColumns = `id, supplier_id, branch_id, cfdi_uuid, series, folio, issued_at, currency,
	exchange_rate, subtotal, discount, tax_transferred, tax_withheld, total,
	issuer_tax_id, issuer_name, xml_content, xml_hash, xml_filename, imported_at, created_at, updated_at`

const getPurchaseSQL = `SELECT ` + purchaseColumns + ` FROM public.purchases WHERE id = $1`

const getPurchaseByUUIDSQL = `SELECT ` + purchaseColumns + ` FROM public.purchases WHERE cfdi_uuid = $1`

const listPurchasesBySupplierSQL = `SELECT ` + purchaseColumns + `
	FROM public.purchases WHERE supplier_id = $1
	ORDER BY issued_at DESC, id DESC
	LIMIT $2 OFFSET $3`

func (r *repository) GetPurchase(ctx context.Context, id int64) (domain.Purchase, error) {
	if err := r.ready(); err != nil {
		return domain.Purchase{}, err
	}
	purchase, err := scanPurchase(r.pool.QueryRow(ctx, getPurchaseSQL, id))
	if err != nil {
		return domain.Purchase{}, wrapRead("get purchase", notFound(err, fmt.Errorf("%w: id %d", domain.ErrPurchaseNotFound, id)))
	}
	return purchase, nil
}

func (r *repository) GetPurchaseByUUID(ctx context.Context, uuid string) (domain.Purchase, error) {
	if err := r.ready(); err != nil {
		return domain.Purchase{}, err
	}
	purchase, err := scanPurchase(r.pool.QueryRow(ctx, getPurchaseByUUIDSQL, normalizeUUID(uuid)))
	if err != nil {
		return domain.Purchase{}, wrapRead("get purchase by uuid", notFound(err, domain.ErrPurchaseNotFound))
	}
	return purchase, nil
}

func (r *repository) ListPurchasesBySupplier(ctx context.Context, supplierID int64, criteria domain.ListCriteria) ([]domain.Purchase, error) {
	if err := r.ready(); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, listPurchasesBySupplierSQL, supplierID, limit(criteria.Limit), offset(criteria.Offset))
	if err != nil {
		return nil, wrapRead("list purchases by supplier", err)
	}
	defer rows.Close()
	purchases := make([]domain.Purchase, 0)
	for rows.Next() {
		purchase, err := scanPurchase(rows)
		if err != nil {
			return nil, wrapRead("scan purchase", err)
		}
		purchases = append(purchases, purchase)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapRead("read purchases", err)
	}
	return purchases, nil
}

func scanPurchase(row scanner) (domain.Purchase, error) {
	var p domain.Purchase
	var exchangeRate *string
	var subtotal, discount, taxTransferred, taxWithheld, total string
	err := row.Scan(
		&p.ID, &p.Supplier.SupplierID, &p.Supplier.BranchID, &p.CFDIUUID, &p.Series, &p.Folio, &p.IssuedAt, &p.Currency,
		&exchangeRate, &subtotal, &discount, &taxTransferred, &taxWithheld, &total,
		&p.IssuerTaxID, &p.IssuerName, &p.XML.Content, &p.XML.Hash, &p.XML.Filename, &p.ImportedAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return domain.Purchase{}, err
	}
	if p.Subtotal, err = decimal.NewFromString(subtotal); err != nil {
		return domain.Purchase{}, fmt.Errorf("decode subtotal: %w", err)
	}
	if p.Discount, err = decimal.NewFromString(discount); err != nil {
		return domain.Purchase{}, fmt.Errorf("decode discount: %w", err)
	}
	if p.TaxTransferred, err = decimal.NewFromString(taxTransferred); err != nil {
		return domain.Purchase{}, fmt.Errorf("decode tax transferred: %w", err)
	}
	if p.TaxWithheld, err = decimal.NewFromString(taxWithheld); err != nil {
		return domain.Purchase{}, fmt.Errorf("decode tax withheld: %w", err)
	}
	if p.Total, err = decimal.NewFromString(total); err != nil {
		return domain.Purchase{}, fmt.Errorf("decode total: %w", err)
	}
	if exchangeRate != nil {
		rate, err := decimal.NewFromString(*exchangeRate)
		if err != nil {
			return domain.Purchase{}, fmt.Errorf("decode exchange rate: %w", err)
		}
		p.ExchangeRate = &rate
	}
	return p, nil
}

// insertPurchase attempts to persist one purchase inside tx, silently doing
// nothing when its CFDI UUID is already registered. ok is false when the
// insert was skipped due to that conflict.
func insertPurchase(ctx context.Context, tx pgx.Tx, draft domain.PurchaseDraft) (domain.Purchase, bool, error) {
	const insertSQL = `INSERT INTO public.purchases
		(supplier_id, branch_id, cfdi_uuid, series, folio, issued_at, currency, exchange_rate,
		 subtotal, discount, tax_transferred, tax_withheld, total, issuer_tax_id, issuer_name,
		 xml_content, xml_hash, xml_filename)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		ON CONFLICT (cfdi_uuid) DO NOTHING
		RETURNING ` + purchaseColumns

	created, err := scanPurchase(tx.QueryRow(ctx, insertSQL,
		draft.Supplier.SupplierID, draft.Supplier.BranchID, draft.CFDIUUID, draft.Series, draft.Folio, draft.IssuedAt, draft.Currency, draft.ExchangeRate,
		draft.Subtotal, draft.Discount, draft.TaxTransferred, draft.TaxWithheld, draft.Total, draft.IssuerTaxID, draft.IssuerName,
		draft.XML.Content, draft.XML.Hash, draft.XML.Filename,
	))
	if err != nil {
		if isNoRows(err) {
			return domain.Purchase{}, false, nil
		}
		return domain.Purchase{}, false, mapWriteError("insert purchase", err)
	}
	return created, true, nil
}

func normalizeUUID(uuid string) string { return strings.ToUpper(strings.TrimSpace(uuid)) }
