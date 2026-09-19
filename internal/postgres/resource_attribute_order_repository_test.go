package postgres

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestAttributeOrderReadRejectsBeforeIO(t *testing.T) {
	repo := NewAttributeOrderRepository(nil)
	for _, tt := range []struct {
		name     string
		scope    domain.ResourceScope
		canceled bool
		want     error
	}{
		{name: "missing type", scope: domain.ResourceScope{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES"}, want: domain.ErrResourceValidation},
		{name: "canceled", scope: domain.ResourceScope{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"}, canceled: true, want: context.Canceled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tt.canceled {
				cancel()
			}
			if _, err := repo.ReadAttributeOrder(ctx, tt.scope); !errors.Is(err, tt.want) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

// Embedding the existing transaction interface lets these query/scan failures
// be tested without a new mocking dependency or a production injection seam.
// execErr/execs extend the double for WriteAttributeOrder's Exec-based lock
// and mutation calls (structural LOCK TABLE, delete/insert/upsert); the
// existing read-only tests never call Exec, so they are unaffected.
type attributeOrderFailureTx struct {
	pgx.Tx
	ctx                context.Context
	scopeErr, queryErr error
	execErr            error
	queries, execs     int
}

func (tx *attributeOrderFailureTx) QueryRow(ctx context.Context, _ string, _ ...any) pgx.Row {
	tx.ctx = ctx
	return attributeOrderScopeRow{err: tx.scopeErr}
}

func (tx *attributeOrderFailureTx) Query(ctx context.Context, _ string, _ ...any) (pgx.Rows, error) {
	tx.ctx = ctx
	tx.queries++
	return nil, tx.queryErr
}

func (tx *attributeOrderFailureTx) Exec(ctx context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	tx.ctx = ctx
	tx.execs++
	return pgconn.CommandTag{}, tx.execErr
}

// attributeOrderScopeRow.Scan serves both the reader's three-column scope
// query (class/family lookup joined with head revision) and the writer's
// single-column locked-type-id query, switching on the caller's own dest
// count rather than adding a second row type.
type attributeOrderScopeRow struct{ err error }

func (r attributeOrderScopeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	switch len(dest) {
	case 1:
		*dest[0].(*int64) = 1
	case 3:
		*dest[0].(*int64), *dest[1].(*int64), *dest[2].(*uint64) = 1, 1, 0
	}
	return nil
}

func TestAttributeOrderReadTransactionFailures(t *testing.T) {
	failure := errors.New("injected query or scan failure")
	for _, tt := range []struct {
		name                     string
		scopeErr, queryErr, want error
		queries                  int
	}{
		{name: "missing scope", scopeErr: pgx.ErrNoRows, want: domain.ErrCatalogRecordNotFound},
		{name: "scope scan failure", scopeErr: failure, want: failure},
		{name: "query canceled", scopeErr: context.Canceled, want: context.Canceled},
		{name: "catalog query failure", queryErr: failure, want: failure, queries: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tx := &attributeOrderFailureTx{scopeErr: tt.scopeErr, queryErr: tt.queryErr}
			ctx := t.Context()
			result, err := readAttributeOrderTx(ctx, tx, domain.ResourceScope{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"})
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v", err)
			}
			if tx.ctx != ctx || tx.queries != tt.queries {
				t.Fatal("context or query sequence changed")
			}
			if !reflect.DeepEqual(result, domain.AttributeOrderReadResult{}) {
				t.Fatal("failure leaked partial snapshot")
			}
		})
	}
}

func TestAttributeOrderReadAssembly(t *testing.T) {
	catalog := domain.SeedResourceCatalog()
	scope := domain.ResourceScope{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"}
	effective, err := catalog.EffectiveAttributesFor(scope, nil)
	if err != nil {
		t.Fatal(err)
	}
	members := make([]domain.AttributeOrderMember, len(effective))
	for i, a := range effective {
		members[i] = domain.AttributeOrderMember{Key: domain.AttributeOrderKey{
			SourceLevel: a.SourceLevel, SourceCode: a.SourceCode, CharacteristicCode: a.Attribute.Definition.Code,
		}, ResourceAttributeID: int64(i + 1), Revision: 1}
	}
	for _, tt := range []struct {
		name      string
		alter     func(*domain.AttributeOrderState)
		wantError bool
	}{
		{name: "baseline"},
		{name: "saved", alter: func(s *domain.AttributeOrderState) { s.SavedBindingIDs = []int64{5, 2, 99} }},
		{name: "missing binding", alter: func(s *domain.AttributeOrderState) { s.Members = s.Members[:4] }, wantError: true},
		{name: "duplicate binding", alter: func(s *domain.AttributeOrderState) { s.Members[1] = s.Members[0] }, wantError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			state := domain.AttributeOrderState{TypeID: 1, Members: append([]domain.AttributeOrderMember(nil), members...)}
			if tt.alter != nil {
				tt.alter(&state)
			}
			got, err := assembleAttributeOrderRead(catalog, scope, state)
			if (err != nil) != tt.wantError {
				t.Fatalf("error=%v", err)
			}
			if tt.wantError {
				return
			}
			want, err := domain.ResolveAttributeOrder(scope, state)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Order, want) || !reflect.DeepEqual(got.Catalog, catalog) {
				t.Fatal("coherent result mismatch")
			}
			got.Catalog.Types[0].Name = "mutated"
			if catalog.Types[0].Name == "mutated" {
				t.Fatal("catalog aliases input")
			}
		})
	}
}

var (
	_ domain.AttributeOrderWriter = (*attributeOrderRepository)(nil)
	_ domain.AttributeOrderStore  = (*attributeOrderRepository)(nil)
)

func TestAttributeOrderWriteRejectsBeforeIO(t *testing.T) {
	repo := NewAttributeOrderRepositoryFull(nil)
	validScope := domain.ResourceScope{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"}
	for _, tt := range []struct {
		name     string
		req      domain.AttributeOrderWriteRequest
		canceled bool
		want     error
	}{
		{name: "missing type", req: domain.AttributeOrderWriteRequest{Scope: domain.ResourceScope{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES"}, ExpectedOrderRevision: "v1:x"}, want: domain.ErrResourceValidation},
		{name: "empty expected revision", req: domain.AttributeOrderWriteRequest{Scope: validScope}, want: domain.ErrResourceValidation},
		{name: "blank expected revision", req: domain.AttributeOrderWriteRequest{Scope: validScope, ExpectedOrderRevision: "   "}, want: domain.ErrResourceValidation},
		{name: "canceled", req: domain.AttributeOrderWriteRequest{Scope: validScope, ExpectedOrderRevision: "v1:x"}, canceled: true, want: context.Canceled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tt.canceled {
				cancel()
			}
			if _, err := repo.WriteAttributeOrder(ctx, tt.req); !errors.Is(err, tt.want) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestAttributeOrderWriteTransactionFailures(t *testing.T) {
	failure := errors.New("injected write failure")
	scope := domain.ResourceScope{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"}
	req := domain.AttributeOrderWriteRequest{Scope: scope, ExpectedOrderRevision: "v1:x"}
	for _, tt := range []struct {
		name                        string
		scopeErr, execErr, queryErr error
		want                        error
		wantExecs, wantQueries      int
	}{
		{name: "missing target type", scopeErr: pgx.ErrNoRows, want: domain.ErrCatalogRecordNotFound},
		{name: "lock scan failure", scopeErr: failure, want: failure},
		{name: "lock canceled", scopeErr: context.Canceled, want: context.Canceled},
		{name: "structural lock failure", execErr: failure, want: failure, wantExecs: 1},
		{name: "state read failure after locks held", queryErr: failure, want: failure, wantExecs: 1, wantQueries: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tx := &attributeOrderFailureTx{scopeErr: tt.scopeErr, execErr: tt.execErr, queryErr: tt.queryErr}
			ctx := t.Context()
			result, err := writeAttributeOrderTx(ctx, tx, scope, req)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v", err)
			}
			if tx.ctx != ctx || tx.execs != tt.wantExecs || tx.queries != tt.wantQueries {
				t.Fatalf("execs=%d queries=%d", tx.execs, tx.queries)
			}
			if !reflect.DeepEqual(result, domain.AttributeOrderReadResult{}) {
				t.Fatal("failure leaked partial result")
			}
		})
	}
}

func attributeOrderPlanFixture() (domain.ResourceCatalog, domain.ResourceScope, domain.AttributeOrderState, domain.AttributeOrderSnapshot) {
	catalog := domain.SeedResourceCatalog()
	scope := domain.ResourceScope{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"}
	effective, err := catalog.EffectiveAttributesFor(scope, nil)
	if err != nil {
		panic(err)
	}
	state := domain.AttributeOrderState{TypeID: 1}
	for i, a := range effective {
		state.Members = append(state.Members, domain.AttributeOrderMember{
			Key:                 domain.AttributeOrderKey{SourceLevel: a.SourceLevel, SourceCode: a.SourceCode, CharacteristicCode: a.Attribute.Definition.Code},
			ResourceAttributeID: int64(i + 1), Revision: 1,
		})
	}
	current, err := domain.ResolveAttributeOrder(scope, state)
	if err != nil {
		panic(err)
	}
	return catalog, scope, state, current
}

func TestAttributeOrderWritePlan(t *testing.T) {
	catalog, scope, state, current := attributeOrderPlanFixture()
	t.Run("stale revision", func(t *testing.T) {
		req := domain.AttributeOrderWriteRequest{Scope: scope, ExpectedOrderRevision: "v1:stale", OrderedAttributes: current.OrderedAttributes}
		if _, _, err := planAttributeOrderWrite(catalog, scope, state, req); !errors.Is(err, domain.ErrAttributeOrderRevisionConflict) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("invalid permutation", func(t *testing.T) {
		req := domain.AttributeOrderWriteRequest{Scope: scope, ExpectedOrderRevision: current.OrderRevision, OrderedAttributes: current.OrderedAttributes[:len(current.OrderedAttributes)-1]}
		if _, _, err := planAttributeOrderWrite(catalog, scope, state, req); !errors.Is(err, domain.ErrResourceValidation) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("unknown key", func(t *testing.T) {
		bogus := slices.Clone(current.OrderedAttributes)
		bogus[0].CharacteristicCode = "does-not-exist"
		req := domain.AttributeOrderWriteRequest{Scope: scope, ExpectedOrderRevision: current.OrderRevision, OrderedAttributes: bogus}
		if _, _, err := planAttributeOrderWrite(catalog, scope, state, req); !errors.Is(err, domain.ErrResourceValidation) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("no-op reorder still accepted", func(t *testing.T) {
		req := domain.AttributeOrderWriteRequest{Scope: scope, ExpectedOrderRevision: current.OrderRevision, OrderedAttributes: current.OrderedAttributes}
		keys, ids, err := planAttributeOrderWrite(catalog, scope, state, req)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(keys, current.OrderedAttributes) {
			t.Fatalf("keys = %+v", keys)
		}
		for i, id := range ids {
			if id != state.Members[i].ResourceAttributeID {
				t.Fatalf("ids = %+v", ids)
			}
		}
	})
	t.Run("full reverse", func(t *testing.T) {
		reversed := slices.Clone(current.OrderedAttributes)
		slices.Reverse(reversed)
		req := domain.AttributeOrderWriteRequest{Scope: scope, ExpectedOrderRevision: current.OrderRevision, OrderedAttributes: reversed}
		keys, ids, err := planAttributeOrderWrite(catalog, scope, state, req)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(keys, reversed) {
			t.Fatalf("keys = %+v", keys)
		}
		n := len(state.Members)
		for i, id := range ids {
			if id != state.Members[n-1-i].ResourceAttributeID {
				t.Fatalf("ids = %+v", ids)
			}
		}
	})
}

// TestAttributeOrderWritePlanRoundTripsMixedCaseCharacteristicCode reproduces
// a real production bug reported against Cable de control's "NUMH"
// characteristic: a client resending the exact, unmodified order a prior GET
// returned must never fail. Characteristic codes are stored as given (only
// comparisons canonicalize them, per resource_canonical.go's
// canonicalAttribute callers), so a mixed-case persisted code is legitimate
// and must resolve identically to an all-lowercase one.
func TestAttributeOrderWritePlanRoundTripsMixedCaseCharacteristicCode(t *testing.T) {
	catalog := domain.SeedResourceCatalog()
	for i := range catalog.Attributes {
		a := &catalog.Attributes[i]
		if a.ClassCode == "MATERIAL" && a.FamilyCode == "CONDUCTORES" && a.TypeCode == "CABLE" && a.Definition.Code == "color" {
			a.Definition.Code = "COLOR"
		}
	}
	scope := domain.ResourceScope{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"}
	effective, err := catalog.EffectiveAttributesFor(scope, nil)
	if err != nil {
		t.Fatal(err)
	}
	state := domain.AttributeOrderState{TypeID: 1}
	for i, a := range effective {
		state.Members = append(state.Members, domain.AttributeOrderMember{
			Key:                 domain.AttributeOrderKey{SourceLevel: a.SourceLevel, SourceCode: a.SourceCode, CharacteristicCode: a.Attribute.Definition.Code},
			ResourceAttributeID: int64(i + 1), Revision: 1,
		})
	}
	current, err := domain.ResolveAttributeOrder(scope, state)
	if err != nil {
		t.Fatal(err)
	}
	req := domain.AttributeOrderWriteRequest{Scope: scope, ExpectedOrderRevision: current.OrderRevision, OrderedAttributes: current.OrderedAttributes}
	if _, _, err := planAttributeOrderWrite(catalog, scope, state, req); err != nil {
		t.Fatalf("round-tripping GET's own order for a mixed-case characteristic code must not fail: %v", err)
	}
}

func TestAttributeOrderCommitFailureClassification(t *testing.T) {
	if classifyAttributeOrderCommitFailure(nil) != nil {
		t.Fatal("nil must stay nil")
	}
	cause := errors.New("commit boom")
	err := classifyAttributeOrderCommitFailure(cause)
	if !errors.Is(err, domain.ErrAttributeOrderUnavailable) {
		t.Fatalf("error = %v", err)
	}
	if errors.Is(err, domain.ErrAttributeOrderRevisionConflict) {
		t.Fatal("commit failure must not classify as a conflict")
	}
}

func TestAttributeOrderPostWriteVerification(t *testing.T) {
	want := []domain.AttributeOrderKey{{SourceLevel: domain.SourceLevelType, SourceCode: "T", CharacteristicCode: "size"}}
	if err := verifyAttributeOrderWrite(want, want); err != nil {
		t.Fatal(err)
	}
	got := []domain.AttributeOrderKey{{SourceLevel: domain.SourceLevelType, SourceCode: "T", CharacteristicCode: "color"}}
	if err := verifyAttributeOrderWrite(got, want); !errors.Is(err, domain.ErrResourceValidation) {
		t.Fatalf("error = %v", err)
	}
}
