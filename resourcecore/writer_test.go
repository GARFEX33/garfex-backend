package resourcecore

import (
	"context"
	"reflect"
	"testing"
)

type fakeWriteCapabilities struct {
	createCatalog        func(ctx context.Context, req CatalogWriteRequest) (CatalogRecord, error)
	createResource       func(ctx context.Context, req ResourceWriteRequest) (Resource, error)
	updateCatalog        func(ctx context.Context, req CatalogUpdateRequest) (CatalogRecord, error)
	updateResource       func(ctx context.Context, req ResourceUpdateRequest) (Resource, error)
	deactivateCatalog    func(ctx context.Context, req CatalogLifecycleRequest) (CatalogRecord, error)
	reactivateCatalog    func(ctx context.Context, req CatalogLifecycleRequest) (CatalogRecord, error)
	deactivateResource   func(ctx context.Context, req ResourceLifecycleRequest) (Resource, error)
	reactivateResource   func(ctx context.Context, req ResourceLifecycleRequest) (Resource, error)
	hardDeleteCatalog    func(ctx context.Context, req CatalogLifecycleRequest) error
	updateAttributeOrder func(ctx context.Context, req AttributeOrderWriteRequest) (ResourceAttributeOrder, error)
}

func (f *fakeWriteCapabilities) CreateCatalog(ctx context.Context, req CatalogWriteRequest) (CatalogRecord, error) {
	return f.createCatalog(ctx, req)
}

func (f *fakeWriteCapabilities) CreateResource(ctx context.Context, req ResourceWriteRequest) (Resource, error) {
	return f.createResource(ctx, req)
}

func (f *fakeWriteCapabilities) UpdateCatalog(ctx context.Context, req CatalogUpdateRequest) (CatalogRecord, error) {
	return f.updateCatalog(ctx, req)
}

func (f *fakeWriteCapabilities) UpdateResource(ctx context.Context, req ResourceUpdateRequest) (Resource, error) {
	return f.updateResource(ctx, req)
}

func (f *fakeWriteCapabilities) DeactivateCatalog(ctx context.Context, req CatalogLifecycleRequest) (CatalogRecord, error) {
	return f.deactivateCatalog(ctx, req)
}

func (f *fakeWriteCapabilities) ReactivateCatalog(ctx context.Context, req CatalogLifecycleRequest) (CatalogRecord, error) {
	return f.reactivateCatalog(ctx, req)
}

func (f *fakeWriteCapabilities) DeactivateResource(ctx context.Context, req ResourceLifecycleRequest) (Resource, error) {
	return f.deactivateResource(ctx, req)
}

func (f *fakeWriteCapabilities) ReactivateResource(ctx context.Context, req ResourceLifecycleRequest) (Resource, error) {
	return f.reactivateResource(ctx, req)
}

func (f *fakeWriteCapabilities) HardDeleteCatalog(ctx context.Context, req CatalogLifecycleRequest) error {
	return f.hardDeleteCatalog(ctx, req)
}

func (f *fakeWriteCapabilities) UpdateAttributeOrder(ctx context.Context, req AttributeOrderWriteRequest) (ResourceAttributeOrder, error) {
	return f.updateAttributeOrder(ctx, req)
}

func validCatalogWriteRequest() CatalogWriteRequest {
	return CatalogWriteRequest{
		Actor: "PI",
		Kind:  KindClass,
		Values: map[string]Value{
			"code": {Kind: ValueCode, Text: "MAT"},
		},
	}
}

