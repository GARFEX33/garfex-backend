package resourcecore

import (
	"context"
	"errors"
	"testing"
)

type readerFakeCapabilities struct {
	activeClasses       []CatalogRecord
	descriptors         []CatalogDescriptor
	listPage            CatalogPage
	getRecord           CatalogRecord
	searchPage          ResourcePage
	getResource         Resource
	describeText        string
	effectiveAttributes []EffectiveAttribute
	activeClassesErr    error
	descriptorsErr      error
	listErr             error
	getErr              error
	searchErr           error
	getResourceErr      error
	describeErr         error
	effectiveAttrsErr   error
	evaluateAttrsErr    error
	activeClassesCalls  int
	descriptorsCalls    int
	listCalls           int
	getCalls            int
	searchCalls         int
	getResourceCalls    int
	describeCalls       int
	effectiveAttrsCalls int
	evaluateAttrsCalls  int
	evaluateAttrsValues []AttributeValue
}

func (f *readerFakeCapabilities) ActiveClasses(ctx context.Context) ([]CatalogRecord, error) {
	f.activeClassesCalls++
	return f.activeClasses, f.activeClassesErr
}

func (f *readerFakeCapabilities) CatalogDescriptors(ctx context.Context) ([]CatalogDescriptor, error) {
	f.descriptorsCalls++
	return f.descriptors, f.descriptorsErr
}

func (f *readerFakeCapabilities) ListCatalog(ctx context.Context, q CatalogQuery) (CatalogPage, error) {
	f.listCalls++
	return f.listPage, f.listErr
}

func (f *readerFakeCapabilities) GetCatalog(ctx context.Context, key CatalogKey) (CatalogRecord, error) {
	f.getCalls++
	return f.getRecord, f.getErr
}

func (f *readerFakeCapabilities) SearchResources(ctx context.Context, q ResourceQuery) (ResourcePage, error) {
	f.searchCalls++
	return f.searchPage, f.searchErr
}

func (f *readerFakeCapabilities) GetResource(ctx context.Context, key ResourceKey) (Resource, error) {
	f.getResourceCalls++
	return f.getResource, f.getResourceErr
}

func (f *readerFakeCapabilities) DescribeResource(ctx context.Context, key ResourceKey) (string, error) {
	f.describeCalls++
	return f.describeText, f.describeErr
}

func (f *readerFakeCapabilities) EffectiveAttributesFor(ctx context.Context, scope ResourceScope) ([]EffectiveAttribute, error) {
	f.effectiveAttrsCalls++
	return f.effectiveAttributes, f.effectiveAttrsErr
}

func (f *readerFakeCapabilities) EvaluateAttributes(ctx context.Context, scope ResourceScope, values []AttributeValue) ([]EffectiveAttribute, error) {
	f.evaluateAttrsCalls++
	f.evaluateAttrsValues = values
	return f.effectiveAttributes, f.evaluateAttrsErr
}

func TestNewReadOnly_NilCapabilitiesIsInvalidArgument(t *testing.T) {
	r, err := NewReadOnly(nil)
	if r != nil {
		t.Fatalf("expected nil reader, got %+v", r)
	}
	if !IsCode(err, InvalidArgument) {
		t.Fatalf("expected INVALID_ARGUMENT, got %v", err)
	}
}

