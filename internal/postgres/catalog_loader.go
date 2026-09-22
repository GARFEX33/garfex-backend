package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/GARFEX33/garfex-backend/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrCatalogEmpty is returned by LoadResourceCatalog when the hydrated
// catalog has zero resource classes — migrations applied but the catalog
// tables were never seeded, as distinct from a query/connection failure.
// cmd/garfex's boot sequence uses errors.Is against this sentinel to show a
// "run the migrations" diagnostic instead of a generic load-failure one
// (design §2).
var ErrCatalogEmpty = errors.New("catálogo de recursos vacío")

// LoadResourceCatalog hydrates the complete domain.ResourceCatalog from
// PostgreSQL (design §2): Postgres is now canonical for catalog *structure*
// (docs/architecture/catalog-source-of-truth.md), replacing the
// domain.SeedResourceCatalog() Go literal as the runtime source. All 11
// queries run inside one RepeatableRead, read-only transaction (design D7)
// so the snapshot cannot be torn by a concurrent admin write mid-load; boot
// is the only caller, so the extra isolation cost is irrelevant.
func LoadResourceCatalog(ctx context.Context, pool *pgxpool.Pool) (domain.ResourceCatalog, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return domain.ResourceCatalog{}, fmt.Errorf("begin catalog load transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	catalog, err := loadResourceCatalogTx(ctx, tx)
	if err != nil {
		return domain.ResourceCatalog{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ResourceCatalog{}, fmt.Errorf("commit catalog load transaction: %w", err)
	}
	return catalog, classifyLoadedCatalog(catalog)
}

// loadResourceCatalogTx hydrates inside an already-open tx (no begin/commit);
// the dormant V2 applicability helpers call it read-write before their own commit.
func loadResourceCatalogTx(ctx context.Context, tx pgx.Tx) (domain.ResourceCatalog, error) {
	classes, err := loadClasses(ctx, tx)
	if err != nil {
		return domain.ResourceCatalog{}, err
	}
	optionSets, err := loadOptionSets(ctx, tx)
	if err != nil {
		return domain.ResourceCatalog{}, err
	}
	families, err := loadFamilies(ctx, tx)
	if err != nil {
		return domain.ResourceCatalog{}, err
	}
	types, err := loadTypes(ctx, tx)
	if err != nil {
		return domain.ResourceCatalog{}, err
	}
	presentationFields, err := loadPresentationFields(ctx, tx)
	if err != nil {
		return domain.ResourceCatalog{}, err
	}
	units, err := loadUnits(ctx, tx)
	if err != nil {
		return domain.ResourceCatalog{}, err
	}
	unitPolicies, err := loadUnitPolicies(ctx, tx)
	if err != nil {
		return domain.ResourceCatalog{}, err
	}
	definitions, err := loadDefinitions(ctx, tx)
	if err != nil {
		return domain.ResourceCatalog{}, err
	}
	rules, err := loadAttributeRules(ctx, tx)
	if err != nil {
		return domain.ResourceCatalog{}, err
	}
	attributes, err := loadAttributes(ctx, tx, rules)
	if err != nil {
		return domain.ResourceCatalog{}, err
	}
	options, err := loadOptions(ctx, tx)
	if err != nil {
		return domain.ResourceCatalog{}, err
	}
	relations, err := loadRelations(ctx, tx)
	if err != nil {
		return domain.ResourceCatalog{}, err
	}

	return domain.ResourceCatalog{
		Classes:            classes,
		Families:           families,
		Types:              types,
		PresentationFields: presentationFields,
		Units:              units,
		UnitPolicies:       unitPolicies,
		Definitions:        definitions,
		Attributes:         attributes,
		Options:            options,
		Relations:          relations,
		OptionSets:         optionSets,
	}, nil
}

// classifyLoadedCatalog reports ErrCatalogEmpty when the hydrated catalog
// has zero classes, so main.go can show a distinct diagnostic from a
// generic load failure. Pure and DB-free — see catalog_loader_test.go.
func classifyLoadedCatalog(c domain.ResourceCatalog) error {
	if len(c.Classes) == 0 {
		return ErrCatalogEmpty
	}
	return nil
}

func loadClasses(ctx context.Context, tx pgx.Tx) ([]domain.ResourceClass, error) {
	rows, err := tx.Query(ctx, `
		SELECT code, name, plural, slug, aliases, keywords, display_order, active
		FROM public.resource_classes
		ORDER BY display_order, code`)
	if err != nil {
		return nil, fmt.Errorf("query resource_classes: %w", err)
	}
	defer rows.Close()
	var classes []domain.ResourceClass
	for rows.Next() {
		var c domain.ResourceClass
		if err := rows.Scan(&c.Code, &c.Name, &c.Plural, &c.Slug, &c.Aliases, &c.Keywords, &c.Order, &c.Active); err != nil {
			return nil, fmt.Errorf("scan resource_classes: %w", err)
		}
		classes = append(classes, c)
	}
	return classes, rows.Err()
}

func loadOptionSets(ctx context.Context, tx pgx.Tx) ([]domain.ResourceOptionSet, error) {
	rows, err := tx.Query(ctx, `SELECT code, name, active FROM public.resource_option_sets ORDER BY code`)
	if err != nil {
		return nil, fmt.Errorf("query resource_option_sets: %w", err)
	}
	defer rows.Close()
	var sets []domain.ResourceOptionSet
	for rows.Next() {
		var set domain.ResourceOptionSet
		if err := rows.Scan(&set.Code, &set.Name, &set.Active); err != nil {
			return nil, fmt.Errorf("scan resource_option_sets: %w", err)
		}
		sets = append(sets, set)
	}
	return sets, rows.Err()
}

func loadFamilies(ctx context.Context, tx pgx.Tx) ([]domain.ResourceFamily, error) {
	rows, err := tx.Query(ctx, `
		SELECT cl.code, f.code, f.name, f.active
		FROM public.resource_families f
		JOIN public.resource_classes cl ON cl.id = f.class_id
		ORDER BY cl.id, f.id`)
	if err != nil {
		return nil, fmt.Errorf("query resource_families: %w", err)
	}
	defer rows.Close()
	var families []domain.ResourceFamily
	for rows.Next() {
		var f domain.ResourceFamily
		if err := rows.Scan(&f.ClassCode, &f.Code, &f.Name, &f.Active); err != nil {
			return nil, fmt.Errorf("scan resource_families: %w", err)
		}
		families = append(families, f)
	}
	return families, rows.Err()
}

func loadTypes(ctx context.Context, tx pgx.Tx) ([]domain.ResourceType, error) {
	rows, err := tx.Query(ctx, `
		SELECT cl.code, f.code, t.code, t.name, t.active
		FROM public.resource_types t
		JOIN public.resource_families f ON f.id = t.family_id
		JOIN public.resource_classes cl ON cl.id = t.class_id
		ORDER BY f.id, t.id`)
	if err != nil {
		return nil, fmt.Errorf("query resource_types: %w", err)
	}
	defer rows.Close()
	var types []domain.ResourceType
	for rows.Next() {
		var t domain.ResourceType
		if err := rows.Scan(&t.ClassCode, &t.FamilyCode, &t.Code, &t.Name, &t.Active); err != nil {
			return nil, fmt.Errorf("scan resource_types: %w", err)
		}
		types = append(types, t)
	}
	return types, rows.Err()
}

func loadPresentationFields(ctx context.Context, tx pgx.Tx) ([]domain.PresentationField, error) {
	rows, err := tx.Query(ctx, `
		SELECT cl.code, f.code, t.code, d.code, pf.position, pf.active
		FROM public.resource_type_presentation_fields pf
		JOIN public.resource_types t ON t.id = pf.type_id
		JOIN public.resource_families f ON f.id = t.family_id
		JOIN public.resource_classes cl ON cl.id = t.class_id
		JOIN public.attribute_definitions d ON d.id = pf.attribute_definition_id
		ORDER BY t.id, pf.position`)
	if err != nil {
		return nil, fmt.Errorf("query resource_type_presentation_fields: %w", err)
	}
	defer rows.Close()
	var fields []domain.PresentationField
	for rows.Next() {
		var f domain.PresentationField
		if err := rows.Scan(&f.ClassCode, &f.FamilyCode, &f.TypeCode, &f.AttributeCode, &f.Position, &f.Active); err != nil {
			return nil, fmt.Errorf("scan resource_type_presentation_fields: %w", err)
		}
		fields = append(fields, f)
	}
	return fields, rows.Err()
}

func loadUnits(ctx context.Context, tx pgx.Tx) ([]domain.UnitDefinition, error) {
	rows, err := tx.Query(ctx, `SELECT code, name, symbol, dimension, active FROM public.unit_definitions ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query unit_definitions: %w", err)
	}
	defer rows.Close()
	var units []domain.UnitDefinition
	for rows.Next() {
		var u domain.UnitDefinition
		if err := rows.Scan(&u.Code, &u.Name, &u.Symbol, &u.Dimension, &u.Active); err != nil {
			return nil, fmt.Errorf("scan unit_definitions: %w", err)
		}
		units = append(units, u)
	}
	return units, rows.Err()
}

func loadUnitPolicies(ctx context.Context, tx pgx.Tx) ([]domain.ResourceUnitPolicy, error) {
	rows, err := tx.Query(ctx, `
		SELECT cl.code, f.code, u.code, p.allowed, p.suggested, p.active
		FROM public.resource_unit_policies p
		JOIN public.resource_families f ON f.id = p.family_id
		JOIN public.resource_classes cl ON cl.id = f.class_id
		JOIN public.unit_definitions u ON u.id = p.unit_id
		ORDER BY f.id, u.id`)
	if err != nil {
		return nil, fmt.Errorf("query resource_unit_policies: %w", err)
	}
	defer rows.Close()
	var policies []domain.ResourceUnitPolicy
	for rows.Next() {
		var p domain.ResourceUnitPolicy
		if err := rows.Scan(&p.ClassCode, &p.FamilyCode, &p.UnitCode, &p.Allowed, &p.Suggested, &p.Active); err != nil {
			return nil, fmt.Errorf("scan resource_unit_policies: %w", err)
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

func loadDefinitions(ctx context.Context, tx pgx.Tx) ([]domain.AttributeDefinition, error) {
	rows, err := tx.Query(ctx, `
		SELECT code, name, value_type, dimension, default_identity_participates, active
		FROM public.attribute_definitions
		ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query attribute_definitions: %w", err)
	}
	defer rows.Close()
	var definitions []domain.AttributeDefinition
	for rows.Next() {
		var d domain.AttributeDefinition
		var valueType string
		var dimension *string
		if err := rows.Scan(&d.Code, &d.Name, &valueType, &dimension, &d.DefaultIdentityParticipates, &d.Active); err != nil {
			return nil, fmt.Errorf("scan attribute_definitions: %w", err)
		}
		d.ValueType = domain.AttributeValueType(valueType)
		d.Dimension = dereference(dimension)
		definitions = append(definitions, d)
	}
	return definitions, rows.Err()
}

// loadAttributeRules returns every resource_attribute_rules row keyed by
// its owning resource_attribute_id, ordered by the rule's own display_order
// — attached to its ResourceAttribute by loadAttributes.
func loadAttributeRules(ctx context.Context, tx pgx.Tx) (map[int64][]domain.AttributeRule, error) {
	rows, err := tx.Query(ctx, `
		SELECT rr.resource_attribute_id, wd.code, rr.when_equals, rr.mode, rr.identity_participates, rr.not_applicable, rr.active
		FROM public.resource_attribute_rules rr
		JOIN public.attribute_definitions wd ON wd.id = rr.when_definition_id
		ORDER BY rr.resource_attribute_id, rr.display_order`)
	if err != nil {
		return nil, fmt.Errorf("query resource_attribute_rules: %w", err)
	}
	defer rows.Close()
	rules := map[int64][]domain.AttributeRule{}
	for rows.Next() {
		var attributeID int64
		var whenCode, whenEquals, mode string
		var identityParticipates, notApplicable, active bool
		if err := rows.Scan(&attributeID, &whenCode, &whenEquals, &mode, &identityParticipates, &notApplicable, &active); err != nil {
			return nil, fmt.Errorf("scan resource_attribute_rules: %w", err)
		}
		rules[attributeID] = append(rules[attributeID], domain.AttributeRule{
			When:                 domain.AttributeCondition{AttributeCode: whenCode, Equals: whenEquals},
			Mode:                 domain.AttributeMode(mode),
			IdentityParticipates: identityParticipates,
			NotApplicable:        notApplicable,
			Active:               active,
		})
	}
	return rules, rows.Err()
}

func loadAttributes(ctx context.Context, tx pgx.Tx, rules map[int64][]domain.AttributeRule) ([]domain.ResourceAttribute, error) {
	rows, err := tx.Query(ctx, `
		SELECT ra.id, cl.code, f.code, COALESCE(t.code, ''), ra.option_set,
		       d.code, d.name, d.value_type, d.dimension, d.default_identity_participates, d.active,
		       ra.mode, ra.identity_participates, ra.active
		FROM public.resource_attributes ra
		JOIN public.resource_families f ON f.id = ra.family_id
		JOIN public.resource_classes cl ON cl.id = ra.class_id
		LEFT JOIN public.resource_types t ON t.id = ra.type_id
		JOIN public.attribute_definitions d ON d.id = ra.definition_id
		ORDER BY f.id, COALESCE(t.id, 0), ra.display_order`)
	if err != nil {
		return nil, fmt.Errorf("query resource_attributes: %w", err)
	}
	defer rows.Close()
	var attributes []domain.ResourceAttribute
	for rows.Next() {
		var id int64
		var attribute domain.ResourceAttribute
		var valueType string
		var mode string
		var dimension *string
		if err := rows.Scan(&id, &attribute.ClassCode, &attribute.FamilyCode, &attribute.TypeCode, &attribute.OptionSet,
			&attribute.Definition.Code, &attribute.Definition.Name, &valueType, &dimension, &attribute.Definition.DefaultIdentityParticipates,
			&attribute.Definition.Active, &mode, &attribute.IdentityParticipates, &attribute.Active); err != nil {
			return nil, fmt.Errorf("scan resource_attributes: %w", err)
		}
		attribute.Definition.ValueType = domain.AttributeValueType(valueType)
		attribute.Definition.Dimension = dereference(dimension)
		attribute.Mode = domain.AttributeMode(mode)
		attribute.Rules = rules[id]
		attributes = append(attributes, attribute)
	}
	return attributes, rows.Err()
}

func loadOptions(ctx context.Context, tx pgx.Tx) ([]domain.AttributeOption, error) {
	rows, err := tx.Query(ctx, `
		SELECT ao.option_set, d.code, ao.code, ao.label, ao.active
		FROM public.attribute_options ao
		JOIN public.attribute_definitions d ON d.id = ao.attribute_definition_id
		ORDER BY d.id, ao.display_order`)
	if err != nil {
		return nil, fmt.Errorf("query attribute_options: %w", err)
	}
	defer rows.Close()
	var options []domain.AttributeOption
	for rows.Next() {
		var o domain.AttributeOption
		if err := rows.Scan(&o.OptionSet, &o.AttributeCode, &o.Code, &o.Label, &o.Active); err != nil {
			return nil, fmt.Errorf("scan attribute_options: %w", err)
		}
		options = append(options, o)
	}
	return options, rows.Err()
}

func loadRelations(ctx context.Context, tx pgx.Tx) ([]domain.AttributeOptionRelation, error) {
	rows, err := tx.Query(ctx, `
		SELECT ar.option_set, fd.code, ar.from_option_code, td.code, ar.to_option_code, ar.active
		FROM public.attribute_option_relations ar
		JOIN public.attribute_definitions fd ON fd.id = ar.from_attribute_definition_id
		JOIN public.attribute_definitions td ON td.id = ar.to_attribute_definition_id
		ORDER BY ar.id`)
	if err != nil {
		return nil, fmt.Errorf("query attribute_option_relations: %w", err)
	}
	defer rows.Close()
	var relations []domain.AttributeOptionRelation
	for rows.Next() {
		var r domain.AttributeOptionRelation
		if err := rows.Scan(&r.OptionSet, &r.FromAttribute, &r.FromOption, &r.ToAttribute, &r.ToOption, &r.Active); err != nil {
			return nil, fmt.Errorf("scan attribute_option_relations: %w", err)
		}
		relations = append(relations, r)
	}
	return relations, rows.Err()
}

// revisionTables lists every table migration 8 adds a `revision` column to:
// public.recursos and the 11 catalog-definition parent tables (design
// "Resource revisions and identity"). resource_attribute_rules deliberately
// has no independent revision and is intentionally absent here.
var revisionTables = map[string]bool{
	"recursos":                          true,
	"resource_classes":                  true,
	"resource_families":                 true,
	"resource_types":                    true,
	"attribute_definitions":             true,
	"resource_option_sets":              true,
	"attribute_options":                 true,
	"attribute_option_relations":        true,
	"unit_definitions":                  true,
	"resource_unit_policies":            true,
	"resource_attributes":               true,
	"resource_type_presentation_fields": true,
}

// loadRevision reads the current migration-8 revision for one row of a
// revision-bearing table, inside the given transaction. It is the dormant
// read half of the CAS revision handling introduced by migration 8; the
// write-path CAS kind handlers land in a later, independently green slice
// (design "Resource revisions and identity", "SQL CAS classification"). The
// table name is validated against revisionTables before being interpolated,
// so this never accepts caller-controlled SQL identifiers.
func loadRevision(ctx context.Context, tx pgx.Tx, table string, id int64) (uint64, error) {
	if !revisionTables[table] {
		return 0, fmt.Errorf("loadRevision: table %q has no revision column", table)
	}
	var revision uint64
	if err := tx.QueryRow(ctx, `SELECT revision FROM public.`+table+` WHERE id = $1`, id).Scan(&revision); err != nil {
		return 0, fmt.Errorf("query %s revision: %w", table, err)
	}
	return revision, nil
}

// normalizeCatalogForParity rewrites representational differences between
// domain.SeedResourceCatalog()'s Go literal and LoadResourceCatalog's
// DB-hydrated result that are semantically equal at runtime (design D6) but
// would spuriously fail a naive reflect.DeepEqual: OptionSet "" (Go zero
// value) vs "DEFAULT" (the DB column default — canonicalOptionSet already
// treats both as equal at runtime), and nil vs empty []string for
// Aliases/Keywords. Deliberately test-only — SeedResourceCatalog itself
// must stay byte-identical for TestKnownResourceCodesAreNeverRenderedOrTranslated
// and the many TUI fixtures that assert against it directly.
func normalizeCatalogForParity(c domain.ResourceCatalog) domain.ResourceCatalog {
	normalizeOptionSet := func(s string) string {
		if s == "" || s == "DEFAULT" {
			return ""
		}
		return s
	}
	normalizeStrings := func(s []string) []string {
		if len(s) == 0 {
			return nil
		}
		return s
	}

	classes := make([]domain.ResourceClass, len(c.Classes))
	for i, class := range c.Classes {
		class.Aliases = normalizeStrings(class.Aliases)
		class.Keywords = normalizeStrings(class.Keywords)
		classes[i] = class
	}

	attributes := make([]domain.ResourceAttribute, len(c.Attributes))
	for i, attribute := range c.Attributes {
		attribute.OptionSet = normalizeOptionSet(attribute.OptionSet)
		attributes[i] = attribute
	}

	options := make([]domain.AttributeOption, len(c.Options))
	for i, option := range c.Options {
		option.OptionSet = normalizeOptionSet(option.OptionSet)
		options[i] = option
	}

	relations := make([]domain.AttributeOptionRelation, len(c.Relations))
	for i, relation := range c.Relations {
		relation.OptionSet = normalizeOptionSet(relation.OptionSet)
		relations[i] = relation
	}

	c.Classes = classes
	c.Attributes = attributes
	c.Options = options
	c.Relations = relations
	return c
}
