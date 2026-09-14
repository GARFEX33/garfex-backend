package postgres

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Stage-3E APLICABILIDAD child-row helpers, now wired into every V2 write's
// coherent-result protocol (design "Catalog transaction and coherent
// publication") via runV2CoherentTx/catalogRegistryV2 below.
var (
	errApplicabilityRulesOmitted     = errors.New("applicability rules are omitted")
	errApplicabilityRevisionRequired = errors.New("applicability update requires a non-zero expected revision")
	errApplicabilityStaleRevision    = domain.ErrRevisionConflict

	// errCatalogCandidateMismatchV2 is 4B's replacement for 3E's bounded,
	// APLICABILIDAD-only errApplicabilityCandidateMismatch/
	// compareApplicabilityCandidate/compareApplicabilityRules (retired): every
	// V2 write across all 11 kinds compares against a real
	// domain.EquivalentResourceCatalogs (4A) candidate now.
	errCatalogCandidateMismatchV2 = errors.New("reloaded catalog does not match the expected candidate")
)

// loadResourceCatalogTxV2 lets tests force a reload failure — renamed from
// 3E's applicability-only loadResourceCatalogTxForApplicabilityV2 now that
// every V2 write shares one coherent-result protocol.
var loadResourceCatalogTxV2 = loadResourceCatalogTx

// catalogRegistryV2 is the fixed, pure kind registry domain.ApplyCatalogMutation
// needs to build each write's private candidate — safe as a package singleton.
var catalogRegistryV2 = domain.NewCatalogRegistry()

func insertApplicabilityRulesV2(ctx context.Context, tx pgx.Tx, attributeID int64, rules []domain.CatalogRuleRecord) error {
	for order, rule := range rules {
		whenDefinitionID, err := resolveDefinitionID(ctx, tx, rule.When.AttributeCode)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO public.resource_attribute_rules
				(resource_attribute_id, display_order, when_definition_id, when_equals, mode, identity_participates, not_applicable, active)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			attributeID, order, whenDefinitionID, rule.When.Equals, string(rule.Mode), rule.IdentityParticipates, rule.NotApplicable, rule.Active)
		if err != nil {
			return mapCatalogWriteError(fmt.Errorf("insert resource_attribute_rules: %w", err))
		}
	}
	return nil
}

func replaceApplicabilityRulesV2(ctx context.Context, tx pgx.Tx, attributeID int64, rules []domain.CatalogRuleRecord) error {
	if _, err := tx.Exec(ctx, `DELETE FROM public.resource_attribute_rules WHERE resource_attribute_id=$1`, attributeID); err != nil {
		return mapCatalogWriteError(fmt.Errorf("delete resource_attribute_rules: %w", err))
	}
	return insertApplicabilityRulesV2(ctx, tx, attributeID, rules)
}

func resolveApplicabilityWriteRefs(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) (classID, familyID int64, typeID *int64, definitionID int64, optionSet string, err error) {
	familyID, classID, err = resolveFamilyID(ctx, tx, fieldRef(rec, "class"), fieldRef(rec, "family"))
	if err != nil {
		return 0, 0, nil, 0, "", err
	}
	typeID, err = resolveOptionalTypeID(ctx, tx, fieldRef(rec, "class"), fieldRef(rec, "family"), fieldRef(rec, "type"), familyID)
	if err != nil {
		return 0, 0, nil, 0, "", err
	}
	definitionID, err = resolveDefinitionID(ctx, tx, fieldRef(rec, "characteristic"))
	if err != nil {
		return 0, 0, nil, 0, "", err
	}
	optionSet = fieldRef(rec, "optionSet")
	if optionSet == "" {
		optionSet = "DEFAULT"
	}
	return classID, familyID, typeID, definitionID, optionSet, nil
}

