package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/GARFEX33/garfex-costos-unitarios/resourcecore"
)

type catalogReaderFunc func(context.Context) ([]resourcecore.CatalogDescriptor, error)

func (f catalogReaderFunc) CatalogDescriptors(ctx context.Context) ([]resourcecore.CatalogDescriptor, error) {
	return f(ctx)
}

func (catalogReaderFunc) ListCatalog(context.Context, resourcecore.CatalogQuery) (resourcecore.CatalogPage, error) {
	return resourcecore.CatalogPage{}, resourcecore.NewError(resourcecore.Internal, "list catalog unavailable")
}

func (catalogReaderFunc) GetCatalog(context.Context, resourcecore.CatalogKey) (resourcecore.CatalogRecord, error) {
	return resourcecore.CatalogRecord{}, resourcecore.NewError(resourcecore.Internal, "get catalog unavailable")
}

func TestCatalogDescriptorsMapsCompleteMetadata(t *testing.T) {
	key := struct{}{}
	descriptors := []resourcecore.CatalogDescriptor{{
		Kind: "CHILD", Singular: "child", Plural: "children", IdentityFields: []string{"code"}, ParentKind: "PARENT", ParentField: "parent_id",
		Fields: []resourcecore.FieldDescriptor{{
			Name: "code", Label: "Code", Kind: resourcecore.ValueEnum, Required: true, RefKind: "REFERENCE", RefScopedBy: []string{"parent_id"},
			EnumValues: []resourcecore.EnumValue{{Value: "A", Label: "Alpha"}},
		}, {Name: "note", Label: "Note", Kind: resourcecore.ValueText}},
	}}
	h := NewRouter(catalogReaderFunc(func(ctx context.Context) ([]resourcecore.CatalogDescriptor, error) {
		if ctx.Value(key) != "propagated" {
			t.Error("request context was not propagated")
		}
		return descriptors, nil
	}))
	r := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/catalog/descriptors", nil).WithContext(context.WithValue(context.Background(), key, "propagated"))
	h.ServeHTTP(r, req)

	if r.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", r.Code)
	}
	var got catalogDescriptorsResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	want := catalogDescriptorsResponse{Descriptors: []catalogDescriptorResponse{{
		Kind: "CHILD", Singular: "child", Plural: "children", IdentityFields: []string{"code"}, ParentKind: "PARENT", ParentField: "parent_id",
		Fields: []fieldDescriptorResponse{{
			Name: "code", Label: "Code", Kind: "ENUM", Required: true, RefKind: "REFERENCE", RefScopedBy: []string{"parent_id"},
			EnumValues: []enumValueResponse{{Value: "A", Label: "Alpha"}},
		}, {Name: "note", Label: "Note", Kind: "TEXT", RefScopedBy: []string{}, EnumValues: []enumValueResponse{}}},
	}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("response = %#v, want %#v", got, want)
	}
}

func TestCatalogDescriptorsNormalizesNilSlice(t *testing.T) {
	h := NewRouter(catalogReaderFunc(func(context.Context) ([]resourcecore.CatalogDescriptor, error) { return nil, nil }))
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/catalog/descriptors", nil))
	var got catalogDescriptorsResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Descriptors == nil || len(got.Descriptors) != 0 {
		t.Fatalf("descriptors = %#v, want empty non-nil slice", got.Descriptors)
	}
}

func TestCatalogDescriptorsMapsAndSanitizesCoreErrors(t *testing.T) {
	for _, tc := range []struct {
		code    resourcecore.ErrorCode
		status  int
		message string
	}{
		{resourcecore.InvalidArgument, 400, "invalid request"}, {resourcecore.NotFound, 404, "not found"},
		{resourcecore.Duplicate, 409, "conflict"}, {resourcecore.Integrity, 409, "conflict"}, {resourcecore.IdentityConflict, 409, "conflict"},
		{resourcecore.InvalidLifecycle, 409, "conflict"}, {resourcecore.ReactivationImpossible, 409, "conflict"}, {resourcecore.InUse, 409, "conflict"},
		{resourcecore.ImmutableCode, 409, "conflict"}, {resourcecore.Conflict, 409, "conflict"}, {resourcecore.InvalidReference, 422, "validation failed"},
		{resourcecore.Validation, 422, "validation failed"}, {resourcecore.InvalidCatalog, 422, "validation failed"}, {resourcecore.Unavailable, 503, "unavailable"},
		{resourcecore.Internal, 500, "internal server error"},
	} {
		t.Run(string(tc.code), func(t *testing.T) {
			h := NewRouter(catalogReaderFunc(func(context.Context) ([]resourcecore.CatalogDescriptor, error) {
				return nil, resourcecore.NewError(tc.code, "postgres://user:secret@host/db")
			}))
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/catalog/descriptors", nil))
			var got errorResponse
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			if r.Code != tc.status || got.Error != tc.message {
				t.Fatalf("status/error = %d/%q, want %d/%q", r.Code, got.Error, tc.status, tc.message)
			}
		})
	}
	if status, _ := catalogError(errors.New("secret")); status != http.StatusInternalServerError {
		t.Fatalf("unknown error status = %d, want 500", status)
	}
}

func TestCatalogDescriptorsRequiresGET(t *testing.T) {
	called := false
	h := NewRouter(catalogReaderFunc(func(context.Context) ([]resourcecore.CatalogDescriptor, error) { called = true; return nil, nil }))
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/catalog/descriptors", nil))
	var got errorResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusMethodNotAllowed || r.Header().Get("Allow") != http.MethodGet || got.Error != "method not allowed" || called {
		t.Fatalf("status/allow/error/called = %d/%q/%q/%t", r.Code, r.Header().Get("Allow"), got.Error, called)
	}
}
