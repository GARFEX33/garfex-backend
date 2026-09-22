package postgres

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/GARFEX33/garfex-backend/internal/app/catalogo"
	"github.com/GARFEX33/garfex-backend/internal/app/recursos"
	resourcebridge "github.com/GARFEX33/garfex-backend/internal/bridge/resourcecore"
	"github.com/GARFEX33/garfex-backend/internal/domain"
	"github.com/GARFEX33/garfex-backend/resourcecore"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestApplicabilityAggregateV2Integration proves the dormant stage-3E helpers against real PostgreSQL.
func TestApplicabilityAggregateV2Integration(t *testing.T) {
	dsn := os.Getenv("GARFEX_TEST_DSN")
	if dsn == "" {
		t.Skip("GARFEX_TEST_DSN not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	t.Cleanup(pool.Close)
	classCode, familyCode, characteristic := setupApplicabilityFixture(t, ctx, pool)
	whenCode := characteristic // self-referencing rule condition; simplifies fixture setup
	rowCount := func(t *testing.T) int {
		return countRows(t, ctx, pool, `
			SELECT count(*) FROM public.resource_attributes ra
			JOIN public.attribute_definitions d ON d.id = ra.definition_id
			JOIN public.resource_families f ON f.id = ra.family_id WHERE d.code=$1 AND f.code=$2`, characteristic, familyCode)
	}
	ruleCount := func(t *testing.T) int {
		return countRows(t, ctx, pool, `
			SELECT count(*) FROM public.resource_attribute_rules rr
			JOIN public.attribute_definitions d ON d.id = rr.when_definition_id WHERE d.code=$1`, whenCode)
	}
	rec := func(mode domain.AttributeMode, rules []domain.CatalogRuleRecord) domain.CatalogRecord {
		return domain.CatalogRecord{
			Kind: domain.KindAttributeBinding, Active: true,
			Values: map[string]domain.CatalogValue{
				"class": refValue(domain.KindClass, classCode, 0), "family": refValue(domain.KindFamily, familyCode, 0),
				"characteristic": refValue(domain.KindAttributeDefinition, characteristic, 0),
				"mode":           textValue(string(mode)), "identityParticipates": boolValue(true),
			},
			Rules: rules,
		}
	}
	condRule := domain.CatalogRuleRecord{When: domain.AttributeCondition{AttributeCode: whenCode, Equals: "X"}, Mode: domain.ModeForbidden, NotApplicable: true, Active: true}
	badFamily := rec(domain.ModeConditional, []domain.CatalogRuleRecord{condRule})
	badFamily.Values["family"] = refValue(domain.KindFamily, "TEST_3E_NO_SUCH_FAMILY", 0)
	badRule := domain.CatalogRuleRecord{When: domain.AttributeCondition{AttributeCode: "TEST_3E_NO_SUCH_WHEN", Equals: "X"}, Mode: domain.ModeForbidden, Active: true}
	badMode := domain.CatalogRuleRecord{When: domain.AttributeCondition{AttributeCode: whenCode, Equals: "X"}, Mode: domain.AttributeMode("BOGUS"), Active: true}
	forceReloadFailure := func() {
		loadResourceCatalogTxV2 = func(context.Context, pgx.Tx) (domain.ResourceCatalog, error) {
			return domain.ResourceCatalog{}, errors.New("forced reload failure")
		}
	}
	for _, tt := range []struct {
		name      string
		rec       domain.CatalogRecord
		wantErrIs error
		before    func()
	}{
		{"nil rules omitted", rec(domain.ModeConditional, nil), errApplicabilityRulesOmitted, nil},
		{"missing family reference", badFamily, domain.ErrCatalogReference, nil},
		{"malformed rule reference", rec(domain.ModeConditional, []domain.CatalogRuleRecord{condRule, badRule}), domain.ErrCatalogReference, nil},
		{"child-write CHECK-constraint failure", rec(domain.ModeConditional, []domain.CatalogRuleRecord{condRule, badMode}), nil, nil},
		{"reload-step failure", rec(domain.ModeConditional, []domain.CatalogRuleRecord{condRule}), nil, forceReloadFailure},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.before != nil {
				original := loadResourceCatalogTxV2
				tt.before()
				defer func() { loadResourceCatalogTxV2 = original }()
			}
			_, err := insertApplicabilityAggregateV2(ctx, pool, tt.rec)
			if err == nil {
				t.Fatal("error = nil, want a rejected write")
			}
			if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
				t.Fatalf("error = %v, want %v", err, tt.wantErrIs)
			}
			if got, gotRules := rowCount(t), ruleCount(t); got != 0 || gotRules != 0 {
				t.Fatalf("counts = attrs:%d rules:%d, want 0/0", got, gotRules)
			}
		})
	}
	t.Run("insert succeeds then update replaces rules under CAS", func(t *testing.T) {
		result, err := insertApplicabilityAggregateV2(ctx, pool, rec(domain.ModeRequired, []domain.CatalogRuleRecord{}))
		if err != nil || result.Record == nil || result.Record.ID == 0 || result.Record.Revision != 1 {
			t.Fatalf("insert result = %+v, err = %v, want a persisted id and revision 1", result.Record, err)
		}
		attributeID := result.Record.ID
		t.Cleanup(func() {
			if _, err := pool.Exec(ctx, `DELETE FROM public.resource_attributes WHERE id=$1`, attributeID); err != nil {
				t.Errorf("cleanup applicability attribute: %v", err)
			}
		})
		if got := rowCount(t); got != 1 {
			t.Fatalf("resource_attributes count = %d, want 1", got)
		}
		updateRec := rec(domain.ModeConditional, []domain.CatalogRuleRecord{condRule})
		updateRec.ID = attributeID
		if _, err := updateApplicabilityAggregateV2(ctx, pool, updateRec, 0); !errors.Is(err, errApplicabilityRevisionRequired) {
			t.Fatalf("zero expected revision error = %v, want errApplicabilityRevisionRequired", err)
		}
		if _, err := updateApplicabilityAggregateV2(ctx, pool, updateRec, 99); !errors.Is(err, errApplicabilityStaleRevision) {
			t.Fatalf("stale revision error = %v, want errApplicabilityStaleRevision", err)
		}
		if got := ruleCount(t); got != 0 {
			t.Fatalf("rule count after rejected update = %d, want 0 (unchanged)", got)
		}
		result, err = updateApplicabilityAggregateV2(ctx, pool, updateRec, 1)
		if err != nil || result.Record == nil || result.Record.Revision != 2 {
			t.Fatalf("update result = %+v, err = %v, want revision 2", result.Record, err)
		}
		if got := ruleCount(t); got != 1 {
			t.Fatalf("rule count after successful update = %d, want 1", got)
		}
		notFoundRec := rec(domain.ModeConditional, []domain.CatalogRuleRecord{condRule})
		notFoundRec.ID = -1
		if _, err := updateApplicabilityAggregateV2(ctx, pool, notFoundRec, 1); !errors.Is(err, domain.ErrCatalogRecordNotFound) {
			t.Fatalf("not-found error = %v, want ErrCatalogRecordNotFound", err)
		}
	})
}

// 3E's bounded compareApplicabilityRules/compareApplicabilityCandidate/
// errApplicabilityCandidateMismatch are retired (4B): rule-order/count/field
// sensitivity is now covered by domain.EquivalentResourceCatalogs's own tests
// plus TestCoherentResultRollbackIntegration below.

// TestCoherentResultRollbackIntegration is 4B's closing proof: every V2 write
// commits ONLY when its reloaded catalog is domain.EquivalentResourceCatalogs
// to the ApplyCatalogMutation-built private candidate. loadResourceCatalogTxV2
// is overridden (same seam TestApplicabilityAggregateV2Integration uses) to
// force a lost field, a reordered rule list, and a reload failure, each
// proving the whole transaction rolls back with nothing persisted.
func TestCoherentResultRollbackIntegration(t *testing.T) {
	dsn := os.Getenv("GARFEX_TEST_DSN")
	if dsn == "" {
		t.Skip("GARFEX_TEST_DSN not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	t.Cleanup(pool.Close)
	repo := NewCatalogAdminRepositoryV2(pool)

	// forceAfterReload replaces only the SECOND loadResourceCatalogTxV2 call
	// within one coherent-result transaction (the post-write "after" reload
	// step 2's second invocation) — the first ("before") call stays real, so
	// candidate construction still starts from genuine pre-write state.
	forceAfterReload := func(t *testing.T, mutate func(domain.ResourceCatalog) (domain.ResourceCatalog, error)) {
		t.Helper()
		original := loadResourceCatalogTxV2
		calls := 0
		loadResourceCatalogTxV2 = func(ctx context.Context, tx pgx.Tx) (domain.ResourceCatalog, error) {
			calls++
			catalog, err := original(ctx, tx)
			if err != nil || calls != 2 {
				return catalog, err
			}
			return mutate(catalog)
		}
		t.Cleanup(func() { loadResourceCatalogTxV2 = original })
	}

	t.Run("lost field: after-reload omitting the new class rolls back and persists nothing", func(t *testing.T) {
		forceAfterReload(t, func(catalog domain.ResourceCatalog) (domain.ResourceCatalog, error) {
			kept := make([]domain.ResourceClass, 0, len(catalog.Classes))
			for _, c := range catalog.Classes {
				if c.Code != "TEST_4B_CLASS_LOST" {
					kept = append(kept, c)
				}
			}
			catalog.Classes = kept
			return catalog, nil
		})
		_, err := repo.Insert(ctx, domain.CatalogRecord{Kind: domain.KindClass, Active: true, Values: map[string]domain.CatalogValue{
			"code": textValue("TEST_4B_CLASS_LOST"), "name": textValue("Test 4B"), "plural": textValue("Test 4Bs"),
			"slug": textValue("test-4b"), "aliases": listValue([]string{}), "keywords": listValue([]string{}),
		}})
		if !errors.Is(err, errCatalogCandidateMismatchV2) {
			t.Fatalf("error = %v, want errCatalogCandidateMismatchV2", err)
		}
		if got := countRows(t, ctx, pool, `SELECT count(*) FROM public.resource_classes WHERE code=$1`, "TEST_4B_CLASS_LOST"); got != 0 {
			t.Fatalf("persisted class rows = %d, want 0 (rollback)", got)
		}
	})

	t.Run("invalid after-reload rolls back and persists nothing", func(t *testing.T) {
		forceAfterReload(t, func(domain.ResourceCatalog) (domain.ResourceCatalog, error) {
			return domain.ResourceCatalog{}, errors.New("forced after-reload failure")
		})
		_, err := repo.Insert(ctx, domain.CatalogRecord{Kind: domain.KindUnit, Active: true, Values: map[string]domain.CatalogValue{
			"code": textValue("TEST_4B_UNIT_RELOAD"), "name": textValue("Test 4B Unit"), "symbol": textValue("t4"), "dimension": textValue("Longitud"),
		}})
		if err == nil {
			t.Fatal("error = nil, want the forced after-reload failure")
		}
		if got := countRows(t, ctx, pool, `SELECT count(*) FROM public.unit_definitions WHERE code=$1`, "TEST_4B_UNIT_RELOAD"); got != 0 {
			t.Fatalf("persisted unit rows = %d, want 0 (rollback)", got)
		}
	})

	t.Run("rule reorder: after-reload with swapped rule order rolls back", func(t *testing.T) {
		classCode, familyCode, characteristic := setupApplicabilityFixture(t, ctx, pool)
		ruleA := domain.CatalogRuleRecord{When: domain.AttributeCondition{AttributeCode: characteristic, Equals: "A"}, Mode: domain.ModeForbidden, Active: true}
		ruleB := domain.CatalogRuleRecord{When: domain.AttributeCondition{AttributeCode: characteristic, Equals: "B"}, Mode: domain.ModeForbidden, Active: true}
		forceAfterReload(t, func(catalog domain.ResourceCatalog) (domain.ResourceCatalog, error) {
			for i := range catalog.Attributes {
				if catalog.Attributes[i].Definition.Code == characteristic && len(catalog.Attributes[i].Rules) == 2 {
					rules := catalog.Attributes[i].Rules
					rules[0], rules[1] = rules[1], rules[0]
				}
			}
			return catalog, nil
		})
		_, err := repo.Insert(ctx, domain.CatalogRecord{
			Kind: domain.KindAttributeBinding, Active: true,
			Values: map[string]domain.CatalogValue{
				"class": refValue(domain.KindClass, classCode, 0), "family": refValue(domain.KindFamily, familyCode, 0),
				"characteristic": refValue(domain.KindAttributeDefinition, characteristic, 0),
				"mode":           textValue(string(domain.ModeConditional)), "identityParticipates": boolValue(true),
			},
			Rules: []domain.CatalogRuleRecord{ruleA, ruleB},
		})
		if !errors.Is(err, errCatalogCandidateMismatchV2) {
			t.Fatalf("error = %v, want errCatalogCandidateMismatchV2", err)
		}
		if got := countRows(t, ctx, pool, `
			SELECT count(*) FROM public.resource_attributes ra
			JOIN public.attribute_definitions d ON d.id = ra.definition_id
			JOIN public.resource_families f ON f.id = ra.family_id WHERE d.code=$1 AND f.code=$2`, characteristic, familyCode); got != 0 {
			t.Fatalf("persisted attribute rows = %d, want 0 (rollback)", got)
		}
	})
}

// setupApplicabilityFixture creates isolated TEST_3E_ fixtures, cleaned up LIFO.
func setupApplicabilityFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (classCode, familyCode, characteristic string) {
	t.Helper()
	repo := NewCatalogAdminRepository(pool)
	insert := func(kind domain.CatalogKindCode, values map[string]domain.CatalogValue) int64 {
		id, err := repo.Insert(ctx, domain.CatalogRecord{Kind: kind, Active: true, Values: values})
		if err != nil {
			t.Fatalf("insert %s fixture: %v", kind, err)
		}
		t.Cleanup(func() {
			if err := repo.Delete(ctx, kind, id); err != nil {
				t.Errorf("cleanup %s fixture: %v", kind, err)
			}
		})
		return id
	}
	insert(domain.KindClass, map[string]domain.CatalogValue{
		"code": {Text: "TEST_3E_CLASS"}, "name": {Text: "Test 3E Clase"}, "plural": {Text: "Test 3E Clases"}, "slug": {Text: "test-3e-clase"},
		"aliases": {List: []string{}}, "keywords": {List: []string{}},
	})
	insert(domain.KindFamily, map[string]domain.CatalogValue{
		"class": refValue(domain.KindClass, "TEST_3E_CLASS", 0), "code": {Text: "TEST_3E_FAM"}, "name": {Text: "Test 3E Familia"},
	})
	insert(domain.KindAttributeDefinition, map[string]domain.CatalogValue{
		"code": {Text: "test_3e_char"}, "name": {Text: "Test 3E Característica"}, "valueType": {Text: string(domain.ValueTypeControlledText)},
	})
	return "TEST_3E_CLASS", "TEST_3E_FAM", "test_3e_char"
}

func countRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, query, args...).Scan(&got); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return got
}

