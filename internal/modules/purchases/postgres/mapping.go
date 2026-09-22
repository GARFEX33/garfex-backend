package postgres

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/GARFEX33/garfex-backend/internal/modules/purchases/domain"
	"github.com/jackc/pgx/v5"
)

const lockSupplierProductSQL = `SELECT ` + supplierProductColumns + `
	FROM public.supplier_products sp LEFT JOIN public.recursos r ON r.id = sp.resource_id
	WHERE sp.id = $1 FOR UPDATE OF sp`

const updateSupplierProductMappingSQL = `UPDATE public.supplier_products
	SET resource_id = $2, mapping_revision = $3, mapping_identity_conflict = $4
	WHERE id = $1 AND mapping_revision = $5`

const insertMappingAuditSQL = `INSERT INTO public.supplier_product_mapping_audit
	(supplier_product_id, previous_resource_id, new_resource_id,
	 previous_identity_conflict, new_identity_conflict, previous_revision, new_revision,
	 operation, actor, origin, reason, decided_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`

const mappingAuditSQL = `SELECT supplier_product_id, previous_resource_id, new_resource_id,
	previous_identity_conflict, new_identity_conflict, previous_revision, new_revision,
	operation, actor, origin, reason, decided_at
	FROM public.supplier_product_mapping_audit
	WHERE supplier_product_id = $1 ORDER BY id LIMIT $2 OFFSET $3`

func mapCommitError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("mapping transaction commit: %w: %v", domain.ErrCommitAmbiguous, err)
}

func mapResourceStateError(active bool) error {
	if !active {
		return domain.ErrResourceInactive
	}
	return nil
}

func confirmRequiresTargetLock(transition domain.MappingTransition) bool {
	return transition.Changed
}

func mappingTransitionRefreshesResourceActivity(transition domain.MappingTransition) bool {
	return transition.Changed
}

func (r *repository) ConfirmMapping(ctx context.Context, command domain.ConfirmMappingCommand) (domain.SupplierProduct, error) {
	return r.runMappingTransition(ctx, command.SupplierProductID, command.ExpectedRevision, func(tx pgx.Tx, product *domain.SupplierProduct) (domain.MappingTransition, int64, error) {
		transition, err := product.ConfirmMapping(command.ResourceID, command.ExpectedRevision, command.Decision)
		if err != nil || !confirmRequiresTargetLock(transition) {
			return transition, 0, err
		}
		active, err := lockResources(ctx, tx, command.ResourceID)
		if err != nil {
			return domain.MappingTransition{}, 0, err
		}
		if err := requireTargetActive(active, command.ResourceID); err != nil {
			return domain.MappingTransition{}, 0, err
		}
		return transition, command.ResourceID, nil
	})
}

func nullableResourceID(id *int64) any {
	if id == nil {
		return nil
	}
	return *id
}

func (r *repository) CorrectMapping(ctx context.Context, command domain.CorrectMappingCommand) (domain.SupplierProduct, error) {
	if err := r.ready(); err != nil {
		return domain.SupplierProduct{}, err
	}
	return r.runMappingTransition(ctx, command.SupplierProductID, command.ExpectedRevision, func(tx pgx.Tx, product *domain.SupplierProduct) (domain.MappingTransition, int64, error) {
		active, err := lockResources(ctx, tx, command.ExpectedCurrentResource, command.ResourceID)
		if err != nil {
			return domain.MappingTransition{}, 0, err
		}
		if err := requireTargetActive(active, command.ResourceID); err != nil {
			return domain.MappingTransition{}, 0, err
		}
		transition, err := product.CorrectMapping(command.ExpectedCurrentResource, command.ResourceID, command.ExpectedRevision, command.Decision)
		return transition, command.ResourceID, err
	})
}

