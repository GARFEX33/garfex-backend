package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/GARFEX33/garfex-backend/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type resourceRepository struct{ pool *pgxpool.Pool }

// NewResourceRepository returns the PostgreSQL implementation of the Resource
// repository port. Reads remain class-scoped and include inactive historical
// rows; writes require canonical resources and active catalog references.
func NewResourceRepository(pool *pgxpool.Pool) domain.ResourceRepository {
	return &resourceRepository{pool: pool}
}

func (r *resourceRepository) Create(ctx context.Context, resource domain.Resource) error {
	if err := resource.ValidateForPersistence(); err != nil {
		return err
	}
	if r.pool == nil {
		return errors.New("resource repository: nil pool")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin resource create: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var resourceID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO public.recursos (class_id, family_id, type_id, natural_unit_id, display_name, identity_key)
		SELECT cl.id, f.id, t.id, u.id, $5, $6
		FROM public.resource_classes cl
		JOIN public.resource_families f ON f.class_id = cl.id AND f.code = $2
		JOIN public.resource_types t ON t.family_id = f.id AND t.code = $3
		JOIN public.unit_definitions u ON u.code = $4
		JOIN public.resource_unit_policies p ON p.family_id = f.id AND p.unit_id = u.id AND p.allowed AND p.active
		WHERE cl.code = $1 AND cl.active AND f.active AND t.active AND u.active
		RETURNING id`, resource.ClassCode, resource.FamilyCode, resource.TypeCode, resource.NaturalUnit, resource.FamilyCode, resource.IdentityKey).Scan(&resourceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: class %q, family %q, type %q, or unit %q", domain.ErrResourceReference, resource.ClassCode, resource.FamilyCode, resource.TypeCode, resource.NaturalUnit)
		}
		return mapRepositoryError(fmt.Errorf("insert resource: %w", err))
	}
	for _, value := range resource.Attributes {
		if err := persistAttributeValue(ctx, tx, resource, resourceID, value); err != nil {
			return err
		}
	}
	if err := verifyAttributeCount(ctx, tx, resourceID, len(resource.Attributes)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit resource create: %w", err)
	}
	return nil
}

func (r *resourceRepository) Get(ctx context.Context, classCode, identityKey string) (domain.Resource, error) {
	if r.pool == nil {
		return domain.Resource{}, errors.New("resource repository: nil pool")
	}
	var resourceID int64
	var naturalUnit, storedClass, storedFamily, typeCode string
	var active bool
	err := r.pool.QueryRow(ctx, `
		SELECT r.id, cl.code, f.code, t.code, u.code, r.active FROM public.recursos r
		JOIN public.resource_classes cl ON cl.id = r.class_id
		JOIN public.resource_families f ON f.id = r.family_id
		JOIN public.resource_types t ON t.id = r.type_id
		JOIN public.unit_definitions u ON u.id = r.natural_unit_id
		WHERE cl.code = $1 AND r.identity_key = $2`, classCode, identityKey).Scan(&resourceID, &storedClass, &storedFamily, &typeCode, &naturalUnit, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Resource{}, fmt.Errorf("%w: resource %s/%s", domain.ErrResourceNotFound, classCode, identityKey)
	}
	if err != nil {
		return domain.Resource{}, fmt.Errorf("load resource: %w", err)
	}
	attributes, err := r.loadEffectiveAttributes(ctx, []int64{resourceID})
	if err != nil {
		return domain.Resource{}, fmt.Errorf("load resource attributes: %w", err)
	}
	set := attributes[resourceID]
	return domain.HydrateResource(domain.ResourceSnapshot{
		ID: resourceID, ClassCode: storedClass, FamilyCode: storedFamily, TypeCode: typeCode,
		NaturalUnit: naturalUnit, Attributes: set, IdentityKey: identityKey, Active: active,
	})
}

func (r *resourceRepository) GetByID(ctx context.Context, id int64) (domain.Resource, error) {
	if r.pool == nil {
		return domain.Resource{}, errors.New("resource repository: nil pool")
	}
	var classCode, identityKey string
	err := r.pool.QueryRow(ctx, `
		SELECT cl.code, r.identity_key
		FROM public.recursos r
		JOIN public.resource_classes cl ON cl.id = r.class_id
		WHERE r.id = $1`, id).Scan(&classCode, &identityKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Resource{}, fmt.Errorf("%w: resource id %d", domain.ErrResourceNotFound, id)
	}
	if err != nil {
		return domain.Resource{}, fmt.Errorf("find resource %d: %w", id, err)
	}
	return r.Get(ctx, classCode, identityKey)
}

func (r *resourceRepository) Update(ctx context.Context, resource domain.Resource) error {
	if err := resource.ValidateForPersistence(); err != nil {
		return err
	}
	if r.pool == nil {
		return errors.New("resource repository: nil pool")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin resource update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var classID, familyID, typeID, unitID int64
	err = tx.QueryRow(ctx, `
		SELECT cl.id, f.id, t.id, u.id
		FROM public.resource_classes cl
		JOIN public.resource_families f ON f.class_id = cl.id AND f.code = $2
		JOIN public.resource_types t ON t.family_id = f.id AND t.code = $3
		JOIN public.unit_definitions u ON u.code = $4
		JOIN public.resource_unit_policies p ON p.family_id = f.id AND p.unit_id = u.id AND p.allowed AND p.active
		WHERE cl.code = $1 AND cl.active AND f.active AND t.active AND u.active`, resource.ClassCode, resource.FamilyCode, resource.TypeCode, resource.NaturalUnit).Scan(&classID, &familyID, &typeID, &unitID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: class %q, family %q, type %q, or unit %q", domain.ErrResourceReference, resource.ClassCode, resource.FamilyCode, resource.TypeCode, resource.NaturalUnit)
		}
		return fmt.Errorf("resolve resource references: %w", err)
	}

	tag, err := tx.Exec(ctx, `
		UPDATE public.recursos
		SET class_id = $1, family_id = $2, type_id = $3, natural_unit_id = $4, display_name = $5, identity_key = $6, updated_at = NOW()
		WHERE id = $7`, classID, familyID, typeID, unitID, resource.FamilyCode, resource.IdentityKey, resource.ID)
	if err != nil {
		return mapRepositoryError(fmt.Errorf("update resource: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: resource id %d", domain.ErrResourceNotFound, resource.ID)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM public.resource_attribute_values WHERE resource_id = $1`, resource.ID); err != nil {
		return fmt.Errorf("clear resource attributes: %w", err)
	}
	for _, value := range resource.Attributes {
		if err := persistAttributeValue(ctx, tx, resource, resource.ID, value); err != nil {
			return err
		}
	}
	if err := verifyAttributeCount(ctx, tx, resource.ID, len(resource.Attributes)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit resource update: %w", err)
	}
	return nil
}