// TestCatalogCASCreateUpdateIntegration proves the dormant stage-3F CAS
// helpers for CLASE, FAMILIA, TIPO, CARACTERISTICA, and CONJUNTO_OPCIONES
// against real PostgreSQL: same-revision success, stale conflict, absent
// not-found, exactly-once increment, duplicate-code classification (with a
// rollback/row-count proof), and hash-ID resolution for CONJUNTO_OPCIONES.
func TestCatalogCASCreateUpdateIntegration(t *testing.T) {
	dsn := os.Getenv("GARFEX_TEST_DSN")
	if dsn == "" {
		t.Skip("GARFEX_TEST_DSN not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	t.Cleanup(pool.Close)
	classCode, familyCode := setupCASFixture(t, ctx, pool)

	type kindCase struct {
		name      string
		insert    func(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) (int64, uint64, error)
		update    func(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord, expected uint64) (uint64, error)
		rec       func(code string) domain.CatalogRecord
		rowsQuery string
		cleanup   string
	}
	// runInsert/runUpdate wrap a tx-level function through runCatalogWriteTxV2,
	// standing in for the pool-level entry points 3G's concrete interface
	// methods will provide.
	runInsert := func(fn func(context.Context, pgx.Tx, domain.CatalogRecord) (int64, uint64, error), rec domain.CatalogRecord) (int64, uint64, error) {
		return runCatalogWriteTxV2(ctx, pool, func(ctx context.Context, tx pgx.Tx) (int64, uint64, error) { return fn(ctx, tx, rec) })
	}
	runUpdate := func(fn func(context.Context, pgx.Tx, domain.CatalogRecord, uint64) (uint64, error), rec domain.CatalogRecord, expected uint64) (int64, uint64, error) {
		return runCatalogWriteTxV2(ctx, pool, func(ctx context.Context, tx pgx.Tx) (int64, uint64, error) {
			revision, err := fn(ctx, tx, rec, expected)
			return rec.ID, revision, err
		})
	}
	cases := []kindCase{
		{
			name: "CLASE", insert: insertClassV2, update: updateClassV2,
			rec: func(code string) domain.CatalogRecord {
				return domain.CatalogRecord{Kind: domain.KindClass, Active: true, Values: map[string]domain.CatalogValue{
					"code": textValue(code), "name": textValue("Test 3F " + code), "plural": textValue("Test 3F " + code + "s"),
					"slug": textValue("test-3f-" + code), "aliases": listValue([]string{}), "keywords": listValue([]string{}),
				}}
			},
			rowsQuery: `SELECT count(*) FROM public.resource_classes WHERE id=$1`,
			cleanup:   `DELETE FROM public.resource_classes WHERE id=$1`,
		},
		{
			name: "FAMILIA", insert: insertFamilyV2, update: updateFamilyV2,
			rec: func(code string) domain.CatalogRecord {
				return domain.CatalogRecord{Kind: domain.KindFamily, Active: true, Values: map[string]domain.CatalogValue{
					"class": refValue(domain.KindClass, classCode, 0), "code": textValue(code), "name": textValue("Test 3F " + code),
				}}
			},
			rowsQuery: `SELECT count(*) FROM public.resource_families WHERE id=$1`,
			cleanup:   `DELETE FROM public.resource_families WHERE id=$1`,
		},
		{
			name: "TIPO", insert: insertTypeV2, update: updateTypeV2,
			rec: func(code string) domain.CatalogRecord {
				return domain.CatalogRecord{Kind: domain.KindType, Active: true, Values: map[string]domain.CatalogValue{
					"class": refValue(domain.KindClass, classCode, 0), "family": refValue(domain.KindFamily, familyCode, 0),
					"code": textValue(code), "name": textValue("Test 3F " + code),
				}}
			},
			rowsQuery: `SELECT count(*) FROM public.resource_types WHERE id=$1`,
			cleanup:   `DELETE FROM public.resource_types WHERE id=$1`,
		},
		{
			name: "CARACTERISTICA", insert: insertDefinitionV2, update: updateDefinitionV2,
			rec: func(code string) domain.CatalogRecord {
				return domain.CatalogRecord{Kind: domain.KindAttributeDefinition, Active: true, Values: map[string]domain.CatalogValue{
					"code": textValue(code), "name": textValue("Test 3F " + code), "valueType": textValue(string(domain.ValueTypeControlledText)),
				}}
			},
			rowsQuery: `SELECT count(*) FROM public.attribute_definitions WHERE id=$1`,
			cleanup:   `DELETE FROM public.attribute_definitions WHERE id=$1`,
		},
		{
			name: "CONJUNTO_OPCIONES", insert: insertOptionSetV2, update: updateOptionSetV2,
			rec: func(code string) domain.CatalogRecord {
				return domain.CatalogRecord{Kind: domain.KindOptionSet, Active: true, Values: map[string]domain.CatalogValue{
					"code": textValue(code), "name": textValue("Test 3F " + code),
				}}
			},
			rowsQuery: `SELECT count(*) FROM public.resource_option_sets WHERE id=$1`,
			cleanup:   `DELETE FROM public.resource_option_sets WHERE id=$1`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code := "TEST_3F_" + tc.name
			id, revision, err := runInsert(tc.insert, tc.rec(code))
			if err != nil || id == 0 || revision != 1 {
				t.Fatalf("insert = id:%d rev:%d err:%v, want a persisted id and revision 1", id, revision, err)
			}
			t.Cleanup(func() {
				if _, err := pool.Exec(ctx, tc.cleanup, id); err != nil {
					t.Errorf("cleanup %s fixture: %v", tc.name, err)
				}
			})
			updated := tc.rec(code)
			updated.ID = id
			updated.Values["name"] = textValue("Test 3F " + code + " Renamed")

			if _, _, err := runUpdate(tc.update, updated, 0); !errors.Is(err, errCatalogRevisionRequiredV2) {
				t.Fatalf("zero expected revision error = %v, want errCatalogRevisionRequiredV2", err)
			}
			if _, _, err := runUpdate(tc.update, updated, 99); !errors.Is(err, errCatalogStaleRevisionV2) {
				t.Fatalf("stale revision error = %v, want errCatalogStaleRevisionV2", err)
			}
			_, revision, err = runUpdate(tc.update, updated, 1)
			if err != nil || revision != 2 {
				t.Fatalf("same-revision update = rev:%d err:%v, want revision 2", revision, err)
			}
			if _, _, err := runUpdate(tc.update, updated, 1); !errors.Is(err, errCatalogStaleRevisionV2) {
				t.Fatalf("exactly-once increment: repeated old-revision update error = %v, want errCatalogStaleRevisionV2", err)
			}
			notFound := tc.rec(code)
			notFound.ID = -1
			if _, _, err := runUpdate(tc.update, notFound, 1); !errors.Is(err, domain.ErrCatalogRecordNotFound) {
				t.Fatalf("not-found error = %v, want ErrCatalogRecordNotFound", err)
			}

			if _, _, err := runInsert(tc.insert, tc.rec(code)); !errors.Is(err, domain.ErrCatalogDuplicate) {
				t.Fatalf("duplicate-code insert error = %v, want ErrCatalogDuplicate", err)
			}
			if got := countRows(t, ctx, pool, tc.rowsQuery, id); got != 1 {
				t.Fatalf("row count after rejected duplicate insert = %d, want 1 (rollback proof)", got)
			}
		})
	}

	t.Run("FAMILIA insert rejects missing class reference", func(t *testing.T) {
		_, _, err := runInsert(insertFamilyV2, domain.CatalogRecord{Kind: domain.KindFamily, Active: true, Values: map[string]domain.CatalogValue{
			"class": refValue(domain.KindClass, "TEST_3F_NO_SUCH_CLASS", 0), "code": textValue("TEST_3F_FAM_BADREF"), "name": textValue("x"),
		}})
		if !errors.Is(err, domain.ErrCatalogReference) {
			t.Fatalf("error = %v, want ErrCatalogReference", err)
		}
	})
	t.Run("TIPO insert rejects missing family reference", func(t *testing.T) {
		_, _, err := runInsert(insertTypeV2, domain.CatalogRecord{Kind: domain.KindType, Active: true, Values: map[string]domain.CatalogValue{
			"class": refValue(domain.KindClass, classCode, 0), "family": refValue(domain.KindFamily, "TEST_3F_NO_SUCH_FAM", 0),
			"code": textValue("TEST_3F_TIPO_BADREF"), "name": textValue("x"),
		}})
		if !errors.Is(err, domain.ErrCatalogReference) {
			t.Fatalf("error = %v, want ErrCatalogReference", err)
		}
	})
}

// TestCatalogCASImmutableCodeOnceReferencedIntegration is the stage-3F
// correction: proves the dormant V2 update functions for the 5 kinds with a
// codeField() (CLASE, FAMILIA, TIPO, CARACTERISTICA, CONJUNTO_OPCIONES) omit
// código from the generated SET clause once referenced by resource
// instances, mirroring catalog_admin_kinds.go's legacy updateClass/
// updateFamily/updateType/updateDefinition/updateOptionSet D11-layer-2
// guard. CLASE alone also triangulates the unreferenced-still-editable path
// and the 3F CAS revision guard, since that branching logic is shared,
// unchanged code identical across all 5 kinds (see the source diff) —
// scoped down from a per-kind repeat to fit this correction's line budget.
func TestCatalogCASImmutableCodeOnceReferencedIntegration(t *testing.T) {
	dsn := os.Getenv("GARFEX_TEST_DSN")
	if dsn == "" {
		t.Skip("GARFEX_TEST_DSN not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	t.Cleanup(pool.Close)
	repo := NewCatalogAdminRepository(pool)
	insert := func(kind domain.CatalogKindCode, values map[string]domain.CatalogValue) int64 {
		id, err := repo.Insert(ctx, domain.CatalogRecord{Kind: kind, Active: true, Values: values})
		if err != nil {
			t.Fatalf("insert %s fixture: %v", kind, err)
		}
		t.Cleanup(func() { _ = repo.Delete(ctx, kind, id) })
		return id
	}
	classID := insert(domain.KindClass, map[string]domain.CatalogValue{
		"code": textValue("TEST_3F_IMM_CLASS"), "name": textValue("X"), "plural": textValue("Xs"),
		"slug": textValue("x"), "aliases": listValue([]string{}), "keywords": listValue([]string{}),
	})
	familyID := insert(domain.KindFamily, map[string]domain.CatalogValue{
		"class": refValue(domain.KindClass, "TEST_3F_IMM_CLASS", 0), "code": textValue("TEST_3F_IMM_FAM"), "name": textValue("X"),
	})
	typeID := insert(domain.KindType, map[string]domain.CatalogValue{
		"class": refValue(domain.KindClass, "TEST_3F_IMM_CLASS", 0), "family": refValue(domain.KindFamily, "TEST_3F_IMM_FAM", 0),
		"code": textValue("TEST_3F_IMM_TIPO"), "name": textValue("X"),
	})
	definitionID := insert(domain.KindAttributeDefinition, map[string]domain.CatalogValue{
		"code": textValue("test_3f_imm_char"), "name": textValue("X"), "valueType": textValue(string(domain.ValueTypeControlledText)),
	})
	optionSetID := insert(domain.KindOptionSet, map[string]domain.CatalogValue{
		"code": textValue("TEST_3F_IMM_OPTSET"), "name": textValue("X"),
	})

	// Reference classID/familyID/typeID via one recursos row and
	// definitionID/optionSetID via one resource_attribute_values row —
	// text_value avoids needing an attribute_options fixture.
	var unitID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM public.unit_definitions WHERE code='PZA'`).Scan(&unitID); err != nil {
		t.Fatalf("resolve seeded unit PZA: %v", err)
	}
	var resourceID int64
	if err := pool.QueryRow(ctx, `INSERT INTO public.recursos (class_id, family_id, type_id, natural_unit_id, display_name, identity_key)
		VALUES ($1,$2,$3,$4,'x','v1|TEST_3F_IMM_RECURSO') RETURNING id`, classID, familyID, typeID, unitID).Scan(&resourceID); err != nil {
		t.Fatalf("insert recursos: %v", err)
	}
	var attributeID int64
	if err := pool.QueryRow(ctx, `INSERT INTO public.resource_attributes (class_id, family_id, definition_id, mode, identity_participates)
		VALUES ($1,$2,$3,'OPTIONAL',FALSE) RETURNING id`, classID, familyID, definitionID).Scan(&attributeID); err != nil {
		t.Fatalf("insert resource_attributes: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM public.resource_attributes WHERE id=$1`, attributeID) })
	// recursos cleanup is registered AFTER resource_attributes's, so it runs
	// FIRST (t.Cleanup is LIFO), cascading away resource_attribute_values
	// before resource_attributes' own delete is attempted.
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM public.recursos WHERE id=$1`, resourceID) })
	if _, err := pool.Exec(ctx, `INSERT INTO public.resource_attribute_values (resource_id, family_id, resource_attribute_id, attribute_definition_id, option_set, text_value)
		VALUES ($1,$2,$3,$4,'TEST_3F_IMM_OPTSET','x')`, resourceID, familyID, attributeID, definitionID); err != nil {
		t.Fatalf("insert resource_attribute_values: %v", err) // cascades on recursos delete
	}

	assertCode := func(t *testing.T, query string, id int64, want string) {
		t.Helper()
		var got string
		if err := pool.QueryRow(ctx, query, id).Scan(&got); err != nil || got != want {
			t.Fatalf("código = %q err=%v, want %q", got, err, want)
		}
	}
	run := func(t *testing.T, fn func(context.Context, pgx.Tx, domain.CatalogRecord, uint64) (uint64, error), rec domain.CatalogRecord, expected uint64) (uint64, error) {
		t.Helper()
		_, revision, err := runCatalogWriteTxV2(ctx, pool, func(ctx context.Context, tx pgx.Tx) (int64, uint64, error) {
			revision, err := fn(ctx, tx, rec, expected)
			return rec.ID, revision, err
		})
		return revision, err
	}

	t.Run("referenced records keep código stable across the 5 guarded kinds", func(t *testing.T) {
		// CLASE additionally proves the two claims apply-progress.md's 3F
		// correction note asserted but this subtest previously never checked:
		// a referenced record's non-código fields (name) DO persist, and the
		// CAS revision DOES increment by 1, even though código itself stays
		// frozen (assertCode below).
		classRevision, err := run(t, updateClassV2, domain.CatalogRecord{Kind: domain.KindClass, ID: classID, Active: true, Values: map[string]domain.CatalogValue{
			"code": textValue("TEST_3F_IMM_CLASS_X"), "name": textValue("Y"), "plural": textValue("Ys"),
			"slug": textValue("y"), "aliases": listValue([]string{}), "keywords": listValue([]string{}),
		}}, 1)
		if err != nil {
			t.Fatalf("CLASE update: %v", err)
		}
		if classRevision != 2 {
			t.Fatalf("CLASE revision = %d, want 2 (expected 1 + 1 increment)", classRevision)
		}
		assertCode(t, `SELECT code FROM public.resource_classes WHERE id=$1`, classID, "TEST_3F_IMM_CLASS")
		assertCode(t, `SELECT name FROM public.resource_classes WHERE id=$1`, classID, "Y")

		if _, err := run(t, updateFamilyV2, domain.CatalogRecord{Kind: domain.KindFamily, ID: familyID, Active: true, Values: map[string]domain.CatalogValue{
			"class": refValue(domain.KindClass, "TEST_3F_IMM_CLASS", 0), "code": textValue("TEST_3F_IMM_FAM_X"), "name": textValue("Y"),
		}}, 1); err != nil {
			t.Fatalf("FAMILIA update: %v", err)
		}
		assertCode(t, `SELECT code FROM public.resource_families WHERE id=$1`, familyID, "TEST_3F_IMM_FAM")

		if _, err := run(t, updateTypeV2, domain.CatalogRecord{Kind: domain.KindType, ID: typeID, Active: true, Values: map[string]domain.CatalogValue{
			"class": refValue(domain.KindClass, "TEST_3F_IMM_CLASS", 0), "family": refValue(domain.KindFamily, "TEST_3F_IMM_FAM", 0),
			"code": textValue("TEST_3F_IMM_TIPO_X"), "name": textValue("Y"),
		}}, 1); err != nil {
			t.Fatalf("TIPO update: %v", err)
		}
		assertCode(t, `SELECT code FROM public.resource_types WHERE id=$1`, typeID, "TEST_3F_IMM_TIPO")

		if _, err := run(t, updateDefinitionV2, domain.CatalogRecord{Kind: domain.KindAttributeDefinition, ID: definitionID, Active: true, Values: map[string]domain.CatalogValue{
			"code": textValue("test_3f_imm_char_x"), "name": textValue("Y"), "valueType": textValue(string(domain.ValueTypeControlledText)),
		}}, 1); err != nil {
			t.Fatalf("CARACTERISTICA update: %v", err)
		}
		assertCode(t, `SELECT code FROM public.attribute_definitions WHERE id=$1`, definitionID, "test_3f_imm_char")

		if _, err := run(t, updateOptionSetV2, domain.CatalogRecord{Kind: domain.KindOptionSet, ID: optionSetID, Active: true, Values: map[string]domain.CatalogValue{
			"code": textValue("TEST_3F_IMM_OPTSET_X"), "name": textValue("Y"),
		}}, 1); err != nil {
			t.Fatalf("CONJUNTO_OPCIONES update: %v", err)
		}
		assertCode(t, `SELECT code FROM public.resource_option_sets WHERE id=$1`, optionSetID, "TEST_3F_IMM_OPTSET")
	})

	t.Run("CLASE triangulation: unreferenced still editable, CAS guard unaffected", func(t *testing.T) {
		freeID := insert(domain.KindClass, map[string]domain.CatalogValue{
			"code": textValue("TEST_3F_IMM_FREE"), "name": textValue("X"), "plural": textValue("Xs"),
			"slug": textValue("free"), "aliases": listValue([]string{}), "keywords": listValue([]string{}),
		})
		freeRec := domain.CatalogRecord{Kind: domain.KindClass, ID: freeID, Active: true, Values: map[string]domain.CatalogValue{
			"code": textValue("TEST_3F_IMM_FREE_X"), "name": textValue("Y"), "plural": textValue("Ys"),
			"slug": textValue("free-y"), "aliases": listValue([]string{}), "keywords": listValue([]string{}),
		}}
		if _, err := run(t, updateClassV2, freeRec, 0); !errors.Is(err, errCatalogRevisionRequiredV2) {
			t.Fatalf("zero expected revision error = %v, want errCatalogRevisionRequiredV2", err)
		}
		if _, err := run(t, updateClassV2, freeRec, 99); !errors.Is(err, errCatalogStaleRevisionV2) {
			t.Fatalf("stale revision error = %v, want errCatalogStaleRevisionV2", err)
		}
		if _, err := run(t, updateClassV2, freeRec, 1); err != nil {
			t.Fatalf("unreferenced update: %v", err)
		}
		assertCode(t, `SELECT code FROM public.resource_classes WHERE id=$1`, freeID, "TEST_3F_IMM_FREE_X")
	})
}

// TestCatalogCASCreateUpdateRemainingIntegration proves the dormant 3G1 CAS
// helpers for OPCION, RELACION_OPCIONES, UNIDAD, POLITICA_UNIDAD, and
// PRESENTACION: same-revision success, stale conflict, not-found,
// exactly-once increment, duplicate-key rollback, hash-ID resolution, and —
// for OPCION/UNIDAD (codeField() kinds) — the D11-layer-2 immutable-código
// guard with a real re-query proof once referenced.
func TestCatalogCASCreateUpdateRemainingIntegration(t *testing.T) {
	dsn := os.Getenv("GARFEX_TEST_DSN")
	if dsn == "" {
		t.Skip("GARFEX_TEST_DSN not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	t.Cleanup(pool.Close)
	classCode, familyCode, typeCode, unitCode, characteristic, characteristic2, optionSet, fromOption, toOption := setupCAS2Fixture(t, ctx, pool)

	runInsert := func(fn func(context.Context, pgx.Tx, domain.CatalogRecord) (int64, uint64, error), rec domain.CatalogRecord) (int64, uint64, error) {
		return runCatalogWriteTxV2(ctx, pool, func(ctx context.Context, tx pgx.Tx) (int64, uint64, error) { return fn(ctx, tx, rec) })
	}
	runUpdate := func(fn func(context.Context, pgx.Tx, domain.CatalogRecord, uint64) (uint64, error), rec domain.CatalogRecord, expected uint64) (int64, uint64, error) {
		return runCatalogWriteTxV2(ctx, pool, func(ctx context.Context, tx pgx.Tx) (int64, uint64, error) {
			revision, err := fn(ctx, tx, rec, expected)
			return rec.ID, revision, err
		})
	}

	type kindCase struct {
		name      string
		insert    func(context.Context, pgx.Tx, domain.CatalogRecord) (int64, uint64, error)
		update    func(context.Context, pgx.Tx, domain.CatalogRecord, uint64) (uint64, error)
		rec       func() domain.CatalogRecord
		mutate    func(domain.CatalogRecord) domain.CatalogRecord
		rowsQuery string
		cleanup   string
	}
	cases := []kindCase{
		{
			name: "UNIDAD", insert: insertUnitV2, update: updateUnitV2,
			rec: func() domain.CatalogRecord {
				return domain.CatalogRecord{Kind: domain.KindUnit, Active: true, Values: map[string]domain.CatalogValue{
					"code": textValue("TEST_3G1_UNIT"), "name": textValue("Test 3G1 Unit"), "symbol": textValue("tu"), "dimension": textValue("Longitud"),
				}}
			},
			mutate: func(r domain.CatalogRecord) domain.CatalogRecord {
				r.Values["name"] = textValue("Test 3G1 Unit Renamed")
				return r
			},
			rowsQuery: `SELECT count(*) FROM public.unit_definitions WHERE id=$1`,
			cleanup:   `DELETE FROM public.unit_definitions WHERE id=$1`,
		},
		{
			name: "OPCION", insert: insertOptionV2, update: updateOptionV2,
			rec: func() domain.CatalogRecord {
				return domain.CatalogRecord{Kind: domain.KindOption, Active: true, Values: map[string]domain.CatalogValue{
					"optionSet": refValue(domain.KindOptionSet, optionSet, 0), "characteristic": refValue(domain.KindAttributeDefinition, characteristic, 0),
					"code": textValue("TEST_3G1_OPT"), "label": textValue("Test 3G1 Option"),
				}}
			},
			mutate: func(r domain.CatalogRecord) domain.CatalogRecord {
				r.Values["label"] = textValue("Test 3G1 Option Renamed")
				return r
			},
			rowsQuery: `SELECT count(*) FROM public.attribute_options WHERE id=$1`,
			cleanup:   `DELETE FROM public.attribute_options WHERE id=$1`,
		},
		{
			name: "RELACION_OPCIONES", insert: insertOptionRelationV2, update: updateOptionRelationV2,
			rec: func() domain.CatalogRecord {
				return domain.CatalogRecord{Kind: domain.KindOptionRelation, Active: true, Values: map[string]domain.CatalogValue{
					"optionSet":          refValue(domain.KindOptionSet, optionSet, 0),
					"fromCharacteristic": refValue(domain.KindAttributeDefinition, characteristic, 0), "fromOption": refValue(domain.KindOption, fromOption, 0),
					"toCharacteristic": refValue(domain.KindAttributeDefinition, characteristic2, 0), "toOption": refValue(domain.KindOption, toOption, 0),
				}}
			},
			mutate:    func(r domain.CatalogRecord) domain.CatalogRecord { r.Active = false; return r },
			rowsQuery: `SELECT count(*) FROM public.attribute_option_relations WHERE id=$1`,
			cleanup:   `DELETE FROM public.attribute_option_relations WHERE id=$1`,
		},
		{
			name: "POLITICA_UNIDAD", insert: insertUnitPolicyV2, update: updateUnitPolicyV2,
			rec: func() domain.CatalogRecord {
				return domain.CatalogRecord{Kind: domain.KindUnitPolicy, Active: true, Values: map[string]domain.CatalogValue{
					"class": refValue(domain.KindClass, classCode, 0), "family": refValue(domain.KindFamily, familyCode, 0), "unit": refValue(domain.KindUnit, unitCode, 0),
					"allowed": boolValue(true), "suggested": boolValue(false),
				}}
			},
			mutate:    func(r domain.CatalogRecord) domain.CatalogRecord { r.Values["suggested"] = boolValue(true); return r },
			rowsQuery: `SELECT count(*) FROM public.resource_unit_policies WHERE id=$1`,
			cleanup:   `DELETE FROM public.resource_unit_policies WHERE id=$1`,
		},
		{
			name: "PRESENTACION", insert: insertPresentationFieldV2, update: updatePresentationFieldV2,
			rec: func() domain.CatalogRecord {
				return domain.CatalogRecord{Kind: domain.KindPresentationField, Active: true, Values: map[string]domain.CatalogValue{
					"class": refValue(domain.KindClass, classCode, 0), "family": refValue(domain.KindFamily, familyCode, 0), "type": refValue(domain.KindType, typeCode, 0),
					"characteristic": refValue(domain.KindAttributeDefinition, characteristic, 0), "position": intValue(0),
				}}
			},
			mutate:    func(r domain.CatalogRecord) domain.CatalogRecord { r.Values["position"] = intValue(1); return r },
			rowsQuery: `SELECT count(*) FROM public.resource_type_presentation_fields WHERE id=$1`,
			cleanup:   `DELETE FROM public.resource_type_presentation_fields WHERE id=$1`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, revision, err := runInsert(tc.insert, tc.rec())
			if err != nil || id == 0 || revision != 1 {
				t.Fatalf("insert = id:%d rev:%d err:%v, want a persisted id and revision 1", id, revision, err)
			}
			t.Cleanup(func() {
				if _, err := pool.Exec(ctx, tc.cleanup, id); err != nil {
					t.Errorf("cleanup %s fixture: %v", tc.name, err)
				}
			})
			updated := tc.mutate(tc.rec())
			updated.ID = id
			if _, _, err := runUpdate(tc.update, updated, 0); !errors.Is(err, errCatalogRevisionRequiredV2) {
				t.Fatalf("zero expected revision error = %v, want errCatalogRevisionRequiredV2", err)
			}
			if _, _, err := runUpdate(tc.update, updated, 99); !errors.Is(err, errCatalogStaleRevisionV2) {
				t.Fatalf("stale revision error = %v, want errCatalogStaleRevisionV2", err)
			}
			_, revision, err = runUpdate(tc.update, updated, 1)
			if err != nil || revision != 2 {
				t.Fatalf("same-revision update = rev:%d err:%v, want revision 2", revision, err)
			}
			if _, _, err := runUpdate(tc.update, updated, 1); !errors.Is(err, errCatalogStaleRevisionV2) {
				t.Fatalf("exactly-once increment: repeated old-revision update error = %v, want errCatalogStaleRevisionV2", err)
			}
			notFound := tc.rec()
			notFound.ID = -1
			if _, _, err := runUpdate(tc.update, notFound, 1); !errors.Is(err, domain.ErrCatalogRecordNotFound) {
				t.Fatalf("not-found error = %v, want ErrCatalogRecordNotFound", err)
			}
			if _, _, err := runInsert(tc.insert, tc.rec()); !errors.Is(err, domain.ErrCatalogDuplicate) {
				t.Fatalf("duplicate insert error = %v, want ErrCatalogDuplicate", err)
			}
			if got := countRows(t, ctx, pool, tc.rowsQuery, id); got != 1 {
				t.Fatalf("row count after rejected duplicate insert = %d, want 1 (rollback proof)", got)
			}
		})
	}

	t.Run("OPCION and UNIDAD keep código stable once referenced", func(t *testing.T) {
		unitID, optionID := referenceUnitAndOptionFixture(t, ctx, pool, classCode, familyCode, typeCode, unitCode, characteristic, optionSet, fromOption)

		_, unitRevision, err := runUpdate(updateUnitV2, domain.CatalogRecord{Kind: domain.KindUnit, ID: unitID, Active: true, Values: map[string]domain.CatalogValue{
			"code": textValue(unitCode + "_X"), "name": textValue("Renamed Unit"), "symbol": textValue("tu"), "dimension": textValue("Longitud"),
		}}, 1)
		if err != nil || unitRevision != 2 {
			t.Fatalf("UNIDAD referenced update = rev:%d err:%v, want revision 2", unitRevision, err)
		}
		assertText(t, ctx, pool, `SELECT code FROM public.unit_definitions WHERE id=$1`, unitID, unitCode)
		assertText(t, ctx, pool, `SELECT name FROM public.unit_definitions WHERE id=$1`, unitID, "Renamed Unit")

		_, optRevision, err := runUpdate(updateOptionV2, domain.CatalogRecord{Kind: domain.KindOption, ID: optionID, Active: true, Values: map[string]domain.CatalogValue{
			"optionSet": refValue(domain.KindOptionSet, optionSet, 0), "characteristic": refValue(domain.KindAttributeDefinition, characteristic, 0),
			"code": textValue(fromOption + "_X"), "label": textValue("Renamed Option"),
		}}, 1)
		if err != nil || optRevision != 2 {
			t.Fatalf("OPCION referenced update = rev:%d err:%v, want revision 2", optRevision, err)
		}
		optionCodeQuery := `SELECT code FROM public.attribute_options WHERE id=$1`
		optionLabelQuery := `SELECT label FROM public.attribute_options WHERE id=$1`
		assertText(t, ctx, pool, optionCodeQuery, optionID, fromOption)
		assertText(t, ctx, pool, optionLabelQuery, optionID, "Renamed Option")
	})
}

// assertText re-queries one text column by id and fails the test unless it
// equals want — the real re-query proof the immutable-código guard needs.
func assertText(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, id int64, want string) {
	t.Helper()
	var got string
	if err := pool.QueryRow(ctx, query, id).Scan(&got); err != nil || got != want {
		t.Fatalf("value = %q err=%v, want %q", got, err, want)
	}
}

// referenceUnitAndOptionFixture: one shared recursos row makes unitCode
// "referenced" (natural_unit_id) and fromOption "referenced" (one
// resource_attribute_values row) — the OPCION/UNIDAD guard test's fixture.
func referenceUnitAndOptionFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, classCode, familyCode, typeCode, unitCode, characteristic, optionSet, fromOption string) (unitID, optionID int64) {
	t.Helper()
	resolve := func(query, code string) int64 {
		var id int64
		if err := pool.QueryRow(ctx, query, code).Scan(&id); err != nil {
			t.Fatalf("resolve %q: %v", code, err)
		}
		return id
	}
	classID := resolve(`SELECT id FROM public.resource_classes WHERE code=$1`, classCode)
	familyID := resolve(`SELECT id FROM public.resource_families WHERE code=$1`, familyCode)
	typeID := resolve(`SELECT id FROM public.resource_types WHERE code=$1`, typeCode)
	unitID = resolve(`SELECT id FROM public.unit_definitions WHERE code=$1`, unitCode)
	definitionID := resolve(`SELECT id FROM public.attribute_definitions WHERE code=$1`, characteristic)
	if err := pool.QueryRow(ctx, `SELECT id FROM public.attribute_options WHERE option_set=$1 AND attribute_definition_id=$2 AND code=$3`,
		optionSet, definitionID, fromOption).Scan(&optionID); err != nil {
		t.Fatalf("resolve option id: %v", err)
	}
	var resourceID int64
	if err := pool.QueryRow(ctx, `INSERT INTO public.recursos (class_id, family_id, type_id, natural_unit_id, display_name, identity_key)
		VALUES ($1,$2,$3,$4,'x','v1|TEST_3G1_REF') RETURNING id`, classID, familyID, typeID, unitID).Scan(&resourceID); err != nil {
		t.Fatalf("insert recursos: %v", err)
	}
	var attributeID int64
	if err := pool.QueryRow(ctx, `INSERT INTO public.resource_attributes (class_id, family_id, definition_id, option_set, mode, identity_participates)
		VALUES ($1,$2,$3,$4,'OPTIONAL',FALSE) RETURNING id`, classID, familyID, definitionID, optionSet).Scan(&attributeID); err != nil {
		t.Fatalf("insert resource_attributes: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM public.resource_attributes WHERE id=$1`, attributeID) })
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM public.recursos WHERE id=$1`, resourceID) })
	if _, err := pool.Exec(ctx, `INSERT INTO public.resource_attribute_values (resource_id, family_id, resource_attribute_id, attribute_definition_id, option_set, option_code)
		VALUES ($1,$2,$3,$4,$5,$6)`, resourceID, familyID, attributeID, definitionID, optionSet, fromOption); err != nil {
		t.Fatalf("insert resource_attribute_values: %v", err)
	}
	return unitID, optionID
}

// setupCAS2Fixture creates isolated TEST_3G1_ class/family/type/unit/
// characteristic(x2)/optionSet/option(x2) fixtures, cleaned up LIFO.
func setupCAS2Fixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (classCode, familyCode, typeCode, unitCode, characteristic, characteristic2, optionSet, fromOption, toOption string) {
	t.Helper()
	repo := NewCatalogAdminRepository(pool)
	insert := func(kind domain.CatalogKindCode, values map[string]domain.CatalogValue) int64 {
		id, err := repo.Insert(ctx, domain.CatalogRecord{Kind: kind, Active: true, Values: values})
		if err != nil {
			t.Fatalf("insert %s fixture: %v", kind, err)
		}
		t.Cleanup(func() {
			if err := repo.Delete(ctx, kind, id); err != nil {
				t.Errorf("cleanup %s fixture: %v", kind, err)
			}
		})
		return id
	}
	insert(domain.KindClass, map[string]domain.CatalogValue{
		"code": textValue("TEST_3G1_CLASS"), "name": textValue("Test 3G1 Clase"), "plural": textValue("Test 3G1 Clases"),
		"slug": textValue("test-3g1-clase"), "aliases": listValue([]string{}), "keywords": listValue([]string{}),
	})
	insert(domain.KindFamily, map[string]domain.CatalogValue{
		"class": refValue(domain.KindClass, "TEST_3G1_CLASS", 0), "code": textValue("TEST_3G1_FAM"), "name": textValue("Test 3G1 Familia"),
	})
	insert(domain.KindType, map[string]domain.CatalogValue{
		"class": refValue(domain.KindClass, "TEST_3G1_CLASS", 0), "family": refValue(domain.KindFamily, "TEST_3G1_FAM", 0),
		"code": textValue("TEST_3G1_TIPO"), "name": textValue("Test 3G1 Tipo"),
	})
	insert(domain.KindUnit, map[string]domain.CatalogValue{
		"code": textValue("TEST_3G1_BASEUNIT"), "name": textValue("Test 3G1 Unit"), "symbol": textValue("bu"), "dimension": textValue("Longitud"),
	})
	insert(domain.KindAttributeDefinition, map[string]domain.CatalogValue{
		// CONTROLLED_OPTION: resource_attribute_values_validate_type only
		// allows option_code on this value_type (needed by the guard fixture).
		"code": textValue("test_3g1_char"), "name": textValue("Test 3G1 Característica"), "valueType": textValue(string(domain.ValueTypeControlledOption)),
	})
	// test_3g1_char2: RELACION_OPCIONES' from/to must reference two distinct
	// characteristics (attribute_option_relations_check CHECK constraint).
	insert(domain.KindAttributeDefinition, map[string]domain.CatalogValue{
		"code": textValue("test_3g1_char2"), "name": textValue("Test 3G1 Característica 2"), "valueType": textValue(string(domain.ValueTypeControlledText)),
	})
	insert(domain.KindOptionSet, map[string]domain.CatalogValue{
		"code": textValue("TEST_3G1_OPTSET"), "name": textValue("Test 3G1 Conjunto"),
	})
	insert(domain.KindOption, map[string]domain.CatalogValue{
		"optionSet": refValue(domain.KindOptionSet, "TEST_3G1_OPTSET", 0), "characteristic": refValue(domain.KindAttributeDefinition, "test_3g1_char", 0),
		"code": textValue("TEST_3G1_FROM"), "label": textValue("From"),
	})
	insert(domain.KindOption, map[string]domain.CatalogValue{
		"optionSet": refValue(domain.KindOptionSet, "TEST_3G1_OPTSET", 0), "characteristic": refValue(domain.KindAttributeDefinition, "test_3g1_char2", 0),
		"code": textValue("TEST_3G1_TO"), "label": textValue("To"),
	})
	return "TEST_3G1_CLASS", "TEST_3G1_FAM", "TEST_3G1_TIPO", "TEST_3G1_BASEUNIT", "test_3g1_char", "test_3g1_char2", "TEST_3G1_OPTSET", "TEST_3G1_FROM", "TEST_3G1_TO"
}

// TestCatalogCASLifecycleDeleteIntegration proves the dormant 3G2
// lifecycle (SetActive) and delete CAS helpers for all 11 kinds: same-
// revision success, stale conflict, not-found, exactly-once increment (for
// SetActive), and a real re-query proof via the legacy repository's Get
// (rather than a fresh custom SQL query per kind). Reuses 3G1's
// setupCAS2Fixture for parent refs — safe because Go subtests run
// sequentially and each test function's own t.Cleanup fully tears its
// fixtures down before the next test function runs.
func TestCatalogCASLifecycleDeleteIntegration(t *testing.T) {
	dsn := os.Getenv("GARFEX_TEST_DSN")
	if dsn == "" {
		t.Skip("GARFEX_TEST_DSN not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	t.Cleanup(pool.Close)
	classCode, familyCode, typeCode, unitCode, characteristic, characteristic2, optionSet, fromOption, toOption := setupCAS2Fixture(t, ctx, pool)
	legacy := NewCatalogAdminRepository(pool)

	runInsert := func(fn func(context.Context, pgx.Tx, domain.CatalogRecord) (int64, uint64, error), rec domain.CatalogRecord) (int64, uint64, error) {
		return runCatalogWriteTxV2(ctx, pool, func(ctx context.Context, tx pgx.Tx) (int64, uint64, error) { return fn(ctx, tx, rec) })
	}
	runSetActive := func(fn func(context.Context, pgx.Tx, int64, bool, uint64) (uint64, error), id int64, active bool, expected uint64) (uint64, error) {
		_, revision, err := runCatalogWriteTxV2(ctx, pool, func(ctx context.Context, tx pgx.Tx) (int64, uint64, error) {
			rev, err := fn(ctx, tx, id, active, expected)
			return id, rev, err
		})
		return revision, err
	}
	runDelete := func(fn func(context.Context, pgx.Tx, int64, uint64) error, id int64, expected uint64) error {
		_, _, err := runCatalogWriteTxV2(ctx, pool, func(ctx context.Context, tx pgx.Tx) (int64, uint64, error) {
			return id, 0, fn(ctx, tx, id, expected)
		})
		return err
	}

	type kindCase struct {
		name      string
		kind      domain.CatalogKindCode
		insert    func(context.Context, pgx.Tx, domain.CatalogRecord) (int64, uint64, error)
		setActive func(context.Context, pgx.Tx, int64, bool, uint64) (uint64, error)
		del       func(context.Context, pgx.Tx, int64, uint64) error
		rec       func() domain.CatalogRecord
	}
	cases := []kindCase{
		{"CLASE", domain.KindClass, insertClassV2, setActiveClassV2, deleteClassV2, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindClass, Active: true, Values: map[string]domain.CatalogValue{
				"code": textValue("TEST_3G2_CLASS"), "name": textValue("Test 3G2 Clase"), "plural": textValue("Test 3G2 Clases"),
				"slug": textValue("test-3g2-clase"), "aliases": listValue([]string{}), "keywords": listValue([]string{}),
			}}
		}},
		{"FAMILIA", domain.KindFamily, insertFamilyV2, setActiveFamilyV2, deleteFamilyV2, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindFamily, Active: true, Values: map[string]domain.CatalogValue{
				"class": refValue(domain.KindClass, classCode, 0), "code": textValue("TEST_3G2_FAM"), "name": textValue("Test 3G2 Familia"),
			}}
		}},
		{"TIPO", domain.KindType, insertTypeV2, setActiveTypeV2, deleteTypeV2, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindType, Active: true, Values: map[string]domain.CatalogValue{
				"class": refValue(domain.KindClass, classCode, 0), "family": refValue(domain.KindFamily, familyCode, 0),
				"code": textValue("TEST_3G2_TIPO"), "name": textValue("Test 3G2 Tipo"),
			}}
		}},
		{"CARACTERISTICA", domain.KindAttributeDefinition, insertDefinitionV2, setActiveDefinitionV2, deleteDefinitionV2, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindAttributeDefinition, Active: true, Values: map[string]domain.CatalogValue{
				"code": textValue("test_3g2_char"), "name": textValue("Test 3G2 Característica"), "valueType": textValue(string(domain.ValueTypeControlledText)),
			}}
		}},
		{"CONJUNTO_OPCIONES", domain.KindOptionSet, insertOptionSetV2, setActiveOptionSetV2, deleteOptionSetV2, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindOptionSet, Active: true, Values: map[string]domain.CatalogValue{
				"code": textValue("TEST_3G2_OPTSET"), "name": textValue("Test 3G2 Conjunto"),
			}}
		}},
		{"OPCION", domain.KindOption, insertOptionV2, setActiveOptionV2, deleteOptionV2, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindOption, Active: true, Values: map[string]domain.CatalogValue{
				"optionSet": refValue(domain.KindOptionSet, optionSet, 0), "characteristic": refValue(domain.KindAttributeDefinition, characteristic, 0),
				"code": textValue("TEST_3G2_OPT"), "label": textValue("Test 3G2 Option"),
			}}
		}},
		{"RELACION_OPCIONES", domain.KindOptionRelation, insertOptionRelationV2, setActiveOptionRelationV2, deleteOptionRelationV2, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindOptionRelation, Active: true, Values: map[string]domain.CatalogValue{
				"optionSet":          refValue(domain.KindOptionSet, optionSet, 0),
				"fromCharacteristic": refValue(domain.KindAttributeDefinition, characteristic, 0), "fromOption": refValue(domain.KindOption, fromOption, 0),
				"toCharacteristic": refValue(domain.KindAttributeDefinition, characteristic2, 0), "toOption": refValue(domain.KindOption, toOption, 0),
			}}
		}},
		{"UNIDAD", domain.KindUnit, insertUnitV2, setActiveUnitV2, deleteUnitV2, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindUnit, Active: true, Values: map[string]domain.CatalogValue{
				"code": textValue("TEST_3G2_UNIT"), "name": textValue("Test 3G2 Unit"), "symbol": textValue("t2"), "dimension": textValue("Longitud"),
			}}
		}},
		{"POLITICA_UNIDAD", domain.KindUnitPolicy, insertUnitPolicyV2, setActiveUnitPolicyV2, deleteUnitPolicyV2, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindUnitPolicy, Active: true, Values: map[string]domain.CatalogValue{
				"class": refValue(domain.KindClass, classCode, 0), "family": refValue(domain.KindFamily, familyCode, 0), "unit": refValue(domain.KindUnit, unitCode, 0),
				"allowed": boolValue(true), "suggested": boolValue(false),
			}}
		}},
		{"APLICABILIDAD", domain.KindAttributeBinding, insertApplicabilityParentV2, setActiveApplicabilityV2, deleteApplicabilityV2, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindAttributeBinding, Active: true, Values: map[string]domain.CatalogValue{
				"class": refValue(domain.KindClass, classCode, 0), "family": refValue(domain.KindFamily, familyCode, 0),
				"characteristic": refValue(domain.KindAttributeDefinition, characteristic, 0), "mode": textValue(string(domain.ModeOptional)), "identityParticipates": boolValue(false),
			}}
		}},
		{"PRESENTACION", domain.KindPresentationField, insertPresentationFieldV2, setActivePresentationFieldV2, deletePresentationFieldV2, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindPresentationField, Active: true, Values: map[string]domain.CatalogValue{
				"class": refValue(domain.KindClass, classCode, 0), "family": refValue(domain.KindFamily, familyCode, 0), "type": refValue(domain.KindType, typeCode, 0),
				"characteristic": refValue(domain.KindAttributeDefinition, characteristic, 0), "position": intValue(0),
			}}
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, _, err := runInsert(tc.insert, tc.rec())
			if err != nil || id == 0 {
				t.Fatalf("insert = id:%d err:%v, want a persisted id", id, err)
			}
			t.Cleanup(func() { _ = legacy.Delete(ctx, tc.kind, id) })

			if _, err := runSetActive(tc.setActive, id, false, 0); !errors.Is(err, errCatalogRevisionRequiredV2) {
				t.Fatalf("SetActive zero expected revision error = %v, want errCatalogRevisionRequiredV2", err)
			}
			if _, err := runSetActive(tc.setActive, id, false, 99); !errors.Is(err, errCatalogStaleRevisionV2) {
				t.Fatalf("SetActive stale revision error = %v, want errCatalogStaleRevisionV2", err)
			}
			rev, err := runSetActive(tc.setActive, id, false, 1)
			if err != nil || rev != 2 {
				t.Fatalf("SetActive same-revision = rev:%d err:%v, want revision 2", rev, err)
			}
			if got, err := legacy.Get(ctx, tc.kind, id); err != nil || got.Active {
				t.Fatalf("Get after SetActive(false) = active:%v err:%v, want active=false", got.Active, err)
			}
			if _, err := runSetActive(tc.setActive, id, true, 1); !errors.Is(err, errCatalogStaleRevisionV2) {
				t.Fatalf("SetActive exactly-once increment: repeated old-revision error = %v, want errCatalogStaleRevisionV2", err)
			}
			if _, err := runSetActive(tc.setActive, -1, true, 1); !errors.Is(err, domain.ErrCatalogRecordNotFound) {
				t.Fatalf("SetActive not-found error = %v, want ErrCatalogRecordNotFound", err)
			}

			if err := runDelete(tc.del, id, 0); !errors.Is(err, errCatalogRevisionRequiredV2) {
				t.Fatalf("Delete zero expected revision error = %v, want errCatalogRevisionRequiredV2", err)
			}
			if err := runDelete(tc.del, id, 99); !errors.Is(err, errCatalogStaleRevisionV2) {
				t.Fatalf("Delete stale revision error = %v, want errCatalogStaleRevisionV2", err)
			}
			if err := runDelete(tc.del, id, 2); err != nil {
				t.Fatalf("Delete same-revision = err:%v, want nil", err)
			}
			if _, err := legacy.Get(ctx, tc.kind, id); !errors.Is(err, domain.ErrCatalogRecordNotFound) {
				t.Fatalf("Get after Delete error = %v, want ErrCatalogRecordNotFound", err)
			}
			if err := runDelete(tc.del, -1, 1); !errors.Is(err, domain.ErrCatalogRecordNotFound) {
				t.Fatalf("Delete not-found error = %v, want ErrCatalogRecordNotFound", err)
			}
		})
	}

	t.Run("APLICABILIDAD SetActive preserves child rule rows", func(t *testing.T) {
		ruleRec := domain.CatalogRecord{
			Kind: domain.KindAttributeBinding, Active: true,
			Values: map[string]domain.CatalogValue{
				"class": refValue(domain.KindClass, classCode, 0), "family": refValue(domain.KindFamily, familyCode, 0),
				"characteristic": refValue(domain.KindAttributeDefinition, characteristic2, 0),
				"mode":           textValue(string(domain.ModeConditional)), "identityParticipates": boolValue(true),
			},
			Rules: []domain.CatalogRuleRecord{{When: domain.AttributeCondition{AttributeCode: characteristic2, Equals: "X"}, Mode: domain.ModeForbidden, Active: true}},
		}
		result, err := insertApplicabilityAggregateV2(ctx, pool, ruleRec)
		if err != nil || result.Record == nil {
			t.Fatalf("insert applicability aggregate: %v", err)
		}
		attributeID := result.Record.ID
		t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM public.resource_attributes WHERE id=$1`, attributeID) })
		ruleCount := func() int {
			return countRows(t, ctx, pool, `SELECT count(*) FROM public.resource_attribute_rules WHERE resource_attribute_id=$1`, attributeID)
		}
		if got := ruleCount(); got != 1 {
			t.Fatalf("rule count before SetActive = %d, want 1", got)
		}
		rev, err := runSetActive(setActiveApplicabilityV2, attributeID, false, result.Record.Revision)
		if err != nil || rev != 2 {
			t.Fatalf("SetActive on applicability = rev:%d err:%v, want revision 2", rev, err)
		}
		if got := ruleCount(); got != 1 {
			t.Fatalf("rule count after SetActive = %d, want 1 (unchanged — must not touch child rules)", got)
		}
		if got, err := legacy.Get(ctx, domain.KindAttributeBinding, attributeID); err != nil || got.Active {
			t.Fatalf("Get after SetActive(false) = active:%v err:%v, want active=false", got.Active, err)
		}
	})

	t.Run("FK backstop blocks delete of a referenced CLASE", func(t *testing.T) {
		var classID int64
		var classRevision uint64
		if err := pool.QueryRow(ctx, `SELECT id, revision FROM public.resource_classes WHERE code=$1`, classCode).Scan(&classID, &classRevision); err != nil {
			t.Fatalf("resolve fixture class: %v", err)
		}
		if err := runDelete(deleteClassV2, classID, classRevision); !errors.Is(err, domain.ErrCatalogInUse) {
			t.Fatalf("delete referenced class error = %v, want ErrCatalogInUse", err)
		}
		if got := countRows(t, ctx, pool, `SELECT count(*) FROM public.resource_classes WHERE id=$1`, classID); got != 1 {
			t.Fatalf("referenced class row count = %d, want 1 (delete correctly blocked)", got)
		}
	})
}