func insertApplicabilityParentV2(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) (id int64, revision uint64, err error) { // starts at revision 1
	classID, familyID, typeID, definitionID, optionSet, err := resolveApplicabilityWriteRefs(ctx, tx, rec)
	if err != nil {
		return 0, 0, err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO public.resource_attributes (class_id, family_id, type_id, definition_id, option_set, mode, identity_participates, active, display_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,
		    COALESCE((SELECT MAX(display_order)+1 FROM public.resource_attributes WHERE family_id=$2 AND type_id IS NOT DISTINCT FROM $3), 0))
		RETURNING id, revision`,
		classID, familyID, typeID, definitionID, optionSet, fieldText(rec, "mode"), fieldBool(rec, "identityParticipates"), rec.Active).Scan(&id, &revision)
	if err != nil {
		return 0, 0, mapCatalogWriteError(fmt.Errorf("insert resource_attributes: %w", err))
	}
	return id, revision, nil
}

func updateApplicabilityParentV2(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord, expectedRevision uint64) (uint64, error) {
	if expectedRevision == 0 {
		return 0, errApplicabilityRevisionRequired
	}
	classID, familyID, typeID, definitionID, optionSet, err := resolveApplicabilityWriteRefs(ctx, tx, rec)
	if err != nil {
		return 0, err
	}
	var revision uint64
	err = tx.QueryRow(ctx, `
		UPDATE public.resource_attributes
		SET class_id=$1, family_id=$2, type_id=$3, definition_id=$4, option_set=$5, mode=$6, identity_participates=$7, active=$8,
		    revision = revision + 1, updated_at = NOW()
		WHERE id=$9 AND revision=$10
		RETURNING revision`,
		classID, familyID, typeID, definitionID, optionSet, fieldText(rec, "mode"), fieldBool(rec, "identityParticipates"), rec.Active, rec.ID, expectedRevision).Scan(&revision)
	if err == nil {
		return revision, nil
	}
	if !isNoRows(err) {
		return 0, mapCatalogWriteError(fmt.Errorf("update resource_attributes: %w", err))
	}
	var current uint64
	lookupErr := tx.QueryRow(ctx, `SELECT revision FROM public.resource_attributes WHERE id=$1 FOR UPDATE`, rec.ID).Scan(&current)
	if isNoRows(lookupErr) {
		return 0, domain.ErrCatalogRecordNotFound
	}
	if lookupErr != nil {
		return 0, fmt.Errorf("resolve resource_attributes revision: %w", lookupErr)
	}
	return 0, errApplicabilityStaleRevision
}

// requireApplicabilityRules rejects a nil Rules slice before any transaction
// begins — Insert/Update both need this same precondition (design "Catalog
// transaction and coherent publication" is a persistence protocol, not an
// input-shape check).
func requireApplicabilityRules(rec domain.CatalogRecord) error {
	if rec.Rules == nil {
		return errApplicabilityRulesOmitted
	}
	return nil
}

func insertApplicabilityAggregateV2(ctx context.Context, pool *pgxpool.Pool, rec domain.CatalogRecord) (domain.CatalogWriteResult, error) {
	if err := requireApplicabilityRules(rec); err != nil {
		return domain.CatalogWriteResult{}, err
	}
	id, revision, catalog, err := runV2CoherentTx(ctx, pool, insertMutationV2(rec), func(ctx context.Context, tx pgx.Tx) (int64, uint64, error) {
		id, revision, err := insertApplicabilityParentV2(ctx, tx, rec)
		if err != nil {
			return 0, 0, err
		}
		return id, revision, insertApplicabilityRulesV2(ctx, tx, id, rec.Rules)
	})
	if err != nil {
		return domain.CatalogWriteResult{}, err
	}
	result := rec
	result.ID, result.Revision = id, revision
	return domain.NewCatalogWriteResult(&result, catalog), nil
}

func updateApplicabilityAggregateV2(ctx context.Context, pool *pgxpool.Pool, rec domain.CatalogRecord, expectedRevision uint64) (domain.CatalogWriteResult, error) {
	if err := requireApplicabilityRules(rec); err != nil {
		return domain.CatalogWriteResult{}, err
	}
	id, revision, catalog, err := runV2CoherentTx(ctx, pool, updateMutationV2(rec), func(ctx context.Context, tx pgx.Tx) (int64, uint64, error) {
		revision, err := updateApplicabilityParentV2(ctx, tx, rec, expectedRevision)
		if err != nil {
			return 0, 0, err
		}
		return rec.ID, revision, replaceApplicabilityRulesV2(ctx, tx, rec.ID, rec.Rules)
	})
	if err != nil {
		return domain.CatalogWriteResult{}, err
	}
	result := rec
	result.ID, result.Revision = id, revision
	return domain.NewCatalogWriteResult(&result, catalog), nil
}

// --- stage-3F CAS transaction helpers for CLASE, FAMILIA, TIPO,
// CARACTERISTICA, and CONJUNTO_OPCIONES; dormant, NOT wired into
// domain.CatalogAdminRepositoryV2. Unlike 3E's APLICABILIDAD aggregate,
// these single-row kinds carry no child rows to reload/validate/compare, so
// this slice's scope (task 3F) is bounded to revision-aware CAS insert/
// update only — the full reload/validate/compare/publish protocol is
// deferred to the concrete CatalogAdminRepositoryV2 methods built in 3G.

var errCatalogRevisionRequiredV2 = errors.New("catalog update requires a non-zero expected revision")

// errCatalogStaleRevisionV2 aliases domain.ErrRevisionConflict directly
// (stage-5 fix, resource-master-core-write-update): the "future" the prior
// comment named has arrived. Kept as a local name — not deleted — because
// casUpdateRevision and its 20+ call sites/tests below still read most
// clearly with the local, purpose-scoped identifier.
var errCatalogStaleRevisionV2 = domain.ErrRevisionConflict

// casUpdateRevision executes updateSQL (which must end
// "... WHERE <key predicate> AND revision=$expected RETURNING revision")
// and disambiguates a zero-row result via a same-transaction row-locked
// existence/revision lookup: absent maps to domain.ErrCatalogRecordNotFound,
// present-but-different maps to errCatalogStaleRevisionV2 — never inferred
// from error text, mirroring updateApplicabilityParentV2's protocol.
func casUpdateRevision(ctx context.Context, tx pgx.Tx, updateSQL string, args []any, selectRevisionSQL string, selectArg int64, expectedRevision uint64) (uint64, error) {
	if expectedRevision == 0 {
		return 0, errCatalogRevisionRequiredV2
	}
	var revision uint64
	err := tx.QueryRow(ctx, updateSQL, args...).Scan(&revision)
	if err == nil {
		return revision, nil
	}
	if !isNoRows(err) {
		return 0, mapCatalogWriteError(fmt.Errorf("cas update: %w", err))
	}
	var current uint64
	lookupErr := tx.QueryRow(ctx, selectRevisionSQL, selectArg).Scan(&current)
	if isNoRows(lookupErr) {
		return 0, domain.ErrCatalogRecordNotFound
	}
	if lookupErr != nil {
		return 0, fmt.Errorf("resolve revision: %w", lookupErr)
	}
	return 0, errCatalogStaleRevisionV2
}

// runCatalogWriteTxV2 begins/commits/rolls back a transaction around one
// insert/update apply closure — no reload/validate/compare steps (see the
// section doc comment above for why this 3F-scoped slice omits them; 4B's
// runV2CoherentTx below is the full coherent-result protocol every concrete
// catalogAdminRepositoryV2 method now uses instead).
func runCatalogWriteTxV2(ctx context.Context, pool *pgxpool.Pool, apply func(context.Context, pgx.Tx) (int64, uint64, error)) (int64, uint64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("begin catalog v2 transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	id, revision, err := apply(ctx, tx)
	if err != nil {
		return 0, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, fmt.Errorf("commit catalog v2 transaction: %w", err)
	}
	return id, revision, nil
}

// --- CLASE ------------------------------------------------------------

func insertClassV2(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) (id int64, revision uint64, err error) {
	err = tx.QueryRow(ctx, `
		INSERT INTO public.resource_classes (code, name, plural, slug, display_order, aliases, keywords, active)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id, revision`,
		fieldText(rec, "code"), fieldText(rec, "name"), fieldText(rec, "plural"), fieldText(rec, "slug"),
		fieldInt(rec, "order"), fieldList(rec, "aliases"), fieldList(rec, "keywords"), rec.Active).Scan(&id, &revision)
	if err != nil {
		return 0, 0, mapCatalogWriteError(fmt.Errorf("insert resource_classes: %w", err))
	}
	return id, revision, nil
}

// updateClassV2 mirrors updateClass's (catalog_admin_kinds.go) D11-layer-2
// defense-in-depth guard: once referenced by a resource, código is omitted
// from the generated SET clause, even under this dormant V2 CAS path.
func updateClassV2(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord, expectedRevision uint64) (uint64, error) {
	referenced, err := classReferencedByResources(ctx, tx, rec.ID)
	if err != nil {
		return 0, err
	}
	if referenced {
		return casUpdateRevision(ctx, tx, `
			UPDATE public.resource_classes SET name=$1, plural=$2, slug=$3, display_order=$4, aliases=$5, keywords=$6, active=$7,
			    revision = revision + 1, updated_at = NOW()
			WHERE id=$8 AND revision=$9 RETURNING revision`,
			[]any{fieldText(rec, "name"), fieldText(rec, "plural"), fieldText(rec, "slug"),
				fieldInt(rec, "order"), fieldList(rec, "aliases"), fieldList(rec, "keywords"), rec.Active, rec.ID, expectedRevision},
			`SELECT revision FROM public.resource_classes WHERE id=$1 FOR UPDATE`, rec.ID, expectedRevision)
	}
	return casUpdateRevision(ctx, tx, `
		UPDATE public.resource_classes SET code=$1, name=$2, plural=$3, slug=$4, display_order=$5, aliases=$6, keywords=$7, active=$8,
		    revision = revision + 1, updated_at = NOW()
		WHERE id=$9 AND revision=$10 RETURNING revision`,
		[]any{fieldText(rec, "code"), fieldText(rec, "name"), fieldText(rec, "plural"), fieldText(rec, "slug"),
			fieldInt(rec, "order"), fieldList(rec, "aliases"), fieldList(rec, "keywords"), rec.Active, rec.ID, expectedRevision},
		`SELECT revision FROM public.resource_classes WHERE id=$1 FOR UPDATE`, rec.ID, expectedRevision)
}

// --- FAMILIA ------------------------------------------------------------

func insertFamilyV2(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) (id int64, revision uint64, err error) {
	classID, err := resolveClassID(ctx, tx, fieldRef(rec, "class"))
	if err != nil {
		return 0, 0, err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO public.resource_families (class_id, code, name, active)
		VALUES ($1,$2,$3,$4) RETURNING id, revision`,
		classID, fieldText(rec, "code"), fieldText(rec, "name"), rec.Active).Scan(&id, &revision)
	if err != nil {
		return 0, 0, mapCatalogWriteError(fmt.Errorf("insert resource_families: %w", err))
	}
	return id, revision, nil
}