type resourceAttributeTarget struct {
	familyID            int64
	resourceAttributeID int64
	definitionID        int64
	optionSet           string
	quantityUnitID      *int64
}

func resolveAttributeTarget(ctx context.Context, tx pgx.Tx, resource domain.Resource, value domain.ResourceAttributeValue) (resourceAttributeTarget, error) {
	rows, err := tx.Query(ctx, `
		SELECT f.id, ra.id, d.id, ra.option_set, qu.id
		FROM public.resource_classes cl
		JOIN public.resource_families f ON f.class_id = cl.id AND f.code = $2
		JOIN public.resource_types t ON t.class_id = cl.id AND t.family_id = f.id AND t.code = $3
		JOIN public.resource_attributes ra ON ra.class_id = cl.id AND ra.family_id = f.id
			AND (ra.type_id = t.id OR ra.type_id IS NULL)
		JOIN public.attribute_definitions d ON d.id = ra.definition_id
			AND d.code = $4 AND d.value_type = $5
		LEFT JOIN public.unit_definitions qu ON qu.code = $6 AND qu.active
		WHERE cl.code = $1 AND cl.active AND f.active AND t.active AND ra.active AND d.active`,
		resource.ClassCode, resource.FamilyCode, resource.TypeCode, value.AttributeCode, value.Type, quantityUnitCode(value))
	if err != nil {
		return resourceAttributeTarget{}, fmt.Errorf("resolve attribute %q: %w", value.AttributeCode, err)
	}
	defer rows.Close()
	var targets []resourceAttributeTarget
	for rows.Next() {
		var target resourceAttributeTarget
		if err := rows.Scan(&target.familyID, &target.resourceAttributeID, &target.definitionID, &target.optionSet, &target.quantityUnitID); err != nil {
			return resourceAttributeTarget{}, fmt.Errorf("scan attribute target %q: %w", value.AttributeCode, err)
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return resourceAttributeTarget{}, fmt.Errorf("read attribute targets %q: %w", value.AttributeCode, err)
	}
	if len(targets) != 1 {
		return resourceAttributeTarget{}, fmt.Errorf("%w: attribute %q resolved to %d targets", domain.ErrResourceIntegrity, value.AttributeCode, len(targets))
	}
	if value.Type == domain.ValueTypeQuantity && targets[0].quantityUnitID == nil {
		return resourceAttributeTarget{}, fmt.Errorf("%w: quantity attribute %q has no active unit %q", domain.ErrResourceIntegrity, value.AttributeCode, value.Quantity.UnitCode)
	}
	return targets[0], nil
}

func quantityUnitCode(value domain.ResourceAttributeValue) string {
	if value.Type == domain.ValueTypeQuantity && value.Quantity != nil {
		return value.Quantity.UnitCode
	}
	return ""
}

func persistAttributeValue(ctx context.Context, tx pgx.Tx, resource domain.Resource, resourceID int64, value domain.ResourceAttributeValue) error {
	payload, err := encodeValue(value)
	if err != nil {
		return fmt.Errorf("encode attribute %q: %w", value.AttributeCode, err)
	}
	target, err := resolveAttributeTarget(ctx, tx, resource, value)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO public.resource_attribute_values
		(resource_id, family_id, resource_attribute_id, attribute_definition_id, option_set, value_state,
		 option_code, integer_value, decimal_value, quantity_value, quantity_unit_id, boolean_value, text_value)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		resourceID, target.familyID, target.resourceAttributeID, target.definitionID, target.optionSet, payload.state,
		nullableString(payload.option), payload.integer, payload.decimal, nullableQuantity(payload), target.quantityUnitID,
		payload.boolean, nullableString(payload.text))
	if err != nil {
		return mapRepositoryError(fmt.Errorf("insert resource attribute %q: %w", value.AttributeCode, err))
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: attribute %q write affected %d rows", domain.ErrResourceIntegrity, value.AttributeCode, tag.RowsAffected())
	}
	return nil
}