// setupCASFixture creates isolated TEST_3F_ class/family fixtures shared by
// TestCatalogCASCreateUpdateIntegration's FAMILIA/TIPO subtests, cleaned up
// LIFO via the legacy repository.
func setupCASFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (classCode, familyCode string) {
	t.Helper()
	repo := NewCatalogAdminRepository(pool)
	insert := func(kind domain.CatalogKindCode, values map[string]domain.CatalogValue) int64 {
		id, err := repo.Insert(ctx, domain.CatalogRecord{Kind: kind, Active: true, Values: values})
		if err != nil {
			t.Fatalf("insert %s fixture: %v", kind, err)
		}
		t.Cleanup(func() {
			if err := repo.Delete(ctx, kind, id); err != nil {
				t.Errorf("cleanup %s fixture: %v", kind, err)
			}
		})
		return id
	}
	insert(domain.KindClass, map[string]domain.CatalogValue{
		"code": textValue("TEST_3F_CLASS"), "name": textValue("Test 3F Clase"), "plural": textValue("Test 3F Clases"),
		"slug": textValue("test-3f-clase"), "aliases": listValue([]string{}), "keywords": listValue([]string{}),
	})
	insert(domain.KindFamily, map[string]domain.CatalogValue{
		"class": refValue(domain.KindClass, "TEST_3F_CLASS", 0), "code": textValue("TEST_3F_FAM"), "name": textValue("Test 3F Familia"),
	})
	return "TEST_3F_CLASS", "TEST_3F_FAM"
}

