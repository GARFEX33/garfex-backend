package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/GARFEX33/garfex-backend/internal/domain"
)

// defaultSearchLimit is applied when SearchCriteria.Limit is not a positive
// value, so that "0 means default" never silently degrades to LIMIT 0
// (which would return zero rows).
const defaultSearchLimit = 50

// clampLimitOffset normalizes the pagination inputs of a Search: a
// non-positive limit falls back to defaultSearchLimit and a negative offset
// clamps to zero.
func clampLimitOffset(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

type resourceSearchBase struct {
	id          int64
	classCode   string
	familyCode  string
	typeCode    string
	naturalUnit string
	identityKey string
	active      bool
	corrupt     bool
}

// Search returns resources in the requested lifecycle scope through the same
// bounded, set-hydrated page path used by SearchPage.
func (r *resourceRepository) Search(ctx context.Context, criteria domain.SearchCriteria) ([]domain.Resource, error) {
	page, err := r.SearchPage(ctx, criteria)
	return page.Resources, err
}

func (r *resourceRepository) SearchPage(ctx context.Context, criteria domain.SearchCriteria) (domain.ResourcePage, error) {
	if r.pool == nil {
		return domain.ResourcePage{}, errors.New("resource repository: nil pool")
	}
	normalized, err := criteria.Normalize()
	if err != nil {
		return domain.ResourcePage{}, fmt.Errorf("search resources: %w", err)
	}
	criteria = normalized
	limit, offset := criteria.Limit, criteria.Offset

	var conditions []string
	var args []any
	arg := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}

	if criteria.LifecycleScope == domain.LifecycleScopeInactive {
		conditions = append(conditions, "NOT r.active")
	} else {
		conditions = append(conditions, "r.active")
	}
	if criteria.ClassCode != "" {
		conditions = append(conditions, "cl.code = "+arg(criteria.ClassCode))
	}
	if criteria.FamilyCode != "" {
		conditions = append(conditions, "f.code = "+arg(criteria.FamilyCode))
	}
	if criteria.TypeCode != "" {
		conditions = append(conditions, "t.code = "+arg(criteria.TypeCode))
	}
	if criteria.Text != "" {
		placeholder := arg("%" + criteria.Text + "%")
		conditions = append(conditions, fmt.Sprintf("(r.identity_key ILIKE %s OR f.code ILIKE %s OR f.name ILIKE %s)", placeholder, placeholder, placeholder))
	}
	for _, filter := range criteria.Filters {
		condition, err := effectiveValueFilter(filter, arg)
		if err != nil {
			return domain.ResourcePage{}, fmt.Errorf("encode search filter %q: %w", filter.AttributeCode, err)
		}
		conditions = append(conditions, condition)
	}

	limitArg, offsetArg := arg(limit+1), arg(offset)
	query := fmt.Sprintf(effectiveValuesCTE+`
		SELECT r.id, cl.code, f.code, t.code, u.code, r.identity_key, r.active
			, EXISTS (SELECT 1 FROM effective_values ev WHERE ev.resource_id = r.id AND ev.scope_count > 1)
		FROM public.recursos r
		JOIN public.resource_classes cl ON cl.id = r.class_id
		JOIN public.resource_families f ON f.id = r.family_id
		JOIN public.resource_types t ON t.id = r.type_id
		JOIN public.unit_definitions u ON u.id = r.natural_unit_id
		WHERE %s
			ORDER BY r.identity_key ASC, r.id ASC
			LIMIT %s OFFSET %s`, strings.Join(conditions, " AND "), limitArg, offsetArg)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return domain.ResourcePage{}, fmt.Errorf("search resources: %w", err)
	}
	defer rows.Close()

	var bases []resourceSearchBase
	var resourceIDs []int64
	for rows.Next() {
		var base resourceSearchBase
		if err := rows.Scan(&base.id, &base.classCode, &base.familyCode, &base.typeCode, &base.naturalUnit, &base.identityKey, &base.active, &base.corrupt); err != nil {
			return domain.ResourcePage{}, fmt.Errorf("scan search result: %w", err)
		}
		bases = append(bases, base)
		resourceIDs = append(resourceIDs, base.id)
	}
	if err := rows.Err(); err != nil {
		return domain.ResourcePage{}, fmt.Errorf("read search results: %w", err)
	}
	hasNext := len(bases) > limit
	if hasNext {
		bases = bases[:limit]
		resourceIDs = resourceIDs[:limit]
	}
	for _, base := range bases {
		if base.corrupt {
			return domain.ResourcePage{}, integrityError("resource %d has duplicate effective attribute values", base.id)
		}
	}
	page := domain.ResourcePage{Criteria: criteria, HasPrevious: offset > 0, HasNext: hasNext}
	if len(bases) == 0 {
		return page, nil
	}

	attributes, err := r.loadEffectiveAttributes(ctx, resourceIDs)
	if err != nil {
		return domain.ResourcePage{}, fmt.Errorf("hydrate search results: %w", err)
	}
	for _, base := range bases {
		values := attributes[base.id]
		resource, err := domain.HydrateResource(domain.ResourceSnapshot{
			ID: base.id, ClassCode: base.classCode, FamilyCode: base.familyCode,
			TypeCode: base.typeCode, NaturalUnit: base.naturalUnit,
			Attributes: values, IdentityKey: base.identityKey, Active: base.active,
		})
		if err != nil {
			return domain.ResourcePage{}, fmt.Errorf("hydrate matched resource %s/%s: %w", base.classCode, base.identityKey, err)
		}
		page.Resources = append(page.Resources, resource)
	}
	return page, nil
}
