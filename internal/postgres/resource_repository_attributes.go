package postgres

import (
	"context"
	"fmt"
	"github.com/GARFEX33/garfex-backend/internal/domain"
)

const effectiveValuesCTE = `WITH effective_candidates AS (SELECT v.resource_id,v.id AS value_id,v.value_state,v.option_code,v.integer_value,v.decimal_value,v.quantity_value,v.quantity_unit_id,v.boolean_value,v.text_value,d.id AS definition_id,d.code AS definition_code,d.value_type,(v.attribute_definition_id=ra.definition_id AND v.option_set=ra.option_set) AS metadata_ok,CASE WHEN ra.type_id IS NULL THEN 0 ELSE 1 END AS scope_rank FROM public.resource_attribute_values v JOIN public.recursos r ON r.id=v.resource_id JOIN public.resource_attributes ra ON ra.id=v.resource_attribute_id AND ra.class_id=r.class_id AND ra.family_id=r.family_id AND (ra.type_id IS NULL OR ra.type_id=r.type_id) JOIN public.attribute_definitions d ON d.id=ra.definition_id), ranked_effective_candidates AS (SELECT c.*,count(*) OVER (PARTITION BY c.resource_id,c.definition_code,c.scope_rank) AS scope_count FROM effective_candidates c), effective_values AS (SELECT c.* FROM ranked_effective_candidates c WHERE c.scope_rank=(SELECT max(w.scope_rank) FROM ranked_effective_candidates w WHERE w.resource_id=c.resource_id AND w.definition_code=c.definition_code))`

func (r *resourceRepository) loadEffectiveAttributes(ctx context.Context, resourceIDs []int64) (map[int64][]domain.ResourceAttributeValue, error) {
	attributes := make(map[int64][]domain.ResourceAttributeValue, len(resourceIDs))
	if len(resourceIDs) == 0 {
		return attributes, nil
	}
	rows, err := r.pool.Query(ctx, effectiveValuesCTE+`, applicable_attributes AS (SELECT r.id AS resource_id,d.id AS definition_id,d.code,d.value_type,COALESCE(rule.mode,CASE WHEN ra.mode='CONDITIONAL' THEN 'REQUIRED' ELSE ra.mode END) AS mode FROM public.recursos r JOIN public.resource_attributes ra ON ra.class_id=r.class_id AND ra.family_id=r.family_id AND (ra.type_id IS NULL OR ra.type_id=r.type_id) JOIN public.attribute_definitions d ON d.id=ra.definition_id LEFT JOIN LATERAL (SELECT rr.mode FROM public.resource_attribute_rules rr WHERE rr.resource_attribute_id=ra.id AND rr.active AND EXISTS (SELECT 1 FROM effective_values cv WHERE cv.resource_id=r.id AND cv.definition_id=rr.when_definition_id AND cv.value_state='SET' AND cv.option_code=rr.when_equals) ORDER BY rr.display_order LIMIT 1) rule ON TRUE WHERE r.id=ANY($1::bigint[]) AND (ra.type_id=r.type_id OR NOT EXISTS (SELECT 1 FROM public.resource_attributes override JOIN public.attribute_definitions od ON od.id=override.definition_id WHERE override.class_id=r.class_id AND override.family_id=r.family_id AND override.type_id=r.type_id AND od.code=d.code))) SELECT a.resource_id,a.code,a.value_type,a.mode,v.value_id,v.scope_count,v.metadata_ok,v.value_state,v.option_code,v.integer_value,v.decimal_value,v.quantity_value,qu.code,v.boolean_value,v.text_value FROM applicable_attributes a LEFT JOIN effective_values v ON v.resource_id=a.resource_id AND v.definition_code=a.code LEFT JOIN public.unit_definitions qu ON qu.id=v.quantity_unit_id ORDER BY a.resource_id,a.code`, resourceIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var resourceID int64
		var code, valueType, mode string
		var valueID, scopeCount *int64
		var metadataOK *bool
		var state, option, unit, text *string
		var integer *int64
		var dec, quantity *string
		var boolean *bool
		if err := rows.Scan(&resourceID, &code, &valueType, &mode, &valueID, &scopeCount, &metadataOK, &state, &option, &integer, &dec, &quantity, &unit, &boolean, &text); err != nil {
			return nil, fmt.Errorf("scan resource attribute: %w", err)
		}
		set := attributes[resourceID]
		if valueID == nil {
			if mode == string(domain.ModeRequired) {
				return nil, integrityError("resource %d missing required attribute %q", resourceID, code)
			}
			if mode != string(domain.ModeOptional) && mode != string(domain.ModeForbidden) {
				return nil, integrityError("resource %d has unsupported missing attribute mode %q for %q", resourceID, mode, code)
			}
			attributes[resourceID] = set
			continue
		}
		if scopeCount == nil || *scopeCount != 1 || metadataOK == nil || !*metadataOK {
			return nil, integrityError("resource %d has inconsistent metadata for attribute %q", resourceID, code)
		}
		if mode == string(domain.ModeForbidden) && dereference(state) != notApplicableState {
			return nil, integrityError("resource %d has forbidden attribute %q", resourceID, code)
		}
		if dereference(state) == notApplicableState && mode != string(domain.ModeForbidden) {
			return nil, integrityError("resource %d has not-applicable required/optional attribute %q", resourceID, code)
		}
		value, err := decodeValue(code, domain.AttributeValueType(valueType), dereference(state), option, integer, dec, quantity, unit, boolean, text)
		if err != nil {
			return nil, integrityError("decode resource %d attribute %q: %v", resourceID, code, err)
		}
		set = append(set, value)
		attributes[resourceID] = set
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read resource attributes: %w", err)
	}
	return attributes, nil
}
func integrityError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", domain.ErrResourceIntegrity, fmt.Sprintf(format, args...))
}
func effectiveValueFilter(filter domain.ResourceAttributeValue, arg func(any) string) (string, error) {
	payload, err := encodeValue(filter)
	if err != nil {
		return "", err
	}
	codeArg := arg(filter.AttributeCode)
	var valueCondition string
	switch filter.Type {
	case domain.ValueTypeControlledOption:
		valueCondition = "ev.option_code = " + arg(payload.option)
	case domain.ValueTypeInteger:
		valueCondition = "ev.integer_value = " + arg(payload.integer)
	case domain.ValueTypeDecimal:
		valueCondition = "ev.decimal_value = " + arg(payload.decimal)
	case domain.ValueTypeQuantity:
		valueCondition = fmt.Sprintf("ev.quantity_value = %s AND qu.code = %s", arg(payload.quantity), arg(payload.quantityUnit))
	case domain.ValueTypeBoolean:
		valueCondition = "ev.boolean_value = " + arg(payload.boolean)
	case domain.ValueTypeControlledText:
		valueCondition = "ev.text_value = " + arg(payload.text)
	default:
		return "", fmt.Errorf("unsupported filter value type %q", filter.Type)
	}
	return fmt.Sprintf(`EXISTS (SELECT 1 FROM effective_values ev
		LEFT JOIN public.unit_definitions qu ON qu.id = ev.quantity_unit_id
		WHERE ev.resource_id = r.id AND (ev.scope_count > 1 OR (ev.definition_code = %s
		  AND ev.value_state = 'SET' AND %s)))`, codeArg, valueCondition), nil
}
