package postgres

import (
	"context"
	"fmt"

	"github.com/GARFEX33/garfex-backend/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// kindTables is the one Go map catalog_admin_repository.go dispatches
// through — design §3's explicit "kindTables map[CatalogKindCode]
// tableMapping (table name, column names, parent join, dependency-probe
// SQL, resource-reference-probe SQL). No table introspection — only
// registered kinds are administrable." Four kinds (OptionSet:
// resource_option_sets, Option: attribute_options, UnitPolicy:
// resource_unit_policies, PresentationField:
// resource_type_presentation_fields) have their own natural composite PK
// alongside a real BIGSERIAL id column added by migration 000009 — every
// kind now addresses by id, never by a hashed natural key.
var kindTables = map[domain.CatalogKindCode]kindHandlers{
	domain.KindClass: {
		list: listClasses, get: getClass, insert: insertClass, update: updateClass,
		setActive: simpleSetActive("public.resource_classes", "id"),
		del:       simpleDelete("public.resource_classes", "id"),
		dependents: []dependencyProbe{
			{kind: domain.KindFamily, blocking: true, query: `SELECT count(*) FROM public.resource_families WHERE class_id=$1`},
		},
		referencedByResources: classReferencedByResources,
	},
	domain.KindFamily: {
		list: listFamilies, get: getFamily, insert: insertFamily, update: updateFamily,
		setActive: simpleSetActive("public.resource_families", "id"),
		del:       simpleDelete("public.resource_families", "id"),
		dependents: []dependencyProbe{
			{kind: domain.KindType, blocking: true, query: `SELECT count(*) FROM public.resource_types WHERE family_id=$1`},
			{kind: domain.KindAttributeBinding, blocking: true, query: `SELECT count(*) FROM public.resource_attributes WHERE family_id=$1`},
			{kind: domain.KindUnitPolicy, blocking: true, query: `SELECT count(*) FROM public.resource_unit_policies WHERE family_id=$1`},
		},
		referencedByResources: familyReferencedByResources,
	},
	domain.KindType: {
		list: listTypes, get: getType, insert: insertType, update: updateType,
		setActive: simpleSetActive("public.resource_types", "id"),
		del:       simpleDelete("public.resource_types", "id"),
		dependents: []dependencyProbe{
			{kind: domain.KindAttributeBinding, blocking: true, query: `SELECT count(*) FROM public.resource_attributes WHERE type_id=$1`},
			{kind: domain.KindPresentationField, blocking: false, query: `SELECT count(*) FROM public.resource_type_presentation_fields WHERE type_id=$1`},
		},
		referencedByResources: typeReferencedByResources,
	},
	domain.KindAttributeDefinition: {
		list: listDefinitions, get: getDefinition, insert: insertDefinition, update: updateDefinition,
		setActive: simpleSetActive("public.attribute_definitions", "id"),
		del:       simpleDelete("public.attribute_definitions", "id"),
		dependents: []dependencyProbe{
			{kind: domain.KindAttributeBinding, blocking: true, query: `SELECT count(*) FROM public.resource_attributes WHERE definition_id=$1`},
			{kind: domain.KindOption, blocking: true, query: `SELECT count(*) FROM public.attribute_options WHERE attribute_definition_id=$1`},
		},
		referencedByResources: definitionReferencedByResources,
	},
	domain.KindOptionSet: {
		list: listOptionSets, get: getOptionSet, insert: insertOptionSet, update: updateOptionSet,
		setActive: setActiveOptionSet, del: deleteOptionSet,
		dependents: []dependencyProbe{
			{kind: domain.KindOption, blocking: true, query: `
				SELECT count(*) FROM public.attribute_options ao
				JOIN public.resource_option_sets os ON os.code = ao.option_set
				WHERE os.id = $1`},
			{kind: domain.KindOptionRelation, blocking: true, query: `
				SELECT count(*) FROM public.attribute_option_relations ar
				JOIN public.resource_option_sets os ON os.code = ar.option_set
				WHERE os.id = $1`},
		},
		referencedByResources: optionSetReferencedByResources,
	},
	domain.KindOption: {
		list: listOptions, get: getOption, insert: insertOption, update: updateOption,
		setActive: setActiveOption, del: deleteOption,
		dependents: []dependencyProbe{
			{kind: domain.KindOptionRelation, blocking: true, query: `
				WITH target AS (
					SELECT ao.option_set, ao.attribute_definition_id, ao.code
					FROM public.attribute_options ao
					WHERE ao.id = $1
				)
				SELECT count(*) FROM public.attribute_option_relations ar, target t
				WHERE (ar.option_set = t.option_set AND ar.from_attribute_definition_id = t.attribute_definition_id AND ar.from_option_code = t.code)
				   OR (ar.option_set = t.option_set AND ar.to_attribute_definition_id = t.attribute_definition_id AND ar.to_option_code = t.code)`},
		},
		referencedByResources: optionReferencedByResources,
	},
	domain.KindOptionRelation: {
		list: listOptionRelations, get: getOptionRelation, insert: insertOptionRelation, update: updateOptionRelation,
		setActive: simpleSetActive("public.attribute_option_relations", "id"),
		del:       simpleDelete("public.attribute_option_relations", "id"),
		// No Children registered for OptionRelation (catalog_kind.go) and no
		// código field (junction-kind exemption) — dependents/reference
		// probe both stay empty/nil.
	},
	domain.KindUnit: {
		list: listUnits, get: getUnit, insert: insertUnit, update: updateUnit,
		setActive: simpleSetActive("public.unit_definitions", "id"),
		del:       simpleDelete("public.unit_definitions", "id"),
		dependents: []dependencyProbe{
			{kind: domain.KindUnitPolicy, blocking: true, query: `SELECT count(*) FROM public.resource_unit_policies WHERE unit_id=$1`},
		},
		referencedByResources: unitReferencedByResources,
	},
	domain.KindUnitPolicy: {
		list: listUnitPolicies, get: getUnitPolicy, insert: insertUnitPolicy, update: updateUnitPolicy,
		setActive: setActiveUnitPolicy, del: deleteUnitPolicy,
	},
	domain.KindAttributeBinding: {
		list: listAttributeBindings, get: getAttributeBinding, insert: insertAttributeBinding, update: updateAttributeBinding,
		setActive: simpleSetActive("public.resource_attributes", "id"),
		del:       simpleDelete("public.resource_attributes", "id"),
	},
	domain.KindPresentationField: {
		list: listPresentationFields, get: getPresentationField, insert: insertPresentationField, update: updatePresentationField,
		setActive: setActivePresentationField, del: deletePresentationField,
	},
}

// --- shared FK-resolution helpers (ref code -> internal row id) -----------

func resolveClassID(ctx context.Context, q querier, code string) (int64, error) {
	var id int64
	err := q.QueryRow(ctx, `SELECT id FROM public.resource_classes WHERE code=$1`, code).Scan(&id)
	if isNoRows(err) {
		return 0, fmt.Errorf("%w: class %q", domain.ErrCatalogReference, code)
	}
	if err != nil {
		return 0, fmt.Errorf("resolve class %q: %w", code, err)
	}
	return id, nil
}

func resolveFamilyID(ctx context.Context, q querier, classCode, familyCode string) (familyID, classID int64, err error) {
	err = q.QueryRow(ctx, `
		SELECT f.id, f.class_id FROM public.resource_families f
		JOIN public.resource_classes cl ON cl.id = f.class_id
		WHERE cl.code=$1 AND f.code=$2`, classCode, familyCode).Scan(&familyID, &classID)
	if isNoRows(err) {
		return 0, 0, fmt.Errorf("%w: class %q, family %q", domain.ErrCatalogReference, classCode, familyCode)
	}
	if err != nil {
		return 0, 0, fmt.Errorf("resolve family %q/%q: %w", classCode, familyCode, err)
	}
	return familyID, classID, nil
}

func resolveTypeID(ctx context.Context, q querier, classCode, familyCode, typeCode string) (typeID, familyID int64, err error) {
	err = q.QueryRow(ctx, `
		SELECT t.id, t.family_id FROM public.resource_types t
		JOIN public.resource_families f ON f.id = t.family_id
		JOIN public.resource_classes cl ON cl.id = t.class_id
		WHERE cl.code=$1 AND f.code=$2 AND t.code=$3`, classCode, familyCode, typeCode).Scan(&typeID, &familyID)
	if isNoRows(err) {
		return 0, 0, fmt.Errorf("%w: class %q, family %q, type %q", domain.ErrCatalogReference, classCode, familyCode, typeCode)
	}
	if err != nil {
		return 0, 0, fmt.Errorf("resolve type %q/%q/%q: %w", classCode, familyCode, typeCode, err)
	}
	return typeID, familyID, nil
}

func resolveDefinitionID(ctx context.Context, q querier, code string) (int64, error) {
	var id int64
	err := q.QueryRow(ctx, `SELECT id FROM public.attribute_definitions WHERE code=$1`, code).Scan(&id)
	if isNoRows(err) {
		return 0, fmt.Errorf("%w: characteristic %q", domain.ErrCatalogReference, code)
	}
	if err != nil {
		return 0, fmt.Errorf("resolve attribute definition %q: %w", code, err)
	}
	return id, nil
}

func catalogRecordWithRevision(record domain.CatalogRecord, revision uint64) domain.CatalogRecord {
	record.Revision = revision
	return record
}

// --- KindClass --------------------------------------------------------

func listClasses(ctx context.Context, q querier, f domain.CatalogFilter) ([]domain.CatalogRecord, error) {
	var conditions []string
	var args []any
	conditions, args = appendTextFilter(conditions, args, f.Text, "code", "name")
	conditions = appendActiveFilter(conditions, f.Status, "active")
	sql := finalizeQuery(`SELECT id, code, name, plural, slug, display_order, aliases, keywords, active, revision FROM public.resource_classes`, conditions, "display_order, code", f)

	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query resource_classes: %w", err)
	}
	defer rows.Close()
	var out []domain.CatalogRecord
	for rows.Next() {
		var id int64
		var code, name, plural, slug string
		var order int
		var aliases, keywords []string
		var active bool
		var revision uint64
		if err := rows.Scan(&id, &code, &name, &plural, &slug, &order, &aliases, &keywords, &active, &revision); err != nil {
			return nil, fmt.Errorf("scan resource_classes: %w", err)
		}
		out = append(out, catalogRecordWithRevision(classRecord(id, code, name, plural, slug, order, aliases, keywords, active), revision))
	}
	return out, rows.Err()
}

