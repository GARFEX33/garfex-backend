package postgres

import (
	"context"
	"errors"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/modules/purchases/domain"
	"github.com/jackc/pgx/v5"
)

const insertResolutionAuditSQL = `INSERT INTO public.purchase_line_resolution_audit
	(purchase_line_id, previous_override, new_override, previous_revision, new_revision,
	 actor, origin, reason, decided_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

func (r *repository) SetResolutionOverride(ctx context.Context, command domain.SetResolutionOverrideCommand) (domain.PurchaseLine, error) {
	if err := r.ready(); err != nil {
		return domain.PurchaseLine{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.PurchaseLine{}, wrapRead("begin purchase line override transaction", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	line, err := scanPurchaseLine(tx.QueryRow(ctx, getPurchaseLineForResolveSQL+` FOR UPDATE OF pl`, command.LineID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PurchaseLine{}, domain.ErrPurchaseLineNotFound
		}
		return domain.PurchaseLine{}, wrapRead("lock purchase line override", err)
	}
	transition, err := line.ChangeResolutionOverride(command.Override, command.ExpectedRevision, command.Decision)
	if err != nil {
		return domain.PurchaseLine{}, err
	}
	if transition.Changed {
		tag, err := tx.Exec(ctx, `UPDATE public.purchase_lines
			SET resolution_override = $2, resolution_revision = $3
			WHERE id = $1 AND resolution_revision = $4`,
			line.ID, string(line.ResolutionOverride), line.ResolutionRevision, command.ExpectedRevision)
		if err != nil {
			return domain.PurchaseLine{}, mapWriteError("update purchase line override", err)
		}
		if tag.RowsAffected() != 1 {
			return domain.PurchaseLine{}, domain.ErrStaleResolutionRevision
		}
		entry := transition.Audit
		if entry == nil {
			return domain.PurchaseLine{}, errors.New("resolution override changed without audit entry")
		}
		if _, err := tx.Exec(ctx, insertResolutionAuditSQL,
			entry.PurchaseLineID, string(entry.PreviousOverride), string(entry.NewOverride),
			entry.PreviousRevision, entry.NewRevision, entry.Decision.Actor,
			string(entry.Decision.Origin), entry.Decision.Reason, entry.Decision.At); err != nil {
			return domain.PurchaseLine{}, mapWriteError("insert purchase line resolution audit", err)
		}
		line, err = scanPurchaseLine(tx.QueryRow(ctx, getPurchaseLineForResolveSQL, command.LineID))
		if err != nil {
			return domain.PurchaseLine{}, wrapRead("read updated purchase line override", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.PurchaseLine{}, mapCommitError(err)
	}
	return line, nil
}
