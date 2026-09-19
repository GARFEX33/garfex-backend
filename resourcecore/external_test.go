package resourcecore_test

import (
	"context"
	"testing"

	"github.com/GARFEX33/garfex-costos-unitarios/resourcecore"
)

type externalFakeCapabilities struct {
	activeClasses []resourcecore.CatalogRecord
	descriptors   []resourcecore.CatalogDescriptor
}

func (f *externalFakeCapabilities) ActiveClasses(ctx context.Context) ([]resourcecore.CatalogRecord, error) {
	return f.activeClasses, nil
}

func (f *externalFakeCapabilities) CatalogDescriptors(ctx context.Context) ([]resourcecore.CatalogDescriptor, error) {
	return f.descriptors, nil
}

func (f *externalFakeCapabilities) ListCatalog(ctx context.Context, q resourcecore.CatalogQuery) (resourcecore.CatalogPage, error) {
	return resourcecore.CatalogPage{Query: q, Records: []resourcecore.CatalogRecord{{Kind: q.Kind, ID: 1}}}, nil
}

func (f *externalFakeCapabilities) GetCatalog(ctx context.Context, key resourcecore.CatalogKey) (resourcecore.CatalogRecord, error) {
	return resourcecore.CatalogRecord{Kind: key.Kind, ID: key.ID}, nil
}

func (f *externalFakeCapabilities) SearchResources(ctx context.Context, q resourcecore.ResourceQuery) (resourcecore.ResourcePage, error) {
	return resourcecore.ResourcePage{Query: q, Resources: []resourcecore.Resource{{ID: 1}}}, nil
}

func (f *externalFakeCapabilities) GetResource(ctx context.Context, key resourcecore.ResourceKey) (resourcecore.Resource, error) {
	return resourcecore.Resource{ID: 1, Scope: resourcecore.ResourceScope{ClassCode: key.ClassCode}}, nil
}

func (f *externalFakeCapabilities) DescribeResource(ctx context.Context, key resourcecore.ResourceKey) (string, error) {
	return "described", nil
}

func (f *externalFakeCapabilities) EffectiveAttributesFor(ctx context.Context, scope resourcecore.ResourceScope) ([]resourcecore.EffectiveAttribute, error) {
	return nil, nil
}

func (f *externalFakeCapabilities) EvaluateAttributes(ctx context.Context, scope resourcecore.ResourceScope, values []resourcecore.AttributeValue) ([]resourcecore.EffectiveAttribute, error) {
	return nil, nil
}

func (f *externalFakeCapabilities) AttributeOrderFor(ctx context.Context, scope resourcecore.ResourceScope) (resourcecore.ResourceAttributeOrder, error) {
	return resourcecore.ResourceAttributeOrder{Scope: scope}, nil
}