func getClass(ctx context.Context, q querier, id int64) (domain.CatalogRecord, error) {
	var code, name, plural, slug string
	var order int
	var aliases, keywords []string
	var active bool
	var revision uint64
	err := q.QueryRow(ctx, `SELECT code, name, plural, slug, display_order, aliases, keywords, active, revision FROM public.resource_classes WHERE id=$1`, id).
		Scan(&code, &name, &plural, &slug, &order, &aliases, &keywords, &active, &revision)
	if isNoRows(err) {
		return domain.CatalogRecord{}, domain.ErrCatalogRecordNotFound
	}
	if err != nil {
		return domain.CatalogRecord{}, fmt.Errorf("get resource_classes: %w", err)
	}
	return catalogRecordWithRevision(classRecord(id, code, name, plural, slug, order, aliases, keywords, active), revision), nil
}

func classRecord(id int64, code, name, plural, slug string, order int, aliases, keywords []string, active bool) domain.CatalogRecord {
	return domain.CatalogRecord{Kind: domain.KindClass, ID: id, Active: active, Values: map[string]domain.CatalogValue{
		"code": textValue(code), "name": textValue(name), "plural": textValue(plural), "slug": textValue(slug),
		"order": intValue(order), "aliases": listValue(aliases), "keywords": listValue(keywords),
	}}
}

func insertClass(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `
		INSERT INTO public.resource_classes (code, name, plural, slug, display_order, aliases, keywords, active)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		fieldText(rec, "code"), fieldText(rec, "name"), fieldText(rec, "plural"), fieldText(rec, "slug"),
		fieldInt(rec, "order"), fieldList(rec, "aliases"), fieldList(rec, "keywords"), rec.Active).Scan(&id)
	if err != nil {
		return 0, mapCatalogWriteError(fmt.Errorf("insert resource_classes: %w", err))
	}
	return id, nil
}

func updateClass(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) error {
	referenced, err := classReferencedByResources(ctx, tx, rec.ID)
	if err != nil {
		return err
	}
	var tag pgconn.CommandTag
	if referenced {
		tag, err = tx.Exec(ctx, `UPDATE public.resource_classes SET name=$1, plural=$2, slug=$3, display_order=$4, aliases=$5, keywords=$6, active=$7 WHERE id=$8`,
			fieldText(rec, "name"), fieldText(rec, "plural"), fieldText(rec, "slug"), fieldInt(rec, "order"), fieldList(rec, "aliases"), fieldList(rec, "keywords"), rec.Active, rec.ID)
	} else {
		tag, err = tx.Exec(ctx, `UPDATE public.resource_classes SET code=$1, name=$2, plural=$3, slug=$4, display_order=$5, aliases=$6, keywords=$7, active=$8 WHERE id=$9`,
			fieldText(rec, "code"), fieldText(rec, "name"), fieldText(rec, "plural"), fieldText(rec, "slug"), fieldInt(rec, "order"), fieldList(rec, "aliases"), fieldList(rec, "keywords"), rec.Active, rec.ID)
	}
	if err != nil {
		return mapCatalogWriteError(fmt.Errorf("update resource_classes: %w", err))
	}
	return requireRow(tag.RowsAffected())
}

func classReferencedByResources(ctx context.Context, q querier, id int64) (bool, error) {
	var exists bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.recursos WHERE class_id=$1)`, id).Scan(&exists); err != nil {
		return false, fmt.Errorf("check class referenced by resources: %w", err)
	}
	return exists, nil
}

// --- KindFamily -------------------------------------------------------

func listFamilies(ctx context.Context, q querier, f domain.CatalogFilter) ([]domain.CatalogRecord, error) {
	var conditions []string
	var args []any
	conditions, args = appendTextFilter(conditions, args, f.Text, "f.code", "f.name", "cl.name")
	conditions = appendActiveFilter(conditions, f.Status, "f.active")
	conditions, args = appendEqualsFilter(conditions, args, "cl.code", parentRefCode(f, "class"))
	sql := finalizeQuery(`
		SELECT f.id, cl.id, cl.code, cl.name, f.code, f.name, f.active, f.revision
		FROM public.resource_families f
		JOIN public.resource_classes cl ON cl.id = f.class_id`, conditions, "cl.id, f.id", f)

	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query resource_families: %w", err)
	}
	defer rows.Close()
	var out []domain.CatalogRecord
	for rows.Next() {
		var id, classID int64
		var classCode, className, code, name string
		var active bool
		var revision uint64
		if err := rows.Scan(&id, &classID, &classCode, &className, &code, &name, &active, &revision); err != nil {
			return nil, fmt.Errorf("scan resource_families: %w", err)
		}
		out = append(out, catalogRecordWithRevision(familyRecord(id, classID, classCode, className, code, name, active), revision))
	}
	return out, rows.Err()
}

func getFamily(ctx context.Context, q querier, id int64) (domain.CatalogRecord, error) {
	var classID int64
	var classCode, className, code, name string
	var active bool
	var revision uint64
	err := q.QueryRow(ctx, `
		SELECT cl.id, cl.code, cl.name, f.code, f.name, f.active, f.revision
		FROM public.resource_families f JOIN public.resource_classes cl ON cl.id = f.class_id
		WHERE f.id=$1`, id).Scan(&classID, &classCode, &className, &code, &name, &active, &revision)
	if isNoRows(err) {
		return domain.CatalogRecord{}, domain.ErrCatalogRecordNotFound
	}
	if err != nil {
		return domain.CatalogRecord{}, fmt.Errorf("get resource_families: %w", err)
	}
	return catalogRecordWithRevision(familyRecord(id, classID, classCode, className, code, name, active), revision), nil
}

func familyRecord(id, classID int64, classCode, className, code, name string, active bool) domain.CatalogRecord {
	return domain.CatalogRecord{Kind: domain.KindFamily, ID: id, Active: active, Values: map[string]domain.CatalogValue{
		"class": refValueLabel(domain.KindClass, classCode, className, classID), "code": textValue(code), "name": textValue(name),
	}}
}

func insertFamily(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `
		INSERT INTO public.resource_families (class_id, code, name, active)
		SELECT id, $2, $3, $4 FROM public.resource_classes WHERE code=$1
		RETURNING id`, fieldRef(rec, "class"), fieldText(rec, "code"), fieldText(rec, "name"), rec.Active).Scan(&id)
	if isNoRows(err) {
		return 0, fmt.Errorf("%w: class %q", domain.ErrCatalogReference, fieldRef(rec, "class"))
	}
	if err != nil {
		return 0, mapCatalogWriteError(fmt.Errorf("insert resource_families: %w", err))
	}
	return id, nil
}

func updateFamily(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) error {
	classID, err := resolveClassID(ctx, tx, fieldRef(rec, "class"))
	if err != nil {
		return err
	}
	referenced, err := familyReferencedByResources(ctx, tx, rec.ID)
	if err != nil {
		return err
	}
	var tag pgconn.CommandTag
	if referenced {
		tag, err = tx.Exec(ctx, `UPDATE public.resource_families SET class_id=$1, name=$2, active=$3 WHERE id=$4`, classID, fieldText(rec, "name"), rec.Active, rec.ID)
	} else {
		tag, err = tx.Exec(ctx, `UPDATE public.resource_families SET class_id=$1, code=$2, name=$3, active=$4 WHERE id=$5`, classID, fieldText(rec, "code"), fieldText(rec, "name"), rec.Active, rec.ID)
	}
	if err != nil {
		return mapCatalogWriteError(fmt.Errorf("update resource_families: %w", err))
	}
	return requireRow(tag.RowsAffected())
}

