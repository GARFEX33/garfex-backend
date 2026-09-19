package recursos

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/core"
	"github.com/GARFEX33/garfex-costos-unitarios/internal/domain"
)

// fakeAttributeOrderStore is a fake domain.AttributeOrderStore. It never
// touches a real database: ReadAttributeOrder/WriteAttributeOrder just
// record what they received and return whatever the test seeded.
type fakeAttributeOrderStore struct {
	gotReadScope domain.ResourceScope
	readResult   domain.AttributeOrderReadResult
	readErr      error

	gotWriteRequest domain.AttributeOrderWriteRequest
	writeResult     domain.AttributeOrderReadResult
	writeErr        error
}

func (f *fakeAttributeOrderStore) ReadAttributeOrder(_ context.Context, scope domain.ResourceScope) (domain.AttributeOrderReadResult, error) {
	f.gotReadScope = scope
	return f.readResult, f.readErr
}

func (f *fakeAttributeOrderStore) WriteAttributeOrder(_ context.Context, req domain.AttributeOrderWriteRequest) (domain.AttributeOrderReadResult, error) {
	f.gotWriteRequest = req
	return f.writeResult, f.writeErr
}

func attributeOrderScope() domain.ResourceScope {
	return domain.ResourceScope{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"}
}

// distinguishableAttributeOrderResult builds an AttributeOrderReadResult
// whose Catalog is deliberately different from domain.SeedResourceCatalog()
// (it carries a class code no seeded fixture uses), so a test can prove the
// distinguishable catalog never leaks into or replaces what a Service's
// s.authority.Current()/Describe/EffectiveAttributes expose.
func distinguishableAttributeOrderResult() domain.AttributeOrderReadResult {
	catalog := domain.ResourceCatalog{
		Classes: []domain.ResourceClass{{Code: "ATTRIBUTE_ORDER_ONLY_CLASS", Name: "Attribute Order Only", Active: true}},
	}
	order := domain.AttributeOrderSnapshot{
		Scope:             attributeOrderScope(),
		OrderedAttributes: []domain.AttributeOrderKey{{SourceLevel: "TYPE", SourceCode: "CABLE", CharacteristicCode: "voltage"}},
		OrderRevision:     "rev-1",
	}
	return domain.NewAttributeOrderReadResult(catalog, order)
}

// TestServiceAttributeOrderStoreUnconfiguredByDefault proves that a Service
// built via the existing constructors (no WithAttributeOrderStore call)
// reports the capability unconfigured and rejects both new methods with
// ErrAttributeOrderStoreUnavailable, classifiable via core.ErrUnavailable —
// matching the catalogo package's own established assertion style.
func TestServiceAttributeOrderStoreUnconfiguredByDefault(t *testing.T) {
	svc := NewService(&fakeRepo{}, domain.SeedResourceCatalog())

	if svc.AttributeOrderStoreConfigured() {
		t.Fatal("AttributeOrderStoreConfigured() = true, want false before WithAttributeOrderStore")
	}

	if _, err := svc.ReadAttributeOrder(context.Background(), attributeOrderScope()); !errors.Is(err, ErrAttributeOrderStoreUnavailable) {
		t.Fatalf("ReadAttributeOrder() error = %v, want ErrAttributeOrderStoreUnavailable", err)
	} else if !errors.Is(err, core.ErrUnavailable) {
		t.Fatalf("ReadAttributeOrder() error = %v, want errors.Is(_, core.ErrUnavailable)", err)
	}

	req := domain.AttributeOrderWriteRequest{Scope: attributeOrderScope(), ExpectedOrderRevision: "rev-1"}
	if _, err := svc.WriteAttributeOrder(context.Background(), req); !errors.Is(err, ErrAttributeOrderStoreUnavailable) {
		t.Fatalf("WriteAttributeOrder() error = %v, want ErrAttributeOrderStoreUnavailable", err)
	} else if !errors.Is(err, core.ErrUnavailable) {
		t.Fatalf("WriteAttributeOrder() error = %v, want errors.Is(_, core.ErrUnavailable)", err)
	}
}

// TestServiceWithAttributeOrderStorePassesThrough proves WithAttributeOrderStore
// wires a genuinely thin pass-through: success returns exactly the fake
// store's result, and a propagated domain sentinel error still satisfies
// errors.Is after crossing Service.WriteAttributeOrder — i.e. C4 never wraps
// it in a way that would break caller error classification.
func TestServiceWithAttributeOrderStorePassesThrough(t *testing.T) {
	wantRead := distinguishableAttributeOrderResult()
	store := &fakeAttributeOrderStore{readResult: wantRead}
	svc := NewService(&fakeRepo{}, domain.SeedResourceCatalog()).WithAttributeOrderStore(store)

	if !svc.AttributeOrderStoreConfigured() {
		t.Fatal("AttributeOrderStoreConfigured() = false, want true after WithAttributeOrderStore")
	}

	scope := attributeOrderScope()
	got, err := svc.ReadAttributeOrder(context.Background(), scope)
	if err != nil {
		t.Fatalf("ReadAttributeOrder() error = %v, want nil", err)
	}
	if got.Order.OrderRevision != wantRead.Order.OrderRevision || len(got.Catalog.Classes) != 1 || got.Catalog.Classes[0].Code != "ATTRIBUTE_ORDER_ONLY_CLASS" {
		t.Fatalf("ReadAttributeOrder() = %+v, want the fake store's own result %+v", got, wantRead)
	}
	if store.gotReadScope != scope {
		t.Fatalf("store received scope = %+v, want %+v", store.gotReadScope, scope)
	}

	// Propagated success on WriteAttributeOrder.
	store.writeResult = wantRead
	writeReq := domain.AttributeOrderWriteRequest{Scope: scope, ExpectedOrderRevision: "rev-1", OrderedAttributes: wantRead.Order.OrderedAttributes}
	gotWrite, err := svc.WriteAttributeOrder(context.Background(), writeReq)
	if err != nil {
		t.Fatalf("WriteAttributeOrder() error = %v, want nil", err)
	}
	if gotWrite.Order.OrderRevision != wantRead.Order.OrderRevision {
		t.Fatalf("WriteAttributeOrder() = %+v, want the fake store's own result %+v", gotWrite, wantRead)
	}
	if !reflect.DeepEqual(store.gotWriteRequest, writeReq) {
		t.Fatalf("store received request = %+v, want %+v", store.gotWriteRequest, writeReq)
	}

	// Propagated domain sentinel error on WriteAttributeOrder: C4 must not
	// reclassify or wrap it in a way that breaks errors.Is.
	store.writeErr = domain.ErrAttributeOrderRevisionConflict
	if _, err := svc.WriteAttributeOrder(context.Background(), writeReq); !errors.Is(err, domain.ErrAttributeOrderRevisionConflict) {
		t.Fatalf("WriteAttributeOrder() error = %v, want errors.Is(_, domain.ErrAttributeOrderRevisionConflict)", err)
	}
}

// TestServiceAttributeOrderStoreNeverPublishesToCatalogAuthority is the
// parent plan's required regression proof: reads/writes through the
// attribute-order capability must never mutate or replace the Service's
// cached s.authority snapshot. The fake store returns a deliberately
// distinguishable ResourceCatalog; this test proves it never leaks into
// s.authority.Current(), Describe, or EffectiveAttributes.
func TestServiceAttributeOrderStoreNeverPublishesToCatalogAuthority(t *testing.T) {
	seeded := domain.SeedResourceCatalog()
	authority := domain.NewCatalogAuthority(seeded)
	svc := NewServiceWithCatalogAuthority(&fakeRepo{}, authority).WithAttributeOrderStore(&fakeAttributeOrderStore{
		readResult:  distinguishableAttributeOrderResult(),
		writeResult: distinguishableAttributeOrderResult(),
	})

	sampleResource := domain.Resource{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE", NaturalUnit: "M", IdentityKey: "v1|MATERIAL|CONDUCTORES|CABLE"}
	_, beforeVersion := authority.Current()
	describeBefore := svc.Describe(sampleResource)
	effectiveBefore, effErrBefore := svc.EffectiveAttributes(attributeOrderScope(), nil)

	if _, err := svc.ReadAttributeOrder(context.Background(), attributeOrderScope()); err != nil {
		t.Fatalf("ReadAttributeOrder() error = %v, want nil", err)
	}
	writeReq := domain.AttributeOrderWriteRequest{Scope: attributeOrderScope(), ExpectedOrderRevision: "rev-1"}
	if _, err := svc.WriteAttributeOrder(context.Background(), writeReq); err != nil {
		t.Fatalf("WriteAttributeOrder() error = %v, want nil", err)
	}

	_, afterVersion := authority.Current()
	if afterVersion != beforeVersion {
		t.Fatalf("authority version changed from %d to %d: attribute order capability must never publish", beforeVersion, afterVersion)
	}
	if got := svc.Describe(sampleResource); got != describeBefore {
		t.Fatalf("Describe() changed from %q to %q after attribute order calls", describeBefore, got)
	}
	effectiveAfter, effErrAfter := svc.EffectiveAttributes(attributeOrderScope(), nil)
	if !errors.Is(effErrAfter, effErrBefore) && (effErrBefore == nil) != (effErrAfter == nil) {
		t.Fatalf("EffectiveAttributes() error changed from %v to %v", effErrBefore, effErrAfter)
	}
	if len(effectiveAfter) != len(effectiveBefore) {
		t.Fatalf("EffectiveAttributes() changed length from %d to %d after attribute order calls", len(effectiveBefore), len(effectiveAfter))
	}
}

// TestServiceAttributeOrderStoreIsPerInstance proves WithAttributeOrderStore
// is a genuinely per-instance additive wire, not shared/global state: two
// independent *Service instances, only one configured, and the other must
// still report ErrAttributeOrderStoreUnavailable.
func TestServiceAttributeOrderStoreIsPerInstance(t *testing.T) {
	configured := NewServiceWithCatalogAuthority(&fakeRepo{}, domain.NewCatalogAuthority(domain.SeedResourceCatalog())).
		WithAttributeOrderStore(&fakeAttributeOrderStore{readResult: distinguishableAttributeOrderResult()})
	unconfigured := NewServiceWithCatalogAuthority(&fakeRepo{}, domain.NewCatalogAuthority(domain.SeedResourceCatalog()))

	if !configured.AttributeOrderStoreConfigured() {
		t.Fatal("configured.AttributeOrderStoreConfigured() = false, want true")
	}
	if unconfigured.AttributeOrderStoreConfigured() {
		t.Fatal("unconfigured.AttributeOrderStoreConfigured() = true, want false")
	}

	if _, err := unconfigured.ReadAttributeOrder(context.Background(), attributeOrderScope()); !errors.Is(err, ErrAttributeOrderStoreUnavailable) {
		t.Fatalf("unconfigured.ReadAttributeOrder() error = %v, want ErrAttributeOrderStoreUnavailable", err)
	}
	req := domain.AttributeOrderWriteRequest{Scope: attributeOrderScope(), ExpectedOrderRevision: "rev-1"}
	if _, err := unconfigured.WriteAttributeOrder(context.Background(), req); !errors.Is(err, ErrAttributeOrderStoreUnavailable) {
		t.Fatalf("unconfigured.WriteAttributeOrder() error = %v, want ErrAttributeOrderStoreUnavailable", err)
	}

	if _, err := configured.ReadAttributeOrder(context.Background(), attributeOrderScope()); err != nil {
		t.Fatalf("configured.ReadAttributeOrder() error = %v, want nil", err)
	}
}
