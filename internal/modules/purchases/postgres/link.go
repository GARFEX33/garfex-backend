package postgres

import (
	"context"
	"fmt"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/modules/purchases/domain"
)

func (r *repository) SetPurchaseLineLinkStatus(ctx context.Context, lineID int64, status domain.LinkStatus) (domain.PurchaseLine, error) {
	if err := r.ready(); err != nil {
		return domain.PurchaseLine{}, err
	}
	const updateSQL = `UPDATE public.purchase_lines SET link_status = $2 WHERE id = $1 RETURNING ` + purchaseLineColumns
	line, err := scanPurchaseLine(r.pool.QueryRow(ctx, updateSQL, lineID, string(status)))
	if err != nil {
		return domain.PurchaseLine{}, mapWriteError("set purchase line link status", notFound(err, fmt.Errorf("%w: id %d", domain.ErrPurchaseLineNotFound, lineID)))
	}
	return line, nil
}