func familyReferencedByResources(ctx context.Context, q querier, id int64) (bool, error) {
	var exists bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.recursos WHERE family_id=$1)`, id).Scan(&exists); err != nil {
		return false, fmt.Errorf("check family referenced by resources: %w", err)
	}
	return exists, nil
}

// --- KindType -----------------------------------------------------------

func listTypes(ctx context.Context, q querier, f domain.CatalogFilter) ([]domain.CatalogRecord, error) {
	var conditions []string
	var args []any
	conditions, args = appendTextFilter(conditions, args, f.Text, "t.code", "t.name", "cl.name", "f.name")
	conditions = appendActiveFilter(conditions, f.Status, "t.active")
	conditions, args = appendEqualsFilter(conditions, args, "cl.code", parentRefCode(f, "class"))
	conditions, args = appendEqualsFilter(conditions, args, "f.code", parentRefCode(f, "family"))
	sql := finalizeQuery(`
		SELECT t.id, cl.id, cl.code, cl.name, f.id, f.code, f.name, t.code, t.name, t.active, t.revision
		FROM public.resource_types t
		JOIN public.resource_families f ON f.id = t.family_id
		JOIN public.resource_classes cl ON cl.id = t.class_id`, conditions, "f.id, t.id", f)

	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query resource_types: %w", err)
	}
	defer rows.Close()
	var out []domain.CatalogRecord
	for rows.Next() {
		var id, classID, familyID int64
		var classCode, className, familyCode, familyName, code, name string
		var active bool
		var revision uint64
		if err := rows.Scan(&id, &classID, &classCode, &className, &familyID, &familyCode, &familyName, &code, &name, &active, &revision); err != nil {
			return nil, fmt.Errorf("scan resource_types: %w", err)
		}
		out = append(out, catalogRecordWithRevision(typeRecord(id, classID, classCode, className, familyID, familyCode, familyName, code, name, active), revision))
	}
	return out, rows.Err()
}

func getType(ctx context.Context, q querier, id int64) (domain.CatalogRecord, error) {
	var classID, familyID int64
	var classCode, className, familyCode, familyName, code, name string
	var active bool
	var revision uint64
	err := q.QueryRow(ctx, `
		SELECT cl.id, cl.code, cl.name, f.id, f.code, f.name, t.code, t.name, t.active, t.revision
		FROM public.resource_types t
		JOIN public.resource_families f ON f.id = t.family_id
		JOIN public.resource_classes cl ON cl.id = t.class_id
		WHERE t.id=$1`, id).Scan(&classID, &classCode, &className, &familyID, &familyCode, &familyName, &code, &name, &active, &revision)
	if isNoRows(err) {
		return domain.CatalogRecord{}, domain.ErrCatalogRecordNotFound
	}
	if err != nil {
		return domain.CatalogRecord{}, fmt.Errorf("get resource_types: %w", err)
	}
	return catalogRecordWithRevision(typeRecord(id, classID, classCode, className, familyID, familyCode, familyName, code, name, active), revision), nil
}

func typeRecord(id, classID int64, classCode, className string, familyID int64, familyCode, familyName, code, name string, active bool) domain.CatalogRecord {
	return domain.CatalogRecord{Kind: domain.KindType, ID: id, Active: active, Values: map[string]domain.CatalogValue{
		"class": refValueLabel(domain.KindClass, classCode, className, classID), "family": refValueLabel(domain.KindFamily, familyCode, familyName, familyID),
		"code": textValue(code), "name": textValue(name),
	}}
}

func insertType(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) (int64, error) {
	familyID, classID, err := resolveFamilyID(ctx, tx, fieldRef(rec, "class"), fieldRef(rec, "family"))
	if err != nil {
		return 0, err
	}
	var id int64
	err = tx.QueryRow(ctx, `INSERT INTO public.resource_types (class_id, family_id, code, name, active) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		classID, familyID, fieldText(rec, "code"), fieldText(rec, "name"), rec.Active).Scan(&id)
	if err != nil {
		return 0, mapCatalogWriteError(fmt.Errorf("insert resource_types: %w", err))
	}
	return id, nil
}

func updateType(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) error {
	familyID, classID, err := resolveFamilyID(ctx, tx, fieldRef(rec, "class"), fieldRef(rec, "family"))
	if err != nil {
		return err
	}
	referenced, err := typeReferencedByResources(ctx, tx, rec.ID)
	if err != nil {
		return err
	}
	var tag pgconn.CommandTag
	if referenced {
		tag, err = tx.Exec(ctx, `UPDATE public.resource_types SET class_id=$1, family_id=$2, name=$3, active=$4 WHERE id=$5`, classID, familyID, fieldText(rec, "name"), rec.Active, rec.ID)
	} else {
		tag, err = tx.Exec(ctx, `UPDATE public.resource_types SET class_id=$1, family_id=$2, code=$3, name=$4, active=$5 WHERE id=$6`, classID, familyID, fieldText(rec, "code"), fieldText(rec, "name"), rec.Active, rec.ID)
	}
	if err != nil {
		return mapCatalogWriteError(fmt.Errorf("update resource_types: %w", err))
	}
	return requireRow(tag.RowsAffected())
}

func typeReferencedByResources(ctx context.Context, q querier, id int64) (bool, error) {
	var exists bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.recursos WHERE type_id=$1)`, id).Scan(&exists); err != nil {
		return false, fmt.Errorf("check type referenced by resources: %w", err)
	}
	return exists, nil
}

// --- KindAttributeDefinition ("Característica") ------------------------

func listDefinitions(ctx context.Context, q querier, f domain.CatalogFilter) ([]domain.CatalogRecord, error) {
	var conditions []string
	var args []any
	conditions, args = appendTextFilter(conditions, args, f.Text, "code", "name")
	conditions = appendActiveFilter(conditions, f.Status, "active")
	sql := finalizeQuery(`SELECT id, code, name, value_type, dimension, default_identity_participates, active, revision FROM public.attribute_definitions`, conditions, "id", f)

	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query attribute_definitions: %w", err)
	}
	defer rows.Close()
	var out []domain.CatalogRecord
	for rows.Next() {
		var id int64
		var code, name, valueType string
		var dimension *string
		var defaultIdentity, active bool
		var revision uint64
		if err := rows.Scan(&id, &code, &name, &valueType, &dimension, &defaultIdentity, &active, &revision); err != nil {
			return nil, fmt.Errorf("scan attribute_definitions: %w", err)
		}
		out = append(out, catalogRecordWithRevision(definitionRecord(id, code, name, valueType, dereference(dimension), defaultIdentity, active), revision))
	}
	return out, rows.Err()
}

func getDefinition(ctx context.Context, q querier, id int64) (domain.CatalogRecord, error) {
	var code, name, valueType string
	var dimension *string
	var defaultIdentity, active bool
	var revision uint64
	err := q.QueryRow(ctx, `SELECT code, name, value_type, dimension, default_identity_participates, active, revision FROM public.attribute_definitions WHERE id=$1`, id).
		Scan(&code, &name, &valueType, &dimension, &defaultIdentity, &active, &revision)
	if isNoRows(err) {
		return domain.CatalogRecord{}, domain.ErrCatalogRecordNotFound
	}
	if err != nil {
		return domain.CatalogRecord{}, fmt.Errorf("get attribute_definitions: %w", err)
	}
	return catalogRecordWithRevision(definitionRecord(id, code, name, valueType, dereference(dimension), defaultIdentity, active), revision), nil
}

func definitionRecord(id int64, code, name, valueType, dimension string, defaultIdentity, active bool) domain.CatalogRecord {
	return domain.CatalogRecord{Kind: domain.KindAttributeDefinition, ID: id, Active: active, Values: map[string]domain.CatalogValue{
		"code": textValue(code), "name": textValue(name), "valueType": textValue(valueType),
		"dimension": textValue(dimension), "defaultIdentityParticipates": boolValue(defaultIdentity),
	}}
}

func insertDefinition(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `
		INSERT INTO public.attribute_definitions (code, name, value_type, dimension, default_identity_participates, active)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		fieldText(rec, "code"), fieldText(rec, "name"), fieldText(rec, "valueType"),
		nullableString(fieldText(rec, "dimension")), fieldBool(rec, "defaultIdentityParticipates"), rec.Active).Scan(&id)
	if err != nil {
		return 0, mapCatalogWriteError(fmt.Errorf("insert attribute_definitions: %w", err))
	}
	return id, nil
}

func updateDefinition(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) error {
	referenced, err := definitionReferencedByResources(ctx, tx, rec.ID)
	if err != nil {
		return err
	}
	var tag pgconn.CommandTag
	if referenced {
		tag, err = tx.Exec(ctx, `UPDATE public.attribute_definitions SET name=$1, value_type=$2, dimension=$3, default_identity_participates=$4, active=$5 WHERE id=$6`,
			fieldText(rec, "name"), fieldText(rec, "valueType"), nullableString(fieldText(rec, "dimension")), fieldBool(rec, "defaultIdentityParticipates"), rec.Active, rec.ID)
	} else {
		tag, err = tx.Exec(ctx, `UPDATE public.attribute_definitions SET code=$1, name=$2, value_type=$3, dimension=$4, default_identity_participates=$5, active=$6 WHERE id=$7`,
			fieldText(rec, "code"), fieldText(rec, "name"), fieldText(rec, "valueType"), nullableString(fieldText(rec, "dimension")), fieldBool(rec, "defaultIdentityParticipates"), rec.Active, rec.ID)
	}
	if err != nil {
		return mapCatalogWriteError(fmt.Errorf("update attribute_definitions: %w", err))
	}
	return requireRow(tag.RowsAffected())
}