// updateFamilyV2 mirrors updateFamily's D11-layer-2 código guard (see
// updateClassV2's doc comment).
func updateFamilyV2(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord, expectedRevision uint64) (uint64, error) {
	classID, err := resolveClassID(ctx, tx, fieldRef(rec, "class"))
	if err != nil {
		return 0, err
	}
	referenced, err := familyReferencedByResources(ctx, tx, rec.ID)
	if err != nil {
		return 0, err
	}
	if referenced {
		return casUpdateRevision(ctx, tx, `
			UPDATE public.resource_families SET class_id=$1, name=$2, active=$3, revision = revision + 1, updated_at = NOW()
			WHERE id=$4 AND revision=$5 RETURNING revision`,
			[]any{classID, fieldText(rec, "name"), rec.Active, rec.ID, expectedRevision},
			`SELECT revision FROM public.resource_families WHERE id=$1 FOR UPDATE`, rec.ID, expectedRevision)
	}
	return casUpdateRevision(ctx, tx, `
		UPDATE public.resource_families SET class_id=$1, code=$2, name=$3, active=$4, revision = revision + 1, updated_at = NOW()
		WHERE id=$5 AND revision=$6 RETURNING revision`,
		[]any{classID, fieldText(rec, "code"), fieldText(rec, "name"), rec.Active, rec.ID, expectedRevision},
		`SELECT revision FROM public.resource_families WHERE id=$1 FOR UPDATE`, rec.ID, expectedRevision)
}

// --- TIPO ---------------------------------------------------------------