func (r *repository) ExceptionalUnlink(ctx context.Context, command domain.ExceptionalUnlinkCommand) (domain.SupplierProduct, error) {
	return r.runMappingTransition(ctx, command.SupplierProductID, command.ExpectedRevision, func(tx pgx.Tx, product *domain.SupplierProduct) (domain.MappingTransition, int64, error) {
		if _, err := lockResources(ctx, tx, command.ExpectedCurrentResource); err != nil {
			return domain.MappingTransition{}, 0, err
		}
		transition, err := product.ExceptionalUnlink(command.ExpectedCurrentResource, command.ExpectedRevision, command.Decision)
		return transition, 0, err
	})
}

func (r *repository) ReportIdentityConflict(ctx context.Context, command domain.ReportIdentityConflictCommand) (domain.SupplierProduct, error) {
	return r.runMappingTransition(ctx, command.SupplierProductID, command.ExpectedRevision, func(tx pgx.Tx, product *domain.SupplierProduct) (domain.MappingTransition, int64, error) {
		if _, err := lockResources(ctx, tx, command.ExpectedCurrentResource); err != nil {
			return domain.MappingTransition{}, 0, err
		}
		transition, err := product.ReportIdentityConflict(command.ExpectedCurrentResource, command.ExpectedRevision, command.Decision)
		return transition, command.ExpectedCurrentResource, err
	})
}

func (r *repository) ResolveIdentityConflict(ctx context.Context, command domain.ResolveIdentityConflictCommand) (domain.SupplierProduct, error) {
	return r.runMappingTransition(ctx, command.SupplierProductID, command.ExpectedRevision, func(tx pgx.Tx, product *domain.SupplierProduct) (domain.MappingTransition, int64, error) {
		active, err := lockResources(ctx, tx, command.ExpectedCurrentResource, command.ResourceID)
		if err != nil {
			return domain.MappingTransition{}, 0, err
		}
		if err := requireTargetActive(active, command.ResourceID); err != nil {
			return domain.MappingTransition{}, 0, err
		}
		transition, err := product.ResolveIdentityConflict(command.ExpectedCurrentResource, command.ResourceID, command.ExpectedRevision, command.Decision)
		return transition, command.ResourceID, err
	})
}