func definitionReferencedByResources(ctx context.Context, q querier, id int64) (bool, error) {
	var exists bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.resource_attribute_values WHERE attribute_definition_id=$1)`, id).Scan(&exists); err != nil {
		return false, fmt.Errorf("check characteristic referenced by resources: %w", err)
	}
	return exists, nil
}

// --- KindOptionSet ("Conjunto de Opciones") ------------------------------
//
// resource_option_sets' natural PK is code itself; migration 000009 added a
// real BIGSERIAL id column (UNIQUE, additive) so every kind addresses
// consistently by id, never by a hashed natural key.

func listOptionSets(ctx context.Context, q querier, f domain.CatalogFilter) ([]domain.CatalogRecord, error) {
	var conditions []string
	var args []any
	conditions, args = appendTextFilter(conditions, args, f.Text, "code", "name")
	conditions = appendActiveFilter(conditions, f.Status, "active")
	sql := finalizeQuery(`SELECT id, code, name, active, revision FROM public.resource_option_sets`, conditions, "code", f)

	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query resource_option_sets: %w", err)
	}
	defer rows.Close()
	var out []domain.CatalogRecord
	for rows.Next() {
		var id int64
		var code, name string
		var active bool
		var revision uint64
		if err := rows.Scan(&id, &code, &name, &active, &revision); err != nil {
			return nil, fmt.Errorf("scan resource_option_sets: %w", err)
		}
		out = append(out, catalogRecordWithRevision(optionSetRecord(id, code, name, active), revision))
	}
	return out, rows.Err()
}

func getOptionSet(ctx context.Context, q querier, id int64) (domain.CatalogRecord, error) {
	var code, name string
	var active bool
	var revision uint64
	err := q.QueryRow(ctx, `SELECT code, name, active, revision FROM public.resource_option_sets WHERE id = $1`, id).Scan(&code, &name, &active, &revision)
	if isNoRows(err) {
		return domain.CatalogRecord{}, domain.ErrCatalogRecordNotFound
	}
	if err != nil {
		return domain.CatalogRecord{}, fmt.Errorf("get resource_option_sets: %w", err)
	}
	return catalogRecordWithRevision(optionSetRecord(id, code, name, active), revision), nil
}

func optionSetRecord(id int64, code, name string, active bool) domain.CatalogRecord {
	return domain.CatalogRecord{Kind: domain.KindOptionSet, ID: id, Active: active, Values: map[string]domain.CatalogValue{
		"code": textValue(code), "name": textValue(name),
	}}
}

func insertOptionSet(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `
		INSERT INTO public.resource_option_sets (code, name, active) VALUES ($1,$2,$3)
		RETURNING id`, fieldText(rec, "code"), fieldText(rec, "name"), rec.Active).Scan(&id)
	if err != nil {
		return 0, mapCatalogWriteError(fmt.Errorf("insert resource_option_sets: %w", err))
	}
	return id, nil
}

func updateOptionSet(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) error {
	var currentCode string
	err := tx.QueryRow(ctx, `SELECT code FROM public.resource_option_sets WHERE id = $1`, rec.ID).Scan(&currentCode)
	if isNoRows(err) {
		return domain.ErrCatalogRecordNotFound
	}
	if err != nil {
		return fmt.Errorf("resolve resource_option_sets: %w", err)
	}
	referenced, err := optionSetReferencedByResources(ctx, tx, rec.ID)
	if err != nil {
		return err
	}
	var tag pgconn.CommandTag
	if referenced {
		tag, err = tx.Exec(ctx, `UPDATE public.resource_option_sets SET name=$1, active=$2 WHERE code=$3`, fieldText(rec, "name"), rec.Active, currentCode)
	} else {
		tag, err = tx.Exec(ctx, `UPDATE public.resource_option_sets SET code=$1, name=$2, active=$3 WHERE code=$4`, fieldText(rec, "code"), fieldText(rec, "name"), rec.Active, currentCode)
	}
	if err != nil {
		return mapCatalogWriteError(fmt.Errorf("update resource_option_sets: %w", err))
	}
	return requireRow(tag.RowsAffected())
}

func setActiveOptionSet(ctx context.Context, tx pgx.Tx, id int64, active bool) error {
	tag, err := tx.Exec(ctx, `UPDATE public.resource_option_sets SET active=$1 WHERE id = $2`, active, id)
	if err != nil {
		return mapCatalogWriteError(fmt.Errorf("set active on resource_option_sets: %w", err))
	}
	return requireRow(tag.RowsAffected())
}

func deleteOptionSet(ctx context.Context, tx pgx.Tx, id int64) error {
	tag, err := tx.Exec(ctx, `DELETE FROM public.resource_option_sets WHERE id = $1`, id)
	if err != nil {
		return mapCatalogDeleteError(fmt.Errorf("delete from resource_option_sets: %w", err))
	}
	return requireRow(tag.RowsAffected())
}

// optionSetReferencedByResources closes a real gap design §6's own probe
// table left out: resource_attribute_values.option_set is a live FK to
// resource_option_sets(code) (migration 000002), so an OptionSet's código
// IS referenced by resource instances, exactly like the 6 kinds design's
// table already lists — added here for the same Código-immutability
// enforcement, flagged for sdd-verify same as PR3's own gap-fills.
func optionSetReferencedByResources(ctx context.Context, q querier, id int64) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM public.resource_option_sets os
			JOIN public.resource_attribute_values v ON v.option_set = os.code
			WHERE os.id = $1
		)`, id).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check option set referenced by resources: %w", err)
	}
	return exists, nil
}

// --- KindOption -----------------------------------------------------------
//
// attribute_options' natural PK is (option_set, attribute_definition_id,
// code); migration 000009 added a real BIGSERIAL id column (UNIQUE,
// additive) so this kind addresses by id like every other kind.

func listOptions(ctx context.Context, q querier, f domain.CatalogFilter) ([]domain.CatalogRecord, error) {
	var conditions []string
	var args []any
	conditions, args = appendTextFilter(conditions, args, f.Text, "ao.code", "ao.label")
	conditions = appendActiveFilter(conditions, f.Status, "ao.active")
	conditions, args = appendEqualsFilter(conditions, args, "ao.option_set", parentRefCode(f, "optionSet"))
	conditions, args = appendEqualsFilter(conditions, args, "d.code", parentRefCode(f, "characteristic"))
	sql := finalizeQuery(`
		SELECT ao.id, os.id, ao.option_set, d.id, d.code, ao.code, ao.label, ao.active, ao.revision
		FROM public.attribute_options ao
		JOIN public.attribute_definitions d ON d.id = ao.attribute_definition_id
		JOIN public.resource_option_sets os ON os.code = ao.option_set`, conditions, "d.id, ao.display_order", f)

	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query attribute_options: %w", err)
	}
	defer rows.Close()
	var out []domain.CatalogRecord
	for rows.Next() {
		var id, optionSetID, definitionID int64
		var optionSet, characteristic, code, label string
		var active bool
		var revision uint64
		if err := rows.Scan(&id, &optionSetID, &optionSet, &definitionID, &characteristic, &code, &label, &active, &revision); err != nil {
			return nil, fmt.Errorf("scan attribute_options: %w", err)
		}
		out = append(out, catalogRecordWithRevision(optionRecord(id, optionSetID, optionSet, definitionID, characteristic, code, label, active), revision))
	}
	return out, rows.Err()
}

func getOption(ctx context.Context, q querier, id int64) (domain.CatalogRecord, error) {
	var optionSetID, definitionID int64
	var optionSet, characteristic, code, label string
	var active bool
	var revision uint64
	err := q.QueryRow(ctx, `
		SELECT os.id, ao.option_set, d.id, d.code, ao.code, ao.label, ao.active, ao.revision
		FROM public.attribute_options ao
		JOIN public.attribute_definitions d ON d.id = ao.attribute_definition_id
		JOIN public.resource_option_sets os ON os.code = ao.option_set
		WHERE ao.id = $1`, id).
		Scan(&optionSetID, &optionSet, &definitionID, &characteristic, &code, &label, &active, &revision)
	if isNoRows(err) {
		return domain.CatalogRecord{}, domain.ErrCatalogRecordNotFound
	}
	if err != nil {
		return domain.CatalogRecord{}, fmt.Errorf("get attribute_options: %w", err)
	}
	return catalogRecordWithRevision(optionRecord(id, optionSetID, optionSet, definitionID, characteristic, code, label, active), revision), nil
}

func optionRecord(id, optionSetID int64, optionSet string, definitionID int64, characteristic, code, label string, active bool) domain.CatalogRecord {
	return domain.CatalogRecord{Kind: domain.KindOption, ID: id, Active: active, Values: map[string]domain.CatalogValue{
		"optionSet": refValue(domain.KindOptionSet, optionSet, optionSetID), "characteristic": refValue(domain.KindAttributeDefinition, characteristic, definitionID),
		"code": textValue(code), "label": textValue(label),
	}}
}