func validResourceWriteRequest() ResourceWriteRequest {
	return ResourceWriteRequest{
		Actor:       "PI",
		Scope:       ResourceScope{ClassCode: "MAT", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"},
		NaturalUnit: "m",
	}
}

func validCatalogUpdateRequest() CatalogUpdateRequest {
	return CatalogUpdateRequest{
		Actor:            "PI",
		Kind:             KindClass,
		ID:               1,
		ExpectedRevision: 1,
		Values: map[string]Value{
			"code": {Kind: ValueCode, Text: "MAT"},
		},
	}
}

func validResourceUpdateRequest() ResourceUpdateRequest {
	return ResourceUpdateRequest{
		Actor:            "PI",
		ID:               1,
		ExpectedRevision: 1,
		Scope:            ResourceScope{ClassCode: "MAT", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"},
		NaturalUnit:      "m",
	}
}

func validAttributeOrderWriteRequest() AttributeOrderWriteRequest {
	return AttributeOrderWriteRequest{
		Actor:                 "PI",
		Scope:                 ResourceScope{ClassCode: "MAT", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"},
		ExpectedOrderRevision: "v1:abc",
		OrderedAttributes:     []AttributeOrderKey{{SourceLevel: "TYPE", SourceCode: "CABLE", CharacteristicCode: "color"}},
	}
}

func TestWriter_NewWriter_NilCapability_InvalidArgument(t *testing.T) {
	w, err := NewWriter(nil)
	if w != nil {
		t.Fatalf("expected nil Writer, got %+v", w)
	}
	if !IsCode(err, InvalidArgument) {
		t.Fatalf("expected INVALID_ARGUMENT, got %v", err)
	}
}

func TestWriter_CreateCatalog_ShapeValidation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*CatalogWriteRequest)
		wantErr bool
	}{
		{"valid", func(r *CatalogWriteRequest) {}, false},
		{"blank actor", func(r *CatalogWriteRequest) { r.Actor = "  " }, true},
		{"unknown kind", func(r *CatalogWriteRequest) { r.Kind = "NOPE" }, true},
		{"nil values", func(r *CatalogWriteRequest) { r.Values = nil }, true},
		{"empty values", func(r *CatalogWriteRequest) { r.Values = map[string]Value{} }, true},
		{"unknown value kind", func(r *CatalogWriteRequest) { r.Values["code"] = Value{Kind: "BOGUS", Text: "x"} }, true},
		{"non-zero reference id", func(r *CatalogWriteRequest) {
			r.Values["ref"] = Value{Kind: ValueReference, Reference: &Reference{Kind: KindFamily, ID: 5, Code: "F"}}
		}, true},
		{"zero reference id ok", func(r *CatalogWriteRequest) {
			r.Values["ref"] = Value{Kind: ValueReference, Reference: &Reference{Kind: KindFamily, Code: "F"}}
		}, false},
		{"unit code on non-quantity", func(r *CatalogWriteRequest) { r.Values["code"] = Value{Kind: ValueCode, Text: "MAT", UnitCode: "m"} }, true},
		{"missing unit code on quantity", func(r *CatalogWriteRequest) { r.Values["q"] = Value{Kind: ValueQuantity, Text: "1"} }, true},
		{"quantity with unit code ok", func(r *CatalogWriteRequest) { r.Values["q"] = Value{Kind: ValueQuantity, Text: "1", UnitCode: "m"} }, false},
		{"invalid integer text", func(r *CatalogWriteRequest) { r.Values["n"] = Value{Kind: ValueInteger, Text: "not-a-number"} }, true},
		{"invalid decimal text", func(r *CatalogWriteRequest) { r.Values["d"] = Value{Kind: ValueDecimal, Text: "not-a-number"} }, true},
		{"applicability equals not text", func(r *CatalogWriteRequest) {
			r.Rules = []ApplicabilityRule{{AttributeCode: "insulation", Equals: Value{Kind: ValueBool, Bool: true}, Mode: "FORBIDDEN"}}
		}, true},
		{"applicability equals text ok", func(r *CatalogWriteRequest) {
			r.Rules = []ApplicabilityRule{{AttributeCode: "insulation", Equals: Value{Kind: ValueText, Text: "DESNUDO"}, Mode: "FORBIDDEN"}}
		}, false},
		{"applicability unknown mode", func(r *CatalogWriteRequest) {
			r.Rules = []ApplicabilityRule{{AttributeCode: "insulation", Equals: Value{Kind: ValueText, Text: "DESNUDO"}, Mode: "BOGUS"}}
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validCatalogWriteRequest()
			tt.mutate(&req)
			cap := &fakeWriteCapabilities{
				createCatalog: func(ctx context.Context, req CatalogWriteRequest) (CatalogRecord, error) {
					return CatalogRecord{Kind: req.Kind, ID: 1, Revision: 1}, nil
				},
			}
			w, err := NewWriter(cap)
			if err != nil {
				t.Fatalf("unexpected NewWriter error: %v", err)
			}
			_, err = w.CreateCatalog(context.Background(), req)
			if tt.wantErr {
				if !IsCode(err, InvalidArgument) {
					t.Fatalf("expected INVALID_ARGUMENT, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestWriter_CreateResource_ShapeValidation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*ResourceWriteRequest)
		wantErr bool
	}{
		{"valid", func(r *ResourceWriteRequest) {}, false},
		{"blank actor", func(r *ResourceWriteRequest) { r.Actor = "" }, true},
		{"blank class code", func(r *ResourceWriteRequest) { r.Scope.ClassCode = "" }, true},
		{"blank family code", func(r *ResourceWriteRequest) { r.Scope.FamilyCode = "" }, true},
		{"blank type code", func(r *ResourceWriteRequest) { r.Scope.TypeCode = "" }, true},
		{"blank natural unit", func(r *ResourceWriteRequest) { r.NaturalUnit = "" }, true},
		{"attribute value unit code on non-quantity", func(r *ResourceWriteRequest) {
			r.Attributes = []AttributeValue{{Code: "gauge", Value: Value{Kind: ValueText, Text: "x", UnitCode: "m"}}}
		}, true},
		{"attribute value quantity missing unit code", func(r *ResourceWriteRequest) {
			r.Attributes = []AttributeValue{{Code: "length", Value: Value{Kind: ValueQuantity, Text: "1"}}}
		}, true},
		{"attribute value quantity ok", func(r *ResourceWriteRequest) {
			r.Attributes = []AttributeValue{{Code: "length", Value: Value{Kind: ValueQuantity, Text: "1", UnitCode: "m"}}}
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validResourceWriteRequest()
			tt.mutate(&req)
			cap := &fakeWriteCapabilities{
				createResource: func(ctx context.Context, req ResourceWriteRequest) (Resource, error) {
					return Resource{ID: 1, Scope: req.Scope, Revision: 1}, nil
				},
			}
			w, err := NewWriter(cap)
			if err != nil {
				t.Fatalf("unexpected NewWriter error: %v", err)
			}
			_, err = w.CreateResource(context.Background(), req)
			if tt.wantErr {
				if !IsCode(err, InvalidArgument) {
					t.Fatalf("expected INVALID_ARGUMENT, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestWriter_NoUngraduatedMethodExported(t *testing.T) {
	typ := reflect.TypeOf((*WriteCapabilities)(nil)).Elem()
	if typ.NumMethod() != 10 {
		t.Fatalf("expected exactly 10 methods on WriteCapabilities, got %d: %v", typ.NumMethod(), typ)
	}
	want := []string{"CreateCatalog", "CreateResource", "UpdateCatalog", "UpdateResource", "DeactivateCatalog", "ReactivateCatalog", "DeactivateResource", "ReactivateResource", "HardDeleteCatalog", "UpdateAttributeOrder"}
	for _, name := range want {
		if _, ok := typ.MethodByName(name); !ok {
			t.Fatalf("expected WriteCapabilities to declare %s", name)
		}
	}
	writerType := reflect.TypeOf((*Writer)(nil))
	allowed := map[string]bool{}
	for _, name := range want {
		allowed[name] = true
	}
	for i := 0; i < writerType.NumMethod(); i++ {
		name := writerType.Method(i).Name
		if !allowed[name] {
			t.Fatalf("unexpected exported Writer method %s; only Create/Update/Deactivate/Reactivate/HardDelete Catalog/Resource and UpdateAttributeOrder may be graduated so far; no HardDeleteResource stub allowed", name)
		}
	}
}

func validCatalogLifecycleRequest() CatalogLifecycleRequest {
	return CatalogLifecycleRequest{Actor: "PI", Kind: KindClass, ID: 1, ExpectedRevision: 1}
}

func validResourceLifecycleRequest() ResourceLifecycleRequest {
	return ResourceLifecycleRequest{Actor: "PI", ID: 1, ExpectedRevision: 1}
}

func TestWriter_LifecycleRequest_NoContentField(t *testing.T) {
	forbidden := map[string]bool{"Values": true, "Rules": true, "Scope": true, "Attributes": true}
	for _, typ := range []reflect.Type{reflect.TypeOf(CatalogLifecycleRequest{}), reflect.TypeOf(ResourceLifecycleRequest{})} {
		for i := 0; i < typ.NumField(); i++ {
			name := typ.Field(i).Name
			if forbidden[name] {
				t.Fatalf("%s must not declare content field %s", typ.Name(), name)
			}
		}
	}
}

func TestWriter_DeactivateCatalog_ShapeValidation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*CatalogLifecycleRequest)
		wantErr bool
	}{
		{"valid", func(r *CatalogLifecycleRequest) {}, false},
		{"blank actor", func(r *CatalogLifecycleRequest) { r.Actor = " " }, true},
		{"zero id", func(r *CatalogLifecycleRequest) { r.ID = 0 }, true},
		{"zero expected revision", func(r *CatalogLifecycleRequest) { r.ExpectedRevision = 0 }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validCatalogLifecycleRequest()
			tt.mutate(&req)
			cap := &fakeWriteCapabilities{
				deactivateCatalog: func(ctx context.Context, req CatalogLifecycleRequest) (CatalogRecord, error) {
					return CatalogRecord{Kind: req.Kind, ID: req.ID, Revision: req.ExpectedRevision + 1, Active: false}, nil
				},
			}
			w, err := NewWriter(cap)
			if err != nil {
				t.Fatalf("unexpected NewWriter error: %v", err)
			}
			_, err = w.DeactivateCatalog(context.Background(), req)
			if tt.wantErr {
				if !IsCode(err, InvalidArgument) {
					t.Fatalf("expected INVALID_ARGUMENT, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestWriter_HardDeleteCatalog_ShapeValidation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*CatalogLifecycleRequest)
		wantErr bool
	}{
		{"valid", func(r *CatalogLifecycleRequest) {}, false},
		{"blank actor", func(r *CatalogLifecycleRequest) { r.Actor = " " }, true},
		{"zero id", func(r *CatalogLifecycleRequest) { r.ID = 0 }, true},
		{"zero expected revision", func(r *CatalogLifecycleRequest) { r.ExpectedRevision = 0 }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validCatalogLifecycleRequest()
			tt.mutate(&req)
			cap := &fakeWriteCapabilities{
				hardDeleteCatalog: func(ctx context.Context, req CatalogLifecycleRequest) error {
					return nil
				},
			}
			w, err := NewWriter(cap)
			if err != nil {
				t.Fatalf("unexpected NewWriter error: %v", err)
			}
			err = w.HardDeleteCatalog(context.Background(), req)
			if tt.wantErr {
				if !IsCode(err, InvalidArgument) {
					t.Fatalf("expected INVALID_ARGUMENT, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestWriter_HardDeleteCatalog_ShapeValidation_NoCapabilityCallOnInvalid(t *testing.T) {
	called := false
	cap := &fakeWriteCapabilities{
		hardDeleteCatalog: func(ctx context.Context, req CatalogLifecycleRequest) error {
			called = true
			return nil
		},
	}
	w, err := NewWriter(cap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	req := validCatalogLifecycleRequest()
	req.ID = 0
	if err := w.HardDeleteCatalog(context.Background(), req); !IsCode(err, InvalidArgument) {
		t.Fatalf("expected INVALID_ARGUMENT, got %v", err)
	}
	if called {
		t.Fatalf("expected the capability to not be called for a shape-invalid request")
	}
}

func TestWriter_ReactivateCatalog_ShapeValidation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*CatalogLifecycleRequest)
		wantErr bool
	}{
		{"valid", func(r *CatalogLifecycleRequest) {}, false},
		{"blank actor", func(r *CatalogLifecycleRequest) { r.Actor = "" }, true},
		{"zero id", func(r *CatalogLifecycleRequest) { r.ID = 0 }, true},
		{"zero expected revision", func(r *CatalogLifecycleRequest) { r.ExpectedRevision = 0 }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validCatalogLifecycleRequest()
			tt.mutate(&req)
			cap := &fakeWriteCapabilities{
				reactivateCatalog: func(ctx context.Context, req CatalogLifecycleRequest) (CatalogRecord, error) {
					return CatalogRecord{Kind: req.Kind, ID: req.ID, Revision: req.ExpectedRevision + 1, Active: true}, nil
				},
			}
			w, err := NewWriter(cap)
			if err != nil {
				t.Fatalf("unexpected NewWriter error: %v", err)
			}
			_, err = w.ReactivateCatalog(context.Background(), req)
			if tt.wantErr {
				if !IsCode(err, InvalidArgument) {
					t.Fatalf("expected INVALID_ARGUMENT, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestWriter_DeactivateResource_ShapeValidation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*ResourceLifecycleRequest)
		wantErr bool
	}{
		{"valid", func(r *ResourceLifecycleRequest) {}, false},
		{"blank actor", func(r *ResourceLifecycleRequest) { r.Actor = "" }, true},
		{"zero id", func(r *ResourceLifecycleRequest) { r.ID = 0 }, true},
		{"zero expected revision", func(r *ResourceLifecycleRequest) { r.ExpectedRevision = 0 }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validResourceLifecycleRequest()
			tt.mutate(&req)
			cap := &fakeWriteCapabilities{
				deactivateResource: func(ctx context.Context, req ResourceLifecycleRequest) (Resource, error) {
					return Resource{ID: req.ID, Revision: req.ExpectedRevision + 1, Active: false}, nil
				},
			}
			w, err := NewWriter(cap)
			if err != nil {
				t.Fatalf("unexpected NewWriter error: %v", err)
			}
			_, err = w.DeactivateResource(context.Background(), req)
			if tt.wantErr {
				if !IsCode(err, InvalidArgument) {
					t.Fatalf("expected INVALID_ARGUMENT, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestWriter_ReactivateResource_ShapeValidation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*ResourceLifecycleRequest)
		wantErr bool
	}{
		{"valid", func(r *ResourceLifecycleRequest) {}, false},
		{"blank actor", func(r *ResourceLifecycleRequest) { r.Actor = "" }, true},
		{"zero id", func(r *ResourceLifecycleRequest) { r.ID = 0 }, true},
		{"zero expected revision", func(r *ResourceLifecycleRequest) { r.ExpectedRevision = 0 }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validResourceLifecycleRequest()
			tt.mutate(&req)
			cap := &fakeWriteCapabilities{
				reactivateResource: func(ctx context.Context, req ResourceLifecycleRequest) (Resource, error) {
					return Resource{ID: req.ID, Revision: req.ExpectedRevision + 1, Active: true}, nil
				},
			}
			w, err := NewWriter(cap)
			if err != nil {
				t.Fatalf("unexpected NewWriter error: %v", err)
			}
			_, err = w.ReactivateResource(context.Background(), req)
			if tt.wantErr {
				if !IsCode(err, InvalidArgument) {
					t.Fatalf("expected INVALID_ARGUMENT, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestWriter_UpdateCatalog_ShapeValidation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*CatalogUpdateRequest)
		wantErr bool
	}{
		{"valid", func(r *CatalogUpdateRequest) {}, false},
		{"blank actor", func(r *CatalogUpdateRequest) { r.Actor = " " }, true},
		{"zero id", func(r *CatalogUpdateRequest) { r.ID = 0 }, true},
		{"zero expected revision", func(r *CatalogUpdateRequest) { r.ExpectedRevision = 0 }, true},
		{"unknown kind", func(r *CatalogUpdateRequest) { r.Kind = "NOPE" }, true},
		{"empty values", func(r *CatalogUpdateRequest) { r.Values = map[string]Value{} }, true},
		{"unknown value kind", func(r *CatalogUpdateRequest) { r.Values["code"] = Value{Kind: "BOGUS", Text: "x"} }, true},
		{"applicability equals not text", func(r *CatalogUpdateRequest) {
			r.Rules = []ApplicabilityRule{{AttributeCode: "insulation", Equals: Value{Kind: ValueBool, Bool: true}, Mode: "FORBIDDEN"}}
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validCatalogUpdateRequest()
			tt.mutate(&req)
			cap := &fakeWriteCapabilities{
				updateCatalog: func(ctx context.Context, req CatalogUpdateRequest) (CatalogRecord, error) {
					return CatalogRecord{Kind: req.Kind, ID: req.ID, Revision: req.ExpectedRevision + 1}, nil
				},
			}
			w, err := NewWriter(cap)
			if err != nil {
				t.Fatalf("unexpected NewWriter error: %v", err)
			}
			_, err = w.UpdateCatalog(context.Background(), req)
			if tt.wantErr {
				if !IsCode(err, InvalidArgument) {
					t.Fatalf("expected INVALID_ARGUMENT, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestWriter_UpdateResource_ShapeValidation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*ResourceUpdateRequest)
		wantErr bool
	}{
		{"valid", func(r *ResourceUpdateRequest) {}, false},
		{"blank actor", func(r *ResourceUpdateRequest) { r.Actor = "" }, true},
		{"zero id", func(r *ResourceUpdateRequest) { r.ID = 0 }, true},
		{"zero expected revision", func(r *ResourceUpdateRequest) { r.ExpectedRevision = 0 }, true},
		{"blank class code", func(r *ResourceUpdateRequest) { r.Scope.ClassCode = "" }, true},
		{"blank family code", func(r *ResourceUpdateRequest) { r.Scope.FamilyCode = "" }, true},
		{"blank type code", func(r *ResourceUpdateRequest) { r.Scope.TypeCode = "" }, true},
		{"blank natural unit", func(r *ResourceUpdateRequest) { r.NaturalUnit = "" }, true},
		{"attribute value quantity missing unit code", func(r *ResourceUpdateRequest) {
			r.Attributes = []AttributeValue{{Code: "length", Value: Value{Kind: ValueQuantity, Text: "1"}}}
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validResourceUpdateRequest()
			tt.mutate(&req)
			cap := &fakeWriteCapabilities{
				updateResource: func(ctx context.Context, req ResourceUpdateRequest) (Resource, error) {
					return Resource{ID: req.ID, Scope: req.Scope, Revision: req.ExpectedRevision + 1}, nil
				},
			}
			w, err := NewWriter(cap)
			if err != nil {
				t.Fatalf("unexpected NewWriter error: %v", err)
			}
			_, err = w.UpdateResource(context.Background(), req)
			if tt.wantErr {
				if !IsCode(err, InvalidArgument) {
					t.Fatalf("expected INVALID_ARGUMENT, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestWriter_UpdateCatalog_RevisionIncreasesOverExpected(t *testing.T) {
	cap := &fakeWriteCapabilities{
		updateCatalog: func(ctx context.Context, req CatalogUpdateRequest) (CatalogRecord, error) {
			return CatalogRecord{Kind: req.Kind, ID: req.ID, Revision: req.ExpectedRevision + 1, Values: req.Values}, nil
		},
	}
	w, err := NewWriter(cap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	req := validCatalogUpdateRequest()
	rec, err := w.UpdateCatalog(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Revision <= req.ExpectedRevision {
		t.Fatalf("expected returned revision > supplied ExpectedRevision, got %d vs %d", rec.Revision, req.ExpectedRevision)
	}
}

func TestWriter_UpdateResource_RevisionIncreasesOverExpected(t *testing.T) {
	cap := &fakeWriteCapabilities{
		updateResource: func(ctx context.Context, req ResourceUpdateRequest) (Resource, error) {
			return Resource{ID: req.ID, Scope: req.Scope, Revision: req.ExpectedRevision + 1, Attributes: req.Attributes}, nil
		},
	}
	w, err := NewWriter(cap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	req := validResourceUpdateRequest()
	res, err := w.UpdateResource(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Revision <= req.ExpectedRevision {
		t.Fatalf("expected returned revision > supplied ExpectedRevision, got %d vs %d", res.Revision, req.ExpectedRevision)
	}
}

func TestWriter_UpdateAttributeOrder_ShapeValidation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*AttributeOrderWriteRequest)
		wantErr bool
	}{
		{"valid", func(r *AttributeOrderWriteRequest) {}, false},
		{"blank actor", func(r *AttributeOrderWriteRequest) { r.Actor = "" }, true},
		{"blank class code", func(r *AttributeOrderWriteRequest) { r.Scope.ClassCode = "" }, true},
		{"blank family code", func(r *AttributeOrderWriteRequest) { r.Scope.FamilyCode = "" }, true},
		{"blank type code", func(r *AttributeOrderWriteRequest) { r.Scope.TypeCode = "" }, true},
		{"blank expected order revision", func(r *AttributeOrderWriteRequest) { r.ExpectedOrderRevision = "" }, true},
		{"empty ordered attributes", func(r *AttributeOrderWriteRequest) { r.OrderedAttributes = nil }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validAttributeOrderWriteRequest()
			tt.mutate(&req)
			cap := &fakeWriteCapabilities{
				updateAttributeOrder: func(ctx context.Context, req AttributeOrderWriteRequest) (ResourceAttributeOrder, error) {
					return ResourceAttributeOrder{Scope: req.Scope, OrderedAttributes: req.OrderedAttributes, OrderRevision: "v2:def"}, nil
				},
			}
			w, err := NewWriter(cap)
			if err != nil {
				t.Fatalf("unexpected NewWriter error: %v", err)
			}
			_, err = w.UpdateAttributeOrder(context.Background(), req)
			if tt.wantErr {
				if !IsCode(err, InvalidArgument) {
					t.Fatalf("expected INVALID_ARGUMENT, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestWriter_UpdateAttributeOrder_MapsResultAndClonesRequest(t *testing.T) {
	var captured AttributeOrderWriteRequest
	cap := &fakeWriteCapabilities{
		updateAttributeOrder: func(ctx context.Context, req AttributeOrderWriteRequest) (ResourceAttributeOrder, error) {
			captured = req
			return ResourceAttributeOrder{Scope: req.Scope, OrderedAttributes: req.OrderedAttributes, OrderRevision: "v2:def"}, nil
		},
	}
	w, err := NewWriter(cap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	req := validAttributeOrderWriteRequest()
	got, err := w.UpdateAttributeOrder(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.OrderRevision != "v2:def" || len(got.OrderedAttributes) != 1 {
		t.Fatalf("unexpected mapped order: %+v", got)
	}

	req.OrderedAttributes[0].CharacteristicCode = "MUTATED"
	got.OrderedAttributes[0].CharacteristicCode = "MUTATED"
	if captured.OrderedAttributes[0].CharacteristicCode != "color" {
		t.Fatalf("caller mutation after call leaked into the capability call: %+v", captured.OrderedAttributes[0])
	}
}

func TestWriteUpdateRequestCopy_CallerMutationAfterCall_NoEffect(t *testing.T) {
	var captured CatalogUpdateRequest
	cap := &fakeWriteCapabilities{
		updateCatalog: func(ctx context.Context, req CatalogUpdateRequest) (CatalogRecord, error) {
			captured = req
			return CatalogRecord{Kind: req.Kind, ID: req.ID, Revision: req.ExpectedRevision + 1, Values: req.Values}, nil
		},
	}
	w, err := NewWriter(cap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	req := validCatalogUpdateRequest()
	rec, err := w.UpdateCatalog(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req.Values["code"] = Value{Kind: ValueCode, Text: "MUTATED"}
	rec.Values["code"] = Value{Kind: ValueCode, Text: "MUTATED"}

	if captured.Values["code"].Text != "MAT" {
		t.Fatalf("caller mutation after call leaked into the capability call: %+v", captured.Values["code"])
	}
}

func TestWriteRequestCopy_CallerMutationAfterCall_NoEffect(t *testing.T) {
	var captured CatalogWriteRequest
	cap := &fakeWriteCapabilities{
		createCatalog: func(ctx context.Context, req CatalogWriteRequest) (CatalogRecord, error) {
			captured = req
			return CatalogRecord{Kind: req.Kind, ID: 1, Revision: 1, Values: req.Values}, nil
		},
	}
	w, err := NewWriter(cap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	req := validCatalogWriteRequest()
	rec, err := w.CreateCatalog(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req.Values["code"] = Value{Kind: ValueCode, Text: "MUTATED"}
	rec.Values["code"] = Value{Kind: ValueCode, Text: "MUTATED"}

	if captured.Values["code"].Text != "MAT" {
		t.Fatalf("caller mutation after call leaked into the capability call: %+v", captured.Values["code"])
	}
}