func verifyAttributeCount(ctx context.Context, tx pgx.Tx, resourceID int64, expected int) error {
	var persisted int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM public.resource_attribute_values WHERE resource_id=$1`, resourceID).Scan(&persisted); err != nil {
		return fmt.Errorf("count resource attributes: %w", err)
	}
	if persisted != expected {
		return fmt.Errorf("%w: resource %d persisted %d attributes, expected %d", domain.ErrResourceIntegrity, resourceID, persisted, expected)
	}
	return nil
}

func (r *resourceRepository) Deactivate(ctx context.Context, id int64) (domain.LifecycleResult, error) {
	return r.setLifecycle(ctx, id, false, "")
}

func (r *resourceRepository) Reactivate(ctx context.Context, id int64, identityKey string) (domain.LifecycleResult, error) {
	return r.setLifecycle(ctx, id, true, identityKey)
}

func (r *resourceRepository) SetActive(ctx context.Context, id int64, active bool) error {
	if active {
		_, err := r.setLifecycle(ctx, id, true, "")
		return err
	}
	_, err := r.Deactivate(ctx, id)
	return err
}

func (r *resourceRepository) setLifecycle(ctx context.Context, id int64, active bool, expectedIdentityKey string) (domain.LifecycleResult, error) {
	if r.pool == nil {
		return domain.LifecycleResult{}, errors.New("resource repository: nil pool")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.LifecycleResult{}, fmt.Errorf("begin resource lifecycle: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var classCode, identityKey string
	var currentActive bool
	err = tx.QueryRow(ctx, `
		SELECT cl.code, r.identity_key, r.active
		FROM public.recursos r
		JOIN public.resource_classes cl ON cl.id = r.class_id
		WHERE r.id = $1 FOR UPDATE`, id).Scan(&classCode, &identityKey, &currentActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.LifecycleResult{}, fmt.Errorf("%w: resource id %d", domain.ErrResourceNotFound, id)
	}
	if err != nil {
		return domain.LifecycleResult{}, fmt.Errorf("load resource lifecycle %d: %w", id, err)
	}
	if expectedIdentityKey != "" && expectedIdentityKey != identityKey {
		return domain.LifecycleResult{}, domain.ErrResourceIntegrity
	}
	if currentActive == active {
		if err := tx.Commit(ctx); err != nil {
			return domain.LifecycleResult{}, fmt.Errorf("commit resource lifecycle no-op: %w", err)
		}
		resource, err := r.Get(ctx, classCode, identityKey)
		return domain.LifecycleResult{Resource: resource}, err
	}
	query := `UPDATE public.recursos SET active = $2, updated_at = NOW() WHERE id = $1`
	args := []any{id, active}
	if active {
		query = `UPDATE public.recursos r SET active = TRUE, updated_at = NOW()
			FROM public.resource_classes cl, public.resource_families f, public.resource_types t,
				public.unit_definitions u, public.resource_unit_policies p
			WHERE r.id = $1 AND cl.id = r.class_id AND cl.active AND f.id = r.family_id AND f.active
				AND t.id = r.type_id AND t.active AND u.id = r.natural_unit_id AND u.active
				AND p.family_id = f.id AND p.unit_id = u.id AND p.allowed AND p.active`
		args = []any{id}
	}
	tag, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return domain.LifecycleResult{}, mapRepositoryError(fmt.Errorf("set resource active: %w", err))
	}
	if tag.RowsAffected() != 1 {
		if active {
			var conflict bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM public.recursos WHERE active AND identity_key = $1 AND id <> $2)`, identityKey, id).Scan(&conflict); err == nil && conflict {
				return domain.LifecycleResult{}, domain.ErrDuplicateResource
			}
			return domain.LifecycleResult{}, domain.ErrResourceReference
		}
		return domain.LifecycleResult{}, fmt.Errorf("%w: resource id %d", domain.ErrResourceNotFound, id)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.LifecycleResult{}, fmt.Errorf("commit resource lifecycle: %w", err)
	}
	resource, err := r.Get(ctx, classCode, identityKey)
	if err != nil {
		return domain.LifecycleResult{}, err
	}
	return domain.LifecycleResult{Resource: resource, Changed: true}, nil
}