func insertOption(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `
		INSERT INTO public.attribute_options (option_set, attribute_definition_id, code, label, active, display_order)
		SELECT $1, d.id, $3, $4, $5,
		       COALESCE((SELECT MAX(display_order)+1 FROM public.attribute_options WHERE option_set=$1 AND attribute_definition_id=d.id), 0)
		FROM public.attribute_definitions d WHERE d.code=$2
		RETURNING id`,
		fieldRef(rec, "optionSet"), fieldRef(rec, "characteristic"), fieldText(rec, "code"), fieldText(rec, "label"), rec.Active).Scan(&id)
	if isNoRows(err) {
		return 0, fmt.Errorf("%w: characteristic %q or option set %q", domain.ErrCatalogReference, fieldRef(rec, "characteristic"), fieldRef(rec, "optionSet"))
	}
	if err != nil {
		return 0, mapCatalogWriteError(fmt.Errorf("insert attribute_options: %w", err))
	}
	return id, nil
}

func updateOption(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) error {
	var optionSet, defCode, code string
	err := tx.QueryRow(ctx, `
		SELECT ao.option_set, d.code, ao.code
		FROM public.attribute_options ao JOIN public.attribute_definitions d ON d.id = ao.attribute_definition_id
		WHERE ao.id = $1`, rec.ID).Scan(&optionSet, &defCode, &code)
	if isNoRows(err) {
		return domain.ErrCatalogRecordNotFound
	}
	if err != nil {
		return fmt.Errorf("resolve attribute_options: %w", err)
	}
	referenced, err := optionReferencedByResources(ctx, tx, rec.ID)
	if err != nil {
		return err
	}
	var tag pgconn.CommandTag
	if referenced {
		tag, err = tx.Exec(ctx, `
			UPDATE public.attribute_options SET label=$1, active=$2
			WHERE option_set=$3 AND attribute_definition_id=(SELECT id FROM public.attribute_definitions WHERE code=$4) AND code=$5`,
			fieldText(rec, "label"), rec.Active, optionSet, defCode, code)
	} else {
		tag, err = tx.Exec(ctx, `
			UPDATE public.attribute_options SET code=$1, label=$2, active=$3
			WHERE option_set=$4 AND attribute_definition_id=(SELECT id FROM public.attribute_definitions WHERE code=$5) AND code=$6`,
			fieldText(rec, "code"), fieldText(rec, "label"), rec.Active, optionSet, defCode, code)
	}
	if err != nil {
		return mapCatalogWriteError(fmt.Errorf("update attribute_options: %w", err))
	}
	return requireRow(tag.RowsAffected())
}

func setActiveOption(ctx context.Context, tx pgx.Tx, id int64, active bool) error {
	tag, err := tx.Exec(ctx, `UPDATE public.attribute_options SET active=$1 WHERE id = $2`, active, id)
	if err != nil {
		return mapCatalogWriteError(fmt.Errorf("set active on attribute_options: %w", err))
	}
	return requireRow(tag.RowsAffected())
}

func deleteOption(ctx context.Context, tx pgx.Tx, id int64) error {
	tag, err := tx.Exec(ctx, `DELETE FROM public.attribute_options WHERE id = $1`, id)
	if err != nil {
		return mapCatalogDeleteError(fmt.Errorf("delete from attribute_options: %w", err))
	}
	return requireRow(tag.RowsAffected())
}

func optionReferencedByResources(ctx context.Context, q querier, id int64) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM public.attribute_options ao
			JOIN public.resource_attribute_values v ON v.option_set = ao.option_set
				AND v.attribute_definition_id = ao.attribute_definition_id AND v.option_code = ao.code
			WHERE ao.id = $1
		)`, id).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check option referenced by resources: %w", err)
	}
	return exists, nil
}

// --- KindOptionRelation ---------------------------------------------------

func listOptionRelations(ctx context.Context, q querier, f domain.CatalogFilter) ([]domain.CatalogRecord, error) {
	var conditions []string
	var args []any
	conditions = appendActiveFilter(conditions, f.Status, "ar.active")
	conditions, args = appendEqualsFilter(conditions, args, "ar.option_set", parentRefCode(f, "optionSet"))
	sql := finalizeQuery(`
		SELECT ar.id, os.id, ar.option_set, fd.id, fd.code, fo.id, ar.from_option_code, td.id, td.code, too.id, ar.to_option_code, ar.active, ar.revision
		FROM public.attribute_option_relations ar
		JOIN public.resource_option_sets os ON os.code = ar.option_set
		JOIN public.attribute_definitions fd ON fd.id = ar.from_attribute_definition_id
		JOIN public.attribute_definitions td ON td.id = ar.to_attribute_definition_id
		JOIN public.attribute_options fo ON fo.option_set = ar.option_set AND fo.attribute_definition_id = fd.id AND fo.code = ar.from_option_code
		JOIN public.attribute_options too ON too.option_set = ar.option_set AND too.attribute_definition_id = td.id AND too.code = ar.to_option_code`, conditions, "ar.id", f)

	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query attribute_option_relations: %w", err)
	}
	defer rows.Close()
	var out []domain.CatalogRecord
	for rows.Next() {
		var id, optionSetID, fromDefinitionID, fromOptionID, toDefinitionID, toOptionID int64
		var optionSet, fromCharacteristic, fromOption, toCharacteristic, toOption string
		var active bool
		var revision uint64
		if err := rows.Scan(&id, &optionSetID, &optionSet, &fromDefinitionID, &fromCharacteristic, &fromOptionID, &fromOption,
			&toDefinitionID, &toCharacteristic, &toOptionID, &toOption, &active, &revision); err != nil {
			return nil, fmt.Errorf("scan attribute_option_relations: %w", err)
		}
		out = append(out, catalogRecordWithRevision(optionRelationRecord(id, optionSetID, optionSet, fromDefinitionID, fromCharacteristic, fromOptionID, fromOption,
			toDefinitionID, toCharacteristic, toOptionID, toOption, active), revision))
	}
	return out, rows.Err()
}

func getOptionRelation(ctx context.Context, q querier, id int64) (domain.CatalogRecord, error) {
	var optionSetID, fromDefinitionID, fromOptionID, toDefinitionID, toOptionID int64
	var optionSet, fromCharacteristic, fromOption, toCharacteristic, toOption string
	var active bool
	var revision uint64
	err := q.QueryRow(ctx, `
		SELECT os.id, ar.option_set, fd.id, fd.code, fo.id, ar.from_option_code, td.id, td.code, too.id, ar.to_option_code, ar.active, ar.revision
		FROM public.attribute_option_relations ar
		JOIN public.resource_option_sets os ON os.code = ar.option_set
		JOIN public.attribute_definitions fd ON fd.id = ar.from_attribute_definition_id
		JOIN public.attribute_definitions td ON td.id = ar.to_attribute_definition_id
		JOIN public.attribute_options fo ON fo.option_set = ar.option_set AND fo.attribute_definition_id = fd.id AND fo.code = ar.from_option_code
		JOIN public.attribute_options too ON too.option_set = ar.option_set AND too.attribute_definition_id = td.id AND too.code = ar.to_option_code
		WHERE ar.id=$1`, id).Scan(&optionSetID, &optionSet, &fromDefinitionID, &fromCharacteristic, &fromOptionID, &fromOption,
		&toDefinitionID, &toCharacteristic, &toOptionID, &toOption, &active, &revision)
	if isNoRows(err) {
		return domain.CatalogRecord{}, domain.ErrCatalogRecordNotFound
	}
	if err != nil {
		return domain.CatalogRecord{}, fmt.Errorf("get attribute_option_relations: %w", err)
	}
	return catalogRecordWithRevision(optionRelationRecord(id, optionSetID, optionSet, fromDefinitionID, fromCharacteristic, fromOptionID, fromOption,
		toDefinitionID, toCharacteristic, toOptionID, toOption, active), revision), nil
}

func optionRelationRecord(id, optionSetID int64, optionSet string, fromDefinitionID int64, fromCharacteristic string, fromOptionID int64, fromOption string,
	toDefinitionID int64, toCharacteristic string, toOptionID int64, toOption string, active bool) domain.CatalogRecord {
	return domain.CatalogRecord{Kind: domain.KindOptionRelation, ID: id, Active: active, Values: map[string]domain.CatalogValue{
		"optionSet":          refValue(domain.KindOptionSet, optionSet, optionSetID),
		"fromCharacteristic": refValue(domain.KindAttributeDefinition, fromCharacteristic, fromDefinitionID),
		"fromOption":         refValue(domain.KindOption, fromOption, fromOptionID),
		"toCharacteristic":   refValue(domain.KindAttributeDefinition, toCharacteristic, toDefinitionID),
		"toOption":           refValue(domain.KindOption, toOption, toOptionID),
	}}
}

func insertOptionRelation(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `
		INSERT INTO public.attribute_option_relations (option_set, from_attribute_definition_id, from_option_code, to_attribute_definition_id, to_option_code, active)
		SELECT $1, fd.id, $3, td.id, $5, $6
		FROM public.attribute_definitions fd, public.attribute_definitions td
		WHERE fd.code=$2 AND td.code=$4
		RETURNING id`,
		fieldRef(rec, "optionSet"), fieldRef(rec, "fromCharacteristic"), fieldRef(rec, "fromOption"),
		fieldRef(rec, "toCharacteristic"), fieldRef(rec, "toOption"), rec.Active).Scan(&id)
	if isNoRows(err) {
		return 0, fmt.Errorf("%w: characteristic %q or %q", domain.ErrCatalogReference, fieldRef(rec, "fromCharacteristic"), fieldRef(rec, "toCharacteristic"))
	}
	if err != nil {
		return 0, mapCatalogWriteError(fmt.Errorf("insert attribute_option_relations: %w", err))
	}
	return id, nil
}

func updateOptionRelation(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) error {
	tag, err := tx.Exec(ctx, `
		UPDATE public.attribute_option_relations
		SET option_set=$1,
		    from_attribute_definition_id=(SELECT id FROM public.attribute_definitions WHERE code=$2), from_option_code=$3,
		    to_attribute_definition_id=(SELECT id FROM public.attribute_definitions WHERE code=$4), to_option_code=$5,
		    active=$6
		WHERE id=$7`,
		fieldRef(rec, "optionSet"), fieldRef(rec, "fromCharacteristic"), fieldRef(rec, "fromOption"),
		fieldRef(rec, "toCharacteristic"), fieldRef(rec, "toOption"), rec.Active, rec.ID)
	if err != nil {
		return mapCatalogWriteError(fmt.Errorf("update attribute_option_relations: %w", err))
	}
	return requireRow(tag.RowsAffected())
}

// --- KindUnit ---------------------------------------------------------

func listUnits(ctx context.Context, q querier, f domain.CatalogFilter) ([]domain.CatalogRecord, error) {
	var conditions []string
	var args []any
	conditions, args = appendTextFilter(conditions, args, f.Text, "code", "name", "symbol", "dimension")
	conditions = appendActiveFilter(conditions, f.Status, "active")
	sql := finalizeQuery(`SELECT id, code, name, symbol, dimension, active, revision FROM public.unit_definitions`, conditions, "id", f)

	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query unit_definitions: %w", err)
	}
	defer rows.Close()
	var out []domain.CatalogRecord
	for rows.Next() {
		var id int64
		var code, name, symbol, dimension string
		var active bool
		var revision uint64
		if err := rows.Scan(&id, &code, &name, &symbol, &dimension, &active, &revision); err != nil {
			return nil, fmt.Errorf("scan unit_definitions: %w", err)
		}
		out = append(out, catalogRecordWithRevision(unitRecord(id, code, name, symbol, dimension, active), revision))
	}
	return out, rows.Err()
}

func getUnit(ctx context.Context, q querier, id int64) (domain.CatalogRecord, error) {
	var code, name, symbol, dimension string
	var active bool
	var revision uint64
	err := q.QueryRow(ctx, `SELECT code, name, symbol, dimension, active, revision FROM public.unit_definitions WHERE id=$1`, id).Scan(&code, &name, &symbol, &dimension, &active, &revision)
	if isNoRows(err) {
		return domain.CatalogRecord{}, domain.ErrCatalogRecordNotFound
	}
	if err != nil {
		return domain.CatalogRecord{}, fmt.Errorf("get unit_definitions: %w", err)
	}
	return catalogRecordWithRevision(unitRecord(id, code, name, symbol, dimension, active), revision), nil
}

func unitRecord(id int64, code, name, symbol, dimension string, active bool) domain.CatalogRecord {
	return domain.CatalogRecord{Kind: domain.KindUnit, ID: id, Active: active, Values: map[string]domain.CatalogValue{
		"code": textValue(code), "name": textValue(name), "symbol": textValue(symbol), "dimension": textValue(dimension),
	}}
}

func insertUnit(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `INSERT INTO public.unit_definitions (code, name, symbol, dimension, active) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		fieldText(rec, "code"), fieldText(rec, "name"), fieldText(rec, "symbol"), fieldText(rec, "dimension"), rec.Active).Scan(&id)
	if err != nil {
		return 0, mapCatalogWriteError(fmt.Errorf("insert unit_definitions: %w", err))
	}
	return id, nil
}