func (r *repository) runMappingTransition(ctx context.Context, supplierProductID int64, expected domain.MappingRevision, apply func(pgx.Tx, *domain.SupplierProduct) (domain.MappingTransition, int64, error)) (domain.SupplierProduct, error) {
	if err := r.ready(); err != nil {
		return domain.SupplierProduct{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.SupplierProduct{}, wrapRead("begin mapping transaction", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	product, err := scanSupplierProduct(tx.QueryRow(ctx, lockSupplierProductSQL, supplierProductID))
	if err != nil {
		return domain.SupplierProduct{}, mapWriteError("lock supplier product", notFound(err, fmt.Errorf("%w: id %d", domain.ErrSupplierProductNotFound, supplierProductID)))
	}
	transition, resourceID, err := apply(tx, &product)
	if err != nil {
		return domain.SupplierProduct{}, err
	}
	if mappingTransitionRefreshesResourceActivity(transition) {
		if resourceID > 0 {
			active, err := readResourceActive(ctx, tx, resourceID)
			if err != nil {
				return domain.SupplierProduct{}, err
			}
			product.ResourceActive = &active
		} else {
			product.ResourceActive = nil
		}
	}
	if err := persistMappingTransition(ctx, tx, product, transition, expected); err != nil {
		return domain.SupplierProduct{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.SupplierProduct{}, mapCommitError(err)
	}
	return product, nil
}

func persistMappingTransition(ctx context.Context, tx pgx.Tx, product domain.SupplierProduct, transition domain.MappingTransition, expected domain.MappingRevision) error {
	if !transition.Changed {
		return nil
	}
	tag, err := tx.Exec(ctx, updateSupplierProductMappingSQL, product.ID, nullableResourceID(product.CurrentMapping.ResourceID), product.MappingRevision, product.CurrentMapping.IdentityConflict, expected)
	if err != nil {
		return mapWriteError("update supplier product mapping", err)
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrStaleMappingRevision
	}
	entry := transition.Audit
	if entry == nil {
		return errors.New("mapping transition changed without audit entry")
	}
	if _, err := tx.Exec(ctx, insertMappingAuditSQL,
		entry.SupplierProductID, nullableResourceID(entry.PreviousMapping.ResourceID), nullableResourceID(entry.NewMapping.ResourceID),
		entry.PreviousMapping.IdentityConflict, entry.NewMapping.IdentityConflict,
		entry.PreviousRevision, entry.NewRevision, string(entry.Operation), entry.Decision.Actor,
		string(entry.Decision.Origin), entry.Decision.Reason, entry.Decision.At); err != nil {
		return mapWriteError("insert supplier product mapping audit", err)
	}
	return nil
}

func lockResource(ctx context.Context, tx pgx.Tx, resourceID int64) (int64, bool, error) {
	if resourceID <= 0 {
		return 0, false, domain.ErrResourceNotFound
	}
	var id int64
	var active bool
	if err := tx.QueryRow(ctx, `SELECT id, active FROM public.recursos WHERE id = $1 FOR UPDATE`, resourceID).Scan(&id, &active); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, domain.ErrResourceNotFound
		}
		return 0, false, wrapRead("lock resource", err)
	}
	return id, active, nil
}

func orderedResourceIDs(ids ...int64) []int64 {
	unique := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id > 0 {
			unique[id] = struct{}{}
		}
	}
	ordered := make([]int64, 0, len(unique))
	for id := range unique {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	return ordered
}

func lockResources(ctx context.Context, tx pgx.Tx, resourceIDs ...int64) (map[int64]bool, error) {
	for _, resourceID := range resourceIDs {
		if resourceID <= 0 {
			return nil, domain.ErrResourceNotFound
		}
	}
	active := make(map[int64]bool, len(resourceIDs))
	for _, resourceID := range orderedResourceIDs(resourceIDs...) {
		_, isActive, err := lockResource(ctx, tx, resourceID)
		if err != nil {
			return nil, err
		}
		active[resourceID] = isActive
	}
	return active, nil
}

func requireTargetActive(active map[int64]bool, targetResourceID int64) error {
	if !active[targetResourceID] {
		return domain.ErrResourceInactive
	}
	return nil
}

func readResourceActive(ctx context.Context, tx pgx.Tx, resourceID int64) (bool, error) {
	var active bool
	if err := tx.QueryRow(ctx, `SELECT active FROM public.recursos WHERE id = $1`, resourceID).Scan(&active); err != nil {
		return false, wrapRead("read resource activity", err)
	}
	return active, nil
}

func (r *repository) ListMappingAudit(ctx context.Context, supplierProductID int64, criteria domain.ListCriteria) ([]domain.MappingAuditEntry, error) {
	if err := r.ready(); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, mappingAuditSQL, supplierProductID, limit(criteria.Limit), offset(criteria.Offset))
	if err != nil {
		return nil, wrapRead("list supplier product mapping audit", err)
	}
	defer rows.Close()
	entries := make([]domain.MappingAuditEntry, 0)
	for rows.Next() {
		entry, err := scanMappingAuditEntry(rows)
		if err != nil {
			return nil, wrapRead("scan supplier product mapping audit", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapRead("read supplier product mapping audit", err)
	}
	return entries, nil
}

func scanMappingAuditEntry(row scanner) (domain.MappingAuditEntry, error) {
	var entry domain.MappingAuditEntry
	var previousResource, newResource *int64
	var operation, origin string
	if err := row.Scan(&entry.SupplierProductID, &previousResource, &newResource,
		&entry.PreviousMapping.IdentityConflict, &entry.NewMapping.IdentityConflict,
		&entry.PreviousRevision, &entry.NewRevision, &operation, &entry.Decision.Actor,
		&origin, &entry.Decision.Reason, &entry.Decision.At); err != nil {
		return domain.MappingAuditEntry{}, err
	}
	entry.PreviousMapping.ResourceID = previousResource
	entry.NewMapping.ResourceID = newResource
	entry.Operation = domain.MappingOperation(operation)
	entry.Decision.Origin = domain.MappingOrigin(origin)
	entry.PreviousState = entry.PreviousMapping.KnowledgeState()
	entry.NewState = entry.NewMapping.KnowledgeState()
	return entry, nil
}