// TestCatalogAdminRepositoryV2ConstructorConformance is a pure, DB-free proof
// that NewCatalogAdminRepositoryV2 builds a working domain.CatalogAdminRepositoryV2
// — the runtime sibling of catalog_admin_repository_v2.go's own
// `var _ domain.CatalogAdminRepositoryV2 = (*catalogAdminRepositoryV2)(nil)`
// compile-time assertion.
func TestCatalogAdminRepositoryV2ConstructorConformance(t *testing.T) {
	repo := NewCatalogAdminRepositoryV2(nil)
	if repo == nil {
		t.Fatal("NewCatalogAdminRepositoryV2(nil) = nil, want a non-nil domain.CatalogAdminRepositoryV2")
	}
}

// TestCatalogCASConcreteRepositoryV2Integration is 3G3's closing proof: the
// concrete catalogAdminRepositoryV2's public Insert/Update/SetActive/Delete
// methods (not the internal per-kind helpers 3E-3G2 already exercised)
// dispatch correctly for all 11 CatalogKindCode kinds, including
// APLICABILIDAD's rule preservation and the FK delete backstop.
func TestCatalogCASConcreteRepositoryV2Integration(t *testing.T) {
	dsn := os.Getenv("GARFEX_TEST_DSN")
	if dsn == "" {
		t.Skip("GARFEX_TEST_DSN not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	t.Cleanup(pool.Close)
	classCode, familyCode, typeCode, unitCode, characteristic, characteristic2, optionSet, fromOption, toOption := setupCAS2Fixture(t, ctx, pool)
	repo := NewCatalogAdminRepositoryV2(pool)
	legacy := NewCatalogAdminRepository(pool)

	rule := domain.CatalogRuleRecord{When: domain.AttributeCondition{AttributeCode: characteristic2, Equals: "X"}, Mode: domain.ModeForbidden, Active: true}

	cases := []struct {
		name   string
		kind   domain.CatalogKindCode
		rec    func() domain.CatalogRecord
		mutate func(domain.CatalogRecord) domain.CatalogRecord
	}{
		{"CLASE", domain.KindClass, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindClass, Active: true, Values: map[string]domain.CatalogValue{
				"code": textValue("TEST_3G3_CLASS"), "name": textValue("Test 3G3 Clase"), "plural": textValue("Test 3G3 Clases"),
				"slug": textValue("test-3g3-clase"), "aliases": listValue([]string{}), "keywords": listValue([]string{}),
			}}
		}, func(r domain.CatalogRecord) domain.CatalogRecord { r.Values["name"] = textValue("Renamed"); return r }},
		{"FAMILIA", domain.KindFamily, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindFamily, Active: true, Values: map[string]domain.CatalogValue{
				"class": refValue(domain.KindClass, classCode, 0), "code": textValue("TEST_3G3_FAM"), "name": textValue("Test 3G3 Familia"),
			}}
		}, func(r domain.CatalogRecord) domain.CatalogRecord { r.Values["name"] = textValue("Renamed"); return r }},
		{"TIPO", domain.KindType, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindType, Active: true, Values: map[string]domain.CatalogValue{
				"class": refValue(domain.KindClass, classCode, 0), "family": refValue(domain.KindFamily, familyCode, 0),
				"code": textValue("TEST_3G3_TIPO"), "name": textValue("Test 3G3 Tipo"),
			}}
		}, func(r domain.CatalogRecord) domain.CatalogRecord { r.Values["name"] = textValue("Renamed"); return r }},
		{"CARACTERISTICA", domain.KindAttributeDefinition, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindAttributeDefinition, Active: true, Values: map[string]domain.CatalogValue{
				"code": textValue("test_3g3_char"), "name": textValue("Test 3G3 Característica"), "valueType": textValue(string(domain.ValueTypeControlledText)),
			}}
		}, func(r domain.CatalogRecord) domain.CatalogRecord { r.Values["name"] = textValue("Renamed"); return r }},
		{"CONJUNTO_OPCIONES", domain.KindOptionSet, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindOptionSet, Active: true, Values: map[string]domain.CatalogValue{
				"code": textValue("TEST_3G3_OPTSET"), "name": textValue("Test 3G3 Conjunto"),
			}}
		}, func(r domain.CatalogRecord) domain.CatalogRecord { r.Values["name"] = textValue("Renamed"); return r }},
		{"OPCION", domain.KindOption, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindOption, Active: true, Values: map[string]domain.CatalogValue{
				"optionSet": refValue(domain.KindOptionSet, optionSet, 0), "characteristic": refValue(domain.KindAttributeDefinition, characteristic, 0),
				"code": textValue("TEST_3G3_OPT"), "label": textValue("Test 3G3 Option"),
			}}
		}, func(r domain.CatalogRecord) domain.CatalogRecord { r.Values["label"] = textValue("Renamed"); return r }},
		{"RELACION_OPCIONES", domain.KindOptionRelation, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindOptionRelation, Active: true, Values: map[string]domain.CatalogValue{
				"optionSet":          refValue(domain.KindOptionSet, optionSet, 0),
				"fromCharacteristic": refValue(domain.KindAttributeDefinition, characteristic, 0), "fromOption": refValue(domain.KindOption, fromOption, 0),
				"toCharacteristic": refValue(domain.KindAttributeDefinition, characteristic2, 0), "toOption": refValue(domain.KindOption, toOption, 0),
			}}
		}, func(r domain.CatalogRecord) domain.CatalogRecord { r.Active = false; return r }},
		{"UNIDAD", domain.KindUnit, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindUnit, Active: true, Values: map[string]domain.CatalogValue{
				"code": textValue("TEST_3G3_UNIT"), "name": textValue("Test 3G3 Unit"), "symbol": textValue("t3"), "dimension": textValue("Longitud"),
			}}
		}, func(r domain.CatalogRecord) domain.CatalogRecord { r.Values["name"] = textValue("Renamed"); return r }},
		{"POLITICA_UNIDAD", domain.KindUnitPolicy, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindUnitPolicy, Active: true, Values: map[string]domain.CatalogValue{
				"class": refValue(domain.KindClass, classCode, 0), "family": refValue(domain.KindFamily, familyCode, 0), "unit": refValue(domain.KindUnit, unitCode, 0),
				"allowed": boolValue(true), "suggested": boolValue(false),
			}}
		}, func(r domain.CatalogRecord) domain.CatalogRecord { r.Values["suggested"] = boolValue(true); return r }},
		{"APLICABILIDAD", domain.KindAttributeBinding, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindAttributeBinding, Active: true, Values: map[string]domain.CatalogValue{
				"class": refValue(domain.KindClass, classCode, 0), "family": refValue(domain.KindFamily, familyCode, 0),
				"characteristic": refValue(domain.KindAttributeDefinition, characteristic2, 0), "mode": textValue(string(domain.ModeConditional)), "identityParticipates": boolValue(true),
			}, Rules: []domain.CatalogRuleRecord{rule}}
		}, func(r domain.CatalogRecord) domain.CatalogRecord {
			r.Rules = []domain.CatalogRuleRecord{rule}
			return r
		}},
		{"PRESENTACION", domain.KindPresentationField, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindPresentationField, Active: true, Values: map[string]domain.CatalogValue{
				"class": refValue(domain.KindClass, classCode, 0), "family": refValue(domain.KindFamily, familyCode, 0), "type": refValue(domain.KindType, typeCode, 0),
				"characteristic": refValue(domain.KindAttributeDefinition, characteristic, 0), "position": intValue(0),
			}}
		}, func(r domain.CatalogRecord) domain.CatalogRecord { r.Values["position"] = intValue(1); return r }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			insertResult, err := repo.Insert(ctx, tc.rec())
			if err != nil || insertResult.Record == nil || insertResult.Record.ID == 0 || insertResult.Record.Revision != 1 {
				t.Fatalf("Insert = %+v, err = %v, want a persisted record at revision 1", insertResult.Record, err)
			}
			if len(insertResult.Catalog.Classes) == 0 {
				t.Fatal("Insert result Catalog has no classes, want the coherent reloaded catalog")
			}
			id := insertResult.Record.ID
			t.Cleanup(func() { _ = legacy.Delete(ctx, tc.kind, id) })

			updated := tc.mutate(tc.rec())
			updated.ID = id
			if _, err := repo.Update(ctx, updated, 99); err == nil {
				t.Fatal("Update with stale expected revision = nil error, want a rejected write")
			}
			updateResult, err := repo.Update(ctx, updated, 1)
			if err != nil || updateResult.Record == nil || updateResult.Record.Revision != 2 {
				t.Fatalf("Update = %+v, err = %v, want revision 2", updateResult.Record, err)
			}
			notFound := tc.rec()
			notFound.ID = -1
			if _, err := repo.Update(ctx, notFound, 1); !errors.Is(err, domain.ErrCatalogRecordNotFound) {
				t.Fatalf("Update not-found error = %v, want ErrCatalogRecordNotFound", err)
			}

			setActiveResult, err := repo.SetActive(ctx, tc.kind, id, false, 2)
			if err != nil || setActiveResult.Record == nil || setActiveResult.Record.Active || setActiveResult.Record.Revision != 3 {
				t.Fatalf("SetActive = %+v, err = %v, want inactive at revision 3", setActiveResult.Record, err)
			}
			if got, err := legacy.Get(ctx, tc.kind, id); err != nil || got.Active || got.Revision != 3 {
				t.Fatalf("Get after SetActive(false) = active:%v revision:%d err:%v, want active=false revision=3", got.Active, got.Revision, err)
			}
			listed, err := legacy.List(ctx, tc.kind, domain.CatalogFilter{Status: domain.CatalogStatusAll})
			if err != nil {
				t.Fatalf("List after SetActive(false): %v", err)
			}
			found := false
			for _, got := range listed {
				if got.ID == id {
					found = true
					if got.Revision != 3 {
						t.Fatalf("List after SetActive(false) revision = %d, want 3", got.Revision)
					}
					break
				}
			}
			if !found {
				t.Fatalf("List after SetActive(false) omitted id %d", id)
			}

			deleteResult, err := repo.Delete(ctx, tc.kind, id, 3)
			if err != nil || deleteResult.Record != nil {
				t.Fatalf("Delete = record:%+v, err = %v, want nil record and no error", deleteResult.Record, err)
			}
			if _, err := legacy.Get(ctx, tc.kind, id); !errors.Is(err, domain.ErrCatalogRecordNotFound) {
				t.Fatalf("Get after Delete error = %v, want ErrCatalogRecordNotFound", err)
			}
		})
	}

	t.Run("unknown kind is rejected uniformly across every method", func(t *testing.T) {
		bogus := domain.CatalogKindCode("BOGUS")
		if _, err := repo.Insert(ctx, domain.CatalogRecord{Kind: bogus}); !errors.Is(err, domain.ErrCatalogKindUnknown) {
			t.Fatalf("Insert error = %v, want ErrCatalogKindUnknown", err)
		}
		if _, err := repo.Update(ctx, domain.CatalogRecord{Kind: bogus}, 1); !errors.Is(err, domain.ErrCatalogKindUnknown) {
			t.Fatalf("Update error = %v, want ErrCatalogKindUnknown", err)
		}
		if _, err := repo.SetActive(ctx, bogus, 1, true, 1); !errors.Is(err, domain.ErrCatalogKindUnknown) {
			t.Fatalf("SetActive error = %v, want ErrCatalogKindUnknown", err)
		}
		if _, err := repo.Delete(ctx, bogus, 1, 1); !errors.Is(err, domain.ErrCatalogKindUnknown) {
			t.Fatalf("Delete error = %v, want ErrCatalogKindUnknown", err)
		}
	})

	t.Run("FK backstop blocks concrete Delete of a referenced CLASE", func(t *testing.T) {
		var classID int64
		var classRevision uint64
		if err := pool.QueryRow(ctx, `SELECT id, revision FROM public.resource_classes WHERE code=$1`, classCode).Scan(&classID, &classRevision); err != nil {
			t.Fatalf("resolve fixture class: %v", err)
		}
		if _, err := repo.Delete(ctx, domain.KindClass, classID, classRevision); !errors.Is(err, domain.ErrCatalogInUse) {
			t.Fatalf("Delete referenced class error = %v, want ErrCatalogInUse", err)
		}
		if got := countRows(t, ctx, pool, `SELECT count(*) FROM public.resource_classes WHERE id=$1`, classID); got != 1 {
			t.Fatalf("referenced class row count = %d, want 1 (delete correctly blocked)", got)
		}
	})

	t.Run("APLICABILIDAD SetActive via the concrete adapter preserves child rule rows", func(t *testing.T) {
		insertResult, err := repo.Insert(ctx, domain.CatalogRecord{
			Kind: domain.KindAttributeBinding, Active: true,
			Values: map[string]domain.CatalogValue{
				"class": refValue(domain.KindClass, classCode, 0), "family": refValue(domain.KindFamily, familyCode, 0),
				"characteristic": refValue(domain.KindAttributeDefinition, characteristic, 0), "mode": textValue(string(domain.ModeConditional)), "identityParticipates": boolValue(true),
			},
			Rules: []domain.CatalogRuleRecord{rule},
		})
		if err != nil || insertResult.Record == nil {
			t.Fatalf("Insert applicability aggregate: %v", err)
		}
		attributeID := insertResult.Record.ID
		t.Cleanup(func() { _ = legacy.Delete(ctx, domain.KindAttributeBinding, attributeID) })
		ruleCount := countRows(t, ctx, pool, `SELECT count(*) FROM public.resource_attribute_rules WHERE resource_attribute_id=$1`, attributeID)
		if ruleCount != 1 {
			t.Fatalf("rule count before SetActive = %d, want 1", ruleCount)
		}
		setActiveResult, err := repo.SetActive(ctx, domain.KindAttributeBinding, attributeID, false, insertResult.Record.Revision)
		if err != nil || setActiveResult.Record == nil || setActiveResult.Record.Revision != 2 {
			t.Fatalf("SetActive on applicability = %+v, err = %v, want revision 2", setActiveResult.Record, err)
		}
		if got := countRows(t, ctx, pool, `SELECT count(*) FROM public.resource_attribute_rules WHERE resource_attribute_id=$1`, attributeID); got != 1 {
			t.Fatalf("rule count after SetActive = %d, want 1 (unchanged)", got)
		}
	})
}