func updateUnit(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) error {
	referenced, err := unitReferencedByResources(ctx, tx, rec.ID)
	if err != nil {
		return err
	}
	var tag pgconn.CommandTag
	if referenced {
		tag, err = tx.Exec(ctx, `UPDATE public.unit_definitions SET name=$1, symbol=$2, dimension=$3, active=$4 WHERE id=$5`, fieldText(rec, "name"), fieldText(rec, "symbol"), fieldText(rec, "dimension"), rec.Active, rec.ID)
	} else {
		tag, err = tx.Exec(ctx, `UPDATE public.unit_definitions SET code=$1, name=$2, symbol=$3, dimension=$4, active=$5 WHERE id=$6`, fieldText(rec, "code"), fieldText(rec, "name"), fieldText(rec, "symbol"), fieldText(rec, "dimension"), rec.Active, rec.ID)
	}
	if err != nil {
		return mapCatalogWriteError(fmt.Errorf("update unit_definitions: %w", err))
	}
	return requireRow(tag.RowsAffected())
}

func unitReferencedByResources(ctx context.Context, q querier, id int64) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM public.recursos WHERE natural_unit_id=$1)
		    OR EXISTS(SELECT 1 FROM public.resource_attribute_values WHERE quantity_unit_id=$1)`, id).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check unit referenced by resources: %w", err)
	}
	return exists, nil
}

// --- KindUnitPolicy ("Política de Unidad") -------------------------------
//
// resource_unit_policies' natural PK is (family_id, unit_id); migration
// 000009 added a real BIGSERIAL id column (UNIQUE, additive).

func listUnitPolicies(ctx context.Context, q querier, f domain.CatalogFilter) ([]domain.CatalogRecord, error) {
	var conditions []string
	var args []any
	conditions = appendActiveFilter(conditions, f.Status, "p.active")
	conditions, args = appendEqualsFilter(conditions, args, "cl.code", parentRefCode(f, "class"))
	conditions, args = appendEqualsFilter(conditions, args, "f.code", parentRefCode(f, "family"))
	sql := finalizeQuery(`
		SELECT p.id, cl.id, cl.code, f.id, f.code, u.id, u.code, p.allowed, p.suggested, p.active, p.revision
		FROM public.resource_unit_policies p
		JOIN public.resource_families f ON f.id = p.family_id
		JOIN public.resource_classes cl ON cl.id = f.class_id
		JOIN public.unit_definitions u ON u.id = p.unit_id`, conditions, "f.id, u.id", f)

	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query resource_unit_policies: %w", err)
	}
	defer rows.Close()
	var out []domain.CatalogRecord
	for rows.Next() {
		var id, classID, familyID, unitID int64
		var classCode, familyCode, unitCode string
		var allowed, suggested, active bool
		var revision uint64
		if err := rows.Scan(&id, &classID, &classCode, &familyID, &familyCode, &unitID, &unitCode, &allowed, &suggested, &active, &revision); err != nil {
			return nil, fmt.Errorf("scan resource_unit_policies: %w", err)
		}
		out = append(out, catalogRecordWithRevision(unitPolicyRecord(id, classID, classCode, familyID, familyCode, unitID, unitCode, allowed, suggested, active), revision))
	}
	return out, rows.Err()
}

func getUnitPolicy(ctx context.Context, q querier, id int64) (domain.CatalogRecord, error) {
	var classID, familyID, unitID int64
	var classCode, familyCode, unitCode string
	var allowed, suggested, active bool
	var revision uint64
	err := q.QueryRow(ctx, `
		SELECT cl.id, cl.code, f.id, f.code, u.id, u.code, p.allowed, p.suggested, p.active, p.revision
		FROM public.resource_unit_policies p
		JOIN public.resource_families f ON f.id = p.family_id
		JOIN public.resource_classes cl ON cl.id = f.class_id
		JOIN public.unit_definitions u ON u.id = p.unit_id
		WHERE p.id = $1`, id).
		Scan(&classID, &classCode, &familyID, &familyCode, &unitID, &unitCode, &allowed, &suggested, &active, &revision)
	if isNoRows(err) {
		return domain.CatalogRecord{}, domain.ErrCatalogRecordNotFound
	}
	if err != nil {
		return domain.CatalogRecord{}, fmt.Errorf("get resource_unit_policies: %w", err)
	}
	return catalogRecordWithRevision(unitPolicyRecord(id, classID, classCode, familyID, familyCode, unitID, unitCode, allowed, suggested, active), revision), nil
}

func unitPolicyRecord(id, classID int64, classCode string, familyID int64, familyCode string, unitID int64, unitCode string, allowed, suggested, active bool) domain.CatalogRecord {
	return domain.CatalogRecord{Kind: domain.KindUnitPolicy, ID: id, Active: active, Values: map[string]domain.CatalogValue{
		"class": refValue(domain.KindClass, classCode, classID), "family": refValue(domain.KindFamily, familyCode, familyID), "unit": refValue(domain.KindUnit, unitCode, unitID),
		"allowed": boolValue(allowed), "suggested": boolValue(suggested),
	}}
}

func insertUnitPolicy(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `
		INSERT INTO public.resource_unit_policies (family_id, unit_id, allowed, suggested, active)
		SELECT f.id, u.id, $4, $5, $6
		FROM public.resource_families f JOIN public.resource_classes cl ON cl.id = f.class_id, public.unit_definitions u
		WHERE cl.code=$1 AND f.code=$2 AND u.code=$3
		RETURNING id`,
		fieldRef(rec, "class"), fieldRef(rec, "family"), fieldRef(rec, "unit"), fieldBool(rec, "allowed"), fieldBool(rec, "suggested"), rec.Active).Scan(&id)
	if isNoRows(err) {
		return 0, fmt.Errorf("%w: class %q, family %q, or unit %q", domain.ErrCatalogReference, fieldRef(rec, "class"), fieldRef(rec, "family"), fieldRef(rec, "unit"))
	}
	if err != nil {
		return 0, mapCatalogWriteError(fmt.Errorf("insert resource_unit_policies: %w", err))
	}
	return id, nil
}

// updateUnitPolicy only mutates allowed/suggested/active — class/family/unit
// are this junction kind's composite PK, so re-scoping it is a
// delete+insert, not an update (same rationale as Option/PresentationField).
func updateUnitPolicy(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) error {
	var familyID, unitID int64
	err := tx.QueryRow(ctx, `SELECT family_id, unit_id FROM public.resource_unit_policies WHERE id = $1`, rec.ID).Scan(&familyID, &unitID)
	if isNoRows(err) {
		return domain.ErrCatalogRecordNotFound
	}
	if err != nil {
		return fmt.Errorf("resolve resource_unit_policies: %w", err)
	}
	tag, err := tx.Exec(ctx, `UPDATE public.resource_unit_policies SET allowed=$1, suggested=$2, active=$3 WHERE family_id=$4 AND unit_id=$5`,
		fieldBool(rec, "allowed"), fieldBool(rec, "suggested"), rec.Active, familyID, unitID)
	if err != nil {
		return mapCatalogWriteError(fmt.Errorf("update resource_unit_policies: %w", err))
	}
	return requireRow(tag.RowsAffected())
}

func setActiveUnitPolicy(ctx context.Context, tx pgx.Tx, id int64, active bool) error {
	tag, err := tx.Exec(ctx, `UPDATE public.resource_unit_policies SET active=$1 WHERE id = $2`, active, id)
	if err != nil {
		return mapCatalogWriteError(fmt.Errorf("set active on resource_unit_policies: %w", err))
	}
	return requireRow(tag.RowsAffected())
}

func deleteUnitPolicy(ctx context.Context, tx pgx.Tx, id int64) error {
	tag, err := tx.Exec(ctx, `DELETE FROM public.resource_unit_policies WHERE id = $1`, id)
	if err != nil {
		return mapCatalogDeleteError(fmt.Errorf("delete from resource_unit_policies: %w", err))
	}
	return requireRow(tag.RowsAffected())
}

// --- KindAttributeBinding ("Aplicabilidad") -------------------------------

func listAttributeBindings(ctx context.Context, q querier, f domain.CatalogFilter) ([]domain.CatalogRecord, error) {
	var conditions []string
	var args []any
	conditions = appendActiveFilter(conditions, f.Status, "ra.active")
	conditions, args = appendEqualsFilter(conditions, args, "cl.code", parentRefCode(f, "class"))
	conditions, args = appendEqualsFilter(conditions, args, "f.code", parentRefCode(f, "family"))
	conditions, args = appendEqualsFilter(conditions, args, "t.code", parentRefCode(f, "type"))
	conditions, args = appendEqualsFilter(conditions, args, "d.code", parentRefCode(f, "characteristic"))
	conditions, args = appendEqualsFilter(conditions, args, "ra.option_set", parentRefCode(f, "optionSet"))
	sql := finalizeQuery(`
		SELECT ra.id, cl.id, cl.code, f.id, f.code, COALESCE(t.id, 0), COALESCE(t.code, ''), d.id, d.code, COALESCE(os.id, 0), ra.option_set, ra.mode, ra.identity_participates, ra.active, ra.revision
		FROM public.resource_attributes ra
		JOIN public.resource_families f ON f.id = ra.family_id
		JOIN public.resource_classes cl ON cl.id = ra.class_id
		LEFT JOIN public.resource_types t ON t.id = ra.type_id
		JOIN public.attribute_definitions d ON d.id = ra.definition_id
		LEFT JOIN public.resource_option_sets os ON os.code = ra.option_set`, conditions, "f.id, COALESCE(t.id, 0), ra.display_order", f)

	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query resource_attributes: %w", err)
	}
	defer rows.Close()
	var out []domain.CatalogRecord
	for rows.Next() {
		var id, classID, familyID, typeID, definitionID, optionSetID int64
		var classCode, familyCode, typeCode, characteristic, optionSet, mode string
		var identityParticipates, active bool
		var revision uint64
		if err := rows.Scan(&id, &classID, &classCode, &familyID, &familyCode, &typeID, &typeCode, &definitionID, &characteristic, &optionSetID, &optionSet, &mode, &identityParticipates, &active, &revision); err != nil {
			return nil, fmt.Errorf("scan resource_attributes: %w", err)
		}
		out = append(out, catalogRecordWithRevision(attributeBindingRecord(id, classID, classCode, familyID, familyCode, typeID, typeCode, definitionID, characteristic, optionSetID, optionSet, mode, identityParticipates, active), revision))
	}
	return out, rows.Err()
}

func getAttributeBinding(ctx context.Context, q querier, id int64) (domain.CatalogRecord, error) {
	var classID, familyID, typeID, definitionID, optionSetID int64
	var classCode, familyCode, typeCode, characteristic, optionSet, mode string
	var identityParticipates, active bool
	var revision uint64
	err := q.QueryRow(ctx, `
		SELECT cl.id, cl.code, f.id, f.code, COALESCE(t.id, 0), COALESCE(t.code, ''), d.id, d.code, COALESCE(os.id, 0), ra.option_set, ra.mode, ra.identity_participates, ra.active, ra.revision
		FROM public.resource_attributes ra
		JOIN public.resource_families f ON f.id = ra.family_id
		JOIN public.resource_classes cl ON cl.id = ra.class_id
		LEFT JOIN public.resource_types t ON t.id = ra.type_id
		JOIN public.attribute_definitions d ON d.id = ra.definition_id
		LEFT JOIN public.resource_option_sets os ON os.code = ra.option_set
		WHERE ra.id=$1`, id).Scan(&classID, &classCode, &familyID, &familyCode, &typeID, &typeCode, &definitionID, &characteristic, &optionSetID, &optionSet, &mode, &identityParticipates, &active, &revision)
	if isNoRows(err) {
		return domain.CatalogRecord{}, domain.ErrCatalogRecordNotFound
	}
	if err != nil {
		return domain.CatalogRecord{}, fmt.Errorf("get resource_attributes: %w", err)
	}
	return catalogRecordWithRevision(attributeBindingRecord(id, classID, classCode, familyID, familyCode, typeID, typeCode, definitionID, characteristic, optionSetID, optionSet, mode, identityParticipates, active), revision), nil
}

func attributeBindingRecord(id, classID int64, classCode string, familyID int64, familyCode string, typeID int64, typeCode string,
	definitionID int64, characteristic string, optionSetID int64, optionSet, mode string, identityParticipates, active bool) domain.CatalogRecord {
	return domain.CatalogRecord{Kind: domain.KindAttributeBinding, ID: id, Active: active, Values: map[string]domain.CatalogValue{
		"class": refValue(domain.KindClass, classCode, classID), "family": refValue(domain.KindFamily, familyCode, familyID), "type": refValue(domain.KindType, typeCode, typeID),
		"characteristic": refValue(domain.KindAttributeDefinition, characteristic, definitionID), "optionSet": refValue(domain.KindOptionSet, optionSet, optionSetID),
		"mode": textValue(mode), "identityParticipates": boolValue(identityParticipates),
	}}
}

// resolveOptionalTypeID resolves the AttributeBinding/PresentationField
// "type" ref: "" means family-scoped (no type row), matching
// domain.ResourceAttribute.TypeCode's existing "" == family-level
// convention (catalog_loader.go's COALESCE(t.code, ”)).
func resolveOptionalTypeID(ctx context.Context, q querier, classCode, familyCode, typeCode string, familyID int64) (*int64, error) {
	if typeCode == "" {
		return nil, nil
	}
	resolvedTypeID, resolvedFamilyID, err := resolveTypeID(ctx, q, classCode, familyCode, typeCode)
	if err != nil {
		return nil, err
	}
	if resolvedFamilyID != familyID {
		return nil, fmt.Errorf("%w: type %q does not belong to family %q", domain.ErrCatalogReference, typeCode, familyCode)
	}
	return &resolvedTypeID, nil
}

func insertAttributeBinding(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) (int64, error) {
	familyID, classID, err := resolveFamilyID(ctx, tx, fieldRef(rec, "class"), fieldRef(rec, "family"))
	if err != nil {
		return 0, err
	}
	typeID, err := resolveOptionalTypeID(ctx, tx, fieldRef(rec, "class"), fieldRef(rec, "family"), fieldRef(rec, "type"), familyID)
	if err != nil {
		return 0, err
	}
	definitionID, err := resolveDefinitionID(ctx, tx, fieldRef(rec, "characteristic"))
	if err != nil {
		return 0, err
	}
	optionSet := fieldRef(rec, "optionSet")
	if optionSet == "" {
		optionSet = "DEFAULT"
	}
	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO public.resource_attributes (class_id, family_id, type_id, definition_id, option_set, mode, identity_participates, active, display_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,
		    COALESCE((SELECT MAX(display_order)+1 FROM public.resource_attributes WHERE family_id=$2 AND type_id IS NOT DISTINCT FROM $3), 0))
		RETURNING id`,
		classID, familyID, typeID, definitionID, optionSet, fieldText(rec, "mode"), fieldBool(rec, "identityParticipates"), rec.Active).Scan(&id)
	if err != nil {
		return 0, mapCatalogWriteError(fmt.Errorf("insert resource_attributes: %w", err))
	}
	return id, nil
}

