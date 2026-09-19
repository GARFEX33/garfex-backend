package postgres

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type attributeOrderRepository struct{ pool *pgxpool.Pool }

var (
	_ domain.AttributeOrderReader = (*attributeOrderRepository)(nil)
	_ domain.AttributeOrderWriter = (*attributeOrderRepository)(nil)
)

// NewAttributeOrderRepository exposes only the coherent read capability.
func NewAttributeOrderRepository(pool *pgxpool.Pool) domain.AttributeOrderReader {
	return &attributeOrderRepository{pool: pool}
}

// NewAttributeOrderRepositoryFull exposes both the coherent read capability
// and the locked CAS write capability from one value — the same concrete
// type NewAttributeOrderRepository already returns, so no behavior of the
// reader-only construction path changes.
func NewAttributeOrderRepositoryFull(pool *pgxpool.Pool) domain.AttributeOrderStore {
	return &attributeOrderRepository{pool: pool}
}

func (r *attributeOrderRepository) ReadAttributeOrder(ctx context.Context, scope domain.ResourceScope) (domain.AttributeOrderReadResult, error) {
	scope, err := domain.CanonicalAttributeOrderScope(scope)
	if err != nil {
		return domain.AttributeOrderReadResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return domain.AttributeOrderReadResult{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return domain.AttributeOrderReadResult{}, fmt.Errorf("begin attribute order read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	result, err := readAttributeOrderTx(ctx, tx, scope)
	if err != nil {
		return domain.AttributeOrderReadResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.AttributeOrderReadResult{}, fmt.Errorf("commit attribute order read: %w", err)
	}
	return result, nil
}

// attributeOrderLockTypeSQL locks only the target resource_types row ("FOR
// UPDATE OF t"), serializing concurrent reorders (and any other structural
// write) on this exact Tipo, without escalating to its joined family/class
// rows (those are covered separately by attributeOrderLockStructuralSQL).
const attributeOrderLockTypeSQL = `SELECT t.id
  FROM public.resource_types t JOIN public.resource_families f ON f.id=t.family_id
  JOIN public.resource_classes c ON c.id=f.class_id
  WHERE c.code=$1 AND f.code=$2 AND t.code=$3
  FOR UPDATE OF t`

// attributeOrderLockStructuralSQL is a short, transaction-scoped SHARE lock
// on every table that gates EffectiveAttributesFor's *membership* for a
// scope (see resource_effective_attributes.go/resource_catalog_query.go):
// resource_classes and resource_families (their codes feed the ClassCode/
// FamilyCode strings ResourceScope.matches compares) and resource_attributes
// itself (the row set AttributesFor filters), plus attribute_definitions
// (its code feeds AttributeOrderKey.CharacteristicCode). SHARE mode blocks a
// concurrent INSERT/UPDATE/DELETE (which needs ROW EXCLUSIVE) for the
// remainder of this transaction while still allowing concurrent readers and
// concurrent reorders of other Tipos (SHARE is compatible with SHARE).
// resource_types is deliberately excluded here: the one row that matters is
// already exclusively locked by attributeOrderLockTypeSQL above, and other
// Tipo rows never affect this scope's membership. resource_attribute_rules
// is deliberately excluded too: EffectiveAttributesFor's Rules evaluation
// only ever changes EffectiveMode/NotApplicable, never which attributes are
// present in the result — AttributeOrderKey carries neither field, so rules
// cannot change membership and locking that table would only serialize
// unrelated CONDITIONAL-rule edits for no coherence benefit.
const attributeOrderLockStructuralSQL = `LOCK TABLE public.resource_classes, public.resource_families, public.attribute_definitions, public.resource_attributes IN SHARE MODE`

const attributeOrderDeleteItemsSQL = `DELETE FROM public.resource_type_attribute_order_items WHERE target_type_id=$1`

// attributeOrderUpsertHeadSQL always bumps the head revision for an accepted
// write, including a first-ever write (default 1, i.e. 0->1) and a no-op
// reorder (existing row: revision+1) — see domain.AttributeOrderWriter's
// doc comment: "the same token has exactly one winner".
const attributeOrderUpsertHeadSQL = `INSERT INTO public.resource_type_attribute_orders (type_id) VALUES ($1)
  ON CONFLICT (type_id) DO UPDATE SET revision = public.resource_type_attribute_orders.revision + 1, updated_at = NOW()`

const attributeOrderInsertItemSQL = `INSERT INTO public.resource_type_attribute_order_items (target_type_id, resource_attribute_id, position) VALUES ($1,$2,$3)`

func (r *attributeOrderRepository) WriteAttributeOrder(ctx context.Context, req domain.AttributeOrderWriteRequest) (domain.AttributeOrderReadResult, error) {
	scope, err := domain.CanonicalAttributeOrderScope(req.Scope)
	if err != nil {
		return domain.AttributeOrderReadResult{}, err
	}
	if strings.TrimSpace(req.ExpectedOrderRevision) == "" {
		return domain.AttributeOrderReadResult{}, fmt.Errorf("%w: attribute order write requires an expected revision", domain.ErrResourceValidation)
	}
	if err := ctx.Err(); err != nil {
		return domain.AttributeOrderReadResult{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.AttributeOrderReadResult{}, fmt.Errorf("begin attribute order write: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	result, err := writeAttributeOrderTx(ctx, tx, scope, req)
	if err != nil {
		return domain.AttributeOrderReadResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.AttributeOrderReadResult{}, classifyAttributeOrderCommitFailure(err)
	}
	return result, nil
}

// classifyAttributeOrderCommitFailure wraps a failing Commit call as
// domain.ErrAttributeOrderUnavailable: whether the write actually applied
// cannot be determined from the error alone, so it is never classified as a
// conflict, a validation failure, or silently treated as success (design
// "Commit uncertainty returns unavailable and requires fresh read, never
// blind replay").
func classifyAttributeOrderCommitFailure(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: commit attribute order write: %v", domain.ErrAttributeOrderUnavailable, err)
}

// writeAttributeOrderTx performs the full locked CAS protocol inside an
// already-open read-write tx: lock the target type row, take short SHARE
// locks on the structural tables that gate membership, take a fresh
// lock-coherent read (reusing readAttributeOrderStateTx), plan the write
// (planAttributeOrderWrite), atomically replace the persisted membership and
// always bump the head revision, then reread and verify before returning —
// the caller is responsible for Commit/Rollback.
func writeAttributeOrderTx(ctx context.Context, tx pgx.Tx, scope domain.ResourceScope, req domain.AttributeOrderWriteRequest) (domain.AttributeOrderReadResult, error) {
	var typeID int64
	err := tx.QueryRow(ctx, attributeOrderLockTypeSQL, scope.ClassCode, scope.FamilyCode, scope.TypeCode).Scan(&typeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AttributeOrderReadResult{}, domain.ErrCatalogRecordNotFound
	}
	if err != nil {
		return domain.AttributeOrderReadResult{}, fmt.Errorf("lock attribute order target type: %w", err)
	}

	if _, err := tx.Exec(ctx, attributeOrderLockStructuralSQL); err != nil {
		return domain.AttributeOrderReadResult{}, fmt.Errorf("lock attribute order structural catalog: %w", err)
	}

	catalog, state, err := readAttributeOrderStateTx(ctx, tx, scope)
	if err != nil {
		return domain.AttributeOrderReadResult{}, err
	}

	orderedKeys, orderedIDs, err := planAttributeOrderWrite(catalog, scope, state, req)
	if err != nil {
		return domain.AttributeOrderReadResult{}, err
	}

	if _, err := tx.Exec(ctx, attributeOrderDeleteItemsSQL, typeID); err != nil {
		return domain.AttributeOrderReadResult{}, fmt.Errorf("clear attribute order items: %w", err)
	}
	if _, err := tx.Exec(ctx, attributeOrderUpsertHeadSQL, typeID); err != nil {
		return domain.AttributeOrderReadResult{}, fmt.Errorf("bump attribute order head revision: %w", err)
	}
	for position, id := range orderedIDs {
		if _, err := tx.Exec(ctx, attributeOrderInsertItemSQL, typeID, id, position); err != nil {
			return domain.AttributeOrderReadResult{}, fmt.Errorf("insert attribute order item: %w", err)
		}
	}

	result, err := readAttributeOrderTx(ctx, tx, scope)
	if err != nil {
		return domain.AttributeOrderReadResult{}, err
	}
	if err := verifyAttributeOrderWrite(result.Order.OrderedAttributes, orderedKeys); err != nil {
		return domain.AttributeOrderReadResult{}, err
	}
	return result, nil
}

// planAttributeOrderWrite performs no I/O: given the just-locked, freshly
// read catalog/state and the caller's request, it resolves the lock-coherent
// baseline (baselineAttributeOrderMembers), computes the current
// OrderRevision (domain.ResolveAttributeOrder) and rejects a stale
// ExpectedOrderRevision (domain.ErrAttributeOrderRevisionConflict), validates
// req.OrderedAttributes is an exact permutation
// (AttributeOrderSnapshot.ValidatePermutation, already covering wrong
// length/unknown/repeated keys), and resolves the validated keys back to
// their persisted ResourceAttributeIDs in accepted order — including the
// no-op case, where OrderedAttributes already equals the current order.
func planAttributeOrderWrite(catalog domain.ResourceCatalog, scope domain.ResourceScope, state domain.AttributeOrderState, req domain.AttributeOrderWriteRequest) (orderedKeys []domain.AttributeOrderKey, orderedIDs []int64, err error) {
	baseline, err := baselineAttributeOrderMembers(catalog, scope, state.Members)
	if err != nil {
		return nil, nil, err
	}
	canonicalState := state
	canonicalState.Members = baseline
	current, err := domain.ResolveAttributeOrder(scope, canonicalState)
	if err != nil {
		return nil, nil, err
	}
	if req.ExpectedOrderRevision != current.OrderRevision {
		return nil, nil, domain.ErrAttributeOrderRevisionConflict
	}
	orderedKeys, err = current.ValidatePermutation(req.OrderedAttributes)
	if err != nil {
		return nil, nil, err
	}
	// baseline's keys are raw, as persisted (characteristic codes are stored
	// as given; only comparisons canonicalize them — see
	// resource_canonical.go's canonicalAttribute callers), but orderedKeys
	// above was already canonicalized by ValidatePermutation. A mixed-case
	// persisted code (e.g. "NUMH") would otherwise never match its
	// canonicalized form ("numh") here, even for a client resending the
	// exact order a prior read returned.
	byKey := make(map[domain.AttributeOrderKey]int64, len(baseline))
	for _, member := range baseline {
		canonicalKey, err := member.Key.CanonicalFor(scope)
		if err != nil {
			return nil, nil, err
		}
		byKey[canonicalKey] = member.ResourceAttributeID
	}
	orderedIDs = make([]int64, len(orderedKeys))
	for i, key := range orderedKeys {
		id, ok := byKey[key]
		if !ok {
			// ValidatePermutation already guarantees every key is one of
			// current.OrderedAttributes, which baselineAttributeOrderMembers
			// derives 1:1 from baseline, so this is unreachable; guarded
			// defensively rather than indexing byKey unchecked.
			return nil, nil, fmt.Errorf("%w: unresolved attribute order key", domain.ErrResourceValidation)
		}
		orderedIDs[i] = id
	}
	return orderedKeys, orderedIDs, nil
}

// verifyAttributeOrderWrite performs no I/O: it re-asserts that a post-write
// reread observed exactly the accepted key order, the last defense before a
// caller ever sees a written result.
func verifyAttributeOrderWrite(got, want []domain.AttributeOrderKey) error {
	if !slices.Equal(got, want) {
		return fmt.Errorf("%w: post-write attribute order mismatch", domain.ErrResourceValidation)
	}
	return nil
}

func readAttributeOrderTx(ctx context.Context, tx pgx.Tx, scope domain.ResourceScope) (domain.AttributeOrderReadResult, error) {
	catalog, state, err := readAttributeOrderStateTx(ctx, tx, scope)
	if err != nil {
		return domain.AttributeOrderReadResult{}, err
	}
	return assembleAttributeOrderRead(catalog, scope, state)
}

// readAttributeOrderStateTx resolves scope, loads the structural catalog and
// the raw (not-yet-baseline-reordered) member/saved-binding state — the
// shared core both readAttributeOrderTx (public read assembly) and
// writeAttributeOrderTx (locked CAS write) build on, so the exact same
// resolve/load/scan queries and error classification are never duplicated.
func readAttributeOrderStateTx(ctx context.Context, tx pgx.Tx, scope domain.ResourceScope) (domain.ResourceCatalog, domain.AttributeOrderState, error) {
	state := domain.AttributeOrderState{}
	var familyID int64
	err := tx.QueryRow(ctx, `SELECT t.id,f.id,COALESCE(o.revision,0)
  FROM public.resource_types t JOIN public.resource_families f ON f.id=t.family_id
  JOIN public.resource_classes c ON c.id=f.class_id
  LEFT JOIN public.resource_type_attribute_orders o ON o.type_id=t.id
  WHERE c.code=$1 AND f.code=$2 AND t.code=$3`, scope.ClassCode, scope.FamilyCode, scope.TypeCode).
		Scan(&state.TypeID, &familyID, &state.HeadRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ResourceCatalog{}, domain.AttributeOrderState{}, domain.ErrCatalogRecordNotFound
	}
	if err != nil {
		return domain.ResourceCatalog{}, domain.AttributeOrderState{}, fmt.Errorf("resolve attribute order scope: %w", err)
	}
	catalog, err := loadResourceCatalogTx(ctx, tx)
	if err != nil {
		return domain.ResourceCatalog{}, domain.AttributeOrderState{}, err
	}
	rows, err := tx.Query(ctx, `SELECT a.id,a.revision,a.type_id IS NULL,d.code
  FROM public.resource_attributes a JOIN public.attribute_definitions d ON d.id=a.definition_id
  WHERE a.family_id=$1 AND (a.type_id IS NULL OR a.type_id=$2)`, familyID, state.TypeID)
	if err != nil {
		return domain.ResourceCatalog{}, domain.AttributeOrderState{}, fmt.Errorf("read attribute order members: %w", err)
	}
	state.Members, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.AttributeOrderMember, error) {
		var member domain.AttributeOrderMember
		var inherited bool
		err := row.Scan(&member.ResourceAttributeID, &member.Revision, &inherited, &member.Key.CharacteristicCode)
		member.Key.SourceLevel, member.Key.SourceCode = domain.SourceLevelType, scope.TypeCode
		if inherited {
			member.Key.SourceLevel, member.Key.SourceCode = domain.SourceLevelFamily, scope.FamilyCode
		}
		return member, err
	})
	if err != nil {
		return domain.ResourceCatalog{}, domain.AttributeOrderState{}, fmt.Errorf("scan attribute order members: %w", err)
	}
	rows, err = tx.Query(ctx, `SELECT resource_attribute_id FROM public.resource_type_attribute_order_items
  WHERE target_type_id=$1 ORDER BY position`, state.TypeID)
	if err != nil {
		return domain.ResourceCatalog{}, domain.AttributeOrderState{}, fmt.Errorf("read saved attribute order: %w", err)
	}
	state.SavedBindingIDs, err = pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return domain.ResourceCatalog{}, domain.AttributeOrderState{}, fmt.Errorf("scan saved attribute order: %w", err)
	}
	return catalog, state, nil
}

func assembleAttributeOrderRead(catalog domain.ResourceCatalog, scope domain.ResourceScope, state domain.AttributeOrderState) (domain.AttributeOrderReadResult, error) {
	baseline, err := baselineAttributeOrderMembers(catalog, scope, state.Members)
	if err != nil {
		return domain.AttributeOrderReadResult{}, err
	}
	state.Members = baseline
	order, err := domain.ResolveAttributeOrder(scope, state)
	if err != nil {
		return domain.AttributeOrderReadResult{}, err
	}
	return domain.NewAttributeOrderReadResult(catalog, order), nil
}

// baselineAttributeOrderMembers reorders members (raw, unordered persisted
// state) to match catalog.EffectiveAttributesFor(scope, nil)'s own order,
// keyed by AttributeOrderKey — the authoritative structural baseline both
// the read assembly and the write plan resolve keys/incarnations against.
// Shared so the "every effective occurrence has exactly one persisted
// binding" coherence check is never duplicated.
func baselineAttributeOrderMembers(catalog domain.ResourceCatalog, scope domain.ResourceScope, members []domain.AttributeOrderMember) ([]domain.AttributeOrderMember, error) {
	effective, err := catalog.EffectiveAttributesFor(scope, nil)
	if err != nil {
		return nil, err
	}
	byKey := make(map[domain.AttributeOrderKey]domain.AttributeOrderMember, len(members))
	for _, member := range members {
		byKey[member.Key] = member
	}
	if len(byKey) != len(members) || len(byKey) != len(effective) {
		return nil, fmt.Errorf("%w: inconsistent attribute order membership", domain.ErrResourceValidation)
	}
	baseline := make([]domain.AttributeOrderMember, len(effective))
	for i, a := range effective {
		key := domain.AttributeOrderKey{SourceLevel: a.SourceLevel, SourceCode: a.SourceCode, CharacteristicCode: a.Attribute.Definition.Code}
		member, ok := byKey[key]
		if !ok {
			return nil, fmt.Errorf("%w: missing attribute order binding", domain.ErrResourceValidation)
		}
		baseline[i] = member
	}
	return baseline, nil
}