func TestReader_ActiveClassesCopiesResult(t *testing.T) {
	shared := []CatalogRecord{{Kind: KindClass, ID: 1, Values: map[string]Value{"code": {Kind: ValueCode, Text: "C"}}}}
	fake := &readerFakeCapabilities{activeClasses: shared}
	r, err := NewReadOnly(fake)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := r.ActiveClasses(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Values["code"].Text != "C" {
		t.Fatalf("unexpected result: %+v", got)
	}
	got[0].Values["code"] = Value{Kind: ValueCode, Text: "Z"}
	if shared[0].Values["code"].Text != "C" {
		t.Fatalf("capability state leaked after caller mutation")
	}
}

func TestReader_CatalogDescriptorsCopiesResult(t *testing.T) {
	shared := []CatalogDescriptor{{Kind: KindClass, Fields: []FieldDescriptor{{Name: "code"}}}}
	fake := &readerFakeCapabilities{descriptors: shared}
	r, _ := NewReadOnly(fake)
	got, err := r.CatalogDescriptors(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got[0].Fields[0].Name = "changed"
	if shared[0].Fields[0].Name != "code" {
		t.Fatalf("capability state leaked after caller mutation")
	}
}

func TestReader_ListCatalogValidatesKind(t *testing.T) {
	fake := &readerFakeCapabilities{}
	r, _ := NewReadOnly(fake)
	_, err := r.ListCatalog(context.Background(), CatalogQuery{Kind: "UNKNOWN"})
	if !IsCode(err, InvalidArgument) {
		t.Fatalf("expected INVALID_ARGUMENT for unknown kind, got %v", err)
	}
	if fake.listCalls != 0 {
		t.Fatalf("expected no capability call")
	}
}

func TestReader_ListCatalogValidatesScope(t *testing.T) {
	fake := &readerFakeCapabilities{}
	r, _ := NewReadOnly(fake)
	_, err := r.ListCatalog(context.Background(), CatalogQuery{Kind: KindClass, Scope: "WEIRD"})
	if !IsCode(err, InvalidArgument) {
		t.Fatalf("expected INVALID_ARGUMENT for invalid scope, got %v", err)
	}
}

func TestReader_ListCatalogCopiesResult(t *testing.T) {
	shared := []CatalogRecord{{Kind: KindClass, ID: 1}}
	fake := &readerFakeCapabilities{listPage: CatalogPage{Records: shared}}
	r, _ := NewReadOnly(fake)
	got, err := r.ListCatalog(context.Background(), CatalogQuery{Kind: KindClass})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got.Records[0].ID = 99
	if shared[0].ID != 1 {
		t.Fatalf("capability state leaked after caller mutation")
	}
}

func TestReader_GetCatalogValidatesKey(t *testing.T) {
	fake := &readerFakeCapabilities{}
	r, _ := NewReadOnly(fake)
	_, err := r.GetCatalog(context.Background(), CatalogKey{Kind: "UNKNOWN", ID: 1})
	if !IsCode(err, InvalidArgument) {
		t.Fatalf("expected INVALID_ARGUMENT for unknown kind, got %v", err)
	}
	_, err = r.GetCatalog(context.Background(), CatalogKey{Kind: KindClass, ID: 0})
	if !IsCode(err, InvalidArgument) {
		t.Fatalf("expected INVALID_ARGUMENT for zero id, got %v", err)
	}
}

func TestReader_GetCatalogCopiesResult(t *testing.T) {
	shared := CatalogRecord{Kind: KindClass, ID: 1, Values: map[string]Value{"code": {Kind: ValueCode, Text: "C"}}}
	fake := &readerFakeCapabilities{getRecord: shared}
	r, _ := NewReadOnly(fake)
	got, err := r.GetCatalog(context.Background(), CatalogKey{Kind: KindClass, ID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got.Values["code"] = Value{Kind: ValueCode, Text: "Z"}
	if shared.Values["code"].Text != "C" {
		t.Fatalf("capability state leaked after caller mutation")
	}
}

func TestReader_SearchResourcesValidatesScope(t *testing.T) {
	fake := &readerFakeCapabilities{}
	r, _ := NewReadOnly(fake)
	_, err := r.SearchResources(context.Background(), ResourceQuery{Scope: "WEIRD"})
	if !IsCode(err, InvalidArgument) {
		t.Fatalf("expected INVALID_ARGUMENT for invalid scope, got %v", err)
	}
}

func TestReader_SearchResourcesCopiesResult(t *testing.T) {
	shared := []Resource{{ID: 1, Attributes: []AttributeValue{{Code: "a", Value: Value{Kind: ValueInteger, Text: "1"}}}}}
	fake := &readerFakeCapabilities{searchPage: ResourcePage{Resources: shared}}
	r, _ := NewReadOnly(fake)
	got, err := r.SearchResources(context.Background(), ResourceQuery{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got.Resources[0].Attributes[0].Value.Text = "99"
	if shared[0].Attributes[0].Value.Text != "1" {
		t.Fatalf("capability state leaked after caller mutation")
	}
}

func TestReader_GetResourceValidatesKey(t *testing.T) {
	fake := &readerFakeCapabilities{}
	r, _ := NewReadOnly(fake)
	_, err := r.GetResource(context.Background(), ResourceKey{ClassCode: "", IdentityV1: "v1|x"})
	if !IsCode(err, InvalidArgument) {
		t.Fatalf("expected INVALID_ARGUMENT for empty class, got %v", err)
	}
	_, err = r.GetResource(context.Background(), ResourceKey{ClassCode: "C", IdentityV1: ""})
	if !IsCode(err, InvalidArgument) {
		t.Fatalf("expected INVALID_ARGUMENT for empty identity, got %v", err)
	}
}

func TestReader_GetResourceCopiesResult(t *testing.T) {
	shared := Resource{ID: 1, Scope: ResourceScope{ClassCode: "C"}}
	fake := &readerFakeCapabilities{getResource: shared}
	r, _ := NewReadOnly(fake)
	got, err := r.GetResource(context.Background(), ResourceKey{ClassCode: "C", IdentityV1: "v1|x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got.Scope.ClassCode = "Z"
	if shared.Scope.ClassCode != "C" {
		t.Fatalf("capability state leaked after caller mutation")
	}
}

func TestReader_DescribeResourceValidatesKey(t *testing.T) {
	fake := &readerFakeCapabilities{}
	r, _ := NewReadOnly(fake)
	_, err := r.DescribeResource(context.Background(), ResourceKey{ClassCode: "", IdentityV1: "v1|x"})
	if !IsCode(err, InvalidArgument) {
		t.Fatalf("expected INVALID_ARGUMENT, got %v", err)
	}
}

func TestReader_DescribeResourcePassesThrough(t *testing.T) {
	fake := &readerFakeCapabilities{describeText: "the canonical text"}
	r, _ := NewReadOnly(fake)
	got, err := r.DescribeResource(context.Background(), ResourceKey{ClassCode: "C", IdentityV1: "v1|x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "the canonical text" {
		t.Fatalf("unexpected description: %q", got)
	}
}

func TestReader_EffectiveAttributesForValidatesScope(t *testing.T) {
	fake := &readerFakeCapabilities{}
	r, _ := NewReadOnly(fake)
	_, err := r.EffectiveAttributesFor(context.Background(), ResourceScope{ClassCode: "", FamilyCode: "F", TypeCode: "T"})
	if !IsCode(err, InvalidArgument) {
		t.Fatalf("expected INVALID_ARGUMENT for empty class, got %v", err)
	}
	_, err = r.EffectiveAttributesFor(context.Background(), ResourceScope{ClassCode: "C", FamilyCode: "", TypeCode: "T"})
	if !IsCode(err, InvalidArgument) {
		t.Fatalf("expected INVALID_ARGUMENT for empty family, got %v", err)
	}
	_, err = r.EffectiveAttributesFor(context.Background(), ResourceScope{ClassCode: "C", FamilyCode: "F", TypeCode: ""})
	if !IsCode(err, InvalidArgument) {
		t.Fatalf("expected INVALID_ARGUMENT for empty type, got %v", err)
	}
}

func TestReader_EffectiveAttributesForCopiesResult(t *testing.T) {
	shared := []EffectiveAttribute{{
		Characteristic: CharacteristicDescriptor{Code: "color"},
		Rules:          []ApplicabilityRule{{AttributeCode: "insulation", Equals: Value{Kind: ValueText, Text: "DESNUDO"}}},
	}}
	fake := &readerFakeCapabilities{effectiveAttributes: shared}
	r, _ := NewReadOnly(fake)
	got, err := r.EffectiveAttributesFor(context.Background(), ResourceScope{ClassCode: "C", FamilyCode: "F", TypeCode: "T"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got[0].Rules[0].Equals.Text = "Z"
	if shared[0].Rules[0].Equals.Text != "DESNUDO" {
		t.Fatalf("capability state leaked after caller mutation")
	}
}

func TestReader_EvaluateAttributesValidatesScope(t *testing.T) {
	fake := &readerFakeCapabilities{}
	r, _ := NewReadOnly(fake)
	_, err := r.EvaluateAttributes(context.Background(), ResourceScope{ClassCode: "", FamilyCode: "F", TypeCode: "T"}, nil)
	if !IsCode(err, InvalidArgument) {
		t.Fatalf("expected INVALID_ARGUMENT for empty class, got %v", err)
	}
}

func TestReader_EvaluateAttributesClonesValuesAndResult(t *testing.T) {
	sharedValues := []AttributeValue{{Code: "insulation", Value: Value{Kind: ValueControlledOption, Text: "DESNUDO"}}}
	sharedResult := []EffectiveAttribute{{Characteristic: CharacteristicDescriptor{Code: "color"}}}
	fake := &readerFakeCapabilities{effectiveAttributes: sharedResult}
	r, _ := NewReadOnly(fake)

	got, err := r.EvaluateAttributes(context.Background(), ResourceScope{ClassCode: "C", FamilyCode: "F", TypeCode: "T"}, sharedValues)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fake.evaluateAttrsValues) != 1 || fake.evaluateAttrsValues[0].Value.Text != "DESNUDO" {
		t.Fatalf("capability did not receive forwarded values: %+v", fake.evaluateAttrsValues)
	}

	fake.evaluateAttrsValues[0].Value.Text = "MUTATED"
	if sharedValues[0].Value.Text != "DESNUDO" {
		t.Fatalf("caller's values slice leaked into capability's copy")
	}

	got[0].Characteristic.Code = "mutated"
	if sharedResult[0].Characteristic.Code != "color" {
		t.Fatalf("capability state leaked after caller mutation")
	}
}

func TestReader_PropagatesCapabilityErrors(t *testing.T) {
	fake := &readerFakeCapabilities{activeClassesErr: errors.New("boom")}
	r, _ := NewReadOnly(fake)
	_, err := r.ActiveClasses(context.Background())
	if err == nil {
		t.Fatalf("expected error")
	}
}