// errResourceRevisionRequired is a defense-in-depth guard; recursos.Service
// already rejects a zero expectedRevision first.
var errResourceRevisionRequired = errors.New("resource lifecycle revision CAS requires a non-zero expected revision")

func (r *resourceRepository) DeactivateRevision(ctx context.Context, id int64, expectedRevision uint64) (domain.Resource, error) {
	return r.setLifecycleRevision(ctx, id, false, "", expectedRevision)
}

func (r *resourceRepository) ReactivateRevision(ctx context.Context, id int64, expectedIdentityKey string, expectedRevision uint64) (domain.Resource, error) {
	return r.setLifecycleRevision(ctx, id, true, expectedIdentityKey, expectedRevision)
}

// setLifecycleRevision mirrors setLifecycle's row-locked transition, adding
// the CAS expectedRevision predicate under the same row lock: absent row is
// domain.ErrResourceNotFound, present-but-different-revision is
// domain.ErrResourceRevisionConflict — never inferred from an error string
// (mirrors casUpdateRevision's disambiguation, catalog_admin_repository_v2.go).
// Revision increments only on an actual active-state transition; a no-op
// commits without incrementing. identity_key is never touched here.
func (r *resourceRepository) setLifecycleRevision(ctx context.Context, id int64, active bool, expectedIdentityKey string, expectedRevision uint64) (domain.Resource, error) {
	if r.pool == nil {
		return domain.Resource{}, errors.New("resource repository: nil pool")
	}
	if expectedRevision == 0 {
		return domain.Resource{}, errResourceRevisionRequired
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Resource{}, fmt.Errorf("begin resource lifecycle revision: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var classCode, identityKey string
	var currentActive bool
	var currentRevision uint64
	err = tx.QueryRow(ctx, `
		SELECT cl.code, r.identity_key, r.active, r.revision
		FROM public.recursos r
		JOIN public.resource_classes cl ON cl.id = r.class_id
		WHERE r.id = $1 FOR UPDATE`, id).Scan(&classCode, &identityKey, &currentActive, &currentRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Resource{}, fmt.Errorf("%w: resource id %d", domain.ErrResourceNotFound, id)
	}
	if err != nil {
		return domain.Resource{}, fmt.Errorf("load resource lifecycle revision %d: %w", id, err)
	}
	if expectedIdentityKey != "" && expectedIdentityKey != identityKey {
		return domain.Resource{}, domain.ErrResourceIntegrity
	}
	if currentRevision != expectedRevision {
		return domain.Resource{}, domain.ErrResourceRevisionConflict
	}
	if currentActive == active {
		if err := tx.Commit(ctx); err != nil {
			return domain.Resource{}, fmt.Errorf("commit resource lifecycle revision no-op: %w", err)
		}
		return r.getWithRevision(ctx, id, classCode, identityKey)
	}

	query := `UPDATE public.recursos SET active=$2, revision=revision+1, updated_at=NOW() WHERE id=$1 AND revision=$3`
	args := []any{id, active, expectedRevision}
	if active {
		query = `UPDATE public.recursos r SET active=TRUE, revision=r.revision+1, updated_at=NOW()
			FROM public.resource_classes cl, public.resource_families f, public.resource_types t,
				public.unit_definitions u, public.resource_unit_policies p
			WHERE r.id=$1 AND r.revision=$2 AND cl.id=r.class_id AND cl.active AND f.id=r.family_id AND f.active
				AND t.id=r.type_id AND t.active AND u.id=r.natural_unit_id AND u.active
				AND p.family_id=f.id AND p.unit_id=u.id AND p.allowed AND p.active`
		args = []any{id, expectedRevision}
	}
	tag, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return domain.Resource{}, mapRepositoryError(fmt.Errorf("set resource active revision: %w", err))
	}
	if tag.RowsAffected() != 1 {
		if active {
			// Already row-locked at a matching revision above, so a zero-row
			// CAS UPDATE here means the catalog scope is no longer active.
			return domain.Resource{}, domain.ErrResourceReference
		}
		return domain.Resource{}, fmt.Errorf("%w: resource id %d", domain.ErrResourceNotFound, id)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Resource{}, fmt.Errorf("commit resource lifecycle revision: %w", err)
	}
	return r.getWithRevision(ctx, id, classCode, identityKey)
}

// UpdateRevision CAS-locks the parent row (mirroring setLifecycleRevision),
// then fully replaces the attribute set in the same transaction, rolling
// both back together on either failure. identity_key is never in the SET.
func (r *resourceRepository) UpdateRevision(ctx context.Context, resource domain.Resource, expectedRevision uint64) (domain.Resource, error) {
	if err := resource.ValidateForPersistence(); err != nil {
		return domain.Resource{}, err
	}
	if r.pool == nil {
		return domain.Resource{}, errors.New("resource repository: nil pool")
	}
	if expectedRevision == 0 {
		return domain.Resource{}, errResourceRevisionRequired
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Resource{}, fmt.Errorf("begin resource update revision: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var classCode, identityKey string
	var currentRevision uint64
	err = tx.QueryRow(ctx, `
		SELECT cl.code, r.identity_key, r.revision
		FROM public.recursos r
		JOIN public.resource_classes cl ON cl.id = r.class_id
		WHERE r.id = $1 FOR UPDATE`, resource.ID).Scan(&classCode, &identityKey, &currentRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Resource{}, fmt.Errorf("%w: resource id %d", domain.ErrResourceNotFound, resource.ID)
	}
	if err != nil {
		return domain.Resource{}, fmt.Errorf("load resource update revision %d: %w", resource.ID, err)
	}
	if currentRevision != expectedRevision {
		return domain.Resource{}, domain.ErrResourceRevisionConflict
	}

	var classID, familyID, typeID, unitID int64
	err = tx.QueryRow(ctx, `
		SELECT cl.id, f.id, t.id, u.id
		FROM public.resource_classes cl
		JOIN public.resource_families f ON f.class_id = cl.id AND f.code = $2
		JOIN public.resource_types t ON t.family_id = f.id AND t.code = $3
		JOIN public.unit_definitions u ON u.code = $4
		JOIN public.resource_unit_policies p ON p.family_id = f.id AND p.unit_id = u.id AND p.allowed AND p.active
		WHERE cl.code = $1 AND cl.active AND f.active AND t.active AND u.active`, resource.ClassCode, resource.FamilyCode, resource.TypeCode, resource.NaturalUnit).Scan(&classID, &familyID, &typeID, &unitID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Resource{}, fmt.Errorf("%w: class %q, family %q, type %q, or unit %q", domain.ErrResourceReference, resource.ClassCode, resource.FamilyCode, resource.TypeCode, resource.NaturalUnit)
		}
		return domain.Resource{}, fmt.Errorf("resolve resource references: %w", err)
	}

	tag, err := tx.Exec(ctx, `
		UPDATE public.recursos
		SET class_id = $1, family_id = $2, type_id = $3, natural_unit_id = $4, display_name = $5,
		    revision = revision + 1, updated_at = NOW()
		WHERE id = $6 AND revision = $7`, classID, familyID, typeID, unitID, resource.FamilyCode, resource.ID, expectedRevision)
	if err != nil {
		return domain.Resource{}, mapRepositoryError(fmt.Errorf("update resource revision: %w", err))
	}
	if tag.RowsAffected() != 1 {
		// Already row-locked above, so a zero-row result is a raced transition.
		return domain.Resource{}, domain.ErrResourceRevisionConflict
	}

	if _, err := tx.Exec(ctx, `DELETE FROM public.resource_attribute_values WHERE resource_id = $1`, resource.ID); err != nil {
		return domain.Resource{}, fmt.Errorf("clear resource attributes: %w", err)
	}
	for _, value := range resource.Attributes {
		if err := persistAttributeValue(ctx, tx, resource, resource.ID, value); err != nil {
			return domain.Resource{}, err
		}
	}
	if err := verifyAttributeCount(ctx, tx, resource.ID, len(resource.Attributes)); err != nil {
		return domain.Resource{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Resource{}, fmt.Errorf("commit resource update revision: %w", err)
	}
	return r.getWithRevision(ctx, resource.ID, classCode, identityKey)
}

// getWithRevision reloads the committed resource plus its revision — Get
// does not select revision yet, so this performs its own minimal reload
// rather than widening Get's query/behavior in this slice.
func (r *resourceRepository) getWithRevision(ctx context.Context, id int64, classCode, identityKey string) (domain.Resource, error) {
	resource, err := r.Get(ctx, classCode, identityKey)
	if err != nil {
		return domain.Resource{}, fmt.Errorf("reload resource %d after lifecycle revision: %w", id, err)
	}
	if err := r.pool.QueryRow(ctx, `SELECT revision FROM public.recursos WHERE id=$1`, id).Scan(&resource.Revision); err != nil {
		return domain.Resource{}, fmt.Errorf("load resource %d revision: %w", id, err)
	}
	return resource, nil
}