// TestPublicCatalogLifecycleRevisionCompositionIntegration proves that the
// public bridge's post-deactivation confirm-read retains the V2 revision needed
// by the next public lifecycle operation.
func TestPublicCatalogLifecycleRevisionCompositionIntegration(t *testing.T) {
	dsn := os.Getenv("GARFEX_TEST_DSN")
	if dsn == "" {
		t.Skip("GARFEX_TEST_DSN not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	t.Cleanup(pool.Close)

	boot, err := LoadResourceCatalog(ctx, pool)
	if err != nil {
		t.Fatalf("boot LoadResourceCatalog: %v", err)
	}
	authority := domain.NewCatalogAuthority(boot)
	catalogService := catalogo.NewServiceWithCatalogAuthority(
		NewCatalogAdminRepository(pool), domain.NewCatalogRegistry(), authority,
	).WithCatalogAdminRepositoryV2(NewCatalogAdminRepositoryV2(pool))
	resourceService := recursos.NewServiceWithCatalogAuthority(NewResourceRepository(pool), authority)
	adapter := resourcebridge.NewAdapter(catalogService, resourceService)
	reader, err := resourcecore.NewReadOnly(adapter)
	if err != nil {
		t.Fatalf("new public reader: %v", err)
	}
	writer, err := resourcecore.NewWriter(adapter)
	if err != nil {
		t.Fatalf("new public writer: %v", err)
	}
	legacy := NewCatalogAdminRepository(pool)

	created, err := writer.CreateCatalog(ctx, resourcecore.CatalogWriteRequest{
		Actor:  "integration-test",
		Kind:   resourcecore.KindUnit,
		Active: true,
		Values: map[string]resourcecore.Value{
			"code":      {Kind: resourcecore.ValueCode, Text: "TEST_PUBLIC_CAS_UNIT"},
			"name":      {Kind: resourcecore.ValueText, Text: "Test Public CAS Unit"},
			"symbol":    {Kind: resourcecore.ValueText, Text: "tpc"},
			"dimension": {Kind: resourcecore.ValueText, Text: "Longitud"},
		},
	})
	if err != nil || created.Revision != 1 {
		t.Fatalf("CreateCatalog = record:%+v err:%v, want revision 1", created, err)
	}
	t.Cleanup(func() { _ = legacy.Delete(ctx, domain.KindUnit, created.ID) })

	deactivated, err := writer.DeactivateCatalog(ctx, resourcecore.CatalogLifecycleRequest{
		Actor: "integration-test", Kind: resourcecore.KindUnit, ID: created.ID, ExpectedRevision: 1,
	})
	if err != nil || deactivated.Revision != 2 || deactivated.Active {
		t.Fatalf("DeactivateCatalog = record:%+v err:%v, want inactive revision 2", deactivated, err)
	}
	current, err := reader.GetCatalog(ctx, resourcecore.CatalogKey{Kind: resourcecore.KindUnit, ID: created.ID})
	if err != nil || current.Revision != 2 || current.Active {
		t.Fatalf("GetCatalog after Deactivate = record:%+v err:%v, want inactive revision 2", current, err)
	}

	if err := writer.HardDeleteCatalog(ctx, resourcecore.CatalogLifecycleRequest{
		Actor: "integration-test", Kind: resourcecore.KindUnit, ID: created.ID, ExpectedRevision: 1,
	}); !resourcecore.IsCode(err, resourcecore.Conflict) {
		t.Fatalf("HardDeleteCatalog stale revision error = %v, want CONFLICT", err)
	}
	if err := writer.HardDeleteCatalog(ctx, resourcecore.CatalogLifecycleRequest{
		Actor: "integration-test", Kind: resourcecore.KindUnit, ID: created.ID, ExpectedRevision: 2,
	}); err != nil {
		t.Fatalf("HardDeleteCatalog current revision: %v", err)
	}
	if _, err := reader.GetCatalog(ctx, resourcecore.CatalogKey{Kind: resourcecore.KindUnit, ID: created.ID}); !resourcecore.IsCode(err, resourcecore.NotFound) {
		t.Fatalf("GetCatalog after HardDelete error = %v, want NOT_FOUND", err)
	}
}

// TestCatalogConcurrentDependencyDeleteRaceIntegration is 3I's mandatory true
// concurrent dependency/delete race proof (design "PostgreSQL fixtures and
// evidence": "Hard-delete/dependency races use separate connections and
// verify the dependency or delete wins consistently with FK integrity, never
// both"). Two independent *pgxpool.Pool connections concurrently attempt to
// (a) insert a FAMILIA referencing a CLASE and (b) delete that same CLASE,
// released together off one start barrier so both are genuinely in flight
// together. Neither side takes an explicit application-level lock: the
// INSERT's own FK-constraint enforcement acquires a FOR KEY SHARE lock on
// the referenced class row, and the DELETE statement implicitly locks the
// same row — conflicting modes PostgreSQL serializes deterministically.
// Whichever transaction wins that lock determines which side succeeds, but
// the outcome shape is always the same real evidence: exactly one operation
// succeeds, the other is rejected by a genuine FK/CAS classification (never
// both, and never an orphaned FAMILIA row surviving a "successful" delete).
func TestCatalogConcurrentDependencyDeleteRaceIntegration(t *testing.T) {
	dsn := os.Getenv("GARFEX_TEST_DSN")
	if dsn == "" {
		t.Skip("GARFEX_TEST_DSN not set")
	}
	ctx := context.Background()
	pool1, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect first isolated connection: %v", err)
	}
	t.Cleanup(pool1.Close)
	pool2, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect second isolated connection: %v", err)
	}
	t.Cleanup(pool2.Close)

	repo1 := NewCatalogAdminRepositoryV2(pool1)
	legacy := NewCatalogAdminRepository(pool1)
	classResult, err := repo1.Insert(ctx, domain.CatalogRecord{Kind: domain.KindClass, Active: true, Values: map[string]domain.CatalogValue{
		"code": textValue("TEST_3I_RACE_CLASS"), "name": textValue("Test 3I Race Clase"), "plural": textValue("Test 3I Race Clases"),
		"slug": textValue("test-3i-race-clase"), "aliases": listValue([]string{}), "keywords": listValue([]string{}),
	}})
	if err != nil || classResult.Record == nil {
		t.Fatalf("insert class fixture: %v", err)
	}
	classID := classResult.Record.ID
	t.Cleanup(func() { _ = legacy.Delete(ctx, domain.KindClass, classID) })

	repo2 := NewCatalogAdminRepositoryV2(pool2)
	familyRec := domain.CatalogRecord{Kind: domain.KindFamily, Active: true, Values: map[string]domain.CatalogValue{
		"class": refValue(domain.KindClass, "TEST_3I_RACE_CLASS", 0), "code": textValue("TEST_3I_RACE_FAM"), "name": textValue("Test 3I Race Familia"),
	}}

	start := make(chan struct{})
	var ready, wg sync.WaitGroup
	ready.Add(2)
	wg.Add(2)
	var insertResult domain.CatalogWriteResult
	var insertErr, deleteErr error
	go func() {
		defer wg.Done()
		ready.Done()
		<-start
		insertResult, insertErr = repo2.Insert(ctx, familyRec)
	}()
	go func() {
		defer wg.Done()
		ready.Done()
		<-start
		_, deleteErr = repo1.Delete(ctx, domain.KindClass, classID, classResult.Record.Revision)
	}()
	ready.Wait()
	close(start)
	wg.Wait()

	insertWon, deleteWon := insertErr == nil, deleteErr == nil
	if insertWon == deleteWon {
		t.Fatalf("race outcome: insert err=%v delete err=%v; want exactly one to succeed, never both/neither", insertErr, deleteErr)
	}
	if insertWon {
		if insertResult.Record == nil || insertResult.Record.ID == 0 {
			t.Fatalf("winning insert result = %+v, want a persisted family", insertResult.Record)
		}
		t.Cleanup(func() { _ = legacy.Delete(ctx, domain.KindFamily, insertResult.Record.ID) })
		if !errors.Is(deleteErr, domain.ErrCatalogInUse) {
			t.Fatalf("losing delete error = %v, want ErrCatalogInUse (real FK dependency proof)", deleteErr)
		}
		if got := countRows(t, ctx, pool1, `SELECT count(*) FROM public.resource_classes WHERE id=$1`, classID); got != 1 {
			t.Fatalf("class row count after blocked delete = %d, want 1 (no corruption)", got)
		}
	} else {
		if !errors.Is(insertErr, domain.ErrCatalogReference) {
			t.Fatalf("losing insert error = %v, want ErrCatalogReference (real missing-parent proof)", insertErr)
		}
		if got := countRows(t, ctx, pool1, `SELECT count(*) FROM public.resource_families WHERE code=$1`, "TEST_3I_RACE_FAM"); got != 0 {
			t.Fatalf("orphan family row count = %d, want 0 (no orphan created)", got)
		}
	}
}

