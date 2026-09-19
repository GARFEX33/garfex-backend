package domain

import (
	"context"
	"errors"
	"testing"
)

func TestAttributeOrderReadResultDetached(t *testing.T) {
	catalog := ResourceCatalog{
		Classes:    []ResourceClass{{Aliases: []string{"alias"}, Keywords: []string{"keyword"}}},
		Attributes: []ResourceAttribute{{Rules: []AttributeRule{{When: AttributeCondition{AttributeCode: "control"}}}}},
	}
	order := AttributeOrderSnapshot{OrderedAttributes: []AttributeOrderKey{{CharacteristicCode: "size"}}}
	result := NewAttributeOrderReadResult(catalog, order)
	result.Catalog.Classes[0].Aliases[0] = "result alias"
	result.Catalog.Classes[0].Keywords[0] = "result keyword"
	result.Catalog.Attributes[0].Rules[0].When.AttributeCode = "result rule"
	result.Order.OrderedAttributes[0].CharacteristicCode = "result key"
	if catalog.Classes[0].Aliases[0] != "alias" || catalog.Classes[0].Keywords[0] != "keyword" {
		t.Fatal("nested class slices alias result")
	}
	if catalog.Attributes[0].Rules[0].When.AttributeCode != "control" || order.OrderedAttributes[0].CharacteristicCode != "size" {
		t.Fatal("nested rules or order keys alias result")
	}
	catalog.Attributes[0].Rules[0].When.AttributeCode = "input rule"
	order.OrderedAttributes[0].CharacteristicCode = "input key"
	if result.Catalog.Attributes[0].Rules[0].When.AttributeCode != "result rule" || result.Order.OrderedAttributes[0].CharacteristicCode != "result key" {
		t.Fatal("input mutation crossed result boundary")
	}
	empty := NewAttributeOrderReadResult(ResourceCatalog{}, AttributeOrderSnapshot{})
	if empty.Order.OrderedAttributes == nil {
		t.Fatal("empty order must be nonnil")
	}
}

// stubAttributeOrderWriter is a compile-time contract check: any
// AttributeOrderWriter (and, combined with the existing AttributeOrderReader,
// any AttributeOrderStore) must accept AttributeOrderWriteRequest and return
// AttributeOrderReadResult, mirroring the reader's own result shape so a
// caller works with either capability identically.
type stubAttributeOrderWriter struct{}

func (stubAttributeOrderWriter) WriteAttributeOrder(context.Context, AttributeOrderWriteRequest) (AttributeOrderReadResult, error) {
	return AttributeOrderReadResult{}, nil
}

func (stubAttributeOrderWriter) ReadAttributeOrder(context.Context, ResourceScope) (AttributeOrderReadResult, error) {
	return AttributeOrderReadResult{}, nil
}

var _ AttributeOrderWriter = stubAttributeOrderWriter{}
var _ AttributeOrderStore = stubAttributeOrderWriter{}

func TestAttributeOrderWriteSentinelsAreDistinct(t *testing.T) {
	for _, tt := range []struct {
		name       string
		a, b       error
		wantEquals bool
	}{
		{name: "conflict is not validation", a: ErrAttributeOrderRevisionConflict, b: ErrResourceValidation},
		{name: "conflict is not not-found", a: ErrAttributeOrderRevisionConflict, b: ErrCatalogRecordNotFound},
		{name: "unavailable is not conflict", a: ErrAttributeOrderUnavailable, b: ErrAttributeOrderRevisionConflict},
		{name: "unavailable is not validation", a: ErrAttributeOrderUnavailable, b: ErrResourceValidation},
		{name: "conflict identifies itself", a: ErrAttributeOrderRevisionConflict, b: ErrAttributeOrderRevisionConflict, wantEquals: true},
		{name: "unavailable identifies itself", a: ErrAttributeOrderUnavailable, b: ErrAttributeOrderUnavailable, wantEquals: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if errors.Is(tt.a, tt.b) != tt.wantEquals {
				t.Fatalf("errors.Is(%v, %v) = %v, want %v", tt.a, tt.b, errors.Is(tt.a, tt.b), tt.wantEquals)
			}
		})
	}
}

func TestAttributeOrderWriteRequestFields(t *testing.T) {
	scope := ResourceScope{ClassCode: "MAT", FamilyCode: "F", TypeCode: "T"}
	key := AttributeOrderKey{SourceLevel: SourceLevelType, SourceCode: "T", CharacteristicCode: "size"}
	req := AttributeOrderWriteRequest{Scope: scope, ExpectedOrderRevision: "v1:abc", OrderedAttributes: []AttributeOrderKey{key}}
	if req.Scope != scope || req.ExpectedOrderRevision != "v1:abc" || len(req.OrderedAttributes) != 1 || req.OrderedAttributes[0] != key {
		t.Fatalf("request = %+v", req)
	}
}
