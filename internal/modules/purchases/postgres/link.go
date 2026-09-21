package postgres

import (
	"context"
	"fmt"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/modules/purchases/domain"
)

func (r *repository) MarkNotApplicable(ctx context.Context, lineID int64) (domain.PurchaseLine, error) {
	return r.updatePurchaseLineOverride(ctx, lineID, domain.LinkNotApplicable)
}

func (r *repository) MarkConflict(ctx context.Context, lineID int64) (domain.PurchaseLine, error) {
	return r.updatePurchaseLineOverride(ctx, lineID, domain.LinkConflict)
}

func (r *repository) ClearOverride(ctx context.Context, lineID int64) (domain.PurchaseLine, error) {
	return r.updatePurchaseLineOverride(ctx, lineID, domain.LinkStatusNone)
}

func (r *repository) updatePurchaseLineOverride(ctx context.Context, lineID int64, override domain.LinkStatus) (domain.PurchaseLine, error) {
	if err := r.ready(); err != nil {
		return domain.PurchaseLine{}, err
	}
	const updateSQL = `UPDATE public.purchase_lines
		SET resolution_override = $2 WHERE id = $1
		RETURNING ` + purchaseLineReturningColumns
	line, err := scanPurchaseLine(r.pool.QueryRow(ctx, updateSQL, lineID, string(override)))
	if err != nil {
		return domain.PurchaseLine{}, mapWriteError("update purchase line resolution override", notFound(err, fmt.Errorf("%w: id %d", domain.ErrPurchaseLineNotFound, lineID)))
	}
	return line, nil
}