func insertTypeV2(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) (id int64, revision uint64, err error) {
	familyID, classID, err := resolveFamilyID(ctx, tx, fieldRef(rec, "class"), fieldRef(rec, "family"))
	if err != nil {
		return 0, 0, err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO public.resource_types (class_id, family_id, code, name, active)
		VALUES ($1,$2,$3,$4,$5) RETURNING id, revision`,
		classID, familyID, fieldText(rec, "code"), fieldText(rec, "name"), rec.Active).Scan(&id, &revision)
	if err != nil {
		return 0, 0, mapCatalogWriteError(fmt.Errorf("insert resource_types: %w", err))
	}
	return id, revision, nil
}

// updateTypeV2 mirrors updateType's D11-layer-2 código guard (see
// updateClassV2's doc comment).
func updateTypeV2(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord, expectedRevision uint64) (uint64, error) {
	familyID, classID, err := resolveFamilyID(ctx, tx, fieldRef(rec, "class"), fieldRef(rec, "family"))
	if err != nil {
		return 0, err
	}
	referenced, err := typeReferencedByResources(ctx, tx, rec.ID)
	if err != nil {
		return 0, err
	}
	if referenced {
		return casUpdateRevision(ctx, tx, `
			UPDATE public.resource_types SET class_id=$1, family_id=$2, name=$3, active=$4, revision = revision + 1, updated_at = NOW()
			WHERE id=$5 AND revision=$6 RETURNING revision`,
			[]any{classID, familyID, fieldText(rec, "name"), rec.Active, rec.ID, expectedRevision},
			`SELECT revision FROM public.resource_types WHERE id=$1 FOR UPDATE`, rec.ID, expectedRevision)
	}
	return casUpdateRevision(ctx, tx, `
		UPDATE public.resource_types SET class_id=$1, family_id=$2, code=$3, name=$4, active=$5, revision = revision + 1, updated_at = NOW()
		WHERE id=$6 AND revision=$7 RETURNING revision`,
		[]any{classID, familyID, fieldText(rec, "code"), fieldText(rec, "name"), rec.Active, rec.ID, expectedRevision},
		`SELECT revision FROM public.resource_types WHERE id=$1 FOR UPDATE`, rec.ID, expectedRevision)
}

// --- CARACTERISTICA -------------------------------------------------------

func insertDefinitionV2(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) (id int64, revision uint64, err error) {
	err = tx.QueryRow(ctx, `
		INSERT INTO public.attribute_definitions (code, name, value_type, dimension, default_identity_participates, active)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id, revision`,
		fieldText(rec, "code"), fieldText(rec, "name"), fieldText(rec, "valueType"),
		nullableString(fieldText(rec, "dimension")), fieldBool(rec, "defaultIdentityParticipates"), rec.Active).Scan(&id, &revision)
	if err != nil {
		return 0, 0, mapCatalogWriteError(fmt.Errorf("insert attribute_definitions: %w", err))
	}
	return id, revision, nil
}

// updateDefinitionV2 mirrors updateDefinition's D11-layer-2 código guard (see
// updateClassV2's doc comment).
func updateDefinitionV2(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord, expectedRevision uint64) (uint64, error) {
	referenced, err := definitionReferencedByResources(ctx, tx, rec.ID)
	if err != nil {
		return 0, err
	}
	if referenced {
		return casUpdateRevision(ctx, tx, `
			UPDATE public.attribute_definitions SET name=$1, value_type=$2, dimension=$3, default_identity_participates=$4, active=$5,
			    revision = revision + 1, updated_at = NOW()
			WHERE id=$6 AND revision=$7 RETURNING revision`,
			[]any{fieldText(rec, "name"), fieldText(rec, "valueType"),
				nullableString(fieldText(rec, "dimension")), fieldBool(rec, "defaultIdentityParticipates"), rec.Active, rec.ID, expectedRevision},
			`SELECT revision FROM public.attribute_definitions WHERE id=$1 FOR UPDATE`, rec.ID, expectedRevision)
	}
	return casUpdateRevision(ctx, tx, `
		UPDATE public.attribute_definitions SET code=$1, name=$2, value_type=$3, dimension=$4, default_identity_participates=$5, active=$6,
		    revision = revision + 1, updated_at = NOW()
		WHERE id=$7 AND revision=$8 RETURNING revision`,
		[]any{fieldText(rec, "code"), fieldText(rec, "name"), fieldText(rec, "valueType"),
			nullableString(fieldText(rec, "dimension")), fieldBool(rec, "defaultIdentityParticipates"), rec.Active, rec.ID, expectedRevision},
		`SELECT revision FROM public.attribute_definitions WHERE id=$1 FOR UPDATE`, rec.ID, expectedRevision)
}

// --- CONJUNTO_OPCIONES ----------------------------------------------------
//
// resource_option_sets' natural PK is code; migration 000009 added a real
// BIGSERIAL id column (UNIQUE, additive), so id stays a stable predicate
// even in the same statement that also SETs a new code.

func insertOptionSetV2(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) (id int64, revision uint64, err error) {
	err = tx.QueryRow(ctx, `
		INSERT INTO public.resource_option_sets (code, name, active) VALUES ($1,$2,$3)
		RETURNING id, revision`,
		fieldText(rec, "code"), fieldText(rec, "name"), rec.Active).Scan(&id, &revision)
	if err != nil {
		return 0, 0, mapCatalogWriteError(fmt.Errorf("insert resource_option_sets: %w", err))
	}
	return id, revision, nil
}

// updateOptionSetV2 mirrors updateOptionSet's D11-layer-2 código guard (see
// updateClassV2's doc comment).
func updateOptionSetV2(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord, expectedRevision uint64) (uint64, error) {
	referenced, err := optionSetReferencedByResources(ctx, tx, rec.ID)
	if err != nil {
		return 0, err
	}
	if referenced {
		return casUpdateRevision(ctx, tx, `
			UPDATE public.resource_option_sets SET name=$1, active=$2, revision = revision + 1, updated_at = NOW()
			WHERE id=$3 AND revision=$4 RETURNING revision`,
			[]any{fieldText(rec, "name"), rec.Active, rec.ID, expectedRevision},
			`SELECT revision FROM public.resource_option_sets WHERE id=$1 FOR UPDATE`, rec.ID, expectedRevision)
	}
	return casUpdateRevision(ctx, tx, `
		UPDATE public.resource_option_sets SET code=$1, name=$2, active=$3, revision = revision + 1, updated_at = NOW()
		WHERE id=$4 AND revision=$5 RETURNING revision`,
		[]any{fieldText(rec, "code"), fieldText(rec, "name"), rec.Active, rec.ID, expectedRevision},
		`SELECT revision FROM public.resource_option_sets WHERE id=$1 FOR UPDATE`, rec.ID, expectedRevision)
}

// --- stage-3G1 dormant CAS helpers for OPCION, RELACION_OPCIONES, UNIDAD,
// POLITICA_UNIDAD, PRESENTACION (create+update only; not wired into
// domain.CatalogAdminRepositoryV2). OPCION/UNIDAD have codeField() and
// replicate the D11-layer-2 guard from the start (unlike 3F); the other 3
// kinds have no código field.

// --- UNIDAD -----------------------------------------------------------

func insertUnitV2(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) (id int64, revision uint64, err error) {
	err = tx.QueryRow(ctx, `
		INSERT INTO public.unit_definitions (code, name, symbol, dimension, active) VALUES ($1,$2,$3,$4,$5) RETURNING id, revision`,
		fieldText(rec, "code"), fieldText(rec, "name"), fieldText(rec, "symbol"), fieldText(rec, "dimension"), rec.Active).Scan(&id, &revision)
	if err != nil {
		return 0, 0, mapCatalogWriteError(fmt.Errorf("insert unit_definitions: %w", err))
	}
	return id, revision, nil
}

// updateUnitV2 mirrors updateUnit's D11-layer-2 código guard.
func updateUnitV2(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord, expectedRevision uint64) (uint64, error) {
	referenced, err := unitReferencedByResources(ctx, tx, rec.ID)
	if err != nil {
		return 0, err
	}
	if referenced {
		return casUpdateRevision(ctx, tx, `
			UPDATE public.unit_definitions SET name=$1, symbol=$2, dimension=$3, active=$4, revision = revision + 1, updated_at = NOW()
			WHERE id=$5 AND revision=$6 RETURNING revision`,
			[]any{fieldText(rec, "name"), fieldText(rec, "symbol"), fieldText(rec, "dimension"), rec.Active, rec.ID, expectedRevision},
			`SELECT revision FROM public.unit_definitions WHERE id=$1 FOR UPDATE`, rec.ID, expectedRevision)
	}
	return casUpdateRevision(ctx, tx, `
		UPDATE public.unit_definitions SET code=$1, name=$2, symbol=$3, dimension=$4, active=$5, revision = revision + 1, updated_at = NOW()
		WHERE id=$6 AND revision=$7 RETURNING revision`,
		[]any{fieldText(rec, "code"), fieldText(rec, "name"), fieldText(rec, "symbol"), fieldText(rec, "dimension"), rec.Active, rec.ID, expectedRevision},
		`SELECT revision FROM public.unit_definitions WHERE id=$1 FOR UPDATE`, rec.ID, expectedRevision)
}

// --- OPCION -------------------------------------------------------------
//
// attribute_options' natural PK is (option_set, characteristic código,
// código); migration 000009 added a real BIGSERIAL id column (UNIQUE,
// additive).

func insertOptionV2(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) (id int64, revision uint64, err error) {
	err = tx.QueryRow(ctx, `
		INSERT INTO public.attribute_options (option_set, attribute_definition_id, code, label, active, display_order)
		SELECT $1, d.id, $3, $4, $5,
		       COALESCE((SELECT MAX(display_order)+1 FROM public.attribute_options WHERE option_set=$1 AND attribute_definition_id=d.id), 0)
		FROM public.attribute_definitions d WHERE d.code=$2
		RETURNING id, revision`,
		fieldRef(rec, "optionSet"), fieldRef(rec, "characteristic"), fieldText(rec, "code"), fieldText(rec, "label"), rec.Active).Scan(&id, &revision)
	if isNoRows(err) {
		return 0, 0, fmt.Errorf("%w: characteristic %q or option set %q", domain.ErrCatalogReference, fieldRef(rec, "characteristic"), fieldRef(rec, "optionSet"))
	}
	if err != nil {
		return 0, 0, mapCatalogWriteError(fmt.Errorf("insert attribute_options: %w", err))
	}
	return id, revision, nil
}

// updateOptionV2 mirrors updateOption's D11-layer-2 código guard; the CAS
// predicate/row-lock resolve through the real id column.
func updateOptionV2(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord, expectedRevision uint64) (uint64, error) {
	var optionSet, defCode, code string
	err := tx.QueryRow(ctx, `
		SELECT ao.option_set, d.code, ao.code
		FROM public.attribute_options ao JOIN public.attribute_definitions d ON d.id = ao.attribute_definition_id
		WHERE ao.id = $1`, rec.ID).Scan(&optionSet, &defCode, &code)
	if isNoRows(err) {
		return 0, domain.ErrCatalogRecordNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("resolve attribute_options: %w", err)
	}
	referenced, err := optionReferencedByResources(ctx, tx, rec.ID)
	if err != nil {
		return 0, err
	}
	selectRevisionSQL := `SELECT revision FROM public.attribute_options WHERE id = $1 FOR UPDATE`
	if referenced {
		return casUpdateRevision(ctx, tx, `
			UPDATE public.attribute_options SET label=$1, active=$2, revision = revision + 1, updated_at = NOW()
			WHERE option_set=$3 AND attribute_definition_id=(SELECT id FROM public.attribute_definitions WHERE code=$4) AND code=$5 AND revision=$6
			RETURNING revision`,
			[]any{fieldText(rec, "label"), rec.Active, optionSet, defCode, code, expectedRevision},
			selectRevisionSQL, rec.ID, expectedRevision)
	}
	return casUpdateRevision(ctx, tx, `
		UPDATE public.attribute_options SET code=$1, label=$2, active=$3, revision = revision + 1, updated_at = NOW()
		WHERE option_set=$4 AND attribute_definition_id=(SELECT id FROM public.attribute_definitions WHERE code=$5) AND code=$6 AND revision=$7
		RETURNING revision`,
		[]any{fieldText(rec, "code"), fieldText(rec, "label"), rec.Active, optionSet, defCode, code, expectedRevision},
		selectRevisionSQL, rec.ID, expectedRevision)
}

// --- RELACION_OPCIONES ---------------------------------------------------
// No código field (junction kind, D11 guard inapplicable).

func insertOptionRelationV2(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) (id int64, revision uint64, err error) {
	err = tx.QueryRow(ctx, `
		INSERT INTO public.attribute_option_relations (option_set, from_attribute_definition_id, from_option_code, to_attribute_definition_id, to_option_code, active)
		SELECT $1, fd.id, $3, td.id, $5, $6
		FROM public.attribute_definitions fd, public.attribute_definitions td
		WHERE fd.code=$2 AND td.code=$4
		RETURNING id, revision`,
		fieldRef(rec, "optionSet"), fieldRef(rec, "fromCharacteristic"), fieldRef(rec, "fromOption"),
		fieldRef(rec, "toCharacteristic"), fieldRef(rec, "toOption"), rec.Active).Scan(&id, &revision)
	if isNoRows(err) {
		return 0, 0, fmt.Errorf("%w: characteristic %q or %q", domain.ErrCatalogReference, fieldRef(rec, "fromCharacteristic"), fieldRef(rec, "toCharacteristic"))
	}
	if err != nil {
		return 0, 0, mapCatalogWriteError(fmt.Errorf("insert attribute_option_relations: %w", err))
	}
	return id, revision, nil
}

func updateOptionRelationV2(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord, expectedRevision uint64) (uint64, error) {
	return casUpdateRevision(ctx, tx, `
		UPDATE public.attribute_option_relations
		SET option_set=$1,
		    from_attribute_definition_id=(SELECT id FROM public.attribute_definitions WHERE code=$2), from_option_code=$3,
		    to_attribute_definition_id=(SELECT id FROM public.attribute_definitions WHERE code=$4), to_option_code=$5,
		    active=$6, revision = revision + 1, updated_at = NOW()
		WHERE id=$7 AND revision=$8 RETURNING revision`,
		[]any{fieldRef(rec, "optionSet"), fieldRef(rec, "fromCharacteristic"), fieldRef(rec, "fromOption"),
			fieldRef(rec, "toCharacteristic"), fieldRef(rec, "toOption"), rec.Active, rec.ID, expectedRevision},
		`SELECT revision FROM public.attribute_option_relations WHERE id=$1 FOR UPDATE`, rec.ID, expectedRevision)
}

// --- POLITICA_UNIDAD ("Política de Unidad") -------------------------------
// Natural PK is (family_id, unit_id); migration 000009 added a real
// BIGSERIAL id column (UNIQUE, additive). No código field (junction kind).

func insertUnitPolicyV2(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) (id int64, revision uint64, err error) {
	err = tx.QueryRow(ctx, `
		INSERT INTO public.resource_unit_policies (family_id, unit_id, allowed, suggested, active)
		SELECT f.id, u.id, $4, $5, $6
		FROM public.resource_families f JOIN public.resource_classes cl ON cl.id = f.class_id, public.unit_definitions u
		WHERE cl.code=$1 AND f.code=$2 AND u.code=$3
		RETURNING id, revision`,
		fieldRef(rec, "class"), fieldRef(rec, "family"), fieldRef(rec, "unit"), fieldBool(rec, "allowed"), fieldBool(rec, "suggested"), rec.Active).Scan(&id, &revision)
	if isNoRows(err) {
		return 0, 0, fmt.Errorf("%w: class %q, family %q, or unit %q", domain.ErrCatalogReference, fieldRef(rec, "class"), fieldRef(rec, "family"), fieldRef(rec, "unit"))
	}
	if err != nil {
		return 0, 0, mapCatalogWriteError(fmt.Errorf("insert resource_unit_policies: %w", err))
	}
	return id, revision, nil
}

// updateUnitPolicyV2 only mutates allowed/suggested/active, mirroring
// legacy updateUnitPolicy (composite PK; re-scoping is delete+insert).
func updateUnitPolicyV2(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord, expectedRevision uint64) (uint64, error) {
	var familyID, unitID int64
	err := tx.QueryRow(ctx, `SELECT family_id, unit_id FROM public.resource_unit_policies WHERE id = $1`, rec.ID).Scan(&familyID, &unitID)
	if isNoRows(err) {
		return 0, domain.ErrCatalogRecordNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("resolve resource_unit_policies: %w", err)
	}
	return casUpdateRevision(ctx, tx, `
		UPDATE public.resource_unit_policies SET allowed=$1, suggested=$2, active=$3, revision = revision + 1, updated_at = NOW()
		WHERE family_id=$4 AND unit_id=$5 AND revision=$6 RETURNING revision`,
		[]any{fieldBool(rec, "allowed"), fieldBool(rec, "suggested"), rec.Active, familyID, unitID, expectedRevision},
		`SELECT revision FROM public.resource_unit_policies WHERE id = $1 FOR UPDATE`, rec.ID, expectedRevision)
}

// --- PRESENTACION ("Campo de Presentación") -------------------------------
// Natural PK is (type_id, attribute_definition_id); migration 000009 added
// a real BIGSERIAL id column (UNIQUE, additive). No código field.

func insertPresentationFieldV2(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) (id int64, revision uint64, err error) {
	err = tx.QueryRow(ctx, `
		INSERT INTO public.resource_type_presentation_fields (type_id, attribute_definition_id, position, active)
		SELECT t.id, d.id, $5, $6
		FROM public.resource_types t
		JOIN public.resource_families f ON f.id = t.family_id
		JOIN public.resource_classes cl ON cl.id = t.class_id, public.attribute_definitions d
		WHERE cl.code=$1 AND f.code=$2 AND t.code=$3 AND d.code=$4
		RETURNING id, revision`,
		fieldRef(rec, "class"), fieldRef(rec, "family"), fieldRef(rec, "type"), fieldRef(rec, "characteristic"), fieldInt(rec, "position"), rec.Active).Scan(&id, &revision)
	if isNoRows(err) {
		return 0, 0, fmt.Errorf("%w: type %q or characteristic %q", domain.ErrCatalogReference, fieldRef(rec, "type"), fieldRef(rec, "characteristic"))
	}
	if err != nil {
		return 0, 0, mapCatalogWriteError(fmt.Errorf("insert resource_type_presentation_fields: %w", err))
	}
	return id, revision, nil
}

// updatePresentationFieldV2 only mutates position/active, mirroring legacy
// updatePresentationField (composite PK).
func updatePresentationFieldV2(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord, expectedRevision uint64) (uint64, error) {
	var typeID, definitionID int64
	err := tx.QueryRow(ctx, `SELECT type_id, attribute_definition_id FROM public.resource_type_presentation_fields WHERE id = $1`, rec.ID).Scan(&typeID, &definitionID)
	if isNoRows(err) {
		return 0, domain.ErrCatalogRecordNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("resolve resource_type_presentation_fields: %w", err)
	}
	return casUpdateRevision(ctx, tx, `
		UPDATE public.resource_type_presentation_fields SET position=$1, active=$2, revision = revision + 1, updated_at = NOW()
		WHERE type_id=$3 AND attribute_definition_id=$4 AND revision=$5 RETURNING revision`,
		[]any{fieldInt(rec, "position"), rec.Active, typeID, definitionID, expectedRevision},
		`SELECT revision FROM public.resource_type_presentation_fields WHERE id = $1 FOR UPDATE`, rec.ID, expectedRevision)
}

// --- stage-3G2 dormant lifecycle (SetActive) and delete CAS helpers for all
// 11 kinds; dormant, NOT wired into domain.CatalogAdminRepositoryV2. Mirrors
// the exact legacy setActive/delete SQL shapes (catalog_admin_kinds.go) plus
// a revision predicate and casUpdateRevision/casDeleteRevision's zero-row
// disambiguation. APLICABILIDAD reuses the plain simple-id shape: its
// statements only ever touch public.resource_attributes, never
// public.resource_attribute_rules — design.md's "applicability lifecycle
// changes only the parent active/revision while rules remain intact" (see
// the integration test's dedicated rule-preservation subtest).

// casDeleteRevision mirrors casUpdateRevision for DELETE: deleteSQL must end
// "... WHERE <key predicate> AND revision=$expected" (no RETURNING needed).
// A foreign-key violation surfaces from tx.Exec and is mapped by
// mapCatalogDeleteError (ErrCatalogInUse) — no dependency counting is
// reinvented here; the database FK is the backstop.
func casDeleteRevision(ctx context.Context, tx pgx.Tx, deleteSQL string, args []any, selectRevisionSQL string, selectArg int64, expectedRevision uint64) error {
	if expectedRevision == 0 {
		return errCatalogRevisionRequiredV2
	}
	tag, err := tx.Exec(ctx, deleteSQL, args...)
	if err != nil {
		return mapCatalogDeleteError(fmt.Errorf("cas delete: %w", err))
	}
	if tag.RowsAffected() > 0 {
		return nil
	}
	var current uint64
	lookupErr := tx.QueryRow(ctx, selectRevisionSQL, selectArg).Scan(&current)
	if isNoRows(lookupErr) {
		return domain.ErrCatalogRecordNotFound
	}
	if lookupErr != nil {
		return fmt.Errorf("resolve revision: %w", lookupErr)
	}
	return errCatalogStaleRevisionV2
}

// simpleSetActiveV2/simpleDeleteV2 build the CAS setActive/delete closures
// for the 7 kinds with a real BIGSERIAL id column (Class, Family, Type,
// AttributeDefinition, OptionRelation, Unit, AttributeBinding) — the V2
// sibling of catalog_admin_repository.go's legacy simpleSetActive/
// simpleDelete. table/idCol are always internal constants, never derived
// from caller input.
func simpleSetActiveV2(table, idCol string) func(ctx context.Context, tx pgx.Tx, id int64, active bool, expectedRevision uint64) (uint64, error) {
	return func(ctx context.Context, tx pgx.Tx, id int64, active bool, expectedRevision uint64) (uint64, error) {
		return casUpdateRevision(ctx, tx,
			fmt.Sprintf(`UPDATE %s SET active=$1, revision=revision+1, updated_at=NOW() WHERE %s=$2 AND revision=$3 RETURNING revision`, table, idCol),
			[]any{active, id, expectedRevision},
			fmt.Sprintf(`SELECT revision FROM %s WHERE %s=$1 FOR UPDATE`, table, idCol), id, expectedRevision)
	}
}

func simpleDeleteV2(table, idCol string) func(ctx context.Context, tx pgx.Tx, id int64, expectedRevision uint64) error {
	return func(ctx context.Context, tx pgx.Tx, id int64, expectedRevision uint64) error {
		return casDeleteRevision(ctx, tx,
			fmt.Sprintf(`DELETE FROM %s WHERE %s=$1 AND revision=$2`, table, idCol),
			[]any{id, expectedRevision},
			fmt.Sprintf(`SELECT revision FROM %s WHERE %s=$1 FOR UPDATE`, table, idCol), id, expectedRevision)
	}
}

var (
	setActiveClassV2          = simpleSetActiveV2("public.resource_classes", "id")
	deleteClassV2             = simpleDeleteV2("public.resource_classes", "id")
	setActiveFamilyV2         = simpleSetActiveV2("public.resource_families", "id")
	deleteFamilyV2            = simpleDeleteV2("public.resource_families", "id")
	setActiveTypeV2           = simpleSetActiveV2("public.resource_types", "id")
	deleteTypeV2              = simpleDeleteV2("public.resource_types", "id")
	setActiveDefinitionV2     = simpleSetActiveV2("public.attribute_definitions", "id")
	deleteDefinitionV2        = simpleDeleteV2("public.attribute_definitions", "id")
	setActiveOptionRelationV2 = simpleSetActiveV2("public.attribute_option_relations", "id")
	deleteOptionRelationV2    = simpleDeleteV2("public.attribute_option_relations", "id")
	setActiveUnitV2           = simpleSetActiveV2("public.unit_definitions", "id")
	deleteUnitV2              = simpleDeleteV2("public.unit_definitions", "id")
	setActiveApplicabilityV2  = simpleSetActiveV2("public.resource_attributes", "id")
	deleteApplicabilityV2     = simpleDeleteV2("public.resource_attributes", "id")
)

// CONJUNTO_OPCIONES / OPCION / POLITICA_UNIDAD / PRESENTACION lifecycle and
// delete: migration 000009 gave each a real BIGSERIAL id column, so these
// now address by id like every other kind, plus the revision predicate.

func setActiveOptionSetV2(ctx context.Context, tx pgx.Tx, id int64, active bool, expectedRevision uint64) (uint64, error) {
	return casUpdateRevision(ctx, tx,
		`UPDATE public.resource_option_sets SET active=$1, revision=revision+1, updated_at=NOW()
		 WHERE id=$2 AND revision=$3 RETURNING revision`,
		[]any{active, id, expectedRevision},
		`SELECT revision FROM public.resource_option_sets WHERE id=$1 FOR UPDATE`, id, expectedRevision)
}

func deleteOptionSetV2(ctx context.Context, tx pgx.Tx, id int64, expectedRevision uint64) error {
	return casDeleteRevision(ctx, tx,
		`DELETE FROM public.resource_option_sets WHERE id=$1 AND revision=$2`,
		[]any{id, expectedRevision},
		`SELECT revision FROM public.resource_option_sets WHERE id=$1 FOR UPDATE`, id, expectedRevision)
}

func setActiveOptionV2(ctx context.Context, tx pgx.Tx, id int64, active bool, expectedRevision uint64) (uint64, error) {
	return casUpdateRevision(ctx, tx,
		`UPDATE public.attribute_options SET active=$1, revision=revision+1, updated_at=NOW()
		 WHERE id=$2 AND revision=$3 RETURNING revision`,
		[]any{active, id, expectedRevision},
		`SELECT revision FROM public.attribute_options WHERE id=$1 FOR UPDATE`, id, expectedRevision)
}

func deleteOptionV2(ctx context.Context, tx pgx.Tx, id int64, expectedRevision uint64) error {
	return casDeleteRevision(ctx, tx,
		`DELETE FROM public.attribute_options WHERE id=$1 AND revision=$2`,
		[]any{id, expectedRevision},
		`SELECT revision FROM public.attribute_options WHERE id=$1 FOR UPDATE`, id, expectedRevision)
}

func setActiveUnitPolicyV2(ctx context.Context, tx pgx.Tx, id int64, active bool, expectedRevision uint64) (uint64, error) {
	return casUpdateRevision(ctx, tx,
		`UPDATE public.resource_unit_policies SET active=$1, revision=revision+1, updated_at=NOW()
		 WHERE id=$2 AND revision=$3 RETURNING revision`,
		[]any{active, id, expectedRevision},
		`SELECT revision FROM public.resource_unit_policies WHERE id=$1 FOR UPDATE`, id, expectedRevision)
}

func deleteUnitPolicyV2(ctx context.Context, tx pgx.Tx, id int64, expectedRevision uint64) error {
	return casDeleteRevision(ctx, tx,
		`DELETE FROM public.resource_unit_policies WHERE id=$1 AND revision=$2`,
		[]any{id, expectedRevision},
		`SELECT revision FROM public.resource_unit_policies WHERE id=$1 FOR UPDATE`, id, expectedRevision)
}

func setActivePresentationFieldV2(ctx context.Context, tx pgx.Tx, id int64, active bool, expectedRevision uint64) (uint64, error) {
	return casUpdateRevision(ctx, tx,
		`UPDATE public.resource_type_presentation_fields SET active=$1, revision=revision+1, updated_at=NOW()
		 WHERE id=$2 AND revision=$3 RETURNING revision`,
		[]any{active, id, expectedRevision},
		`SELECT revision FROM public.resource_type_presentation_fields WHERE id=$1 FOR UPDATE`, id, expectedRevision)
}

func deletePresentationFieldV2(ctx context.Context, tx pgx.Tx, id int64, expectedRevision uint64) error {
	return casDeleteRevision(ctx, tx,
		`DELETE FROM public.resource_type_presentation_fields WHERE id=$1 AND revision=$2`,
		[]any{id, expectedRevision},
		`SELECT revision FROM public.resource_type_presentation_fields WHERE id=$1 FOR UPDATE`, id, expectedRevision)
}

// --- stage-3G3 concrete domain.CatalogAdminRepositoryV2 adapter: dispatches
// generically by domain.CatalogKindCode to the per-kind CAS helpers built
// across 3E-3G2. Still dormant — no service/TUI composition root references
// NewCatalogAdminRepositoryV2 (design "Catalog transaction and coherent
// publication").

// catalogWriteHandlersV2 bundles one kind's tx-level insert/update/setActive/
// delete closures — the V2 sibling of catalog_admin_repository.go's
// kindHandlers. KindAttributeBinding's insert/update are intentionally left
// nil here: catalogAdminRepositoryV2.Insert/Update special-case that kind to
// call insertApplicabilityAggregateV2/updateApplicabilityAggregateV2 directly
// (3E), which already run the whole begin/apply/reload/validate/compare/
// commit protocol for its child rule rows; its setActive/del reuse the plain
// simple-id shape here since those never touch resource_attribute_rules.
type catalogWriteHandlersV2 struct {
	insert    func(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) (int64, uint64, error)
	update    func(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord, expectedRevision uint64) (uint64, error)
	setActive func(ctx context.Context, tx pgx.Tx, id int64, active bool, expectedRevision uint64) (uint64, error)
	del       func(ctx context.Context, tx pgx.Tx, id int64, expectedRevision uint64) error
}

var kindTablesV2 = map[domain.CatalogKindCode]catalogWriteHandlersV2{
	domain.KindClass:               {insert: insertClassV2, update: updateClassV2, setActive: setActiveClassV2, del: deleteClassV2},
	domain.KindFamily:              {insert: insertFamilyV2, update: updateFamilyV2, setActive: setActiveFamilyV2, del: deleteFamilyV2},
	domain.KindType:                {insert: insertTypeV2, update: updateTypeV2, setActive: setActiveTypeV2, del: deleteTypeV2},
	domain.KindAttributeDefinition: {insert: insertDefinitionV2, update: updateDefinitionV2, setActive: setActiveDefinitionV2, del: deleteDefinitionV2},
	domain.KindOptionSet:           {insert: insertOptionSetV2, update: updateOptionSetV2, setActive: setActiveOptionSetV2, del: deleteOptionSetV2},
	domain.KindOption:              {insert: insertOptionV2, update: updateOptionV2, setActive: setActiveOptionV2, del: deleteOptionV2},
	domain.KindOptionRelation:      {insert: insertOptionRelationV2, update: updateOptionRelationV2, setActive: setActiveOptionRelationV2, del: deleteOptionRelationV2},
	domain.KindUnit:                {insert: insertUnitV2, update: updateUnitV2, setActive: setActiveUnitV2, del: deleteUnitV2},
	domain.KindUnitPolicy:          {insert: insertUnitPolicyV2, update: updateUnitPolicyV2, setActive: setActiveUnitPolicyV2, del: deleteUnitPolicyV2},
	domain.KindAttributeBinding:    {setActive: setActiveApplicabilityV2, del: deleteApplicabilityV2},
	domain.KindPresentationField:   {insert: insertPresentationFieldV2, update: updatePresentationFieldV2, setActive: setActivePresentationFieldV2, del: deletePresentationFieldV2},
}

// insertMutationV2 builds the OpInsert candidate mutation for rec — no DB
// access needed since Insert carries its own full identity fields.
func insertMutationV2(rec domain.CatalogRecord) func(context.Context, pgx.Tx) (domain.CatalogMutation, error) {
	return func(context.Context, pgx.Tx) (domain.CatalogMutation, error) {
		return domain.CatalogMutation{Op: domain.OpInsert, Record: rec}, nil
	}
}

// updateMutationV2 resolves rec's currently persisted identity via the same
// legacy kindTables.get, then supplies rec as the OpUpdate Replacement —
// mirrors catalogo.Service.Update's Record/Replacement split (service.go).
func updateMutationV2(rec domain.CatalogRecord) func(context.Context, pgx.Tx) (domain.CatalogMutation, error) {
	return func(ctx context.Context, tx pgx.Tx) (domain.CatalogMutation, error) {
		h, ok := kindTables[rec.Kind]
		if !ok || h.get == nil {
			return domain.CatalogMutation{}, fmt.Errorf("%w: %q", domain.ErrCatalogKindUnknown, rec.Kind)
		}
		current, err := h.get(ctx, tx, rec.ID)
		if err != nil {
			return domain.CatalogMutation{}, err
		}
		replacement := rec
		return domain.CatalogMutation{Op: domain.OpUpdate, Record: current, Replacement: &replacement}, nil
	}
}

// lifecycleMutationV2 resolves id's identity for SetActive/Delete — no
// Replacement needed since mutateSlice toggles/removes in place.
func lifecycleMutationV2(kind domain.CatalogKindCode, id int64, op domain.MutationOp) func(context.Context, pgx.Tx) (domain.CatalogMutation, error) {
	return func(ctx context.Context, tx pgx.Tx) (domain.CatalogMutation, error) {
		h, ok := kindTables[kind]
		if !ok || h.get == nil {
			return domain.CatalogMutation{}, fmt.Errorf("%w: %q", domain.ErrCatalogKindUnknown, kind)
		}
		current, err := h.get(ctx, tx, id)
		if err != nil {
			return domain.CatalogMutation{}, err
		}
		return domain.CatalogMutation{Op: op, Record: current}, nil
	}
}

// runV2CoherentTx is the one coherent-result protocol every V2 write shares
// (design "Catalog transaction and coherent publication" steps 1-7): resolve
// buildMutation, load the pre-write catalog, run apply's physical
// CAS/insert/delete, build the private candidate via
// domain.ApplyCatalogMutation, reload+validate the post-write catalog, and
// commit ONLY when it is domain.EquivalentResourceCatalogs (4A) to the
// candidate — never a hand-built candidate returned as if persisted.
// buildMutation/apply run in this order so a genuine physical failure (stale
// revision, FK violation, CHECK constraint) keeps its own classification
// instead of surfacing as a generic candidate-mismatch.
func runV2CoherentTx(
	ctx context.Context, pool *pgxpool.Pool,
	buildMutation func(context.Context, pgx.Tx) (domain.CatalogMutation, error),
	apply func(context.Context, pgx.Tx) (int64, uint64, error),
) (int64, uint64, domain.ResourceCatalog, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, 0, domain.ResourceCatalog{}, fmt.Errorf("begin catalog v2 transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	mutation, err := buildMutation(ctx, tx)
	if err != nil {
		return 0, 0, domain.ResourceCatalog{}, err
	}
	before, err := loadResourceCatalogTxV2(ctx, tx)
	if err != nil {
		return 0, 0, domain.ResourceCatalog{}, fmt.Errorf("load catalog before v2 write: %w", err)
	}

	id, revision, err := apply(ctx, tx)
	if err != nil {
		return 0, 0, domain.ResourceCatalog{}, err
	}

	candidate, err := domain.ApplyCatalogMutation(before, catalogRegistryV2, mutation)
	if err != nil {
		return 0, 0, domain.ResourceCatalog{}, err
	}
	after, err := loadResourceCatalogTxV2(ctx, tx)
	if err != nil {
		return 0, 0, domain.ResourceCatalog{}, fmt.Errorf("reload catalog after v2 write: %w", err)
	}
	if err := after.Validate(); err != nil {
		return 0, 0, domain.ResourceCatalog{}, err
	}
	// canonicalOrderV2: loadResourceCatalogTx's SQL ORDER BY is a reload
	// artifact, not domain identity, for TOP-LEVEL collections (unlike nested
	// Rules order, which stays untouched and positional). ApplyCatalogMutation
	// always appends at the end, which does not generally match the reload's
	// own sort position (e.g. a new CLASE with display_order 0 sorting before
	// existing higher-order ones) — sort both sides by full content first so
	// the compare is multiset-equivalent per collection, still exact per field.
	if !domain.EquivalentResourceCatalogs(canonicalOrderV2(after), canonicalOrderV2(candidate)) {
		return 0, 0, domain.ResourceCatalog{}, errCatalogCandidateMismatchV2
	}

	// Commit-before-[future]-publication (design "Catalog transaction and
	// coherent publication"): only `after`, the reloaded/validated committed
	// catalog, is ever returned — publication itself is stage 4C+.
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, domain.ResourceCatalog{}, fmt.Errorf("commit catalog v2 transaction: %w", err)
	}
	return id, revision, after, nil
}

// canonicalOrderV2 returns a comparison-only copy of catalog, sorted per
// runV2CoherentTx's doc comment. catalog is pass-by-value with fresh slice
// headers, so the caller's own value (e.g. the returned `after` result) is
// never mutated in place.
func canonicalOrderV2(catalog domain.ResourceCatalog) domain.ResourceCatalog {
	catalog.Classes = sortedByContentV2(catalog.Classes)
	catalog.Families = sortedByContentV2(catalog.Families)
	catalog.Types = sortedByContentV2(catalog.Types)
	catalog.PresentationFields = sortedByContentV2(catalog.PresentationFields)
	catalog.Units = sortedByContentV2(catalog.Units)
	catalog.UnitPolicies = sortedByContentV2(catalog.UnitPolicies)
	catalog.Definitions = sortedByContentV2(catalog.Definitions)
	catalog.Attributes = sortedByContentV2(catalog.Attributes) // each element's own Rules order is untouched
	catalog.Options = sortedByContentV2(catalog.Options)
	catalog.Relations = sortedByContentV2(catalog.Relations)
	catalog.OptionSets = sortedByContentV2(catalog.OptionSets)
	return catalog
}

// sortedByContentV2 returns a NEW slice sorted by each element's full %+v
// content, including any nested slice (so two elements identical except for
// nested Rules order sort differently, keeping rule-order sensitivity intact).
func sortedByContentV2[T any](items []T) []T {
	out := append([]T(nil), items...)
	sort.SliceStable(out, func(i, j int) bool {
		return fmt.Sprintf("%+v", out[i]) < fmt.Sprintf("%+v", out[j])
	})
	return out
}

// catalogAdminRepositoryV2 is the concrete, still-dormant implementation of
// domain.CatalogAdminRepositoryV2 — the first stage of this change allowed to
// assert full conformance (design "Catalog transaction and coherent
// publication").
type catalogAdminRepositoryV2 struct{ pool *pgxpool.Pool }

// NewCatalogAdminRepositoryV2 returns a domain.CatalogAdminRepositoryV2
// backed by pool, mirroring NewCatalogAdminRepository's construction
// convention. No production composition root, service, or TUI file calls
// this constructor yet — it remains dormant/non-authoritative.
func NewCatalogAdminRepositoryV2(pool *pgxpool.Pool) domain.CatalogAdminRepositoryV2 {
	return &catalogAdminRepositoryV2{pool: pool}
}

var _ domain.CatalogAdminRepositoryV2 = (*catalogAdminRepositoryV2)(nil)

func (r *catalogAdminRepositoryV2) Insert(ctx context.Context, rec domain.CatalogRecord) (domain.CatalogWriteResult, error) {
	if rec.Kind == domain.KindAttributeBinding {
		return insertApplicabilityAggregateV2(ctx, r.pool, rec)
	}
	h, ok := kindTablesV2[rec.Kind]
	if !ok || h.insert == nil {
		return domain.CatalogWriteResult{}, fmt.Errorf("%w: %q", domain.ErrCatalogKindUnknown, rec.Kind)
	}
	id, revision, catalog, err := runV2CoherentTx(ctx, r.pool, insertMutationV2(rec), func(ctx context.Context, tx pgx.Tx) (int64, uint64, error) {
		return h.insert(ctx, tx, rec)
	})
	if err != nil {
		return domain.CatalogWriteResult{}, err
	}
	result := rec
	result.ID, result.Revision = id, revision
	return domain.NewCatalogWriteResult(&result, catalog), nil
}

func (r *catalogAdminRepositoryV2) Update(ctx context.Context, rec domain.CatalogRecord, expectedRevision uint64) (domain.CatalogWriteResult, error) {
	if rec.Kind == domain.KindAttributeBinding {
		return updateApplicabilityAggregateV2(ctx, r.pool, rec, expectedRevision)
	}
	h, ok := kindTablesV2[rec.Kind]
	if !ok || h.update == nil {
		return domain.CatalogWriteResult{}, fmt.Errorf("%w: %q", domain.ErrCatalogKindUnknown, rec.Kind)
	}
	id, revision, catalog, err := runV2CoherentTx(ctx, r.pool, updateMutationV2(rec), func(ctx context.Context, tx pgx.Tx) (int64, uint64, error) {
		revision, err := h.update(ctx, tx, rec, expectedRevision)
		return rec.ID, revision, err
	})
	if err != nil {
		return domain.CatalogWriteResult{}, err
	}
	result := rec
	result.ID, result.Revision = id, revision
	return domain.NewCatalogWriteResult(&result, catalog), nil
}

// SetActive returns a minimal CatalogWriteResult.Record (kind/id/revision/
// active only, not the kind's other field values) — this V2 port takes no
// field values on SetActive, mirroring the legacy port's own SetActive
// signature (domain/catalog_record.go).
func (r *catalogAdminRepositoryV2) SetActive(ctx context.Context, kind domain.CatalogKindCode, id int64, active bool, expectedRevision uint64) (domain.CatalogWriteResult, error) {
	h, ok := kindTablesV2[kind]
	if !ok || h.setActive == nil {
		return domain.CatalogWriteResult{}, fmt.Errorf("%w: %q", domain.ErrCatalogKindUnknown, kind)
	}
	op := domain.OpDeactivate
	if active {
		op = domain.OpReactivate
	}
	touchedID, revision, catalog, err := runV2CoherentTx(ctx, r.pool, lifecycleMutationV2(kind, id, op), func(ctx context.Context, tx pgx.Tx) (int64, uint64, error) {
		revision, err := h.setActive(ctx, tx, id, active, expectedRevision)
		return id, revision, err
	})
	if err != nil {
		return domain.CatalogWriteResult{}, err
	}
	rec := domain.CatalogRecord{Kind: kind, ID: touchedID, Revision: revision, Active: active}
	return domain.NewCatalogWriteResult(&rec, catalog), nil
}

func (r *catalogAdminRepositoryV2) Delete(ctx context.Context, kind domain.CatalogKindCode, id int64, expectedRevision uint64) (domain.CatalogWriteResult, error) {
	h, ok := kindTablesV2[kind]
	if !ok || h.del == nil {
		return domain.CatalogWriteResult{}, fmt.Errorf("%w: %q", domain.ErrCatalogKindUnknown, kind)
	}
	_, _, catalog, err := runV2CoherentTx(ctx, r.pool, lifecycleMutationV2(kind, id, domain.OpDelete), func(ctx context.Context, tx pgx.Tx) (int64, uint64, error) {
		return 0, 0, h.del(ctx, tx, id, expectedRevision)
	})
	if err != nil {
		return domain.CatalogWriteResult{}, err
	}
	return domain.NewCatalogWriteResult(nil, catalog), nil
}
