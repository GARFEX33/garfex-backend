package postgres

import (
	"context"
	"fmt"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/modules/purchases/domain"
	"github.com/jackc/pgx/v5"
)

// Import persists draft as a single all-or-nothing transaction: the
// purchase row, every line, and the supplier-product identity/link each
// line resolves to. If draft.CFDIUUID is already registered, the insert is
// skipped by the database's own uniqueness guarantee (safe under
// concurrent imports of the same document), and the existing purchase is
// returned unchanged: with AlreadyExisted true when its content matches
// draft, or domain.ErrPurchaseConflict when it does not.
func (r *repository) Import(ctx context.Context, draft domain.PurchaseDraft) (domain.ImportResult, error) {
	if err := r.ready(); err != nil {
		return domain.ImportResult{}, err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.ImportResult{}, wrapRead("begin import transaction", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	purchase, created, err := insertPurchase(ctx, tx, draft)
	if err != nil {
		return domain.ImportResult{}, err
	}

	if !created {
		result, err := resolveDuplicateImport(ctx, tx, draft)
		if err != nil {
			return domain.ImportResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return domain.ImportResult{}, wrapRead("commit import (duplicate)", err)
		}
		return result, nil
	}

	lines := make([]domain.PurchaseLine, 0, len(draft.Lines))
	for _, lineDraft := range draft.Lines {
		var supplierProductID *int64
		if lineDraft.HasSupplierIdentity() {
			product, err := upsertSupplierProduct(ctx, tx, draft.Supplier.SupplierID, lineDraft.SupplierSKU, lineDraft.Description)
			if err != nil {
				return domain.ImportResult{}, err
			}
			supplierProductID = &product.ID
		}
		line, err := insertPurchaseLine(ctx, tx, purchase.ID, lineDraft, supplierProductID)
		if err != nil {
			return domain.ImportResult{}, err
		}
		lines = append(lines, line)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.ImportResult{}, wrapRead("commit import", err)
	}
	return domain.ImportResult{Purchase: purchase, Lines: lines, AlreadyExisted: false}, nil
}

func resolveDuplicateImport(ctx context.Context, tx pgx.Tx, draft domain.PurchaseDraft) (domain.ImportResult, error) {
	existing, err := scanPurchase(tx.QueryRow(ctx, getPurchaseByUUIDSQL, draft.CFDIUUID))
	if err != nil {
		return domain.ImportResult{}, wrapRead("load existing purchase", err)
	}
	existingLines, err := queryPurchaseLines(ctx, tx, listPurchaseLinesSQL, existing.ID)
	if err != nil {
		return domain.ImportResult{}, err
	}
	if existing.SameRelevantContent(draft) && domain.SameLines(existingLines, draft.Lines) {
		return domain.ImportResult{Purchase: existing, Lines: existingLines, AlreadyExisted: true}, nil
	}
	return domain.ImportResult{}, fmt.Errorf("import purchase %s: %w", draft.CFDIUUID, domain.ErrPurchaseConflict)
}