// TestAll11KindsAuthorityOracleIntegration is 4E's closing proof (design
// "Catalog transaction and coherent publication": "All 11 kinds and all
// five operations compare authority with this oracle"). It drives every
// catalog kind through the exact real, composed catalogo.Service
// cmd/garfex/main.go's newCatalogService wires (legacy repository +
// WithCatalogAdminRepositoryV2, 4D): Create publishes through the V2-CAS
// authoritative path while Update/Deactivate/Reactivate/Delete still publish
// through the pre-existing legacy candidate-publish path (deferred to
// 4F/4G) — proving BOTH kinds of path publish exactly once and match a
// genuinely independent, fresh LoadResourceCatalog call after every
// successful mutation, never the locally-built candidate.
func TestAll11KindsAuthorityOracleIntegration(t *testing.T) {
	dsn := os.Getenv("GARFEX_TEST_DSN")
	if dsn == "" {
		t.Skip("GARFEX_TEST_DSN not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	t.Cleanup(pool.Close)
	classCode, familyCode, typeCode, unitCode, characteristic, characteristic2, optionSet, fromOption, toOption := setupCAS2Fixture(t, ctx, pool)

	boot, err := LoadResourceCatalog(ctx, pool)
	if err != nil {
		t.Fatalf("boot LoadResourceCatalog: %v", err)
	}
	authority := domain.NewCatalogAuthority(boot)
	svc := catalogo.NewServiceWithCatalogAuthority(NewCatalogAdminRepository(pool), domain.NewCatalogRegistry(), authority).
		WithCatalogAdminRepositoryV2(NewCatalogAdminRepositoryV2(pool))
	legacy := NewCatalogAdminRepository(pool)

	// verify proves the authority published exactly once since before, AND
	// that the newly published catalog is domain.EquivalentResourceCatalogs
	// to a fresh, independent LoadResourceCatalog call — the real oracle,
	// never the locally-built candidate.
	verify := func(t *testing.T, step string, before uint64) uint64 {
		t.Helper()
		published, after := authority.Current()
		if after != before+1 {
			t.Fatalf("%s: authority version = %d, want %d (published exactly once)", step, after, before+1)
		}
		reloaded, err := LoadResourceCatalog(ctx, pool)
		if err != nil {
			t.Fatalf("%s: independent LoadResourceCatalog: %v", step, err)
		}
		if !domain.EquivalentResourceCatalogs(published, reloaded) {
			t.Fatalf("%s: published authority does not match independent reload", step)
		}
		return after
	}

	rule := domain.CatalogRuleRecord{When: domain.AttributeCondition{AttributeCode: characteristic2, Equals: "X"}, Mode: domain.ModeForbidden, Active: true}

	cases := []struct {
		name   string
		kind   domain.CatalogKindCode
		rec    func() domain.CatalogRecord
		mutate func(domain.CatalogRecord) domain.CatalogRecord
	}{
		{"CLASE", domain.KindClass, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindClass, Active: true, Values: map[string]domain.CatalogValue{
				"code": textValue("TEST_4E_CLASS"), "name": textValue("Test 4E Clase"), "plural": textValue("Test 4E Clases"),
				"slug": textValue("test-4e-clase"), "aliases": listValue([]string{}), "keywords": listValue([]string{}),
			}}
		}, func(r domain.CatalogRecord) domain.CatalogRecord { r.Values["name"] = textValue("Renamed"); return r }},
		{"FAMILIA", domain.KindFamily, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindFamily, Active: true, Values: map[string]domain.CatalogValue{
				"class": refValue(domain.KindClass, classCode, 0), "code": textValue("TEST_4E_FAM"), "name": textValue("Test 4E Familia"),
			}}
		}, func(r domain.CatalogRecord) domain.CatalogRecord { r.Values["name"] = textValue("Renamed"); return r }},
		{"TIPO", domain.KindType, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindType, Active: true, Values: map[string]domain.CatalogValue{
				"class": refValue(domain.KindClass, classCode, 0), "family": refValue(domain.KindFamily, familyCode, 0),
				"code": textValue("TEST_4E_TIPO"), "name": textValue("Test 4E Tipo"),
			}}
		}, func(r domain.CatalogRecord) domain.CatalogRecord { r.Values["name"] = textValue("Renamed"); return r }},
		{"CARACTERISTICA", domain.KindAttributeDefinition, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindAttributeDefinition, Active: true, Values: map[string]domain.CatalogValue{
				"code": textValue("test_4e_char"), "name": textValue("Test 4E Característica"), "valueType": textValue(string(domain.ValueTypeControlledText)),
			}}
		}, func(r domain.CatalogRecord) domain.CatalogRecord { r.Values["name"] = textValue("Renamed"); return r }},
		{"CONJUNTO_OPCIONES", domain.KindOptionSet, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindOptionSet, Active: true, Values: map[string]domain.CatalogValue{
				"code": textValue("TEST_4E_OPTSET"), "name": textValue("Test 4E Conjunto"),
			}}
		}, func(r domain.CatalogRecord) domain.CatalogRecord { r.Values["name"] = textValue("Renamed"); return r }},
		{"OPCION", domain.KindOption, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindOption, Active: true, Values: map[string]domain.CatalogValue{
				"optionSet": refValue(domain.KindOptionSet, optionSet, 0), "characteristic": refValue(domain.KindAttributeDefinition, characteristic, 0),
				"code": textValue("TEST_4E_OPT"), "label": textValue("Test 4E Option"),
			}}
		}, func(r domain.CatalogRecord) domain.CatalogRecord { r.Values["label"] = textValue("Renamed"); return r }},
		{"RELACION_OPCIONES", domain.KindOptionRelation, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindOptionRelation, Active: true, Values: map[string]domain.CatalogValue{
				"optionSet":          refValue(domain.KindOptionSet, optionSet, 0),
				"fromCharacteristic": refValue(domain.KindAttributeDefinition, characteristic, 0), "fromOption": refValue(domain.KindOption, fromOption, 0),
				"toCharacteristic": refValue(domain.KindAttributeDefinition, characteristic2, 0), "toOption": refValue(domain.KindOption, toOption, 0),
			}}
		}, func(r domain.CatalogRecord) domain.CatalogRecord { return r }},
		{"UNIDAD", domain.KindUnit, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindUnit, Active: true, Values: map[string]domain.CatalogValue{
				"code": textValue("TEST_4E_UNIT"), "name": textValue("Test 4E Unit"), "symbol": textValue("t4"), "dimension": textValue("Longitud"),
			}}
		}, func(r domain.CatalogRecord) domain.CatalogRecord { r.Values["name"] = textValue("Renamed"); return r }},
		{"POLITICA_UNIDAD", domain.KindUnitPolicy, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindUnitPolicy, Active: true, Values: map[string]domain.CatalogValue{
				"class": refValue(domain.KindClass, classCode, 0), "family": refValue(domain.KindFamily, familyCode, 0), "unit": refValue(domain.KindUnit, unitCode, 0),
				"allowed": boolValue(true), "suggested": boolValue(false),
			}}
		}, func(r domain.CatalogRecord) domain.CatalogRecord { r.Values["suggested"] = boolValue(true); return r }},
		{"APLICABILIDAD", domain.KindAttributeBinding, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindAttributeBinding, Active: true, Values: map[string]domain.CatalogValue{
				"class": refValue(domain.KindClass, classCode, 0), "family": refValue(domain.KindFamily, familyCode, 0),
				"characteristic": refValue(domain.KindAttributeDefinition, characteristic2, 0), "mode": textValue(string(domain.ModeConditional)), "identityParticipates": boolValue(true),
			}, Rules: []domain.CatalogRuleRecord{rule}}
			// mutate deliberately never touches Rules: the legacy (non-CAS)
			// updateAttributeBinding never rewrites resource_attribute_rules,
			// so a Rules change here would make Update's candidate diverge
			// from persistence — a genuine pre-existing legacy-Update gap,
			// out of this slice's scope (see apply-progress.md 4E).
		}, func(r domain.CatalogRecord) domain.CatalogRecord {
			r.Values["identityParticipates"] = boolValue(false)
			return r
		}},
		{"PRESENTACION", domain.KindPresentationField, func() domain.CatalogRecord {
			return domain.CatalogRecord{Kind: domain.KindPresentationField, Active: true, Values: map[string]domain.CatalogValue{
				"class": refValue(domain.KindClass, classCode, 0), "family": refValue(domain.KindFamily, familyCode, 0), "type": refValue(domain.KindType, typeCode, 0),
				"characteristic": refValue(domain.KindAttributeDefinition, characteristic, 0), "position": intValue(0),
			}}
		}, func(r domain.CatalogRecord) domain.CatalogRecord { r.Values["position"] = intValue(1); return r }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, before := authority.Current()
			created, err := svc.Create(ctx, tc.kind, tc.rec())
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			id := created.ID
			t.Cleanup(func() { _ = legacy.Delete(ctx, tc.kind, id) })
			before = verify(t, "Create", before)

			updated := tc.mutate(tc.rec())
			updated.ID = id
			if _, err := svc.Update(ctx, tc.kind, updated); err != nil {
				t.Fatalf("Update: %v", err)
			}
			before = verify(t, "Update", before)

			if err := svc.Deactivate(ctx, tc.kind, id); err != nil {
				t.Fatalf("Deactivate: %v", err)
			}
			before = verify(t, "Deactivate", before)

			if err := svc.Reactivate(ctx, tc.kind, id); err != nil {
				t.Fatalf("Reactivate: %v", err)
			}
			before = verify(t, "Reactivate", before)

			if err := svc.Deactivate(ctx, tc.kind, id); err != nil {
				t.Fatalf("Deactivate before delete: %v", err)
			}
			before = verify(t, "Deactivate before delete", before)

			if err := svc.Delete(ctx, tc.kind, id); err != nil {
				t.Fatalf("Delete: %v", err)
			}
			verify(t, "Delete", before)
		})
	}

	// TRIANGULATE: a rejected mutation must leave persistence AND authority
	// completely unchanged, proven via a genuine independent reload — not
	// just err != nil.
	t.Run("rejected Create leaves persistence and authority unchanged against an independent reload", func(t *testing.T) {
		_, before := authority.Current()
		beforeReload, err := LoadResourceCatalog(ctx, pool)
		if err != nil {
			t.Fatalf("independent LoadResourceCatalog: %v", err)
		}
		dup := domain.CatalogRecord{Kind: domain.KindClass, Active: true, Values: map[string]domain.CatalogValue{
			"code": textValue(classCode), "name": textValue("Duplicate"), "plural": textValue("Duplicates"),
			"slug": textValue("duplicate"), "aliases": listValue([]string{}), "keywords": listValue([]string{}),
		}}
		if _, err := svc.Create(ctx, domain.KindClass, dup); err == nil {
			t.Fatal("Create with duplicate class code = nil error, want rejection")
		}
		if _, after := authority.Current(); after != before {
			t.Fatalf("authority version = %d after rejected Create, want unchanged %d", after, before)
		}
		afterReload, err := LoadResourceCatalog(ctx, pool)
		if err != nil {
			t.Fatalf("independent LoadResourceCatalog: %v", err)
		}
		if !domain.EquivalentResourceCatalogs(beforeReload, afterReload) {
			t.Fatal("independent reload changed after a rejected Create")
		}
	})
}