func TestExternal_ConstructsReaderWithOnlyPublicTypes(t *testing.T) {
	cap := &externalFakeCapabilities{
		activeClasses: []resourcecore.CatalogRecord{{Kind: resourcecore.KindClass, ID: 1}},
		descriptors:   []resourcecore.CatalogDescriptor{{Kind: resourcecore.KindClass}},
	}
	reader, err := resourcecore.NewReadOnly(cap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	classes, err := reader.ActiveClasses(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(classes) != 1 || classes[0].Kind != resourcecore.KindClass {
		t.Fatalf("unexpected classes: %+v", classes)
	}

	descs, err := reader.CatalogDescriptors(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(descs) != 1 {
		t.Fatalf("unexpected descriptors: %+v", descs)
	}

	page, err := reader.ListCatalog(context.Background(), resourcecore.CatalogQuery{Kind: resourcecore.KindClass})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Records) != 1 || page.Records[0].Kind != resourcecore.KindClass {
		t.Fatalf("unexpected page: %+v", page)
	}

	rec, err := reader.GetCatalog(context.Background(), resourcecore.CatalogKey{Kind: resourcecore.KindClass, ID: 7})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.ID != 7 {
		t.Fatalf("unexpected record: %+v", rec)
	}

	rpage, err := reader.SearchResources(context.Background(), resourcecore.ResourceQuery{Scope: resourcecore.ScopeActive})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rpage.Resources) != 1 {
		t.Fatalf("unexpected resources: %+v", rpage)
	}

	res, err := reader.GetResource(context.Background(), resourcecore.ResourceKey{ClassCode: "MAT", IdentityV1: "v1|x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Scope.ClassCode != "MAT" {
		t.Fatalf("unexpected resource: %+v", res)
	}

	desc, err := reader.DescribeResource(context.Background(), resourcecore.ResourceKey{ClassCode: "MAT", IdentityV1: "v1|x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if desc != "described" {
		t.Fatalf("unexpected description: %q", desc)
	}
}

type externalFakeWriteCapabilities struct{}

func (f *externalFakeWriteCapabilities) CreateCatalog(ctx context.Context, req resourcecore.CatalogWriteRequest) (resourcecore.CatalogRecord, error) {
	return resourcecore.CatalogRecord{Kind: req.Kind, ID: 1, Revision: 1, Values: req.Values}, nil
}

func (f *externalFakeWriteCapabilities) CreateResource(ctx context.Context, req resourcecore.ResourceWriteRequest) (resourcecore.Resource, error) {
	return resourcecore.Resource{ID: 1, IdentityV1: "v1|x", Scope: req.Scope, Revision: 1, Attributes: req.Attributes}, nil
}

func (f *externalFakeWriteCapabilities) UpdateCatalog(ctx context.Context, req resourcecore.CatalogUpdateRequest) (resourcecore.CatalogRecord, error) {
	return resourcecore.CatalogRecord{Kind: req.Kind, ID: req.ID, Revision: req.ExpectedRevision + 1, Values: req.Values}, nil
}

func (f *externalFakeWriteCapabilities) UpdateResource(ctx context.Context, req resourcecore.ResourceUpdateRequest) (resourcecore.Resource, error) {
	return resourcecore.Resource{ID: req.ID, IdentityV1: "v1|x", Scope: req.Scope, Revision: req.ExpectedRevision + 1, Attributes: req.Attributes}, nil
}

func (f *externalFakeWriteCapabilities) DeactivateCatalog(ctx context.Context, req resourcecore.CatalogLifecycleRequest) (resourcecore.CatalogRecord, error) {
	return resourcecore.CatalogRecord{Kind: req.Kind, ID: req.ID, Revision: req.ExpectedRevision + 1, Active: false}, nil
}

func (f *externalFakeWriteCapabilities) ReactivateCatalog(ctx context.Context, req resourcecore.CatalogLifecycleRequest) (resourcecore.CatalogRecord, error) {
	return resourcecore.CatalogRecord{Kind: req.Kind, ID: req.ID, Revision: req.ExpectedRevision + 1, Active: true}, nil
}

func (f *externalFakeWriteCapabilities) DeactivateResource(ctx context.Context, req resourcecore.ResourceLifecycleRequest) (resourcecore.Resource, error) {
	return resourcecore.Resource{ID: req.ID, IdentityV1: "v1|x", Revision: req.ExpectedRevision + 1, Active: false}, nil
}

func (f *externalFakeWriteCapabilities) ReactivateResource(ctx context.Context, req resourcecore.ResourceLifecycleRequest) (resourcecore.Resource, error) {
	return resourcecore.Resource{ID: req.ID, IdentityV1: "v1|x", Revision: req.ExpectedRevision + 1, Active: true}, nil
}

func (f *externalFakeWriteCapabilities) HardDeleteCatalog(ctx context.Context, req resourcecore.CatalogLifecycleRequest) error {
	return nil
}

func (f *externalFakeWriteCapabilities) UpdateAttributeOrder(ctx context.Context, req resourcecore.AttributeOrderWriteRequest) (resourcecore.ResourceAttributeOrder, error) {
	return resourcecore.ResourceAttributeOrder{Scope: req.Scope, OrderedAttributes: req.OrderedAttributes, OrderRevision: "v2:def"}, nil
}

func TestExternalWrite_ConsumerConstructsWriterUsingPublicTypesOnly(t *testing.T) {
	writer, err := resourcecore.NewWriter(&externalFakeWriteCapabilities{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rec, err := writer.CreateCatalog(context.Background(), resourcecore.CatalogWriteRequest{
		Actor: "PI",
		Kind:  resourcecore.KindClass,
		Values: map[string]resourcecore.Value{
			"code": {Kind: resourcecore.ValueCode, Text: "MAT"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.ID != 1 || rec.Revision != 1 {
		t.Fatalf("unexpected catalog record: %+v", rec)
	}

	res, err := writer.CreateResource(context.Background(), resourcecore.ResourceWriteRequest{
		Actor:       "PI",
		Scope:       resourcecore.ResourceScope{ClassCode: "MAT", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"},
		NaturalUnit: "m",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ID != 1 || res.IdentityV1 != "v1|x" {
		t.Fatalf("unexpected resource: %+v", res)
	}
}

func TestExternalWrite_ConsumerUpdatesCatalogAndResourceUsingPublicTypesOnly(t *testing.T) {
	writer, err := resourcecore.NewWriter(&externalFakeWriteCapabilities{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rec, err := writer.UpdateCatalog(context.Background(), resourcecore.CatalogUpdateRequest{
		Actor:            "PI",
		Kind:             resourcecore.KindClass,
		ID:               1,
		ExpectedRevision: 1,
		Values: map[string]resourcecore.Value{
			"code": {Kind: resourcecore.ValueCode, Text: "MAT"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.ID != 1 || rec.Revision <= 1 {
		t.Fatalf("unexpected catalog record: %+v", rec)
	}

	res, err := writer.UpdateResource(context.Background(), resourcecore.ResourceUpdateRequest{
		Actor:            "PI",
		ID:               1,
		ExpectedRevision: 1,
		Scope:            resourcecore.ResourceScope{ClassCode: "MAT", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"},
		NaturalUnit:      "m",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ID != 1 || res.Revision <= 1 {
		t.Fatalf("unexpected resource: %+v", res)
	}
}

func TestExternalWrite_ConsumerDeactivatesAndReactivatesCatalogAndResourceUnderCAS(t *testing.T) {
	writer, err := resourcecore.NewWriter(&externalFakeWriteCapabilities{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rec, err := writer.DeactivateCatalog(context.Background(), resourcecore.CatalogLifecycleRequest{
		Actor: "PI", Kind: resourcecore.KindClass, ID: 1, ExpectedRevision: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Active {
		t.Fatalf("expected deactivated catalog record, got %+v", rec)
	}

	res, err := writer.DeactivateResource(context.Background(), resourcecore.ResourceLifecycleRequest{
		Actor: "PI", ID: 1, ExpectedRevision: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Active {
		t.Fatalf("expected deactivated resource, got %+v", res)
	}
}

func TestExternalWrite_ConsumerReactivatesPreviouslyDeactivatedCatalogAndResource(t *testing.T) {
	writer, err := resourcecore.NewWriter(&externalFakeWriteCapabilities{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rec, err := writer.ReactivateCatalog(context.Background(), resourcecore.CatalogLifecycleRequest{
		Actor: "PI", Kind: resourcecore.KindClass, ID: 1, ExpectedRevision: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rec.Active {
		t.Fatalf("expected reactivated catalog record, got %+v", rec)
	}

	res, err := writer.ReactivateResource(context.Background(), resourcecore.ResourceLifecycleRequest{
		Actor: "PI", ID: 1, ExpectedRevision: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Active {
		t.Fatalf("expected reactivated resource, got %+v", res)
	}
}

func TestExternalWrite_ConsumerHardDeletesInactiveUnreferencedCatalog(t *testing.T) {
	writer, err := resourcecore.NewWriter(&externalFakeWriteCapabilities{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = writer.HardDeleteCatalog(context.Background(), resourcecore.CatalogLifecycleRequest{
		Actor: "PI", Kind: resourcecore.KindClass, ID: 1, ExpectedRevision: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExternal_NoInternalImports(t *testing.T) {
	// This test lives in resourcecore_test so it can only reference exported
	// symbols from the public module package. Compilation success is the proof.
	_ = resourcecore.KindClass
	_ = resourcecore.ScopeActive
	_ = resourcecore.InvalidArgument
}