func updateAttributeBinding(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) error {
	familyID, classID, err := resolveFamilyID(ctx, tx, fieldRef(rec, "class"), fieldRef(rec, "family"))
	if err != nil {
		return err
	}
	typeID, err := resolveOptionalTypeID(ctx, tx, fieldRef(rec, "class"), fieldRef(rec, "family"), fieldRef(rec, "type"), familyID)
	if err != nil {
		return err
	}
	definitionID, err := resolveDefinitionID(ctx, tx, fieldRef(rec, "characteristic"))
	if err != nil {
		return err
	}
	optionSet := fieldRef(rec, "optionSet")
	if optionSet == "" {
		optionSet = "DEFAULT"
	}
	tag, err := tx.Exec(ctx, `
		UPDATE public.resource_attributes
		SET class_id=$1, family_id=$2, type_id=$3, definition_id=$4, option_set=$5, mode=$6, identity_participates=$7, active=$8
		WHERE id=$9`,
		classID, familyID, typeID, definitionID, optionSet, fieldText(rec, "mode"), fieldBool(rec, "identityParticipates"), rec.Active, rec.ID)
	if err != nil {
		return mapCatalogWriteError(fmt.Errorf("update resource_attributes: %w", err))
	}
	return requireRow(tag.RowsAffected())
}

