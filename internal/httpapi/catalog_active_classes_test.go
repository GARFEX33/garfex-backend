package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GARFEX33/garfex-costos-unitarios/resourcecore"
)

type activeClassesReaderFunc func(context.Context) ([]resourcecore.CatalogRecord, error)

func (f activeClassesReaderFunc) CatalogDescriptors(context.Context) ([]resourcecore.CatalogDescriptor, error) {
	return nil, resourcecore.NewError(resourcecore.Internal, "descriptors unavailable")
}

func (activeClassesReaderFunc) ListCatalog(context.Context, resourcecore.CatalogQuery) (resourcecore.CatalogPage, error) {
	return resourcecore.CatalogPage{}, resourcecore.NewError(resourcecore.Internal, "list catalog unavailable")
}

func (activeClassesReaderFunc) GetCatalog(context.Context, resourcecore.CatalogKey) (resourcecore.CatalogRecord, error) {
	return resourcecore.CatalogRecord{}, resourcecore.NewError(resourcecore.Internal, "get catalog unavailable")
}

func (f activeClassesReaderFunc) ActiveClasses(ctx context.Context) ([]resourcecore.CatalogRecord, error) {
	return f(ctx)
}

func TestActiveClassesMapsRecords(t *testing.T) {
	reader := activeClassesReaderFunc(func(context.Context) ([]resourcecore.CatalogRecord, error) {
		return []resourcecore.CatalogRecord{
			{Kind: resourcecore.KindClass, ID: 1, Revision: 1, Active: true, Values: map[string]resourcecore.Value{"nombre": {Kind: resourcecore.ValueText, Text: "Acero"}}},
		}, nil
	})
	h := NewRouter(reader, nil, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/catalog/classes", nil))
	if r.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", r.Code, r.Body.String())
	}
	var response struct {
		Records []struct {
			Kind string `json:"kind"`
		} `json:"records"`
	}
	if err := json.NewDecoder(r.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if len(response.Records) != 1 || response.Records[0].Kind != "CLASE" {
		t.Fatalf("response = %#v", response)
	}
}

func TestActiveClassesNormalizesNilSlice(t *testing.T) {
	reader := activeClassesReaderFunc(func(context.Context) ([]resourcecore.CatalogRecord, error) { return nil, nil })
	h := NewRouter(reader, nil, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/catalog/classes", nil))
	var response activeClassesResponse
	if err := json.NewDecoder(r.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Records == nil || len(response.Records) != 0 {
		t.Fatalf("records = %#v, want empty non-nil slice", response.Records)
	}
}

func TestActiveClassesSanitizesNilReaderAndCoreErrors(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/catalog/classes", nil))
	var got errorResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusInternalServerError || got.Error != "internal server error" {
		t.Fatalf("status/error = %d/%q", r.Code, got.Error)
	}

	reader := activeClassesReaderFunc(func(context.Context) ([]resourcecore.CatalogRecord, error) {
		return nil, resourcecore.NewError(resourcecore.Unavailable, "postgres://user:secret@host/db")
	})
	h = NewRouter(reader, nil, nil, nil, nil, nil)
	r = httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/catalog/classes", nil))
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusServiceUnavailable || got.Error != "unavailable" {
		t.Fatalf("status/error = %d/%q", r.Code, got.Error)
	}
}

func TestActiveClassesRejectsWrongMethod(t *testing.T) {
	h := NewRouter(activeClassesReaderFunc(nil), nil, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/catalog/classes", nil))
	if r.Code != http.StatusMethodNotAllowed || r.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("status/allow = %d/%q", r.Code, r.Header().Get("Allow"))
	}
}