// --- KindPresentationField ("Campo de Presentación") ---------------------
//
// resource_type_presentation_fields' natural PK is (type_id,
// attribute_definition_id); migration 000009 added a real BIGSERIAL id
// column (UNIQUE, additive).

func listPresentationFields(ctx context.Context, q querier, f domain.CatalogFilter) ([]domain.CatalogRecord, error) {
	var conditions []string
	var args []any
	conditions = appendActiveFilter(conditions, f.Status, "pf.active")
	conditions, args = appendEqualsFilter(conditions, args, "cl.code", parentRefCode(f, "class"))
	conditions, args = appendEqualsFilter(conditions, args, "f.code", parentRefCode(f, "family"))
	conditions, args = appendEqualsFilter(conditions, args, "t.code", parentRefCode(f, "type"))
	sql := finalizeQuery(`
		SELECT pf.id, cl.id, cl.code, f.id, f.code, t.id, t.code, d.id, d.code, pf.position, pf.active, pf.revision
		FROM public.resource_type_presentation_fields pf
		JOIN public.resource_types t ON t.id = pf.type_id
		JOIN public.resource_families f ON f.id = t.family_id
		JOIN public.resource_classes cl ON cl.id = t.class_id
		JOIN public.attribute_definitions d ON d.id = pf.attribute_definition_id`, conditions, "t.id, pf.position", f)

	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query resource_type_presentation_fields: %w", err)
	}
	defer rows.Close()
	var out []domain.CatalogRecord
	for rows.Next() {
		var id, classID, familyID, typeID, definitionID int64
		var classCode, familyCode, typeCode, characteristic string
		var position int
		var active bool
		var revision uint64
		if err := rows.Scan(&id, &classID, &classCode, &familyID, &familyCode, &typeID, &typeCode, &definitionID, &characteristic, &position, &active, &revision); err != nil {
			return nil, fmt.Errorf("scan resource_type_presentation_fields: %w", err)
		}
		out = append(out, catalogRecordWithRevision(presentationFieldRecord(id, classID, classCode, familyID, familyCode, typeID, typeCode, definitionID, characteristic, position, active), revision))
	}
	return out, rows.Err()
}

func getPresentationField(ctx context.Context, q querier, id int64) (domain.CatalogRecord, error) {
	var classID, familyID, typeID, definitionID int64
	var classCode, familyCode, typeCode, characteristic string
	var position int
	var active bool
	var revision uint64
	err := q.QueryRow(ctx, `
		SELECT cl.id, cl.code, f.id, f.code, t.id, t.code, d.id, d.code, pf.position, pf.active, pf.revision
		FROM public.resource_type_presentation_fields pf
		JOIN public.resource_types t ON t.id = pf.type_id
		JOIN public.resource_families f ON f.id = t.family_id
		JOIN public.resource_classes cl ON cl.id = t.class_id
		JOIN public.attribute_definitions d ON d.id = pf.attribute_definition_id
		WHERE pf.id = $1`, id).
		Scan(&classID, &classCode, &familyID, &familyCode, &typeID, &typeCode, &definitionID, &characteristic, &position, &active, &revision)
	if isNoRows(err) {
		return domain.CatalogRecord{}, domain.ErrCatalogRecordNotFound
	}
	if err != nil {
		return domain.CatalogRecord{}, fmt.Errorf("get resource_type_presentation_fields: %w", err)
	}
	return catalogRecordWithRevision(presentationFieldRecord(id, classID, classCode, familyID, familyCode, typeID, typeCode, definitionID, characteristic, position, active), revision), nil
}

func presentationFieldRecord(id, classID int64, classCode string, familyID int64, familyCode string, typeID int64, typeCode string,
	definitionID int64, characteristic string, position int, active bool) domain.CatalogRecord {
	return domain.CatalogRecord{Kind: domain.KindPresentationField, ID: id, Active: active, Values: map[string]domain.CatalogValue{
		"class": refValue(domain.KindClass, classCode, classID), "family": refValue(domain.KindFamily, familyCode, familyID), "type": refValue(domain.KindType, typeCode, typeID),
		"characteristic": refValue(domain.KindAttributeDefinition, characteristic, definitionID), "position": intValue(position),
	}}
}

func insertPresentationField(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `
		INSERT INTO public.resource_type_presentation_fields (type_id, attribute_definition_id, position, active)
		SELECT t.id, d.id, $5, $6
		FROM public.resource_types t
		JOIN public.resource_families f ON f.id = t.family_id
		JOIN public.resource_classes cl ON cl.id = t.class_id, public.attribute_definitions d
		WHERE cl.code=$1 AND f.code=$2 AND t.code=$3 AND d.code=$4
		RETURNING id`,
		fieldRef(rec, "class"), fieldRef(rec, "family"), fieldRef(rec, "type"), fieldRef(rec, "characteristic"), fieldInt(rec, "position"), rec.Active).Scan(&id)
	if isNoRows(err) {
		return 0, fmt.Errorf("%w: type %q or characteristic %q", domain.ErrCatalogReference, fieldRef(rec, "type"), fieldRef(rec, "characteristic"))
	}
	if err != nil {
		return 0, mapCatalogWriteError(fmt.Errorf("insert resource_type_presentation_fields: %w", err))
	}
	return id, nil
}

// updatePresentationField only mutates position/active — class/family/type/
// characteristic are this junction kind's composite PK (same rationale as
// updateUnitPolicy/updateOption).
func updatePresentationField(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) error {
	var typeID, definitionID int64
	err := tx.QueryRow(ctx, `SELECT type_id, attribute_definition_id FROM public.resource_type_presentation_fields WHERE id = $1`, rec.ID).Scan(&typeID, &definitionID)
	if isNoRows(err) {
		return domain.ErrCatalogRecordNotFound
	}
	if err != nil {
		return fmt.Errorf("resolve resource_type_presentation_fields: %w", err)
	}
	tag, err := tx.Exec(ctx, `UPDATE public.resource_type_presentation_fields SET position=$1, active=$2 WHERE type_id=$3 AND attribute_definition_id=$4`,
		fieldInt(rec, "position"), rec.Active, typeID, definitionID)
	if err != nil {
		return mapCatalogWriteError(fmt.Errorf("update resource_type_presentation_fields: %w", err))
	}
	return requireRow(tag.RowsAffected())
}

func setActivePresentationField(ctx context.Context, tx pgx.Tx, id int64, active bool) error {
	tag, err := tx.Exec(ctx, `UPDATE public.resource_type_presentation_fields SET active=$1 WHERE id = $2`, active, id)
	if err != nil {
		return mapCatalogWriteError(fmt.Errorf("set active on resource_type_presentation_fields: %w", err))
	}
	return requireRow(tag.RowsAffected())
}

func deletePresentationField(ctx context.Context, tx pgx.Tx, id int64) error {
	tag, err := tx.Exec(ctx, `DELETE FROM public.resource_type_presentation_fields WHERE id = $1`, id)
	if err != nil {
		return mapCatalogDeleteError(fmt.Errorf("delete from resource_type_presentation_fields: %w", err))
	}
	return requireRow(tag.RowsAffected())
}
